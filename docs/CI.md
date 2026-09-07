# Public CI and local checks

G03 establishes the reproducible development baseline. The module requires Go 1.26.8 in [go.mod](../go.mod); the exact patch in the `go` directive is the pin, so no redundant `toolchain` directive is kept. Go's [release policy](https://go.dev/doc/devel/release) and [security guidance](https://go.dev/doc/security/) support security fixes for the two most recent major releases; on 2026-09-07 the official release history lists Go 1.27.1 and Go 1.26.8. The project uses 1.26.8 because it is the latest patch in the supported 1.26 line observed on that date and the released Scale Set SDK baseline requires Go 1.25.3 or newer. Revisit the pin when a newer supported patch is selected and compatibility is rechecked.

The local Go installation is currently 1.25.8. With the default `GOTOOLCHAIN=auto`, a command run from this module downloads the exact `go1.26.8` toolchain into Go's toolchain cache; this is scoped to Go's normal cache and does not install a system toolchain or change host configuration. The hosted workflow uses `actions/setup-go` to install 1.26.8 from `go.mod`, then sets `GOTOOLCHAIN=local` so every check uses that selected version.

The public workflow runs on a GitHub-hosted `ubuntu-24.04` environment for `pull_request` and pushes to `main`. It grants only `contents: read`, disables checkout credential persistence, and pins every external action to a reviewed commit. It does not use `pull_request_target`, self-hosted runners, secrets, Docker, the local runner manager or any privileged hardware. A fork can therefore run the same checks without access to organization credentials or local runners.

Run the same checks locally with:

```console
make check
```

Individual commands are available when iterating:

| Command | Check |
| --- | --- |
| `make toolchain` | Confirm the module directives and selected Go toolchain are exactly 1.26.8. |
| `make fmt` / `make fmt-check` | Format or verify all Go sources with the selected toolchain. |
| `make vet` | Run `go vet ./...`. |
| `make test` | Run uncached `go test ./...`; the bootstrap package currently has no behavior tests, so this is a compile-only check until G04 contracts land. |
| `make test-race` | Run uncached `go test -race ./...`; the current package has no concurrent behavior tests yet. |
| `make fuzz-smoke` | Run each discovered fuzz target for a fixed one-second smoke window, or print an explicit `SKIPPED` result when no target exists. |
| `make deps` | Require a clean `go mod tidy -diff`, verified module sums and a read-only dependency load. |
| `make licenses` | Verify every module in the runtime graph has an inventory row and a top-level license file. |
| `make experiments` | Run race and vet checks for the explicitly listed G01/G02 offline modules when those reviewed modules are present; otherwise report a visible skip. |
| `make vuln` | Run the exact `golang.org/x/vuln/cmd/govulncheck@v1.7.0` tool. |

No hardware, live GitHub, Docker or daemon suite is part of this public check. Those profiles remain explicit future or maintainer-controlled runs; they are not silently converted into passing tests here. G04 introduces the first application behavior contracts and should add meaningful unit and fuzz targets before claiming those forms of coverage.

There is no runtime migration or runner cleanup to roll back in G03. If the workflow or toolchain baseline causes a hosted failure, revert the G03 commit or make the focused workflow/module correction under a reviewed follow-up; no runner enrollment, daemon installation or host cleanup is required.
