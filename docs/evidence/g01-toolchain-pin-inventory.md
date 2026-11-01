# Historical G01 toolchain pin inventory — Go 1.26.9 snapshot

## Current status — 2026-11-01

This inventory is a static snapshot of source commit
`9776562fc792e0fb76684fb7be90a7ed118ed0ec`; its Go `1.26.9`, SDK `v0.4.0`
and runner `2.337.0` values are historical, not active pins. The merged profile
is SDK `v0.5.0` with Go `1.27.3` and byte-verified runner `2.338.0` ARM64; see
the [adoption ledger](g01-sdk-v05-adoption.md), [live-canary profile](g01-live-canary.md)
and [runner checkpoint](g01-runner-release-checkpoint.md#2026-10-25-target-offer-archive-and-image-bytes).
Do not use the catalog below for current acceptance or live preparation. G01
remains open, and this note does not qualify runtime or authorize live work.

Source: `9776562fc792e0fb76684fb7be90a7ed118ed0ec`
(`9776562 Clarify Go 1.26.9 red evidence and review`),
branch `orca/issue-1-offline-protocol-signoff`.
Static inventory only; bounded scope, no G01 closure claim.
Writer: Cursor Agent, Muse Spark 1.3 Max, selected by the current user override.
Independent and GitHub review were not requested for this local work.

The current-status links above intentionally point to current documents; source
links in the historical inventory below are pinned to commit
`9776562fc792e0fb76684fb7be90a7ed118ed0ec`.

## Method and boundary

Static file reads plus `rg` text search only. No `go`/`make`/script
execution of any kind, no builds, tests, vet, scans, or advisory queries.
Packet recipes and specimens are inert text: never executed, compiled,
evaluated, or imported. No independent review, GitHub review, pushes, PRs,
merges, or live operations. No source, pin, or `go.sum` edits; no
`git add`/`git commit` (the coordinator owns the index). Only this new
file is added; all historical records are unaltered.

## Historical active pin catalog at source snapshot `9776562`

| # | Anchor | Value |
| --- | --- | --- |
| 1 | Module directives: [root](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/go.mod), [G01](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/experiments/g01-scaleset/go.mod), [G02](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/experiments/g02-auth/go.mod), [R1](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/experiments/r1-credentials/go.mod) | `go 1.26.9` in all four; no `toolchain` directive in any |
| 2 | [Makefile](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/Makefile) | `override GOTOOLCHAIN := go1.26.9`; `GOVULNCHECK_VERSION ?= v1.7.0` |
| 3 | [check-toolchain.sh](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/scripts/check-toolchain.sh) | `expected_toolchain go1.26.9`, `expected_go_directive 1.26.9`; rejects a redundant `toolchain` directive and any mismatched `go version` |
| 4 | [check-offline-experiments.sh](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/scripts/check-offline-experiments.sh) | `exact_toolchain go1.26.9` for every partition (G01 static, G02 static, R1, tagged `g01_live,g01_worker`, five `g01_pair_fixture` livecanary partitions, vet) |
| 5 | [fast-check.sh](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/scripts/fast-check.sh) | `exact_toolchain go1.26.9`, exported with `GOWORK=off` |
| 6 | [compare-sdk.sh](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/experiments/g01-scaleset/compare-sdk.sh) | `export GOTOOLCHAIN=go1.26.9` |
| 7 | [ci.yml](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/.github/workflows/ci.yml) | `GOTOOLCHAIN: go1.26.9` in `root`, `race`, `offline`, `vuln`; `go-version-file: go.mod`; `checkout@3d3c42e5aac5ba805825da76410c181273ba90b1` (`v7.0.1`), `setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e` (`v7.0.0`) |
| 8 | [pr-fast.yml](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/.github/workflows/pr-fast.yml) | `GOTOOLCHAIN: go1.26.9`; same immutable action SHAs; `go-version-file: go.mod` |
| 9 | G01 gates: [live](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/experiments/g01-scaleset/cmd/g01-live/main.go) and [worker](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/experiments/g01-scaleset/cmd/g01-worker/main.go) `buildRevisionFromInfo` | Require `GoVersion == "go1.26.9"` plus a clean 40-hex revision (live additionally requires SDK `v0.4.0` with no replace) |
| 10 | G02 broker [broker_process.go](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/experiments/g02-auth/broker_process.go) `validBrokerBuild` | Requires `go1.26.9`, program `cmd/g01-live`, tags exactly `g01_live`, SDK `v0.4.0` with no replace, matching `GOOS`/`GOARCH`, `CGO_ENABLED=1`, clean expected revision; fixture tags refused |
| 11 | G02 paired fixture [broker_paired_prep_test.go](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/experiments/g02-auth/broker_paired_prep_test.go) | `pairedFixtureGoVersion = "go1.26.9"`; fixture builds compiled with `GOTOOLCHAIN=go1.26.9`; prepared receipt line `go go1.26.9`; `validPairedFixtureBuild` enforces version, path, SDK, tags, platform, cgo, and revision |
| 12 | Tooling harness [tooling_test.go](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/scripts/tooling_test.go) | Fixture child env sets `GOTOOLCHAIN=go1.26.9`; synthetic `go.mod` fixtures declare `go 1.26.9` |
| 13 | govulncheck `v1.7.0` | Makefile default, [script default](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/scripts/govulncheck.sh), [DEPENDENCIES CI-only tool](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/docs/DEPENDENCIES.md#ci-only-vulnerability-tool), [CI command table](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/docs/CI.md) |
| 14 | Dependency/CI prose | [DEPENDENCIES Toolchain](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/docs/DEPENDENCIES.md#toolchain) (`go1.26.9`, `go.mod` declaration, `GOTOOLCHAIN=go1.26.9`, `cache: false`, [matcher notice](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/docs/third-party/go-testing-matcher.md)); [CI.md](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/docs/CI.md) (approved current shared pin Go 1.26.9; `make toolchain` row requires exactly 1.26.9); [G01 README](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/experiments/g01-scaleset/README.md) (`GOTOOLCHAIN=go1.26.9` verify commands); [G02 README](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/experiments/g02-auth/README.md) (Go 1.26.9 pinned, no third-party deps); [R1 README active instructions](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/experiments/r1-credentials/README.md) (lines 27-29); [live-canary checklist](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/docs/evidence/g01-live-canary.md#required-environment-and-authorization-record) (SDK `v0.4.0`, Go `1.26.9`, runner `2.337.0`) |

SDK `v0.4.0` and runner `2.337.0` are unchanged by the toolchain
remediation; no Go dependency or checksum pin changed.

## Stale active pin finding at the historical snapshot: none

- `rg -n "1\.26\.8"` over the twelve active pin files (four `go.mod`,
  `Makefile`, `check-toolchain.sh`, `check-offline-experiments.sh`,
  `fast-check.sh`, `compare-sdk.sh`, both workflows,
  `DEPENDENCIES.md`) returned no matches (exit 1).
- `rg -n "^toolchain "` over the four `go.mod` files returned no
  matches (exit 1): no redundant directive, as the checker requires.
- `go.sum` exists only at `experiments/g01-scaleset/go.sum`; root, G02,
  and R1 have none, consistent with the dependency-free runtime module
  record. No out-of-scope fix made; none required by this sweep.

## Negative-test 1.26.8 (ACTIVE source, intentional reject fixtures)

These tests accept only the exact `go1.26.9` build value and reject
`go1.26.8`, `go1.27.1`, and empty versions while retaining the existing
revision, SDK, tag, cgo, and platform checks. They are not stale pins:

- [tooling_test.go:151](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/scripts/tooling_test.go)
  `TestToolingCheckToolchainRequiresGo1269`, case `reject previous patch`.
- [g01-live main_test.go:36](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/experiments/g01-scaleset/cmd/g01-live/main_test.go)
  `TestBuildRevisionAdmissionRequiresGo1269`.
- [g01-worker main_test.go:27](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/experiments/g01-scaleset/cmd/g01-worker/main_test.go)
  `TestBuildRevisionAdmissionRequiresGo1269`.
- [broker_process_test.go:415](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/experiments/g02-auth/broker_process_test.go)
  `TestBrokerBuildMustMatchReviewedController` toolchain case.
- [broker_paired_prep_test.go:197](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/experiments/g02-auth/broker_paired_prep_test.go)
  `TestPairedFixtureBuildRequiresExactGo1269`.

Per the corrected [pin admission record](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/docs/evidence/g01-runner-release-checkpoint.md#pin-admission-and-focused-verification),
the RED showed the prior checker rejecting the Go 1.26.9 fixture on the
go-directive mismatch; it did not establish acceptance of a fake 1.26.8
report. No new RED/RUN interpretation is claimed here.

## Historical and provenance 1.26.8 (unaltered, truthful)

- G03 baseline and audit: the [G03 evidence record](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/docs/G03-EVIDENCE.md)
  and the [CI audit table](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/docs/CI.md#issue-73-audit-evidence) record Go
  1.26.8 as the 2026-09-07 snapshot and warmed baseline. Historical
  measurements, not current pins.
- Checkpoint: the [Go 1.26.8 source-mode findings](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/docs/evidence/g01-runner-release-checkpoint.md#local-source-mode-go-vulnerability-and-reachability-analysis)
  (exit 3, ten symbol-result IDs, fix Go 1.26.9) remain historical; the
  [Go 1.26.9 remediation and re-verification](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/docs/evidence/g01-runner-release-checkpoint.md#go-1269-remediation-and-source-re-verification)
  (seven profiles, exit 0, no finding IDs) is the recorded
  re-verification. The [independent review note](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/docs/evidence/g01-runner-release-checkpoint.md#independent-review-of-the-go-1269-pin-commit)
  covered the pin commit plus one Profile A verbose scan only; it did
  not repeat all seven source profiles or the full module/CI matrix.
  No independent rerun of any kind is claimed by this inventory.
- Dated evidence commands issued under 1.26.8 at record time: the
  [R1 2026-09-13 evidence section](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/experiments/r1-credentials/README.md#evidence-recorded-for-issue-67),
  [G01 contract](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/docs/evidence/g01-contract.md) (including historical
  `govulncheck@v1.1.4`), [strict inventory](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/docs/evidence/g01-strict-inventory.md),
  [G02 offline packet](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/docs/evidence/g02-offline-packet.md),
  [worker harness](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/docs/evidence/g01-worker-harness.md), handoffs, and paired and
  live-safety followups. Historical; not current instructions.
- Matcher provenance: the [adaptation notice](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/docs/third-party/go-testing-matcher.md)
  records `splitSelector`, `rewrite`, and `isSpace` as adapted from the
  Go 1.26.8 `src/testing/match.go` source, with the Go license text
  reproduced verbatim. Preserved as provenance truth, not a toolchain
  pin to bump.

## Packet execution recipes: inert, requalification remaining

- The [recovery packet](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/docs/evidence/g01-recovery-packet.md) embeds
  `reviewed_toolchain = "go1.26.8"` and 171 `go1.26.8` recipe lines
  (`rg -c`; lines counted, not expanded here). Inert text only: never
  executed, compiled, evaluated, or imported for this inventory.
- The [live-canary checklist](https://github.com/1XP-AI/gh-runnerd/blob/9776562fc792e0fb76684fb7be90a7ed118ed0ec/docs/evidence/g01-live-canary.md#required-environment-and-authorization-record)
  states those embedded 1.26.8 commands remain inert
  historical/specimen text rather than current execution instructions,
  superseded by the Go 1.26.9 checklist pin.
- Remaining: packet recipe requalification under separate authorized
  ownership. No packet fix in this inventory; historical packet text
  unaltered. No exhaustive packet AST was performed.

## Static evidence (actual commands, sanitized)

| Command | Exit | Result |
| --- | --- | --- |
| `git rev-parse HEAD` | 0 | `9776562fc792e0fb76684fb7be90a7ed118ed0ec` |
| `git status --short` | 0 | Empty at run start; only this new file added by this task (a co-worker's separate new file is preserved untouched) |
| `rg -n "^go 1\." go.mod experiments/*/go.mod` | 0 | Four `go 1.26.9` directives |
| Pin sweep over `Makefile` and `scripts/` plus `compare-sdk.sh` | 0 | Every exact/expected toolchain anchor is `go1.26.9` |
| Pin sweep over `.github/workflows/ci.yml` and `pr-fast.yml` | 0 | Five `GOTOOLCHAIN: go1.26.9` anchors, `go-version-file: go.mod`, immutable checkout/setup-go SHAs |
| `rg` over live/worker `main.go`, `broker_process.go`, `broker_paired_prep_test.go` | 0 | All BuildInfo/fixture gates require `go1.26.9` |
| `rg` for `GOVULNCHECK_VERSION` and `govulncheck@` | 0 | `v1.7.0` in `Makefile`, script default, `DEPENDENCIES.md`, `CI.md` |
| `rg -n "^toolchain "` over four `go.mod` files | 1 | No matches; no redundant directive |
| `rg -n "1\.26\.8"` over twelve active pin files | 1 | No matches; no stale ACTIVE pin |
| `rg -n "go1\.26\.8"` over five test files | 0 | Five intentional reject fixtures listed above |
| `rg -c "go1\.26\.8" docs/evidence/g01-recovery-packet.md` | 0 | `171` (inert count only; not executed) |
| `ls go.sum experiments/*/go.sum` | — | Only `experiments/g01-scaleset/go.sum` exists |
| `git show --oneline -s HEAD` | 0 | `9776562 Clarify Go 1.26.9 red evidence and review` |

No dynamic tests, scans, builds, packet operations, independent review,
or live operations were run for this inventory.

## Remaining gaps

Packet recipe requalification is outstanding. Full G01/G02,
live-authorization, safe-drain, and tenant/archive/image qualification
gates remain open. No G01 closure is claimed.
