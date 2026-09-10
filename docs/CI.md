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

The hosted workflow keeps the required public check name `Go checks` as a
status aggregator over three bounded jobs: `Root and tooling checks` runs the
root Make checks through `make licenses`, `Offline experiment checks` runs
`make experiments`, and `Vulnerability check` runs `make vuln`. Every
test-bearing job has its own 15-minute cap, the same pinned checkout/setup-go
actions, an explicit immutable pull-request head ref, and setup-go caching
disabled. The aggregator uses `always()` so a failed, cancelled, or skipped
required job is observed, then fails unless all three dependency results are
exactly `success`; it does not check out source or run a test itself.

The pull-request workflow is the premerge source gate. The identical workflow on
`main` is postmerge integration evidence; a green postmerge run cannot substitute
for the exact-head premerge gate. During local editing, use focused checks and the
opt-in `make fast` selector below. After source, documentation and finding-ledger
changes are batched into one stable candidate, hosted CI supplies the complete
matrix; reviewers and the coordinator use that exact-source evidence rather than
rerunning the full suite independently. Release, macOS, soak and other trusted
profiles run only before an applicable release/live qualification, with explicit
maintainer authorization, and are not implied by this hosted check.

### Issue #73 audit evidence

This audit was captured from `origin/main` at
`cf67d4aeb511116fee0de31a4ac38409f28fa29f` and the live PR #72 record on
2026-09-10. It records measured observations, not a forecast of savings:

| Surface | Measured observation | Policy consequence |
| --- | --- | --- |
| Main CI | `ci.yml` has three parallel test-bearing jobs (`root`, `offline`, `vuln`) plus the required `Go checks` aggregator; each remains capped at 15 minutes, with pinned actions and `cache: false`. | Preserve the current complete coverage, job/check names, cache policy and timeouts; no path classifier or cache shortcut was added. |
| Makefile | Existing `check` prerequisites remain `toolchain`, formatting, build, vet, unit/race, fuzz, dependency, license, offline-experiment and vulnerability checks. New `fast` is an opt-in target and is not a `check` prerequisite. | Keep `make check` complete and unchanged as the local public gate; focused iteration cannot silently weaken it. |
| [PR #72 review history](https://github.com/1XP-AI/gh-runnerd/pull/72) | Audit snapshot through immutable PR #72 head `116beda04dc2bf69280cdefc4de4ef2fef397ef3`, captured 2026-09-10. Review records and public checkpoints do not measure actor-side full-suite run counts; the checkpoint comments identify focused/offline or focused race regressions. | Batch findings, source and docs before one candidate push; reviewers perform delta/risk probes against shared CI evidence, and the coordinator audits rather than acting as a third tester. |
| [PR #72 hosted critical path](https://github.com/1XP-AI/gh-runnerd/actions/runs/34419651240) | `gh run view 34419651240 --json jobs` measured workflow start `00:04:06Z`, required jobs finishing by `00:15:14Z`, and aggregator completion at `00:15:18Z`; `00:15:19Z` is a workflow metadata update, not completion. Start-to-aggregator completion was 11m12s. Root ran 11m06s, offline 8m49s, vulnerability 33s, aggregator 2s. | No workflow critical-path speedup is claimed or changed; full CI remains the stable candidate gate. |
| Focused local command | Warmed direct baseline: `env GOTOOLCHAIN=go1.26.8 GOWORK=off go test -count=1 -run '^TestFixedTarget$' ./internal/scheduler/capacity` → `real 0.32s`. New entry point: `env FAST_MODULE=. FAST_PACKAGE=./internal/scheduler/capacity FAST_TEST='^TestFixedTarget$' make fast` → `real 0.34s`; both passed. | This one local pair demonstrates bounded behavior only; it does not claim a speedup or predict CI duration. |

```console
$ gh run view 34419651240 --repo 1XP-AI/gh-runnerd --json headSha,startedAt,updatedAt,jobs
# head 116beda04dc2bf69280cdefc4de4ef2fef397ef3; start 00:04:06Z; aggregator 00:15:18Z; metadata update 00:15:19Z;
# jobs: root 00:04:08Z-00:15:14Z, offline 00:04:09Z-00:12:58Z,
# vuln 00:04:10Z-00:04:43Z, checks 00:15:16Z-00:15:18Z
```

The audit commands and exact observed values are kept in this single summary
instead of accumulating per-step logs. Finding ledgers must retain original URLs,
source SHAs and resolution evidence without copying credentials or private job
logs.

Run the same checks locally with:

```console
make check
```

`make check` remains the complete local public gate, but it is not a per-commit
requirement; use it on demand when the environment supports it and rely on hosted
CI for the stable candidate gate.

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
| `make fast FAST_MODULE=... FAST_PACKAGE=... FAST_TEST=...` | Run one explicit test selector in one selected module/package. All three selectors are required and invalid/no-match selectors fail closed; this is focused evidence only, never the complete gate. |

For example:

```console
FAST_MODULE=. FAST_PACKAGE=./internal/scheduler/capacity FAST_TEST='^TestFixedTarget$' make fast
```

The script resolves the selected module and every package matched by the Go
pattern, preserving wildcard forms such as `./.../capacity`, and rejects
selectors whose physical paths leave the current repository or selected module,
as well as absolute or lexical `..` paths, missing `go.mod`, missing selectors,
package patterns that match no package, and selectors that match no compiled
test. The focused invocation explicitly bounds Go's test execution flags:
inherited `-bench` and `-fuzz` selectors are cleared so they cannot expand work
beyond `FAST_TEST`; `-list`, `-skip`, and build-only `-c` are cleared so they
cannot suppress it; explicit `-run`/`-count` values retain the requested
selector and `-cpu=1` bounds each selected test to one execution. Inherited
`-exec` is cleared so a wrapper cannot bypass the test binary. The command uses
Go's JSON test events and requires a structured per-test `Action=run` event whose
`Test` field is a complete match under Go's slash-separated component and
top-level alternation semantics; possible-parent events do not count. Package
summaries and arbitrary `TestMain` output do not count, and dry-run or
unsupported build modes fail closed when no test event is observed. Useful build
flags such as `-race` and `-mod=readonly` remain inherited; this is not a
blanket `GOFLAGS` removal. This is a trusted local helper, not a hostile-code
sandbox. If selectors are passed as Make command-line variables instead of
environment assignments, escape literal `$` as `$$` so Make preserves the
regexp anchor.

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
