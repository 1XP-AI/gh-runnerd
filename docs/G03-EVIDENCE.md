# G03 validation record

Checked 2026-09-07 in the dedicated G03 worktree on macOS `darwin/arm64`. The host's default Go is `go1.25.8`; running from this module with `GOTOOLCHAIN=auto` selected `go1.26.8` from the exact `go 1.26.8` directive. No system toolchain, Docker daemon, runner, credential or daemon installation was used.

## Red evidence

Before adding any G03 module or package files, a disposable malformed Go source containing `func Broken( {` was passed to `gofmt -d`. The formatter produced its parser error and exited 2:

```text
/dev/fd/11:3:14: expected ')', found '{'
gofmt exit: 2
```

The fixture was removed after the check and is not part of the repository. After the formatter wrapper was implemented, the same malformed shape was also run through `make fmt-check`; it printed `work/g03-negative-fixture/broken.go:3:14: expected ')', found '{'`, returned `make: *** [fmt-check] Error 2`, and preserved the non-zero parser status. These checks verify that a formatter parse failure cannot be mistaken for an empty clean result.

## Green evidence

The following commands passed after the fixture was removed:

```text
make toolchain
make fmt-check
make build
make vet
make test
make test-race
make fuzz-smoke
make deps
make licenses
make experiments
make vuln
```

Representative results were:

```text
toolchain: go1.26.8
?    github.com/1XP-AI/gh-runnerd/cmd/gh-runnerd    [no test files]
SKIPPED: no Go fuzz targets are present; this bootstrap does not claim fuzz coverage.
all modules verified
license inventory verified: 1 module(s)
No vulnerabilities found.
```

The initial G03 branch recorded both experiment paths as absent. After G01 was integrated on `main` at `1396e20`, the combined `make check` run executed its reviewed module and printed:

```text
offline experiment: experiments/g01-scaleset (toolchain=go1.26.8)
ok   github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset  1.503s
SKIPPED: experiments/g02-auth is not present in this checkout.
offline experiment checks passed: 1 module(s)
```

The same target is included in public CI and runs only the reviewed offline module paths after those commits are integrated, with `go test -race` and `go vet`; it never enables the `g02runtime` build tag or invokes a live experiment.

For integration coverage, temporary links to those reviewed modules were used solely for the check; both completed `go test -race -count=1` and `go vet`, ending with `offline experiment checks passed: 2 module(s)`. The links were removed after the run.

The fuzz command was also checked with a disposable valid `FuzzSmokeFixture`. `make fuzz-smoke FUZZTIME=1s` discovered `FuzzSmokeFixture`, ran it for the one-second smoke window, and returned `PASS`; the fixture was removed afterward. The repository currently has no product fuzz target, so the normal public run reports a visible skip rather than claiming fuzz coverage.

## Independent review corrections

An independent Astra `xhigh` review identified that source-text fuzz discovery
could report execution for a build-excluded target, and that `rg` was an
undeclared prerequisite on the hosted image. The integrator reproduced the
first problem before fixing it: a disposable target guarded by an absent build
tag printed `[no test files]` followed by `RAN: 1`, while exiting zero.

Discovery now uses Go's compiled test listing, with checked package/compilation
exit status. The same excluded-target fixture reports `SKIPPED`; adding one real
`FuzzActive` target and an ordinary `Fuzzhelper` function runs exactly one target.
That smoke run completed seed coverage and 1,221,073 fuzz executions before
`PASS`. A malformed active test then made discovery exit 1 without `RAN` or
`SKIPPED`. All disposable fixtures were removed. These are script validation
results, not product fuzz coverage.

Formatting uses NUL-delimited Git file discovery and license inventory lookup
uses standard `grep`, so public CI does not require ripgrep. The combined
`make check` suite was rerun after these fixes.

## Gaps and rollback

The bootstrap package has no product behavior tests yet; `go test` and `go test -race` therefore compile the package and visibly report `[no test files]`. G04 owns the first behavior contracts and should add meaningful unit and fuzz targets. Live GitHub, Docker, native hardware, credential and daemon suites remain explicit maintainer-controlled profiles outside this public workflow.

G03 has no runtime migration or runner lifecycle effect. Reverting the focused commit is sufficient rollback; no runner cleanup or host restoration is needed.
