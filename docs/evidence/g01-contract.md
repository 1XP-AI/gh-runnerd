# G01 Scale Set contract evidence

Date: 2026-09-07. **Offline evidence only; live gate unresolved.**
Implementation/model: Astra (`gpt-6-astra`, `xhigh`). Independent Astra `xhigh`
review approved the offline artifact at `4e58c55`; the reviewer reproduced race
tests, vet and both pinned SDK comparisons. This is not approval of a live
canary or the unresolved production integration gate.

## Exact inputs and primary sources

| Input | Exact selection / finding |
|---|---|
| SDK pin | `github.com/actions/scaleset v0.4.0`, tag commit `6ce025902cd964747a078c2aabe7340ebc667eca` |
| Audited comparison | `cb0405b2d874500e75ae34eff8d582ab75956b45`; Go pseudo-version `v0.4.1-0.20260721134647-cb0405b2d874` |
| Upstream state checked | Official `main` resolved to the audited commit; latest SDK release was `v0.4.0` |
| SDK Go requirements | Release `go 1.25.3`; audited commit `go 1.26.3` |
| Actual red runtime | `go1.25.8 darwin/arm64` |
| Green/comparison runtime | `GOTOOLCHAIN=go1.26.8`, macOS ARM64; aligned with G03 |
| Compiled external packages | Scale Set/client/listener (MIT), `golang-jwt/jwt/v4 v4.5.2` (MIT), `google/uuid v1.6.0` (BSD-3-Clause), `hashicorp/go-retryablehttp v0.7.8` and `go-cleanhttp v0.5.2` (MPL-2.0) |

The SDK's module graph also names example/dev tooling dependencies. This isolated
test binary does not import Docker, the example provider or those tools. This
table is the imported package inventory, not a product distribution SBOM.

Primary references: [release](https://github.com/actions/scaleset/releases/tag/v0.4.0),
[source comparison](https://github.com/actions/scaleset/compare/6ce025902cd964747a078c2aabe7340ebc667eca...cb0405b2d874500e75ae34eff8d582ab75956b45),
[release module](https://github.com/actions/scaleset/blob/6ce025902cd964747a078c2aabe7340ebc667eca/go.mod),
[audited module](https://github.com/actions/scaleset/blob/cb0405b2d874500e75ae34eff8d582ab75956b45/go.mod).

The diff has 21 changed files and no listener/README changes. Beyond dependencies
and tests, it adds list-scale-sets, mTLS options, credential validation and selected
HTTP error sentinels, and changes client/session locking and request construction.
Release session methods serialize long polls under a session mutex. The audited
version uses atomic session snapshots and refresh serialization. No performance
or live authentication comparison was performed. Source:
[session diff](https://github.com/actions/scaleset/compare/6ce025902cd964747a078c2aabe7340ebc667eca...cb0405b2d874500e75ae34eff8d582ab75956b45).

## Observed contract and limits

| Test / source | Actual observation | What this cannot establish |
|---|---|---|
| `TestSDKACKBoundaries` | Failure before successful DELETE leaves fixture delivery available; committed DELETE with response loss leaves no callback and no fixture replay; successful ACK precedes callback failure | GitHub retention/expiry/redelivery timing |
| `TestRecoveryAfterACKCallbackCrash` | Real listener ACK happens before crash barrier; independent scale-set GET restores desired 4 | Durable journal, actual process kill/restart, lost request identity |
| `TestRecoveryMissingLifecycleCallback` | Previous ready record becomes quarantined; runner reference cannot prove idle | Safe runner/process deletion |
| `TestSDKAcquisitionResponseLossAfterACK` | ACK precedes acquisition; fake service commits acquisition but drops response; listener exits before desired callback | Whether GitHub reacquires/requeues/rejects a repeated request ID |
| `TestSDKDemandAboveFiftyAndPartialAcquisition` | 50 available events with desired 125 produce desired 125; one returned acquisition ID does not reduce it. Artificial 51-event response also parses and is acquired in one request | GitHub accepting a >50-event response/request; 51 is an adversarial client test, not a server claim |
| `TestSDKRepeatedStatisticsAnd202ReuseLastObservation` | Desired sequence `0,3,3,1,1`; 202 repeats cached 1; `JobAssigned` produces no lifecycle callback | A version/freshness guarantee; a lower total may be valid or stale |
| `TestSDKHTTPFailuresAndSessionRefresh` | Poll/ACK/acquire 401 invokes one PATCH and retries once; refreshed session fields replace previous fields while poll cursor 91 is preserved. 403/429 return errors with configured zero HTTP retries | Real server session replacement/replay semantics, default retry timing, App credential refresh, installation revocation |
| `TestSDKCapacityWithdrawalDoesNotFenceInFlightAcquisition` | Channel barrier holds poll with capacity 1; `SetMaxRunners(0)` occurs; reply still triggers acquisition; next poll advertises 0 | Server-side withdrawal/deregistration atomicity |
| `TestSDKBusyRemovalSentinelAndRawErrorExposure` | Fake `JobStillRunningException` maps to `JobStillRunningError`; raw SDK error contains synthetic response-body marker | Server refusing every real busy/assignment race; production-wide log redaction |
| `TestSDKJITResponseLossDiscoversIdentityWithoutReissuing` | Lost fake JIT response, one create attempt, stable-name lookup recovers runner ID, reservation stays quarantined | Recovering JIT secret, duplicate-name/idempotency guarantees, real runner launch |
| `TestRecoveryErrorsHoldReservationsAndRedact` | Missing/invalid stats, denied inventory, foreign scale set and absent reference retain reservations and pause admission; error marker does not escape recovery errors | Transactional persistence, OS isolation or all application sinks |

Tests exercise the imported SDK rather than a reimplementation of its loop. The
server intentionally supplies scripted statistics, queue retention, side effects
and busy refusal; those are **fixture assumptions**. Channel barriers are causal
ordering controls; timeout timers are watchdogs only. No sleep-based race stimulus,
live credentials, production endpoint or worker provider is used.

Source details: [listener](https://github.com/actions/scaleset/blob/6ce025902cd964747a078c2aabe7340ebc667eca/listener/listener.go),
[documented message semantics](https://github.com/actions/scaleset/blob/6ce025902cd964747a078c2aabe7340ebc667eca/README.md#autoscaling),
[session implementation](https://github.com/actions/scaleset/blob/6ce025902cd964747a078c2aabe7340ebc667eca/session_client.go),
[reference/statistic types](https://github.com/actions/scaleset/blob/6ce025902cd964747a078c2aabe7340ebc667eca/types.go),
[SDK error formatting](https://github.com/actions/scaleset/blob/6ce025902cd964747a078c2aabe7340ebc667eca/errors.go).

## Runner and JIT transport

Official runner source checked: `v2.337.0`, commit
`397b032cbf865e9c3ddfab89d533ec19325e1273`, released 2026-08-26. No runner binary
was downloaded or executed. Candidate canary assets from the official release
API (must verify bytes before execution):

| Asset | Published SHA-256 digest |
|---|---|
| `actions-runner-linux-arm64-2.337.0.tar.gz` | `9b1dc70626422526e3c94767cf024896beb15da5342a3f4819bf2feac13e0393` |
| `actions-runner-osx-arm64-2.337.0.tar.gz` | `5a2cd92908a93d7276a194e1de6008099f3e7946f3f8e14aa7a1a7b4a31fdec2` |

Source: [official runner release](https://github.com/actions/runner/releases/tag/v2.337.0).

| Transport | Evidence and exposure |
|---|---|
| `Runner.Listener run --jitconfig <encoded>` / `run.sh --jitconfig <encoded>` | Command parser recognizes `jitconfig`; argv exposes the encoded credential to process inspection and command capture. Avoid for the canary. |
| `ACTIONS_RUNNER_INPUT_JITCONFIG` | Supported input fallback; parser masks the value and removes the variable from its environment after reading. Choose for canary worker-only injection. Initial environment, process memory, parent launch configuration and Docker container metadata if passed as `Env` can still expose it. |
| stdin / inherited FD / `--jitconfig-file` | No such bootstrap input is established in the inspected command parser. Do not implement or claim one based on an invented option. A future wrapper would need separate review. |

The runner decodes JIT into configuration files under its root; sensitive files
therefore exist on disk even with an environment transport. Base64 is encoding,
not protection. Keep the runner root private and disposable, never publish its
contents, and never place App private keys or installation/admin tokens in it.
The parser's masking/removal is a mitigation, not hostile same-user isolation.
Real environment inheritance and filesystem residue remain canary evidence gaps.

Source: [command/environment parsing](https://github.com/actions/runner/blob/397b032cbf865e9c3ddfab89d533ec19325e1273/src/Runner.Listener/CommandSettings.cs),
[secret argument list](https://github.com/actions/runner/blob/397b032cbf865e9c3ddfab89d533ec19325e1273/src/Runner.Common/Constants.cs),
[JIT file materialization](https://github.com/actions/runner/blob/397b032cbf865e9c3ddfab89d533ec19325e1273/src/Runner.Listener/Runner.cs).

## Validation record

[Compiled red evidence](g01-red.md) was committed as `ce9e4ca` before the recovery
implementation. It contains two assertion failures, not fabricated build errors.
During green development, inventory fixture routing was corrected from `/runners`
to the SDK's `/agents`; that setup correction is separate from the red behavior.

Executed from `experiments/g01-scaleset` on 2026-09-07:

```text
$ GOTOOLCHAIN=go1.26.8 go test -count=1 -v -timeout=30s ./...
11 top-level tests and 20 subtests PASS; package PASS (0.393s)
$ GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=45s ./...
PASS
$ GOTOOLCHAIN=go1.26.8 go vet ./...
exit 0, no diagnostics
$ ./compare-sdk.sh
github.com/actions/scaleset v0.4.0
PASS (race enabled)
github.com/actions/scaleset v0.4.1-0.20260721134647-cb0405b2d874
PASS (race enabled)
$ GOTOOLCHAIN=go1.26.8 go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 -test ./...
No vulnerabilities found.
```

Vulnerability checking includes tests and covers this isolated module at
the time of the scan. It is not a finding about every SDK example/dependency or
the runner release. No fuzz, live, hardware, persistence or soak tests are counted
as passed. Exact final repeat/comparison results must remain green after review.

## Gate status and rollback

The [provisional decision](../decisions/0002-scaleset-integration.md) selects the
released listener with independent reconciliation. Aggregate demand and stable
reference IDs are recoverable candidates. Lost job identity/JIT contents and
ambiguous acquisition are quarantined, not automatically replayed. Unknown or
busy resources keep reservations; no exactly-once claim is made.

Outstanding: reviewed private canary harness;
explicit authorization for the [concrete live plan](g01-live-canary.md); sanitized
live evidence; safe drain contract decision. G01 and dependent production work
remain gated. The spike has no live effects; reverting its files is sufficient
rollback. An evidence-only PR must not automatically close issue #1.
