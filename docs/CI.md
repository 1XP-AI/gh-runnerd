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
| `make experiments` | Require both established G01/G02 modules, run their default race/vet suites, then exercise the two explicitly reviewed G01 CLI packages with `g01_live,g01_worker` tags and the reviewed `g01_pair_fixture` livecanary collection/terminal partitions with tagged vet. |
| `make vuln` | Run the exact `golang.org/x/vuln/cmd/govulncheck@v1.7.0` tool. |

No hardware, live GitHub, Docker or daemon suite is part of this public check. Those profiles remain explicit future or maintainer-controlled runs; they are not silently converted into passing tests here. G04 introduces the first application behavior contracts and should add meaningful unit and fuzz targets before claiming those forms of coverage.

The tagged CLI tests use synthetic input/subprocess fixtures and static plan or
refusal paths. The `g01_pair_fixture` livecanary checks use private synthetic
fixtures: one collection run excludes `^TestPairedTerminal`, one terminal run
selects that prefix while skipping the reviewed persistence set, and a third run
selects the exact persistence set
`^TestPairedTerminal(Actual(Controller|Worker)SyncFailures|PostIntent(JournalIdentity|AuthorityBoundaries)|ClosedReplayActualFile|WorkerReceiptSurvivesControllerWriteFailure)$`.
One tagged vet follows those three race-tested runs. None of these tagged checks
executes approved live controller/worker operations or exposes a public terminal
phase/API. The implementation and evidence boundaries are recorded in the
[G01 paired terminal guide](evidence/g01-paired-terminal.md). The script permits
only `cmd/g01-live`, `cmd/g01-worker` and the reviewed `livecanary` package; it
does not discover other opt-in commands or enable the `g02runtime` Keychain probe.

There is no runtime migration or runner cleanup to roll back in G03. If the workflow or toolchain baseline causes a hosted failure, revert the G03 commit or make the focused workflow/module correction under a reviewed follow-up; no runner enrollment, daemon installation or host cleanup is required.
