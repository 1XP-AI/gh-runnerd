# Public CI and local checks

G03 establishes the reproducible development baseline. The module requires Go 1.26.8 in [go.mod](../go.mod). That directive is a minimum under automatic toolchain selection; Make explicitly exports `GOTOOLCHAIN=go1.26.8` to select the exact version even on a newer installation. No redundant `toolchain` directive is kept. Go's [release policy](https://go.dev/doc/devel/release) and [security guidance](https://go.dev/doc/security/) support security fixes for the two most recent major releases; on 2026-09-07 the official release history lists Go 1.27.1 and Go 1.26.8. The project uses 1.26.8 because it is the latest patch in the supported 1.26 line observed on that date and the released Scale Set SDK baseline requires Go 1.25.3 or newer. Revisit the pin when a newer supported patch is selected and compatibility is rechecked.

Make and standalone `scripts/check-toolchain.sh` select `go1.26.8` explicitly;
they do not change the user's global Go configuration. Go may download that
version into its normal per-user toolchain cache. The hosted workflow uses
`actions/setup-go` to install 1.26.8 from `go.mod` and uses the same explicit
`GOTOOLCHAIN` value. The equality check still refuses an unexpected toolchain.
Conventional settings such as `make GOFLAGS=-mod=readonly build` are exported
through Go's environment, rather than placed before its subcommand.

The public workflow runs on a GitHub-hosted `ubuntu-24.04` environment for `pull_request` and pushes to `main`. It grants only `contents: read`, disables checkout credential persistence, and pins every external action to a reviewed commit. It does not use `pull_request_target`, self-hosted runners, secrets, Docker, the local runner manager or any privileged hardware. A fork can therefore run the same checks without access to organization credentials or local runners.

Run the same checks locally with:

```console
make check
```

Individual commands are available when iterating:

| Command | Check |
| --- | --- |
| `make toolchain` | Confirm the module directives and selected Go toolchain are exactly 1.26.8. |
| `make build` | Link the application executable; included in `make check` and hosted CI. |
| `make fmt` / `make fmt-check` | Format or verify all Go sources with the selected toolchain. |
| `make vet` | Run `go vet ./...`. |
| `make test` | Run uncached `go test ./...`, including synthetic tooling regressions. Application behavior contracts remain future G04 work. |
| `make test-race` | Run uncached `go test -race ./...`, including tooling tests. |
| `make fuzz-smoke` | Run each discovered fuzz target for a fixed one-second smoke window, or print an explicit `SKIPPED` result when no target exists. |
| `make deps` | Require a clean `go mod tidy -diff`, verified module sums and a read-only dependency load. |
| `make licenses` | Compare the exact runtime module/version/replacement graph with its inventory and require a top-level license file. |
| `make experiments` | Require both established G01/G02 modules, run their static-partitioned default race/vet suites, then exercise the two explicitly reviewed G01 CLI packages with `g01_live,g01_worker` tags and the reviewed `g01_pair_fixture` livecanary collection/listener and terminal partitions with tagged vet. |
| `make vuln` | Run the exact `golang.org/x/vuln/cmd/govulncheck@v1.7.0` tool. |

No hardware, live GitHub, Docker or daemon suite is part of this public check. Those profiles remain explicit future or maintainer-controlled runs; they are not silently converted into passing tests here. G04 introduces the first application behavior contracts and should add meaningful unit and fuzz targets before claiming those forms of coverage.

The default untagged G01 and G02 race suites keep their existing 45-second
per-process deadline. G01 uses two sequential static partitions: the exact
`TestBaselineStatisticsPresenceAndEligibility` name through `./...`, then an
unfiltered `./...` with only that exact name skipped. G02 uses four sequential
static partitions, all with `-race -count=1 -timeout=45s` and `./...` package
discovery: the exact `TestPairedBrokerPrepareReviewedG01LiveBinary` fixture
clone/build of the distinct `g01_live,g01_pair_fixture` and
`g01_live,g01_pair_fixture,g01_pair_real_cadence` tagged variants; the exact
`TestPairedBrokerRealCadenceChildExceedsThirtySeconds` name; the remaining
`^TestPaired` family with those exact prep and cadence names skipped; and an
unfiltered complement that skips `^TestPaired`. The default gate exports an
owned `G01_PAIR_BRIDGE_PREP_DIR` and refuses missing or invalid prepared
receipts instead of rebuilding inside the cadence process. Standalone
clone/build remains only when that variable is unset. Cleanup of that owned
mktemp path is registered immediately after `mktemp` and before `chmod`. An
EXIT trap removes only that path and does not exit from the handler, so
success and command-failure status are preserved. Explicit INT, TERM, and
HUP traps remove the same owned path, disarm EXIT, and exit 130, 143, or
129. Keeping package discovery in every command means
a same-named test in another package is run in the matching named partition
rather than silently dropped by a global skip, while the unfiltered G02
complement still executes every ordinary non-paired test, Example Output and
fuzz seed. No widened named-test timeout is part of the public contract. The
production seven-times-five-second wait stays inside the cadence partition;
clone/build is a separate bounded process.

The tooling regression matrix generates positive and independent failing
witnesses for each G02 partition boundary: the named fixture-prep test, the
named cadence test, the remaining `TestPaired` family, remainder, another
package, a same-name prep test in another package, a same-name cadence test in
another package, a same-name remaining `TestPaired` test in another package, an
Example Output and a fuzz seed. Each witness must execute exactly once, and
each failing witness must propagate a nonzero offline-gate result. Owned G02
prep cleanup is covered by success, `exit 91`, ordinary prep failure, and
generated-fixture SIGTERM/SIGINT/SIGHUP cases that assert status 0, 91, 143,
130, or 129 and remove only that mktemp directory.

The tagged CLI tests use synthetic input/subprocess fixtures and static plan or
refusal paths. The `g01_pair_fixture` livecanary checks use private synthetic
fixtures: one paired-collection run selects `^TestPaired` while excluding
`^TestPairedTerminal`; one remaining collection/listener run has no `-run`
filter while excluding `^TestPaired`; one heavy terminal run selects the exact
five names
`^TestPairedTerminal(FinalResultCapacity|PendingChildCapacity|EligibilityUsesFreshExactFacts|CapturedAcknowledgementCancellation|MissingAcknowledgementsAndPostchecks)$`;
one complementary remainder selects `^TestPairedTerminal` while skipping those
five names and the persistence set; and a fifth run selects the exact
persistence set
`^TestPairedTerminal(Actual(Controller|Worker)SyncFailures|PostIntent(JournalIdentity|AuthorityBoundaries)|ClosedReplayActualFile|WorkerReceiptSurvivesControllerWriteFailure|FixtureStorageFailure)$`.
`FixtureStorageFailure` is the unique generated sentinel from
`scripts/tooling_test.go`; it is intentionally distinct from the real
`TestPairedTerminalClosedReplayActualFile` test. This exact expression is the
script's `storage_regex` value and is also the `STORAGE` alias in issue #54.
The two collection/listener partitions are explicit and disjoint: every
non-terminal `TestPaired*` test is in the first, and every other test, example or
fuzz seed is in the second. The three terminal partitions are likewise
exhaustive and disjoint for current `TestPairedTerminal` names, and a future
top-level terminal name lands in the remainder. The unfiltered second command
preserves Go's normal execution of tagged examples and fuzz seeds. One tagged
vet follows those five race-tested runs. None of these tagged checks
executes approved live controller/worker operations or exposes a public terminal
phase/API. The implementation and evidence boundaries are recorded in the
[G01 paired terminal guide](evidence/g01-paired-terminal.md). The script permits
only `cmd/g01-live`, `cmd/g01-worker` and the reviewed `livecanary` package; it
does not discover other opt-in commands or enable the `g02runtime` Keychain probe.

There is no runtime migration or runner cleanup to roll back in G03. If the workflow or toolchain baseline causes a hosted failure, revert the G03 commit or make the focused workflow/module correction under a reviewed follow-up; no runner enrollment, daemon installation or host cleanup is required.
