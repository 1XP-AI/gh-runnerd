# G03 tooling audit correction

Tracks [audit issue #30](https://github.com/1XP-AI/gh-runnerd/issues/30) and the
[G03 baseline](https://github.com/1XP-AI/gh-runnerd/issues/3).
This correction addresses the five P2 findings posted on merged PR23:

- [GOFLAGS placement](https://github.com/1XP-AI/gh-runnerd/pull/23#discussion_r3948855753): Make exports the value to Go's environment.
- [Executable build omission](https://github.com/1XP-AI/gh-runnerd/pull/23#discussion_r3948855773): local `make check` and hosted CI both link the executable.
- [Stale license identity](https://github.com/1XP-AI/gh-runnerd/pull/23#discussion_r3948855748): the runtime inventory must exactly match selected module paths, versions and replacements, with no stale or duplicate rows.
- [Missing established experiment](https://github.com/1XP-AI/gh-runnerd/pull/23#discussion_r3948855767): both landed G01/G02 modules are mandatory before suites run.
- [Newer local Go selection](https://github.com/1XP-AI/gh-runnerd/pull/23#discussion_r3948855742): Make and the standalone toolchain check explicitly select `go1.26.8`; equality validation remains.

The separately approved hosted-test addition explicitly runs race tests and vet
for the reviewed G01 CLI packages with `g01_live,g01_worker` after the default G01
suite. These are synthetic tests only; this does not enable a live command,
platform probe, self-hosted runner or secret-bearing workflow.

## Actual red evidence

Commit `a62f9ea` adds six tooling regressions before the implementation. Running
`GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=120s -run '^TestTooling' ./scripts`
failed all six groups (`2.812s`):

- The conventional `GOFLAGS=-mod=readonly` setting failed build, vet and tests.
- A controlled newer-installation fixture reported Go 1.27.1 under `auto` and
  the real check rejected it instead of selecting the pin. No real newer Go was
  downloaded or executed; explicitly selecting the pin delegates to installed Go.
- A valid positive `make check` fixture passed, then the same suite accepted a
  command whose main function references an undefined assembly symbol.
- Removing either established module's `go.mod` still returned success.
- A deliberately failing G01 CLI test behind both tags was skipped.
- An actual local Go module graph with version `v1.2.0` and a local replacement
  accepted an inventory with version `v1.1.0`, a missing replacement, stale module
  rows or duplicate rows. The matching inventory passed as a positive control.

Fixtures use temporary files, local modules, real Go/Make commands and test
executables. The orchestration fixture substitutes a no-op vulnerability stage
to avoid making that unrelated database request inside every unit test; the
repository's complete validation still runs its real pinned vulnerability scan.
All fixture files are owned and removed by Go's test cleanup.

## Validation and limits

Focused tooling tests passed (`3.759s`) and the race run passed (`5.099s`).
The full plain `make check` selected Go 1.26.8 and passed: executable build,
formatting/vet, root tests (`3.553s`) and race (`4.283s`), dependency/license checks,
both offline modules, the tagged G01 CLI tests/vet, and pinned root vulnerability
scan with no vulnerabilities found. Fuzz explicitly skipped because no target
exists. The CLI packages passed (`1.765s` and `1.422s`) using synthetic fixtures;
the G02 library passed (`5.249s`). Shell syntax and diff whitespace checks passed.
An additional focused check verifies the Make build recipe itself receives the
pin, so the standalone checker's own selection cannot mask a Make regression.

Independent review, external Codex review of the exact final PR head and hosted
CI are still required.
This change does not resolve GitHub review threads or establish any live G01/G02
gate. License checking verifies recorded dependency identity and file presence;
the actual license interpretation remains a human review responsibility.
