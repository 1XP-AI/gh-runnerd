# G01 SDK v0.5.0 offline candidate

Historical qualification at PR #106/main `193c53d8e5b8a10951eabfa2ab1d3fb2184814ba`.
The [adoption ledger](g01-sdk-v05-adoption.md) tracks the later pin/adapter change
and promotion of these unchanged test files into the G01 module root. Statements
below about unchanged shared pins and manual-only coverage describe that earlier
qualification batch, not the later adoption candidate.

Issue: [G01 #1](https://github.com/1XP-AI/gh-runnerd/issues/1).
Writer: Codex, current maintainer-selected Sol medium route.
Measured 2026-10-25; **candidate evidence, not adoption or G01 closure**.

## Change-impact and review ledger

This is the successor candidate's single ledger. The preceding
[release checkpoint](g01-runner-release-checkpoint.md#2026-10-25-sdk-release-checkpoint-and-baseline-comparison)
records the unchanged Go 1.26.9/v0.4.0 comparison and discovery of v0.5.0.

| Surface | Evidence / disposition |
| --- | --- |
| New listener and HTTP/retry API | Actual SDK HTTP probes below; old callback results are not relabeled v0.5.0 evidence. |
| Existing recovery helper | Copied unchanged into a disposable candidate module; new causal/negative probes exercise supported GET calls. No production recovery change. |
| Existing pins, driver/broker and main matrix | Unchanged. Reuse immutable baseline and PR #103/main evidence; no duplicate matrix run. |
| Inert recovery packet | Unchanged; frozen static result retains its original scope. No recipe execution/requalification. |
| Independent protocol/security review | Pending for this candidate. The PR #103-only review waiver does not apply. |
| Exact-head GitHub Codex and PR quick | Required before merge. A request is not a completed review. |
| Adoption, live recovery/drain, G01 closure | Unresolved. No live experiment or dependent production work authorized. |

## Inputs and isolation

SDK `github.com/actions/scaleset v0.5.0` is the published source at
[`af394d86c0d5024225238b0209043c70193bcfe6`](https://github.com/actions/scaleset/tree/af394d86c0d5024225238b0209043c70193bcfe6).
Its minimum Go is 1.27.3. Official Go selector execution in this batch reported
`go version go1.27.3 darwin/arm64`; the prior checkpoint's non-execution statement
remains true for its earlier batch.

[`compare-v05.sh`](../../experiments/g01-scaleset/compare-v05.sh) creates one owned
temporary module from [`testdata/sdk-v05`](../../experiments/g01-scaleset/testdata/sdk-v05)
plus unchanged parent `recovery.go`, selects exact Go 1.27.3, disables workspace
mode, verifies tidy consistency, runs race/vet, and removes its own directory.
The nested `testdata` module is excluded from default `./...` discovery and the
three-module Public CI allowlist. Direct testing in that directory is unsupported
because it needs the copied recovery helper. No Docker example, packet text,
live driver, runner binary or production adapter is executed.

All SDK requests, including inherited session clients, use a cloned transport
without proxy and a dial guard accepting only that fixture server's exact
address. Tokens/PAT/unsigned JWTs are synthetic. A dedicated denial probe checks
rejection before any non-fixture dial. Public Go proxy, checksum/toolchain and
advisory downloads are separate network activity, not SDK traffic.

Request contexts are bounded to five seconds; client timeout is two seconds
except for the SDK's poll-client copy. The package limit is sixty seconds.
Channels control cancellation, drain and late responses; watchdog timers are
failure bounds, not race stimuli. Cancellation-independent ACK remains bounded
by the HTTP client timeout. No ambient credential/configuration was forwarded.

Git blob identities (SHA-1, not commit/review identities):

| Input | Blob |
| --- | --- |
| Upstream unmodified listener | `1ca83e264edfa170291925d9a3ac9ed8a97f6040` |
| Candidate `client_test.go` | `0b5d2d35191e1db183628b662eb0789cae68a989` |
| Candidate `listener_test.go` | `946dfb3e37d431e9bcd195f5e21c0fbe851e9cff` |
| Candidate `server_test.go` | `00a00ac32836737e4092f307105d064930b5ba16` |
| Candidate `go.mod` | `49012b7dd00193f7714ad8b87d59e769ac3db73d` |
| Candidate `go.sum` | `84d4aae93e0ee9744961d7cfae86094860484d5a` |
| Profile runner | `030513fff4ab41827d083dc830fbc901929f5bb3` |
| Unchanged `recovery.go` | `902355de2b52be46c2694ba0884a103a30aa6734` |

## Actual negative control and canonical green

In an owned temporary copy of pinned upstream non-test root Go files, module
manifests, license and listener, verify the original listener blob above. Move
the `DeleteMessage(context.WithoutCancel(ctx), msg.MessageID)` block to immediately
before `scaler.Scale(ctx, msg)` and remove the later duplicate ACK. Point a
disposable probe module's local replace at this copy. **Do not modify the shared
module cache or commit a fork.** Actual negative-control command:

```sh
go test -mod=readonly -count=1 -timeout=30s \
  -run '^TestV05HandlerFailureLeavesMessageUnacked$' .
```

Exit **1**, package 0.322s: `before-effects` and `partial-effects` both failed
`handler failure must precede ACK; message must remain unacked`. The mutant
compiled and entered the real HTTP probe. This deliberately unsafe mutation is
**not an upstream v0.5.0 defect**, compiler/setup RED, or production fix claim.

The canonical profile had no replace and selected the published checksummed SDK.
From the repository root:

```sh
sh experiments/g01-scaleset/compare-v05.sh
```

Exit **0**: exact Go 1.27.3 / SDK v0.5.0 reported, tidy consistency passed,
all **17 top-level tests** passed under `-race -count=1 -timeout=60s` (package
1.432s), then `go vet -mod=readonly ./...` passed without diagnostics. Package
duration excludes downloads/compilation/vet. Execution used scrubbed exact
cached Go/compiler paths, public proxy without direct fallback and checksum
database, `GOENV=off`, `GOWORK=off`, `GOAUTH=off`, telemetry off, empty private
module selectors, `GOEXPERIMENT=none`, `GOMAXPROCS=2`, `GOFLAGS=-p=2`, cgo enabled,
`GOOS=darwin`, `GOARCH=arm64`, `GOARM64=v8.0`.

Additional actual checks in a canonical disposable copy:

| Check | Result |
| --- | --- |
| `go mod verify` | Exit 0; all modules verified. |
| `go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 -test ./...` | Exit 0; no vulnerabilities found in this test-inclusive source graph/platform at scan time. Not release/platform-wide assurance. |
| `go list -mod=readonly -deps -test` | Imported external modules: SDK v0.5.0, JWT/v5 v5.3.1, uuid v1.6.0; not the upstream Docker/tool graph. |
| License inspection | SDK and JWT/v5 MIT; uuid BSD-3-Clause. Preserve notices on redistribution. No shipped runtime dependency added. |
| `sh -n experiments/g01-scaleset/compare-v05.sh` | Exit 0. |

## Measured contracts and limitations

| Probe | Observation with real SDK and controlled HTTP fixture |
| --- | --- |
| Handler failure before/after partial effect | No ACK; fresh Run sees retained fixture message. Partial effect repeats without caller idempotency. Fixture retention is not measured GitHub redelivery. |
| Successful canceled handler / canceled failing handler | Success still ACKs once after cancellation; canceled error does not ACK. Handler uses original context. |
| ACK denial / response loss before or after fixture commit | One completed handler and one ACK attempt with retries off. Controlled commit changes retention; error alone cannot identify commit state. |
| Acquisition not attempted / response loss before or after commit | No ACK; message retained. Independent reads preserve quarantine/admission pause; no blind retry. |
| Caller acquisition, 0/50/51 IDs | No automatic acquisition. Caller can submit 51; controlled partial result returns one. Desired total 125 is not event/acquired count. No real server batch limit proved. |
| Initial/repeated/stale statistics and 202 | Initial synthetic demand 0; sequence 3,3,1 is not monotonic. 202 yields nil; unknown/cache policy belongs to caller. |
| Capacity withdrawn during held poll | Next poll advertises zero, but old in-flight response can still acquire/ACK. Local test fence prevents these actions; no production/server drain barrier proved. |
| Poll/ACK/acquisition 401/403/429 | 401 refreshes once and retries same operation; cursor/ACK ID retained. 403/429 do not refresh with HTTP retries off; 429 is typed RateLimitError. |
| Held old poll plus concurrent 401 refresh | Current session demand 8 coexists with returned old-message demand 1. Message carries no generation; caller fencing remains necessary. |
| Poll cancellation/client ownership | Cancellation terminates poll; supplied client timeout unchanged. Not a measured two-minute timeout test. |
| JIT transport loss/request retry override | Parent Max=2 issues three fixture POST attempts, per-request Max=0 one. Next normal request retains parent policy. Not method-idempotency proof. |
| Retry-After 3600 versus five-second context | One typed 429 returns before context expiry; no hour-long wait/retry. |
| Busy removal/raw errors | Sentinel retained; raw error includes synthetic private response marker. Probe never prints it; adapter redaction still required. |
| JIT not attempted/lost response before or after commit | Independent statistics/name GETs restore reference only after controlled commit. Every result stays quarantined/paused, without second JIT POST. |
| Recovery invalid statistics, GET failures, foreign owner, absence | Fail closed, preserve input/reservations, sanitize errors. Absence does not prove process exit. |

## Decision and rollback

v0.5.0 fixes ACK-before-handler ordering at SDK level, not durable processing,
exactly-once execution, acquisition/JIT idempotency, freshness or safe deletion.
Keep the provisional high-level-listener plus independent-reconciliation design.
Keep active v0.4.0/Go 1.26.9 until adoption receives its own reviewed shared-pin,
driver/broker-admission and compatibility change. No old binary becomes v0.5.0,
and no admission check is weakened by this profile.

Independent protocol/security and exact-head GitHub reviews remain merge gates.
Hosted CI does not run this opt-in profile; manual results are separate evidence.
A future adoption must explicitly decide ongoing CI coverage. Existing main
matrix remains Go 1.26.9. G01/Project stay open/In progress until all remaining
criteria, including separately authorized private live recovery/drain evidence,
are satisfied. Rollback is reverting this profile/evidence addition; there is no
production state or live resource to clean up.
