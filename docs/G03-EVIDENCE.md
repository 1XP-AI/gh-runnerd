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

At the time of this record the G01/G02 experiment modules were in their separate review worktrees, so the current branch's `make experiments` output is an explicit absence message. The same target is included in public CI and runs only the reviewed offline module paths after those commits are integrated, with `go test -race` and `go vet`; it never enables the `g02runtime` build tag or invokes a live experiment.

For integration coverage, temporary links to those reviewed modules were used solely for the check; both completed `go test -race -count=1` and `go vet`, ending with `offline experiment checks passed: 2 module(s)`. The links were removed after the run.

The fuzz command was also checked with a disposable valid `FuzzSmokeFixture`. `make fuzz-smoke FUZZTIME=1s` discovered `FuzzSmokeFixture`, ran it for the one-second smoke window, and returned `PASS`; the fixture was removed afterward. The repository currently has no product fuzz target, so the normal public run reports a visible skip rather than claiming fuzz coverage.

## Gaps and rollback

The bootstrap package has no product behavior tests yet; `go test` and `go test -race` therefore compile the package and visibly report `[no test files]`. G04 owns the first behavior contracts and should add meaningful unit and fuzz targets. Live GitHub, Docker, native hardware, credential and daemon suites remain explicit maintainer-controlled profiles outside this public workflow.

G03 has no runtime migration or runner lifecycle effect. Reverting the focused commit is sufficient rollback; no runner cleanup or host restoration is needed.
