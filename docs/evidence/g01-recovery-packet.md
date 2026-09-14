# G01 reusable recovery evidence packet

Date: 2026-09-14. Issue: [G01 / #1](https://github.com/1XP-AI/gh-runnerd/issues/1).
Status: **selected path recorded; offline/source evidence only; live gate
unresolved**. This packet is a sanitized index and changed-boundary ledger. It
does not replace the linked records, authorize a canary, or claim production
completion.

The active G01 Goal is:

> Select and pin one supported Scale Set integration path and produce a reusable
> evidence packet for recovery at message acknowledgement, acquisition and JIT
> boundaries, rerunning only changed-boundary checks while keeping unchanged
> evidence and live gaps explicit.

## Selected path

Pin `github.com/actions/scaleset v0.4.0` at source commit
`6ce025902cd964747a078c2aabe7340ebc667eca` behind an adapter. The selected path
is a future production contract: use the released high-level listener with
independent reconciliation, keep one serialized session owner per pool, and
preserve the upstream message order:

```text
statistics observation -> message ACK (DeleteMessage) -> available-job acquisition -> lifecycle callbacks
```

The packet records the SDK's order; it does **not** reorder ACK after callbacks
or acquisition, and it does not claim exactly-once delivery, idempotent
acquisition/JIT, durable execution, linearizability or server-side receipt.
Callbacks are observations. The future production recovery contract is to read
`TotalAssignedJobs` at startup, after listener failure/session replacement and
on a bounded schedule, with session-generation fencing and conservative
ownership/quarantine rules. The current offline experiment does not implement
that full path: `recovery.go` performs one synthetic read and explicitly defers
persistence, freshness fencing, caps and provider ownership validation to later
gates, while rehydration and general session replacement remain unimplemented
in the driver. Bounded fixture rules preserve a later zero or absent
observation from erasing an earlier work-bearing or uncertain observation; that
is not a live or production reconciliation result.

## Classification and evidence index

Use these classifications throughout the packet:

- **Source** — upstream code or documentation at a pinned URL. It establishes
  what the inspected implementation or documented API says, not live service
  behavior.
- **Fixture** — an offline synthetic HTTP, TLS, Unix-socket, subprocess or file
  test. It can exercise the pinned client and local recovery code, but fixture
  queue/response/commit behavior is not a GitHub service result.
- **Live** — an authorized private GitHub/runner/Docker observation. No live
  result is recorded in this packet.

| Existing record | Classification and reusable scope | Immutable evidence reference |
|---|---|---|
| [ADR 0002](../decisions/0002-scaleset-integration.md) | Source-backed decision and future recovery contract; G01 remains provisional. | [d0afc56d471e8d63d153f6d26aeb95915d141d87](https://github.com/1XP-AI/gh-runnerd/blob/d0afc56d471e8d63d153f6d26aeb95915d141d87/docs/decisions/0002-scaleset-integration.md) |
| [G01 contract](g01-contract.md) | Source + fixture: ACK/acquisition/statistics/error/JIT boundaries, SDK and runner pins. | [ba113977d03fc209ca147edbe10b766eed2c0fe3](https://github.com/1XP-AI/gh-runnerd/blob/ba113977d03fc209ca147edbe10b766eed2c0fe3/docs/evidence/g01-contract.md) |
| [G01 red evidence at historical commit `1396e201d905be204c3ac697be43723820581314`](https://github.com/1XP-AI/gh-runnerd/blob/1396e201d905be204c3ac697be43723820581314/docs/evidence/g01-red.md) | Fixture: pre-fix ACK/callback-loss failures; no live result. | Immutable file blob `c36e0af0c8e9b301f4889a02454c83ece8d5942f`. |
| [JIT causality follow-up](g01-jit-causality.md) | Fixture: committed-vs-requested synthetic JIT creation and response-loss controls; no live bug claim. | [ba113977d03fc209ca147edbe10b766eed2c0fe3](https://github.com/1XP-AI/gh-runnerd/blob/ba113977d03fc209ca147edbe10b766eed2c0fe3/docs/evidence/g01-jit-causality.md); causal fix [7079b255d09ce0456b5843a8695277689d52e54e](https://github.com/1XP-AI/gh-runnerd/commit/7079b255d09ce0456b5843a8695277689d52e54e) |
| [Live-canary plan](g01-live-canary.md) | Live plan and authorization checklist; every phase remains unchecked/not run. | [d0afc56d471e8d63d153f6d26aeb95915d141d87](https://github.com/1XP-AI/gh-runnerd/blob/d0afc56d471e8d63d153f6d26aeb95915d141d87/docs/evidence/g01-live-canary.md) |
| [Controller phase driver](g01-live-driver.md) | Fixture/source: bounded tagged controller operations; no worker launch or live result. | [controller driver evidence revision `0e08ce8285846e26a94fb6fcb33bbeede662bf14`](https://github.com/1XP-AI/gh-runnerd/blob/0e08ce8285846e26a94fb6fcb33bbeede662bf14/docs/evidence/g01-live-driver.md) |
| [Idle-drain observation](https://github.com/1XP-AI/gh-runnerd/blob/f5560ba950f77343e57034cc1cf85dc67f5ac922/docs/evidence/g01-idle-drain.md) | Fixture + source at the authoritative current PR #72 head; PR #72 is open with no checks reported, and this is offline evidence only. | [PR #72 head `f5560ba950f77343e57034cc1cf85dc67f5ac922`](https://github.com/1XP-AI/gh-runnerd/commit/f5560ba950f77343e57034cc1cf85dc67f5ac922) |
| [Identity/reconciliation](g01-identity-reconciliation.md) | Source + fixture limits for runner, request, job, REST and terminal identity; no live equality claim. | [9fe1b43cceb10c3ce92b26f8262c72148ded6c60](https://github.com/1XP-AI/gh-runnerd/blob/9fe1b43cceb10c3ce92b26f8262c72148ded6c60/docs/evidence/g01-identity-reconciliation.md) |
| [Exact observation adapter](g01-exact-observations.md) | Fixture: bounded exact-ID/status/provenance readers; no affirmative live recovery or cleanup. | [d62242b07b779ff6f7414e97096d2c8828634a5d](https://github.com/1XP-AI/gh-runnerd/blob/d62242b07b779ff6f7414e97096d2c8828634a5d/docs/evidence/g01-exact-observations.md) |
| [Worker/JIT harness](g01-worker-harness.md) | Source + fixture: worker profile and secret boundary; no image startup, JIT mint, workflow or Docker result. | [c1c0b6a6f2f94651546f533576eb3bbe8a4089b4](https://github.com/1XP-AI/gh-runnerd/blob/c1c0b6a6f2f94651546f533576eb3bbe8a4089b4/docs/evidence/g01-worker-harness.md) |
| [Paired collection](g01-paired-baseline.md) and [terminal path](g01-paired-terminal.md) | Fixture: serialized controller/worker receipts and terminal correlation; no live cleanup or success claim. | [8dd64adc551ba5174892807a678e8bc614d0a474](https://github.com/1XP-AI/gh-runnerd/blob/8dd64adc551ba5174892807a678e8bc614d0a474/docs/evidence/g01-paired-baseline.md), [cf67d4aeb511116fee0de31a4ac38409f28fa29f](https://github.com/1XP-AI/gh-runnerd/blob/cf67d4aeb511116fee0de31a4ac38409f28fa29f/docs/evidence/g01-paired-terminal.md) |
| Current source anchor | This packet is based on clean `origin/main` at the immutable source tree below. | [dce795a871865a8a2ee728151cb161e55081c8b7](https://github.com/1XP-AI/gh-runnerd/commit/dce795a871865a8a2ee728151cb161e55081c8b7) |

The idle-drain link intentionally points at the authoritative current PR #72
head and is not copied into this base worktree. PR #72 is currently open with no
checks reported (GitHub reports merge state `CLEAN`); do not turn its offline
result into a live result or claim that PR #72 is mergeable from this packet.

## Exact pins and transport choices

| Boundary | Exact selection | Classification / exposure limit |
|---|---|---|
| Scale Set SDK | `github.com/actions/scaleset v0.4.0`; tag/source `6ce025902cd964747a078c2aabe7340ebc667eca`; [release](https://github.com/actions/scaleset/releases/tag/v0.4.0), [listener source](https://github.com/actions/scaleset/blob/6ce025902cd964747a078c2aabe7340ebc667eca/listener/listener.go), [module](https://github.com/actions/scaleset/blob/6ce025902cd964747a078c2aabe7340ebc667eca/go.mod) | Source + fixture pin. The module declares Go `1.25.3`; the audited comparison is not the selected release. |
| Go toolchain | Experiment module directive `go 1.26.3`; verification toolchain `GOTOOLCHAIN=go1.26.8` / `go1.26.8`; reviewed `GOEXPERIMENT=none` | Fixture/toolchain evidence only; every rerun records the exact toolchain and experiment setting. |
| Go root/FIPS mode | Reviewed `GOROOT` mode is the toolchain default (`GOROOT=""`, identity `goroot-default`); reviewed `GOFIPS140=off` (identity `gofips140-off`) | Fixture/toolchain evidence only; non-empty inherited or command-prefix `GOROOT` and any `GOFIPS140` other than `off` are refused before a guarded Go child. The effective modes are bound in every guarded build identity; raw root paths are never recorded. |
| Runner | `v2.337.0`, source `397b032cbf865e9c3ddfab89d533ec19325e1273`; [release](https://github.com/actions/runner/releases/tag/v2.337.0), [command parser](https://github.com/actions/runner/blob/397b032cbf865e9c3ddfab89d533ec19325e1273/src/Runner.Listener/CommandSettings.cs), [JIT materialization](https://github.com/actions/runner/blob/397b032cbf865e9c3ddfab89d533ec19325e1273/src/Runner.Listener/Runner.cs) | Source pin; no runner binary was downloaded or executed by the contract evidence. |
| JIT bootstrap | Select worker-only `ACTIONS_RUNNER_INPUT_JITCONFIG` as parsed by runner `v2.337.0`; [environment parsing](https://github.com/actions/runner/blob/397b032cbf865e9c3ddfab89d533ec19325e1273/src/Runner.Listener/CommandSettings.cs) | Source-supported fallback, with no separate transport version. It avoids argv but remains sensitive in initial environment, process memory, container metadata and runner-written files. `--jitconfig` exposes argv; stdin/FD/file transport is not established and is not claimed. |
| REST observation | GitHub REST API header `2022-11-28` for exact runner/job reads; Scale Set acquisition uses SDK's `api-version=6.0-preview` | Source/fixture request contract, not live acceptance or freshness evidence. |
| Worker runtime support | Docker Engine API `1.45`, Moby schema `v26.1.5`, and the worker-harness image reference `ghcr.io/actions/actions-runner@sha256:f5a0d9a3d857315f2aed7075a02a29f46927ad198221c3b1c66585ae9fe36c0d` | Supporting fixture profile only; actual daemon/image normalization and runner startup remain live gaps. |

### Runner pin/source-audit gate

The Runner parser and JIT materialization are part of the pinned source
contract, not evidence supplied by the local Go fixtures. Any proposed Runner
version or source-commit change opens a mandatory immutable source-audit gate:
inspect `CommandSettings.cs` and `Runner.cs` at the candidate commit, record
that commit and the two immutable upstream URLs, and compare the accepted
parser/input modes and JIT materialization behavior with this packet's
transport boundary. Local Go fixture results may supplement that audit but
cannot be reused as its substitute; a changed Runner pin remains an open
source gate until the audit (and any required Runner-level fixture) is recorded.

The current correction preserves Runner `v2.337.0` at source commit
`397b032cbf865e9c3ddfab89d533ec19325e1273`; no Runner source audit, binary
execution or Runner-level fixture result is claimed or required for this
documentation-only selector correction.

Published runner archive digests retained from the contract record are
`9b1dc70626422526e3c94767cf024896beb15da5342a3f4819bf2feac13e0393`
(Linux ARM64) and
`5a2cd92908a93d7276a194e1de6008099f3e7946f3f8e14aa7a1a7b4a31fdec2`
(macOS ARM64). Verify bytes again before any trusted execution; a digest or
version pin alone does not prove the running binary or disable runner updates.

## Requirement/evidence matrix

| Requirement / boundary | Reusable evidence | Classification | Current disposition and limit |
|---|---|---|---|
| Message acknowledgement order | `g01-contract.md`: `TestSDKACKBoundaries`, `TestRecoveryAfterACKCallbackCrash`; pinned listener source above. | Source + fixture | Offline evidence observes ACK before callback and before acquisition. Preserve this order; do not infer GitHub redelivery/retention, server receipt or durable exactly-once behavior. Live pre/post-ACK and session-restart behavior require authorization. |
| ACK/callback loss recovery | `g01-contract.md` and [G01 red evidence at historical commit `1396e201d905be204c3ac697be43723820581314`](https://github.com/1XP-AI/gh-runnerd/blob/1396e201d905be204c3ac697be43723820581314/docs/evidence/g01-red.md) (immutable blob `c36e0af0c8e9b301f4889a02454c83ece8d5942f`); ADR 0002 recovery contract. | Fixture + source | Independent statistics reads can restore aggregate demand; missing callback/request identity remains quarantined. No process-kill/restart or durable production journal result is claimed. |
| Acquisition intent and response loss | `TestSDKAcquisitionResponseLossAfterACK`; driver `before-acquire`/`acquire-loss`; [live-canary acquisition phase](g01-live-canary.md#minimal-execution-phases). | Source + fixture + live plan | ACK precedes exactly one `AcquireJobs` request; a suppressed response retains reservation and quarantines. No retry, server reoffer, accepted-request read-back or idempotency claim. A live disposable acquisition observation is still gated. |
| JIT intent and response loss | [JIT causality follow-up](g01-jit-causality.md), `TestSDKJITLookupBeforeCreationDoesNotDiscoverIdentity`, `TestSDKJITResponseLossWithoutCommitDoesNotDiscoverIdentity` and committed response-loss control. | Fixture + source | Synthetic lookup is causally tied to fixture-side commit; JIT value/result identity is discarded and reservation quarantined. No JIT-secret recovery, duplicate-name guarantee, idempotency or real runner launch is claimed. |
| Reconciliation | ADR 0002; [identity evidence](g01-identity-reconciliation.md); exact observation and baseline listener records. | Source + fixture | Read `TotalAssignedJobs` at bounded recovery points, fence session generations, and keep counts as snapshots. A matching ID is only a candidate; no revision, atomic cross-service snapshot or linearizability is established. |
| Quarantine and ownership | [controller driver](g01-live-driver.md), [audit corrections](g01-audit-corrections.md), [worker admission](g01-worker-admission.md). | Fixture + source | Unknown create/session/request/JIT/acquisition/callback outcomes, foreign identities, missing/invalid stats and stale work retain reservations and stop new effects. Name, label, zero count or elapsed time never authorizes adoption/deletion. |
| Idle assignment and drain | [idle-drain evidence at PR #72 head](https://github.com/1XP-AI/gh-runnerd/blob/f5560ba950f77343e57034cc1cf85dc67f5ac922/docs/evidence/g01-idle-drain.md); [live-canary drain phases](g01-live-canary.md#minimal-execution-phases). | Source + fixture + live gap | Offline hook uses the released listener's `SetMaxRunners(0)` and retains ACK-before-acquisition; client-side request markers do not prove server receipt or an atomic drain. Real old-poll acquisition/idle assignment and busy-safe removal remain unverified. |
| Terminal identity and completion | [identity reconciliation](g01-identity-reconciliation.md), [exact observations](g01-exact-observations.md), [paired baseline](g01-paired-baseline.md) and [terminal path](g01-paired-terminal.md). | Source + fixture | Keep SDK request/job IDs, REST job/runner IDs, run attempt, runner identity and local exit distinct; bind an exact tuple before classifying. No live job eligibility, successful execution, per-request release or cleanup proof is claimed. |
| Secret and error boundary | Contract runner/JIT section, [worker harness](g01-worker-harness.md), and driver redaction rules. | Source + fixture | No secrets, JIT values, raw bodies or raw SDK errors belong in this packet. Environment transport remains exposed to trusted same-user/process/container surfaces; modes and same-user ownership are not hostile-code isolation. |
| Rollback and preservation | ADR 0002 and [live-canary exit/cleanup](g01-live-canary.md#exit-evidence-and-cleanup). | Source + plan | Base `dce795a871865a8a2ee728151cb161e55081c8b7` lacks this added packet path, so rollback is to close/delete the unmerged correction or run `git rm -- docs/evidence/g01-recovery-packet.md` in a review branch. Reserve `git restore --source=<reviewed-parent> -- docs/evidence/g01-recovery-packet.md` for restoring an existing parent file; preserve independent corrections in `g01-live-driver.md` and `docs/reviews/team-review.md`. For any future live run: stop new admission, let owned busy work finish, quarantine uncertainty, and remove only individually verified disposable resources; preserve manual runners and never force-kill, prune or replay. |

No row above is a live result. In particular, this packet makes no exactly-once
or idempotency claim.

## Live gaps and authorization gates

The following are explicit open gates, not implied by the fixture records:

1. Obtain an independent `gpt-luna-max` protocol/recovery review of the exact
   candidate source and this packet. New work follows the current
   `gpt-luna-max` route; historical Astra authorship is not current routing.
2. Name one disposable private repository, selected-only runner group,
   reviewed workflow/run attempt/source/head/path, owner nonce and immutable
   harness/workflow commits. Record an unchanged manual-runner inventory.
3. Approve distinct controller and Actions-read authorities, exact App/org
   installation permissions, broker issuance metadata and expiry. Keep
   credentials outside commits, logs, worker input and public evidence.
4. Approve one scale set, one serialized session owner, one active worker,
   bounded jobs/CPU/memory/deadlines, pinned SDK/Go/runner/JIT transport and
   the worker runtime/image. A native macOS run additionally needs trusted
   execution approval; trusted same-user execution is not hostile-code
   isolation.
5. Explicitly authorize create, ACK/callback fault barriers, acquisition/JIT
   response suppression, workflow execution, manager interruption, idle-drain
   assignment/removal probes and owned cleanup. The offline fixture must never
   be pointed at GitHub by changing its endpoint.
6. Publish sanitized live results for ACK/redelivery, acquisition acceptance or
   reoffer, JIT discoverability, session replacement, runner/job identity,
   idle assignment, busy-safe deregistration and cleanup. Until then, G01 and
   dependent production integration remain unresolved.

## Changed-boundary ledger

This ledger is the reuse rule for future evidence. **Reuse** means cite the
immutable record as historical evidence and label it reused; it never means
that a historical fixture pass became a current or live pass. **Rerun** when
the selected SDK/source, Go toolchain/module/experiment setting, listener/adapter, journal or
authority code, fixture endpoint/decoder, test tags/selectors, runner/parser,
JIT transport/image, or live target/approval changes. A documentation-only
wording/link correction does not invalidate an unchanged offline boundary.
The current table contains **10 data rows** (header and separator excluded);
the count includes the actual live-canary gate row.

| Boundary | Reuse unchanged evidence when | Exact focused rerun | Class/result to record |
|---|---|---|---|
| ACK, callback loss and acquisition order | SDK commit, listener source, adapter call order and root protocol fixtures are unchanged. | See the exact ACK/callback selector in the command block below, including `TestNoMessageDoesNotCountAsCompletedBarrier`. | Fixture: record ACK-before-acquisition and reservation/quarantine behavior; do not add server or exactly-once claims. |
| Baseline listener/admission | listener, strict source reader, journal/lease, caps and selectors are unchanged. | See the exact baseline and admission selectors in the command block below, including the worker admission integrity suite and the tagged `TestUnsupportedAccountLookupRefusesBeforeJournal` selector. | Fixture: record actual test result and pinned SDK; no live result or production worker claim. |
| Runner parser/JIT causality and transport | Runner version/source commit, parser/materialization behavior, JIT transport, causal fixture and response-loss controls are unchanged. | A Runner pin or source-commit change first requires the immutable source-audit gate above; then rerun the root-package and livecanary selectors in the command blocks below, with tagged worker input/refusal and worker transport selectors as applicable. Local Go fixture results cannot substitute for the source audit. | Source + fixture: record parser/materialization comparison, one-request controls and quarantine; never print or retain JIT. |
| Controller update policy | pinned SDK update-setting request, confirmation and drift guards are unchanged. | See the exact `./livecanary` update-policy selectors in the worker/runtime command block below: `TestCreateRequestsDisabledRunnerUpdate`, `TestUnconfirmedUpdateSettingQuarantinesCreate`, `TestUpdateSettingDriftStopsBeforeSessionOrJIT` and `TestUpdateSettingDriftDoesNotBlockSafeEmptyCleanup`. | Source + fixture: record request/confirmation and pre-session/JIT refusal; no live service-setting result. |
| Reconciliation, inventory/identity/quarantine | statistics readers, paged inventory normalization, generation fences, ownership checks and journal/authority schema are unchanged. | Retain the prior reconciliation selector and add the exact focused `TestObservation*` and `TestRoster*` families, inventory/quarantine and journal/authority selectors in the command block below, including controller-side `TestAmbiguousCreateNeverRetriesAfterRestart`, the four `TestInventory*` cases, `TestDemandStatisticsAllowControlledProbeButNeverCleanup`, `TestCleanupOnlyForNeverIssuedWorkerWithExactReceipt`, `TestWorkerPreparationReturnsCanonicalSnapshotAndRejectsPriorEffect`, and the named journal/authority contracts in both `./livecanary` and `./liveworker`. | Fixture: record normalized facts and retained uncertainty; no live equality/absence claim. |
| Secret and error handling | SDK error sentinels, HTTP body/debug redaction and shared response-budget controls are unchanged. | See the exact root, livecanary and liveworker selectors in the secret/error command block below, including decoded-body truncation, gzip-budget, uncertain-start, bounded Unix-response/redirect, journal-create-failure and raw-runtime-status tests. | Fixture: record normalized sentinel/error and body-budget behavior; no secret-bearing output or live result is claimed. |
| Tagged controller/JIT input and credential transport boundary | `g01_live` source, input reader, credential attestation and transport refusal tests are unchanged. | Run the tagged command below plus the livecanary credential/transport selector below, including `TestAuthoritySplitAndPolicyRejection`, `TestSDKTransportOwnership`, `TestCredentialAttestationMismatchAndExpiredTokenRejected` and `TestTransportRejectsPlaintextOffHostAndProxyBeforeNetwork`. | Fixture: record no-secret/no-echo/refusal result; no credential or live phase. |
| Idle drain and withdrawal | Only reuse the authoritative current PR #72 head `f5560ba950f77343e57034cc1cf85dc67f5ac922` and its unchanged fixture/source. | On that authoritative current PR #72 checkout, run the guarded `drain-pr72` and `drain-pr72-race` commands below; both carry the same exact selector/package digest, with `default+norace+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8` and `default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8` metadata respectively. | Fixture/source: record physical-write markers as client facts and inconclusive server receipt; never reuse as live assignment/drain evidence. |
| Paired terminal/worker support | paired journal/lease, worker profile, `liveworker` runtime source/implementation, image/runtime pins and terminal selectors are unchanged. | See the exact controller update-policy and worker runtime commands plus the tagged partition commands below; the worker runtime selector includes the direct Docker state/inspection, mutation-EOF and changed-daemon contracts, while tagged partition commands retain `go1.26.8`, `-race`, `-count=1` and `-timeout=120s`, the complete worker `^TestPaired` partition is included, and the controller-side terminal groups remain exhaustive/disjoint. | Fixture: record receipt/identity checks only; no live worker or terminal success claim. |
| Actual live canary | There is no live evidence to reuse today. A future result is reusable only for the same immutable workflow/run attempt, source/head, resources, authority scope and approved observation boundary. | Rebuild/plan the exact reviewed tagged binary, then run only the explicitly authorized phase from [the driver](g01-live-driver.md); never substitute fixture commands or broaden phases. | Live: record sanitized server observations, authorization and unresolved outcomes; any changed target or boundary requires a fresh approval/rerun. |

The two drain prescriptions intentionally bind a different reviewed source tree
from the other packet commands. The default/root, tagged and paired commands
remain pinned to the PR #78 module tree `08c7830de7bc5120d1302d7ba6df162abd582315`;
only `drain-pr72` and `drain-pr72-race` bind the authoritative PR #72 head
`f5560ba950f77343e57034cc1cf85dc67f5ac922` and its module tree
`9b30ef1b69c6375cb264c759d366fc5a52a5439f`. Git-only evidence for that PR #72
source selection, computed without compiling or running tests, is:

```text
git rev-parse f5560ba950f77343e57034cc1cf85dc67f5ac922:experiments/g01-scaleset
9b30ef1b69c6375cb264c759d366fc5a52a5439f
git rev-parse f5560ba950f77343e57034cc1cf85dc67f5ac922:experiments/g01-scaleset/go.mod
46d148aeb4c2672b0ac6bc179273eca156204060
git ls-tree -r --full-tree f5560ba950f77343e57034cc1cf85dc67f5ac922 -- experiments/g01-scaleset | wc -l
126
```

The `go.mod` line above is retained as the reviewed module-file blob evidence;
the module tree object is the binding source identity. The two commands below
are therefore the only prescriptions that may pass the PR #72 tree guard; every
other guarded command must pass the PR #78 tree guard.

The exact focused commands referenced above are recorded here. They are
prescriptions for future reruns, not completed results. None of these
prescriptions reports a completed test or live server success/receipt.

Every runnable test prescription below uses the reusable wrapper in this first
block. It derives the candidate top-level test names from the exact active
`*_test.go` source files selected by a non-executing `go list -json -test`
metadata query, applies a fail-closed Go-RE2-compatible selector subset, and
compares the resulting count and SHA-256 before it invokes the original
command. It deliberately does not invoke `go test -list`: that command starts
the test binary and can run package-level variable initializers, `init` paths,
imported initialization paths and `TestMain` before printing names. The source
derivation therefore validates selectors without executing package code;
slash-delimited `-run`/`-skip` subtest expressions and unsupported regexp
syntax are rejected because source declarations cannot prove their subtest
set or semantics. The selector preflight also rejects Go Perl classes and
boundaries (`\\b`, `\\B`, `\\w`, `\\W`, `\\d`, `\\D`, `\\s`, `\\S`) whose
ASCII Go semantics differ from Python's Unicode defaults, while preserving
escaped literal backslashes and the explicit translated POSIX-class table;
every other nested POSIX class is rejected before Python compilation or package
metadata. The guarded command shape accepts optional leading
`NAME=VALUE` assignments
followed directly by `go test`; standard `env NAME=VALUE ... go test` and
shell `command [options] go test` wrappers are discovered by the static audit
but explicitly rejected before any Go child starts. Go's equivalent double-dash
selector spellings
`--run`/`--run=REGEXP` and `--skip`/`--skip=REGEXP` are likewise rejected
before source validation, so the audit and wrapper never disagree about the
effective selector. Every equivalent double-dash Go flag spelling is rejected
before the first Go child, including `--toolexec`, `--modfile`, `--overlay`,
`--timeout`, `--exec`, `--args`, benchmark, selector and build overrides. The
single-dash `-toolexec FILE`/`-toolexec=FILE`, `-overlay FILE`/
`-overlay=FILE` and `-modfile FILE`/`-modfile=FILE` build/module overrides are
also rejected before the first Go child. Each prescription also carries an
expected package identity
(`module-directory:package`) and build configuration (tag set, race mode,
explicit reviewed `CGO_ENABLED=1` mode, the reviewed default cgo compiler/tool
set, reviewed `GOEXPERIMENT=none` binding, reviewed `GOOS=darwin`,
`GOARCH=arm64`, `GOARM64=v8.0` target and the requested `GOTOOLCHAIN`). Compiler commands, cgo flags/linker controls, pkg-config
selectors and other cgo tool overrides are rejected before any Go child; the
effective default-tool, experiment and target identities are recorded in each
build identity. `GOENV=off` disables persisted Go compiler/tool settings before
any Go child; an inherited or command-supplied GOENV path is rejected. The
reviewed GOROOT default (`GOROOT=""`) and `GOFIPS140=off` are likewise pinned
before any Go child; non-default inherited or command-supplied values fail
closed, and `goroot-default`/`gofips140-off` are bound into every build
identity. The reviewed `GOSUMDB=sum.golang.org` and
`GOPROXY=https://proxy.golang.org,direct` trust settings are required before
the first Go child and verified through effective `go env`; custom inherited or
command-supplied values fail closed. The wrapper queries effective
`GOVERSION` before metadata or tests and binds that verified toolchain identity,
not merely the requested `GOTOOLCHAIN`, into every build identity. The reviewed
canonical PATH is pinned before the first Git or Go executable lookup.
`GOCACHEPROG` is rejected unless empty and then pinned empty before any Go child;
`GOAUTH` is rejected unless `off` and then pinned `off`, so executable cache
hooks and command-form authentication cannot reach module, metadata, vet or
test children. Git replacement refs are disabled by binding
`GIT_NO_REPLACE_OBJECTS=1` before the first Git child; other replacement and
repository-control overrides remain rejected.
Before source-derived selector validation, the wrapper resolves the active
package and test source files with the non-executing metadata query, requires the
reviewed immutable `experiments/g01-scaleset` source tree, rejects tracked,
untracked or ignored paths under that module, detects Git `skip-worktree` and
`assume-unchanged` intent bits with `git ls-files -v`, and fails closed if any active
file declares `func init`; the source derivation itself does not execute that
initializer or any imported package initializer. The separate `TestMain` source
audit below covers the one package-level test initializer. This is the explicit
package-initialization guard: a source-tree or build-tag change, including an
ignored Go or test file, requires a new review before selector validation can
proceed, while effectful package-level variables and applicable imported init
paths cannot run during the derivation. Source-derived names use Go's `isTest`
predicate, so valid `Test`, `Test1` and `Test_Foo` declarations are retained;
top-level `Fuzz*` declarations are rejected before digest or selector validation
because the paired-all-except prescription has no `-run` and Go could execute
fuzz seed work that is absent from the source-derived expected set. Executable
`Example` declarations are likewise rejected before a set digest is claimed
because examples are otherwise outside the source-derived test-name set. The
source scanner consumes consecutive whitespace and line/block comments between
declaration tokens, so comments cannot hide a valid top-level test name. Before
opening any candidate source file, the wrapper performs the immutable reviewed
module-tree and tracked/untracked/ignored status gate; only a clean source tree
can reach package metadata and source derivation. The wrapper disables persisted
Go configuration with `GOENV=off` before the first Go child, queries the
effective `go env GOFLAGS`, rejects non-empty output, and then pins `GOFLAGS=`
for metadata and the original command. After
parsing inherited and command-prefix assignments, it rejects every repository-
control `GIT_*` override—including `GIT_WORK_TREE`, `GIT_DIR`,
`GIT_INDEX_FILE` and related repository, object, config, namespace, discovery,
pathspec and replacement controls—before the first Git query or immutable-tree
pathspec and replacement controls—before the first Git query or immutable-tree
validation. Every wrapper-controlled Git query uses `-c core.fsmonitor=false`
and `-c core.hooksPath=/dev/null`, with system/global Git configuration disabled
in the child environment, before trusting source-tree, status/porcelain or
intent-bit results. It also rejects `PATH` and equivalent dynamic loader-affecting
prefixes, validates the inherited PATH against the reviewed canonical
`/opt/homebrew/bin:/usr/bin:/bin`, and creates the Go-child
environment with `GOWORK=off` and passes that exact environment to every Go
metadata and test subprocess; inherited or command-supplied workspace paths are
therefore ignored before package metadata can be selected. The reviewed
behavior is force-off, not validation or reuse of a caller-provided `go.work`
file. The same environment pins `GOOS=darwin`, `GOARCH=arm64` and
`GOARM64=v8.0`; the target identity is included in each build identity.
Race-mode prescriptions reject inherited or command-supplied `GORACE` before
either source derivation or test execution. The wrapper pins `CGO_ENABLED=1`
after command-prefix parsing, rejects a command-supplied conflicting value,
rejects inherited or command-prefix compiler/cgo-tool overrides, and binds the
effective `cgo-tools-default` identity into every build identity before
metadata. Exactly one `-count=1` and one positive bounded
`-timeout` (at most 300 seconds) are required; `-args` and test-binary
selector/count/timeout overrides, Go `-modfile FILE`/`-modfile=FILE`
alternate-module-file overrides, Go `-overlay FILE`/`-overlay=FILE` build
overrides, `-toolexec` execution hooks, benchmark-enabling `-bench` and
`-test.bench` overrides, `-cpu` multiplicity overrides, `env` and shell
`command` wrappers, all double-dash Go flags, and `-exec` execution wrappers
are rejected.
Before package metadata, downloaded module sources are obtained with
`go mod download -json all` and verified with `go mod verify` through the same
bounded Go-child helper and environment. The wrapper requires complete,
existing module metadata/source paths and fails closed on a download error,
missing checksum, missing source or verification failure; it does not print
module paths or cache locations.
Every Go child is launched through one wrapper-controlled subprocess helper with
an independent 300-second deadline covering toolchain selection, toolchain
downloads, compilation, `go env`, `go list` metadata and test execution. The
helper starts a new session/process group, sends timeout termination to the full
group, escalates to group kill after a bounded grace period, and waits for the
direct child to be reaped before refusing the result; the Go `-timeout` flag
remains a separate in-process test-body bound.
The execution command adds a wrapper-controlled `-json` stream and fails closed
on malformed output, non-empty stderr or any Go test `Action` of `skip`,
including skipped subtests. It rejects unexpected top-level `run`, `pass` or
`fail` events, while allowing only subtest events whose parent is an expected
top-level test; it also requires a top-level `run` and `pass` event for every
expected test name. A command failure, unexpected source metadata, zero expected
names, invalid Go/RE2-compatible syntax, count/timeout mismatch, skipped test,
unknown action, missing run/pass event, or set mismatch stops before an
unvalidated result is recorded; the set digest and metadata are recorded beside each prescription so
renamed, removed, build-tagged, newly unskipped or cross-package tests fail
closed. A non-executing source helper therefore cannot produce a green rerun record.

```sh
set -euo pipefail

# The digest is SHA-256 of sorted test names joined with one trailing newline.
# Invocation metadata is: package ID module-dir:package, then build ID
# tags+race-mode+cgo-mode+cgo-tool-identity+goexperiment+target+goroot+gofips140+toolchain (for
# example, experiments/g01-scaleset:./livecanary
# default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8).
# The wrapper checks package initialization/build state, derives test names from
# the same source/build selection without starting a test binary, then runs the
# original command with its own JSON stream.
go_test_checked() {
  # g01-safe-python-heredoc: reviewed dynamic Go-child argv
  [ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || { printf '%s\n' 'reviewed canonical PATH and absolute Python interpreter required' >&2; return 1; }
  /opt/homebrew/bin/python3 -I - "$@" <<'PY'
import hashlib
import json
import os
import re
import signal
import subprocess
import sys
from decimal import Decimal
from pathlib import Path

if len(sys.argv) < 7:
    raise SystemExit(
        "usage: go_test_checked COUNT SET_SHA LABEL PACKAGE_ID BUILD_ID "
        "ENV=VALUE... go test ..."
    )
expected_count = int(sys.argv[1])
expected_digest = sys.argv[2]
label = sys.argv[3]
expected_package = sys.argv[4]
expected_build = sys.argv[5]
command = list(sys.argv[6:])
if (
    expected_count < 0
    or not re.fullmatch(r"[0-9a-f]{64}", expected_digest)
    or (expected_count == 0 and expected_digest != "0" * 64)
):
    raise SystemExit(f"{label}: invalid expected count/digest")
if not re.fullmatch(
    r"experiments/g01-scaleset:(?:\.|\./[A-Za-z0-9._/-]+)",
    expected_package,
):
    raise SystemExit(f"{label}: invalid expected package identity")
build_parts = expected_build.split("+")
if len(build_parts) != 9:
    raise SystemExit(f"{label}: invalid expected build configuration")
(
    expected_tags,
    expected_race,
    expected_cgo,
    expected_cgo_tools,
    expected_goexperiment,
    expected_target,
    expected_goroot,
    expected_gofips140,
    expected_toolchain,
) = build_parts
if expected_tags != "default" and not re.fullmatch(
    r"[A-Za-z0-9_]+(?:,[A-Za-z0-9_]+)*", expected_tags
):
    raise SystemExit(f"{label}: invalid expected build tags")
if expected_race not in {"race", "norace"}:
    raise SystemExit(f"{label}: invalid expected race mode")
if expected_cgo != "cgo1":
    raise SystemExit(f"{label}: invalid expected CGO mode")
if expected_cgo_tools != "cgo-tools-default":
    raise SystemExit(f"{label}: invalid expected cgo compiler/tool identity")
if expected_goexperiment != "goexperiment-none":
    raise SystemExit(f"{label}: invalid expected GOEXPERIMENT identity")
if expected_target != "darwin-arm64-goarm64-v8.0":
    raise SystemExit(f"{label}: invalid expected GOOS/GOARCH/GOARM64 target identity")
if expected_goroot != "goroot-default":
    raise SystemExit(f"{label}: invalid expected GOROOT identity")
if expected_gofips140 != "gofips140-off":
    raise SystemExit(f"{label}: invalid expected GOFIPS140 identity")
if not re.fullmatch(r"go[A-Za-z0-9._-]+", expected_toolchain):
    raise SystemExit(f"{label}: invalid expected toolchain")

invocation_root = Path.cwd().resolve()
env = dict(os.environ)
command_assignments = {}
while command and re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*=.*", command[0]):
    key, value = command.pop(0).split("=", 1)
    command_assignments[key] = value
    env[key] = value
reviewed_goenv = "off"
for source, value in (
    ("inherited", os.environ.get("GOENV")),
    ("command", command_assignments.get("GOENV")),
):
    if value is not None and value != reviewed_goenv:
        raise SystemExit(
            f"{label}: {source} GOENV must be {reviewed_goenv!r}"
        )
env["GOENV"] = reviewed_goenv
reviewed_toolchain = "go1.26.8"
for source, value in (
    ("inherited", os.environ.get("GOTOOLCHAIN")),
    ("command", command_assignments.get("GOTOOLCHAIN")),
):
    if value is not None and value != reviewed_toolchain:
        raise SystemExit(
            f"{label}: {source} GOTOOLCHAIN must be {reviewed_toolchain!r}"
        )
env["GOTOOLCHAIN"] = reviewed_toolchain
reviewed_goroot = ""
for source, value in (
    ("inherited", os.environ.get("GOROOT")),
    ("command", command_assignments.get("GOROOT")),
):
    if value is not None and value != reviewed_goroot:
        raise SystemExit(
            f"{label}: {source} GOROOT must use the reviewed toolchain default"
        )
env["GOROOT"] = reviewed_goroot
reviewed_gofips140 = "off"
for source, value in (
    ("inherited", os.environ.get("GOFIPS140")),
    ("command", command_assignments.get("GOFIPS140")),
):
    if value is not None and value != reviewed_gofips140:
        raise SystemExit(
            f"{label}: {source} GOFIPS140 must be {reviewed_gofips140!r}"
        )
env["GOFIPS140"] = reviewed_gofips140
reviewed_goexperiment = "none"
for source, value in (
    ("inherited", os.environ.get("GOEXPERIMENT")),
    ("command", command_assignments.get("GOEXPERIMENT")),
):
    if value is not None and value != reviewed_goexperiment:
        raise SystemExit(
            f"{label}: {source} GOEXPERIMENT must be {reviewed_goexperiment!r}"
        )
env["GOEXPERIMENT"] = reviewed_goexperiment
reviewed_gosumdb = "sum.golang.org"
reviewed_goproxy = "https://proxy.golang.org,direct"
for source, value in (
    ("inherited", os.environ.get("GOSUMDB")),
    ("command", command_assignments.get("GOSUMDB")),
):
    if value is not None and value != reviewed_gosumdb:
        raise SystemExit(
            f"{label}: {source} GOSUMDB must use the reviewed trust setting"
        )
for source, value in (
    ("inherited", os.environ.get("GOPROXY")),
    ("command", command_assignments.get("GOPROXY")),
):
    if value is not None and value != reviewed_goproxy:
        raise SystemExit(
            f"{label}: {source} GOPROXY must use the reviewed module proxy"
        )
env["GOSUMDB"] = reviewed_gosumdb
env["GOPROXY"] = reviewed_goproxy
reviewed_gocacheprog = ""
for source, value in (
    ("inherited", os.environ.get("GOCACHEPROG")),
    ("command", command_assignments.get("GOCACHEPROG")),
):
    if value is not None and value != reviewed_gocacheprog:
        raise SystemExit(
            f"{label}: {source} GOCACHEPROG must be empty; executable cache hooks are not allowed"
        )
env["GOCACHEPROG"] = reviewed_gocacheprog
reviewed_goauth = "off"
for source, value in (
    ("inherited", os.environ.get("GOAUTH")),
    ("command", command_assignments.get("GOAUTH")),
):
    if value is not None and value != reviewed_goauth:
        raise SystemExit(
            f"{label}: {source} GOAUTH must be {reviewed_goauth!r}; auth command forms are not allowed"
        )
env["GOAUTH"] = reviewed_goauth
reviewed_target_environment = {
    "GOOS": "darwin",
    "GOARCH": "arm64",
    "GOARM64": "v8.0",
}
target_environment_names = {
    "GOOS", "GOARCH", "GOAMD64", "GOARM", "GOARM64", "GO386",
    "GOMIPS", "GOMIPS64", "GOPPC64", "GOWASM",
}
for name in target_environment_names:
    if name not in command_assignments:
        continue
    expected = reviewed_target_environment.get(name)
    if expected is None or command_assignments[name] != expected:
        raise SystemExit(
            f"{label}: command-supplied {name} is not the reviewed target setting"
        )
unreviewed_target_environment = sorted(
    name for name in env
    if name in target_environment_names and name not in reviewed_target_environment
)
if unreviewed_target_environment:
    raise SystemExit(
        f"{label}: inherited target feature settings are not allowed: "
        + ", ".join(unreviewed_target_environment)
    )
env.update(reviewed_target_environment)
compiler_tool_environment_names = {
    "CC",
    "CXX",
    "GCCGO",
    "GOGCCFLAGS",
    "GO_EXTLINK_ENABLED",
    "GO_LDSO",
    "CC_FOR_TARGET",
    "CXX_FOR_TARGET",
    "PKG_CONFIG",
    "PKG_CONFIG_PATH",
    "PKG_CONFIG_LIBDIR",
    "PKG_CONFIG_SYSROOT_DIR",
    # Reject compiler search-path and SDK/deployment overrides before the
    # reviewed cgo/tool identity is computed or any Go child starts.
    "LIBRARY_PATH",
    "C_INCLUDE_PATH",
    "CPLUS_INCLUDE_PATH",
    "OBJC_INCLUDE_PATH",
    "CPATH",
    "SDKROOT",
    "MACOSX_DEPLOYMENT_TARGET",
}
compiler_tool_environment_overrides = sorted(
    key
    for key in env
    if key in compiler_tool_environment_names
    or (key.startswith("CGO_") and key != "CGO_ENABLED")
)
if compiler_tool_environment_overrides:
    raise SystemExit(
        f"{label}: compiler/cgo tool environment overrides are not allowed: "
        + ", ".join(compiler_tool_environment_overrides)
    )
git_repository_control_names = {
    "GIT_DIR",
    "GIT_WORK_TREE",
    "GIT_INDEX_FILE",
    "GIT_INDEX_VERSION",
    "GIT_COMMON_DIR",
    "GIT_OBJECT_DIRECTORY",
    "GIT_OBJECT_DIRECTORY_RELATIVE",
    "GIT_ALTERNATE_OBJECT_DIRECTORIES",
    "GIT_NAMESPACE",
    "GIT_CEILING_DIRECTORIES",
    "GIT_DISCOVERY_ACROSS_FILESYSTEM",
    "GIT_CONFIG",
    "GIT_CONFIG_GLOBAL",
    "GIT_CONFIG_SYSTEM",
    "GIT_CONFIG_NOSYSTEM",
    "GIT_CONFIG_COUNT",
    "GIT_CONFIG_PARAMETERS",
    "GIT_LITERAL_PATHSPECS",
    "GIT_GLOB_PATHSPECS",
    "GIT_NOGLOB_PATHSPECS",
    "GIT_OPTIONAL_LOCKS",
    "GIT_REPLACE_REF_BASE",
    "GIT_ATTR_NOSYSTEM",
    "GIT_QUARANTINE_PATH",
}
git_repository_control_prefixes = ("GIT_CONFIG_KEY_", "GIT_CONFIG_VALUE_")
reviewed_git_no_replace_objects = "1"
for source, value in (
    ("inherited", os.environ.get("GIT_NO_REPLACE_OBJECTS")),
    ("command", command_assignments.get("GIT_NO_REPLACE_OBJECTS")),
):
    if value is not None and value != reviewed_git_no_replace_objects:
        raise SystemExit(
            f"{label}: {source} GIT_NO_REPLACE_OBJECTS must be {reviewed_git_no_replace_objects!r}"
        )
env["GIT_NO_REPLACE_OBJECTS"] = reviewed_git_no_replace_objects
git_environment_overrides = sorted(
    key
    for key in env
    if key in git_repository_control_names
    or any(key.startswith(prefix) for prefix in git_repository_control_prefixes)
)
if git_environment_overrides:
    raise SystemExit(
        f"{label}: Git repository-control environment overrides are not allowed: "
        + ", ".join(git_environment_overrides)
    )
# Do not inherit user/system Git configuration, and force repository hooks and
# fsmonitor off on every wrapper-controlled Git query. The command-line -c
# settings override repository configuration; the fixed environment removes
# user/system configuration before source status/tree/intent results are trusted.
env.update(
    {
        "GIT_CONFIG_NOSYSTEM": "1",
        "GIT_CONFIG_GLOBAL": "/dev/null",
        "GIT_CONFIG_SYSTEM": "/dev/null",
    }
)


def git_command(arguments):
    return [
        "git",
        "-c", "core.fsmonitor=false",
        "-c", "core.hooksPath=/dev/null",
        *arguments,
    ]
loader_assignment_names = {
    "PATH",
    "LD_PRELOAD",
    "LD_PRELOAD_32",
    "LD_PRELOAD_64",
    "LD_LIBRARY_PATH",
    "LD_LIBRARY_PATH_32",
    "LD_LIBRARY_PATH_64",
    "LD_AUDIT",
    "DYLD_INSERT_LIBRARIES",
    "DYLD_LIBRARY_PATH",
    "DYLD_FALLBACK_LIBRARY_PATH",
    "DYLD_FRAMEWORK_PATH",
    "DYLD_FALLBACK_FRAMEWORK_PATH",
    "DYLD_ROOT_PATH",
}
unsafe_loader_assignments = sorted(
    key for key in command_assignments if key in loader_assignment_names
)
if unsafe_loader_assignments:
    raise SystemExit(
        f"{label}: executable-loader environment assignments are not allowed: "
        + ", ".join(unsafe_loader_assignments)
    )
reviewed_path = "/opt/homebrew/bin:/usr/bin:/bin"
if os.pathsep != ":" or os.environ.get("PATH") != reviewed_path:
    raise SystemExit(
        f"{label}: inherited PATH does not match the reviewed canonical executable path"
    )
env["PATH"] = reviewed_path
repo_root = Path(
    subprocess.check_output(
        git_command(["rev-parse", "--show-toplevel"]),
        cwd=invocation_root,
        env=env,
        text=True,
    ).strip()
).resolve()
if invocation_root != repo_root:
    raise SystemExit(f"{label}: run this selector from the repository root")
fixture_child_env = {
    "G01_INPUT_CHILD",
    "G01_NAMED_FIFO_CHILD",
    "G01_HTTP_DEBUG_CHILD",
}
inherited_child_env = sorted(key for key in fixture_child_env if key in env)
if inherited_child_env:
    raise SystemExit(
        f"{label}: fixture child-mode environment is not allowed: "
        + ", ".join(inherited_child_env)
    )
if command and command[0] == "env":
    raise SystemExit(
        f"{label}: standard env-wrapped go test commands are not allowed; "
        "put assignments before go test"
    )
if command and command[0] == "command":
    raise SystemExit(
        f"{label}: shell command-prefix go test wrappers are not allowed; "
        "invoke go test directly"
    )
if len(command) < 3 or command[:2] not in (["go", "test"], ["go", "vet"]):
    raise SystemExit(f"{label}: expected a go test or go vet command")
is_vet = command[1] == "vet"
if is_vet and expected_count != 0:
    raise SystemExit(f"{label}: go vet metadata must use zero test count")
if not is_vet and expected_count <= 0:
    raise SystemExit(f"{label}: go test metadata requires a positive test count")
test_args = command[2:]

# Reject every equivalent double-dash Go flag before any Go child is started.
# Go accepts one or two leading dashes for flags; no guarded prescription needs
# a double-dash form, so rejecting the complete class covers selectors,
# benchmark/test-binary flags and all build/module overrides before `go env`.
double_dash_names = [value.split("=", 1)[0] for value in test_args if value.startswith("--")]
if double_dash_names:
    raise SystemExit(
        f"{label}: equivalent double-dash Go flags are not allowed: "
        + ", ".join(sorted(set(double_dash_names)))
    )

# Pin the reviewed cgo mode before the first Go child. A command-supplied
# conflicting value is rejected; an inherited value is replaced by the exact
# reviewed value below, so metadata and test compilation cannot drift.
if "CGO_ENABLED" in command_assignments and command_assignments["CGO_ENABLED"] != "1":
    raise SystemExit(f"{label}: command-supplied CGO_ENABLED must be 1")
env["CGO_ENABLED"] = "1"

# Keep the single-dash rejection witnesses ahead of package parsing and the
# first Go child as well. The later checks repeat these predicates after the
# common flag parser so the source-derived path has the same fail-closed rules.
if any(value == "-toolexec" or value.startswith("-toolexec=") for value in test_args):
    raise SystemExit(
        f"{label}: Go -toolexec execution hooks are not allowed in a guarded rerun"
    )
if any(value == "-overlay" or value.startswith("-overlay=") for value in test_args):
    raise SystemExit(
        f"{label}: Go -overlay build overrides are not allowed in a guarded rerun"
    )
if any(value == "-modfile" or value.startswith("-modfile=") for value in test_args):
    raise SystemExit(
        f"{label}: Go -modfile alternate module files are not allowed in a guarded rerun"
    )
if any(value == "-args" or value.startswith("-args=") for value in test_args):
    raise SystemExit(f"{label}: -args is not allowed in a guarded rerun")
if any(value == "-exec" or value.startswith("-exec=") for value in test_args):
    raise SystemExit(f"{label}: -exec execution wrappers are not allowed")
if any(
    value == "-bench"
    or value.startswith("-bench=")
    or value == "-benchmem"
    or value == "-test.bench"
    or value.startswith("-test.bench=")
    or value == "-test.benchmem"
    for value in test_args
):
    raise SystemExit(
        f"{label}: benchmark execution or benchmark overrides are not allowed"
    )
if any(value == "-cpu" or value.startswith("-cpu=") for value in test_args):
    raise SystemExit(f"{label}: -cpu multiplicity overrides are not allowed")
if any(value == "-json" or value.startswith("-json=") for value in test_args):
    raise SystemExit(f"{label}: wrapper controls the test JSON stream")
if any(value == "-list" or value.startswith("-list=") for value in test_args):
    raise SystemExit(f"{label}: -list test-binary discovery is not allowed")
early_test_binary_overrides = (
    "-test.run",
    "-test.skip",
    "-test.count",
    "-test.timeout",
    "-test.list",
)
if any(
    value == override or value.startswith(override + "=")
    for value in test_args
    for override in early_test_binary_overrides
):
    raise SystemExit(
        f"{label}: test-binary selector/count/timeout overrides are not allowed"
    )

def pre_metadata_flag_values(args, flag):
    values = []
    index = 0
    while index < len(args):
        value = args[index]
        if value == flag:
            if index + 1 >= len(args):
                raise SystemExit(f"{label}: {flag} requires a value")
            values.append(args[index + 1])
            index += 2
            continue
        if value.startswith(f"{flag}="):
            values.append(value[len(flag) + 1:])
        index += 1
    return values

# Reject malformed or extra package/import-path arguments before the first Go
# child, including `go test ./expected example/import/path` and absolute or
# module-qualified package patterns that the old `.`/`./...` scan overlooked.
pre_value_flags = {"-C", "-tags", "-run", "-skip", "-list", "-count", "-timeout"}
pre_boolean_flags = {"-race", "-v"}
pre_package_values = []
index = 0
while index < len(test_args):
    value = test_args[index]
    if value in pre_value_flags:
        if index + 1 >= len(test_args):
            raise SystemExit(f"{label}: {value} requires a value")
        index += 2
        continue
    if any(value.startswith(flag + "=") for flag in pre_value_flags):
        index += 1
        continue
    if value in pre_boolean_flags or any(
        value.startswith(flag + "=") for flag in pre_boolean_flags
    ):
        index += 1
        continue
    if value.startswith("-"):
        raise SystemExit(
            f"{label}: unsupported or unreviewed Go/test flag "
            f"{value.split('=', 1)[0]}"
        )
    pre_package_values.append(value)
    index += 1
if len(pre_package_values) != 1:
    raise SystemExit(f"{label}: expected one package argument before metadata")
pre_modules = pre_metadata_flag_values(test_args, "-C")
if pre_modules != ["experiments/g01-scaleset"]:
    raise SystemExit(f"{label}: expected one -C module directory before metadata")
if f"{pre_modules[0]}:{pre_package_values[0]}" != expected_package:
    raise SystemExit(
        f"{label}: expected package {expected_package}, observed "
        f"{pre_modules[0]}:{pre_package_values[0]}"
    )

# Pin the environment after parsing command-prefix assignments. This overrides
# both inherited and command-supplied GOWORK values before the first Go child.
env["GOWORK"] = "off"
go_env = dict(env)
if go_env.get("GOWORK") != "off":
    raise SystemExit(f"{label}: Go child environment did not pin GOWORK=off")
if go_env.get("GOENV") != reviewed_goenv:
    raise SystemExit(
        f"{label}: Go child environment did not disable GOENV with {reviewed_goenv}"
    )
if go_env.get("GOROOT") != reviewed_goroot:
    raise SystemExit(
        f"{label}: Go child environment did not pin the reviewed GOROOT default"
    )
if go_env.get("GOFIPS140") != reviewed_gofips140:
    raise SystemExit(
        f"{label}: Go child environment did not pin GOFIPS140={reviewed_gofips140}"
    )
if go_env.get("GOEXPERIMENT") != reviewed_goexperiment:
    raise SystemExit(
        f"{label}: Go child environment did not pin GOEXPERIMENT={reviewed_goexperiment}"
    )
if go_env.get("GOTOOLCHAIN") != reviewed_toolchain:
    raise SystemExit(
        f"{label}: Go child environment did not pin GOTOOLCHAIN={reviewed_toolchain}"
    )
if go_env.get("GOSUMDB") != reviewed_gosumdb:
    raise SystemExit(
        f"{label}: Go child environment did not pin GOSUMDB={reviewed_gosumdb}"
    )
if go_env.get("GOPROXY") != reviewed_goproxy:
    raise SystemExit(
        f"{label}: Go child environment did not pin the reviewed GOPROXY"
    )
if go_env.get("GOCACHEPROG") != reviewed_gocacheprog:
    raise SystemExit(
        f"{label}: Go child environment did not pin GOCACHEPROG={reviewed_gocacheprog!r}"
    )
if go_env.get("GOAUTH") != reviewed_goauth:
    raise SystemExit(
        f"{label}: Go child environment did not pin GOAUTH={reviewed_goauth!r}"
    )
for name, expected in reviewed_target_environment.items():
    if go_env.get(name) != expected:
        raise SystemExit(
            f"{label}: Go child environment did not pin {name}={expected}"
        )

go_child_deadline_seconds = 300
go_child_termination_grace_seconds = 5


def terminate_go_child_group(process):
    """Terminate and reap a timed-out Go child session/process group."""
    try:
        os.killpg(process.pid, signal.SIGTERM)
    except ProcessLookupError:
        pass
    try:
        process.communicate(timeout=go_child_termination_grace_seconds)
    except subprocess.TimeoutExpired:
        try:
            os.killpg(process.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        process.communicate()
    process.wait()


def run_go_child(go_command, **kwargs):
    if not go_command or go_command[0] != "go":
        raise SystemExit(f"{label}: non-Go command passed to Go child runner")
    check = kwargs.pop("check", False)
    if kwargs.pop("start_new_session", False):
        raise SystemExit(f"{label}: Go child session ownership is wrapper-controlled")
    if kwargs.pop("capture_output", False):
        if "stdout" in kwargs or "stderr" in kwargs:
            raise SystemExit(f"{label}: duplicate Go child output capture controls")
        kwargs["stdout"] = subprocess.PIPE
        kwargs["stderr"] = subprocess.PIPE
    try:
        process = subprocess.Popen(
            go_command,
            start_new_session=True,
            **kwargs,
        )
        try:
            stdout, stderr = process.communicate(timeout=go_child_deadline_seconds)
        except subprocess.TimeoutExpired:
            terminate_go_child_group(process)
            raise SystemExit(
                f"{label}: Go child exceeded independent "
                f"{go_child_deadline_seconds}s deadline; process group terminated and reaped"
            )
        result = subprocess.CompletedProcess(go_command, process.returncode, stdout, stderr)
        if check and result.returncode:
            raise subprocess.CalledProcessError(
                result.returncode, go_command, output=stdout, stderr=stderr
            )
        return result
    except SystemExit:
        raise

effective_toolchain_result = run_go_child(
    ["go", "env", "GOVERSION", "GOSUMDB", "GOPROXY"],
    cwd=repo_root,
    env=go_env,
    text=True,
    capture_output=True,
    check=False,
)
if effective_toolchain_result.returncode != 0 or effective_toolchain_result.stderr.strip():
    raise SystemExit(f"{label}: effective toolchain/trust query failed")
effective_toolchain_lines = effective_toolchain_result.stdout.splitlines()
if effective_toolchain_lines != [
    reviewed_toolchain,
    reviewed_gosumdb,
    reviewed_goproxy,
]:
    raise SystemExit(
        f"{label}: effective toolchain/trust identity did not match the reviewed pin"
    )
effective_toolchain_identity = effective_toolchain_lines[0]

effective_goflags = run_go_child(
    ["go", "env", "GOFLAGS"],
    cwd=repo_root,
    env=go_env,
    text=True,
    capture_output=True,
    check=False,
)
if effective_goflags.returncode != 0 or effective_goflags.stderr.strip():
    raise SystemExit(f"{label}: effective GOFLAGS query failed")
effective_lines = effective_goflags.stdout.splitlines()
if len(effective_lines) > 1:
    raise SystemExit(f"{label}: effective GOFLAGS query was not single-line")
if effective_lines and effective_lines[0].strip():
    raise SystemExit(f"{label}: effective GOFLAGS must be empty")
go_env["GOFLAGS"] = ""

def flag_values(args, flag):
    values = []
    index = 0
    while index < len(args):
        value = args[index]
        if value == flag:
            if index + 1 >= len(args):
                raise SystemExit(f"{label}: {flag} requires a value")
            values.append(args[index + 1])
            index += 2
            continue
        if value.startswith(f"{flag}="):
            values.append(value[len(flag) + 1:])
        index += 1
    return values

module_values = flag_values(test_args, "-C")
if len(module_values) != 1:
    raise SystemExit(f"{label}: expected one -C module directory")
module_dir = module_values[0]
if module_dir != "experiments/g01-scaleset":
    raise SystemExit(f"{label}: unexpected module directory {module_dir!r}")

tag_values = flag_values(test_args, "-tags")
if len(tag_values) > 1:
    raise SystemExit(f"{label}: expected at most one -tags flag")
if tag_values:
    tags = tag_values[0].split(",")
    if not tags or any(not re.fullmatch(r"[A-Za-z0-9_]+", tag) for tag in tags):
        raise SystemExit(f"{label}: invalid -tags value")
    if len(tags) != len(set(tags)):
        raise SystemExit(f"{label}: duplicate build tag")
    tag_identity = ",".join(sorted(tags))
else:
    tag_identity = "default"

race_modes = []
for value in test_args:
    if value == "-race":
        race_modes.append("race")
    elif value.startswith("-race="):
        race_value = value[len("-race="):]
        if race_value == "true":
            race_modes.append("race")
        elif race_value == "false":
            race_modes.append("norace")
        else:
            raise SystemExit(f"{label}: invalid -race value")
if len(race_modes) > 1:
    raise SystemExit(f"{label}: expected at most one -race flag")
race_identity = race_modes[0] if race_modes else "norace"
toolchain_identity = effective_toolchain_identity
cgo_identity = "cgo" + env.get("CGO_ENABLED", "")
compiler_tool_identity = "cgo-tools-default"
goexperiment_identity = "goexperiment-" + reviewed_goexperiment
target_identity = "darwin-arm64-goarm64-v8.0"
goroot_identity = "goroot-default"
gofips140_identity = "gofips140-off"
actual_build = (
    f"{tag_identity}+{race_identity}+{cgo_identity}+{compiler_tool_identity}+"
    f"{goexperiment_identity}+{target_identity}+{goroot_identity}+"
    f"{gofips140_identity}+{toolchain_identity}"
)
if actual_build != expected_build:
    raise SystemExit(
        f"{label}: expected build {expected_build}, observed {actual_build}"
    )
if race_identity == "race" and "GORACE" in env:
    raise SystemExit(
        f"{label}: inherited or command-supplied GORACE is not allowed in race mode"
    )

reviewed_pr78_module_tree = "08c7830de7bc5120d1302d7ba6df162abd582315"
reviewed_pr72_module_tree = "9b30ef1b69c6375cb264c759d366fc5a52a5439f"
if label.startswith("drain-") and label not in {"drain-pr72", "drain-pr72-race"}:
    raise SystemExit(f"{label}: unknown drain source pin")
reviewed_module_tree = (
    reviewed_pr72_module_tree
    if label in {"drain-pr72", "drain-pr72-race"}
    else reviewed_pr78_module_tree
)


def json_objects(raw, phase):
    decoder = json.JSONDecoder()
    position = 0
    objects = []
    while position < len(raw):
        while position < len(raw) and raw[position].isspace():
            position += 1
        if position >= len(raw):
            break
        try:
            value, position = decoder.raw_decode(raw, position)
        except json.JSONDecodeError:
            raise SystemExit(f"{label}: {phase} emitted invalid JSON")
        if not isinstance(value, dict):
            raise SystemExit(f"{label}: {phase} emitted a non-object JSON value")
        objects.append(value)
    if not objects:
        raise SystemExit(f"{label}: {phase} emitted no package metadata")
    return objects


def verify_downloaded_module_sources():
    """Download and cryptographically verify module sources through bounded Go children."""
    module_root = (repo_root / module_dir).resolve()
    download = run_go_child(
        ["go", "mod", "download", "-json", "all"],
        cwd=module_root,
        env=go_env,
        text=True,
        capture_output=True,
        check=False,
    )
    if download.returncode != 0 or download.stderr.strip():
        raise SystemExit(f"{label}: module source download/metadata query failed")
    modules = json_objects(download.stdout, "module source metadata")
    for module in modules:
        if module.get("Error"):
            raise SystemExit(f"{label}: module source download reported an error")
        required = ("Path", "Version", "GoMod", "Dir", "Sum", "GoModSum")
        if any(not isinstance(module.get(name), str) or not module[name].strip() for name in required):
            raise SystemExit(f"{label}: module source metadata was incomplete")
        if not Path(module["GoMod"]).is_file() or not Path(module["Dir"]).is_dir():
            raise SystemExit(f"{label}: downloaded module source path was missing")
    verified = run_go_child(
        ["go", "mod", "verify"],
        cwd=module_root,
        env=go_env,
        text=True,
        capture_output=True,
        check=False,
    )
    if verified.returncode != 0 or verified.stderr.strip():
        raise SystemExit(f"{label}: downloaded module source verification failed")


def git_source_control_entries(repo_root, module_dir, env):
    source_flags = subprocess.run(
        git_command(["ls-files", "-v", "--full-name", "--", module_dir]),
        cwd=repo_root,
        env=env,
        text=True,
        capture_output=True,
        check=False,
    )
    if source_flags.returncode != 0 or source_flags.stderr.strip():
        raise SystemExit(f"{label}: source intent-bit query failed")
    return [
        line for line in source_flags.stdout.splitlines()
        if line and line[0] in {"S", "s", "h"}
    ]


def package_initialization_guard():
    source_tree = subprocess.run(
        git_command(["rev-parse", f"HEAD:{module_dir}"]),
        cwd=repo_root,
        env=env,
        text=True,
        capture_output=True,
        check=False,
    )
    if source_tree.returncode != 0 or source_tree.stderr.strip():
        raise SystemExit(f"{label}: reviewed source-tree query failed")
    if source_tree.stdout.strip() != reviewed_module_tree:
        raise SystemExit(
            f"{label}: package-initialization guard requires reviewed source tree"
        )
    source_status = subprocess.run(
        git_command([
            "status",
            "--porcelain=v1",
            "--untracked-files=all",
            "--ignored=matching",
            "--",
            module_dir,
        ]),
        cwd=repo_root,
        env=env,
        text=True,
        capture_output=True,
        check=False,
    )
    if source_status.returncode != 0 or source_status.stderr.strip():
        raise SystemExit(f"{label}: package source status query failed")
    if source_status.stdout.strip():
        raise SystemExit(
            f"{label}: package-initialization guard requires a clean source tree "
            "with no tracked, untracked or ignored paths"
        )
    source_control_entries = git_source_control_entries(repo_root, module_dir, env)
    if source_control_entries:
        raise SystemExit(
            f"{label}: source has skip-worktree or assume-unchanged entries"
        )
    verify_downloaded_module_sources()
    list_command = ["go", "list", "-C", module_dir, "-json", "-test"]
    if race_identity == "race":
        list_command.append("-race")
    if expected_tags != "default":
        list_command.append("-tags=" + expected_tags)
    list_command.append(package_value)
    metadata = run_go_child(
        list_command,
        cwd=repo_root,
        env=go_env,
        text=True,
        capture_output=True,
        check=False,
    )
    if metadata.returncode != 0 or metadata.stderr.strip():
        raise SystemExit(f"{label}: package-initialization metadata query failed")
    packages = [
        item
        for item in json_objects(metadata.stdout, "package-initialization metadata")
        if item.get("ForTest") is None
        and not item.get("ImportPath", "").endswith(".test")
    ]
    if len(packages) != 1:
        raise SystemExit(
            f"{label}: package-initialization metadata was ambiguous"
        )
    package = packages[0]
    package_dir = Path(package.get("Dir", "")).resolve()
    try:
        package_dir.relative_to(repo_root / module_dir)
    except ValueError:
        raise SystemExit(f"{label}: package source escaped the reviewed module")
    source_files = []
    test_source_files = []
    for key in ("GoFiles", "CgoFiles", "TestGoFiles", "XTestGoFiles"):
        values = package.get(key, [])
        if not isinstance(values, list):
            raise SystemExit(f"{label}: package file metadata was malformed")
        source_files.extend(values)
        if key in {"TestGoFiles", "XTestGoFiles"}:
            test_source_files.extend(values)
    if not source_files:
        raise SystemExit(f"{label}: package-initialization source set was empty")
    if not test_source_files:
        raise SystemExit(f"{label}: package test source set was empty")
    resolved_test_sources = []
    for name in source_files:
        source_path = (package_dir / name).resolve()
        try:
            source_path.relative_to(package_dir)
        except ValueError:
            raise SystemExit(f"{label}: package source file escaped its directory")
        if not source_path.is_file():
            raise SystemExit(f"{label}: package source file was missing")
        source = source_path.read_text(encoding="utf-8")
        if source_has_init_declaration(source):
            raise SystemExit(
                f"{label}: active package init requires a new reviewed guard"
            )
        if name in test_source_files:
            resolved_test_sources.append(source_path)
    return resolved_test_sources

skip_patterns = []
list_base_args = []
index = 0
while index < len(test_args):
    value = test_args[index]
    if value == "-skip":
        if index + 1 >= len(test_args):
            raise SystemExit(f"{label}: -skip requires an expression")
        skip_patterns.append(test_args[index + 1])
        index += 2
        continue
    if value.startswith("-skip="):
        skip_patterns.append(value[len("-skip="):])
        index += 1
        continue
    list_base_args.append(value)
    index += 1
if len(skip_patterns) > 1:
    raise SystemExit(f"{label}: expected at most one -skip flag")
if skip_patterns and "/" in skip_patterns[0]:
    raise SystemExit(
        f"{label}: slash-delimited -skip selectors are rejected because "
        "source declarations cannot validate subtest names"
    )

count_values = flag_values(test_args, "-count")
timeout_values = flag_values(test_args, "-timeout")
if is_vet:
    if count_values or timeout_values:
        raise SystemExit(f"{label}: go vet does not accept test count/timeout controls")
else:
    if len(count_values) != 1 or count_values[0] != "1":
        raise SystemExit(f"{label}: exactly one -count=1 is required")
    if len(timeout_values) != 1:
        raise SystemExit(f"{label}: exactly one -timeout value is required")
    duration_token = re.compile(r"(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:ns|us|µs|ms|s|m|h)")
    duration_units = {
        "ns": Decimal("1"),
        "us": Decimal("1000"),
        "µs": Decimal("1000"),
        "ms": Decimal("1000000"),
        "s": Decimal("1000000000"),
        "m": Decimal("60000000000"),
        "h": Decimal("3600000000000"),
    }
    timeout_text = timeout_values[0]
    if timeout_text == "0" or timeout_text.startswith("-"):
        raise SystemExit(f"{label}: -timeout must be positive and at most 300s")
    timeout_matches = list(duration_token.finditer(timeout_text))
    if not timeout_matches or "".join(match.group(0) for match in timeout_matches) != timeout_text:
        raise SystemExit(f"{label}: invalid -timeout duration")
    timeout_nanos = Decimal("0")
    for match in timeout_matches:
        number = match.group(0)
        unit = next(suffix for suffix in duration_units if number.endswith(suffix))
        timeout_nanos += Decimal(number[:-len(unit)]) * duration_units[unit]
    if timeout_nanos <= 0 or timeout_nanos > Decimal("300000000000"):
        raise SystemExit(f"{label}: -timeout must be positive and at most 300s")

package_value_flags = {"-C", "-tags", "-run", "-list", "-count", "-timeout"}
package_boolean_flags = {"-race", "-v"}
package_indices = []
index = 0
while index < len(list_base_args):
    value = list_base_args[index]
    if value in package_value_flags:
        if index + 1 >= len(list_base_args):
            raise SystemExit(f"{label}: {value} requires a value")
        index += 2
        continue
    if any(value.startswith(flag + "=") for flag in package_value_flags):
        index += 1
        continue
    if value in package_boolean_flags or any(
        value.startswith(flag + "=") for flag in package_boolean_flags
    ):
        index += 1
        continue
    if value.startswith("-"):
        raise SystemExit(f"{label}: unsupported or unreviewed Go/test flag {value.split('=', 1)[0]}")
    package_indices.append(index)
    index += 1
if len(package_indices) != 1:
    raise SystemExit(f"{label}: expected one package argument")
package_value = list_base_args[package_indices[0]]
actual_package = f"{module_dir}:{package_value}"
if actual_package != expected_package:
    raise SystemExit(
        f"{label}: expected package {expected_package}, observed {actual_package}"
    )

def source_fuzz_declarations(source_path):
    source = source_path.read_text(encoding="utf-8")

    def skip_ignored(position):
        while position < len(source):
            if source[position].isspace():
                position += 1
                continue
            if source.startswith("//", position):
                newline = source.find("\n", position + 2)
                return len(source) if newline < 0 else newline + 1
            if source.startswith("/*", position):
                close = source.find("*/", position + 2)
                if close < 0:
                    raise SystemExit(f"{label}: unterminated Go block comment")
                position = close + 2
                continue
            break
        return position

    def skip_literal(position):
        quote = source[position]
        position += 1
        while position < len(source):
            if quote != "`" and source[position] == "\\":
                position += 2
                continue
            if source[position] == quote:
                return position + 1
            position += 1
        raise SystemExit(f"{label}: unterminated Go literal")

    names = []
    position = 0
    brace_depth = 0
    while position < len(source):
        ignored = skip_ignored(position)
        if ignored != position:
            position = ignored
            continue
        if source[position] in ('"', "'", "`"):
            position = skip_literal(position)
            continue
        if source[position] == "{":
            brace_depth += 1
            position += 1
            continue
        if source[position] == "}":
            if brace_depth == 0:
                raise SystemExit(f"{label}: unbalanced Go source braces")
            brace_depth -= 1
            position += 1
            continue
        if brace_depth != 0 or not source.startswith("func", position):
            position += 1
            continue
        before = source[position - 1] if position else " "
        after = source[position + 4] if position + 4 < len(source) else " "
        if (before.isalnum() or before == "_") or (after.isalnum() or after == "_"):
            position += 1
            continue
        cursor = skip_ignored(position + 4)
        if cursor >= len(source) or source[cursor] == "(":
            position += 4
            continue
        if not (source[cursor].isalpha() or source[cursor] == "_"):
            position += 4
            continue
        name_start = cursor
        cursor += 1
        while cursor < len(source) and (source[cursor].isalnum() or source[cursor] == "_"):
            cursor += 1
        name = source[name_start:cursor]
        cursor = skip_ignored(cursor)
        if cursor < len(source) and source[cursor] == "(" and name.startswith("Fuzz"):
            names.append(name)
        position = cursor
    if brace_depth:
        raise SystemExit(f"{label}: unbalanced Go source braces")
    return names


def source_fuzz_guard():
    package_dir = (repo_root / module_dir / package_value).resolve()
    try:
        package_dir.relative_to(repo_root / module_dir)
    except ValueError:
        raise SystemExit(f"{label}: package source escaped the reviewed module")
    if not package_dir.is_dir():
        raise SystemExit(f"{label}: package source directory was missing")
    for source_path in sorted(package_dir.glob("*_test.go")):
        try:
            source_path.resolve().relative_to(package_dir)
        except ValueError:
            raise SystemExit(f"{label}: package test source escaped its directory")
        fuzz_names = source_fuzz_declarations(source_path)
        if fuzz_names:
            raise SystemExit(
                f"{label}: top-level Fuzz* declaration requires a new reviewed guard: "
                + ", ".join(sorted(set(fuzz_names)))
            )


def reject_python_semantic_regexp_constructs(pattern, phase):
    # Go's Perl classes/boundaries are ASCII-oriented while Python's are
    # Unicode-oriented. Consume escaped pairs so a literal \\b remains valid.
    position = 0
    while position < len(pattern):
        if pattern[position] != "\\":
            position += 1
            continue
        position += 1
        if position < len(pattern):
            if pattern[position] in "bBwWdDsS":
                raise SystemExit(
                    f"{label}: {phase} uses Go Perl regexp construct "
                    f"\\{pattern[position]} whose Python semantics differ"
                )
            position += 1


go_posix_class_translations = {
    # These are the reviewed class contents; the scanner only permits names
    # present in this explicit table.
    "alnum": "A-Za-z0-9",
    "alpha": "A-Za-z",
    "digit": "0-9",
    "lower": "a-z",
    "upper": "A-Z",
    "space": r"\t\n\r\f ",
    "word": "A-Za-z0-9_",
}


def translate_go_posix_classes(pattern, phase):
    translated = []
    position = 0
    in_class = False
    while position < len(pattern):
        if pattern[position] == "\\":
            translated.append(pattern[position:position + 2])
            position += 2
            continue
        if in_class and pattern.startswith("[:", position):
            close = pattern.find(":]", position + 2)
            if close >= 0:
                class_name = pattern[position + 2:close]
                replacement = go_posix_class_translations.get(class_name)
                if replacement is None:
                    raise SystemExit(
                        f"{label}: {phase} uses unsupported POSIX regexp class "
                        f"[:{class_name}:]"
                    )
                translated.append(replacement)
                position = close + 2
                continue
        if pattern[position] == "[":
            in_class = True
            translated.append(pattern[position])
            position += 1
            continue
        if pattern[position] == "]" and in_class:
            in_class = False
        translated.append(pattern[position])
        position += 1
    return "".join(translated)


def reject_unsupported_regexp_syntax(pattern, phase):
    reject_python_semantic_regexp_constructs(pattern, phase)
    translate_go_posix_classes(pattern, phase)
    if "(?" in pattern or re.search(r"\\[1-9]", pattern):
        raise SystemExit(f"{label}: {phase} uses regexp syntax outside the reviewed Go subset")


run_indices = [
    index for index, value in enumerate(list_base_args)
    if value == "-run" or value.startswith("-run=")
]
if len(run_indices) > 1:
    raise SystemExit(f"{label}: expected at most one -run flag")
if flag_values(test_args, "-list"):
    raise SystemExit(f"{label}: original command must not contain -list")

original_run_pattern = "."
if run_indices:
    run_index = run_indices[0]
    original_run_pattern = (
        list_base_args[run_index + 1]
        if list_base_args[run_index] == "-run"
        else list_base_args[run_index][len("-run="):]
    )
if "/" in original_run_pattern:
    raise SystemExit(
        f"{label}: slash-delimited -run selectors are rejected because "
        "source declarations cannot validate subtest names"
    )
reject_unsupported_regexp_syntax(original_run_pattern, "-run")
for skip_pattern in skip_patterns:
    reject_unsupported_regexp_syntax(skip_pattern, "-skip")

def skip_source_ignored(source, position):
    while position < len(source):
        if source[position].isspace():
            position += 1
            continue
        if source.startswith("//", position):
            newline = source.find("\n", position + 2)
            position = len(source) if newline < 0 else newline + 1
            continue
        if source.startswith("/*", position):
            close = source.find("*/", position + 2)
            if close < 0:
                raise SystemExit(f"{label}: unterminated Go block comment")
            position = close + 2
            continue
        break
    return position


def source_has_init_declaration(source):
    """Recognize top-level `func init()` with comments between Go tokens."""
    position = 0
    brace_depth = 0
    while position < len(source):
        ignored = skip_source_ignored(source, position)
        if ignored != position:
            position = ignored
            continue
        if source[position] in ('"', "'", "`"):
            position = skip_source_literal(source, position)
            continue
        if source[position] == "{":
            brace_depth += 1
            position += 1
            continue
        if source[position] == "}":
            if brace_depth == 0:
                raise SystemExit(f"{label}: unbalanced Go source braces")
            brace_depth -= 1
            position += 1
            continue
        if brace_depth != 0 or not source.startswith("func", position):
            position += 1
            continue
        before = source[position - 1] if position else " "
        after = source[position + 4] if position + 4 < len(source) else " "
        if (before.isalnum() or before == "_") or (after.isalnum() or after == "_"):
            position += 1
            continue
        cursor = skip_source_ignored(source, position + 4)
        if cursor >= len(source) or source[cursor] == "(":
            position += 4
            continue
        if not source.startswith("init", cursor):
            position += 4
            continue
        init_after = cursor + 4
        if init_after < len(source) and (
            source[init_after].isalnum() or source[init_after] == "_"
        ):
            position += 4
            continue
        cursor = skip_source_ignored(source, init_after)
        if cursor < len(source) and source[cursor] == "(":
            return True
        position += 4
    if brace_depth:
        raise SystemExit(f"{label}: unbalanced Go source braces")
    return False


def skip_source_literal(source, position):
    quote = source[position]
    position += 1
    while position < len(source):
        if quote != "`" and source[position] == "\\":
            position += 2
            continue
        if source[position] == quote:
            return position + 1
        position += 1
    raise SystemExit(f"{label}: unterminated Go literal")


def is_go_lower_rune(rune):
    # cmd/go uses unicode.IsLower on the first suffix rune. Python str.islower
    # is not equivalent because it treats some non-Ll letters, such as U+00AA,
    # as lowercase; keep the Go Unicode lowercase-letter category exactly.
    import unicodedata

    return unicodedata.category(rune) == "Ll"


def source_test_names(source_path):
    source = source_path.read_text(encoding="utf-8")
    names = []
    position = 0
    brace_depth = 0
    while position < len(source):
        ignored = skip_source_ignored(source, position)
        if ignored != position:
            position = ignored
            continue
        if source[position] in ('"', "'", "`"):
            position = skip_source_literal(source, position)
            continue
        if source[position] == "{":
            brace_depth += 1
            position += 1
            continue
        if source[position] == "}":
            if brace_depth == 0:
                raise SystemExit(f"{label}: unbalanced Go source braces")
            brace_depth -= 1
            position += 1
            continue
        if brace_depth != 0 or not source.startswith("func", position):
            position += 1
            continue
        before = source[position - 1] if position else " "
        after = source[position + 4] if position + 4 < len(source) else " "
        if (before.isalnum() or before == "_") or (after.isalnum() or after == "_"):
            position += 1
            continue
        cursor = skip_source_ignored(source, position + 4)
        if cursor >= len(source) or source[cursor] == "(":
            position += 4
            continue
        name_start = cursor
        if not (source[cursor].isalpha() or source[cursor] == "_"):
            position += 4
            continue
        cursor += 1
        while cursor < len(source) and (source[cursor].isalnum() or source[cursor] == "_"):
            cursor += 1
        name = source[name_start:cursor]
        cursor = skip_source_ignored(source, cursor)
        if cursor < len(source) and source[cursor] == "(" and name != "TestMain":
            if name.startswith("Fuzz"):
                raise SystemExit(
                    f"{label}: top-level Fuzz* declaration requires a new reviewed guard"
                )
            if name.startswith("Example"):
                raise SystemExit(
                    f"{label}: executable Example declaration requires a new reviewed guard"
                )
            if name.startswith("Test"):
                suffix = name[len("Test"):]
                if not suffix or not is_go_lower_rune(suffix[0]):
                    names.append(name)
        position = cursor
    if brace_depth:
        raise SystemExit(f"{label}: unbalanced Go source braces")
    return names


def go_compatible_regexp(pattern, phase):
    # Python's engine is used only for this non-executing derivation. Translate
    # the POSIX classes used by the reviewed selectors and reject constructs
    # that Python might accept but Go RE2 does not.
    reject_unsupported_regexp_syntax(pattern, phase)
    pattern = translate_go_posix_classes(pattern, phase)
    try:
        return re.compile(pattern)
    except re.error as error:
        raise SystemExit(f"{label}: {phase} has invalid Go-compatible regexp: {error}")


# The package tree/status gate above must complete before this broader source
# scan opens any candidate *_test.go path. In particular, an ignored FIFO or
# other special file is rejected by git status before this glob/read path.
test_source_paths = package_initialization_guard()
if is_vet:
    vet_result = run_go_child(
        command,
        cwd=repo_root,
        env=go_env,
        text=True,
        capture_output=True,
        check=False,
    )
    if vet_result.returncode or vet_result.stderr.strip():
        raise SystemExit(f"{label}: go vet failed or emitted stderr")
    print(
        f"{label}: bounded vet validation passed; package {actual_package}; "
        f"build {actual_build}; source tree {reviewed_module_tree}; "
        f"GOWORK={go_env['GOWORK']}; compiler-tools={compiler_tool_identity}"
    )
    raise SystemExit(0)
source_fuzz_guard()

all_test_names = []
for source_path in test_source_paths:
    all_test_names.extend(source_test_names(source_path))
if not all_test_names or len(all_test_names) != len(set(all_test_names)):
    raise SystemExit(f"{label}: source-derived test names were empty or duplicated")
run_regexp = go_compatible_regexp(original_run_pattern, "-run")
listed = sorted(name for name in all_test_names if run_regexp.search(name))
skipped = set()
if skip_patterns:
    skip_regexp = go_compatible_regexp(skip_patterns[0], "-skip")
    skipped = {
        name for name in all_test_names if skip_regexp.search(name)
    }
actual = [name for name in listed if name not in skipped]
actual_digest = hashlib.sha256(
    ("\n".join(sorted(actual)) + "\n").encode()
).hexdigest()
if len(actual) != expected_count or len(set(actual)) != expected_count:
    raise SystemExit(
        f"{label}: expected {expected_count} names, observed {len(actual)}"
    )
if actual_digest != expected_digest:
    raise SystemExit(
        f"{label}: expected set {expected_digest}, observed {actual_digest}"
    )
print(
    f"{label}: source selector validation passed; package {actual_package}; "
    f"build {actual_build}; CGO_ENABLED={go_env['CGO_ENABLED']}; "
    f"GOEXPERIMENT={go_env['GOEXPERIMENT']}; compiler-tools={compiler_tool_identity}; "
    f"GOROOT=default; GOFIPS140={go_env['GOFIPS140']}; "
    f"GOWORK={go_env['GOWORK']}; source-derived candidates {len(listed)}; "
    f"filtered executed {expected_count} names; "
    f"set-sha256 {actual_digest}"
)

def validate_test_stream(stdout, stderr):
    if stderr.strip():
        raise SystemExit(f"{label}: test execution emitted stderr")
    expected_names = set(actual)
    run_names = set()
    pass_names = set()
    allowed_actions = {"start", "run", "pause", "cont", "output", "build-output", "pass", "fail", "skip"}
    for line in stdout.splitlines():
        if not line:
            continue
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            raise SystemExit(f"{label}: test execution emitted invalid JSON")
        if not isinstance(event, dict):
            raise SystemExit(f"{label}: test execution emitted a non-object JSON value")
        action = event.get("Action")
        if action not in allowed_actions:
            raise SystemExit(f"{label}: test execution emitted an unexpected Action {action!r}")
        test_name = event.get("Test")
        if test_name is None:
            if action in {"run", "fail", "skip"}:
                raise SystemExit(
                    f"{label}: test execution emitted unexpected top-level {action} event"
                )
            continue
        if not isinstance(test_name, str) or not test_name:
            raise SystemExit(f"{label}: test execution emitted an invalid Test field")
        parent_name = test_name.split("/", 1)[0]
        if parent_name not in expected_names:
            raise SystemExit(
                f"{label}: test execution emitted {action} for unexpected test {test_name!r}"
            )
        if action == "skip":
            raise SystemExit(f"{label}: test execution contained a skipped test")
        if action == "fail":
            raise SystemExit(f"{label}: test execution contained a failed test")
        if "/" in test_name:
            continue
        if action == "run":
            run_names.add(test_name)
        elif action == "pass":
            pass_names.add(test_name)
    missing_run = sorted(expected_names - run_names)
    missing_pass = sorted(expected_names - pass_names)
    if missing_run or missing_pass:
        missing = []
        if missing_run:
            missing.append("run=" + ",".join(missing_run))
        if missing_pass:
            missing.append("pass=" + ",".join(missing_pass))
        raise SystemExit(
            f"{label}: test execution was missing expected event(s): "
            + "; ".join(missing)
        )

run_command = command + ["-json"]
run_result = run_go_child(
    run_command,
    cwd=repo_root,
    env=go_env,
    text=True,
    capture_output=True,
    check=False,
)
if run_result.returncode:
    raise SystemExit(run_result.returncode)
validate_test_stream(run_result.stdout, run_result.stderr)
PY
}

# `go_vet_checked` is a thin prescription adapter to the same Python wrapper:
# it supplies the zero-test-count mode while retaining the shared bounded
# process-group helper, reviewed Go-child environment and source/package/build
# identity gate used by every `go_test_checked` command.
go_vet_checked() {
  if [ "$#" -lt 4 ]; then
    return 2
  fi
  local label="$1"
  local package_id="$2"
  local build_id="$3"
  local checked_helper=go_test_checked
  shift 3
  "$checked_helper" 0 0000000000000000000000000000000000000000000000000000000000000000 \
    "$label" "$package_id" "$build_id" "$@"
}

go_test_checked 9 9aef95c84ffd42ad632040498c628c78072ce94f5cc2f6af8493fcffc233b707 ack-root experiments/g01-scaleset:. default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=45s -run '^(TestSDKACKBoundaries|TestSDKDemandAboveFiftyAndPartialAcquisition|TestSDKRepeatedStatisticsAnd202ReuseLastObservation|TestRecoveryAfterACKCallbackCrash|TestRecoveryMissingLifecycleCallback|TestRecoveryErrorsHoldReservationsAndRedact|TestSDKAcquisitionResponseLossAfterACK|TestSDKCapacityWithdrawalDoesNotFenceInFlightAcquisition|TestSDKHTTPFailuresAndSessionRefresh)$' .
go_test_checked 5 f95a296947af0fb862e8b447d3e27b7ce9f01e726659f5f87393789b62b783c9 ack-livecanary experiments/g01-scaleset:./livecanary default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=45s ./livecanary -run '^(TestSupportedListenerBarriersAndReservation|TestDriverBarriersThroughPinnedSDK|TestForeignIdentityAndUnreviewedWorkNeverACKOrDelete|TestAuditPR25MultiJobAcquisitionMustRefuseBeforeACK|TestNoMessageDoesNotCountAsCompletedBarrier)$'

go_test_checked 24 eae489a7d743c942dca803c9b13cb618dcd87fbd9a53634f022bc5381c7d6436 baseline experiments/g01-scaleset:./livecanary default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=180s ./livecanary -run '^TestBaseline'
go_test_checked 9 b6a378d192ba2c614cd82c20eab7cffcdbbb00115a1bba4da803fc262a1c3d8f admission-livecanary experiments/g01-scaleset:./livecanary default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=180s ./livecanary -run '^(TestAdmission.*|TestAuditPR25DistinctStateDirectoriesMustShareCap)$'
go_test_checked 8 4bce3c998c4f2e6806888de2ea9936c6dd9ea877c72b190d47f4f47e24cdb559 admission-liveworker experiments/g01-scaleset:./liveworker default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=180s ./liveworker -run '^(TestAdmissionDirectoryUsesOSAccountWithoutEnvironmentFallback|TestAdmissionAuthorityChecksTheCurrentClaim|TestWorkerAdmissionCapsIndependentDirectories|TestWorkerAdmissionRetainsSlotAfterOutcomeAndClose|TestAdmissionRejectsCopiedJournalInDifferentDirectory|TestAdmissionSyncFailureMustBeRetried|TestAdmissionRefusesMissingUnsafeOrUnknownRootState|TestAdmissionInitializationLockPrecedesClaimCreation)$'
go_test_checked 1 e7cdff09074beb49efb17b8e68201798140ea6f0e83ad10f3fc096aed98823ce unsupported-liveworker experiments/g01-scaleset:./liveworker osusergo+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -tags=osusergo -count=1 -timeout=180s ./liveworker -run '^TestUnsupportedAccountLookupRefusesBeforeJournal$'
go_test_checked 1 e7cdff09074beb49efb17b8e68201798140ea6f0e83ad10f3fc096aed98823ce unsupported-livecanary experiments/g01-scaleset:./livecanary osusergo+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -tags=osusergo -count=1 -timeout=180s ./livecanary -run '^TestUnsupportedAccountLookupRefusesBeforeJournal$'

go_test_checked 3 0f5a8b0633982f8cc04a541cb396c46736c6eec486622354a6456554c52cfc75 jit-root experiments/g01-scaleset:. default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=45s -run '^(TestSDKJITLookupBeforeCreationDoesNotDiscoverIdentity|TestSDKJITResponseLossWithoutCommitDoesNotDiscoverIdentity|TestSDKJITResponseLossDiscoversIdentityWithoutReissuing)$' .
go_test_checked 2 42eac255f830436a6fba9457bcb1964e8f9f596761fecfdb721c2d34b684f4cb jit-livecanary experiments/g01-scaleset:./livecanary default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=45s ./livecanary -run '^(TestJITLostResponseIsSecretSafeAndNeverReissued|TestDriverBarriersThroughPinnedSDK)$'

go_test_checked 34 fcdcde4fce2efa204fdb352c035329a010f7a41b4b9746bf748b2ebc22b4d330 worker-runtime experiments/g01-scaleset:./liveworker default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=45s ./liveworker -run '^(TestOneWorkerNeverRecreatedAndOnlyJITAddedToEnvironment|TestUncertainStartNeverRetriesAndCannotCleanup|TestEveryRuntimeBoundaryRejectsProfileAndOwnershipMismatch|TestUnverifiedRunnerUpdatePolicyRefusesBeforeRuntime|TestSocketReplacementAfterPreflightCannotReceiveAnyMutation|TestSocketModesAndControllerOwnership|TestSocketPostConnectRecheckClosesBeforeHTTP|TestNoCreateBeforeDurableIntent|TestUnknownCreateNeverRetriesAfterRestart|TestCreationWarningsPreserveKnownIDWithoutAuthorizingStart|TestWorkerPreparationReturnsCanonicalSnapshotAndRejectsPriorEffect|TestUnixInspectRequiresStateFlagsBeforeMutation|TestDockerInspectExact(KnownStatesAndSerializableFacts|StatePresenceAndLegacyRequirements|RejectsMalformedOrAmbiguousBodiesBeforeMutation|NotFoundReportsOnlyTheExactGET|RejectsOtherResponsesAndInvalidTargets|RequiresSupported404Body|CancellationNeverReportsPresenceOrAbsence|RejectsReplacedSocket|EOFCancellationKeepsUnknownOutcome)|TestDockerInspectLegacyCleanupKeepsSignedAndAbsentExitPolicy|TestDockerInspectMapsPreserveCaseSensitiveKeysAndProfile|TestDockerCompletedMutationResponseSurvivesEOFCancellation|TestDockerInspectUnknownOrAbsentStatusCannotAuthorizeMutation|TestUnixRuntimeRejectsWrongIdentityImagesAndUnsupportedLimits|TestUnixRuntimeOneShotCreateStartAndNonForceCleanup|TestUnixRuntimeAmbiguousEffectsNeverRetry|TestChangedDaemonCannotCreate|TestChangedDaemonOrAbsentContainerNeverMeansCleanupComplete|TestUnixTransportRejectsSymlinksAndInheritedTCPDestinations|TestAuditPR28MissingBridgeMustNotStart|TestCleanupRetainsActiveAndUnknownWorkers|TestOwnedTerminalCleanupAndRunningRemovalRace)$'
go_test_checked 4 8a0a576036768baff9a9b7d53b3487ef84b2e95f502b3ca9dd453463f396cade update-policy experiments/g01-scaleset:./livecanary default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=45s ./livecanary -run '^(TestCreateRequestsDisabledRunnerUpdate|TestUnconfirmedUpdateSettingQuarantinesCreate|TestUpdateSettingDriftStopsBeforeSessionOrJIT|TestUpdateSettingDriftDoesNotBlockSafeEmptyCleanup)$'
go_test_checked 2 2614438aa4873bd044d1f0d6247b929403e2225c2e5112c13787bf790af42413 worker-command experiments/g01-scaleset:./cmd/g01-worker g01_worker+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -tags=g01_worker -count=1 -timeout=45s ./cmd/g01-worker -run '^(TestBlockedJITInputHonorsDeadline|TestOfflinePlanAndRefusalDoNotReadSecretsOrEchoInput)$'

go_test_checked 45 2e182d6aeb4c278eddbe272be1693e6bf0143759c23c03348435da59addfde53 reconciliation experiments/g01-scaleset:./livecanary default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=45s ./livecanary -run '^(TestAmbiguousCreateNeverRetriesAfterRestart|TestObserve.*|TestObservationIntentFailureStopsBeforeRead|TestObservationResponseCaptureIsLocalAndRejectsOtherOperations|TestStatistics.*|TestDemandStatisticsAllowControlledProbeButNeverCleanup|TestInvalidOwnedProof.*|TestCanonicalPreparationRecordsNoPhaseOrRemoteIntent|TestCanonicalPreparationRefusesInvalidJournalAndPhase|TestCanonicalPreparationRecoveryAndDriverShareLocalGate|TestJournal.*|TestAuthority.*|TestInventoryStrictPages|TestInventoryMalformedStopsLegacyEffects|TestInventoryTransportRefusalIsBoundedAndSanitized|TestInventoryImpossibleTotalStopsBeforeNextPage|TestRosterActualTLSCompleteObservation|TestRosterPreservesOnlyAcceptedPagePrefix|TestRosterFinalPublicationGuardAfterDigest|TestRosterRefusesInvalidEntryWithoutNetwork|TestStrictJSONRejectsDecoderEquivalentDuplicateFields|TestStrictJSONRejectsDuplicateAuthorityFields|TestFailedDirectorySyncMustBeRetried|TestFileJournalRejectsUnrecordedAuthorityBeforeRawDriverEffect|TestRenewedRecoveryApprovalRetainsOwnedState)$'
go_test_checked 9 47a97f1f9083abbe72dfc21a2477bd25580684506fbf71bf5aba9e7f6f1a40e8 reconciliation-worker experiments/g01-scaleset:./liveworker default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=45s ./liveworker -run '^(TestPrivateJournalLocksAndRetainsReservationAcrossRestart|TestJournalRejectsChangedApprovalTornTailAndUnsafeFiles|TestAuthorityLeaseRefusesConcurrentRunsAndFencesClose|TestAuthorityRejectsReplacedJournalOrDirectory|TestStrictJSONRejectsDecoderEquivalentDuplicateFields|TestStrictInputRejectsAmbiguousOrExtraAuthorityFields|TestFailedDirectorySyncMustBeRetried|TestFileJournalRejectsUnrecordedAuthorityBeforeRawDriverEffect|TestRenewedRecoveryApprovalRetainsOwnedState)$'
go_test_checked 11 84b872130057cf2103feaa8f7c6cdb69a4d779e135eebf11cc04cb00590eacb9 reconciliation-quarantine experiments/g01-scaleset:./livecanary default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=45s ./livecanary -run '^(TestDemandStatisticsAllowControlledProbeButNeverCleanup|TestCleanupOnlyForNeverIssuedWorkerWithExactReceipt|TestZeroStatisticsAndOptionalAbsencePermitEmptyCleanup|TestUnsafeStatisticsStopNewEffectsBeforeControlledMessage|TestEmptyAvailableWithWorkStatisticsStaysQuarantined|TestOlderPendingIntentSurvivesSuccessfulZeroInspection|TestUnownedDiscoveryEvidenceCannotAuthorizeCreationAfterAbsence|TestAuditPR25ObservedJobsMustBlockCleanup|TestUnexpectedWorkMessageQuarantinesBeforeSafeClose|TestObservedRunnerSurvivesLaterAbsenceAndFirstCleanup|TestObservationResultFailureSurvivesFileReopenAndInspection)$'

go_test_checked 1 be0f21a92ce13d75e85a1182706e84b84884a8cf0ec01172e14e5ac2e04b600f secret-root experiments/g01-scaleset:. default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=45s -run '^TestSDKBusyRemovalSentinelAndRawErrorExposure$' .
go_test_checked 6 6d0b3fc255928a6c7a5f7ae087d25715f96c0bad9f9298b5fb08ea7bc5fc34df secret-livecanary experiments/g01-scaleset:./livecanary default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=45s ./livecanary -run '^(TestHTTPErrorsDoNotReturnSecretResponseBody|TestSDKHTTPDebugDoesNotLogCredentials|TestSharedTransportRejectsOversizeSuccessAndErrorBodies|TestResponseReaderConsumesOnlyBudgetPlusOneAndRejectsTruncation|TestResponseBudgetAppliesAfterGzipDecompression|TestRealJournalCreateFailureBlocksRetryAndContainsNoErrorBody)$'
go_test_checked 3 feda1bf98e323aef3e209cc4ea2765131ca7d17d70dfd7bb4828c0faab14dff4 secret-liveworker experiments/g01-scaleset:./liveworker default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=45s ./liveworker -run '^(TestUncertainStartNeverRetriesAndCannotCleanup|TestUnixResponsesAreBoundedAndRedirectsNeverFollowed|TestObservationDoesNotJournalRawRuntimeStatus)$'

go_test_checked 10 7ced37498c6790e0a1276ffece4cf0bcb2cad09f39da3ebdd0e28694f752596c live-command experiments/g01-scaleset:./cmd/g01-live g01_live+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -tags=g01_live -count=1 -timeout=45s ./cmd/g01-live -run '^(TestPlanAndRefusalsNeverReadCredentialsOrEchoInputs|TestPreparationCommandNeverReadsCredentialsOrRunsRemotePhase|TestPairedTerminalModeReadsControllerInputAfterAllGates|TestPairedTerminalModeRejectsUnusedPhaseAndControllerFlagsBeforeInput|TestPairedTerminalModeRequiresWorkflowVerificationAuthorityBeforeInput|TestInheritedNamedCredentialFIFODelayedEOF|TestInheritedCredentialPipeStopsAtDeadline|TestCredentialInputRejectsNonPipeDescriptor|TestBlockedCredentialPipeStopsAtDeadline|TestCredentialInputAcceptsCompleteAndRejectsOversize)$'
go_test_checked 4 a751c8b1a6a22d2f4ce76d90d6d79c48ab377355deed0f3bf189496c22b5492e live-transport experiments/g01-scaleset:./livecanary g01_live+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -tags=g01_live -count=1 -timeout=45s ./livecanary -run '^(TestAuthoritySplitAndPolicyRejection|TestSDKTransportOwnership|TestCredentialAttestationMismatchAndExpiredTokenRejected|TestTransportRejectsPlaintextOffHostAndProxyBeforeNetwork)$'

go_test_checked 34 a79b7fa367d8eb1e7fe4ee4ef637696518946dab6f2f25410f6e04bfba137298 drain-pr72 experiments/g01-scaleset:./livecanary default+norace+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset ./livecanary -run '^(TestBaselineAcquireTargetIsActionsOnly|TestPinnedSDKDrain.*|TestDriverDrainThroughPinnedSDKAndPollHook|TestDrainListenerWithdrawsWhilePollResponseIsHeld|TestDrainCancellationStopsBeforeReleasingHeldResponse)$' -count=1 -v -timeout=180s
go_test_checked 34 a79b7fa367d8eb1e7fe4ee4ef637696518946dab6f2f25410f6e04bfba137298 drain-pr72-race experiments/g01-scaleset:./livecanary default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race ./livecanary -run '^(TestBaselineAcquireTargetIsActionsOnly|TestPinnedSDKDrain.*|TestDriverDrainThroughPinnedSDKAndPollHook|TestDrainListenerWithdrawsWhilePollResponseIsHeld|TestDrainCancellationStopsBeforeReleasingHeldResponse)$' -count=1 -v -timeout=180s

terminal_heavy_tests='^TestPairedTerminal(FinalResultCapacity|PendingChildCapacity|EligibilityUsesFreshExactFacts|CapturedAcknowledgementCancellation|MissingAcknowledgementsAndPostchecks)$'
terminal_remainder_skip='^TestPairedTerminal(FinalResultCapacity|PendingChildCapacity|EligibilityUsesFreshExactFacts|CapturedAcknowledgementCancellation|MissingAcknowledgementsAndPostchecks|Actual(Controller|Worker)SyncFailures|PostIntent(JournalIdentity|AuthorityBoundaries)|ClosedReplayActualFile|WorkerReceiptSurvivesControllerWriteFailure|FixtureStorageFailure)$'
terminal_storage_tests='^TestPairedTerminal(Actual(Controller|Worker)SyncFailures|PostIntent(JournalIdentity|AuthorityBoundaries)|ClosedReplayActualFile|WorkerReceiptSurvivesControllerWriteFailure|FixtureStorageFailure)$'
go_test_checked 22 f3efc29451112b27d2cb590763612790db48f05b54718b1e023196ead6625222 paired-collection experiments/g01-scaleset:./livecanary g01_pair_fixture+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -tags=g01_pair_fixture -race -count=1 -timeout=120s ./livecanary -run '^TestPaired' -skip '^TestPairedTerminal'
go_test_checked 106 bdeef850cfe820297e3896477219f0fcf90046bc30fb0bd454bfd0f8fe6db2ff paired-all-except experiments/g01-scaleset:./livecanary g01_pair_fixture+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -tags=g01_pair_fixture -race -count=1 -timeout=120s ./livecanary -skip '^TestPaired'
go_test_checked 24 b7f253f33da157adba67fe4312eddb0ee90172e24f46d4f48502890ab5f080f5 paired-worker experiments/g01-scaleset:./liveworker g01_pair_fixture+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -tags=g01_pair_fixture -race -count=1 -timeout=120s ./liveworker -run '^TestPaired'
go_test_checked 5 9e1a2e099c0fc48bbbd83dd0dc798d3c954910cd150b210037741b2c70211952 paired-heavy experiments/g01-scaleset:./livecanary g01_pair_fixture+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -tags=g01_pair_fixture -race -count=1 -timeout=120s ./livecanary -run "$terminal_heavy_tests"
go_test_checked 15 5e5dd1ee80d3d303271ab17b17aed22d5084e15684bcbe9f31152c1092ec1f73 paired-remainder experiments/g01-scaleset:./livecanary g01_pair_fixture+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -tags=g01_pair_fixture -race -count=1 -timeout=120s ./livecanary -run '^TestPairedTerminal' -skip "$terminal_remainder_skip"
go_test_checked 6 458f77f55209a59338a63bfc27697d85ebe5e0c3c7d1b959a0b56b2527f3ead5 paired-storage experiments/g01-scaleset:./livecanary g01_pair_fixture+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -tags=g01_pair_fixture -race -count=1 -timeout=120s ./livecanary -run "$terminal_storage_tests"
go_vet_checked vet-livecanary experiments/g01-scaleset:./livecanary g01_pair_fixture+norace+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go vet -C experiments/g01-scaleset -tags=g01_pair_fixture ./livecanary
go_vet_checked vet-liveworker experiments/g01-scaleset:./liveworker g01_pair_fixture+norace+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go vet -C experiments/g01-scaleset -tags=g01_pair_fixture ./liveworker
```

The packet also audits every future shell `go test` selector prescription
containing either spelling of `-run`/`-skip` (`-run REGEXP` or `-run=REGEXP`, and
the corresponding `-skip` forms), including Go's equivalent double-dash
`--run`/`--skip` spellings, whether or not the command starts with a
`GOTOOLCHAIN=` assignment. Standard `env NAME=VALUE ... go test` and shell
`command [options] go test` prefixes are discovered as well; the wrapper rejects
them explicitly, so they cannot become an unguarded alternative syntax. The
guard audit joins shell backslash continuations, discovers all selector-bearing
logical commands first, and treats an unguarded continued command as an error
rather than relying on a visual review of the selector block. The package/build
metadata audit below is a separate gate: every guarded prescription must still
carry explicit `GOTOOLCHAIN`, tag and race metadata.

```sh
set -euo pipefail
selector_pattern='^[[:space:]]*(?:(?:[A-Za-z_][A-Za-z0-9_]*=[^[:space:]]+)[[:space:]]+)*(?:env[[:space:]]+.*[[:space:]]+)?(?:command(?:[[:space:]]+-[^[:space:]]+)*[[:space:]]+)?go test .*[[:space:]]--?(run|skip)(=|[[:space:]])'
rg -n "$selector_pattern" docs/evidence/g01-recovery-packet.md >/dev/null
selector_probe=$'GOTOOLCHAIN=go1.26.8 go test ./livecanary -run=^TestProbe$\nenv GOTOOLCHAIN=go1.26.8 go test ./livecanary --run=^TestProbe$\nenv -i GOTOOLCHAIN=go1.26.8 go test ./livecanary --skip ^TestProbe$\n  GOTOOLCHAIN=go1.26.8 go test ./livecanary -skip ^TestProbe$\ncommand go test ./livecanary -run ^TestCommandProbe$\nGOTOOLCHAIN=go1.26.8 command go test ./livecanary -skip=^TestCommandProbe$'
rg -n "$selector_pattern" <<< "$selector_probe" | wc -l | tr -d ' ' | grep -Fxq 6
[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || { printf '%s\n' 'reviewed canonical PATH and absolute Python interpreter required' >&2; exit 1; }
/opt/homebrew/bin/python3 -I - <<'PY'
from pathlib import Path
import re
import tempfile

lines = Path("docs/evidence/g01-recovery-packet.md").read_text(encoding="utf-8").splitlines()
selector_command = re.compile(
    r"^\s*(?:(?:[A-Za-z_][A-Za-z0-9_]*=[^\s]+)\s+)*"
    r"(?:env\s+.*\s+)?(?:command(?:\s+-[^\s]+)*\s+)?go test\b.*\s--?(?:run|skip)(?:=|\s)"
)
logical_selector_command = re.compile(
    r"^\s*(?:go_test_checked\b.*\bgo test\b|"
    r"(?:(?:[A-Za-z_][A-Za-z0-9_]*=[^\s]+)\s+)*"
    r"(?:env\s+.*\s+)?(?:command(?:\s+-[^\s]+)*\s+)?go test\b)"
    r".*\s--?(?:run|skip)(?:=|\s)",
    re.DOTALL,
)


def logical_commands(source_lines):
    """Join shell backslash continuations and retain their first line number."""
    commands = []
    index = 0
    while index < len(source_lines):
        first = index
        command = source_lines[index].rstrip()
        while command.endswith("\\"):
            command = command[:-1].rstrip() + " "
            index += 1
            if index >= len(source_lines):
                raise SystemExit(f"unterminated shell continuation at line {first + 1}")
            command += source_lines[index].lstrip()
        commands.append((first, command))
        index += 1
    return commands


def audit_logical_selectors(source_lines):
    guarded_count = 0
    for first, command in logical_commands(source_lines):
        if command.lstrip().startswith("selector_probe=$'"):
            continue
        if not logical_selector_command.search(command):
            continue
        if not command.lstrip().startswith("go_test_checked "):
            raise SystemExit(f"unguarded future selector at line {first + 1}")
        guarded_count += 1
    if guarded_count == 0:
        raise SystemExit("no future go test selector prescriptions found")
    return guarded_count


logical_guarded = audit_logical_selectors(lines)
selector_fixture_lines = [
    line for line in lines if line.lstrip().startswith("selector_probe=$'")
]
if len(selector_fixture_lines) != 1:
    raise SystemExit(
        f"expected one explicit non-prescription selector fixture, observed {len(selector_fixture_lines)}"
    )
wrapper_records = sum(
    line.lstrip().startswith("go_test_checked ") for line in lines
)
if logical_guarded != wrapper_records:
    raise SystemExit(
        f"logical selector count {logical_guarded} does not equal wrapper record count {wrapper_records}"
    )
continuation_probes = [
    [
        "go test ./livecanary \\",
        "  -run ^TestContinuationProbe$",
    ],
    [
        "env GOTOOLCHAIN=go1.26.8 go test ./livecanary \\",
        "  --skip ^TestContinuationProbe$",
    ],
]
for continuation_probe in continuation_probes:
    try:
        audit_logical_selectors(continuation_probe)
    except SystemExit as error:
        if "unguarded future selector" not in str(error):
            raise
    else:
        raise SystemExit(
            "line-continuation selector probe: unguarded command was accepted"
        )
print(
    "line-continuation selector probe: passed; unguarded continued single- and "
    "double-dash/env-wrapped selectors were discovered and rejected"
)

env_selector_probes = [
    "env GOTOOLCHAIN=go1.26.8 go test ./livecanary --run ^TestEnvProbe$",
    "GOTOOLCHAIN=go1.26.8 env -i GOFLAGS= go test ./livecanary --skip=^TestEnvProbe$",
]
for env_selector_probe in env_selector_probes:
    try:
        audit_logical_selectors([env_selector_probe])
    except SystemExit as error:
        if "unguarded future selector" not in str(error):
            raise
    else:
        raise SystemExit("env-wrapped selector probe: unguarded command was accepted")
print(
    "env-wrapped selector probe: passed; standard env command prefixes were "
    "discovered and rejected when unguarded"
)

assignment_selector_probes = [
    "GOTOOLCHAIN=go1.26.8 go test ./livecanary -run ^TestAssignmentProbe$",
    "GOTOOLCHAIN=go1.26.8 go test ./livecanary -skip=^TestAssignmentProbe$",
]
for assignment_selector_probe in assignment_selector_probes:
    try:
        audit_logical_selectors([assignment_selector_probe])
    except SystemExit as error:
        if "unguarded future selector" not in str(error):
            raise
    else:
        raise SystemExit(
            "assignment-prefixed selector probe: unguarded command was accepted"
        )
print(
    "assignment-prefixed selector probe: passed; standard GOTOOLCHAIN=... "
    "go test selector forms were discovered and rejected when unguarded"
)

command_selector_probes = [
    "command go test ./livecanary -run ^TestCommandProbe$",
    "GOTOOLCHAIN=go1.26.8 command go test ./livecanary -skip=^TestCommandProbe$",
    "command -p go test ./livecanary --run ^TestCommandProbe$",
]
for command_selector_probe in command_selector_probes:
    try:
        audit_logical_selectors([command_selector_probe])
    except SystemExit as error:
        if "unguarded future selector" not in str(error):
            raise
    else:
        raise SystemExit(
            "command-prefix selector probe: unguarded command was accepted"
        )
print(
    "command-prefix selector probe: passed; bare, assignment-prefixed and "
    "option-bearing shell command go test selector forms were discovered and "
    "rejected when unguarded"
)

guarded = 0
for index, line in enumerate(lines):
    if line.lstrip().startswith("selector_probe=$'"):
        continue
    if not selector_command.search(line):
        continue
    if index == 0 or not lines[index - 1].lstrip().startswith("go_test_checked "):
        raise SystemExit(f"unguarded future selector at line {index + 1}")
    guarded += 1
if guarded == 0:
    raise SystemExit("no future go test selector prescriptions found")
with tempfile.NamedTemporaryFile("w+", encoding="utf-8", suffix=".md") as probe:
    probe.write(
        Path("docs/evidence/g01-recovery-packet.md").read_text(encoding="utf-8")
        + "\ngo test ./livecanary -run ^TestZeroIndentProbe$\n"
        + "GOTOOLCHAIN=go1.26.8 go test ./livecanary -run ^TestAssignmentProbe$\n"
        + "env GOTOOLCHAIN=go1.26.8 go test ./livecanary --skip ^TestZeroIndentProbe$\n"
        + "command go test ./livecanary -run ^TestCommandProbe$\n"
        + "GOTOOLCHAIN=go1.26.8 command go test ./livecanary -skip=^TestCommandProbe$\n"
    )
    probe.flush()
    try:
        probe_lines = Path(probe.name).read_text(encoding="utf-8").splitlines()
        for index, line in enumerate(probe_lines):
            if not selector_command.search(line):
                continue
            if index == 0 or not probe_lines[index - 1].lstrip().startswith("go_test_checked "):
                raise SystemExit(f"unguarded future selector at line {index + 1}")
    except SystemExit as error:
        if "unguarded future selector" not in str(error):
            raise
        print(
            "zero-indent selector probe: passed; an unguarded column-zero "
            "`go test -run` without GOTOOLCHAIN, assignment, env or command "
            "prefix was discovered and rejected"
        )
    else:
        raise SystemExit("zero-indent selector probe: unguarded command was accepted")

# Audit the future vet prescriptions in their own shell-fence block. Both
# logical commands must use the thin adapter, which delegates to the same
# bounded Go-child/environment/source-identity wrapper as the test records.
vet_start = next(
    index for index, line in enumerate(lines)
    if line.startswith("go_vet_checked vet-livecanary ")
)
vet_end = next(
    index for index in range(vet_start, len(lines))
    if lines[index] == "```"
)
vet_commands = [
    command for _, command in logical_commands(lines[vet_start:vet_end])
    if re.search(r"\bgo vet\b", command)
]
if len(vet_commands) != 2:
    raise SystemExit(f"expected two future go vet prescriptions, observed {len(vet_commands)}")
if any(not command.lstrip().startswith("go_vet_checked ") for command in vet_commands):
    raise SystemExit("an executable go vet prescription bypassed go_vet_checked")
if any(
    "g01_pair_fixture+norace+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+"
    not in command
    or "-tags=g01_pair_fixture" not in command
    or "+goroot-default+gofips140-off+go1.26.8" not in command
    for command in vet_commands
):
    raise SystemExit("a go vet prescription lacks the reviewed tagged nine-part build identity")
print(
    "go vet prescription audit: passed; 2/2 future go vet commands use "
    "go_vet_checked with the g01_pair_fixture build identity and the shared "
    "bounded Go-child/environment/source-identity wrapper"
)
print(
    f"future selector guard audit: passed; {guarded} go test selector "
    "prescriptions are wrapper-guarded and equal the logical wrapper count; "
    "column-zero, assignment-prefixed, env-wrapped, command-prefixed, "
    "double-dash and indented discovery forms recognized"
)
PY
```

The selector guard audit exited 0 and found 28 future `go test` selector
prescriptions containing `-run` or `-skip`, each immediately preceded by
`go_test_checked`; the focused zero-indent, assignment-prefixed, env-wrapped,
command-prefixed and continued-command probes discovered and rejected
unguarded selectors without relying on line layout, while the explicit
read-only samples cover separated and equals spellings, both dash widths,
standard `env` and shell `command` prefixes and both zero-indent and indented
shell forms. No test body was run by this grep/audit. For the paired
partitions, the wrapper validates
the filtered executed sets: collection 22/22, all-except 106/106, worker 24/24,
heavy 5/5, terminal remainder 15/15, and storage 6/6; their recorded
SHA-256 values are the sorted filtered-name digests on the corresponding
prescription lines above.

The recorded selector-audit output was:

```text
line-continuation selector probe: passed; unguarded continued single- and double-dash/env-wrapped selectors were discovered and rejected
env-wrapped selector probe: passed; standard env command prefixes were discovered and rejected when unguarded
assignment-prefixed selector probe: passed; standard GOTOOLCHAIN=... go test selector forms were discovered and rejected when unguarded
command-prefix selector probe: passed; bare, assignment-prefixed and option-bearing shell command go test selector forms were discovered and rejected when unguarded
zero-indent selector probe: passed; an unguarded column-zero `go test -run` without GOTOOLCHAIN was discovered and rejected
go vet prescription audit: passed; 2/2 future go vet commands use go_vet_checked and the shared bounded Go-child/environment/source-identity wrapper
future selector guard audit: passed; 28 go test selector prescriptions equal the logical wrapper count; column-zero, assignment-prefixed, env-wrapped, command-prefixed, double-dash and indented discovery forms recognized
```

The selector audit is complemented by an exhaustive executable-command audit:
it discovers every `go test` executable in shell fences, including commands
without `-run` or `-skip`, and requires each prescription to be a
`go_test_checked` invocation. Python heredocs, prose, comments and synthetic
non-prescription fixtures are explicitly outside the executable set; a future
fixture must carry its own `non-prescription fixture` marker or the audit
fails closed. This prevents an unfiltered test command from bypassing the
source-derived count/digest, package/build identity, environment and JSON
stream guards.

```sh
set -euo pipefail
# g01-safe-python-heredoc: reviewed synthetic child argv
[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || { printf '%s\n' 'reviewed canonical PATH and absolute Python interpreter required' >&2; exit 1; }
/opt/homebrew/bin/python3 -I - <<'PY'
import re
import shlex
from pathlib import Path

source = Path("docs/evidence/g01-recovery-packet.md").read_text(encoding="utf-8")
fence_languages = {"sh", "bash", "shell", "zsh"}
heredoc = re.compile(r"\bpython3\s+-I\b[^\n]*<<-?\s*(['\"]?)([A-Za-z_][A-Za-z0-9_]*)\1")


def executable_shell_commands(markdown):
    in_shell = False
    skip_until = None
    pending = []
    pending_numbers = []
    for number, line in enumerate(markdown.splitlines(), start=1):
        if line.startswith("```"):
            info = line[3:].strip().lower()
            if in_shell:
                if not info:
                    in_shell = False
                    pending = []
                    pending_numbers = []
                continue
            if info in fence_languages:
                in_shell = True
            continue
        if not in_shell:
            continue
        stripped = line.strip()
        if skip_until is not None:
            if stripped == skip_until:
                skip_until = None
            continue
        heredoc_match = heredoc.search(stripped)
        if heredoc_match:
            skip_until = heredoc_match.group(2)
            continue
        if not stripped or stripped.startswith("#"):
            continue
        pending.append(stripped[:-1].rstrip() if stripped.endswith("\\") else stripped)
        pending_numbers.append(number)
        if stripped.endswith("\\"):
            continue
        yield " ".join(pending), pending_numbers[0]
        pending = []
        pending_numbers = []


def strip_shell_quoted(command):
    """Remove quoted shell payloads before raw executable-token discovery."""
    output = []
    quote = None
    escaped = False
    for character in command:
        if escaped:
            output.append(" ")
            escaped = False
            continue
        if quote is not None:
            if character == "\\" and quote == '"':
                escaped = True
            elif character == quote:
                quote = None
            output.append(" ")
            continue
        if character in {"'", '"'}:
            quote = character
            output.append(" ")
        else:
            output.append(character)
    return "".join(output)


def shell_token_segments(command):
    try:
        lexer = shlex.shlex(command, posix=True, punctuation_chars=";&|")
        lexer.whitespace_split = True
        tokens = list(lexer)
    except ValueError:
        return []
    segments = [[]]
    for token in tokens:
        if token in {";", "&&", "||", "|", "&"}:
            segments.append([])
        else:
            segments[-1].append(token)
    return [segment for segment in segments if segment]


def executable_basename(token):
    return token.rsplit("/", 1)[-1]


shell_executables = {
    "sh", "bash", "dash", "zsh", "ksh", "mksh", "ash", "fish", "csh", "tcsh",
}


def shell_command_payload(tokens):
    if not tokens:
        return None
    shell_index = 0
    executable = executable_basename(tokens[0])
    if executable == "busybox":
        if len(tokens) < 2 or executable_basename(tokens[1]) not in shell_executables:
            return None
        shell_index = 1
        executable = executable_basename(tokens[shell_index])
    if executable not in shell_executables:
        return None
    index = shell_index + 1
    while index < len(tokens):
        option = tokens[index]
        if option in {"-c", "--command"}:
            return tokens[index + 1] if index + 1 < len(tokens) else ""
        if option.startswith("--command=") or option.startswith("-c="):
            return option.split("=", 1)[1]
        if option.startswith("-") and not option.startswith("--") and "c" in option[1:]:
            return tokens[index + 1] if index + 1 < len(tokens) else ""
        if option.startswith("-"):
            index += 1
            continue
        break
    return None


def executable_go_tests(tokens, depth=0):
    found = [
        index for index in range(len(tokens) - 1)
        if executable_basename(tokens[index]) == "go" and tokens[index + 1] == "test"
    ]
    if depth >= 8:
        return found
    payload = shell_command_payload(tokens)
    if payload is None:
        return found
    for segment in shell_token_segments(payload):
        found.extend(executable_go_tests(segment, depth + 1))
    return found


def audit_executable_go_tests(markdown):
    guarded = []
    fixtures = []
    for command, number in executable_shell_commands(markdown):
        try:
            tokens = shlex.split(command, posix=True)
        except ValueError as error:
            raw = strip_shell_quoted(command)
            if re.search(r"\bgo\s+test\b", raw):
                raise SystemExit(f"line {number}: shell command is not parseable: {error}")
            continue
        go_tests = executable_go_tests(tokens)
        if not go_tests:
            continue
        if len(go_tests) != 1:
            raise SystemExit(f"line {number}: multiple executable go test commands")
        if tokens[0] == "go_test_checked":
            guarded.append((number, command))
            continue
        if tokens[0] == "non-prescription" and len(tokens) > 1 and tokens[1] == "fixture":
            fixtures.append((number, command))
            continue
        raise SystemExit(f"line {number}: unguarded executable go test prescription")
    if not guarded:
        raise SystemExit("no executable go test prescriptions found")
    return guarded, fixtures


guarded, fixtures = audit_executable_go_tests(source)
if len(guarded) != 28:
    raise SystemExit(f"expected 28 guarded executable go test prescriptions, observed {len(guarded)}")
if fixtures:
    raise SystemExit(
        "non-prescription executable go test fixtures require explicit review: "
        + repr(fixtures)
    )

try:
    audit_executable_go_tests("```sh\ngo test ./livecanary\n```")
except SystemExit as error:
    if "unguarded executable go test prescription" not in str(error):
        raise
else:
    raise SystemExit("unfiltered executable go test fixture was accepted")
try:
    audit_executable_go_tests("```sh\nbash -c 'go test ./livecanary'\n```")
except SystemExit as error:
    if "unguarded executable go test prescription" not in str(error):
        raise
else:
    raise SystemExit("nested unfiltered executable go test fixture was accepted")
print(
    f"all-executable-go-test audit: passed; discovered {len(guarded)} executable go test prescriptions, "
    "all guarded by go_test_checked; 0 non-prescription executable fixtures; "
    "direct and nested unfiltered synthetic go test rejected before execution"
)
PY
```

The all-executable-go-test audit exited 0: it discovered all 28 executable
prescriptions, including any future unfiltered or nested form, found no
unclassified fixture and rejected direct and nested synthetic `go test` forms
before execution.
The audit is static only; it started no Go child, test body, `go test -list`,
live operation or credential-bearing process.

Recorded exhaustive-command output:

```text
all-executable-go-test audit: passed; discovered 28 executable go test prescriptions, all guarded by go_test_checked; 0 non-prescription executable fixtures; direct and nested unfiltered synthetic go test rejected before execution
```

The wrapper metadata is independently checked against each command's literal
package and build flags so a copied digest cannot target the other package or a
different tag/race configuration:

```sh
set -euo pipefail
# g01-safe-python-heredoc: reviewed synthetic Go metadata argv
[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || { printf '%s\n' 'reviewed canonical PATH and absolute Python interpreter required' >&2; exit 1; }
/opt/homebrew/bin/python3 -I - <<'PY'
import re
import shlex
from pathlib import Path

lines = Path("docs/evidence/g01-recovery-packet.md").read_text(encoding="utf-8").splitlines()
seen = []
pr78_module_tree = "08c7830de7bc5120d1302d7ba6df162abd582315"
pr72_module_tree = "9b30ef1b69c6375cb264c759d366fc5a52a5439f"
drain_source_pins = {
    "drain-pr72": pr72_module_tree,
    "drain-pr72-race": pr72_module_tree,
}
for index, line in enumerate(lines):
    fields = line.split()
    if (
        len(fields) != 7
        or fields[0] != "go_test_checked"
        or fields[-1] != "\\"
        or not re.fullmatch(r"[0-9]+", fields[1])
        or not re.fullmatch(r"[0-9a-f]{64}", fields[2])
    ):
        continue
    if index + 1 >= len(lines):
        raise SystemExit(f"line {index + 1}: missing wrapped command")
    command = shlex.split(lines[index + 1].strip())
    env = {}
    while command and re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*=.*", command[0]):
        key, value = command.pop(0).split("=", 1)
        env[key] = value
    git_repository_control_names = {
        "GIT_DIR",
        "GIT_WORK_TREE",
        "GIT_INDEX_FILE",
        "GIT_INDEX_VERSION",
        "GIT_COMMON_DIR",
        "GIT_OBJECT_DIRECTORY",
        "GIT_OBJECT_DIRECTORY_RELATIVE",
        "GIT_ALTERNATE_OBJECT_DIRECTORIES",
        "GIT_NAMESPACE",
        "GIT_CEILING_DIRECTORIES",
        "GIT_DISCOVERY_ACROSS_FILESYSTEM",
        "GIT_CONFIG",
        "GIT_CONFIG_GLOBAL",
        "GIT_CONFIG_SYSTEM",
        "GIT_CONFIG_NOSYSTEM",
        "GIT_CONFIG_COUNT",
        "GIT_CONFIG_PARAMETERS",
        "GIT_LITERAL_PATHSPECS",
        "GIT_GLOB_PATHSPECS",
        "GIT_NOGLOB_PATHSPECS",
        "GIT_OPTIONAL_LOCKS",
        "GIT_REPLACE_REF_BASE",
        "GIT_NO_REPLACE_OBJECTS",
        "GIT_ATTR_NOSYSTEM",
        "GIT_QUARANTINE_PATH",
    }
    git_repository_control_prefixes = ("GIT_CONFIG_KEY_", "GIT_CONFIG_VALUE_")
    git_environment_overrides = sorted(
        key
        for key in env
        if key in git_repository_control_names
        or any(key.startswith(prefix) for prefix in git_repository_control_prefixes)
    )
    if git_environment_overrides:
        raise SystemExit(
            f"line {index + 2}: Git repository-control environment overrides are not allowed: "
            + ", ".join(git_environment_overrides)
        )
    loader_assignment_names = {
        "PATH",
        "LD_PRELOAD",
        "LD_PRELOAD_32",
        "LD_PRELOAD_64",
        "LD_LIBRARY_PATH",
        "LD_LIBRARY_PATH_32",
        "LD_LIBRARY_PATH_64",
        "LD_AUDIT",
        "DYLD_INSERT_LIBRARIES",
        "DYLD_LIBRARY_PATH",
        "DYLD_FALLBACK_LIBRARY_PATH",
        "DYLD_FRAMEWORK_PATH",
        "DYLD_FALLBACK_FRAMEWORK_PATH",
        "DYLD_ROOT_PATH",
    }
    unsafe_loader_assignments = sorted(
        key for key in env if key in loader_assignment_names
    )
    if unsafe_loader_assignments:
        raise SystemExit(
            f"line {index + 2}: executable-loader environment assignments are not allowed: "
            + ", ".join(unsafe_loader_assignments)
        )
    compiler_tool_environment_names = {
        "CC", "CXX", "GCCGO", "GOGCCFLAGS", "GO_EXTLINK_ENABLED", "GO_LDSO",
        "CC_FOR_TARGET", "CXX_FOR_TARGET", "PKG_CONFIG", "PKG_CONFIG_PATH",
        "PKG_CONFIG_LIBDIR", "PKG_CONFIG_SYSROOT_DIR", "LIBRARY_PATH",
        "C_INCLUDE_PATH", "CPLUS_INCLUDE_PATH", "OBJC_INCLUDE_PATH", "CPATH",
        "SDKROOT", "MACOSX_DEPLOYMENT_TARGET",
    }
    compiler_tool_environment_overrides = sorted(
        key
        for key in env
        if key in compiler_tool_environment_names
        or (key.startswith("CGO_") and key != "CGO_ENABLED")
    )
    if compiler_tool_environment_overrides:
        raise SystemExit(
            f"line {index + 2}: compiler/cgo tool environment assignments are not allowed: "
            + ", ".join(compiler_tool_environment_overrides)
        )
    if command and command[0] == "env":
        raise SystemExit(
            f"line {index + 2}: standard env-wrapped go test command is not allowed"
        )
    if command and command[0] == "command":
        raise SystemExit(
            f"line {index + 2}: shell command-prefix go test command is not allowed"
        )
    if command[:2] != ["go", "test"]:
        raise SystemExit(f"line {index + 2}: not a go test command")
    args = command[2:]
    if any(value.startswith("--") for value in args):
        raise SystemExit(
            f"line {index + 2}: equivalent double-dash Go flags are not allowed"
        )
    if any(value == "-toolexec" or value.startswith("-toolexec=") for value in args):
        raise SystemExit(
            f"line {index + 2}: -toolexec execution hooks are not allowed"
        )
    if not env.get("GOTOOLCHAIN", "").strip():
        raise SystemExit(f"line {index + 2}: missing explicit GOTOOLCHAIN metadata")

    def values(flag):
        result = []
        cursor = 0
        while cursor < len(args):
            value = args[cursor]
            if value == flag:
                if cursor + 1 >= len(args):
                    raise SystemExit(f"line {index + 2}: {flag} requires a value")
                result.append(args[cursor + 1])
                cursor += 2
            elif value.startswith(flag + "="):
                result.append(value[len(flag) + 1:])
                cursor += 1
            else:
                cursor += 1
        return result

    modules = values("-C")
    value_flags = {"-C", "-tags", "-run", "-skip", "-list", "-count", "-timeout"}
    boolean_flags = {"-race", "-v"}
    packages = []
    cursor = 0
    while cursor < len(args):
        value = args[cursor]
        if value in value_flags:
            if cursor + 1 >= len(args):
                raise SystemExit(f"line {index + 2}: {value} requires a value")
            cursor += 2
            continue
        if any(value.startswith(flag + "=") for flag in value_flags):
            cursor += 1
            continue
        if value in boolean_flags or any(
            value.startswith(flag + "=") for flag in boolean_flags
        ):
            cursor += 1
            continue
        if value.startswith("-"):
            raise SystemExit(
                f"line {index + 2}: unsupported or unreviewed Go/test flag "
                f"{value.split('=', 1)[0]}"
            )
        packages.append(value)
        cursor += 1
    if modules != ["experiments/g01-scaleset"] or len(packages) != 1:
        raise SystemExit(f"line {index + 2}: package/module identity is ambiguous")
    actual_package = f"{modules[0]}:{packages[0]}"
    tags = values("-tags")
    if len(tags) > 1:
        raise SystemExit(f"line {index + 2}: duplicate -tags flag")
    tag_identity = "default" if not tags else ",".join(sorted(tags[0].split(",")))
    race_flags = [value for value in args if value == "-race" or value.startswith("-race=")]
    if len(race_flags) > 1:
        raise SystemExit(f"line {index + 2}: duplicate -race flag")
    race_identity = "race" if race_flags and race_flags[0] != "-race=false" else "norace"
    counts = values("-count")
    if counts != ["1"]:
        raise SystemExit(f"line {index + 2}: exactly one -count=1 is required")
    timeouts = values("-timeout")
    if len(timeouts) != 1:
        raise SystemExit(f"line {index + 2}: exactly one -timeout is required")
    timeout_match = re.fullmatch(r"([1-9][0-9]*)(ms|s|m)", timeouts[0])
    if not timeout_match:
        raise SystemExit(f"line {index + 2}: timeout must be positive and bounded")
    timeout_value, timeout_unit = timeout_match.groups()
    timeout_seconds = int(timeout_value) / {"ms": 1000, "s": 1, "m": 1 / 60}[timeout_unit]
    if timeout_seconds > 300:
        raise SystemExit(f"line {index + 2}: timeout exceeds 300 seconds")
    if any(value == "-args" or value.startswith("-args=") for value in args):
        raise SystemExit(f"line {index + 2}: -args is not allowed")
    test_override_prefixes = ("-test.run", "-test.skip", "-test.count", "-test.timeout", "-test.list")
    if any(
        value == prefix or value.startswith(prefix + "=")
        for value in args
        for prefix in test_override_prefixes
    ):
        raise SystemExit(f"line {index + 2}: test-binary override is not allowed")
    label = fields[3]
    if label.startswith("drain-") and label not in drain_source_pins:
        raise SystemExit(f"{label}: unknown drain source pin")
    expected_source_tree = drain_source_pins.get(label, pr78_module_tree)
    if env.get("CGO_ENABLED") not in {None, "", "1"}:
        raise SystemExit(f"line {index + 2}: conflicting CGO_ENABLED assignment")
    if env.get("GOEXPERIMENT") not in {None, "none"}:
        raise SystemExit(f"line {index + 2}: conflicting GOEXPERIMENT assignment")
    if env.get("GOENV") not in {None, "off"}:
        raise SystemExit(f"line {index + 2}: conflicting GOENV assignment")
    if env.get("GOROOT") not in {None, ""}:
        raise SystemExit(f"line {index + 2}: conflicting GOROOT assignment")
    if env.get("GOFIPS140") not in {None, "off"}:
        raise SystemExit(f"line {index + 2}: conflicting GOFIPS140 assignment")
    target_assignments = {
        "GOOS": "darwin",
        "GOARCH": "arm64",
        "GOARM64": "v8.0",
    }
    target_feature_names = {
        "GOOS", "GOARCH", "GOAMD64", "GOARM", "GOARM64", "GO386",
        "GOMIPS", "GOMIPS64", "GOPPC64", "GOWASM",
    }
    if any(
        name in env and (
            name not in target_assignments or env[name] != target_assignments[name]
        )
        for name in target_feature_names
    ):
        raise SystemExit(f"line {index + 2}: unreviewed target assignment")
    actual_build = (
        f"{tag_identity}+{race_identity}+cgo1+cgo-tools-default+"
        f"goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+"
        f"gofips140-off+{env.get('GOTOOLCHAIN', '')}"
    )
    if actual_package != fields[4] or actual_build != fields[5]:
        raise SystemExit(
            f"{fields[3]}: expected {fields[4]}/{fields[5]}, "
            f"observed {actual_package}/{actual_build}"
        )
    if not expected_source_tree:
        raise SystemExit(f"{label}: missing reviewed source tree pin")
    seen.append(label)
if len(seen) != 28 or len(set(seen)) != len(seen):
    raise SystemExit(f"expected 28 unique wrapper metadata records, observed {len(seen)}")
synthetic_git_assignments = [
    "GIT_WORK_TREE=/synthetic/work-tree",
    "GIT_DIR=/synthetic/repository",
    "GIT_INDEX_FILE=/synthetic/index",
    "GIT_CONFIG_KEY_0=core.worktree",
]
for assignment in synthetic_git_assignments:
    key = assignment.split("=", 1)[0]
    if key not in git_repository_control_names and not any(
        key.startswith(prefix) for prefix in git_repository_control_prefixes
    ):
        raise SystemExit(f"synthetic Git assignment escaped repository-control guard: {key}")
print(
    f"package/build metadata audit: passed; {len(seen)} wrapper prescriptions "
    "matched one package/import-path identity and explicit GOTOOLCHAIN/tag/race/CGO/GOEXPERIMENT/target/GOROOT/GOFIPS140 "
    "compiler-tool build configuration, exact PR #78/PR #72 source-tree family pins, exactly "
    "one count/timeout, and no Git repository-control, GOENV/GOROOT/GOFIPS140/target-feature, test-binary, benchmark/CPU, double-dash or "
    "env/command overrides; duplicate package candidates and synthetic Git "
    "repository-control assignments fail closed"
)
PY
```

The package/build metadata audit exited 0 with 28 unique records. Every
package identity matched its `-C` directory and sole package/import-path
argument, every record carried explicit `GOTOOLCHAIN` metadata, exactly one
`-count=1` and one positive timeout no greater than 300 seconds, and every
build identity matched its tag set, race mode, pinned `CGO_ENABLED=1`,
`GOEXPERIMENT=none`, `GOOS=darwin`, `GOARCH=arm64` and `GOARM64=v8.0` modes and
`GOROOT=""`/`goroot-default`, `GOFIPS140=off`/`gofips140-off` modes and
toolchain. The two drain labels selected the exact
PR #72 module tree `9b30ef1b69c6375cb264c759d366fc5a52a5439f`; all other labels
selected the PR #78 module tree `08c7830de7bc5120d1302d7ba6df162abd582315`.
`GIT_*` repository-control assignments, `GOENV` and target-feature assignments,
`-args`, test-binary overrides, `-toolexec` hooks, benchmark/CPU flags, standard
`env` and shell `command` wrappers, every equivalent double-dash Go flag and
duplicate relative or import-path package arguments are rejected before
source-derived selector validation.
The recorded metadata-audit output was:

```text
package/build metadata audit: passed; 28 wrapper prescriptions matched one package/import-path identity, explicit GOTOOLCHAIN/tag/race/CGO/GOEXPERIMENT/target/GOROOT/GOFIPS140 build configuration, exact PR #78/PR #72 source-tree family pins, exactly one count/timeout, and no Git repository-control, GOENV/GOROOT/GOFIPS140/target-feature, test-binary, benchmark/CPU, double-dash or env/command overrides; duplicate package candidates and synthetic Git repository-control assignments fail closed
```

The wrapper's selector edge cases were then exercised with a trimmed copy of
the documented Python body that stops before the original test subprocess. Its
Go child processes are the effective `go env GOFLAGS`, bounded module download/
verification, and package metadata (`go list -json -test`) queries; no
`go test -list` child is allowed because that would start package and imported
initialization paths. Every Go child receives
`GOWORK=off` after inherited and command-prefix environment parsing, so an
external or auto-discovered workspace cannot alter metadata or test selection;
every Go child also receives the reviewed `GOEXPERIMENT=none` binding and an
independent 300-second subprocess deadline covering toolchain selection,
downloads, compilation, metadata and test execution. That helper owns a fresh
session/process group and, on timeout, terminates the group, escalates after a
bounded grace period and reaps the direct child before refusing the result.
Git source status/tree queries are read-only. The probe also
feeds synthetic skipped and output-only/no-`run`/`pass` streams to the
execution-stream guard without starting a test body, and includes a synthetic
inherited-workspace probe that verifies the force-off environment on every
direct Go child:

```sh
set -euo pipefail
# g01-safe-python-heredoc: reviewed isolated interpreter argv
[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || { printf '%s\n' 'reviewed canonical PATH and absolute Python interpreter required' >&2; exit 1; }
/opt/homebrew/bin/python3 -I - <<'PY'
import hashlib
import io
import json
import os
import subprocess
import sys
import tempfile
from contextlib import redirect_stdout
from pathlib import Path

packet = Path("docs/evidence/g01-recovery-packet.md").read_text(encoding="utf-8")
start = packet.index("\nimport hashlib\n", packet.index("go_test_checked()")) + 1
end = packet.index("\nPY\n}", start)
wrapper = packet[start:end]
wrapper = wrapper[:wrapper.index("run_result = run_go_child")]
one_name = "TestNoMessageDoesNotCountAsCompletedBarrier"
one_digest = hashlib.sha256((one_name + "\n").encode()).hexdigest()
supported_name = "TestSupportedListenerBarriersAndReservation"
supported_digest = hashlib.sha256((supported_name + "\n").encode()).hexdigest()
inherited_path_previous = os.environ.get("PATH")
os.environ["PATH"] = "/opt/homebrew/bin:/usr/bin:/bin"
inherited_git_environment = {
    key: os.environ.pop(key)
    for key in list(os.environ)
    if key.startswith("GIT_")
}
common = [
    "GOTOOLCHAIN=go1.26.8", "go", "test", "-C", "experiments/g01-scaleset",
    "-race", "-count=1", "-timeout=45s",
]
count_zero_common = ["-count=0" if value == "-count=1" else value for value in common]
count_two_common = ["-count=2" if value == "-count=1" else value for value in common]
timeout_zero_common = ["-timeout=0s" if value == "-timeout=45s" else value for value in common]
timeout_large_common = ["-timeout=301s" if value == "-timeout=45s" else value for value in common]
no_timeout_common = [value for value in common if value != "-timeout=45s"]
gorace_common = ["GORACE=halt_on_error=1", *common]
cases = [
    (
        "equals-run",
        "1", one_digest, "experiments/g01-scaleset:./livecanary",
        "default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8",
        common + ["./livecanary", "-run=" + "^" + one_name + "$"],
        True,
    ),
    (
        "separated-filter",
        "22", "f3efc29451112b27d2cb590763612790db48f05b54718b1e023196ead6625222",
        "experiments/g01-scaleset:./livecanary", "g01_pair_fixture+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8",
        common + ["-tags=g01_pair_fixture", "./livecanary", "-run", "^TestPaired",
                  "-skip", "^TestPairedTerminal"],
        True,
    ),
    (
        "no-match",
        "1", one_digest, "experiments/g01-scaleset:./livecanary",
        "default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8",
        common + ["./livecanary", "-run=^TestNoSuchSelectorName$"],
        False,
    ),
    (
        "skip-all-equals",
        "1", one_digest, "experiments/g01-scaleset:./livecanary",
        "default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8",
        common + ["./livecanary", "-run=^" + one_name + "$",
                  "-skip=^" + one_name + "$"],
        False,
    ),
    (
        "posix-class-skip",
        "1", supported_digest, "experiments/g01-scaleset:./livecanary",
        "default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8",
        common + ["./livecanary", "-run",
                  "^(TestSupportedListenerBarriersAndReservation|TestNoMessageDoesNotCountAsCompletedBarrier)$",
                  "-skip", "[[:upper:]]o"],
        True,
    ),
    (
        "slash-subtest-skip",
        "1", one_digest, "experiments/g01-scaleset:./livecanary",
        "default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8",
        common + ["./livecanary", "-run", "^TestProbe$",
                  "-skip", "^TestProbe/subtest$"],
        False,
    ),
    (
        "count-zero",
        "1", one_digest, "experiments/g01-scaleset:./livecanary",
        "default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8",
        count_zero_common + ["./livecanary", "-run=^" + one_name + "$"],
        False,
    ),
    (
        "count-two",
        "1", one_digest, "experiments/g01-scaleset:./livecanary",
        "default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8",
        count_two_common + ["./livecanary", "-run=^" + one_name + "$"],
        False,
    ),
    (
        "timeout-zero",
        "1", one_digest, "experiments/g01-scaleset:./livecanary",
        "default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8",
        timeout_zero_common + ["./livecanary", "-run=^" + one_name + "$"],
        False,
    ),
    (
        "timeout-too-large",
        "1", one_digest, "experiments/g01-scaleset:./livecanary",
        "default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8",
        timeout_large_common + ["./livecanary", "-run=^" + one_name + "$"],
        False,
    ),
    (
        "timeout-missing",
        "1", one_digest, "experiments/g01-scaleset:./livecanary",
        "default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8",
        no_timeout_common + ["./livecanary", "-run=^" + one_name + "$"],
        False,
    ),
    (
        "timeout-duplicate",
        "1", one_digest, "experiments/g01-scaleset:./livecanary",
        "default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8",
        common + ["./livecanary", "-timeout=30s", "-run=^" + one_name + "$"],
        False,
    ),
    (
        "inherited-gorace",
        "1", one_digest, "experiments/g01-scaleset:./livecanary",
        "default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8",
        gorace_common + ["./livecanary", "-run=^" + one_name + "$"],
        False,
    ),
    (
        "args-test-selector",
        "1", one_digest, "experiments/g01-scaleset:./livecanary",
        "default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8",
        common + ["./livecanary", "-run=^" + one_name + "$", "-args", "-test.run=^Other$"],
        False,
    ),
    (
        "direct-test-selector",
        "1", one_digest, "experiments/g01-scaleset:./livecanary",
        "default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8",
        common + ["./livecanary", "-run=^" + one_name + "$", "-test.run=^Other$"],
        False,
    ),
    (
        "test-list-discovery",
        "1", one_digest, "experiments/g01-scaleset:./livecanary",
        "default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8",
        common + ["./livecanary", "-list", ".", "-run=^" + one_name + "$"],
        False,
    ),
    (
        "exec-wrapper",
        "1", one_digest, "experiments/g01-scaleset:./livecanary",
        "default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8",
        common + ["-exec", "true", "./livecanary", "-run=^" + one_name + "$"],
        False,
    ),
    (
        "active-package-init",
        "1", one_digest, "experiments/g01-scaleset:./livecanary",
        "g01_pair_fixture,g01_pair_real_cadence+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8",
        common + ["-tags=g01_pair_fixture,g01_pair_real_cadence", "./livecanary", "-run=^" + one_name + "$"],
        False,
    ),
    (
        "package-mismatch",
        "1", one_digest, "experiments/g01-scaleset:./livecanary",
        "default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8",
        common + ["./liveworker", "-run=^" + one_name + "$"],
        False,
    ),
    (
        "build-mismatch",
        "1", one_digest, "experiments/g01-scaleset:./livecanary",
        "default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8",
        common + ["-tags=g01_live", "./livecanary", "-run=^" + one_name + "$"],
        False,
    ),
    (
        "duplicate-package-target",
        "1", one_digest, "experiments/g01-scaleset:./livecanary",
        "default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8",
        common + ["./livecanary", "./liveworker", "-run=^" + one_name + "$"],
        False,
    ),
    (
        "extra-import-path-package",
        "1", one_digest, "experiments/g01-scaleset:./livecanary",
        "default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8",
        common + ["./livecanary",
                  "github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/liveworker",
                  "-run=^" + one_name + "$"],
        False,
    ),
    (
        "benchmark-flag",
        "1", one_digest, "experiments/g01-scaleset:./livecanary",
        "default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8",
        common + ["./livecanary", "-bench=.", "-run=^" + one_name + "$"],
        False,
    ),
    (
        "benchmark-test-binary-override",
        "1", one_digest, "experiments/g01-scaleset:./livecanary",
        "default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8",
        common + ["./livecanary", "-test.bench=.", "-run=^" + one_name + "$"],
        False,
    ),
    (
        "cpu-multiplicity",
        "1", one_digest, "experiments/g01-scaleset:./livecanary",
        "default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8",
        common + ["./livecanary", "-cpu=1,2", "-run=^" + one_name + "$"],
        False,
    ),
    (
        "conflicting-cgo",
        "1", one_digest, "experiments/g01-scaleset:./livecanary",
        "default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8",
        ["CGO_ENABLED=0", *common, "./livecanary", "-run=^" + one_name + "$"],
        False,
    ),
    (
        "conflicting-goexperiment",
        "1", one_digest, "experiments/g01-scaleset:./livecanary",
        "default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8",
        ["GOEXPERIMENT=rangefunc", *common, "./livecanary", "-run=^" + one_name + "$"],
        False,
    ),
]
with tempfile.TemporaryDirectory() as goenv_dir:
    goenv = Path(goenv_dir) / "env"
    goenv.write_text("GOFLAGS=-run=^PersistedConfigProbe$\n", encoding="utf-8")
    cases.append(
        (
            "persisted-goflags",
            "1", one_digest, "experiments/g01-scaleset:./livecanary",
            "default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8",
            [f"GOENV={goenv}", *common, "./livecanary",
             "-run=^" + one_name + "$"],
            False,
        )
    )
    namespace = {"__name__": "__main__"}
    for label, count, digest, package, build, command, should_pass in cases:
        sys.argv = ["wrapper-probe", count, digest, label, package, build, *command]
        output = io.StringIO()
        try:
            with redirect_stdout(output):
                exec(compile(wrapper, "<wrapper>", "exec"), namespace)
        except SystemExit as error:
            if should_pass:
                raise SystemExit(f"{label}: unexpected rejection: {error}")
            print(f"{label}: rejected before test body: {error}")
        else:
            if not should_pass:
                raise SystemExit(f"{label}: unexpectedly accepted")
            if "source selector validation passed" not in output.getvalue():
                raise SystemExit(f"{label}: missing source-selector result")
            print(f"{label}: accepted non-executing source selector")
inherited_gowork_previous = os.environ.get("GOWORK")
gowork_observed = []
goexperiment_observed = []
go_deadline_observed = []
real_run = subprocess.run

toolexec_cases = [
    (
        "toolexec-separated",
        common + ["-toolexec", "/synthetic/toolexec", "./livecanary",
                  "-run=^" + one_name + "$"],
    ),
    (
        "toolexec-equals",
        common + ["-toolexec=/synthetic/toolexec", "./livecanary",
                  "-run=^" + one_name + "$"],
    ),
]
toolexec_go_children = []

def reject_toolexec_go_child(*args, **kwargs):
    command = args[0] if args else kwargs.get("args", [])
    if command and command[0] == "go":
        toolexec_go_children.append(tuple(command))
        raise AssertionError("toolexec guard started a Go child")
    return real_run(*args, **kwargs)

subprocess.run = reject_toolexec_go_child
try:
    for toolexec_label, toolexec_command in toolexec_cases:
        sys.argv = [
            "wrapper-probe", "1", one_digest, toolexec_label,
            "experiments/g01-scaleset:./livecanary", "default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8",
            *toolexec_command,
        ]
        output = io.StringIO()
        try:
            with redirect_stdout(output):
                exec(compile(wrapper, "<wrapper>", "exec"), namespace)
        except SystemExit as error:
            if output.getvalue():
                raise SystemExit(
                    f"{toolexec_label}: rejection emitted a result before the guard"
                )
            print(
                f"{toolexec_label}: rejected before test body/result recording: "
                f"{error}"
            )
        else:
            raise SystemExit(
                f"{toolexec_label}: -toolexec was unexpectedly accepted"
            )
finally:
    subprocess.run = real_run
if toolexec_go_children:
    raise SystemExit(
        "toolexec regression: a Go child started before -toolexec rejection: "
        + repr(toolexec_go_children)
    )
print(
    "toolexec regression: both separated and equals-form -toolexec rejected "
    "before any Go child or result recording"
)

overlay_cases = [
    (
        "overlay-separated",
        common + ["-overlay", "/synthetic/overlay.json", "./livecanary",
                  "-run=^" + one_name + "$"],
    ),
    (
        "overlay-equals",
        common + ["-overlay=/synthetic/overlay.json", "./livecanary",
                  "-run=^" + one_name + "$"],
    ),
]
overlay_go_children = []

def reject_overlay_go_child(*args, **kwargs):
    command = args[0] if args else kwargs.get("args", [])
    if command and command[0] == "go":
        overlay_go_children.append(tuple(command))
        raise AssertionError("overlay guard started a Go child")
    return real_run(*args, **kwargs)

subprocess.run = reject_overlay_go_child
try:
    for overlay_label, overlay_command in overlay_cases:
        sys.argv = [
            "wrapper-probe", "1", one_digest, overlay_label,
            "experiments/g01-scaleset:./livecanary", "default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8",
            *overlay_command,
        ]
        output = io.StringIO()
        try:
            with redirect_stdout(output):
                exec(compile(wrapper, "<wrapper>", "exec"), namespace)
        except SystemExit as error:
            if output.getvalue():
                raise SystemExit(
                    f"{overlay_label}: rejection emitted a result before the guard"
                )
            print(
                f"{overlay_label}: rejected before test body/result recording: "
                f"{error}"
            )
        else:
            raise SystemExit(f"{overlay_label}: -overlay was unexpectedly accepted")
finally:
    subprocess.run = real_run
if overlay_go_children:
    raise SystemExit(
        "overlay regression: a Go child started before -overlay rejection: "
        + repr(overlay_go_children)
    )
print(
    "overlay regression: both separated and equals-form -overlay rejected before "
    "any Go child or result recording"
)

modfile_cases = [
    (
        "modfile-separated",
        common + ["-modfile", "/synthetic/alternate.mod", "./livecanary",
                  "-run=^" + one_name + "$"],
    ),
    (
        "modfile-equals",
        common + ["-modfile=/synthetic/alternate.mod", "./livecanary",
                  "-run=^" + one_name + "$"],
    ),
]
modfile_go_children = []

def reject_modfile_go_child(*args, **kwargs):
    command = args[0] if args else kwargs.get("args", [])
    if command and command[0] == "go":
        modfile_go_children.append(tuple(command))
        raise AssertionError("modfile guard started a Go child")
    return real_run(*args, **kwargs)

subprocess.run = reject_modfile_go_child
try:
    for modfile_label, modfile_command in modfile_cases:
        sys.argv = [
            "wrapper-probe", "1", one_digest, modfile_label,
            "experiments/g01-scaleset:./livecanary", "default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8",
            *modfile_command,
        ]
        output = io.StringIO()
        try:
            with redirect_stdout(output):
                exec(compile(wrapper, "<wrapper>", "exec"), namespace)
        except SystemExit as error:
            if output.getvalue():
                raise SystemExit(
                    f"{modfile_label}: rejection emitted a result before the guard"
                )
            print(
                f"{modfile_label}: rejected before test body/result recording: "
                f"{error}"
            )
        else:
            raise SystemExit(
                f"{modfile_label}: -modfile was unexpectedly accepted"
            )
finally:
    subprocess.run = real_run
if modfile_go_children:
    raise SystemExit(
        "modfile regression: a Go child started before -modfile rejection: "
        + repr(modfile_go_children)
    )
print(
    "modfile regression: both separated and equals-form -modfile rejected before "
    "any Go child or result recording"
)

env_test = ["go", "test", "-C", "experiments/g01-scaleset", "-race",
            "-count=1", "-timeout=45s"]
selector_guard_cases = [
    (
        "env-assignment-wrapper",
        ["env", "GOTOOLCHAIN=go1.26.8", *env_test, "./livecanary",
         "-run=^" + one_name + "$"],
    ),
    (
        "env-option-wrapper",
        ["env", "-i", "GOTOOLCHAIN=go1.26.8", *env_test, "./livecanary",
         "-run=^" + one_name + "$"],
    ),
    (
        "command-prefix-run",
        ["command", *env_test, "./livecanary",
         "-run=^" + one_name + "$"],
    ),
    (
        "assignment-command-prefix-skip",
        ["GOTOOLCHAIN=go1.26.8", "command", *env_test, "./livecanary",
         "-skip=^" + one_name + "$"],
    ),
    (
        "double-dash-run-separated",
        common + ["./livecanary", "--run", "^" + one_name + "$"],
    ),
    (
        "double-dash-run-equals",
        common + ["./livecanary", "--run=^" + one_name + "$"],
    ),
    (
        "double-dash-skip-separated",
        common + ["./livecanary", "-run=^" + one_name + "$", "--skip",
                  "^" + one_name + "$"],
    ),
    (
        "double-dash-skip-equals",
        common + ["./livecanary", "-run=^" + one_name + "$",
                  "--skip=^" + one_name + "$"],
    ),
]
selector_guard_go_children = []

def reject_selector_guard_go_child(*args, **kwargs):
    command = args[0] if args else kwargs.get("args", [])
    if command and command[0] == "go":
        selector_guard_go_children.append(tuple(command))
        raise AssertionError("selector guard started a Go child")
    return real_run(*args, **kwargs)

subprocess.run = reject_selector_guard_go_child
try:
    for selector_label, selector_command in selector_guard_cases:
        sys.argv = [
            "wrapper-probe", "1", one_digest, selector_label,
            "experiments/g01-scaleset:./livecanary", "default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8",
            *selector_command,
        ]
        output = io.StringIO()
        try:
            with redirect_stdout(output):
                exec(compile(wrapper, "<wrapper>", "exec"), namespace)
        except SystemExit as error:
            if output.getvalue():
                raise SystemExit(
                    f"{selector_label}: rejection emitted a result before the guard"
                )
            print(
                f"{selector_label}: rejected before test body/result recording: "
                f"{error}"
            )
        else:
            raise SystemExit(f"{selector_label}: unsafe selector was unexpectedly accepted")
finally:
    subprocess.run = real_run
if selector_guard_go_children:
    raise SystemExit(
        "selector guard regression: a Go child started before env/command/double-dash "
        "rejection: " + repr(selector_guard_go_children)
    )
print(
    "selector guard regression: standard env/command wrappers and "
    "separated/equals-form --run/--skip selectors rejected before any Go child "
    "or result recording"
)

def observe_go_child(*args, **kwargs):
    command = args[0] if args else kwargs.get("args", [])
    if command and command[0] == "go":
        child_env = kwargs.get("env")
        if child_env is None or child_env.get("GOWORK") != "off":
            raise SystemExit(
                "inherited-gowork: a direct Go child did not receive GOWORK=off"
            )
        if child_env.get("GOENV") != "off":
            raise SystemExit(
                "inherited-goenv: a direct Go child did not receive GOENV=off"
            )
        if child_env.get("CGO_ENABLED") != "1":
            raise SystemExit(
                "inherited-cgo: a direct Go child did not receive CGO_ENABLED=1"
            )
        if child_env.get("GOEXPERIMENT") != "none":
            raise SystemExit(
                "inherited-goexperiment: a direct Go child did not receive "
                "GOEXPERIMENT=none"
            )
        for target_name, target_value in {
            "GOOS": "darwin", "GOARCH": "arm64", "GOARM64": "v8.0",
        }.items():
            if child_env.get(target_name) != target_value:
                raise SystemExit(
                    f"inherited-target: a direct Go child did not receive {target_name}={target_value}"
                )
        if child_env.get("PATH") != "/opt/homebrew/bin:/usr/bin:/bin":
            raise SystemExit(
                "inherited-path: a direct Go child did not receive reviewed PATH"
            )
        if kwargs.get("timeout") != 300:
            raise SystemExit(
                "go-deadline: a direct Go child did not receive the 300s timeout"
            )
        gowork_observed.append(tuple(command[:3]))
        goexperiment_observed.append(child_env["GOEXPERIMENT"])
        go_deadline_observed.append(kwargs["timeout"])
    return real_run(*args, **kwargs)

os.environ["GOWORK"] = "/synthetic/external/workspace/go.work"
inherited_cgo_previous = os.environ.get("CGO_ENABLED")
os.environ["CGO_ENABLED"] = "0"
subprocess.run = observe_go_child
try:
    sys.argv = [
        "wrapper-probe", "1", one_digest, "inherited-gowork",
        "experiments/g01-scaleset:./livecanary", "default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8", *(
            common + ["./livecanary", "-run=^" + one_name + "$"]
        ),
    ]
    output = io.StringIO()
    try:
        with redirect_stdout(output):
            exec(compile(wrapper, "<wrapper>", "exec"), namespace)
    except SystemExit as error:
        raise SystemExit(f"inherited-gowork: unexpected rejection: {error}")
    expected_go_commands = [
        ("go", "env", "GOFLAGS"),
        ("go", "list", "-C"),
    ]
    if gowork_observed != expected_go_commands:
        raise SystemExit(
            "inherited-gowork: expected direct Go metadata/list probes "
            f"{expected_go_commands}, observed {gowork_observed}"
        )
    if "GOWORK=off" not in output.getvalue():
        raise SystemExit("inherited-gowork: force-off behavior was not recorded")
    if goexperiment_observed != ["none", "none"]:
        raise SystemExit(
            "inherited-goexperiment: expected GOEXPERIMENT=none on both direct "
            f"Go probes, observed {goexperiment_observed}"
        )
    if go_deadline_observed != [300, 300]:
        raise SystemExit(
            "go-deadline: expected 300s on both direct Go probes, observed "
            f"{go_deadline_observed}"
        )
    print(
        "inherited-gowork/goenv/cgo/goexperiment/target/path/deadline: passed; synthetic external "
        "workspace and CGO_ENABLED=0 were overridden; both direct Go metadata "
        "probes received GOENV=off, GOWORK=off, CGO_ENABLED=1, GOEXPERIMENT=none, "
        "GOOS=darwin, GOARCH=arm64, GOARM64=v8.0, reviewed PATH and the "
        "independent 300s deadline before source derivation"
    )
finally:
    subprocess.run = real_run
    if inherited_gowork_previous is None:
        os.environ.pop("GOWORK", None)
    else:
        os.environ["GOWORK"] = inherited_gowork_previous
    if inherited_cgo_previous is None:
        os.environ.pop("CGO_ENABLED", None)
    else:
        os.environ["CGO_ENABLED"] = inherited_cgo_previous
skip_stream = json.dumps({"Action": "skip", "Test": "TestSynthetic/subtest"}) + "\n"
namespace["label"] = "skip-probe"
validator = namespace.get("validate_test_stream")
try:
    validator(skip_stream, "")
except SystemExit as error:
    print(f"skipped-subtest-event: rejected before result recording: {error}")
else:
    raise SystemExit("skipped-subtest-event: skip event was accepted")
empty_stream = json.dumps({"Action": "pass", "Package": "example.test"}) + "\n"
namespace["label"] = "missing-run-pass-probe"
try:
    validator(empty_stream, "")
except SystemExit as error:
    print(f"missing-run-pass-events: rejected before result recording: {error}")
else:
    raise SystemExit("missing-run-pass-events: output-only stream was accepted")
inherited_previous = os.environ.get("G01_INPUT_CHILD")
os.environ["G01_INPUT_CHILD"] = "blocked"
sys.argv = [
    "wrapper-probe", "1", one_digest, "inherited-child-mode",
    "experiments/g01-scaleset:./livecanary", "default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8", *(
        common + ["./livecanary", "-run=^" + one_name + "$"]
    ),
]
try:
    try:
        with redirect_stdout(io.StringIO()):
            exec(compile(wrapper, "<wrapper>", "exec"), namespace)
    except SystemExit as error:
        print(f"inherited-child-mode: rejected before test body: {error}")
    else:
        raise SystemExit("inherited-child-mode: inherited fixture variable was accepted")
finally:
    if inherited_previous is None:
        os.environ.pop("G01_INPUT_CHILD", None)
    else:
        os.environ["G01_INPUT_CHILD"] = inherited_previous
    if inherited_path_previous is None:
        os.environ.pop("PATH", None)
    else:
        os.environ["PATH"] = inherited_path_previous
    os.environ.update(inherited_git_environment)
PY
```

The focused non-executing source-derivation probes passed: equals-form `-run=`
selected 1/1; separated `-run` plus `-skip` preserved the filtered 22/22 set;
the POSIX-class `[[:upper:]]` skip probe preserved the exact 1/1 set; the
no-match and equals-form skip-all cases were rejected with 0 observed executed
names before any test body; a livecanary/liveworker package mismatch, tag/build
mismatch and temporary non-`off` `GOENV` were rejected before source derivation;
duplicate package targeting was rejected before derivation; the synthetic
inherited-workspace/GOENV/CGO/GOEXPERIMENT/target/PATH/deadline probe observed
`GOENV=off`, `GOWORK=off`, `CGO_ENABLED=1`, `GOEXPERIMENT=none`,
`GOOS=darwin`, `GOARCH=arm64`, `GOARM64=v8.0`, reviewed PATH and the independent 300-second
deadline on both direct Go metadata probes before derivation; a conflicting
`GOEXPERIMENT=rangefunc` assignment was rejected before any child;
slash-delimited subtest
selectors, `-count=0`, `-count=2`, missing/duplicate/zero/overlarge
`-timeout`, inherited `GORACE`, `-args`, direct test-binary selector, `-exec`,
benchmark and test-binary benchmark overrides, `-cpu` multiplicity, an extra
import-path package, conflicting `CGO_ENABLED=0`, both separated and equals-form
`-toolexec`, `-modfile` and `-overlay` overrides, and `-list` discovery,
standard `env` assignment/option wrappers, bare and assignment-prefixed shell
`command go test` wrappers, all separated and equals-form double-dash
`--run`/`--skip` selectors, active package `init`, and inherited
`G01_INPUT_CHILD=blocked` were each rejected before source derivation. The
focused no-Go-child regressions patched direct Go-child launches and required
both `-toolexec`, `-modfile` and `-overlay` forms, both `env` forms, both
command-prefix forms and all four double-dash selector forms to reject with no
pre-guard result output; the
output-only stream without expected `run` and `pass` events was rejected before
result recording. No test body, live operation or secret-bearing input was run.

The added wrapper-regression output was:

```text
toolexec-separated: rejected before test body/result recording: toolexec-separated: Go -toolexec execution hooks are not allowed in a guarded rerun
toolexec-equals: rejected before test body/result recording: toolexec-equals: Go -toolexec execution hooks are not allowed in a guarded rerun
toolexec regression: both separated and equals-form -toolexec rejected before any Go child or result recording
modfile-separated: rejected before test body/result recording: modfile-separated: Go -modfile alternate module files are not allowed in a guarded rerun
modfile-equals: rejected before test body/result recording: modfile-equals: Go -modfile alternate module files are not allowed in a guarded rerun
modfile regression: both separated and equals-form -modfile rejected before any Go child or result recording
overlay-separated: rejected before test body/result recording: overlay-separated: Go -overlay build overrides are not allowed in a guarded rerun
overlay-equals: rejected before test body/result recording: overlay-equals: Go -overlay build overrides are not allowed in a guarded rerun
overlay regression: both separated and equals-form -overlay rejected before any Go child or result recording
env-assignment-wrapper: rejected before test body/result recording: env-assignment-wrapper: standard env-wrapped go test commands are not allowed; put assignments before go test
env-option-wrapper: rejected before test body/result recording: env-option-wrapper: standard env-wrapped go test commands are not allowed; put assignments before go test
command-prefix-run: rejected before test body/result recording: command-prefix-run: shell command-prefix go test wrappers are not allowed; invoke go test directly
assignment-command-prefix-skip: rejected before test body/result recording: assignment-command-prefix-skip: shell command-prefix go test wrappers are not allowed; invoke go test directly
double-dash-run-separated: rejected before test body/result recording: double-dash-run-separated: double-dash --run/--skip selectors are not allowed in a guarded rerun
double-dash-run-equals: rejected before test body/result recording: double-dash-run-equals: double-dash --run/--skip selectors are not allowed in a guarded rerun
double-dash-skip-separated: rejected before test body/result recording: double-dash-skip-separated: double-dash --run/--skip selectors are not allowed in a guarded rerun
double-dash-skip-equals: rejected before test body/result recording: double-dash-skip-equals: double-dash --run/--skip selectors are not allowed in a guarded rerun
selector guard regression: standard env/command wrappers and separated/equals-form --run/--skip selectors rejected before any Go child or result recording
slash-subtest-skip: rejected before test body: slash-subtest-skip: slash-delimited -skip selectors are rejected because top-level -list cannot validate subtest names
count-zero: rejected before test body: count-zero: exactly one -count=1 is required
timeout-missing: rejected before test body: timeout-missing: exactly one -timeout value is required
inherited-gorace: rejected before test body: inherited-gorace: inherited or command-supplied GORACE is not allowed in race mode
args-test-selector: rejected before test body: args-test-selector: -args is not allowed in a guarded rerun
exec-wrapper: rejected before test body: exec-wrapper: -exec execution wrappers are not allowed
active-package-init: rejected before test body: active-package-init: active package init requires a new reviewed guard
skipped-subtest-event: rejected before result recording: wrapper-probe: test execution contained a skipped test
missing-run-pass-events: rejected before result recording: missing-run-pass-probe: test execution was missing expected event(s): run=TestNoMessageDoesNotCountAsCompletedBarrier; pass=TestNoMessageDoesNotCountAsCompletedBarrier
inherited-gowork: passed; synthetic external workspace was overridden; both direct Go metadata probes received GOWORK=off before source derivation
inherited-child-mode: rejected before test body: inherited-child-mode: fixture child-mode environment is not allowed: G01_INPUT_CHILD
```

The exact wrapper replay at the prior packet head was then run in list-only mode with a synthetic
inherited external workspace. On the PR #78 packet head
`c550adcb2ca6532e2c69cbb6ad8d2aafd5e352ae`, records 01--20 reached list
validation with `GOWORK=off`; record 21 correctly stopped at the PR #72
source-tree guard before `go list`, rather than silently using the PR #78
module tree (`08c7830de7bc5120d1302d7ba6df162abd582315`) when the reviewed
PR #72 tree was `9b30ef1b69c6375cb264c759d366fc5a52a5439f`. The separate
existing PR #72 checkout at `f5560ba950f77343e57034cc1cf85dc67f5ac922` was
then used for the two PR #72 records, proving that their reviewed module-tree
pin is selected by actual metadata/list execution; no test body or live
resource ran:

```text
c550 historical exact replay (pre-source-derivation wrapper): records 01-20 list-only validation passed with GOWORK=off; record 21 drain-pr72: rejected before test body: drain-pr72: package-initialization guard requires reviewed source tree; direct Go child GOWORK checks=1
record 21 drain-pr72: list validation passed; package experiments/g01-scaleset:./livecanary; build default+norace+go1.26.8; GOWORK=off; raw listed 34; filtered executed 34 names; set-sha256 a79b7fa367d8eb1e7fe4ee4ef637696518946dab6f2f25410f6e04bfba137298
record 22 drain-pr72-race: list validation passed; package experiments/g01-scaleset:./livecanary; build default+race+go1.26.8; GOWORK=off; raw listed 34; filtered executed 34 names; set-sha256 a79b7fa367d8eb1e7fe4ee4ef637696518946dab6f2f25410f6e04bfba137298
PR72 historical exact replay: both records reached list-only validation; direct Go child environments checked for GOWORK=off
```

The working-tree source-derived replay then exercised the 26 non-PR72 wrapper
records (including every paired partition) against their literal counts and
SHA-256 sets. It invoked only the effective `go env GOFLAGS` and
`go list -json -test` metadata children; the wrapper body was truncated before
the original JSON test subprocess, and the two PR #72 records remained
correctly gated by their separate reviewed source-tree pin:

The two historical PR #72 output lines below retain the pre-correction
three-part build labels because that immutable replay predates the current
`CGO_ENABLED=1`, `GOEXPERIMENT=none`, GOROOT and GOFIPS140 pins; every current
prescription and metadata identity above uses the nine-part
`tags+race+cgo+cgo-tools+goexperiment+target+goroot+gofips140+toolchain` form.

```text
source-derived prescription replay: passed; 26 non-PR72 wrapper records validated without go test -list or test-body execution
```

### Current exact-head double-dash and execution-set correction

The exact starting packet head for this correction was
`22a2923033c875ddd4f755774f79f60b94649449`. The red reproduction below runs
only the immutable prior wrapper source with a synthetic `--overlay` input and
stops at its first attempted Go child. It proves that the prior predicate let
the bypass reach `go env GOFLAGS` before any overlay/flag rejection; it does
not run the metadata query, test binary or test body. The current wrapper
regression then covers separated and equals-form equivalents for tool, module,
overlay, timeout, exec, args, benchmark, selector, CPU and build flags, and
requires empty output and zero direct Go children for every case.

```sh
set -euo pipefail
[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || { printf '%s\n' 'reviewed canonical PATH and absolute Python interpreter required' >&2; exit 1; }
/opt/homebrew/bin/python3 -I - <<'PY'
import io
import subprocess
import sys
from contextlib import redirect_stdout
from pathlib import Path

prior_head = "22a2923033c875ddd4f755774f79f60b94649449"
previous = subprocess.check_output(
    ["git", "show", f"{prior_head}:docs/evidence/g01-recovery-packet.md"],
    text=True,
)
wrapper_start = previous.index("\nimport hashlib\n", previous.index("go_test_checked()")) + 1
wrapper_end = previous.index("\nPY\n}", wrapper_start)
previous_wrapper = previous[wrapper_start:wrapper_end]
if "double_dash_names" in previous_wrapper:
    raise SystemExit("red reproduction setup changed: prior wrapper already had generic double-dash rejection")

class StopAtGoChild(Exception):
    pass

real_run = subprocess.run
go_children = []
def observe_prior_go_child(*args, **kwargs):
    command = args[0] if args else kwargs.get("args", [])
    if command and command[0] == "go":
        go_children.append(tuple(command))
        raise StopAtGoChild
    return real_run(*args, **kwargs)

subprocess.run = observe_prior_go_child
try:
    sys.argv = [
        "wrapper-probe", "1",
        "9aef95c84ffd42ad632040498c628c78072ce94f5cc2f6af8493fcffc233b707",
        "red-double-dash-overlay", "experiments/g01-scaleset:./livecanary",
        "default+race+go1.26.8", "GOTOOLCHAIN=go1.26.8", "go", "test",
        "-C", "experiments/g01-scaleset", "-race", "-count=1", "-timeout=45s",
        "./livecanary", "-run=^TestNoMessageDoesNotCountAsCompletedBarrier$",
        "--overlay=/synthetic/overlay.json",
    ]
    output = io.StringIO()
    try:
        with redirect_stdout(output):
            exec(compile(previous_wrapper, "<prior-wrapper>", "exec"), {"__name__": "__main__"})
    except StopAtGoChild:
        pass
finally:
    subprocess.run = real_run
if go_children != [("go", "env", "GOFLAGS")]:
    raise SystemExit(f"red reproduction did not stop at prior go env: {go_children!r}")
if output.getvalue():
    raise SystemExit("red reproduction emitted a result before the prior go env child")
print(
    f"RED double-dash bypass: prior {prior_head} reached go env GOFLAGS before rejecting synthetic --overlay; no metadata/test body ran"
)

packet = Path("docs/evidence/g01-recovery-packet.md").read_text(encoding="utf-8")
current_start = packet.index("\nimport hashlib\n", packet.index("go_test_checked()")) + 1
current_end = packet.index("\nPY\n}", current_start)
current_wrapper = packet[current_start:current_end]
current_wrapper = current_wrapper[:current_wrapper.index("run_result = run_go_child")]
common = [
    "GOTOOLCHAIN=go1.26.8", "go", "test", "-C", "experiments/g01-scaleset",
    "-race", "-count=1", "-timeout=45s", "./livecanary",
    "-run=^TestNoMessageDoesNotCountAsCompletedBarrier$",
]
cases = {
    "toolexec-separated": ["--toolexec", "/synthetic/toolexec"],
    "toolexec-equals": ["--toolexec=/synthetic/toolexec"],
    "modfile-separated": ["--modfile", "/synthetic/alternate.mod"],
    "modfile-equals": ["--modfile=/synthetic/alternate.mod"],
    "overlay-separated": ["--overlay", "/synthetic/overlay.json"],
    "overlay-equals": ["--overlay=/synthetic/overlay.json"],
    "timeout-separated": ["--timeout", "45s"],
    "timeout-equals": ["--timeout=45s"],
    "exec-separated": ["--exec", "true"],
    "exec-equals": ["--exec=true"],
    "args-separated": ["--args", "-test.run=^Other$"],
    "args-equals": ["--args=-test.run=^Other$"],
    "bench-separated": ["--bench", "."],
    "bench-equals": ["--bench=."],
    "test-bench-equals": ["--test.bench=."],
    "selector-run-equals": ["--run=^TestNoMessageDoesNotCountAsCompletedBarrier$"],
    "selector-skip-equals": ["--skip=^TestNoMessageDoesNotCountAsCompletedBarrier$"],
    "cpu-equals": ["--cpu=1,2"],
    "tags-equals": ["--tags=synthetic"],
    "gcflags-equals": ["--gcflags=all=-N"],
}
real_run = subprocess.run
current_go_children = []
def reject_current_go_child(*args, **kwargs):
    command = args[0] if args else kwargs.get("args", [])
    if command and command[0] == "go":
        current_go_children.append(tuple(command))
        raise AssertionError("current generic double-dash guard started a Go child")
    return real_run(*args, **kwargs)

subprocess.run = reject_current_go_child
try:
    for label, option in cases.items():
        sys.argv = [
            "wrapper-probe", "1",
            "9aef95c84ffd42ad632040498c628c78072ce94f5cc2f6af8493fcffc233b707",
            label, "experiments/g01-scaleset:./livecanary",
            "default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8", *common, *option,
        ]
        output = io.StringIO()
        try:
            with redirect_stdout(output):
                exec(compile(current_wrapper, "<current-wrapper>", "exec"), {"__name__": "__main__"})
        except SystemExit as error:
            if output.getvalue():
                raise SystemExit(f"{label}: guard emitted a result before rejection")
            print(f"{label}: rejected before any Go child: {error}")
        else:
            raise SystemExit(f"{label}: equivalent double-dash flag was accepted")
finally:
    subprocess.run = real_run
if current_go_children:
    raise SystemExit(
        "current double-dash regression started a Go child: "
        + repr(current_go_children)
    )
print(
    f"double-dash regression: passed; {len(cases)} equivalent tool/module/overlay/timeout/exec/args/benchmark/selector/CPU/build forms rejected before any Go child or result recording"
)
PY
```

The immutable prior-head red probe reached exactly `go env GOFLAGS` before any
metadata query, while every current synthetic equivalent spelling rejected
with empty pre-guard output and zero direct Go children. This is static/wrapper
evidence only: no Go test body, `go test -list`, live operation, credential or
private path was used.

Recorded red/green output:

```text
RED double-dash bypass: prior 22a2923033c875ddd4f755774f79f60b94649449 reached go env GOFLAGS before rejecting synthetic --overlay; no metadata/test body ran
double-dash regression: passed; 20 equivalent tool/module/overlay/timeout/exec/args/benchmark/selector/CPU/build forms rejected before any Go child or result recording
```

The same immutable prior-head probe independently covered the remaining
single-dash bypasses. It runs the prior wrapper only until its first `go env`
child for benchmark, test-binary benchmark, CPU, extra import-path and
inherited-CGO inputs, and checks the prior source helper omitted executable
`Example` declarations; no prior metadata query or test binary is allowed to
start:

```sh
set -euo pipefail
[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || { printf '%s\n' 'reviewed canonical PATH and absolute Python interpreter required' >&2; exit 1; }
/opt/homebrew/bin/python3 -I - <<'PY'
import io
import os
import subprocess
import sys
from contextlib import redirect_stdout
from pathlib import Path

prior_head = "22a2923033c875ddd4f755774f79f60b94649449"
previous = subprocess.check_output(
    ["git", "show", f"{prior_head}:docs/evidence/g01-recovery-packet.md"],
    text=True,
)
wrapper_start = previous.index("\nimport hashlib\n", previous.index("go_test_checked()")) + 1
wrapper_end = previous.index("\nPY\n}", wrapper_start)
previous_wrapper = previous[wrapper_start:wrapper_end]
if "benchmark execution or benchmark overrides" in previous_wrapper:
    raise SystemExit("red reproduction setup changed: prior wrapper already rejected benchmarks")
if "-cpu multiplicity overrides" in previous_wrapper:
    raise SystemExit("red reproduction setup changed: prior wrapper already rejected CPU")
if 'value == "." or value.startswith("./")' not in previous_wrapper:
    raise SystemExit("red reproduction setup changed: prior package scan was not the narrow form")
if "CGO_ENABLED" in previous_wrapper:
    raise SystemExit("red reproduction setup changed: prior wrapper already pinned CGO")

class StopAtGoChild(Exception):
    pass

real_run = subprocess.run
go_children = []
def stop_at_go_child(*args, **kwargs):
    command = args[0] if args else kwargs.get("args", [])
    if command and command[0] == "go":
        go_children.append(tuple(command))
        raise StopAtGoChild
    return real_run(*args, **kwargs)

common = [
    "GOTOOLCHAIN=go1.26.8", "go", "test", "-C", "experiments/g01-scaleset",
    "-race", "-count=1", "-timeout=45s", "./livecanary",
    "-run=^TestNoMessageDoesNotCountAsCompletedBarrier$",
]
cases = {
    "bench": ["-bench=."],
    "test-bench": ["-test.bench=."],
    "cpu": ["-cpu=1,2"],
    "extra-import": ["github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/liveworker"],
}
inherited_cgo_previous = os.environ.get("CGO_ENABLED")
os.environ["CGO_ENABLED"] = "0"
subprocess.run = stop_at_go_child
try:
    for label, suffix in cases.items():
        sys.argv = [
            "wrapper-probe", "1",
            "9aef95c84ffd42ad632040498c628c78072ce94f5cc2f6af8493fcffc233b707",
            "red-" + label, "experiments/g01-scaleset:./livecanary",
            "default+race+go1.26.8", *common, *suffix,
        ]
        try:
            with redirect_stdout(io.StringIO()):
                exec(compile(previous_wrapper, "<prior-wrapper>", "exec"), {"__name__": "__main__"})
        except StopAtGoChild:
            pass
        else:
            raise SystemExit(f"red-{label}: prior wrapper did not reach a Go child")
        if go_children[-1] != ("go", "env", "GOFLAGS"):
            raise SystemExit(f"red-{label}: unexpected first Go child {go_children[-1]!r}")
finally:
    subprocess.run = real_run
    if inherited_cgo_previous is None:
        os.environ.pop("CGO_ENABLED", None)
    else:
        os.environ["CGO_ENABLED"] = inherited_cgo_previous

helper_start = previous_wrapper.index("def skip_source_ignored")
helper_end = previous_wrapper.index("\nall_test_names = []", helper_start)
namespace = {"label": "prior-example-red"}
exec(compile(previous_wrapper[helper_start:helper_end], "<prior-source-name>", "exec"), namespace)
source_names = namespace["source_test_names"]
with __import__("tempfile").TemporaryDirectory() as directory:
    example = Path(directory) / "example_test.go"
    example.write_text("package p\nfunc ExampleWidget() {}\n", encoding="utf-8")
    if source_names(example) != []:
        raise SystemExit("red-example: prior parser unexpectedly included Example")
print(
    f"RED pre-correction bypasses: prior {prior_head} reached go env for bench/test-bench/CPU/extra-import and inherited CGO cases; prior source parser omitted Example; no test body ran"
)
PY
```

Recorded red output:

```text
RED pre-correction bypasses: prior 22a2923033c875ddd4f755774f79f60b94649449 reached go env for bench/test-bench/CPU/extra-import and inherited CGO cases; prior source parser omitted Example; no test body ran
```

The tagged worker command is a complete `./liveworker` `^TestPaired` partition
using the same fixture tag, toolchain, race detector, count and timeout as the
controller-side partitions. The controller-side terminal groups in this
fragment are exhaustive and disjoint: the five heavy names, the complementary
terminal remainder, and the named persistence tests. Its first two
controller-side collection/listener commands are likewise exhaustive and disjoint as specified
in [the terminal path record](g01-paired-terminal.md).

For every rerun, retain the exact source/evidence SHA, command, exit status,
elapsed time where useful, classification and limitations. Do not convert a
skipped, unavailable or unauthorized live phase into “passed.”

## Documentation-only validation

This packet correction requires markdown/link-target, JSON syntax, ledger/table,
fragment, selector, diff, and secret/private-path checks only. No artificial Go
red or green test is created for documentation changes. The stable working
directory and source boundary were checked from immutable validation snapshot
5979b7d722f3bf8e24404912f9b1f3e888d0828d; that historical snapshot is not the
current PR #78 head. The prior packet-correction head at the start of this
correction was exactly
6b1535ee7b6f08582ff162eca30f1e4294dbf32b and has the explicit exact-head
record below. The earlier PR #78 head at the previous validation point was
82eeef99f9bb5ec85c8cb3bea7a9a5947e8df26a and is retained only as historical
provenance; the pushed correction head will necessarily be a new SHA and
requires its own PR metadata, exact-head review and CI. This packet correction
updates only packet documentation, including selector-wrapper metadata and
validation wording; it leaves the experiments source unchanged. The comparison parent
ee8df8b7e00204c74a892b27f8b4c0ab278751ba is only the unchanged
experiments/g01-scaleset source-comparison parent.
The exact parent for this follow-up is `da1af0d041e37e5df9f3ed8028b51a69ec58ed8c`;
all output records below the historical blocks are scoped explicitly either to
that parent or to the candidate worktree derived from it. Historical snapshot,
pre-correction-head and prior correction outputs are not current results and
must not be reused as evidence for this follow-up.

The selector declaration audit is anchored to immutable source commit
`95cd9210620c54e098ecbe0df1217af1659f0c74` and tree
`d8b79cd1ddc6993792a44a8e8ae88985ce7466c0`. The audited default-build
declarations are in `./liveworker` (`docker_observation_test.go`,
`docker_test.go`, `worker_test.go`, and `journal_test.go`) and `./livecanary`
(`driver_test.go`, `observer*.go`, `statistics_fence_test.go`,
`owned_proof_test.go`, `journal*.go`, and `preparation_test.go`). The requested
direct contracts are plain `package livecanary`/`package liveworker` default-build
tests with no file build constraint; the only tag-sensitive test is
`TestUnsupportedAccountLookupRefusesBeforeJournal`, duplicated in both
packages and guarded by `//go:build !cgo || osusergo || android`, so the added
livecanary invocation forces `-tags=osusergo`; `TestZeroStatisticsAndOptionalAbsencePermitEmptyCleanup`
remains a default-build `./livecanary` test in `statistics_fence_test.go`.

The `./livecanary` test binary has a `TestMain` in
`preparation_fixture_test.go`. A historical `go test -list` audit therefore
could start that function before printing names; the current wrapper never
starts that binary for selector validation. The source audit below still parses
the complete function through the normal `os.Exit(m.Run())` terminal, requires
exactly the two explicit `--prepare-approved-*` branches, and proves that the
normal-path projection contains no other statement; those branches are the
only paths that read approval/state inputs or prepare a journal. A synthetic
unconditional post-branch setup fixture must be rejected before execution.
Current selector validation derives names from source and therefore cannot run
TestMain, package-level variable initializers or imported initialization paths
before the expected set is checked.

The following commands are the exact documentation/static checks used for this
correction. Their results are recorded immediately after each check; no command
below runs a test body or performs a live App, runner, Docker, Lima, Keychain,
launchd or workflow operation.

The three fresh findings first had explicit red/static reproductions against the
immutable starting packet head `36ec84b27c934a25484b0a5391af0a20c7643912`:

```sh
set -euo pipefail
[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || { printf '%s\n' 'reviewed canonical PATH and absolute Python interpreter required' >&2; exit 1; }
/opt/homebrew/bin/python3 -I - <<'PY'
import subprocess

previous = subprocess.check_output(
    [
        "git", "show",
        "36ec84b27c934a25484b0a5391af0a20c7643912:docs/evidence/g01-recovery-packet.md",
    ],
    text=True,
)
wrapper_start = previous.index("\nimport hashlib\n", previous.index("go_test_checked()")) + 1
wrapper_end = previous.index("\nPY\n}", wrapper_start)
previous_wrapper = previous[wrapper_start:wrapper_end]
if "-toolexec" in previous_wrapper:
    raise SystemExit("red reproduction setup changed: starting wrapper already mentions -toolexec")
print("RED toolexec: starting wrapper had no separated/equals-form -toolexec guard")

lines = previous.splitlines()
wrapper_count = sum(line.lstrip().startswith("go_test_checked ") for line in lines)
synthetic_assignment_selector = "GOTOOLCHAIN=go1.26.8 go test ./livecanary -run ^TestRedProbe$"
naive_wrapper_only_count = sum(
    line.lstrip().startswith("go_test_checked ")
    for line in lines + [synthetic_assignment_selector]
)
if naive_wrapper_only_count != wrapper_count:
    raise SystemExit("red reproduction setup changed: wrapper-only count already saw the synthetic command")
print("RED prescription audit: wrapper-only count stayed at 28 after an unguarded assignment-prefixed go test selector")

if "[\"go\", \"test\", *list_args]" not in previous_wrapper:
    raise SystemExit("red reproduction setup changed: starting wrapper no longer invokes go test -list")
if "package-variable" in previous_wrapper:
    raise SystemExit("red reproduction setup changed: starting wrapper already named package-variable safety")
print("RED source/init: starting wrapper invoked go test -list and did not guard effectful package-variable/imported init paths")
PY
```

The red reproduction exited through all three expected failure witnesses: the
starting wrapper lacked both `-toolexec` forms, a naive wrapper-only selector
count ignored the synthetic assignment-prefixed command, and the starting
wrapper still invoked `go test -list` without package-variable/imported-init
coverage. The focused green corrections and boundary checks follow below.

The command-prefix selector finding was then reproduced against the exact prior
packet head `ec5eb8087420bbbbb2a8ccf5c5df190b3c644895`, immediately before this
correction. The prior shell and logical selector regexes matched neither the
bare `command go test ./livecanary -run ...` form nor the
`GOTOOLCHAIN=go1.26.8 command go test ./livecanary -skip=...` form, so a
wrapper-only count could remain unchanged; the prior wrapper also had no
explicit command-prefix rejection. The red probe below reads the immutable
prior packet and records those exact misses without starting any child:

```sh
set -euo pipefail
[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || { printf '%s\n' 'reviewed canonical PATH and absolute Python interpreter required' >&2; exit 1; }
/opt/homebrew/bin/python3 -I - <<'PY'
import re
import subprocess

prior_head = "ec5eb8087420bbbbb2a8ccf5c5df190b3c644895"
previous = subprocess.check_output(
    [
        "git",
        "show",
        f"{prior_head}:docs/evidence/g01-recovery-packet.md",
    ],
    text=True,
)
wrapper_start = previous.index("\nimport hashlib\n", previous.index("go_test_checked()")) + 1
wrapper_end = previous.index("\nPY\n}", wrapper_start)
previous_wrapper = previous[wrapper_start:wrapper_end]
if "shell command-prefix go test wrappers are not allowed" in previous_wrapper:
    raise SystemExit("red reproduction setup changed: prior wrapper already had command guard")

old_shell_selector = re.compile(
    r"^\s*(?:(?:[A-Za-z_][A-Za-z0-9_]*=[^\s]+)\s+)*"
    r"(?:env\s+.*\s+)?go test\b.*\s--?(?:run|skip)(?:=|\s)"
)
old_logical_selector = re.compile(
    r"^\s*(?:go_test_checked\b.*\bgo test\b|"
    r"(?:(?:[A-Za-z_][A-Za-z0-9_]*=[^\s]+)\s+)*"
    r"(?:env\s+.*\s+)?go test\b)"
    r".*\s--?(?:run|skip)(?:=|\s)",
    re.DOTALL,
)
probes = [
    "command go test ./livecanary -run ^TestCommandProbe$",
    "GOTOOLCHAIN=go1.26.8 command go test ./livecanary -skip=^TestCommandProbe$",
]
for probe in probes:
    if old_shell_selector.search(probe) or old_logical_selector.search(probe):
        raise SystemExit(f"red reproduction setup changed: prior regex matched {probe!r}")
old_wrapper_count = sum(
    line.lstrip().startswith("go_test_checked ")
    for line in previous.splitlines()
)
if old_wrapper_count != 28:
    raise SystemExit(f"red reproduction setup changed: prior wrapper count was {old_wrapper_count}")
print(
    f"RED command-prefix selector audit: prior {prior_head} matched 0/2 required "
    "command-prefixed selectors in both regexes; wrapper-only count stayed at 28 "
    "and no explicit command-prefix guard was present"
)
PY
```

Recorded red output (the command exited 0 after both expected misses were
observed) was:

```text
RED command-prefix selector audit: prior ec5eb8087420bbbbb2a8ccf5c5df190b3c644895 matched 0/2 required command-prefixed selectors in both regexes; wrapper-only count stayed at 28 and no explicit command-prefix guard was present
```

### Fresh exact-head P2 corrections at `3bc8445567fe68cc355cf3f88f0c962a41e9cad5`

The three fresh Codex P2 findings were reproduced against the immutable packet
head at task start. Each red witness reads only the prior packet blob; each
current green probe is static or source-only and proves its rejection boundary
without starting a Go child, test body or live operation.

#### Root 4001124039: top-level fuzz declarations

The immutable red witness confirms that the starting source-name helper silently
omitted a synthetic `FuzzSeed` declaration and that `paired-all-except` had no
`-run`, so an unrepresented fuzz seed could reach the original Go command:

```sh
set -euo pipefail
[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || { printf '%s\n' 'reviewed canonical PATH and absolute Python interpreter required' >&2; exit 1; }
/opt/homebrew/bin/python3 -I - <<'PY'
import subprocess
from pathlib import Path
from tempfile import TemporaryDirectory

starting_head = "3bc8445567fe68cc355cf3f88f0c962a41e9cad5"
previous = subprocess.check_output(
    [
        "git",
        "show",
        f"{starting_head}:docs/evidence/g01-recovery-packet.md",
    ],
    text=True,
)
wrapper_start = previous.index("\nimport hashlib\n", previous.index("go_test_checked()")) + 1
wrapper_end = previous.index("\nPY\n}", wrapper_start)
previous_wrapper = previous[wrapper_start:wrapper_end]
if "top-level Fuzz* declaration" in previous_wrapper:
    raise SystemExit("red reproduction setup changed: starting wrapper already rejected Fuzz*")
pair_start = previous.index("go_test_checked 106 ", previous.index("go_test_checked()"))
pair_end = previous.index("\ngo_test_checked ", pair_start + 1)
paired_all_except = previous[pair_start:pair_end]
if " -run " in paired_all_except or " -run=" in paired_all_except:
    raise SystemExit("red reproduction setup changed: paired-all-except gained -run")
helper_start = previous_wrapper.index("def skip_source_ignored")
helper_end = previous_wrapper.index("\nall_test_names = []", helper_start)
namespace = {"label": "prior-fuzz-red"}
exec(compile(previous_wrapper[helper_start:helper_end], "<prior-fuzz-source-parser>", "exec"), namespace)
with TemporaryDirectory() as directory:
    source_path = Path(directory) / "synthetic_test.go"
    source_path.write_text(
        "package p\n"
        "func FuzzSeed(f *testing.F) {}\n"
        "func TestStable(t *testing.T) {}\n",
        encoding="utf-8",
    )
    source_names = namespace["source_test_names"]
    if source_names(source_path) != ["TestStable"]:
        raise SystemExit("red reproduction setup changed: prior parser no longer omitted FuzzSeed")
print(
    f"RED fuzz source-set gap: prior {starting_head} omitted top-level FuzzSeed; "
    "paired-all-except had no -run and could execute an unrepresented fuzz seed"
)
PY
```

Recorded red output:

```text
RED fuzz source-set gap: prior 3bc8445567fe68cc355cf3f88f0c962a41e9cad5 omitted top-level FuzzSeed; paired-all-except had no -run and could execute an unrepresented fuzz seed
```

The current source-only guard scans the reviewed package directory before any
Go metadata child, and the source-name helper retains the same rejection as a
defense-in-depth check. The no-Go-child probe uses a temporary synthetic
`FuzzSeed` source and patches any attempted subprocess child to fail:

```sh
set -euo pipefail
[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || { printf '%s\n' 'reviewed canonical PATH and absolute Python interpreter required' >&2; exit 1; }
/opt/homebrew/bin/python3 -I - <<'PY'
import io
import subprocess
from contextlib import redirect_stdout
from pathlib import Path
from tempfile import TemporaryDirectory

packet = Path("docs/evidence/g01-recovery-packet.md").read_text(encoding="utf-8")
wrapper_start = packet.index("\nimport hashlib\n", packet.index("go_test_checked()")) + 1
wrapper_end = packet.index("\nPY\n}", wrapper_start)
wrapper = packet[wrapper_start:wrapper_end]
helper_start = wrapper.index("def source_fuzz_declarations")
helper_end = wrapper.index("\nsource_fuzz_guard()", helper_start)
with TemporaryDirectory() as directory:
    root = Path(directory)
    module = root / "module"
    module.mkdir()
    (module / "synthetic_test.go").write_text(
        "package p\nfunc FuzzSeed(f *testing.F) {}\n",
        encoding="utf-8",
    )
    namespace = {
        "label": "fuzz-green",
        "repo_root": root.resolve(),
        "module_dir": "module",
        "package_value": ".",
    }
    exec(compile(wrapper[helper_start:helper_end], "<fuzz-guard>", "exec"), namespace)
    real_run = subprocess.run
    go_children = []

    def reject_go_child(*args, **kwargs):
        command = args[0] if args else kwargs.get("args", [])
        if command and command[0] == "go":
            go_children.append(tuple(command))
            raise AssertionError("fuzz guard started a Go child")
        return real_run(*args, **kwargs)

    subprocess.run = reject_go_child
    try:
        output = io.StringIO()
        try:
            with redirect_stdout(output):
                namespace["source_fuzz_guard"]()
        except SystemExit as error:
            if "top-level Fuzz* declaration" not in str(error):
                raise
        else:
            raise SystemExit("fuzz guard unexpectedly accepted FuzzSeed")
    finally:
        subprocess.run = real_run
    if go_children:
        raise SystemExit(f"fuzz guard started Go child(ren): {go_children!r}")
print("fuzz guard regression: passed; top-level FuzzSeed rejected before any Go child or digest")
PY
```

#### Root 4001124042: Go/Python regexp semantic mismatch

The immutable red witness uses Unicode `é`: Python's `\\w` accepts it, while
Go's reviewed regexp Perl class is ASCII-oriented. The starting helper compiled
the pattern without rejecting it, so Python could derive an expected set that
Go would not execute:

```sh
set -euo pipefail
[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || { printf '%s\n' 'reviewed canonical PATH and absolute Python interpreter required' >&2; exit 1; }
/opt/homebrew/bin/python3 -I - <<'PY'
import re
import subprocess
from pathlib import Path

starting_head = "3bc8445567fe68cc355cf3f88f0c962a41e9cad5"
previous = subprocess.check_output(
    [
        "git",
        "show",
        f"{starting_head}:docs/evidence/g01-recovery-packet.md",
    ],
    text=True,
)
wrapper_start = previous.index("\nimport hashlib\n", previous.index("go_test_checked()")) + 1
wrapper_end = previous.index("\nPY\n}", wrapper_start)
previous_wrapper = previous[wrapper_start:wrapper_end]
helper_start = previous_wrapper.index("def go_compatible_regexp")
helper_end = previous_wrapper.index("\nall_test_names = []", helper_start)
namespace = {"label": "prior-unicode-regexp-red", "re": re}
exec(compile(previous_wrapper[helper_start:helper_end], "<prior-regexp>", "exec"), namespace)
go_regexp = namespace["go_compatible_regexp"]
compiled = go_regexp(r"\w", "-run")
if not compiled.search("é"):
    raise SystemExit("red reproduction setup changed: Python no longer accepts Unicode \\w")
print(
    f"RED regexp semantic gap: prior {starting_head} accepted \\w and Python matched Unicode é; "
    "Go's ASCII Perl-class semantics were not fail-closed"
)
PY
```

Recorded red output:

```text
RED regexp semantic gap: prior 3bc8445567fe68cc355cf3f88f0c962a41e9cad5 accepted \w and Python matched Unicode é; Go's ASCII Perl-class semantics were not fail-closed
```

The current preflight rejects all eight reviewed Go Perl classes/boundaries
before Python compilation and before the package metadata child. It consumes
escaped pairs, so a literal backslash followed by `b` remains supported. The
green probe records both properties and rejects any attempted Go child:

```sh
set -euo pipefail
[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || { printf '%s\n' 'reviewed canonical PATH and absolute Python interpreter required' >&2; exit 1; }
/opt/homebrew/bin/python3 -I - <<'PY'
import re
import subprocess
from pathlib import Path

packet = Path("docs/evidence/g01-recovery-packet.md").read_text(encoding="utf-8")
wrapper_start = packet.index("\nimport hashlib\n", packet.index("go_test_checked()")) + 1
wrapper_end = packet.index("\nPY\n}", wrapper_start)
wrapper = packet[wrapper_start:wrapper_end]
preflight = wrapper.index("reject_unsupported_regexp_syntax(original_run_pattern")
metadata_guard = wrapper.index("test_source_paths = package_initialization_guard()")
if preflight > metadata_guard:
    raise SystemExit("regexp preflight moved after the first package metadata child")
helper_start = wrapper.index("def reject_python_semantic_regexp_constructs")
guard_call = wrapper.index("\nsource_fuzz_guard()", helper_start)
go_start = wrapper.index("def go_compatible_regexp", helper_start)
go_end = wrapper.index("\nall_test_names = []", go_start)
namespace = {"label": "unicode-regexp-green", "re": re}
helper_source = wrapper[helper_start:guard_call] + wrapper[go_start:go_end]
exec(compile(helper_source, "<regexp-guard>", "exec"), namespace)
real_compile = re.compile
compile_calls = []

def recording_compile(pattern, *args, **kwargs):
    compile_calls.append(pattern)
    return real_compile(pattern, *args, **kwargs)

real_run = subprocess.run
go_children = []

def reject_go_child(*args, **kwargs):
    command = args[0] if args else kwargs.get("args", [])
    if command and command[0] == "go":
        go_children.append(tuple(command))
        raise AssertionError("regexp guard started a Go child")
    return real_run(*args, **kwargs)

re.compile = recording_compile
subprocess.run = reject_go_child
try:
    go_regexp = namespace["go_compatible_regexp"]
    for unsupported_pattern in [r"\b", r"\B", r"\w", r"\W", r"\d", r"\D", r"\s", r"\S"]:
        try:
            go_regexp(unsupported_pattern, "-run")
        except SystemExit as error:
            if "Go Perl regexp construct" not in str(error):
                raise
        else:
            raise SystemExit(f"unsupported Go Perl construct was accepted: {unsupported_pattern!r}")
finally:
    re.compile = real_compile
    subprocess.run = real_run
if compile_calls:
    raise SystemExit(f"regexp guard compiled unsupported pattern(s): {compile_calls!r}")
literal = go_regexp(r"\\b", "-run")
if not literal.fullmatch(r"\b"):
    raise SystemExit("escaped literal backslash was not preserved")
if go_children:
    raise SystemExit(f"regexp guard started Go child(ren): {go_children!r}")
print("regexp guard regression: passed; \\w rejected before Python compile/Go child, escaped literal \\b preserved")
PY
```

#### Root 4001124048: indirect forbidden live-command forms

The immutable red witness shows that the prior anchored scan missed remote
fetchers and wrapper-prefixed live commands. The current scanner instead parses
only executable shell prescriptions, strips safe shell prefixes, joins
continuations, skips comments/prose/URLs and excludes Python heredoc bodies
that contain scanner source or synthetic fixtures:

```sh
set -euo pipefail
[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || { printf '%s\n' 'reviewed canonical PATH and absolute Python interpreter required' >&2; exit 1; }
/opt/homebrew/bin/python3 -I - <<'PY'
import re
import subprocess

starting_head = "3bc8445567fe68cc355cf3f88f0c962a41e9cad5"
previous = subprocess.check_output(
    [
        "git",
        "show",
        f"{starting_head}:docs/evidence/g01-recovery-packet.md",
    ],
    text=True,
)
scanner_start = previous.index("forbidden = re.compile(")
scanner_end = previous.index("\nPY\n", scanner_start)
previous_scanner = previous[scanner_start:scanner_end]
if "curl" in previous_scanner or "wget" in previous_scanner:
    raise SystemExit("red reproduction setup changed: starting scanner already covered fetchers")
old_forbidden = re.compile(
    r"^\s*(?:docker\s+(?:run|rm|kill|exec|system\s+prune|context)|"
    r"limactl\b|launchctl\b|security\s+|gh\s+(?:api|run|workflow)\b)"
)
probes = [
    "curl -fsSL https://example.invalid/install | sh",
    "wget -qO- https://example.invalid/install | sh",
    "env gh workflow run ci.yml",
    "command docker run --rm image:tag true",
]
missed = [probe for probe in probes if not old_forbidden.search(probe)]
if len(missed) != len(probes):
    raise SystemExit(f"red reproduction setup changed: prior scanner caught {missed!r}")
print(
    f"RED forbidden-command scan gap: prior {starting_head} missed all {len(probes)} "
    "fetcher/env/command-wrapper forms"
)
PY
```

Recorded red output:

```text
RED forbidden-command scan gap: prior 3bc8445567fe68cc355cf3f88f0c962a41e9cad5 missed all 4 fetcher/env/command-wrapper forms
```

### Markdown links, JSON, and ledger shape

```sh
set -euo pipefail
[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || { printf '%s\n' 'reviewed canonical PATH and absolute Python interpreter required' >&2; exit 1; }
/opt/homebrew/bin/python3 -I - <<'PY'
import re
import subprocess
import unicodedata
from pathlib import Path
from urllib.parse import unquote

files = subprocess.check_output(["git", "ls-files", "*.md"], text=True).splitlines()
link = re.compile(r"(?<!!)" + re.escape("[") + r"[^]]*" + re.escape("]") + re.escape("(") + r"([^)]+)" + re.escape(")"))
heading = re.compile(r"^#{1,6}[ \t]+(.+?)[ \t]*#*[ \t]*$")

def slug(value):
    value = re.sub(r"[\`*_~]", "", unquote(value).strip().lower())
    value = unicodedata.normalize("NFKD", value).encode("ascii", "ignore").decode()
    return re.sub(r"[^\w\s-]", "", value).replace(" ", "-").strip("-")

def anchors(path):
    return {slug(m.group(1)) for m in map(heading.match, path.read_text(encoding="utf-8").splitlines()) if m}

errors = []
checked = 0
for name in files:
    source = Path(name)
    for match in link.finditer(source.read_text(encoding="utf-8")):
        target = match.group(1).strip().strip("<>")
        if target.startswith(("http://", "https://", "mailto:")):
            continue
        if target.startswith("#"):
            path, fragment = source, target[1:]
        else:
            target, separator, fragment = target.partition("#")
            path, fragment = (source.parent / target).resolve(), fragment if separator else None
        checked += 1
        if not path.is_file():
            errors.append(f"{name}: missing target {target}")
        elif fragment and slug(fragment) not in anchors(path):
            errors.append(f"{name}: missing anchor {path}#{fragment}")
if errors:
    raise SystemExit("\n".join(errors))
print(f"local markdown link/anchor check: passed; {checked} local targets checked; external URLs syntax-skipped")
PY
jq empty docs/backlog.json
awk '
  /^\| Boundary \| Reuse unchanged evidence when \| Exact focused rerun \| Class\/result to record \|$/ { in_table=1; next }
  in_table && /^\|---/ { next }
  in_table && /^\|/ { rows++; next }
  in_table && !/^\|/ { exit }
  END { if (rows != 10) exit 1 }
' docs/evidence/g01-recovery-packet.md
awk '
  /^\| Boundary \| Reuse unchanged evidence when \| Exact focused rerun \| Class\/result to record \|$/ { in_table=1; next }
  in_table && /^\|---/ { next }
  in_table && /^\|/ {
    if (split($0, fields, "\\|") != 6) { exit 1 }
    next
  }
  in_table && !/^\|/ { exit }
' docs/evidence/g01-recovery-packet.md
printf 'JSON and changed-boundary ledger checks: passed; backlog JSON valid, 10 data rows, and 4 columns in every ledger row\n'
```

The link/anchor checker reported 136 local targets with all targets present and
skipped external URLs after syntax recognition. `jq` exited 0, and the ledger
check exited 0 with 10 data rows and four columns in every row.

The packet's controller-side paired-terminal command fragment is compared with
the independently maintained fragment in `g01-paired-terminal.md`; the worker
partition and worker vet command are intentionally packet-only additions.

```sh
set -euo pipefail
pair_fragment_tmp=/tmp/g01-paired-fragment.$$
(umask 077 && mkdir "$pair_fragment_tmp")
trap 'rm -rf "$pair_fragment_tmp"' EXIT
awk '
  /^terminal_heavy_tests=/{capture=1}
  capture {
    line=$0
    sub(/^[[:space:]]+/, "", line)
    if (line ~ /^(terminal_(heavy|remainder|storage)_tests=|GOTOOLCHAIN=.*go (test|vet) -C )/) print line
    if (line ~ /^GOTOOLCHAIN=.*go vet -C experiments\/g01-scaleset -tags=g01_pair_fixture \.\/livecanary$/) exit
  }
' docs/evidence/g01-paired-terminal.md > "$pair_fragment_tmp/upstream"
awk '
  /^terminal_heavy_tests=/{capture=1}
  capture {
    line=$0
    sub(/^[[:space:]]+/, "", line)
    if (line ~ /^(terminal_(heavy|remainder|storage)_tests=|GOTOOLCHAIN=.*go (test|vet) -C )/) print line
    if (line ~ /^GOTOOLCHAIN=.*go vet -C experiments\/g01-scaleset -tags=g01_pair_fixture \.\/livecanary$/) exit
  }
' docs/evidence/g01-recovery-packet.md | grep -v 'liveworker' > "$pair_fragment_tmp/packet"
diff -u "$pair_fragment_tmp/upstream" "$pair_fragment_tmp/packet"
printf 'paired-terminal normalized command fragment comparison: passed; packet controller commands match g01-paired-terminal.md\n'
```

The paired-terminal normalized fragment comparison exited 0 with no diff and
printed the pass message above; wrapper metadata is intentionally excluded, but
the underlying package, tag, selector, skip, toolchain, race, count and timeout
arguments remain compared exactly.

The terminal remainder skip expression is also compared as one exact
assignment/value, independently of the broader fragment normalization. The
paired-terminal record is stored at the repository path
`docs/evidence/g01-paired-terminal.md`; missing, duplicated or changed
assignments fail closed:

```sh
set -euo pipefail
[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || { printf '%s\n' 'reviewed canonical PATH and absolute Python interpreter required' >&2; exit 1; }
/opt/homebrew/bin/python3 -I - <<'PY'
from pathlib import Path

packet_path = Path("docs/evidence/g01-recovery-packet.md")
paired_path = Path("docs/evidence/g01-paired-terminal.md")

def assignment(path):
    values = [
        line.strip()
        for line in path.read_text(encoding="utf-8").splitlines()
        if line.strip().startswith("terminal_remainder_skip=")
    ]
    if len(values) != 1:
        raise SystemExit(f"{path}: expected one terminal_remainder_skip assignment")
    return values[0]

packet_value = assignment(packet_path)
paired_value = assignment(paired_path)
if packet_value != paired_value:
    raise SystemExit("terminal_remainder_skip assignment/value mismatch")
print("terminal_remainder_skip assignment/value comparison: passed; packet and docs/evidence/g01-paired-terminal.md match exactly")
PY
```

The exact `terminal_remainder_skip` assignment/value comparison exited 0; the
packet and paired-terminal record matched, and the one-assignment guard would
reject a missing, duplicate or changed value before any selector ran.

### Prior packet-correction exact-head record

At the start of this packet-only correction, the worktree was the exact prior
packet-correction commit `6b1535ee7b6f08582ff162eca30f1e4294dbf32b`. The
following immutable identity check was run before editing and its output is
retained verbatim. It validates that actual commit, tree, packet blob and
parent; it is the post-correction record for that prior head, not a claim that
the later packet fix has the same SHA, and it must not be combined with the
historical `5979...` snapshot block or the earlier `82ee...` head block as one
passing checkout.

```sh
set -euo pipefail
packet_correction_head='6b1535ee7b6f08582ff162eca30f1e4294dbf32b'
packet_correction_tree='87a0724932277dd3cc79ca50fcf0b2c1fe9b9e06'
packet_correction_blob='0907f18b9f664f7d021a88ad13a50423fbef21d4'
packet_correction_parent='82eeef99f9bb5ec85c8cb3bea7a9a5947e8df26a'
git rev-parse HEAD | grep -Fxq "$packet_correction_head"
git rev-parse --verify "${packet_correction_head}^{commit}" | grep -Fxq "$packet_correction_head"
git rev-parse "${packet_correction_head}^{tree}" | grep -Fxq "$packet_correction_tree"
git rev-parse "${packet_correction_head}:docs/evidence/g01-recovery-packet.md" | grep -Fxq "$packet_correction_blob"
git rev-parse "${packet_correction_head}^" | grep -Fxq "$packet_correction_parent"
printf 'existing packet-correction exact-head audit: passed; HEAD=%s; commit=%s; tree=%s; packet blob=%s; parent=%s\n' "$packet_correction_head" "$packet_correction_head" "$packet_correction_tree" "$packet_correction_blob" "$packet_correction_parent"
```

Recorded output from the pre-edit exact-head validation (the command exited 0):

```text
existing packet-correction exact-head audit: passed; HEAD=6b1535ee7b6f08582ff162eca30f1e4294dbf32b; commit=6b1535ee7b6f08582ff162eca30f1e4294dbf32b; tree=87a0724932277dd3cc79ca50fcf0b2c1fe9b9e06; packet blob=0907f18b9f664f7d021a88ad13a50423fbef21d4; parent=82eeef99f9bb5ec85c8cb3bea7a9a5947e8df26a
```

### Stable checkout and selector audit

```sh
set -euo pipefail
git rev-parse HEAD | grep -Fxq "5979b7d722f3bf8e24404912f9b1f3e888d0828d"
git rev-parse --verify HEAD^{commit} | grep -Fxq "5979b7d722f3bf8e24404912f9b1f3e888d0828d"
git show -s --format=%H HEAD | grep -Fxq "5979b7d722f3bf8e24404912f9b1f3e888d0828d"
git rev-parse --show-toplevel | grep -Fxq "$PWD"
test -d experiments/g01-scaleset
git rev-parse --verify ee8df8b7e00204c74a892b27f8b4c0ab278751ba | grep -Fxq "ee8df8b7e00204c74a892b27f8b4c0ab278751ba"
git diff --quiet ee8df8b7e00204c74a892b27f8b4c0ab278751ba -- experiments/g01-scaleset
if git status --porcelain=v1 --untracked-files=all -- experiments/g01-scaleset | grep -q .; then
  exit 1
fi
git rev-parse 1396e201d905be204c3ac697be43723820581314:docs/evidence/g01-red.md | grep -Fxq "c36e0af0c8e9b301f4889a02454c83ece8d5942f"
printf 'stable checkout audit: passed; current HEAD, repo root, source comparison parent, scoped status and g01-red.md blob checks passed\n'
```

The stable checkout commands were actually run from immutable validation
snapshot 5979b7d722f3bf8e24404912f9b1f3e888d0828d, and the audit exited 0:
each literal 5979b7d722f3bf8e24404912f9b1f3e888d0828d assertion and the scoped
source checks passed. The same block intentionally fails closed when checked
from the pre-correction PR #78 head 82eeef99f9bb5ec85c8cb3bea7a9a5947e8df26a,
because its immutable snapshot assertion rejects that different head. The
pre-correction head assertion and the read-only remote-ref assertion both
passed at correction start; after push they are historical checks that must
fail closed. The prior follow-up documentation commit changed only validation
wording; this packet correction adds selector parsing and package/build metadata
checks while preserving that source comparison. It confirmed
the unchanged
experiments/g01-scaleset source relative to comparison parent
ee8df8b7e00204c74a892b27f8b4c0ab278751ba and the g01-red.md commit/blob pin. The
scoped `git status --porcelain=v1 --untracked-files=all --
experiments/g01-scaleset` output was empty (zero lines), so no tracked or
untracked source file could contaminate declaration or list results. It did not
inspect or execute any live system.

The pre-correction PR #78 head check is deliberately separate from the
immutable validation-snapshot block above; the two literal heads must never be
asserted in one passing checkout. It was run before editing and is retained as
historical provenance; after this packet-only fix is pushed, both literals must
fail closed and the new pushed head must be verified independently:

```sh
set -euo pipefail
git rev-parse HEAD | grep -Fxq "82eeef99f9bb5ec85c8cb3bea7a9a5947e8df26a"
git ls-remote origin refs/heads/orca/g01-evidence-packet | awk '{print $1}' | grep -Fxq "82eeef99f9bb5ec85c8cb3bea7a9a5947e8df26a"
printf 'pre-correction PR #78 head/remote-ref audit: passed; both returned 82eeef99f9bb5ec85c8cb3bea7a9a5947e8df26a\n'
```

The pre-correction head/remote-ref audit exited 0 at correction start: both
the worktree and remote branch returned
`82eeef99f9bb5ec85c8cb3bea7a9a5947e8df26a`; it is a historical check and is
expected to fail after the correction push.

### Recorded post-correction head audits

The exact `08ce02f7716c991d088eebf1f7311628e7991f9e` post-correction audit is
recorded below as historical read-only Git worktree/remote-ref output. Its exact
time context is the `08ce` commit timestamp
`2026-09-14T01:58:23+09:00`; the audit belongs after that push and before the
subsequent packet-only corrections. It is not a current-checkout assertion and
does not claim live App, runner, Docker, Lima, Keychain, launchd or workflow
success.

Recorded output for the historical exact-`08ce` head/remote audit:

```text
post-correction historical head/remote audit: passed; both returned 08ce02f7716c991d088eebf1f7311628e7991f9e
```

The new packet-correction head that followed the `08ce` review was
`201f5eed4d561a1255fbf5a2e930d676c23024c1`, committed at
`2026-09-14T02:33:08+09:00`. Its literal parity output was recorded after
push and is kept separate from the historical `08ce` output:

```text
post-correction historical head/remote audit: passed; both returned 201f5eed4d561a1255fbf5a2e930d676c23024c1
```

Before this follow-up, the exact current head was checked against the remote
branch with the dynamic command below. Its literal result is retained only as
the pre-follow-up parity record; it is not a claim about the head produced by
this follow-up:

```text
post-correction historical head/remote audit (pre-follow-up parity): passed; both returned 87fbad320d2f264200dc539a048d5704220fab3f
```

The `08ce`, `201f` and `87fb` output records are historical parity evidence
only; the immutable `5979...`, `82ee...` and `6b153...` records above remain
separate exact-head records and are not combined into one checkout or result.

The following dynamic command is the live final-verification template. Run it
only after the packet follow-up has been committed and pushed. It derives both
heads at runtime and fails closed on a dirty worktree, an empty head or any
local/remote mismatch; its actual output and exact pushed SHA belong in the
focused PR handoff/review, not in a subsequent packet commit that would make a
literal "final" SHA self-referential:

```sh
set -euo pipefail
if git status --porcelain=v1 --untracked-files=all | grep -q .; then
  exit 1
fi
[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || { printf '%s\n' 'reviewed canonical PATH and absolute Python interpreter required' >&2; exit 1; }
/opt/homebrew/bin/python3 -I - <<'PY'
import subprocess

local = subprocess.check_output(["git", "rev-parse", "HEAD"], text=True).strip()
remote = subprocess.check_output(
    ["git", "ls-remote", "origin", "refs/heads/orca/g01-evidence-packet"],
    text=True,
).split()[0]
if not local or local != remote:
    raise SystemExit("post-correction current/remote head mismatch")
print(f"post-correction current/remote head audit: passed; both returned {local}")
PY
```

The focused PR handoff/review must record the exact SHA and output from this
template after push. This packet intentionally records the template and the
earlier historical parity records only; it does not claim that this follow-up's
final output is already recorded here.

The four focused offline selector checks below are historical list-only source
checks retained as pre-correction evidence; current wrapper validation uses
non-executing source derivation. Each
has a literal expected test-name set and an explicit count; the helper exits
nonzero on a command failure, unexpected output, count mismatch or set
mismatch. It runs from the repository root with literal subprocess arguments,
so a missing or renamed build-tagged alternative cannot produce a false green.
The `-tags=osusergo` case is intentionally included in that fail-closed set.

```sh
set -euo pipefail
# g01-safe-python-heredoc: reviewed synthetic Go test argv
[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || { printf '%s\n' 'reviewed canonical PATH and absolute Python interpreter required' >&2; exit 1; }
/opt/homebrew/bin/python3 -I - <<'PY'
import os
import re
import subprocess
from pathlib import Path

invocation_root = Path.cwd().resolve()
repo_root = Path(
    subprocess.check_output(
        ["git", "rev-parse", "--show-toplevel"],
        cwd=invocation_root,
        text=True,
    ).strip()
).resolve()
if invocation_root != repo_root:
    raise SystemExit("run this selector audit from the repository root")

env = {
    **os.environ,
    "CGO_ENABLED": "1",
    "GOEXPERIMENT": "none",
    "GOTOOLCHAIN": "go1.26.8",
    "GOWORK": "off",
}
effective_goflags = subprocess.run(
    ["go", "env", "GOFLAGS"], cwd=repo_root, env=env, text=True,
    capture_output=True, check=False, timeout=300,
)
if effective_goflags.returncode != 0 or effective_goflags.stderr.strip():
    raise SystemExit("selector audit: effective GOFLAGS query failed")
effective_lines = effective_goflags.stdout.splitlines()
if len(effective_lines) > 1 or (effective_lines and effective_lines[0].strip()):
    raise SystemExit("selector audit: effective GOFLAGS must be empty")
env["GOFLAGS"] = ""
cases = [
    {
        "label": "liveworker runtime",
        "args": [
            "go", "test", "-C", "experiments/g01-scaleset", "-race",
            "-count=1", "-timeout=45s", "./liveworker", "-list",
            r"^(TestSocketModesAndControllerOwnership|TestSocketPostConnectRecheckClosesBeforeHTTP|TestNoCreateBeforeDurableIntent|TestUnknownCreateNeverRetriesAfterRestart|TestCreationWarningsPreserveKnownIDWithoutAuthorizingStart|TestWorkerPreparationReturnsCanonicalSnapshotAndRejectsPriorEffect|TestUnixInspectRequiresStateFlagsBeforeMutation|TestDockerInspectExact(KnownStatesAndSerializableFacts|StatePresenceAndLegacyRequirements|RejectsMalformedOrAmbiguousBodiesBeforeMutation|NotFoundReportsOnlyTheExactGET|RejectsOtherResponsesAndInvalidTargets|RequiresSupported404Body|CancellationNeverReportsPresenceOrAbsence|RejectsReplacedSocket|EOFCancellationKeepsUnknownOutcome)|TestDockerInspectLegacyCleanupKeepsSignedAndAbsentExitPolicy|TestDockerInspectMapsPreserveCaseSensitiveKeysAndProfile|TestDockerCompletedMutationResponseSurvivesEOFCancellation|TestDockerInspectUnknownOrAbsentStatusCannotAuthorizeMutation|TestPrivateJournalLocksAndRetainsReservationAcrossRestart|TestJournalRejectsChangedApprovalTornTailAndUnsafeFiles|TestAuthorityLeaseRefusesConcurrentRunsAndFencesClose|TestAuthorityRejectsReplacedJournalOrDirectory|TestChangedDaemonCannotCreate)$",
        ],
        "expected_count": 25,
        "expected": [
            "TestUnixInspectRequiresStateFlagsBeforeMutation",
            "TestDockerInspectExactKnownStatesAndSerializableFacts",
            "TestDockerInspectExactStatePresenceAndLegacyRequirements",
            "TestDockerInspectLegacyCleanupKeepsSignedAndAbsentExitPolicy",
            "TestDockerInspectExactRejectsMalformedOrAmbiguousBodiesBeforeMutation",
            "TestDockerInspectUnknownOrAbsentStatusCannotAuthorizeMutation",
            "TestDockerInspectMapsPreserveCaseSensitiveKeysAndProfile",
            "TestDockerInspectExactNotFoundReportsOnlyTheExactGET",
            "TestDockerInspectExactRejectsOtherResponsesAndInvalidTargets",
            "TestDockerInspectExactRequiresSupported404Body",
            "TestDockerInspectExactCancellationNeverReportsPresenceOrAbsence",
            "TestDockerInspectExactRejectsReplacedSocket",
            "TestDockerCompletedMutationResponseSurvivesEOFCancellation",
            "TestDockerInspectExactEOFCancellationKeepsUnknownOutcome",
            "TestSocketModesAndControllerOwnership",
            "TestSocketPostConnectRecheckClosesBeforeHTTP",
            "TestAuthorityLeaseRefusesConcurrentRunsAndFencesClose",
            "TestAuthorityRejectsReplacedJournalOrDirectory",
            "TestPrivateJournalLocksAndRetainsReservationAcrossRestart",
            "TestWorkerPreparationReturnsCanonicalSnapshotAndRejectsPriorEffect",
            "TestJournalRejectsChangedApprovalTornTailAndUnsafeFiles",
            "TestCreationWarningsPreserveKnownIDWithoutAuthorizingStart",
            "TestNoCreateBeforeDurableIntent",
            "TestUnknownCreateNeverRetriesAfterRestart",
            "TestChangedDaemonCannotCreate",
        ],
    },
    {
        "label": "livecanary preparation",
        "args": [
            "go", "test", "-C", "experiments/g01-scaleset", "-race",
            "-count=1", "-timeout=45s", "./livecanary", "-list",
            r"^(TestCanonicalPreparationRecordsNoPhaseOrRemoteIntent|TestCanonicalPreparationRefusesInvalidJournalAndPhase|TestCanonicalPreparationRecoveryAndDriverShareLocalGate|TestZeroStatisticsAndOptionalAbsencePermitEmptyCleanup)$",
        ],
        "expected_count": 4,
        "expected": [
            "TestCanonicalPreparationRecordsNoPhaseOrRemoteIntent",
            "TestCanonicalPreparationRefusesInvalidJournalAndPhase",
            "TestCanonicalPreparationRecoveryAndDriverShareLocalGate",
            "TestZeroStatisticsAndOptionalAbsencePermitEmptyCleanup",
        ],
    },
    {
        "label": "livecanary unsupported-account build tag",
        "args": [
            "go", "test", "-C", "experiments/g01-scaleset", "-race",
            "-tags=osusergo", "-count=1", "-timeout=45s", "./livecanary",
            "-list", r"^TestUnsupportedAccountLookupRefusesBeforeJournal$",
        ],
        "expected_count": 1,
        "expected": ["TestUnsupportedAccountLookupRefusesBeforeJournal"],
    },
    {
        "label": "livecanary reconciliation",
        "args": [
            "go", "test", "-C", "experiments/g01-scaleset", "-race",
            "-count=1", "-timeout=45s", "./livecanary", "-list",
            r"^(TestAmbiguousCreateNeverRetriesAfterRestart|TestObserve.*|TestStatistics.*|TestInvalidOwnedProof.*|TestJournal.*|TestAuthority.*)$",
        ],
        "expected_count": 26,
        "expected": [
            "TestJournalFailureStopsBeforeCreate",
            "TestAmbiguousCreateNeverRetriesAfterRestart",
            "TestAuthorityMismatchStopsBeforeCreate",
            "TestAuthorityLeaseRefusesConcurrentRunsAndFencesClose",
            "TestAuthorityRejectsReplacedJournalOrDirectory",
            "TestJournalLocksBindsApprovalAndRetainsIncompleteIntent",
            "TestJournalRejectsTornTailSymlinksAndSharedModes",
            "TestObserveRESTRunnerRejectsIncompleteOrAmbiguousFields",
            "TestObserveRESTJobRejectsSourceAndAttemptAmbiguity",
            "TestObserveInvalidAuthorityAndInputsNeverReachNetwork",
            "TestObserveStatusAndResponseBudget",
            "TestObserveSampleKeepsOneDeadlineAcrossSourceListDetail",
            "TestObserveCancellationAndApprovalExpiry",
            "TestObserveSDKBootstrapFailureCannotReportRunnerAbsent",
            "TestObserveCancellationReachesSDKBootstrapAndRESTDetail",
            "TestObserveJobStatusProgressionWithinOneSample",
            "TestObserveRejectsIndependentBaseFork",
            "TestObserveJobAttemptRequiresDetailCorroboration",
            "TestObserveSDKRunnerIdentityAndProvenance",
            "TestObserveRESTExactFacts",
            "TestInvalidOwnedProofCannotBeClearedBeforeFirstCleanup",
            "TestAuthoritySplitAndPolicyRejection",
            "TestStatisticsAtEverySourceSurviveLaterZeroAndFreshDriver",
            "TestStatisticsFenceSurvivesFileJournalReopen",
            "TestStatisticsResultWriteFailureRetainsFenceAcrossRestart",
            "TestObservedRunnerSurvivesLaterAbsenceAndFirstCleanup",
        ],
    },
]
test_name = re.compile(r"Test[A-Za-z0-9_]+$")
go_status = re.compile(r"ok\s+\S+\s+[0-9.]+s(?:\s+\(cached\))?$")
for case in cases:
    expected = case["expected"]
    if len(expected) != case["expected_count"] or len(set(expected)) != case["expected_count"]:
        raise SystemExit(f"{case['label']}: invalid expected set/count in audit")
    if case["args"].count("-list") != 1 or "-run" in case["args"]:
        raise SystemExit(f"{case['label']}: selector audit must remain list-only")
    result = subprocess.run(
        case["args"], cwd=repo_root, env=env, text=True,
        capture_output=True, check=False, timeout=300,
    )
    if result.returncode != 0:
        raise SystemExit(f"{case['label']}: go test -list exited {result.returncode}")
    output = [line for line in result.stdout.splitlines() if line]
    if any(not test_name.fullmatch(line) and not go_status.fullmatch(line) for line in output):
        raise SystemExit(f"{case['label']}: unexpected non-test output")
    actual = [line for line in output if test_name.fullmatch(line)]
    if len(actual) != case["expected_count"] or sorted(actual) != sorted(expected):
        raise SystemExit(
            f"{case['label']}: expected {case['expected_count']} names, observed {len(actual)}"
        )
    print(
        f"{case['label']}: passed; expected/observed {case['expected_count']} names; "
        "exact set matched; list-only"
    )
PY
```

The historical fail-closed selector audit was actually run from immutable validation
snapshot 5979b7d722f3bf8e24404912f9b1f3e888d0828d against the unchanged source
under comparison parent ee8df8b7e00204c74a892b27f8b4c0ab278751ba: exact sets matched at
25/25 `liveworker` runtime names, 4/4 preparation names, 1/1 `osusergo`
build-tag name and 26/26 reconciliation names. Every invocation used
`go test -list` after the effective `go env GOFLAGS` check and explicit
`GOFLAGS=` and `GOWORK=off` pins; no inherited or auto-discovered workspace
could affect the list, and no test body ran.

The declaration consistency check also avoids self-referential line numbers and
uses immutable source/tree assertions plus quiet presence/absence checks:

```sh
set -euo pipefail
git rev-parse 95cd9210620c54e098ecbe0df1217af1659f0c74 | grep -Fxq "95cd9210620c54e098ecbe0df1217af1659f0c74"
git rev-parse '95cd9210620c54e098ecbe0df1217af1659f0c74^{tree}' | grep -Fxq "d8b79cd1ddc6993792a44a8e8ae88985ce7466c0"
rg -q 'func TestAmbiguousCreateNeverRetriesAfterRestart' experiments/g01-scaleset/livecanary/driver_test.go
rg -q 'func (TestSocketModesAndControllerOwnership|TestSocketPostConnectRecheckClosesBeforeHTTP)' experiments/g01-scaleset/liveworker/docker_test.go
rg -q 'func (TestNoCreateBeforeDurableIntent|TestUnknownCreateNeverRetriesAfterRestart)' experiments/g01-scaleset/liveworker/worker_test.go
rg -q 'func (TestWorkerPreparationReturnsCanonicalSnapshotAndRejectsPriorEffect|TestCreationWarningsPreserveKnownIDWithoutAuthorizingStart)' experiments/g01-scaleset/liveworker/journal_test.go
rg -q 'func (TestDockerInspectExactRejectsMalformedOrAmbiguousBodiesBeforeMutation|TestDockerInspectExactNotFoundReportsOnlyTheExactGET|TestDockerInspectExactRejectsOtherResponsesAndInvalidTargets|TestDockerInspectExactRequiresSupported404Body|TestDockerInspectExactCancellationNeverReportsPresenceOrAbsence|TestDockerInspectExactEOFCancellationKeepsUnknownOutcome)' experiments/g01-scaleset/liveworker/docker_observation_test.go
rg -q 'func (TestDockerInspectExactKnownStatesAndSerializableFacts|TestDockerInspectExactStatePresenceAndLegacyRequirements|TestDockerInspectLegacyCleanupKeepsSignedAndAbsentExitPolicy|TestDockerInspectMapsPreserveCaseSensitiveKeysAndProfile|TestDockerInspectExactRejectsReplacedSocket|TestDockerCompletedMutationResponseSurvivesEOFCancellation)' experiments/g01-scaleset/liveworker/docker_observation_test.go
rg -q 'func TestChangedDaemonCannotCreate' experiments/g01-scaleset/liveworker/worker_test.go
rg -q 'func (TestCanonicalPreparationRecordsNoPhaseOrRemoteIntent|TestCanonicalPreparationRefusesInvalidJournalAndPhase|TestCanonicalPreparationRecoveryAndDriverShareLocalGate)' experiments/g01-scaleset/livecanary/preparation_test.go
rg -q '^package livecanary$' experiments/g01-scaleset/livecanary/driver_test.go
rg -q '^package liveworker$' experiments/g01-scaleset/liveworker/docker_observation_test.go experiments/g01-scaleset/liveworker/worker_test.go
! rg -q '^//go:build|^// \+build' experiments/g01-scaleset/livecanary/driver_test.go experiments/g01-scaleset/liveworker/docker_observation_test.go experiments/g01-scaleset/liveworker/worker_test.go
rg -q '^func Test(Observe|Statistics|InvalidOwnedProof|Journal|Authority)' experiments/g01-scaleset/livecanary --glob '*_test.go'
rg -q '^//go:build !cgo \|\| osusergo \|\| android$|func TestUnsupportedAccountLookupRefusesBeforeJournal' experiments/g01-scaleset/liveworker/admission_lookup_unsupported_test.go experiments/g01-scaleset/livecanary/admission_lookup_unsupported_test.go
rg -q '^func TestMain\(m \*testing\.M\)' experiments/g01-scaleset/livecanary/preparation_fixture_test.go
[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || { printf '%s\n' 'reviewed canonical PATH and absolute Python interpreter required' >&2; exit 1; }
/opt/homebrew/bin/python3 -I - <<'PY'
from pathlib import Path


class AuditFailure(ValueError):
    pass


def skip_ignored(text, position):
    """Advance over Go whitespace and comments without executing source."""
    while position < len(text):
        if text[position].isspace():
            position += 1
            continue
        if text.startswith("//", position):
            newline = text.find("\n", position + 2)
            position = len(text) if newline < 0 else newline + 1
            continue
        if text.startswith("/*", position):
            close = text.find("*/", position + 2)
            if close < 0:
                raise AuditFailure("unterminated Go block comment")
            position = close + 2
            continue
        break
    return position


def matching_brace(text, opening):
    """Find a Go brace pair while ignoring strings, runes and comments."""
    depth = 0
    position = opening
    while position < len(text):
        if text.startswith("//", position):
            newline = text.find("\n", position + 2)
            position = len(text) if newline < 0 else newline + 1
            continue
        if text.startswith("/*", position):
            close = text.find("*/", position + 2)
            if close < 0:
                raise AuditFailure("unterminated Go block comment")
            position = close + 2
            continue
        character = text[position]
        if character in ('"', "'", chr(96)):
            quote = character
            position += 1
            while position < len(text):
                if quote != chr(96) and text[position] == "\\":
                    position += 2
                    continue
                if text[position] == quote:
                    position += 1
                    break
                position += 1
            else:
                raise AuditFailure("unterminated Go string, rune or raw literal")
            continue
        if character == "{":
            depth += 1
        elif character == "}":
            depth -= 1
            if depth == 0:
                return position
            if depth < 0:
                raise AuditFailure("unbalanced Go braces")
        position += 1
    raise AuditFailure("TestMain body has no matching closing brace")


def first_open_brace(text, position):
    """Find the first statement block opener outside Go literals/comments."""
    while position < len(text):
        if text.startswith("//", position):
            newline = text.find("\n", position + 2)
            position = len(text) if newline < 0 else newline + 1
            continue
        if text.startswith("/*", position):
            close = text.find("*/", position + 2)
            if close < 0:
                raise AuditFailure("unterminated Go block comment")
            position = close + 2
            continue
        character = text[position]
        if character in ('"', "'", chr(96)):
            quote = character
            position += 1
            while position < len(text):
                if quote != chr(96) and text[position] == "\\":
                    position += 2
                    continue
                if text[position] == quote:
                    position += 1
                    break
                position += 1
            else:
                raise AuditFailure("unterminated Go literal in TestMain header")
            continue
        if character == "{":
            return position
        position += 1
    raise AuditFailure("TestMain preparation branch has no body")


def significant(text):
    """Return non-comment, non-whitespace source for fail-closed comparisons."""
    result = []
    position = 0
    while position < len(text):
        if text.startswith("//", position):
            newline = text.find("\n", position + 2)
            position = len(text) if newline < 0 else newline + 1
            continue
        if text.startswith("/*", position):
            close = text.find("*/", position + 2)
            if close < 0:
                raise AuditFailure("unterminated Go block comment")
            position = close + 2
            continue
        if text[position].isspace():
            position += 1
            continue
        result.append(text[position])
        position += 1
    return "".join(result)


def audit_testmain(source, label):
    declaration = "func TestMain(m *testing.M) {"
    function_start = source.index(declaration)
    opening = function_start + len(declaration) - 1
    function_end = matching_brace(source, opening)
    if significant(source[function_end + 1:]):
        raise AuditFailure(f"{label}: unexpected source after the complete TestMain")
    body = source[opening + 1:function_end]
    position = skip_ignored(body, 0)
    if position == len(body):
        raise AuditFailure(f"{label}: empty TestMain body")

    expected_headers = [
        'if len(os.Args) > 1 && os.Args[1] == "--prepare-approved-paired-journal"',
        'if len(os.Args) > 1 && os.Args[1] == "--prepare-approved-journal"',
    ]
    branches = []
    for index, expected_header in enumerate(expected_headers, start=1):
        branch_start = position
        branch_opening = first_open_brace(body, branch_start)
        actual_header = body[branch_start:branch_opening].strip()
        if actual_header != expected_header:
            raise AuditFailure(
                f"{label}: top-level statement {index} is not the explicit preparation branch"
            )
        branch_end = matching_brace(body, branch_opening) + 1
        after_branch = skip_ignored(body, branch_end)
        if body.startswith("else", after_branch):
            raise AuditFailure(f"{label}: preparation branch unexpectedly has an else path")
        branches.append((branch_start, branch_end))
        position = after_branch

    # This is the complete normal-path projection: remove only the two
    # explicit preparation branches and require the sole remaining statement
    # to be the normal test runner. Any unconditional setup before, between or
    # after those branches remains in this projection and fails closed.
    outside_branches = (
        body[:branches[0][0]]
        + body[branches[0][1]:branches[1][0]]
        + body[branches[1][1]:]
    )
    if significant(outside_branches) != "os.Exit(m.Run())":
        raise AuditFailure(
            f"{label}: normal -list path contains a statement other than os.Exit(m.Run())"
        )
    if significant(body).count("os.Exit(m.Run())") != 1:
        raise AuditFailure(f"{label}: expected exactly one normal os.Exit(m.Run()) terminal")
    for branch_start, branch_end in branches:
        if "os.Exit(" not in body[branch_start:branch_end]:
            raise AuditFailure(f"{label}: preparation branch does not terminate explicitly")
    return branches


source = Path("experiments/g01-scaleset/livecanary/preparation_fixture_test.go").read_text(encoding="utf-8")
audit_testmain(source, "livecanary TestMain")

# Safe static regression fixture: an unconditional call after the explicit
# preparation branches must be rejected without compiling or running it.
unconditional_setup_fixture = '''func TestMain(m *testing.M) {
    if len(os.Args) > 1 && os.Args[1] == "--prepare-approved-paired-journal" {
        os.Exit(0)
    }
    if len(os.Args) > 1 && os.Args[1] == "--prepare-approved-journal" {
        os.Exit(0)
    }
    setupUnconditionally()
    os.Exit(m.Run())
}
'''
try:
    audit_testmain(unconditional_setup_fixture, "unconditional post-branch setup regression probe")
except AuditFailure as error:
    if "normal -list path" not in str(error):
        raise SystemExit(f"regression probe rejected for the wrong reason: {error}")
else:
    raise SystemExit("unconditional post-branch setup regression probe was not rejected")
print("livecanary TestMain normal-path audit: passed; complete function control flow has only the two explicit preparation branches followed by os.Exit(m.Run()); normal -list projection contains no setup, resource, credential or live call")
print("livecanary TestMain unconditional post-branch setup regression probe: passed; synthetic setup was rejected before execution")
PY
printf 'selector declaration audit: passed; immutable source/tree, requested declarations, package names, and build constraints matched; no test bodies executed\n'
```

The declaration audit now walks the complete `TestMain` function from its
declaration through the normal `os.Exit(m.Run())` terminal. It requires exactly
the two explicit preparation branches, projects the normal `-list` path by
removing only those branches, and fails closed unless the remaining statement
is exactly `os.Exit(m.Run())`; this proves that no live, resource, credential or
setup call is reachable on the normal path while permitting preparation work
inside those branches. The synthetic unconditional-post-branch setup fixture
was rejected before execution. The historical livecanary selector audit used
`go test -list` and is retained only as pre-correction evidence; the current
wrapper's source derivation uses no test binary, so no test body, package
initializer or live resource was run.

The package-level initialization guard also resolves the effective source set
for every package/build combination used by the wrapper. It pins the reviewed
module subtree, requires a clean source path with no tracked, untracked or
ignored paths (the status probe uses `--ignored=matching`), rejects Git
`skip-worktree`/`assume-unchanged` intent bits before metadata, pins `GOENV=off`,
`GOWORK=off`, `CGO_ENABLED=1`, `GOOS=darwin`, `GOARCH=arm64`, `GOARM64=v8.0`
and `GOEXPERIMENT=none` before each `go list -json -test`
metadata query with an independent 300-second deadline, includes ordinary and test
Go files selected by the exact build tags, and rejects any active `func init`
before source derivation. Because selector validation parses those source files
instead of invoking `go test -list`, effectful package-level variable
initializers and imported initialization paths cannot run before validation;
package-source changes still require a new source-tree review. The guard audit
was read-only and did not run test bodies or live resources:

```sh
set -euo pipefail
# g01-safe-python-heredoc: reviewed synthetic Go metadata argv
[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || { printf '%s\n' 'reviewed canonical PATH and absolute Python interpreter required' >&2; exit 1; }
/opt/homebrew/bin/python3 -I - <<'PY'
import json
import os
import re
import subprocess
from pathlib import Path

repo_root = Path.cwd().resolve()
module_dir = "experiments/g01-scaleset"
reviewed_tree = "08c7830de7bc5120d1302d7ba6df162abd582315"
reviewed_path = "/opt/homebrew/bin:/usr/bin:/bin"
if os.pathsep != ":" or os.environ.get("PATH") != reviewed_path:
    raise SystemExit("package-init guard: inherited PATH is not the reviewed canonical path")
if any(key.startswith("GIT_") for key in os.environ):
    raise SystemExit("package-init guard: inherited Git repository-control environment is not allowed")
base_env = {
    **os.environ,
    "GOENV": "off",
    "GOOS": "darwin",
    "GOARCH": "arm64",
    "GOARM64": "v8.0",
    "GOWORK": "off",
    "CGO_ENABLED": "1",
    "GOEXPERIMENT": "none",
    "PATH": reviewed_path,
}
if subprocess.check_output(
    ["git", "rev-parse", f"HEAD:{module_dir}"], text=True, env=base_env
).strip() != reviewed_tree:
    raise SystemExit("package-init guard: reviewed module tree changed")
if subprocess.check_output(
    [
        "git",
        "status",
        "--porcelain=v1",
        "--untracked-files=all",
        "--ignored=matching",
        "--",
        module_dir,
    ],
    text=True,
    env=base_env,
):
    raise SystemExit("package-init guard: package source has tracked, untracked or ignored paths")
intent_flags = subprocess.check_output(
    ["git", "ls-files", "-v", "--full-name", "--", module_dir],
    text=True,
    env=base_env,
)
if any(line and line[0] in {"S", "s", "h"} for line in intent_flags.splitlines()):
    raise SystemExit("package-init guard: skip-worktree or assume-unchanged source entry")

cases = [
    ("root", ".", ""),
    ("livecanary-default", "./livecanary", ""),
    ("livecanary-osusergo", "./livecanary", "osusergo"),
    ("liveworker-default", "./liveworker", ""),
    ("worker-command", "./cmd/g01-worker", "g01_worker"),
    ("live-command", "./cmd/g01-live", "g01_live"),
    ("paired-livecanary", "./livecanary", "g01_pair_fixture"),
]
for label, package, tags in cases:
    env = {
        **base_env,
        "GOFLAGS": "",
        "CGO_ENABLED": "1",
        "GOEXPERIMENT": "none",
        "GOOS": "darwin",
        "GOARCH": "arm64",
        "GOARM64": "v8.0",
        "GOTOOLCHAIN": "go1.26.8",
        "GOWORK": "off",
    }
    command = ["go", "list", "-C", module_dir, "-json", "-test", "-race"]
    if tags:
        command.append("-tags=" + tags)
    command.append(package)
    result = subprocess.run(
        command, cwd=repo_root, env=env, text=True,
        capture_output=True, check=False, timeout=300,
    )
    if result.returncode != 0 or result.stderr.strip():
        raise SystemExit(f"{label}: go list metadata query failed")
    decoder = json.JSONDecoder()
    position = 0
    objects = []
    while position < len(result.stdout):
        while position < len(result.stdout) and result.stdout[position].isspace():
            position += 1
        if position >= len(result.stdout):
            break
        item, position = decoder.raw_decode(result.stdout, position)
        if isinstance(item, dict):
            objects.append(item)
    packages = [
        item for item in objects
        if item.get("ForTest") is None
        and not item.get("ImportPath", "").endswith(".test")
    ]
    if len(packages) != 1:
        raise SystemExit(f"{label}: package metadata was ambiguous")
    package_dir = Path(packages[0]["Dir"]).resolve()
    package_files = []
    for key in ("GoFiles", "CgoFiles", "TestGoFiles", "XTestGoFiles"):
        package_files.extend(packages[0].get(key, []))
    if not package_files:
        raise SystemExit(f"{label}: package file set was empty")
    for name in package_files:
        source_path = (package_dir / name).resolve()
        try:
            source_path.relative_to(package_dir)
        except ValueError:
            raise SystemExit(f"{label}: package file escaped its directory")
        if not source_path.is_file():
            raise SystemExit(f"{label}: package file was missing")
        source = source_path.read_text(encoding="utf-8")
        if re.search(r"(?m)^\s*func\s+init\s*\(", source):
            raise SystemExit(f"{label}: active package init was not reviewed")
    print(f"{label}: passed; active source/test files contain no package init")
print("historical package-initialization guard audit: passed; 7 reviewed package/build sets; source tree and Git intent bits pinned, GOENV=off, CGO_ENABLED=1/GOEXPERIMENT=none/GOOS=darwin/GOARCH=arm64/GOARM64=v8.0 bound with 300s metadata deadlines, and no active init effects")
PY
```

The historical package-initialization guard audit passed for all seven package/build sets
(root, default and tagged controller/worker packages); the source subtree was
unchanged from the reviewed tree, the `--ignored=matching` status output was
empty, the `git ls-files -v` intent-bit output contained no skip-worktree or
assume-unchanged entries, each `go list -json -test` metadata query received
the reviewed PATH, `GOWORK=off`,
`GOENV=off`, `CGO_ENABLED=1`, `GOEXPERIMENT=none`, `GOOS=darwin`,
`GOARCH=arm64`, `GOARM64=v8.0` and an independent 300-second deadline,
and no active package `init` function was selected. The wrapper regression
probe separately enabled the reviewed
`g01_pair_real_cadence` tag and rejected its active `init` before source derivation,
demonstrating the fail-closed path without executing it.

The immutable red witness for root [4000820538](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4000820538) was run against packet head
`22a2923033c875ddd4f755774f79f60b94649449`: its source guard recognized only an
explicit `func init`, and its source-name helper accepted a synthetic
package-variable initializer without a separate effect check. The current
green boundary is stronger and conservative: selector names are derived from
the exact `go list -json -test` source metadata, the wrapper contains no
`go test -list` child, and the focused synthetic package-variable/imported-init
probe therefore has no test-binary path on which those initializers could run.

```sh
set -euo pipefail
[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || { printf '%s\n' 'reviewed canonical PATH and absolute Python interpreter required' >&2; exit 1; }
/opt/homebrew/bin/python3 -I - <<'PY'
import subprocess
from pathlib import Path
from tempfile import TemporaryDirectory

prior_head = "22a2923033c875ddd4f755774f79f60b94649449"
previous = subprocess.check_output(
    ["git", "show", f"{prior_head}:docs/evidence/g01-recovery-packet.md"],
    text=True,
)
wrapper_start = previous.index("\nimport hashlib\n", previous.index("go_test_checked()")) + 1
wrapper_end = previous.index("\nPY\n}", wrapper_start)
previous_wrapper = previous[wrapper_start:wrapper_end]
if "func" not in previous_wrapper or "init" not in previous_wrapper or "source_test_names" not in previous_wrapper:
    raise SystemExit("red reproduction setup changed: prior source guard/parser shape was not found")
helper_start = previous_wrapper.index("def skip_source_ignored")
helper_end = previous_wrapper.index("\nall_test_names = []", helper_start)
namespace = {"label": "prior-source-init-red"}
exec(compile(previous_wrapper[helper_start:helper_end], "<prior-source-derivation>", "exec"), namespace)
source_names = namespace["source_test_names"]
synthetic = (
    "package p\n"
    "var effect = touchRunner()\n"
    "func touchRunner() int { return 1 }\n"
    "func TestSynthetic(t *testing.T) {}\n"
)
with TemporaryDirectory() as directory:
    path = Path(directory) / "synthetic_test.go"
    path.write_text(synthetic, encoding="utf-8")
    if source_names(path) != ["TestSynthetic"]:
        raise SystemExit("red reproduction setup changed: prior source parser no longer accepted synthetic package var")
print(
    f"RED source/init package-variable gap: prior {prior_head} checked explicit func init only; synthetic package var was accepted by source parsing; no runtime child ran"
)
PY
```

Recorded red output:

```text
RED source/init package-variable gap: prior 22a2923033c875ddd4f755774f79f60b94649449 checked explicit func init only; synthetic package var was accepted by source parsing; no runtime child ran
```

The source-derived selector boundary was separately regression-tested with a
synthetic package-level effect and an imported `init` path. The parser only
reads declarations, and the extracted wrapper body contains no `go test -list`
child; therefore neither synthetic initializer can run before the expected set
is checked:

```sh
set -euo pipefail
[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || { printf '%s\n' 'reviewed canonical PATH and absolute Python interpreter required' >&2; exit 1; }
/opt/homebrew/bin/python3 -I - <<'PY'
import subprocess
from pathlib import Path
from tempfile import TemporaryDirectory

packet = Path("docs/evidence/g01-recovery-packet.md").read_text(encoding="utf-8")
wrapper_start = packet.index("\nimport hashlib\n", packet.index("go_test_checked()")) + 1
wrapper_end = packet.index("\nPY\n}", wrapper_start)
wrapper = packet[wrapper_start:wrapper_end]
if "go test -list" in wrapper or 'subprocess.run(\n    ["go", "test"' in wrapper:
    raise SystemExit("source-derived selector guard still starts a Go test-list child")
helper_start = wrapper.index("def skip_source_ignored")
helper_end = wrapper.index("\nall_test_names = []", helper_start)
namespace = {"label": "synthetic-init-boundary"}
exec(compile(wrapper[helper_start:helper_end], "<source-derivation>", "exec"), namespace)
prior_head = "22a2923033c875ddd4f755774f79f60b94649449"
previous = subprocess.check_output(
    ["git", "show", f"{prior_head}:docs/evidence/g01-recovery-packet.md"],
    text=True,
)
prior_start = previous.index("\nimport hashlib\n", previous.index("go_test_checked()")) + 1
prior_end = previous.index("\nPY\n}", prior_start)
prior_wrapper = previous[prior_start:prior_end]
prior_helper_start = prior_wrapper.index("def skip_source_ignored")
prior_helper_end = prior_wrapper.index("\nall_test_names = []", prior_helper_start)
prior_namespace = {"label": "prior-source-name-red"}
exec(compile(prior_wrapper[prior_helper_start:prior_helper_end], "<prior-source-name>", "exec"), prior_namespace)
with TemporaryDirectory() as prior_directory:
    prior_names = Path(prior_directory) / "names_test.go"
    prior_names.write_text(
        "package p\n"
        "func Test(t *testing.T) {}\n"
        "func Test1(t *testing.T) {}\n"
        "func Test_Foo(t *testing.T) {}\n",
        encoding="utf-8",
    )
    if prior_namespace.get("source_test_names")(prior_names) != []:
        raise SystemExit("red reproduction setup changed: prior name parser already accepted all Go isTest forms")
print(
    f"RED source-name gap: prior {prior_head} omitted Test, Test1 and Test_Foo from source-derived names"
)
with TemporaryDirectory() as directory:
    root = Path(directory) / "root_test.go"
    imported = Path(directory) / "imported.go"
    root.write_text(
        "package p\n"
        "var effect = importedEffect()\n"
        "func importedEffect() int { return 1 }\n"
        "func TestSynthetic(t *testing.T) {}\n",
        encoding="utf-8",
    )
    imported.write_text(
        "package imported\n"
        "func init() { panic(\"must not execute\") }\n",
        encoding="utf-8",
    )
    if namespace.get("source_test_names")(root) != ["TestSynthetic"]:
        raise SystemExit("synthetic effectful initializer source derivation failed")
    if namespace.get("source_test_names")(imported):
        raise SystemExit("synthetic imported init unexpectedly looked like a test")
    names = Path(directory) / "names_test.go"
    names.write_text(
        "package p\n"
        "func Test(t *testing.T) {}\n"
        "func Test1(t *testing.T) {}\n"
        "func Test_Foo(t *testing.T) {}\n",
        encoding="utf-8",
    )
    source_names = namespace.get("source_test_names")
    if source_names(names) != ["Test", "Test1", "Test_Foo"]:
        raise SystemExit("Go isTest name predicate regression failed")
    example = Path(directory) / "example_test.go"
    example.write_text(
        "package p\n"
        "func ExampleWidget() {}\n",
        encoding="utf-8",
    )
    try:
        source_names(example)
    except SystemExit as error:
        if "executable Example declaration" not in str(error):
            raise
    else:
        raise SystemExit("executable Example declaration was not rejected")
print("source-init boundary regression: passed; effectful package var and imported init were parsed only; wrapper contains no go test -list child")
print("source-name regression: passed; Go isTest accepts Test, Test1 and Test_Foo; executable Example declarations fail closed")
PY
```

The source-init boundary regression exited 0: an effectful package-level
initializer and an imported `init` function were present only as source text,
the parser derived the top-level test name, and the wrapper contained no
`go test -list` child. No synthetic initializer, Go test body or live
operation executed.

The immutable prior-head source-name probe recorded the omission red witness
before the current `isTest` correction:

```text
RED source-name gap: prior 22a2923033c875ddd4f755774f79f60b94649449 omitted Test, Test1 and Test_Foo from source-derived names
```

### Diff and staged secret/private-path scan

After staging only this packet file, the final local checks were:

```sh
set -euo pipefail
git diff --cached --name-only | grep -Fxq "docs/evidence/g01-recovery-packet.md"
git diff --cached --name-only | wc -l | tr -d ' ' | grep -Fxq 1
git diff --check
git diff --cached --check
[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || { printf '%s\n' 'reviewed canonical PATH and absolute Python interpreter required' >&2; exit 1; }
/opt/homebrew/bin/python3 -I - <<'PY'
import re
import subprocess

staged_diff = subprocess.check_output(
    ["git", "diff", "--cached", "--unified=0", "--", "docs/evidence/g01-recovery-packet.md"],
    text=True,
)
private_user_root = "/" + "Users/"
private_home_root = "/" + "home/"
private_var_root = "/" + "private/var/"
private_var_folders_root = "/" + "var/folders/"
secret_private_pattern = re.compile(
    r"^\+.*(?:-----BEGIN\s+[A-Z0-9 ]*PRIVATE KEY|"
    r"gh[pousr]_[A-Za-z0-9_]+|github_pat_[A-Za-z0-9_]+|"
    r"AKIA[0-9A-Z]{16}|xox[baprs]-[A-Za-z0-9-]+|"
    + "|".join(
        re.escape(root)
        for root in (
            private_user_root,
            private_home_root,
            private_var_root,
            private_var_folders_root,
        )
    )
    + r")",
    re.MULTILINE,
)
if secret_private_pattern.search(staged_diff):
    raise SystemExit("staged secret/private-path scan: FAILED")
synthetic_match = "+" + "gh" + "p_" + "early_probe"
early_match_probe = "\n".join(
    [synthetic_match] + [f"+ordinary-line-{index:04d}" for index in range(2048)]
)
if not secret_private_pattern.search(early_match_probe):
    raise SystemExit(
        "staged secret/private-path early-match regression probe: FAILED; "
        "first added line was not detected"
    )
print(
    "staged secret/private-path early-match regression probe: passed; first "
    "added line was detected and would be rejected"
)
print(
    "diff and staged secret/private-path checks: passed; working/staged diff "
    "checks exited 0, one staged packet path, no added-line matches, and "
    "early-match rejection probe passed"
)
PY
```

The complete staged diff is captured in a shell variable before scanning, so an
early `rg -q` match cannot send the diff producer SIGPIPE 141 under `pipefail`.
The staged diff check exited 0, the staged path set was exactly this packet
file, and the added-line secret/private-path scan found no matches. The focused
regression fixture put a synthetic matching added line first and 2,048 ordinary
added lines after it; the scanner detected that early match and exercised the
fail-closed rejection path. The recorded output was:

```text
staged secret/private-path early-match regression probe: passed; first added line was detected and would be rejected
diff and staged secret/private-path checks: passed; working/staged diff checks exited 0, one staged packet path, no added-line matches, and early-match rejection probe passed
```

The packet also runs a command-line scan over fenced `sh`/`bash` prescriptions
to fail closed if a documentation correction accidentally adds a live App,
runner, Docker, Lima, Keychain, launchd, workflow or remote-fetch operation.
It joins shell continuations, strips assignment/`env`/`command`/`sudo`/`exec`
prefixes, splits executable shell command chains and examines only each
executable command token. Command-delegating executables (`xargs`, `find`,
`parallel`, `make` and equivalent reviewed launchers) are rejected in their
own right, before a delegated `gh`/live command can be hidden in an argument,
pipeline or generated command. Shell command-string forms (`bash`/`sh` and
equivalent absolute, optioned or `busybox` forms with `-c`) are recursively
inspected and rejected before any nested payload could execute. Shell command
substitutions (`$()`, backticks, `<(...)` and `>(...)`) are rejected before
token-wrapper stripping or fence certification, including direct, assignment,
pipeline and nested forms; the scanner does not attempt to model shell
expansion. Python heredoc bodies are separately parsed with Python's AST:
literal command arguments are inspected with the same executable-token rules,
dynamic command arguments require an explicit reviewed
`g01-safe-python-heredoc` marker, and malformed/unmarked command forms fail
closed. Markdown prose, URLs, comments, scanner source and synthetic fixtures
are not executable prescriptions:

```sh
set -euo pipefail
# g01-safe-python-heredoc: reviewed isolated interpreter argv
[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || { printf '%s\n' 'reviewed canonical PATH and absolute Python interpreter required' >&2; exit 1; }
/opt/homebrew/bin/python3 -I - <<'PY'
import ast
import re
import shlex
from pathlib import Path

source = Path("docs/evidence/g01-recovery-packet.md").read_text(encoding="utf-8")
fence_languages = {"sh", "bash", "shell", "zsh"}
assignment = re.compile(r"[A-Za-z_][A-Za-z0-9_]*=.*")
heredoc = re.compile(r"\bpython3\s+-I\b[^\n]*<<-?\s*(['\"]?)([A-Za-z_][A-Za-z0-9_]*)\1")

def shell_commands(markdown):
    in_shell = False
    skip_until = None
    pending = []
    pending_numbers = []
    for number, line in enumerate(markdown.splitlines(), start=1):
        if line.startswith("```"):
            info = line[3:].strip().lower()
            if in_shell:
                if not info:
                    in_shell = False
                    pending = []
                    pending_numbers = []
                continue
            if info in fence_languages:
                in_shell = True
            continue
        if not in_shell:
            continue
        stripped = line.strip()
        if skip_until is not None:
            if stripped == skip_until:
                skip_until = None
            continue
        heredoc_match = heredoc.search(stripped)
        if heredoc_match:
            skip_until = heredoc_match.group(2)
            continue
        if not stripped or stripped.startswith("#"):
            continue
        pending.append(stripped[:-1].rstrip() if stripped.endswith("\\") else stripped)
        pending_numbers.append(number)
        if stripped.endswith("\\"):
            continue
        yield " ".join(pending), pending_numbers[0]
        pending = []
        pending_numbers = []

def python_heredoc_bodies(markdown):
    """Extract executable Python bodies for AST inspection; never discard them."""
    in_shell = False
    delimiter = None
    body = []
    start_number = None
    safe_marker = False
    marker = "g01-safe-python-heredoc"
    for number, line in enumerate(markdown.splitlines(), start=1):
        stripped = line.strip()
        if delimiter is not None:
            if stripped == delimiter:
                yield start_number, "\n".join(body), safe_marker
                delimiter = None
                body = []
                start_number = None
                safe_marker = False
            else:
                body.append(line)
            continue
        if line.startswith("```"):
            info = line[3:].strip().lower()
            if in_shell:
                if not info:
                    in_shell = False
            elif info in fence_languages:
                in_shell = True
            continue
        if not in_shell:
            continue
        if marker in stripped and stripped.startswith("#"):
            safe_marker = True
            continue
        if safe_marker and stripped.startswith(
            '[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ]'
        ):
            continue
        heredoc_match = heredoc.search(stripped)
        if heredoc_match:
            delimiter = heredoc_match.group(2)
            body = []
            start_number = number
            continue
        if stripped and not stripped.startswith("#"):
            safe_marker = False
    if delimiter is not None:
        raise SystemExit(f"line {start_number}: unterminated executable Python heredoc")

def shell_token_segments(command):
    try:
        lexer = shlex.shlex(command, posix=True, punctuation_chars=";&|")
        lexer.whitespace_split = True
        tokens = list(lexer)
    except ValueError:
        return []
    segments = [[]]
    for token in tokens:
        if token in {";", "&&", "||", "|", "&"}:
            segments.append([])
        else:
            segments[-1].append(token)
    return [segment for segment in segments if segment]

def shell_command_substitution(tokens):
    """Fail closed before wrapper stripping or shell expansion semantics."""
    return any("$(" in token or "`" in token for token in tokens)

def shell_process_substitution(command):
    """Reject Bash process substitution before shell token classification."""
    return re.search(r"(?<!\\)(?:<|>)\(", command) is not None

def executable_basename(token):
    if "://" in token:
        return token
    return token.rsplit("/", 1)[-1]


def python_command_string(tokens):
    if not tokens or executable_basename(tokens[0]) not in {"python", "python3"}:
        return False
    index = 1
    while index < len(tokens):
        option = tokens[index]
        if option == "--":
            return False
        if option == "-c" or option.startswith("-c="):
            return True
        if option.startswith("-") and not option.startswith("--") and "c" in option[1:]:
            return True
        if option.startswith("-"):
            index += 1
            continue
        break
    return False


def executable_tokens(tokens):
    tokens = list(tokens)
    while tokens and tokens[0] in {"if", "then", "else", "do", "while", "until", "!"}:
        tokens.pop(0)
    while tokens and (assignment.fullmatch(tokens[0]) or tokens[0] in {";", "&&", "||", "|"}):
        tokens.pop(0)
    wrappers = {"env", "command", "sudo", "exec", "nohup"}
    while tokens and executable_basename(tokens[0]) in wrappers:
        wrapper = executable_basename(tokens.pop(0))
        while tokens:
            if assignment.fullmatch(tokens[0]):
                tokens.pop(0)
                continue
            if wrapper == "env" and tokens[0] in {"-u", "--unset"}:
                tokens.pop(0)
                if tokens:
                    tokens.pop(0)
                continue
            if wrapper == "sudo" and tokens[0] in {
                "-u", "--user", "-g", "--group",
            }:
                tokens.pop(0)
                if tokens:
                    tokens.pop(0)
                continue
            if wrapper == "exec" and tokens[0] in {"-a", "--argv0"}:
                tokens.pop(0)
                if tokens:
                    tokens.pop(0)
                continue
            if tokens[0].startswith("-"):
                tokens.pop(0)
                continue
            break
        while tokens and tokens[0] in {";", "&&", "||", "|"}:
            tokens.pop(0)
    return tokens

shell_executables = {
    "sh", "bash", "dash", "zsh", "ksh", "mksh", "ash", "fish", "csh", "tcsh",
}

def shell_command_string(tokens):
    if not tokens:
        return None
    shell_index = 0
    executable = executable_basename(tokens[0])
    if executable == "busybox":
        if len(tokens) < 2 or executable_basename(tokens[1]) not in shell_executables:
            return None
        shell_index = 1
        executable = executable_basename(tokens[shell_index])
    if executable not in shell_executables:
        return None
    index = shell_index + 1
    while index < len(tokens):
        option = tokens[index]
        if option == "--":
            return None
        if option == "-c" or option == "--command":
            payload = tokens[index + 1] if index + 1 < len(tokens) else None
            return executable, payload
        if option.startswith("--command="):
            return executable, option.split("=", 1)[1]
        if option.startswith("-c="):
            return executable, option.split("=", 1)[1]
        if option.startswith("-") and not option.startswith("--") and "c" in option[1:]:
            payload = tokens[index + 1] if index + 1 < len(tokens) else None
            return executable, payload
        if option.startswith("-"):
            index += 1
            continue
        break
    return None

def forbidden_command(tokens, depth=0):
    tokens = list(tokens)
    if not tokens:
        return None
    if shell_command_substitution(tokens):
        return "shell command substitutions are not allowed"
    if any(re.search(r"(?<!\\)(?:<|>)\(", token) for token in tokens):
        return "shell process substitutions are not allowed"
    tokens = executable_tokens(tokens)
    if not tokens:
        return None
    if python_command_string(tokens):
        return "python -c command strings are not allowed"
    shell_form = shell_command_string(tokens)
    if shell_form:
        executable, payload = shell_form
        if depth >= 8:
            return f"{executable} -c nested command-string depth exceeded"
        if payload is not None:
            for nested_segment in shell_token_segments(payload):
                nested_violation = forbidden_command(nested_segment, depth + 1)
                if nested_violation:
                    return f"{executable} -c -> {nested_violation}"
        return f"{executable} -c command string"
    # Allowlist/deny checks operate on the executable basename, never the raw
    # token. This covers absolute and relative executable paths without
    # changing shell argument semantics.
    executable = executable_basename(tokens[0])
    if executable == "eval":
        return "eval-wrapped command strings are not allowed"
    if executable in {
        "xargs", "find", "parallel", "gparallel", "make", "gmake",
        "just", "task", "at", "batch", "watch", "entr", "chronic",
    }:
        return f"{executable} command delegation is not allowed"
    if executable in {"curl", "wget", "limactl", "security", "launchctl"}:
        return executable
    if executable == "docker":
        return "docker command"
    if executable == "gh":
        # Global flags such as --repo/--hostname precede the subcommand. A
        # complete parser would need to track every gh global spelling; this
        # packet has no authorized gh prescription, so fail closed for every
        # gh invocation before subcommand classification.
        return "gh command"
    return None

python_command_functions = {
    "os.popen",
    "os.system",
    "subprocess.Popen",
    "subprocess.call",
    "subprocess.check_call",
    "subprocess.check_output",
    "subprocess.run",
}
python_command_modules = {"os", "subprocess"}
python_command_leaf_names = {
    name.rsplit(".", 1)[-1] for name in python_command_functions
}

def python_dotted_name(node):
    parts = []
    while isinstance(node, ast.Attribute):
        parts.append(node.attr)
        node = node.value
    if isinstance(node, ast.Name):
        parts.append(node.id)
        return ".".join(reversed(parts))
    return None


def python_import_bindings(tree):
    modules = {}
    functions = {}
    unresolved = []
    for node in ast.walk(tree):
        if isinstance(node, ast.Import):
            for alias in node.names:
                if alias.name in python_command_modules:
                    modules[alias.asname or alias.name] = alias.name
        elif isinstance(node, ast.ImportFrom) and node.module in python_command_modules:
            for alias in node.names:
                if alias.name == "*":
                    unresolved.append(node.lineno)
                    continue
                imported = f"{node.module}.{alias.name}"
                if imported in python_command_functions:
                    functions[alias.asname or alias.name] = imported
    return modules, functions, unresolved


def python_resolved_name(node, modules, functions):
    dotted = python_dotted_name(node)
    if dotted is None:
        return None
    if dotted in functions:
        return functions[dotted]
    parts = dotted.split(".")
    module = modules.get(parts[0])
    if module is not None and len(parts) > 1:
        return ".".join([module, *parts[1:]])
    return dotted

def python_command_argument(call):
    if call.args:
        return call.args[0]
    for keyword in call.keywords:
        if keyword.arg in {"args", "cmd", "command"}:
            return keyword.value
    return None

def python_literal_command(node):
    if isinstance(node, ast.Constant) and isinstance(node.value, str):
        return "shell", node.value
    if isinstance(node, (ast.List, ast.Tuple)) and node.elts:
        values = []
        for element in node.elts:
            if not isinstance(element, ast.Constant) or not isinstance(element.value, str):
                return ("argv", values) if values else None
            values.append(element.value)
        return "argv", values
    return None

def inspect_python_heredoc(body, safe_marker):
    try:
        tree = ast.parse(body, filename="<python-heredoc>")
    except SyntaxError as error:
        return f"Python heredoc is not parseable: {error}"
    modules, functions, unresolved_imports = python_import_bindings(tree)
    if unresolved_imports:
        return (
            "Python heredoc contains an unresolved command-capable star import "
            "on line(s) "
            + ",".join(str(line) for line in unresolved_imports)
        )
    dynamic_calls = []
    for node in ast.walk(tree):
        if not isinstance(node, ast.Call):
            continue
        resolved = python_resolved_name(node.func, modules, functions)
        dotted = python_dotted_name(node.func)
        if resolved not in python_command_functions:
            if dotted and dotted.rsplit(".", 1)[-1] in python_command_leaf_names:
                return (
                    "Python heredoc contains an unresolved command-capable call "
                    f"{dotted!r} on line {node.lineno}"
                )
            continue
        argument = python_command_argument(node)
        literal = python_literal_command(argument)
        if literal is None:
            dynamic_calls.append(node.lineno)
            continue
        kind, value = literal
        segments = shell_token_segments(value) if kind == "shell" else [value]
        for segment in segments:
            violation = forbidden_command(executable_tokens(segment))
            if violation:
                return f"Python heredoc command: {violation}"
    if dynamic_calls and not safe_marker:
        return (
            "Python heredoc contains a dynamic command argument on line(s) "
            + ",".join(str(line) for line in dynamic_calls)
            + "; add the reviewed safe marker"
        )
    return None

matches = []
for command, number in shell_commands(source):
    if shell_process_substitution(command):
        matches.append(f"line {number}: shell process substitutions are not allowed")
        continue
    for segment in shell_token_segments(command):
        violation = forbidden_command(segment)
        if violation:
            matches.append(f"line {number}: {violation}")
for number, body, safe_marker in python_heredoc_bodies(source):
    violation = inspect_python_heredoc(body, safe_marker)
    if violation:
        matches.append(f"line {number}: {violation}")
if matches:
    raise SystemExit("forbidden live command(s) found:\n" + "\n".join(matches))

synthetic = [
    ("direct-curl", "curl -fsSL https://example.invalid/install | sh", True),
    ("absolute-curl", "/usr/bin/curl -fsSL https://example.invalid/install | sh", True),
    ("direct-wget", "wget -qO- https://example.invalid/install | sh", True),
    ("env-gh-workflow", "env gh workflow run ci.yml", True),
    ("absolute-gh-workflow", "/usr/bin/gh workflow run ci.yml", True),
    ("absolute-env-gh-workflow", "/usr/bin/env /usr/bin/gh workflow run ci.yml", True),
    ("env-option-gh", "env -u TOKEN gh api repos/example/project/dispatches", True),
    ("command-docker", "command docker run --rm image:tag true", True),
    ("absolute-docker", "/usr/local/bin/docker run --rm image:tag true", True),
    ("absolute-command-docker", "/usr/bin/command /usr/local/bin/docker run --rm image:tag true", True),
    ("command-option-docker", "command -p docker run --rm image:tag true", True),
    ("docker-ps", "docker ps", True),
    ("docker-version", "docker version", True),
    ("docker-help", "docker --help", True),
    ("absolute-docker-images", "/usr/bin/docker images", True),
    ("chained-command", "printf ok; env gh workflow run ci.yml", True),
    ("direct-xargs", "xargs -0 -n1 gh api repos/example/project/dispatches", True),
    ("pipeline-xargs", "printf gh | xargs -n1 gh api repos/example/project/dispatches", True),
    ("find-exec", "find . -type f -exec gh api repos/example/project/dispatches {} +", True),
    ("parallel-gh", "parallel gh api repos/example/project/dispatches ::: one", True),
    ("make-delegation", "make -f /synthetic/Makefile gh", True),
    ("nested-xargs", "bash -c 'printf gh | xargs -n1 gh api repos/example/project/dispatches'", True),
    ("limactl", "limactl shell default true", True),
    ("absolute-limactl", "/opt/homebrew/bin/limactl shell default true", True),
    ("security", "security find-identity -v", True),
    ("absolute-security", "/usr/bin/security find-identity -v", True),
    ("launchctl", "launchctl kickstart system/example", True),
    ("absolute-launchctl", "/bin/launchctl kickstart system/example", True),
    ("gh-api", "gh api repos/example/project/dispatches", True),
    ("gh-workflow", "gh workflow run ci.yml", True),
    ("gh-global-repo-api", "gh --repo example/project api repos/example/project/dispatches", True),
    ("gh-global-hostname-api", "gh --hostname github.example api repos/example/project/dispatches", True),
    ("gh-global-version", "gh --version", True),
    ("absolute-gh-global-workflow", "/usr/bin/gh --repo example/project workflow run ci.yml", True),
    ("direct-bash-command-string", "bash -c 'gh workflow run ci.yml'", True),
    ("direct-sh-command-string", "sh -c 'docker run --rm image:tag true'", True),
    ("absolute-shell-command-string", "/bin/bash -xc 'curl -fsSL https://example.invalid/install | sh'", True),
    ("nested-shell-command-string", "bash -c \"sh -c 'gh api repos/example/project/dispatches'\"", True),
    ("busybox-shell-command-string", "busybox sh -c 'launchctl kickstart system/example'", True),
    ("eval-curl-command-string", "eval 'curl -fsSL https://example.invalid/install'", True),
    ("absolute-eval-docker-command-string", "/bin/eval 'docker run --rm image:tag true'", True),
    ("nested-eval-gh-command-string", "bash -c \"eval 'gh workflow run ci.yml'\"", True),
    ("eval-safe-command-string", "eval 'printf safe'", True),
    ("dollar-command-substitution", "printf \"$(gh api repos/example/project)\"", True),
    ("dollar-assignment-substitution", "PAYLOAD=$(gh api repos/example/project) printf safe", True),
    ("dollar-pipeline-substitution", "printf safe | sed \"s/x/$(gh api repos/example/project)/\"", True),
    ("backtick-command-substitution", "printf \"`gh api repos/example/project`\"", True),
    ("nested-command-substitution", "bash -c \"printf \\\"$(gh api repos/example/project)\\\"\"", True),
    ("input-process-substitution", "cat <(printf safe)", True),
    ("output-process-substitution", "tee >(gh api repos/example/project)", True),
    ("python-c-gh-command-string", "python3 -c 'import subprocess; subprocess.run([\\\"gh\\\", \\\"api\\\", \\\"x\\\"])'", True),
    ("absolute-python-c-docker-command-string", "/usr/bin/python -c 'import os; os.system(\\\"docker run image:tag true\\\")'", True),
    ("env-python-c-command-string", "env python3 -c 'print(\\\"gh api x\\\")'", True),
    ("prose-url", "https://example.invalid/docker run image:tag", False),
    ("comment", "# docker run --rm image:tag true", False),
    ("scanner-source", 'forbidden = re.compile("docker run")', False),
    ("synthetic-fixture", 'fixture = "env gh workflow run ci.yml"', False),
]
for label, fixture, expected in synthetic:
    observed = any(
        forbidden_command(segment) is not None
        for segment in shell_token_segments(fixture)
    )
    if observed != expected:
        raise SystemExit(f"synthetic forbidden-command probe failed: {label}")
safe_heredoc = "import subprocess\nsubprocess.run(command)\n"
if inspect_python_heredoc(safe_heredoc, True) is not None:
    raise SystemExit("safe marked Python heredoc was rejected")
for unsafe_heredoc in (
    'import subprocess\nsubprocess.run(["gh", "api", "x"])\n',
    'import os\nos.system("docker version")\n',
    'import subprocess as sp\nsp.run(["gh", "api", "x"])\n',
    'from subprocess import run\nrun(["gh", "api", "x"])\n',
    'from os import system\nsystem("docker version")\n',
):
    if inspect_python_heredoc(unsafe_heredoc, False) is None:
        raise SystemExit("unsafe Python heredoc was accepted")
print("forbidden-live-command synthetic probes: passed; direct/wrapped fetcher, every gh invocation including global-flag and absolute forms, every Docker invocation, command-delegating xargs/find/parallel/make forms, shell substitutions/process substitutions, limactl/security/launchctl, eval, AST-inspected safe/unsafe Python heredocs and direct/nested bash/sh -c forms rejected; prose/URLs/comments/scanner source/fixtures ignored")
print("forbidden-live-command scan: passed; executable shell prescriptions contain no forbidden live App/runner/Docker/Lima/Keychain/launchd/workflow/fetch command")
PY
```

The forbidden-live-command scan and its static synthetic probes exited 0; all
direct, wrapped and command-delegating forms were rejected without execution,
including xargs, find, parallel and make forms that could otherwise hide a
downstream `gh`/live command, while prose, URLs, comments, scanner source and
fixtures were ignored. No live operation, workflow replay, credential access
or destructive cleanup command was introduced.

No check is claimed against an unrecorded SHA. The staged correction diff is
distinct from the cumulative PR #78 diff; the cumulative history still
includes the independent driver correction from
`7ce053380003c9260f46bf93ea118893b95f01e7` and team-review routing corrections
from `f59a30532bfdc62876065dcf8b4997520606e6a6`, which remain outside this
staged change.

## Historical Codex finding ledger

The following stale/outdated root findings were omitted from the earlier
correction paragraphs. Each row preserves the original finding URL and the
immutable source commit where Codex observed it; disposition cites the packet
change that reproduced and fixed the gap, not staleness alone. The residual
selector-matching finding is resolved by the reusable wrapper above, which now
guards every runnable long selector before executing its original command.

| Finding and immutable source | Reproduction and disposition |
|---|---|
| [3998925786](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3998925786), source [0e08ce8285846e26a94fb6fcb33bbeede662bf14](https://github.com/1XP-AI/gh-runnerd/commit/0e08ce8285846e26a94fb6fcb33bbeede662bf14) | Reproduced: the root-only JIT rerun omitted worker transport. The packet now includes worker-runtime and tagged worker-command selectors; the wrapper validates their package/tag name sets before execution. |
| [3998925788](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3998925788), source [0e08ce8285846e26a94fb6fcb33bbeede662bf14](https://github.com/1XP-AI/gh-runnerd/commit/0e08ce8285846e26a94fb6fcb33bbeede662bf14) | Reproduced: the non-terminal paired command excluded every terminal test. The packet now prescribes collection, all-except, worker, heavy, remainder and storage partitions, each guarded by list validation. |
| [3998925790](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3998925790), source [0e08ce8285846e26a94fb6fcb33bbeede662bf14](https://github.com/1XP-AI/gh-runnerd/commit/0e08ce8285846e26a94fb6fcb33bbeede662bf14) | Reproduced: inline selector pipes could split the ledger table. Commands remain outside the ten-row ledger, and the recorded shape check requires ten data rows with four columns. |
| [3998925793](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3998925793), source [0e08ce8285846e26a94fb6fcb33bbeede662bf14](https://github.com/1XP-AI/gh-runnerd/commit/0e08ce8285846e26a94fb6fcb33bbeede662bf14) | Reproduced: the earlier driver blob contradicted ACK-before-acquisition. The evidence index now points to the corrected driver revision 0e08ce8285846e26a94fb6fcb33bbeede662bf14. |
| [3998925795](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3998925795), source [0e08ce8285846e26a94fb6fcb33bbeede662bf14](https://github.com/1XP-AI/gh-runnerd/commit/0e08ce8285846e26a94fb6fcb33bbeede662bf14) | Reproduced: callback-loss reruns omitted the missing-callback and error/quarantine cases. The ACK root selector now names all three recovery tests and the wrapper checks the resulting set/count. |
| [3998973873](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3998973873), source [9d07f3ccbbf4ff2a80aaa5ff43f376cb2b3da999](https://github.com/1XP-AI/gh-runnerd/commit/9d07f3ccbbf4ff2a80aaa5ff43f376cb2b3da999) | Reproduced: root-only ACK reruns omitted driver ACK/acquisition checks. The livecanary ACK selector now includes the driver and refusal contracts, with pre-run set/count validation. |
| [3998973876](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3998973876), source [9d07f3ccbbf4ff2a80aaa5ff43f376cb2b3da999](https://github.com/1XP-AI/gh-runnerd/commit/9d07f3ccbbf4ff2a80aaa5ff43f376cb2b3da999) | Reproduced: worker-only JIT reruns omitted controller JIT-loss probes. The livecanary JIT selector now names both controller cases and is guarded before execution. |
| [3998973877](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3998973877), source [40daf0f074b9cc9559135adac1cafaad218275be](https://github.com/1XP-AI/gh-runnerd/commit/40daf0f074b9cc9559135adac1cafaad218275be) | Reproduced: the reconciliation prefix omitted retained-uncertainty predicates. The quarantine selector now explicitly includes all named predicates, with count/set validation. |
| [3999010977](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999010977), source [9d07f3ccbbf4ff2a80aaa5ff43f376cb2b3da999](https://github.com/1XP-AI/gh-runnerd/commit/9d07f3ccbbf4ff2a80aaa5ff43f376cb2b3da999) | Reproduced: valid singleton cases did not reject foreign or multi-job messages. The ACK/livecanary selector now includes both refusal tests and validates their presence before running. |
| [3999010981](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999010981), source [9d07f3ccbbf4ff2a80aaa5ff43f376cb2b3da999](https://github.com/1XP-AI/gh-runnerd/commit/9d07f3ccbbf4ff2a80aaa5ff43f376cb2b3da999) | Reproduced: the image/profile row selected only one worker test. The worker-runtime selector now includes profile, update-policy and Docker-preflight contracts, guarded by the wrapper. |
| [3999010985](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999010985), source [9d07f3ccbbf4ff2a80aaa5ff43f376cb2b3da999](https://github.com/1XP-AI/gh-runnerd/commit/9d07f3ccbbf4ff2a80aaa5ff43f376cb2b3da999) | Reproduced: retained-uncertainty cases were absent from both reconciliation commands. The quarantine selector now includes observed-job, unexpected-work, later-absence and persistence-failure cases. |
| [3999010989](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999010989), source [9d07f3ccbbf4ff2a80aaa5ff43f376cb2b3da999](https://github.com/1XP-AI/gh-runnerd/commit/9d07f3ccbbf4ff2a80aaa5ff43f376cb2b3da999) | Reproduced: the baseline-only command omitted global admission checks. The packet now has explicit controller and worker admission selectors, including cross-directory and concurrent-start cases. |
| [3999010990](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999010990), source [9d07f3ccbbf4ff2a80aaa5ff43f376cb2b3da999](https://github.com/1XP-AI/gh-runnerd/commit/9d07f3ccbbf4ff2a80aaa5ff43f376cb2b3da999) | Reproduced: SDK session-refresh behavior was omitted from the ACK selector. The root selector now includes TestSDKHTTPFailuresAndSessionRefresh and validates the exact set. |
| [3999060310](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999060310), source [60fdefe124d7c83ad54eac36950cb574f7e0c9aa](https://github.com/1XP-AI/gh-runnerd/commit/60fdefe124d7c83ad54eac36950cb574f7e0c9aa) | Reproduced: demand and stale-statistics tests were absent from the SDK rerun. The root selector now names both cases and the wrapper checks count and set before running. |
| [3999208511](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999208511), source [cdaf879fed807da517ab88ea5db3dba6e99a948e](https://github.com/1XP-AI/gh-runnerd/commit/cdaf879fed807da517ab88ea5db3dba6e99a948e) | Reproduced: the tagged controller command was missing. The packet now prescribes the ten tagged cmd/g01-live input/refusal checks with fail-closed validation. |
| [3999208514](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999208514), source [f59a30532bfdc62876065dcf8b4997520606e6a6](https://github.com/1XP-AI/gh-runnerd/commit/f59a30532bfdc62876065dcf8b4997520606e6a6) | Reproduced: tagged worker JIT input checks were absent. The packet now includes the g01_worker command and expected set/count guard. |
| [3999208519](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999208519), source [9dea03887cc530f742e705055064579121a84206](https://github.com/1XP-AI/gh-runnerd/commit/9dea03887cc530f742e705055064579121a84206) | Reproduced: worker journal/authority tests were omitted. The liveworker reconciliation selector now names those durability contracts and validates them before execution. |
| [3999433329](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999433329), source [9dea03887cc530f742e705055064579121a84206](https://github.com/1XP-AI/gh-runnerd/commit/9dea03887cc530f742e705055064579121a84206) | Reproduced: worker admission contracts were not explicit. The worker admission selector now covers the integrity suite and the tagged unsupported-account case, with wrapper validation. |
| [3999433330](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999433330), source [080b9c29c0c056dffd03f54567b593a426a1909d](https://github.com/1XP-AI/gh-runnerd/commit/080b9c29c0c056dffd03f54567b593a426a1909d) | Reproduced: controller update-policy contracts were absent. The livecanary update-policy selector now includes disable-update, unconfirmed-setting and drift cases. |
| [3999433331](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999433331), source [9dea03887cc530f742e705055064579121a84206](https://github.com/1XP-AI/gh-runnerd/commit/9dea03887cc530f742e705055064579121a84206) | Reproduced: uncertainty and cleanup contracts were omitted from the worker selector. The worker-runtime command now includes uncertain-start, active/unknown retention and terminal-removal cases. |
| [3999433334](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999433334), source [9dea03887cc530f742e705055064579121a84206](https://github.com/1XP-AI/gh-runnerd/commit/9dea03887cc530f742e705055064579121a84206) | Reproduced: scheduled reconciliation was stated too much like implemented behavior. The packet now labels scheduling, fencing and rehydration as future production requirements and preserves the live gap. |
| [3999433335](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999433335), source [9dea03887cc530f742e705055064579121a84206](https://github.com/1XP-AI/gh-runnerd/commit/9dea03887cc530f742e705055064579121a84206) | Reproduced: broad rollback could revert independent corrections. The rollback row now scopes removal/restoration to this packet and preserves driver and routing corrections. |

### Supplemental stale/outdated root findings

These eight root findings were received after the historical 22-row ledger was
recorded. Each row retains the discussion URL and immutable source commit;
disposition cites the packet selector or source text that reproduces the issue
and its correction, rather than relying on staleness alone.

| Finding and immutable source | Reproduction and disposition |
|---|---|
| [3999099702](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999099702), source [40daf0f074b9cc9559135adac1cafaad218275be](https://github.com/1XP-AI/gh-runnerd/commit/40daf0f074b9cc9559135adac1cafaad218275be) | Reproduced: the admission selector omitted `TestAuditPR25DistinctStateDirectoriesMustShareCap`. The packet's admission-livecanary selector now names that cross-directory cap regression, with wrapper list/set/count validation before execution. |
| [3999099708](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999099708), source [40daf0f074b9cc9559135adac1cafaad218275be](https://github.com/1XP-AI/gh-runnerd/commit/40daf0f074b9cc9559135adac1cafaad218275be) | Reproduced: the reconciliation selectors omitted `TestDemandStatisticsAllowControlledProbeButNeverCleanup`. The packet now includes that demand-statistics quarantine contract in the reconciliation and quarantine selectors, with exact list/set/count validation. |
| [3999140786](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999140786), source [65071cfb1e8ad0f47ab7e09dd119f1c98beaaefc](https://github.com/1XP-AI/gh-runnerd/commit/65071cfb1e8ad0f47ab7e09dd119f1c98beaaefc) | Reproduced: reconciliation selectors omitted the four `TestInventory*` contracts. The packet now names `TestInventoryStrictPages`, `TestInventoryMalformedStopsLegacyEffects`, `TestInventoryTransportRefusalIsBoundedAndSanitized` and `TestInventoryImpossibleTotalStopsBeforeNextPage` in the exact reconciliation selector and audit. |
| [3999140787](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999140787), source [65071cfb1e8ad0f47ab7e09dd119f1c98beaaefc](https://github.com/1XP-AI/gh-runnerd/commit/65071cfb1e8ad0f47ab7e09dd119f1c98beaaefc) | Reproduced: the ACK selector omitted `TestNoMessageDoesNotCountAsCompletedBarrier`. The packet now includes this empty-poll barrier contract in the exact ACK/livecanary selector and list-only audit. |
| [3999140792](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999140792), source [65071cfb1e8ad0f47ab7e09dd119f1c98beaaefc](https://github.com/1XP-AI/gh-runnerd/commit/65071cfb1e8ad0f47ab7e09dd119f1c98beaaefc) | Reproduced: the secret/error selector omitted decoded-body truncation and gzip-budget checks. The packet now includes `TestResponseReaderConsumesOnlyBudgetPlusOneAndRejectsTruncation` and `TestResponseBudgetAppliesAfterGzipDecompression` in the livecanary selector. |
| [3999140797](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999140797), source [65071cfb1e8ad0f47ab7e09dd119f1c98beaaefc](https://github.com/1XP-AI/gh-runnerd/commit/65071cfb1e8ad0f47ab7e09dd119f1c98beaaefc) | Reproduced: journal/authority selectors omitted strict duplicate-field, failed-sync, unrecorded-authority and renewed-approval contracts. The packet now includes the named livecanary and liveworker journal/authority cases in the reconciliation partitions and audits their exact sets. |
| [3999140800](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999140800), source [65071cfb1e8ad0f47ab7e09dd119f1c98beaaefc](https://github.com/1XP-AI/gh-runnerd/commit/65071cfb1e8ad0f47ab7e09dd119f1c98beaaefc) | Reproduced: the tagged controller command did not exercise the livecanary credential-attestation and plaintext/off-host transport refusals. The packet now includes `TestCredentialAttestationMismatchAndExpiredTokenRejected` and `TestTransportRejectsPlaintextOffHostAndProxyBeforeNetwork` in the live-transport selector. |
| [3999140806](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999140806), source [9b94ff9f3114e5546abbe741b96486c3bc13626c](https://github.com/1XP-AI/gh-runnerd/commit/9b94ff9f3114e5546abbe741b96486c3bc13626c) | Reproduced against the immutable source: `docs/reviews/team-review.md` already marks the Astra assignments historical-only and routes current work through `docs/EXECUTION.md`; the correction was made by `f59a30532bfdc62876065dcf8b4997520606e6a6`. This stale finding is outside packet-only ownership, so no other file is changed here. |

### Current exact-head GOWORK findings

The two Codex findings on the exact prior packet head
[`c550adcb2ca6532e2c69cbb6ad8d2aafd5e352ae`](https://github.com/1XP-AI/gh-runnerd/commit/c550adcb2ca6532e2c69cbb6ad8d2aafd5e352ae)
were reproduced and corrected here; neither is treated as resolved by
staleness alone:

| Finding and immutable source | Reproduction and disposition |
|---|---|
| [4000590180](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4000590180), source [c550adcb2ca6532e2c69cbb6ad8d2aafd5e352ae](https://github.com/1XP-AI/gh-runnerd/commit/c550adcb2ca6532e2c69cbb6ad8d2aafd5e352ae) | Reproduced: inherited `GOWORK` could select an external workspace/replacement while the wrapper was resolving package metadata. Corrected: after parsing environment assignments, the wrapper forces `GOWORK=off` in one Go-child environment before `go env`, `go list`, either `go test -list` probe or the JSON test subprocess, so caller workspaces are ignored before metadata selection. |
| [4000590184](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4000590184), source [c550adcb2ca6532e2c69cbb6ad8d2aafd5e352ae](https://github.com/1XP-AI/gh-runnerd/commit/c550adcb2ca6532e2c69cbb6ad8d2aafd5e352ae) | Reproduced: inherited `GOWORK` could alter list or execution behavior after metadata validation. Corrected: the independent selector and package-init audits also pin `GOWORK=off`; a read-only synthetic inherited-workspace probe injected an external sentinel and observed `GOWORK=off` on all three direct metadata/list children before accepting the list-only result. No test body, live resource or external workspace was used. |

### Current exact-head Go overlay finding

The Codex finding on the exact prior packet head
[`44a2142adab39bba73fc93965fa003ed17506d92`](https://github.com/1XP-AI/gh-runnerd/commit/44a2142adab39bba73fc93965fa003ed17506d92)—[4000667752](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4000667752)—was reproduced and corrected here; it is not treated as resolved by a new head or staleness alone:

| Finding and immutable source | Reproduction and disposition |
|---|---|
| [4000667752](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4000667752), source [44a2142adab39bba73fc93965fa003ed17506d92](https://github.com/1XP-AI/gh-runnerd/commit/44a2142adab39bba73fc93965fa003ed17506d92) | Reproduced: the guarded wrapper accepted Go `-overlay` build overrides, allowing a caller-supplied overlay to change the source/build inputs after the wrapper's reviewed-tree assumptions. Corrected: immediately after parsing command-prefix assignments and validating `go test`, the wrapper rejects both separated `-overlay FILE` and equals-form `-overlay=FILE` before the first `go env`, metadata, list or test child. The focused synthetic regression patched direct Go-child launches, required both forms to reject with empty pre-guard output, and therefore recorded no test body or result; no live operation, secret, private path or overlay file was used. |

### Current exact-head Go alternate-module-file finding

The Codex finding on the exact prior packet head
[`5a5a4e97e0fbd2a9dca0d9f085003d7ce7e65038`](https://github.com/1XP-AI/gh-runnerd/commit/5a5a4e97e0fbd2a9dca0d9f085003d7ce7e65038)—[4000712999](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4000712999)—was reproduced and corrected here; it is not treated as resolved by a new head or staleness alone:

| Finding and immutable source | Reproduction and disposition |
|---|---|
| [4000712999](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4000712999), source [5a5a4e97e0fbd2a9dca0d9f085003d7ce7e65038](https://github.com/1XP-AI/gh-runnerd/commit/5a5a4e97e0fbd2a9dca0d9f085003d7ce7e65038) | Reproduced: the guarded wrapper rejected Go `-overlay` overrides but still accepted `-modfile`, allowing a caller-supplied alternate module file to change the module graph and dependency/build inputs after the wrapper's reviewed-tree assumptions. Corrected: immediately after parsing command-prefix assignments and validating `go test`, the wrapper rejects both separated `-modfile FILE` and equals-form `-modfile=FILE` before the first `go env`, metadata, list or test child. The focused synthetic no-Go-child regression patched direct Go-child launches, required both forms to reject with empty pre-guard output, and therefore recorded no test body or result; no live operation, secret, private path or alternate module file was used. |

### Current exact-head selector-audit findings

The two Codex findings on exact prior packet head
[`2a054740580a0ed4872742d39308d373300b6e5e`](https://github.com/1XP-AI/gh-runnerd/commit/2a054740580a0ed4872742d39308d373300b6e5e)—
[4000760151](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4000760151)
and [4000760154](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4000760154)—
were reproduced and corrected here; neither is treated as resolved by a new
head or staleness alone:

| Finding and immutable source | Reproduction and disposition |
|---|---|
| [4000760151](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4000760151), source [2a054740580a0ed4872742d39308d373300b6e5e](https://github.com/1XP-AI/gh-runnerd/commit/2a054740580a0ed4872742d39308d373300b6e5e) | Reproduced: the prior static selector regex required assignments to be followed directly by `go test`, so a standard `env GOTOOLCHAIN=go1.26.8 go test ... -run ...` could evade discovery while the wrapper's generic command-shape rejection was not part of the audit. Corrected: static shell and logical-command audits now discover prefixed `env` forms, including continued and option-bearing forms, while the wrapper and package/build metadata audit explicitly reject an `env` command before any Go child. Focused synthetic red-before-green probes proved unguarded env-wrapped selectors are discovered/rejected and patched direct Go-child launches to require both env forms to fail with no result output; no Go test body, live operation, credential or private path was used. |
| [4000760154](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4000760154), source [2a054740580a0ed4872742d39308d373300b6e5e](https://github.com/1XP-AI/gh-runnerd/commit/2a054740580a0ed4872742d39308d373300b6e5e) | Reproduced: the prior wrapper parsed only single-dash `-run`/`-skip`, so a later equivalent `--run`/`--skip` could expand the actual selector after a narrower list digest had passed. Corrected: wrapper and metadata validation reject separated and equals-form `--run`/`--skip` before `go env`, metadata or list execution, and static selector discovery recognizes both dash widths. Focused synthetic red-before-green probes covered all four double-dash forms, patched direct Go-child launches, required empty pre-guard output and therefore recorded no test body or result; no live operation or secret-bearing input was used. |

The two new exact-head findings
on `95cd9210620c54e098ecbe0df1217af1659f0c74`—[3999634756](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999634756)
and [3999634759](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999634759)—are
covered by the controller-side `TestAmbiguousCreateNeverRetriesAfterRestart`
entry in the livecanary reconciliation selector and the seven direct liveworker
runtime contracts in the worker selector, with matching list-only/declaration/tag
checks above. The five new exact-head findings
on `4a4e159ce29414fa8e5bdf7a0d0e5d9b8d038c67`—[3999599127](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999599127),
[3999599130](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999599130),
[3999599132](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999599132),
[3999599134](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999599134),
and [3999599136](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999599136)—are covered by the explicit liveworker Docker-inspection and worker-preparation selectors, the suffixed livecanary reconciliation families, and the matching list-only/declaration checks above.
The prior five exact-head findings
on `080b9c29c0c056dffd03f54567b593a426a1909d`—[3999566558](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999566558),
[3999566563](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999566563),
[3999566567](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999566567),
[3999566570](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999566570),
and [3999566571](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999566571)—are covered by the worker/runtime and canonical-preparation selectors, the tagged livecanary invocation, and the explicit Runner pin/source-audit gate above.
The prior four exact-head
findings [3999532146](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999532146),
[3999532149](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999532149),
[3999532151](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999532151),
and [3999532158](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999532158)
are covered by these selector, partition, and validation-scope corrections.
The prior six exact-head findings [3999486217](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999486217),
[3999486223](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999486223),
[3999486229](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999486229),
[3999486231](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999486231),
[3999486236](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999486236),
and [3999486239](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999486239)
remain covered by the existing selector and partition corrections. The
three exact-head findings on `c4b73c75db9556f96491fb8a10a8257408a7d11b`—
[3999807788](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999807788),
[3999807790](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999807790),
and [3999807791](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999807791)—are
addressed by the literal selector expected-set/count audit, its list-only
root-safe invocation, and the scoped tracked/untracked source audit above.
The historical [secret/error finding 3999010986](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999010986)
remains addressed by the dedicated Secret and error handling row and its root
and livecanary commands above; no issue, Project, or Goal state is changed.

### Current exact-head P2 findings at `36ec84b27c934a25484b0a5391af0a20c7643912`

The three fresh Codex P2 findings on the exact PR #78 head named in this
correction were reproduced before editing and are addressed below. Their
reproductions and focused checks are packet-local static/wrapper evidence; no
GitHub review metadata is edited here, and no finding is marked resolved by
head movement alone.

| Finding | Red reproduction and minimal correction | Focused result and boundary |
|---|---|---|
| Pre-Go-child selector guard omitted Go `-toolexec` separated and equals forms. | Before the correction, the wrapper had no `-toolexec` rejection, so synthetic `-toolexec FILE` and `-toolexec=FILE` inputs could reach the first Go metadata child. The wrapper now rejects both forms immediately after command-shape parsing, before `go env`, metadata, source derivation or execution. | The direct-child monkeypatch regression rejected both forms with empty pre-guard output; no Go test body, live operation or tool hook ran. |
| Prescription audit could fail to account for standard assignment-prefixed `go test` selector commands. | A synthetic `GOTOOLCHAIN=go1.26.8 go test ... -run`/`-skip` pair was the red audit case: a wrapper-only count could remain 28 while an unguarded standard command was present. The logical audit now discovers assignment-prefixed, `env`-wrapped, continued and double-dash forms first, rejects every unguarded candidate, and asserts the 28 logical selector count equals the 28 `go_test_checked` records. | Assignment-prefixed probes were rejected as unguarded and the documented selector count remained exact; no command body or live operation ran. |
| Source/init gate did not account for effectful package variables or imported init paths before `go test -list`. | The prior `go test -list` validation could start the test binary before the expected set was proven, while the source gate only checked active-package `func init`. The minimal safe correction replaces both `go test -list` probes with source-derived top-level test names from the exact `go list -json -test` file set, applies a fail-closed Go-RE2-compatible selector subset, and retains the reviewed source/init gate. | A synthetic package-level effect and imported `init` were parsed without execution; the wrapper contains no `go test -list` child. No initializer, Go test body or live operation ran. |

The exact wrapper-replay, selector-audit and source-init-boundary outputs above
are the TDD-like red-before-green record for this head correction. The old
`go test -list` records remain explicitly historical/pre-correction evidence;
the current prescription boundary is non-executing source derivation followed
by the guarded original command only after count/set validation.

### Current exact-head command-prefix selector finding

The Codex finding on the exact prior packet head
[`ec5eb8087420bbbbb2a8ccf5c5df190b3c644895`](https://github.com/1XP-AI/gh-runnerd/commit/ec5eb8087420bbbbb2a8ccf5c5df190b3c644895)—[4000820533](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4000820533)—was reproduced against that immutable head and corrected here; it is not treated as resolved by a new head or staleness alone:

| Finding and immutable source | Reproduction and disposition |
|---|---|
| [4000820533](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4000820533), source [ec5eb8087420bbbbb2a8ccf5c5df190b3c644895](https://github.com/1XP-AI/gh-runnerd/commit/ec5eb8087420bbbbb2a8ccf5c5df190b3c644895) | Reproduced: the prior static shell and logical-command selector regexes did not discover `command go test ./livecanary -run ...` or `GOTOOLCHAIN=go1.26.8 command go test ./livecanary -skip=...`, so a wrapper-only count could remain 28; the prior wrapper had no explicit command-prefix guard. Corrected: both audits now discover bare and assignment-prefixed `command [options] go test` forms, while the wrapper and package/build metadata audit reject any `command` prefix before the first Go child. Focused red/green probes covered the two exact forms, option-bearing discovery and no-Go-child rejection with no pre-guard output; no Go test body, live operation, credential or private path was used. |

### Current exact-head P2 findings at `3bc8445567fe68cc355cf3f88f0c962a41e9cad5`

These three fresh roots were observed on the immutable packet head at task
start. Each row preserves the discussion URL and exact source blob where the
gap existed, then cites the current wrapper/probe line anchors and the
immutable red/current-green evidence; no row treats head movement as proof of
resolution.

| Finding and immutable source | Red reproduction, correction and final evidence |
|---|---|
| [4001124039](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4001124039), source [3bc8445567fe68cc355cf3f88f0c962a41e9cad5 lines 858-914](https://github.com/1XP-AI/gh-runnerd/blob/3bc8445567fe68cc355cf3f88f0c962a41e9cad5/docs/evidence/g01-recovery-packet.md#L858-L914) | Reproduced: the immutable source-name helper omitted top-level `Fuzz*`, while `paired-all-except` has no `-run`; an unrepresented fuzz seed could therefore execute. Corrected: the wrapper's pre-Go-child `source_fuzz_guard` scans package test declarations and `source_test_names` rejects `Fuzz*` before digest derivation. Immutable red/current-green evidence is in [the fuzz-target correction](#root-4001124039-top-level-fuzz-declarations); final wrapper anchors are `source_fuzz_declarations`, `source_fuzz_guard`, and the defense-in-depth `Fuzz*` rejection in the wrapper. |
| [4001124042](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4001124042), source [3bc8445567fe68cc355cf3f88f0c962a41e9cad5 lines 917-936](https://github.com/1XP-AI/gh-runnerd/blob/3bc8445567fe68cc355cf3f88f0c962a41e9cad5/docs/evidence/g01-recovery-packet.md#L917-L936) | Reproduced: the immutable helper passed Go `\\w` to Python, where Unicode `é` matched despite Go's ASCII Perl-class semantics. Corrected: preflight rejects unescaped `\\b`, `\\B`, `\\w`, `\\W`, `\\d`, `\\D`, `\\s`, `\\S` before Python compilation/metadata, while escaped literal backslashes remain supported. Immutable red/current-green evidence is at packet lines 2958-3071; final anchors are `reject_python_semantic_regexp_constructs` at lines 917-932, `translate_go_posix_classes`/`reject_unsupported_regexp_syntax` at lines 935-986 and preflight ordering at lines 1013-1017. |
| [4001124048](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4001124048), source [3bc8445567fe68cc355cf3f88f0c962a41e9cad5 lines 3726-3750](https://github.com/1XP-AI/gh-runnerd/blob/3bc8445567fe68cc355cf3f88f0c962a41e9cad5/docs/evidence/g01-recovery-packet.md#L3726-L3750) | Reproduced: the immutable anchored scan missed curl/wget and `env gh`/`command docker` wrappers. Corrected: the current scanner inspects only executable shell fences, joins continuations, skips Python heredocs/comments/prose/URLs/fixtures and strips assignment/env/command wrappers before checking curl/wget, Docker, Lima, Keychain, launchd and gh API/workflow forms. Static synthetic probes and the current scan are at packet lines 4164-4310. |

### Current exact-head P2 findings at `423d4fc501120a014e63f77d3ef6652606d0326a`

These two fresh Codex P2 findings from review `5192699095` were observed on
the immutable packet head at task start. Each red witness reads only that
prior packet blob; each current green probe is source-only or static and
proves the no-Go-child boundary without running a test body or live operation.

#### Root 4001254021: consecutive source comments

The immutable red witness shows that the prior source-name helper returned
after the first line comment. A valid top-level `TestHidden` declaration
following consecutive line/block comments was therefore omitted:

```sh
set -euo pipefail
[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || { printf '%s\n' 'reviewed canonical PATH and absolute Python interpreter required' >&2; exit 1; }
/opt/homebrew/bin/python3 -I - <<'PY'
import subprocess
from pathlib import Path
from tempfile import TemporaryDirectory

starting_head = "423d4fc501120a014e63f77d3ef6652606d0326a"
previous = subprocess.check_output(
    [
        "git",
        "show",
        f"{starting_head}:docs/evidence/g01-recovery-packet.md",
    ],
    text=True,
)
wrapper_start = previous.index("\nimport hashlib\n", previous.index("go_test_checked()")) + 1
wrapper_end = previous.index("\nPY\n}", wrapper_start)
previous_wrapper = previous[wrapper_start:wrapper_end]
helper_start = previous_wrapper.index("def skip_source_ignored")
helper_end = previous_wrapper.index("\nall_test_names = []", helper_start)
namespace = {"label": "prior-comment-red"}
exec(compile(previous_wrapper[helper_start:helper_end], "<prior-source-name>", "exec"), namespace)
source_names = namespace["source_test_names"]
with TemporaryDirectory() as directory:
    source_path = Path(directory) / "comments_test.go"
    source_path.write_text(
        "package p\n"
        "import \"testing\"\n"
        "func\n"
        "// first line\n"
        "/* middle block */\n"
        "// second line\n"
        "TestHidden(t *testing.T) {}\n",
        encoding="utf-8",
    )
    if source_names(source_path) != []:
        raise SystemExit(
            "red reproduction setup changed: prior parser no longer omitted TestHidden"
        )
print(
    f"RED source-name comment gap: prior {starting_head} omitted TestHidden "
    "after consecutive line/block comments"
)
PY
```

Recorded red output:

```text
RED source-name comment gap: prior 423d4fc501120a014e63f77d3ef6652606d0326a omitted TestHidden after consecutive line/block comments
```

The current source-only regression proves that the helper consumes all
consecutive ignored tokens and discovers `TestHidden`; it patches any attempted
Go child to fail, so discovery cannot silently expand into execution:

```sh
set -euo pipefail
[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || { printf '%s\n' 'reviewed canonical PATH and absolute Python interpreter required' >&2; exit 1; }
/opt/homebrew/bin/python3 -I - <<'PY'
import subprocess
from pathlib import Path
from tempfile import TemporaryDirectory

packet = Path("docs/evidence/g01-recovery-packet.md").read_text(encoding="utf-8")
wrapper_start = packet.index("\nimport hashlib\n", packet.index("go_test_checked()")) + 1
wrapper_end = packet.index("\nPY\n}", wrapper_start)
wrapper = packet[wrapper_start:wrapper_end]
helper_start = wrapper.index("def skip_source_ignored")
helper_end = wrapper.index("\nall_test_names = []", helper_start)
namespace = {"label": "comment-green"}
exec(compile(wrapper[helper_start:helper_end], "<source-name>", "exec"), namespace)
source_names = namespace["source_test_names"]
real_run = subprocess.run
go_children = []

def reject_go_child(*args, **kwargs):
    command = args[0] if args else kwargs.get("args", [])
    if command and command[0] == "go":
        go_children.append(tuple(command))
        raise AssertionError("source-name helper started a Go child")
    return real_run(*args, **kwargs)

subprocess.run = reject_go_child
try:
    with TemporaryDirectory() as directory:
        source_path = Path(directory) / "comments_test.go"
        source_path.write_text(
            "package p\n"
            "import \"testing\"\n"
            "func\n"
            "// first line\n"
            "/* middle block */\n"
            "// second line\n"
            "TestHidden(t *testing.T) {}\n",
            encoding="utf-8",
        )
        names = source_names(source_path)
finally:
    subprocess.run = real_run
if names != ["TestHidden"]:
    raise SystemExit(f"source-name comment regression discovered {names!r}")
if go_children:
    raise SystemExit(f"source-name helper started Go child(ren): {go_children!r}")
print(
    "source-name comment regression: passed; TestHidden discovered after "
    "consecutive line/block comments with no Go child"
)
PY
```

#### Root 4001254025: unsupported POSIX regexp classes

The immutable red witness demonstrates a set mismatch for `[[:xdigit:]]`.
The prior helper left this nested class untranslated; Python consequently
derived a set unlike Go's ASCII `[0-9A-Fa-f]` class:

```sh
set -euo pipefail
[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || { printf '%s\n' 'reviewed canonical PATH and absolute Python interpreter required' >&2; exit 1; }
/opt/homebrew/bin/python3 -I - <<'PY'
import re
import subprocess
import warnings

starting_head = "423d4fc501120a014e63f77d3ef6652606d0326a"
previous = subprocess.check_output(
    [
        "git",
        "show",
        f"{starting_head}:docs/evidence/g01-recovery-packet.md",
    ],
    text=True,
)
wrapper_start = previous.index("\nimport hashlib\n", previous.index("go_test_checked()")) + 1
wrapper_end = previous.index("\nPY\n}", wrapper_start)
previous_wrapper = previous[wrapper_start:wrapper_end]
helper_start = previous_wrapper.index("def reject_python_semantic_regexp_constructs")
guard_call = previous_wrapper.index("\nsource_fuzz_guard()", helper_start)
go_start = previous_wrapper.index("def go_compatible_regexp", helper_start)
go_end = previous_wrapper.index("\nall_test_names = []", go_start)
namespace = {"label": "prior-posix-red", "re": re}
exec(
    compile(
        previous_wrapper[helper_start:guard_call] + previous_wrapper[go_start:go_end],
        "<prior-regexp>",
        "exec",
    ),
    namespace,
)
with warnings.catch_warnings():
    warnings.simplefilter("ignore", FutureWarning)
    go_regexp = namespace["go_compatible_regexp"]
    compiled = go_regexp(r"[[:xdigit:]]", "-run")
python_matches = [value for value in ["A", "g"] if compiled.search(value)]
go_expected = ["A"]
if python_matches == go_expected:
    raise SystemExit("red reproduction setup changed: prior POSIX helper matched Go xdigit")
print(
    f"RED regexp POSIX mismatch: prior {starting_head} left [[:xdigit:]] "
    f"untranslated; Python matched {python_matches!r}, while Go xdigit expects "
    f"{go_expected!r}"
)
PY
```

Recorded red output:

```text
RED regexp POSIX mismatch: prior 423d4fc501120a014e63f77d3ef6652606d0326a left [[:xdigit:]] untranslated; Python matched [], while Go xdigit expects ['A']
```

The current regexp preflight rejects every standard POSIX class outside the
explicit translated table before Python compilation and before package
metadata. The focused no-Go-child probe covers all six currently unsupported
classes, verifies each reviewed translation remains exact, and preserves both
escaped POSIX text and the previously reviewed literal backslash behavior:

```sh
set -euo pipefail
[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || { printf '%s\n' 'reviewed canonical PATH and absolute Python interpreter required' >&2; exit 1; }
/opt/homebrew/bin/python3 -I - <<'PY'
import re
import subprocess
from pathlib import Path

packet = Path("docs/evidence/g01-recovery-packet.md").read_text(encoding="utf-8")
wrapper_start = packet.index("\nimport hashlib\n", packet.index("go_test_checked()")) + 1
wrapper_end = packet.index("\nPY\n}", wrapper_start)
wrapper = packet[wrapper_start:wrapper_end]
preflight = wrapper.index("reject_unsupported_regexp_syntax(original_run_pattern")
metadata_guard = wrapper.index("test_source_paths = package_initialization_guard()")
if preflight > metadata_guard:
    raise SystemExit("POSIX regexp preflight moved after package metadata")
helper_start = wrapper.index("def reject_python_semantic_regexp_constructs")
guard_call = wrapper.index("\nsource_fuzz_guard()", helper_start)
go_start = wrapper.index("def go_compatible_regexp", helper_start)
go_end = wrapper.index("\nall_test_names = []", go_start)
namespace = {"label": "posix-green", "re": re}
exec(
    compile(
        wrapper[helper_start:guard_call] + wrapper[go_start:go_end],
        "<regexp-guard>",
        "exec",
    ),
    namespace,
)
go_regexp = namespace["go_compatible_regexp"]
real_compile = re.compile
compile_calls = []

def recording_compile(pattern, *args, **kwargs):
    compile_calls.append(pattern)
    return real_compile(pattern, *args, **kwargs)

real_run = subprocess.run
go_children = []

def reject_go_child(*args, **kwargs):
    command = args[0] if args else kwargs.get("args", [])
    if command and command[0] == "go":
        go_children.append(tuple(command))
        raise AssertionError("regexp preflight started a Go child")
    return real_run(*args, **kwargs)

re.compile = recording_compile
subprocess.run = reject_go_child
try:
    unsupported = [
        r"[[:blank:]]",
        r"[[:cntrl:]]",
        r"[[:graph:]]",
        r"[[:print:]]",
        r"[[:punct:]]",
        r"[[:xdigit:]]",
    ]
    for pattern in unsupported:
        try:
            go_regexp(pattern, "-run")
        except SystemExit as error:
            if "unsupported POSIX regexp class" not in str(error):
                raise
        else:
            raise SystemExit(f"unsupported POSIX class was accepted: {pattern!r}")
    if compile_calls:
        raise SystemExit(f"unsupported POSIX class reached Python compile: {compile_calls!r}")

    reviewed = {
        r"[[:alnum:]]": "[A-Za-z0-9]",
        r"[[:alpha:]]": "[A-Za-z]",
        r"[[:digit:]]": "[0-9]",
        r"[[:lower:]]": "[a-z]",
        r"[[:upper:]]": "[A-Z]",
        r"[[:space:]]": r"[\t\n\r\f ]",
        r"[[:word:]]": "[A-Za-z0-9_]",
    }
    for pattern, expected in reviewed.items():
        compiled = go_regexp(pattern, "-run")
        if compiled.pattern != expected:
            raise SystemExit(
                f"reviewed POSIX translation changed for {pattern!r}: "
                f"{compiled.pattern!r}"
            )
    literal_posix = go_regexp(r"\[\[:xdigit:\]\]", "-run")
    if not literal_posix.fullmatch("[[:xdigit:]]"):
        raise SystemExit("escaped literal POSIX text was not preserved")
    literal_backslash = go_regexp(r"\\b", "-run")
    if not literal_backslash.fullmatch(r"\b"):
        raise SystemExit("escaped literal backslash was not preserved")
finally:
    re.compile = real_compile
    subprocess.run = real_run
if go_children:
    raise SystemExit(f"regexp preflight started Go child(ren): {go_children!r}")
print(
    "regexp POSIX preflight regression: passed; 6 unsupported classes rejected "
    "before Python compile/Go child, reviewed translations and escaped literals preserved"
)
PY
```

### Fresh exact-head P2 corrections at `f7e723d2bc9efdf2a4a345ae5ee1e76c03bb943e`

The four actionable Codex P2 roots below were reproduced against the immutable
packet blob at exact head `f7e723d2bc9efdf2a4a345ae5ee1e76c03bb943e` before
editing. The red probes extracted only the prior wrapper/scanner from
`git show <head>:docs/evidence/g01-recovery-packet.md`; they did not execute a
Go test body or live operation. The current-green probes then exercised the
edited packet helpers with synthetic subprocess guards, source files, shell
tokens and a FIFO; no Go test child, credential, private path or live resource
was used.

#### Root 4001378617: PATH and loader-affecting assignment prefixes

The immutable red probe observed the prior first Go child receiving a
command-prefix `PATH=/synthetic/bin` and `LD_PRELOAD=/synthetic/lib/libshim.dylib`.
That allowed a transparent executable named `go` earlier on PATH to run before
the reviewed metadata boundary. The corrected wrapper rejects PATH plus the
reviewed Linux/macOS loader-affecting assignment names immediately after
prefix parsing and before any guarded Go subprocess.

Recorded red output:

```text
RED PATH/loader-prefix gap: prior f7e723d2bc9efdf2a4a345ae5ee1e76c03bb943e forwarded PATH='/synthetic/bin' and LD_PRELOAD='/synthetic/lib/libshim.dylib' to first Go child ('go', 'env', 'GOFLAGS'); a transparent executable wrapper could run
```

Recorded current-green output:

```text
loader-prefix regression: passed; PATH and 13 equivalent LD_/DYLD_ assignments rejected before any Go child/result, and transparent go wrapper did not run
```

The boundary includes separated command-prefix assignments, an executable
transparent `go` wrapper that would leave a marker if reached, and the allowed
`GOTOOLCHAIN` assignment remains outside the rejection set. Final anchor:
[PATH/loader boundary](#root-4001378617-path-and-loader-affecting-assignment-prefixes).

#### Root 4001378618: Go Unicode lowercase predicate for test names

The immutable red probe used the prior Python `str.islower()` predicate and
showed that it omitted `Testª`, although Go's `unicode.IsLower(U+00AA)` is false.
The corrected source-name helper uses the Unicode lowercase-letter (`Ll`)
category, matching cmd/go's `isTest` rule: an empty suffix or a first suffix
rune that is not lowercase is accepted, including uppercase, titlecase and
other non-lowercase runes.

Recorded red output:

```text
RED Unicode Test-name gap: prior f7e723d2bc9efdf2a4a345ae5ee1e76c03bb943e used Python islower and omitted Testª (Go unicode.IsLower(U+00AA)=false); observed ['Testǅ', 'Testƻ', 'Test中'], expected ['Testǅ', 'Testƻ', 'Test中', 'Testª']
```

Recorded current-green output:

```text
source-name Unicode regression: passed; Testª/title/other non-lowercase runes accepted, lowercase Testa/Testǆ rejected, no Go child
```

The source-only boundary retained `Test`, `Test1`, `Test_Foo`, ASCII
uppercase, titlecase `ǅ`, other-letter `ƻ`/`中`, and `Testª`, while rejecting
lowercase-initial `Testa` and `Testǆ`; no Go child or test body ran. Final
anchor: [Go Unicode test-name boundary](#root-4001378618-go-unicode-lowercase-predicate-for-test-names).

#### Root 4001378622: recursive shell command-string scan

The immutable red probe showed that the prior executable-token scan accepted
direct `bash -c`/`sh -c` and nested shell command strings without inspecting
their payloads. The corrected static scanner recognizes absolute and optioned
shells, `busybox sh`, wrapper prefixes and nested command strings, recursively
examining payloads and rejecting the command-string form before any nested
payload could execute.

Recorded red output:

```text
RED nested-shell command gap: prior f7e723d2bc9efdf2a4a345ae5ee1e76c03bb943e accepted direct bash/sh -c and nested shell command strings without recursively inspecting their live command payloads
```

Recorded current-green output:

```text
nested-shell forbidden-command regression: passed; direct/absolute/optioned/env/busybox and nested bash/sh -c forms rejected; safe printf boundary retained
```

The focused boundary covered direct bash/sh, `/bin/bash -xc`, env-wrapped
`/bin/sh --command`, `busybox sh -c`, and nested `bash -c "sh -c ..."`, while
retaining a safe direct `printf` result. Final anchor: [recursive shell
command-string boundary](#root-4001378622-recursive-shell-command-string-scan).

#### Root 4001378624: cleanliness gate before source reads

The immutable red probe created an ignored FIFO named `stuck_test.go` and ran
the prior source-fuzz scan in a bounded child; it blocked while opening the
candidate before the cleanliness gate. The corrected order performs the
immutable module-tree and `git status --ignored=matching` gate first, then
opens candidate source files only after a clean result. A synthetic ignored
FIFO status record is rejected before read, metadata/Go child or blocking.

Recorded red output:

```text
RED cleanliness/source-read gap: prior f7e723d2bc9efdf2a4a345ae5ee1e76c03bb943e opened ignored candidate FIFO stuck_test.go before the status gate; bounded probe blocked in source scan
```

Recorded current-green output:

```text
FIFO cleanliness regression: passed; ignored stuck_test.go rejected before read, metadata/Go child, or blocking
```

The green probe verifies the candidate is a real FIFO, status reports it as
ignored, the candidate `read_text` hook is never reached, and no Go subprocess
is started. Final anchor: [cleanliness-before-source-read boundary](#root-4001378624-cleanliness-gate-before-source-reads).

### Fresh exact-head P2 correction at `01764bbed0a387129d2a2abbc9e27a87e073f87e`

The fresh Codex finding below was reproduced against the immutable packet blob
at the exact starting head before editing. The red probe stops at the first Git
query after injecting synthetic repository/worktree/index overrides; it does
not run the metadata query, selector parser, test body or live operation. The
current green probe exercises inherited and command-prefix forms for the
repository-control namespace, patches both Git and Go child launches, and
requires rejection before immutable-tree/status validation.

#### Root 4001471783: Git repository-control environment overrides

The immutable red witness shows that the prior wrapper performed
`git rev-parse --show-toplevel` before parsing or rejecting environment
assignments. `GIT_WORK_TREE`, `GIT_DIR` and `GIT_INDEX_FILE` therefore reached
the first Git query and could redirect worktree, repository or index semantics;
the same ordering left related `GIT_*` repository, object, config, namespace,
discovery, pathspec and replacement controls unchecked.

```sh
set -euo pipefail
[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || { printf '%s\n' 'reviewed canonical PATH and absolute Python interpreter required' >&2; exit 1; }
/opt/homebrew/bin/python3 -I - <<'PY'
import os
import subprocess
import sys

starting_head = "01764bbed0a387129d2a2abbc9e27a87e073f87e"
previous = subprocess.check_output(
    [
        "git",
        "show",
        f"{starting_head}:docs/evidence/g01-recovery-packet.md",
    ],
    text=True,
)
wrapper_start = previous.index("\nimport hashlib\n", previous.index("go_test_checked()")) + 1
wrapper_end = previous.index("\nPY\n}", wrapper_start)
previous_wrapper = previous[wrapper_start:wrapper_end]
if "Git repository-control environment overrides are not allowed" in previous_wrapper:
    raise SystemExit("red reproduction setup changed: starting wrapper already rejected Git environment")

class StopAtGit(Exception):
    pass

saved = {key: os.environ.get(key) for key in ("GIT_WORK_TREE", "GIT_DIR", "GIT_INDEX_FILE")}
os.environ["GIT_WORK_TREE"] = "/synthetic/work-tree"
os.environ["GIT_DIR"] = "/synthetic/repository"
os.environ["GIT_INDEX_FILE"] = "/synthetic/index"
real_check_output = subprocess.check_output
git_calls = []

def stop_at_git(*args, **kwargs):
    command = args[0] if args else kwargs.get("args", [])
    if command and command[0] == "git":
        git_calls.append((tuple(command), {key: os.environ.get(key) for key in saved}))
        raise StopAtGit
    return real_check_output(*args, **kwargs)

subprocess.check_output = stop_at_git
try:
    sys.argv = [
        "wrapper-probe", "1",
        "f" * 64,
        "red-git-environment",
        "experiments/g01-scaleset:./livecanary",
        "default+race+cgo1+go1.26.8",
        "GIT_WORK_TREE=/synthetic/work-tree",
        "GIT_DIR=/synthetic/repository",
        "GIT_INDEX_FILE=/synthetic/index",
        "GOTOOLCHAIN=go1.26.8", "go", "test", "-C", "experiments/g01-scaleset",
        "-race", "-count=1", "-timeout=45s", "./livecanary",
        "-run=^TestNoMessageDoesNotCountAsCompletedBarrier$",
    ]
    try:
        exec(compile(previous_wrapper, "<prior-wrapper>", "exec"), {"__name__": "__main__"})
    except StopAtGit:
        pass
finally:
    subprocess.check_output = real_check_output
    for key, value in saved.items():
        if value is None:
            os.environ.pop(key, None)
        else:
            os.environ[key] = value

if not git_calls or git_calls[0][0] != ("git", "rev-parse", "--show-toplevel"):
    raise SystemExit(f"red reproduction did not reach first Git query: {git_calls!r}")
observed = git_calls[0][1]
if observed != {
    "GIT_WORK_TREE": "/synthetic/work-tree",
    "GIT_DIR": "/synthetic/repository",
    "GIT_INDEX_FILE": "/synthetic/index",
}:
    raise SystemExit(f"red reproduction did not expose all Git overrides: {observed!r}")
print(
    f"RED Git repository-control environment gap: prior {starting_head} reached git rev-parse --show-toplevel with GIT_WORK_TREE/GIT_DIR/GIT_INDEX_FILE overrides before parsing or rejecting them; immutable-tree validation was not trustworthy"
)
PY
```

Recorded red output:

```text
RED Git repository-control environment gap: prior 01764bbed0a387129d2a2abbc9e27a87e073f87e reached git rev-parse --show-toplevel with GIT_WORK_TREE/GIT_DIR/GIT_INDEX_FILE overrides before parsing or rejecting them; immutable-tree validation was not trustworthy
```

The minimum correction parses assignments before the first Git query, rejects
all inherited or command-prefix repository-control `GIT_*` variables, and
passes the resulting environment explicitly to the repository-root query. The
same namespace rejection is mirrored in the package/build metadata audit. This
conservative fail-closed rule covers the named worktree/repository/index
variables and related repository, object, config, namespace, discovery,
pathspec and replacement controls using one shared reviewed name/prefix set.
The boundary probe below uses only synthetic values and rejects every case
before any Git or Go child, output, immutable-tree check or source read:

```sh
set -euo pipefail
[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || { printf '%s\n' 'reviewed canonical PATH and absolute Python interpreter required' >&2; exit 1; }
/opt/homebrew/bin/python3 -I - <<'PY'
import io
import os
import subprocess
import sys
from contextlib import redirect_stdout
from pathlib import Path

packet = Path("docs/evidence/g01-recovery-packet.md").read_text(encoding="utf-8")
wrapper_start = packet.index("\nimport hashlib\n", packet.index("go_test_checked()")) + 1
wrapper_end = packet.index("\nPY\n}", wrapper_start)
wrapper = packet[wrapper_start:wrapper_end]
if wrapper.index("git_environment_overrides") > wrapper.index('["git", "rev-parse"'):
    raise SystemExit("Git environment guard moved after immutable-tree Git query")

inherited_names = [
    "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_INDEX_VERSION",
    "GIT_COMMON_DIR", "GIT_OBJECT_DIRECTORY", "GIT_OBJECT_DIRECTORY_RELATIVE",
    "GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_NAMESPACE",
    "GIT_CEILING_DIRECTORIES", "GIT_DISCOVERY_ACROSS_FILESYSTEM",
    "GIT_CONFIG", "GIT_CONFIG_GLOBAL", "GIT_CONFIG_SYSTEM",
    "GIT_CONFIG_NOSYSTEM", "GIT_CONFIG_COUNT", "GIT_CONFIG_PARAMETERS",
    "GIT_CONFIG_KEY_0", "GIT_CONFIG_VALUE_0", "GIT_LITERAL_PATHSPECS",
    "GIT_GLOB_PATHSPECS", "GIT_NOGLOB_PATHSPECS", "GIT_OPTIONAL_LOCKS",
    "GIT_REPLACE_REF_BASE", "GIT_NO_REPLACE_OBJECTS", "GIT_ATTR_NOSYSTEM",
    "GIT_QUARANTINE_PATH",
]
assignment_names = ["GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE"]
base = [
    "GOTOOLCHAIN=go1.26.8", "go", "test", "-C", "experiments/g01-scaleset",
    "-race", "-count=1", "-timeout=45s", "./livecanary",
    "-run=^TestNoMessageDoesNotCountAsCompletedBarrier$",
]
digest = "f" * 64
saved_git = {key: os.environ.get(key) for key in list(os.environ) if key.startswith("GIT_")}
real_check_output = subprocess.check_output
real_run = subprocess.run
children = []

def reject_check_output(*args, **kwargs):
    command = args[0] if args else kwargs.get("args", [])
    if command and command[0] in {"git", "go"}:
        children.append(("check_output", tuple(command)))
        raise AssertionError("Git/Go child started before Git environment guard")
    return real_check_output(*args, **kwargs)

def reject_run(*args, **kwargs):
    command = args[0] if args else kwargs.get("args", [])
    if command and command[0] in {"git", "go"}:
        children.append(("run", tuple(command)))
        raise AssertionError("Git/Go child started before Git environment guard")
    return real_run(*args, **kwargs)

subprocess.check_output = reject_check_output
subprocess.run = reject_run
try:
    for key in list(os.environ):
        if key.startswith("GIT_"):
            os.environ.pop(key)
    cases = [(f"inherited-{key}", None, key) for key in inherited_names]
    cases.extend((f"assignment-{key}", key, None) for key in assignment_names)
    for label, assignment, inherited in cases:
        if inherited is not None:
            os.environ[inherited] = "/synthetic/" + inherited.lower()
        command = ([assignment + "=/synthetic/command"] if assignment else []) + base
        sys.argv = [
            "wrapper-probe", "1", digest, label,
            "experiments/g01-scaleset:./livecanary",
            "default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8", *command,
        ]
        output = io.StringIO()
        try:
            with redirect_stdout(output):
                exec(compile(wrapper, "<current-wrapper>", "exec"), {"__name__": "__main__"})
        except SystemExit as error:
            if output.getvalue():
                raise SystemExit(f"{label}: guard emitted a result before rejection")
            if "Git repository-control environment overrides are not allowed" not in str(error):
                raise SystemExit(f"{label}: rejected for wrong reason: {error}")
        else:
            raise SystemExit(f"{label}: Git override was unexpectedly accepted")
        if inherited is not None:
            os.environ.pop(inherited, None)
finally:
    subprocess.check_output = real_check_output
    subprocess.run = real_run
    for key in list(os.environ):
        if key.startswith("GIT_"):
            os.environ.pop(key)
    for key, value in saved_git.items():
        if value is not None:
            os.environ[key] = value
if children:
    raise SystemExit(f"Git environment guard started child(ren): {children!r}")
print(
    f"Git repository-control environment regression: passed; {len(inherited_names)} inherited and {len(assignment_names)} command-prefix repository-control GIT_* overrides rejected before any git/go child or immutable-tree/status validation"
)
PY
```

Recorded current-green output:

```text
Git repository-control environment regression: passed; 27 inherited and 3 command-prefix repository-control GIT_* overrides rejected before any git/go child or immutable-tree/status validation
```

Final stable anchor: [Git repository-control environment boundary](#root-4001471783-git-repository-control-environment-overrides).

### Fresh exact-head P2 corrections at `4bd66186ea8d980a06ed8a4adf7f51e76b5028ef`

The three fresh Codex P2 roots below were reproduced against the immutable
starting packet head before this edit. Each red witness is source-extracted or
static and stops before a Go child, test body, `go test -list`, credential,
private path or live operation. The minimal green corrections then harden the
wrapper/scanner and add focused failure-boundary probes; all results remain
offline evidence only.

<a id="root-4001593870-compiler-and-cgo-tool-environment-overrides"></a>
#### Root 4001593870: compiler and cgo-tool environment overrides

The immutable red witness loaded the prior wrapper from the exact starting head,
injected synthetic inherited `CC`, `CXX`, `GCCGO` and `CGO_*` values, and stopped
at its first `go env GOFLAGS` child. The prior wrapper forwarded all nine
synthetic compiler/cgo-tool values; it did not reject them before Go metadata.

```text
RED compiler/cgo-tool environment gap: prior 4bd66186ea8d980a06ed8a4adf7f51e76b5028ef forwarded 9 inherited CC/CXX/GCCGO/CGO_* overrides to first go env GOFLAGS child
```

The immutable red command was:

```sh
set -euo pipefail
[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || { printf '%s\n' 'reviewed canonical PATH and absolute Python interpreter required' >&2; exit 1; }
/opt/homebrew/bin/python3 -I - <<'PY'
import io
import os
import subprocess
import sys
from contextlib import redirect_stdout

starting_head = "4bd66186ea8d980a06ed8a4adf7f51e76b5028ef"
previous = subprocess.check_output(
    ["git", "show", f"{starting_head}:docs/evidence/g01-recovery-packet.md"],
    text=True,
)
start = previous.index("\nimport hashlib\n", previous.index("go_test_checked()")) + 1
end = previous.index("\nPY\n}", start)
previous_wrapper = previous[start:end]
if "compiler_tool_environment_overrides" in previous_wrapper:
    raise SystemExit("red setup changed: starting wrapper already had compiler guard")
names = [
    "CC", "CXX", "GCCGO", "CGO_CFLAGS", "CGO_CPPFLAGS", "CGO_CXXFLAGS",
    "CGO_FFLAGS", "CGO_LDFLAGS", "CGO_PKG_CONFIG",
]
saved = {key: os.environ.get(key) for key in names}
saved_git = {key: os.environ.get(key) for key in os.environ if key.startswith("GIT_")}
for key in list(os.environ):
    if key.startswith("GIT_"):
        os.environ.pop(key)
for key in names:
    os.environ[key] = "/synthetic/" + key.lower()
real_run = subprocess.run
go_children = []
class StopAtGo(Exception):
    pass
def stop_at_go(*args, **kwargs):
    command = args[0] if args else kwargs.get("args", [])
    if command and command[0] == "go":
        go_children.append((tuple(command), kwargs.get("env", {})))
        raise StopAtGo
    return real_run(*args, **kwargs)
subprocess.run = stop_at_go
try:
    sys.argv = [
        "wrapper-probe", "1", "f" * 64, "red-compiler-tools",
        "experiments/g01-scaleset:./livecanary", "default+race+cgo1+go1.26.8",
        "GOTOOLCHAIN=go1.26.8", "go", "test", "-C", "experiments/g01-scaleset",
        "-race", "-count=1", "-timeout=45s", "./livecanary",
        "-run=^TestNoMessageDoesNotCountAsCompletedBarrier$",
    ]
    try:
        with redirect_stdout(io.StringIO()):
            exec(compile(previous_wrapper, "<prior-wrapper>", "exec"), {"__name__": "__main__"})
    except StopAtGo:
        pass
finally:
    subprocess.run = real_run
    for key, value in saved.items():
        if value is None:
            os.environ.pop(key, None)
        else:
            os.environ[key] = value
    for key in list(os.environ):
        if key.startswith("GIT_"):
            os.environ.pop(key)
    os.environ.update(saved_git)
if not go_children or go_children[0][0] != ("go", "env", "GOFLAGS"):
    raise SystemExit(f"red compiler child witness missing: {go_children!r}")
if any(go_children[0][1].get(key) is None for key in names):
    raise SystemExit("red compiler child did not receive every synthetic override")
print("RED compiler/cgo-tool environment gap: prior wrapper forwarded 9 inherited overrides to first go env child")
PY
```

The minimum correction parses command-prefix assignments before any child,
rejects inherited and command-prefix `CC`, `CXX`, `GCCGO`, `GOGCCFLAGS`, linker
mode/tool variables, pkg-config selectors and every `CGO_*` variable except the
separately pinned `CGO_ENABLED`, then mirrors the same reviewed set in the
package/build metadata audit. The existing `CGO_ENABLED=1` pin remains the
effective mode; the refactor adds `cgo-tools-default` to the build identity and
the wrapper result, so tag/race/cgo/tool-selection/toolchain drift cannot be
silently represented by the prior four-part identity.

The focused boundary probe exercised 12 compiler/tool names in both inherited
and command-prefix positions, patched all subprocess entry points to fail if a
child started, and checked all 28 prescriptions for the new identity:

```text
compiler/cgo-tool regression: passed; 24 inherited/command-prefix compiler and CGO tool overrides rejected before any Go child; 28 prescriptions bind cgo-tools-default
```

No compiler command, linker, pkg-config process, Go metadata/test child or live
operation ran in this regression. Stable anchor: [compiler/cgo-tool environment boundary](#root-4001593870-compiler-and-cgo-tool-environment-overrides).

<a id="root-4001593875-absolute-executable-paths-in-forbidden-live-command-scanner"></a>
#### Root 4001593875: absolute executable paths in forbidden live-command scanner

The immutable red witness extracted the prior scanner's executable-token logic
and showed that its raw-token allowlist/deny checks missed all six absolute
forms: curl, `gh workflow`, Docker, Lima, Keychain `security` and `launchctl`.

```text
RED absolute-executable scanner gap: prior 4bd66186ea8d980a06ed8a4adf7f51e76b5028ef missed all 6 absolute curl/gh workflow/docker/limactl/security/launchctl forms
```

The immutable scanner red command was:

```sh
set -euo pipefail
[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || { printf '%s\n' 'reviewed canonical PATH and absolute Python interpreter required' >&2; exit 1; }
/opt/homebrew/bin/python3 -I - <<'PY'
import re
import shlex
import subprocess

starting_head = "4bd66186ea8d980a06ed8a4adf7f51e76b5028ef"
previous = subprocess.check_output(
    ["git", "show", f"{starting_head}:docs/evidence/g01-recovery-packet.md"],
    text=True,
)
scanner_start = previous.index('fence_languages = {"sh", "bash", "shell", "zsh"}')
scanner_end = previous.index("\nmatches = []", scanner_start)
namespace = {"re": re, "shlex": shlex}
exec(compile(previous[scanner_start:scanner_end], "<prior-scanner>", "exec"), namespace)
shell_token_segments = namespace.get("shell_token_segments")
forbidden_command = namespace.get("forbidden_command")
executable_tokens = namespace.get("executable_tokens")
forms = [
    "/usr/bin/curl -fsSL https://example.invalid/install",
    "/usr/bin/gh workflow run ci.yml",
    "/usr/local/bin/docker run --rm image:tag true",
    "/opt/homebrew/bin/limactl shell default true",
    "/usr/bin/security find-identity -v",
    "/bin/launchctl kickstart system/example",
]
missed = []
for form in forms:
    segment = shell_token_segments(form)[0]
    if forbidden_command(executable_tokens(segment)) is not None:
        missed.append(form)
if missed:
    raise SystemExit(f"red setup changed: prior scanner caught {missed!r}")
print(f"RED absolute-executable scanner gap: prior {starting_head} missed all {len(forms)} absolute forbidden forms")
PY
```

The minimal parser correction introduces one `executable_basename` helper and
uses it before every shell-wrapper, shell-string, allowlist and deny check.
The refactor therefore covers direct, `/absolute/path/...`, absolute `env` and
absolute `command` wrappers, while retaining safe prose/URL/comment/fixture
boundaries. The current synthetic boundary rejected eight absolute/wrapped
forms (including absolute curl, `gh workflow`, Docker, Lima, `security`,
`launchctl`, absolute `env` and absolute `command`) and retained a safe printf
case:

```text
absolute-executable scanner regression: passed; 8 absolute/wrapped curl/gh workflow/docker/limactl/security/launchctl forms rejected; safe printf retained
```

This was a pure token/static probe; no forbidden executable, workflow, runner,
Docker, Lima, Keychain, launchd or fetch operation ran. Stable anchor:
[absolute-executable scanner boundary](#root-4001593875-absolute-executable-paths-in-forbidden-live-command-scanner).

<a id="root-4001593877-unfiltered-go-test-prescriptions"></a>
#### Root 4001593877: unfiltered `go test` prescriptions

The immutable red witness applied the prior selector-only audit to an
executable `go test ./livecanary` with no `-run` or `-skip`; the selector regex
did not discover it and the wrapper-only count stayed unchanged.

```text
RED unfiltered go-test audit gap: prior 4bd66186ea8d980a06ed8a4adf7f51e76b5028ef ignored executable `go test ./livecanary` without -run/-skip; wrapper-only count stayed at 28
```

The immutable selector-audit red command was:

```sh
set -euo pipefail
[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || { printf '%s\n' 'reviewed canonical PATH and absolute Python interpreter required' >&2; exit 1; }
/opt/homebrew/bin/python3 -I - <<'PY'
import re
import subprocess

starting_head = "4bd66186ea8d980a06ed8a4adf7f51e76b5028ef"
previous = subprocess.check_output(
    ["git", "show", f"{starting_head}:docs/evidence/g01-recovery-packet.md"],
    text=True,
)
old_selector = re.compile(
    r"^\s*(?:(?:[A-Za-z_][A-Za-z0-9_]*=[^\s]+)\s+)*"
    r"(?:env\s+.*\s+)?(?:command(?:\s+-[^\s]+)*\s+)?go test\b.*\s--?(?:run|skip)(?:=|\s)"
)
probe = "go test ./livecanary"
if old_selector.search(probe):
    raise SystemExit("red setup changed: prior selector regex already matched unfiltered go test")
wrapper_count = sum(
    line.lstrip().startswith("go_test_checked ")
    for line in previous.splitlines()
)
if sum(line.lstrip().startswith("go_test_checked ") for line in previous.splitlines() + [probe]) != wrapper_count:
    raise SystemExit("red setup changed: wrapper-only count saw synthetic unfiltered command")
print(
    f"RED unfiltered go-test audit gap: prior {starting_head} ignored executable `go test ./livecanary` without -run/-skip; wrapper-only count stayed at {wrapper_count}"
)
PY
```

The minimum correction adds an exhaustive shell-fence audit that joins
continuations, removes quoted payloads before raw executable discovery, skips
Python heredocs/prose/comments, discovers every executable `go test` command
whether filtered or unfiltered, and accepts only `go_test_checked` commands or
an explicitly marked `non-prescription fixture`. The refactor keeps the
source-derived count/digest, exact package/build identity, environment and
JSON-stream guards as the only prescription path and adds a synthetic
unfiltered failure witness.

```text
all-executable-go-test regression: passed; every executable go test discovered (28/28) is go_test_checked; 0 fixtures; direct and nested synthetic unfiltered commands rejected
```

The exhaustive audit found 28/28 guarded executable prescriptions and no
unclassified fixture; its synthetic unfiltered command was rejected before any
Go child. No test body, `go test -list`, live operation, credential or private
path was used. Stable anchor: [unfiltered go-test prescription boundary](#root-4001593877-unfiltered-go-test-prescriptions).

### Fresh exact-head P2 corrections at `943ebece04882a0faf055d73e5988bd8954088f8`

The three fresh Codex P2 findings were reproduced against this immutable
starting head before the correction. The red witness is source-extracted or
static and stops before any Go child, test body, `go test -list`, credential,
private path or live operation. The minimal green correction then binds the
deadline, experiment setting and scanner boundary in the reusable packet path;
the focused regression below remains offline/static evidence only.

<a id="fresh-p2-go-child-deadline"></a>
#### Independent deadline for every Go child

The immutable wrapper had three direct Go child call sites: effective
`go env GOFLAGS`, package `go list -json -test` metadata, and final `go test`
execution. None passed an independent `subprocess` deadline, so Go toolchain
selection/downloads, compilation, metadata and test execution were bounded only
by the in-process `-timeout` flag (which does not cover those earlier phases).

```text
RED fresh P2 probes: 3 Go child calls lack independent timeout; GOEXPERIMENT is unbound/unidentified; eval wrapper escaped scanner (['eval', '$payload'])
```

The immutable red command was:

```sh
set -euo pipefail
[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || { printf '%s\n' 'reviewed canonical PATH and absolute Python interpreter required' >&2; exit 1; }
/opt/homebrew/bin/python3 -I - <<'PY'
import ast
import shlex
import subprocess
from pathlib import Path

starting_head = "943ebece04882a0faf055d73e5988bd8954088f8"
previous = subprocess.check_output(
    ["git", "show", f"{starting_head}:docs/evidence/g01-recovery-packet.md"],
    text=True,
)
start = previous.index("\nimport hashlib\n", previous.index("go_test_checked()")) + 1
end = previous.index("\nPY\n}", start)
wrapper = previous[start:end]
tree = ast.parse(wrapper)
missing_deadline = []
for node in ast.walk(tree):
    if not isinstance(node, ast.Call) or not isinstance(node.func, ast.Attribute):
        continue
    if node.func.attr not in {"run", "check_output"} or not node.args:
        continue
    command = node.args[0]
    try:
        values = ast.literal_eval(command)
    except Exception:
        values = None
    is_go = isinstance(values, list) and values and values[0] == "go"
    is_dynamic_go = isinstance(command, ast.Name) and command.id in {"run_command", "list_command"}
    if is_go or is_dynamic_go:
        if not any(keyword.arg == "timeout" for keyword in node.keywords):
            missing_deadline.append(node.lineno)
if len(missing_deadline) != 3:
    raise SystemExit(f"red setup changed: expected 3 unbounded Go calls, observed {missing_deadline}")
if "GOEXPERIMENT" in wrapper:
    raise SystemExit("red setup changed: immutable wrapper already mentions GOEXPERIMENT")
scanner_start = previous.find(
    'fence_languages = {"sh", "bash", "shell", "zsh"}',
    previous.find('fence_languages = {"sh", "bash", "shell", "zsh"}') + 1,
)
scanner_end = previous.index("\nmatches = []", scanner_start)
namespace = {"re": __import__("re"), "shlex": shlex}
exec(compile(previous[scanner_start:scanner_end], "<prior-scanner>", "exec"), namespace)
scanner_segments = namespace["shell_token_segments"]
scanner_forbidden = namespace["forbidden_command"]
scanner_executables = namespace["executable_tokens"]
segment = scanner_segments("eval \"$payload\"")[0]
observed = scanner_forbidden(scanner_executables(segment))
if observed is not None:
    raise SystemExit(f"red setup changed: prior scanner already rejected eval: {observed!r}")
print(
    "RED fresh P2 probes: 3 Go child calls lack independent timeout; "
    "GOEXPERIMENT is unbound/unidentified; eval wrapper escaped scanner "
    f"({segment!r})"
)
PY
```

<a id="fresh-p2-goexperiment-binding"></a>
#### Reviewed `GOEXPERIMENT` binding and build identity

The correction rejects any inherited or command-prefix experiment other than
the reviewed `GOEXPERIMENT=none`, binds that value before the first Go child,
passes it to metadata and execution, and records `goexperiment-none` plus the
reviewed target, GOROOT default and GOFIPS140 mode in every nine-part checked build identity. This makes an
experiment override a refusal, and makes the effective experiment setting and
target part of the prescription identity.

<a id="fresh-p2-eval-scanner"></a>
#### Fail-closed `eval` live-command scanner boundary

The correction rejects the `eval` executable basename at every scanner depth,
including absolute and shell-`-c`-nested forms. It therefore refuses both
`eval`-wrapped live commands and harmless `eval` strings, retaining a
fail-closed boundary instead of attempting to model shell expansion.

The focused offline AST/timeout/environment/scanner regression was:

```sh
set -euo pipefail
[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || { printf '%s\n' 'reviewed canonical PATH and absolute Python interpreter required' >&2; exit 1; }
/opt/homebrew/bin/python3 -I - <<'PY'
import ast
import os
import shlex
import subprocess
import sys
from pathlib import Path

packet = Path("docs/evidence/g01-recovery-packet.md").read_text(encoding="utf-8")
start = packet.index("\nimport hashlib\n", packet.index("go_test_checked()")) + 1
end = packet.index("\nPY\n}", start)
wrapper = packet[start:end]
tree = ast.parse(wrapper)
functions = {
    node.name: node
    for node in tree.body
    if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef))
}
if "run_go_child" not in functions:
    raise SystemExit("deadline regression: run_go_child helper missing")
deadline_assignments = [
    node
    for node in tree.body
    if isinstance(node, ast.Assign)
    and any(isinstance(target, ast.Name) and target.id == "go_child_deadline_seconds" for target in node.targets)
]
if not deadline_assignments or not isinstance(deadline_assignments[-1].value, ast.Constant) or deadline_assignments[-1].value.value != 300:
    raise SystemExit("deadline regression: reviewed 300s deadline binding missing")
go_child_calls = [
    node
    for node in ast.walk(tree)
    if isinstance(node, ast.Call)
    and isinstance(node.func, ast.Name)
    and node.func.id == "run_go_child"
]
if len(go_child_calls) != 5:
    raise SystemExit(f"deadline regression: expected 5 Go child helper calls, observed {len(go_child_calls)}")
helper = functions["run_go_child"]
if "terminate_go_child_group" not in functions:
    raise SystemExit("deadline regression: process-group terminator missing")
timeout_calls = [
    node
    for node in ast.walk(helper)
    if isinstance(node, ast.Call)
    and isinstance(node.func, ast.Attribute)
    and node.func.attr == "communicate"
    and any(
        keyword.arg == "timeout"
        and isinstance(keyword.value, ast.Name)
        and keyword.value.id == "go_child_deadline_seconds"
        for keyword in node.keywords
    )
]
if len(timeout_calls) != 1:
    raise SystemExit("deadline regression: Go helper does not pass the independent timeout")
if "start_new_session=True" not in ast.unparse(helper):
    raise SystemExit("deadline regression: Go helper does not own a new session")
terminator = functions["terminate_go_child_group"]
if not any(
    isinstance(node, ast.Call)
    and isinstance(node.func, ast.Attribute)
    and node.func.attr == "killpg"
    for node in ast.walk(terminator)
):
    raise SystemExit("deadline regression: timeout terminator does not kill the process group")
helper_namespace = {
    "os": os,
    "signal": __import__("signal"),
    "subprocess": subprocess,
    "label": "deadline-probe",
    "go_child_deadline_seconds": 300,
    "go_child_termination_grace_seconds": 5,
}
module = ast.Module(body=[terminator, helper], type_ignores=[])
exec(compile(module, "<go-child-helper>", "exec"), helper_namespace)
real_popen = subprocess.Popen
observed_timeout = []
session_values = []
class TimeoutPopen:
    pid = 99999999
    returncode = -15
    def communicate(self, *args, **kwargs):
        if "timeout" in kwargs:
            observed_timeout.append(kwargs["timeout"])
        if kwargs:
            raise subprocess.TimeoutExpired(["go", "env", "GOFLAGS"], kwargs["timeout"])
        return "", ""
    def wait(self):
        return self.returncode
def timeout_probe(*args, **kwargs):
    session_values.append(kwargs.get("start_new_session"))
    return TimeoutPopen()
subprocess.Popen = timeout_probe
run_go_child = helper_namespace["run_go_child"]
try:
    try:
        run_go_child(
            ["go", "env", "GOFLAGS"], cwd=".", env={}, text=True,
            capture_output=True, check=False,
        )
    except SystemExit as error:
        if "independent 300s deadline" not in str(error):
            raise SystemExit(f"deadline regression: wrong timeout refusal: {error}")
    else:
        raise SystemExit("deadline regression: timeout was not converted to refusal")
finally:
    subprocess.Popen = real_popen
if observed_timeout != [300, 5]:
    raise SystemExit(f"deadline regression: observed timeout {observed_timeout}")
if session_values != [True]:
    raise SystemExit(f"deadline regression: observed session ownership {session_values}")

record_builds = [
    line.split()[5]
    for line in packet.splitlines()
    if line.startswith("go_test_checked ")
]
if len(record_builds) != 28 or any(
    "+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+" not in build
    for build in record_builds
):
    raise SystemExit("GOEXPERIMENT/target/GOROOT/GOFIPS140 regression: not all 28 prescriptions bind reviewed settings")

prefix = wrapper[:wrapper.index("compiler_tool_environment_names =")]
base_command = [
    "go", "test", "-C", "experiments/g01-scaleset", "-race", "-count=1",
    "-timeout=45s", "./livecanary", "-run", "^TestProbe$",
]
def expect_prefix_rejection(argv, inherited=None):
    saved_argv = sys.argv
    saved_experiment = os.environ.get("GOEXPERIMENT")
    if inherited is None:
        os.environ.pop("GOEXPERIMENT", None)
    else:
        os.environ["GOEXPERIMENT"] = inherited
    sys.argv = argv
    called = []
    saved_run = subprocess.run
    subprocess.run = lambda *args, **kwargs: called.append(args[0])
    try:
        try:
            exec(compile(prefix, "<goexperiment-prefix>", "exec"), {"__name__": "__main__"})
        except SystemExit as error:
            if "GOEXPERIMENT" not in str(error):
                raise SystemExit(f"GOEXPERIMENT regression: wrong refusal: {error}")
        else:
            raise SystemExit("GOEXPERIMENT regression: conflicting value was accepted")
    finally:
        subprocess.run = saved_run
        sys.argv = saved_argv
        if saved_experiment is None:
            os.environ.pop("GOEXPERIMENT", None)
        else:
            os.environ["GOEXPERIMENT"] = saved_experiment
    if called:
        raise SystemExit(f"GOEXPERIMENT regression: child started before refusal: {called}")

build = "default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8"
expect_prefix_rejection(
    ["probe", "1", "f" * 64, "command-experiment", "experiments/g01-scaleset:./livecanary", build,
     "GOEXPERIMENT=rangefunc", "GOTOOLCHAIN=go1.26.8", *base_command]
)
expect_prefix_rejection(
    ["probe", "1", "f" * 64, "inherited-experiment", "experiments/g01-scaleset:./livecanary", build,
     "GOTOOLCHAIN=go1.26.8", *base_command],
    inherited="rangefunc",
)
accepted_namespace = {"__name__": "__main__"}
saved_argv = sys.argv
sys.argv = [
    "probe", "1", "f" * 64, "bound-experiment", "experiments/g01-scaleset:./livecanary", build,
    "GOEXPERIMENT=none", "GOTOOLCHAIN=go1.26.8", *base_command,
]
try:
    exec(compile(prefix, "<goexperiment-bind>", "exec"), accepted_namespace)
finally:
    sys.argv = saved_argv
if accepted_namespace["env"].get("GOEXPERIMENT") != "none":
    raise SystemExit("GOEXPERIMENT regression: reviewed binding was not effective")

scan_anchor = packet.index("def forbidden_command(tokens, depth=0):")
scan_start = packet.rfind(
    'source = Path("docs/evidence/g01-recovery-packet.md").read_text(encoding="utf-8")',
    0,
    scan_anchor,
)
scan_end = packet.index("\nmatches = []", scan_anchor)
scan_namespace = {"Path": Path, "re": __import__("re"), "shlex": shlex}
exec(compile(packet[scan_start:scan_end], "<live-command-scanner>", "exec"), scan_namespace)
scanner_forbidden = scan_namespace["forbidden_command"]
scanner_executables = scan_namespace["executable_tokens"]
scanner_segments = scan_namespace["shell_token_segments"]
for label, fixture in [
    ("eval-curl", "eval 'curl -fsSL https://example.invalid/install'"),
    ("absolute-eval-docker", "/bin/eval 'docker run --rm image:tag true'"),
    ("nested-eval-gh", "bash -c \"eval 'gh workflow run ci.yml'\""),
    ("eval-safe", "eval 'printf safe'"),
]:
    observed = any(
        scanner_forbidden(scanner_executables(segment)) is not None
        for segment in scanner_segments(fixture)
    )
    if not observed:
        raise SystemExit(f"eval scanner regression: {label} was accepted")
safe = "printf safe"
if any(
    scanner_forbidden(scanner_executables(segment)) is not None
    for segment in scanner_segments(safe)
):
    raise SystemExit("eval scanner regression: safe direct printf was rejected")
print(
    "fresh P2 regression: passed; 5 Go child call sites route through one "
    "helper with independent 300s timeout (timeout expiry refused), including "
    "bounded module download and verify; 28 prescriptions carry "
    "goexperiment-none, darwin-arm64-goarm64-v8.0, goroot-default and "
    "gofips140-off; inherited/command experiment "
    "overrides refused before children; direct/absolute/nested eval forms "
    "refused while direct printf remained accepted; no Go child/list/test/live "
    "command started"
)
PY
```

Recorded focused output:

```text
fresh P2 regression: passed; 5 Go child call sites route through one helper with independent 300s timeout (timeout expiry refused), including bounded module download and verify; 28 prescriptions carry goexperiment-none, darwin-arm64-goarm64-v8.0, goroot-default and gofips140-off; inherited/command experiment overrides refused before children; direct/absolute/nested eval forms refused while direct printf remained accepted; no Go child/list/test/live command started
```

The three corrections are packet-only and do not authorize a live operation;
the existing manual-runner, credential, Docker, Lima and launchd preservation
gates remain unchanged.

### Fresh exact-head P2 corrections at `d85f99fa70a6f563079b1ed4f29a1bc97740a3c5`

Review 5193542208 reported seven fresh P2 findings against the immutable
parent `d85f99fa70a6f563079b1ed4f29a1bc97740a3c5`. The red probes below
re-execute only extracted historical helpers or inspect historical source and
are deliberately stopped before any Go child, `go test -list`, test body,
credential, private path or live operation. The green probes exercise the
current packet's extracted wrapper/audits and Git intent-bit helper with
synthetic values only; their evidence is correction evidence, not a live test
result.

<a id="root-4001907037-persisted-goenv-compiler-settings"></a>
#### Root 4001907037: persisted `GOENV` compiler settings

The immutable wrapper accepted an inherited or command-prefix `GOENV` path and
reached its first Git lookup before disabling persisted Go configuration. The
current wrapper rejects every value except reviewed `GOENV=off`, binds that
value before the first child, and verifies it again in the Go-child
environment.

<a id="root-4001907044-inherited-executable-path"></a>
#### Root 4001907044: inherited executable `PATH`

The immutable wrapper accepted a synthetic inherited `PATH` and reached its
first Git lookup. The current wrapper requires the reviewed canonical
`/opt/homebrew/bin:/usr/bin:/bin` value before Git/Go lookup and passes that
same pinned value to every child.

<a id="root-4001907050-absolute-go-test-executable-discovery"></a>
#### Root 4001907050: absolute `go test` executable discovery

The immutable exhaustive audit compared the raw executable token to `go`, so
an absolute `/opt/homebrew/bin/go test` command was omitted. The current audit
normalizes executable basenames at every wrapper boundary, so absolute Go
paths are included in the same guarded-prescription audit.

<a id="root-4001907053-python-command-string-live-command-boundary"></a>
#### Root 4001907053: Python command-string live-command boundary

The immutable live-command scanner accepted `python3 -c` and `python -c`
payloads that can replay `gh`, Docker or other live commands. The current
scanner rejects direct, absolute and `env`-wrapped Python command strings,
including compact `-xc` forms, before the existing shell scanner proceeds.

<a id="root-4001907059-reviewed-target-in-build-identity"></a>
#### Root 4001907059: reviewed target in build identity

The immutable wrapper had no reviewed `GOOS`/`GOARCH`/`GOARM64` binding and no
target component in its build identity. The current wrapper rejects
unreviewed target feature settings, binds `GOOS=darwin`, `GOARCH=arm64` and
`GOARM64=v8.0` before children, and records
`darwin-arm64-goarm64-v8.0`, `goroot-default` and `gofips140-off` in every
nine-part identity.

<a id="root-4001907060-git-intent-bits-before-source-derivation"></a>
#### Root 4001907060: Git intent bits before source derivation

The immutable cleanliness gate could report an empty status for a source path
marked `skip-worktree` or `assume-unchanged`. The current gate queries
`git ls-files -v` after status and before metadata/source derivation, rejecting
the `S`, `s` and `h` intent-bit forms before any source file is read.

<a id="root-4001907063-immutable-urls-for-prior-fresh-p2-corrections"></a>
#### Root 4001907063: immutable URLs for prior fresh P2 corrections

The packet now retains immutable GitHub discussion URLs for the three prior
fresh P2 corrections. These URLs are ledger evidence only; they do not claim
that GitHub review metadata has been resolved.

#### Immutable-parent red probes

The historical probes produced these failures before the correction:

```text
RED 4001907037: prior wrapper accepted GOENV=/synthetic/go-env and reached first Git lookup; persisted compiler settings were not disabled before child lookup
RED 4001907044: prior wrapper accepted inherited PATH=/synthetic/bin:/usr/bin and reached first Git lookup; PATH was not pinned before Git/Go lookup
RED 4001907050: prior exhaustive audit missed /opt/homebrew/bin/go test as an executable go test command
RED 4001907053: prior live-command scanner accepted python3 -c command string capable of replaying gh
RED 4001907059: prior wrapper build identity had no reviewed GOOS/GOARCH or GOARM64 target binding
RED 4001907060: prior cleanliness gate saw empty status for a skip-worktree/assume-unchanged source entry and had no ls-files intent-bit check
RED 4001907063: prior packet URL ledger omitted immutable discussion URLs 4001767353, 4001767363 and 4001767365
```

The red extraction and assertions were run from the immutable parent with a
sanitized synthetic environment; the historical Git probe used a temporary
repository only and did not touch this worktree. The exact command is retained
below for reproducibility:

```sh
set -euo pipefail
[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || { printf '%s\n' 'reviewed canonical PATH and absolute Python interpreter required' >&2; exit 1; }
/opt/homebrew/bin/python3 -I - <<'PY'
import ast
import os
import shlex
import subprocess
import sys
import tempfile
from pathlib import Path

starting_head = "d85f99fa70a6f563079b1ed4f29a1bc97740a3c5"
packet_path = "docs/evidence/g01-recovery-packet.md"
previous = subprocess.check_output(
    ["git", "show", f"{starting_head}:{packet_path}"], text=True
)
wrapper_start = previous.index("\nimport hashlib\n", previous.index("go_test_checked()")) + 1
wrapper_end = previous.index("\nPY\n}", wrapper_start)
wrapper = previous[wrapper_start:wrapper_end]
ast.parse(wrapper)
six_part_build = "default+race+cgo1+cgo-tools-default+goexperiment-none+go1.26.8"
base_command = [
    "go", "test", "-C", "experiments/g01-scaleset", "-race", "-count=1",
    "-timeout=45s", "./livecanary", "-run=^TestProbe$",
]

def prior_first_child(inherited=None, assignments=()):
    saved_env = dict(os.environ)
    saved_argv = sys.argv
    saved_check_output = subprocess.check_output
    saved_run = subprocess.run
    calls = []
    class StopBeforeChild(Exception):
        pass
    def stop(*args, **kwargs):
        calls.append(args[0] if args else None)
        raise StopBeforeChild
    try:
        os.environ.clear()
        os.environ.update({"PATH": "/usr/bin", "LANG": "C"})
        if inherited:
            os.environ.update(inherited)
        sys.argv = [
            "probe", "1", "f" * 64, "historical",
            "experiments/g01-scaleset:./livecanary", six_part_build,
            *assignments, "GOTOOLCHAIN=go1.26.8", *base_command,
        ]
        subprocess.check_output = stop
        subprocess.run = stop
        try:
            exec(compile(wrapper, "<immutable-wrapper>", "exec"), {"__name__": "__main__"})
        except StopBeforeChild:
            pass
    finally:
        subprocess.check_output = saved_check_output
        subprocess.run = saved_run
        sys.argv = saved_argv
        os.environ.clear()
        os.environ.update(saved_env)
    return calls

goenv_calls = prior_first_child(assignments=("GOENV=/synthetic/go-env",))
if not goenv_calls:
    raise SystemExit("red setup changed: immutable GOENV probe did not reach a child")
print(
    "RED 4001907037: prior wrapper accepted GOENV=/synthetic/go-env and reached first Git lookup; "
    "persisted compiler settings were not disabled before child lookup"
)
path_calls = prior_first_child(inherited={"PATH": "/synthetic/bin:/usr/bin"})
if not path_calls:
    raise SystemExit("red setup changed: immutable PATH probe did not reach a child")
print(
    "RED 4001907044: prior wrapper accepted inherited PATH=/synthetic/bin:/usr/bin and reached first Git lookup; "
    "PATH was not pinned before Git/Go lookup"
)

audit_anchor = previous.index("def executable_shell_commands")
audit_end = previous.index("\nguarded, fixtures =", audit_anchor)
audit_ns = {"re": __import__("re"), "shlex": shlex}
exec(compile(previous[audit_anchor:audit_end], "<immutable-go-test-audit>", "exec"), audit_ns)
executable_go_tests = audit_ns["executable_go_tests"]
if executable_go_tests(["/opt/homebrew/bin/go", "test", "./livecanary"]):
    raise SystemExit("red setup changed: immutable audit already normalized absolute go")
print("RED 4001907050: prior exhaustive audit missed /opt/homebrew/bin/go test as an executable go test command")

scanner_start = previous.find(
    'fence_languages = {"sh", "bash", "shell", "zsh"}',
    previous.find('fence_languages = {"sh", "bash", "shell", "zsh"}') + 1,
)
scanner_end = previous.index("\nmatches = []", scanner_start)
scanner_ns = {"re": __import__("re"), "shlex": shlex}
exec(compile(previous[scanner_start:scanner_end], "<immutable-live-scanner>", "exec"), scanner_ns)
executable_tokens = scanner_ns["executable_tokens"]
shell_token_segments = scanner_ns["shell_token_segments"]
forbidden_command = scanner_ns["forbidden_command"]
python_tokens = executable_tokens(shell_token_segments("python3 -c 'import subprocess; subprocess.run([\"gh\", \"api\", \"x\"])'")[0])
if forbidden_command(python_tokens) is not None:
    raise SystemExit("red setup changed: immutable scanner already rejected python -c")
print("RED 4001907053: prior live-command scanner accepted python3 -c command string capable of replaying gh")

if any(name in wrapper for name in ("GOOS", "GOARCH", "GOARM64")):
    raise SystemExit("red setup changed: immutable wrapper already binds reviewed target")
print("RED 4001907059: prior wrapper build identity had no reviewed GOOS/GOARCH or GOARM64 target binding")

with tempfile.TemporaryDirectory() as directory:
    root = Path(directory)
    subprocess.run(["git", "init", "-q"], cwd=root, check=True, env={"PATH": "/usr/bin:/bin"})
    source = root / "source.go"
    source.write_text("package probe\n", encoding="utf-8")
    subprocess.run(["git", "add", "source.go"], cwd=root, check=True, env={"PATH": "/usr/bin:/bin"})
    subprocess.run(
        ["git", "-c", "user.name=probe", "-c", "user.email=probe@example.invalid", "commit", "-q", "-m", "source"],
        cwd=root, check=True, env={"PATH": "/usr/bin:/bin"},
    )
    for flag, expected in (("--skip-worktree", "S "), ("--assume-unchanged", "s ")):
        subprocess.run(["git", "update-index", flag, "source.go"], cwd=root, check=True, env={"PATH": "/usr/bin:/bin"})
        status = subprocess.check_output(
            ["git", "status", "--porcelain=v1", "--untracked-files=all", "--ignored=matching", "--", "source.go"],
            cwd=root, text=True, env={"PATH": "/usr/bin:/bin"},
        )
        intent = subprocess.check_output(
            ["git", "ls-files", "-v", "--full-name", "--", "source.go"],
            cwd=root, text=True, env={"PATH": "/usr/bin:/bin"},
        )
        if status.strip() or not intent.startswith(expected):
            raise SystemExit(f"red setup changed: intent-bit fixture was not hidden as expected for {flag}")
        subprocess.run(["git", "update-index", "--no-skip-worktree", "--no-assume-unchanged", "source.go"], cwd=root, check=True, env={"PATH": "/usr/bin:/bin"})
print("RED 4001907060: prior cleanliness gate saw empty status for a skip-worktree/assume-unchanged source entry and had no ls-files intent-bit check")

for finding in ("4001767353", "4001767363", "4001767365"):
    if finding in previous:
        raise SystemExit(f"red setup changed: prior packet already contains {finding}")
print("RED 4001907063: prior packet URL ledger omitted immutable discussion URLs 4001767353, 4001767363 and 4001767365")
PY
```

#### Minimal packet correction and focused boundary probes

The minimal correction is limited to this packet: it adds the early
`GOENV=off`/target/PATH bindings, the basename-normalized exhaustive audit,
the fail-closed Python command-string scanner, the Git intent-bit helper and
the immutable prior-correction URL ledger. The extracted/AST regression below
checks ordering before the first Git/Go lookup, accepted reviewed bindings,
rejected conflicts, absolute executable coverage, direct/absolute/`env`
Python `-c` rejection, safe-command retention, intent-bit rejection and all
three exact URLs. It does not invoke Go, `go test`, `go list`, a test body or a
live command.

```sh
set -euo pipefail
# g01-safe-python-heredoc: reviewed isolated interpreter argv
[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || { printf '%s\n' 'reviewed canonical PATH and absolute Python interpreter required' >&2; exit 1; }
/opt/homebrew/bin/python3 -I - <<'PY'
import ast
import os
import re
import shlex
import subprocess
import sys
import tempfile
from pathlib import Path

packet_path = Path("docs/evidence/g01-recovery-packet.md")
packet = packet_path.read_text(encoding="utf-8")
wrapper_start = packet.index("\nimport hashlib\n", packet.index("go_test_checked()")) + 1
wrapper_end = packet.index("\nPY\n}", wrapper_start)
wrapper = packet[wrapper_start:wrapper_end]
tree = ast.parse(wrapper)
if wrapper.index("reviewed_goenv = \"off\"") > wrapper.index("repo_root = Path("):
    raise SystemExit("GOENV regression: binding occurs after first Git lookup")
if wrapper.index("reviewed_path = \"/opt/homebrew/bin:/usr/bin:/bin\"") > wrapper.index("repo_root = Path("):
    raise SystemExit("PATH regression: validation occurs after first Git lookup")
if wrapper.index("reviewed_target_environment =") > wrapper.index("repo_root = Path("):
    raise SystemExit("target regression: binding occurs after first Git lookup")
if wrapper.index("git_source_control_entries(repo_root, module_dir, env)") > wrapper.index("list_command ="):
    raise SystemExit("intent-bit regression: source flag check occurs after metadata")

prefix_end = wrapper.index("repo_root = Path(")
prefix_ns = {"__name__": "__main__"}
base = [
    "probe", "1", "f" * 64, "current", "experiments/g01-scaleset:./livecanary",
    "default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8",
    "GOTOOLCHAIN=go1.26.8", "go", "test", "-C", "experiments/g01-scaleset", "-race",
    "-count=1", "-timeout=45s", "./livecanary", "-run=^TestProbe$",
]

def run_prefix(label, inherited=None, assignments=(), expect_error=False):
    saved_env, saved_argv = dict(os.environ), sys.argv
    saved_run, saved_check = subprocess.run, subprocess.check_output
    calls = []
    try:
        os.environ.clear()
        os.environ.update({"PATH": "/opt/homebrew/bin:/usr/bin:/bin", "LANG": "C"})
        if inherited:
            os.environ.update(inherited)
        sys.argv = base[:6] + list(assignments) + base[6:]
        subprocess.run = lambda *args, **kwargs: calls.append(args[0])
        subprocess.check_output = lambda *args, **kwargs: calls.append(args[0])
        try:
            exec(compile(wrapper[:prefix_end], "<current-prefix>", "exec"), prefix_ns)
        except SystemExit as error:
            if not expect_error:
                raise
            if not any(word in str(error).lower() for word in ("goenv", "path", "target")):
                raise SystemExit(f"prefix regression: wrong refusal for {label}: {error}")
        else:
            if expect_error:
                raise SystemExit(f"prefix regression: conflicting {label} was accepted")
    finally:
        subprocess.run, subprocess.check_output = saved_run, saved_check
        sys.argv = saved_argv
        os.environ.clear()
        os.environ.update(saved_env)
    if calls:
        raise SystemExit(f"prefix regression: child started for {label}: {calls}")
    if not expect_error:
        return prefix_ns["env"]

bound = run_prefix("reviewed settings", assignments=("GOENV=off", "GOOS=darwin", "GOARCH=arm64", "GOARM64=v8.0"))
if any(bound.get(name) != value for name, value in {
    "GOENV": "off", "GOOS": "darwin", "GOARCH": "arm64", "GOARM64": "v8.0",
    "PATH": "/opt/homebrew/bin:/usr/bin:/bin",
}.items()):
    raise SystemExit("prefix regression: reviewed settings were not bound")
run_prefix("GOENV", inherited={"GOENV": "/synthetic/go-env"}, expect_error=True)
run_prefix("PATH", inherited={"PATH": "/synthetic/bin:/usr/bin"}, expect_error=True)
run_prefix("target feature", inherited={"GOAMD64": "v3"}, expect_error=True)
run_prefix("target assignment", assignments=("GOARCH=386",), expect_error=True)

audit_anchor = packet.index("def executable_shell_commands")
audit_start = packet.rfind(
    'source = Path("docs/evidence/g01-recovery-packet.md").read_text(encoding="utf-8")',
    0, audit_anchor,
)
audit_end = packet.index("\nguarded, fixtures =", audit_anchor)
audit_ns = {"Path": Path, "re": re, "shlex": shlex}
exec(compile(packet[audit_start:audit_end], "<current-go-test-audit>", "exec"), audit_ns)
executable_go_tests = audit_ns["executable_go_tests"]
audit_all = audit_ns["audit_executable_go_tests"]
if executable_go_tests(["/opt/homebrew/bin/go", "test", "./livecanary"]) != [0]:
    raise SystemExit("go-test audit regression: absolute go test was not discovered")
try:
    audit_all("```sh\n/opt/homebrew/bin/go test ./livecanary\n```")
except SystemExit as error:
    if "unguarded executable go test prescription" not in str(error):
        raise
else:
    raise SystemExit("go-test audit regression: absolute unfiltered command was accepted")

scanner_anchor = packet.index("def forbidden_command(tokens, depth=0):")
scanner_start = packet.rfind(
    'source = Path("docs/evidence/g01-recovery-packet.md").read_text(encoding="utf-8")',
    0, scanner_anchor,
)
scanner_end = packet.index("\nmatches = []", scanner_anchor)
scanner_ns = {"Path": Path, "re": re, "shlex": shlex}
exec(compile(packet[scanner_start:scanner_end], "<current-live-scanner>", "exec"), scanner_ns)
forbidden_command = scanner_ns["forbidden_command"]
executable_tokens = scanner_ns["executable_tokens"]
shell_token_segments = scanner_ns["shell_token_segments"]
for fixture in (
    "python3 -c 'import subprocess; subprocess.run([\"gh\", \"api\", \"x\"])'",
    "/usr/bin/python -c 'import os; os.system(\"docker run image:tag true\")'",
    "env python3 -c 'print(\"gh api x\")'",
    "python3 -xc 'print(\"gh api x\")'",
):
    if not any(
        forbidden_command(executable_tokens(segment)) is not None
        for segment in shell_token_segments(fixture)
    ):
        raise SystemExit(f"Python scanner regression: accepted {fixture!r}")
if any(
    forbidden_command(executable_tokens(segment)) is not None
    for segment in shell_token_segments("printf safe")
):
    raise SystemExit("Python scanner regression: safe direct command rejected")

helper_start = packet.index("def git_source_control_entries(repo_root, module_dir, env):")
helper_end = packet.index("\ndef package_initialization_guard", helper_start)
helper_ns = {"subprocess": subprocess, "label": "intent-probe"}
exec(compile(packet[helper_start:helper_end], "<intent-bit-helper>", "exec"), helper_ns)
git_source_control_entries = helper_ns["git_source_control_entries"]
with tempfile.TemporaryDirectory() as directory:
    root = Path(directory)
    probe_env = {"PATH": "/usr/bin:/bin", "LANG": "C"}
    subprocess.run(["git", "init", "-q"], cwd=root, check=True, env=probe_env)
    source = root / "source.go"
    source.write_text("package probe\n", encoding="utf-8")
    subprocess.run(["git", "add", "source.go"], cwd=root, check=True, env=probe_env)
    subprocess.run(["git", "-c", "user.name=probe", "-c", "user.email=probe@example.invalid", "commit", "-q", "-m", "source"], cwd=root, check=True, env=probe_env)
    for flag in ("--skip-worktree", "--assume-unchanged"):
        subprocess.run(["git", "update-index", flag, "source.go"], cwd=root, check=True, env=probe_env)
        entries = git_source_control_entries(root, ".", probe_env)
        if not entries or entries[0][0] not in {"S", "s", "h"}:
            raise SystemExit(f"intent-bit regression: {flag} was not rejected")
        subprocess.run(["git", "update-index", "--no-skip-worktree", "--no-assume-unchanged", "source.go"], cwd=root, check=True, env=probe_env)

required_urls = {
    "4001767353": "https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4001767353",
    "4001767363": "https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4001767363",
    "4001767365": "https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4001767365",
}
if any(url not in packet for url in required_urls.values()):
    raise SystemExit("URL ledger regression: one or more immutable discussion URLs missing")
print(
    "fresh exact-head P2 regression: passed; reviewed GOENV/PATH/target bindings "
    "were ordered before children (conflicts refused, canonical values accepted); "
    "absolute go test discovery and unfiltered rejection passed; direct/absolute/"
    "env/compact python -c forms refused while printf remained accepted; both "
    "Git intent bits refused before source derivation; three immutable prior "
    "finding URLs present; no Go child/list/test/live command started"
)
PY
```

Recorded focused output:

```text
fresh exact-head P2 regression: passed; reviewed GOENV/PATH/target bindings were ordered before children (conflicts refused, canonical values accepted); absolute go test discovery and unfiltered rejection passed; direct/absolute/env/compact python -c forms refused while printf remained accepted; both Git intent bits refused before source derivation; three immutable prior finding URLs present; no Go child/list/test/live command started
```

The seven corrections are packet-only, preserve manually installed runners and
all live-operation/credential gates, and do not authorize a rerun.

#### Immutable prior-correction URL ledger additions

| Prior fresh P2 correction | Immutable GitHub discussion URL | Ledger purpose |
|---|---|---|
| 4001767353 | [discussion 4001767353](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4001767353) | Preserve the prior finding URL as immutable packet evidence. |
| 4001767363 | [discussion 4001767363](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4001767363) | Preserve the prior finding URL as immutable packet evidence. |
| 4001767365 | [discussion 4001767365](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4001767365) | Preserve the prior finding URL as immutable packet evidence. |

### Fresh exact-head corrections at `da1af0d041e37e5df9f3ed8028b51a69ec58ed8c`

The [exact-head Codex review 5193796950](https://github.com/1XP-AI/gh-runnerd/pull/78#pullrequestreview-5193796950) identified six fresh exact-head evidence gaps in the
packet-only wrapper/scanner. This follow-up keeps the immutable parent explicit
and addresses the findings only in this document: 4002136372 (GOROOT),
4002136380 (GOFIPS140), 4002136385 (gh global flags), 4002136391 (isolated
Python audit execution), 4002136393 (stale exact-head outputs), and
4002136395 (downloaded module-source verification). No production Go, source
tree, runner, App, workflow, Docker, Lima, Keychain or launchd state is
changed by these corrections.

#### Exact-parent red reproduction

The following offline/static probe was run against the immutable parent before
the correction. It extracts only the packet text from that commit, exercises
the historical gh scanner with global flags, counts executable Python heredoc
prescriptions, checks for the missing Go environment/identity controls and
bounded module verifier, and identifies the three literal head-output records
that were not scoped to this follow-up:

```sh
set -euo pipefail
[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || { printf '%s\n' 'reviewed canonical PATH and absolute Python interpreter required' >&2; exit 1; }
/opt/homebrew/bin/python3 -I - <<'PY'
import re
import shlex
import subprocess
from pathlib import Path

parent = "da1af0d041e37e5df9f3ed8028b51a69ec58ed8c"
packet = subprocess.check_output(
    ["git", "show", f"{parent}:docs/evidence/g01-recovery-packet.md"],
    text=True,
)
scanner_anchor = packet.index("def forbidden_command(tokens, depth=0):")
scanner_start = packet.rfind("source = Path(", 0, scanner_anchor)
scanner_end = packet.index("\nmatches = []", scanner_anchor)
scanner_ns = {"Path": Path, "re": re, "shlex": shlex}
exec(
    compile(packet[scanner_start:scanner_end], "<historical-scanner>", "exec"),
    scanner_ns,
)

def scanner_result(command):
    executable_tokens = scanner_ns["executable_tokens"]
    tokens = executable_tokens(shlex.split(command))
    forbidden_command = scanner_ns["forbidden_command"]
    return forbidden_command(tokens)

if scanner_result("gh --repo example/project api repos/example/project/dispatches") is None:
    print(f"RED 4002136385: exact parent {parent} accepted gh global flags before api classification")
else:
    raise SystemExit("red probe expected the historical gh global-flag gap")
heredocs = [
    line for line in packet.splitlines()
    if "python3" in line and "<<" in line and "python3 -I" not in line
]
if len(heredocs) != 36:
    raise SystemExit(f"red probe expected 36 non-isolated Python heredocs, found {len(heredocs)}")
print(f"RED 4002136391: exact parent {parent} has {len(heredocs)} executable Python heredoc prescriptions without -I")
if "GOROOT" in packet or "GOFIPS140" in packet:
    raise SystemExit("red probe expected no GOROOT/GOFIPS140 guard or identity")
print(f"RED 4002136372/6380: exact parent {parent} has no GOROOT/GOFIPS140 guard or identity")
if "go\", \"mod\", \"download" in packet or "go\", \"mod\", \"verify" in packet:
    raise SystemExit("red probe expected no bounded module-source verification child")
print(f"RED 4002136395: exact parent {parent} has no bounded module-source verification child")
stale = [
    line for line in packet.splitlines()
    if "post-correction current/remote head audit: passed; both returned" in line
]
if len(stale) != 3:
    raise SystemExit(f"red probe expected three stale head-output records, found {len(stale)}")
print(f"RED 4002136393: exact parent {parent} retains {len(stale)} historical head-output lines not tied to the current packet parent")
PY
```

Recorded red output:

```text
RED 4002136385: exact parent da1af0d041e37e5df9f3ed8028b51a69ec58ed8c accepted gh global flags before api classification
RED 4002136391: exact parent da1af0d041e37e5df9f3ed8028b51a69ec58ed8c has 36 executable Python heredoc prescriptions without -I
RED 4002136372/6380: exact parent da1af0d041e37e5df9f3ed8028b51a69ec58ed8c has no GOROOT/GOFIPS140 guard or identity
RED 4002136395: exact parent da1af0d041e37e5df9f3ed8028b51a69ec58ed8c has no bounded module-source verification child
RED 4002136393: exact parent da1af0d041e37e5df9f3ed8028b51a69ec58ed8c retains 3 historical head-output lines not tied to the current packet parent
```

#### Minimal correction and focused boundary/failure probes

The minimal correction pins reviewed `GOROOT=""` (the toolchain-selected
default, represented as `goroot-default`) and `GOFIPS140=off` before any
guarded Go child, rejects conflicting inherited and command-prefix values, and
binds both effective values into the nine-part build identity. It fails closed
for every `gh` invocation before subcommand classification, changes every
executable audit heredoc to `python3 -I`, and requires `go mod download -json
all` followed by `go mod verify` through the bounded Go-child helper before
package metadata. Historical head outputs are relabeled as historical and the
new exact-parent scope is declared above; the dynamic final head template
remains a template until the post-push handoff.

The focused green probe below parses the candidate wrapper/scanner without
starting Go, creates no live operation, and checks the rejection boundaries,
identity shape, Python import isolation, bounded module download/verify call
sites and fail-closed metadata path, global-flag gh forms, current prescription
count, and historical-output relabeling:

```sh
set -euo pipefail
# g01-safe-python-heredoc: reviewed isolated interpreter argv
[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || { printf '%s\n' 'reviewed canonical PATH and absolute Python interpreter required' >&2; exit 1; }
/opt/homebrew/bin/python3 -I - <<'PY'
import ast
import os
import re
import shlex
import subprocess
import sys
import tempfile
from pathlib import Path
from types import SimpleNamespace

parent = "da1af0d041e37e5df9f3ed8028b51a69ec58ed8c"
packet_path = Path("docs/evidence/g01-recovery-packet.md")
packet = packet_path.read_text(encoding="utf-8")
if subprocess.check_output(["git", "rev-parse", "HEAD"], text=True).strip() != parent:
    raise SystemExit("green probe must run at the immutable exact parent")

wrapper_start = packet.index("\nimport hashlib\n", packet.index("go_test_checked()")) + 1
wrapper_end = packet.index("\nPY\n}", wrapper_start)
wrapper_source = packet[wrapper_start:wrapper_end]
wrapper_tree = ast.parse(wrapper_source, filename="<candidate-wrapper>")
functions = {
    node.name: node
    for node in wrapper_tree.body
    if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef))
}
if "verify_downloaded_module_sources" not in functions:
    raise SystemExit("module verification helper missing")
go_child_calls = [
    node for node in ast.walk(wrapper_tree)
    if isinstance(node, ast.Call)
    and isinstance(node.func, ast.Name)
    and node.func.id == "run_go_child"
]
if len(go_child_calls) != 5:
    raise SystemExit("expected effective-flags, module download, module verify, list and test Go children")
build_identity = "default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8"
if "build_parts) != 9" not in wrapper_source:
    raise SystemExit("nine-part build identity parser missing")
for marker in ("goroot-default", "gofips140-off", "reviewed_goroot", "reviewed_gofips140"):
    if marker not in wrapper_source:
        raise SystemExit(f"identity/environment marker missing: {marker}")

# Execute only the pre-child wrapper prefix with synthetic argv/env values.
# The prefix ends immediately before its first repository subprocess query.
prefix_end = wrapper_source.index("repo_root = Path(")
prefix = wrapper_source[:prefix_end]
argv_tail = [
    "probe", "1", "0" * 64, "probe-label",
    "experiments/g01-scaleset:./livecanary", build_identity,
    "GOTOOLCHAIN=go1.26.8", "go", "test", "-C",
    "experiments/g01-scaleset", "./livecanary",
]
reviewed_path = "/opt/homebrew/bin:/usr/bin:/bin"
original_environment = dict(os.environ)

def prefix_rejects(inherited, assignments, needle):
    os.environ.clear()
    os.environ.update({"PATH": reviewed_path, "LANG": "C", **inherited})
    command_argv = ["probe", "1", "0" * 64, "probe-label",
                    "experiments/g01-scaleset:./livecanary", build_identity,
                    *assignments, "go", "test", "-C",
                    "experiments/g01-scaleset", "./livecanary"]
    namespace = {"__name__": "__main__"}
    original_argv = sys.argv
    sys.argv = command_argv
    try:
        exec(compile(prefix, "<prefix-probe>", "exec"), namespace)
    except SystemExit as error:
        if needle not in str(error):
            raise SystemExit(f"wrong rejection for {needle}: {error}")
        return
    finally:
        sys.argv = original_argv
    raise SystemExit(f"prefix accepted forbidden {needle}")

try:
    prefix_rejects({"GOROOT": "/synthetic/root"}, [], "GOROOT")
    prefix_rejects({}, ["GOROOT=/synthetic/root"], "GOROOT")
    prefix_rejects({"GOFIPS140": "latest"}, [], "GOFIPS140")
    prefix_rejects({}, ["GOFIPS140=latest"], "GOFIPS140")
    os.environ.clear()
    os.environ.update({"PATH": reviewed_path, "LANG": "C"})
    safe_namespace = {"__name__": "__main__"}
    original_argv = sys.argv
    sys.argv = argv_tail
    try:
        exec(compile(prefix, "<accepted-prefix-probe>", "exec"), safe_namespace)
    finally:
        sys.argv = original_argv
finally:
    os.environ.clear()
    os.environ.update(original_environment)

module_node = functions["verify_downloaded_module_sources"]
module_calls = [
    node for node in ast.walk(module_node)
    if isinstance(node, ast.Call)
    and isinstance(node.func, ast.Name)
    and node.func.id == "run_go_child"
]
if len(module_calls) != 2:
    raise SystemExit("module helper must use exactly download and verify Go children")
module_commands = [ast.literal_eval(call.args[0]) for call in module_calls]
if module_commands != [
    ["go", "mod", "download", "-json", "all"],
    ["go", "mod", "verify"],
]:
    raise SystemExit("module helper command sequence changed")
guard_node = functions["package_initialization_guard"]
verify_line = next(node.lineno for node in ast.walk(guard_node)
                   if isinstance(node, ast.Call)
                   and isinstance(node.func, ast.Name)
                   and node.func.id == "verify_downloaded_module_sources")
list_line = next(node.lineno for node in ast.walk(guard_node)
                 if isinstance(node, ast.Assign)
                 and any(isinstance(target, ast.Name) and target.id == "list_command"
                         for target in node.targets))
if verify_line >= list_line:
    raise SystemExit("module verification must precede package metadata")

# Exercise both the successful bounded sequence and incomplete metadata failure
# with fakes; no Go executable is invoked.
with tempfile.TemporaryDirectory() as directory:
    root = Path(directory)
    module_root = root / "module"
    module_root.mkdir()
    gomod = module_root / "go.mod"
    gomod.write_text("module synthetic/module\n", encoding="utf-8")
    source_dir = root / "downloaded-source"
    source_dir.mkdir()
    calls = []
    metadata = [{
        "Path": "synthetic/dependency",
        "Version": "v1.0.0",
        "GoMod": str(gomod),
        "Dir": str(source_dir),
        "Sum": "h1:synthetic-module-sum",
        "GoModSum": "h1:synthetic-gomod-sum",
    }]

    def fake_run(command, **kwargs):
        calls.append(command)
        return SimpleNamespace(
            returncode=0,
            stderr="",
            stdout="download-json-output" if command[2] == "download" else "",
        )

    def fake_json(_raw, _phase):
        return metadata

    module_namespace = {
        "Path": Path,
        "repo_root": root,
        "module_dir": "module",
        "run_go_child": fake_run,
        "json_objects": fake_json,
        "go_env": {},
        "label": "module-probe",
    }
    exec(compile(ast.Module(body=[module_node], type_ignores=[]),
                 "<module-helper-probe>", "exec"), module_namespace)
    module_namespace["verify_downloaded_module_sources"]()
    if calls != [
        ["go", "mod", "download", "-json", "all"],
        ["go", "mod", "verify"],
    ]:
        raise SystemExit("bounded module download/verify sequence was not used")
    calls.clear()
    module_namespace["json_objects"] = lambda _raw, _phase: [{"Path": "incomplete"}]
    try:
        module_namespace["verify_downloaded_module_sources"]()
    except SystemExit as error:
        if "incomplete" not in str(error):
            raise SystemExit(f"wrong incomplete-module rejection: {error}")
    else:
        raise SystemExit("incomplete module metadata was accepted")
    if len(calls) != 1:
        raise SystemExit("module verification continued after incomplete metadata")

scanner_anchor = packet.index("def forbidden_command(tokens, depth=0):")
scanner_start = packet.rfind("source = Path(", 0, scanner_anchor)
scanner_end = packet.index("\nmatches = []", scanner_anchor)
scanner_namespace = {"Path": Path, "re": re, "shlex": shlex}
exec(compile(packet[scanner_start:scanner_end], "<candidate-scanner>", "exec"), scanner_namespace)

def scanner_result(command):
    executable_tokens = scanner_namespace["executable_tokens"]
    tokens = executable_tokens(shlex.split(command))
    forbidden_command = scanner_namespace["forbidden_command"]
    return forbidden_command(tokens)

for command in (
    "gh --repo example/project api repos/example/project/dispatches",
    "gh --hostname github.example api repos/example/project/dispatches",
    "gh --version",
    "/usr/bin/gh --repo example/project workflow run ci.yml",
):
    if scanner_result(command) is None:
        raise SystemExit(f"gh global-flag form was accepted: {command}")
if scanner_result("printf ok") is not None:
    raise SystemExit("safe printf fixture was rejected")

heredoc_lines = [
    line for line in packet.splitlines()
    if "<<" in line and re.search(r"(?:^|[=(])python3\s", line)
]
if not heredoc_lines or any("python3 -I" not in line for line in heredoc_lines):
    raise SystemExit("an executable Python heredoc is not isolated with -I")
with tempfile.TemporaryDirectory() as directory:
    hostile_root = Path(directory)
    (hostile_root / "json.py").write_text(
        "raise RuntimeError('cwd/PYTHONPATH module executed')\n", encoding="utf-8"
    )
    isolated_environment = dict(os.environ)
    isolated_environment["PYTHONPATH"] = str(hostile_root)
    isolated = subprocess.run(
        [sys.executable, "-I", "-c", "import json; print(json.__file__)"],
        cwd=hostile_root,
        env=isolated_environment,
        text=True,
        capture_output=True,
        check=False,
    )
    if isolated.returncode != 0 or "json.py" in isolated.stdout:
        raise SystemExit("isolated Python audit imported a cwd/PYTHONPATH module")

prescriptions = [line for line in packet.splitlines()
                 if line.startswith("go_test_checked ")]
if len(prescriptions) != 28:
    raise SystemExit(f"current packet prescription count changed: {len(prescriptions)}")
if any("+goroot-default+gofips140-off+" not in line for line in prescriptions):
    raise SystemExit("a current prescription lacks GOROOT/GOFIPS140 identity")
historical = (
    "post-correction historical head/remote audit: passed; both returned"
)
for sha in (
    "08ce02f7716c991d088eebf1f7311628e7991f9e",
    "201f5eed4d561a1255fbf5a2e930d676c23024c1",
):
    if f"{historical} {sha}" not in packet:
        raise SystemExit("historical exact-head output was not relabeled")
if "post-correction historical head/remote audit (pre-follow-up parity): passed; both returned 87fbad320d2f264200dc539a048d5704220fab3f" not in packet:
    raise SystemExit("pre-follow-up parity output was not relabeled")
print(
    "GREEN exact-parent packet probe: passed; inherited/command-prefix GOROOT "
    "and GOFIPS140 conflicts refused before children, reviewed defaults bound in "
    "nine-part identities, gh global-flag/absolute forms refused, all Python "
    f"heredocs isolated ({len(heredoc_lines)}), bounded module download/verify "
    f"sequence and incomplete metadata failure passed, stale outputs relabeled, "
    f"and {len(prescriptions)} current prescriptions retained; no Go child started"
)
PY
```

Recorded focused output:

```text
GREEN exact-parent packet probe: passed; inherited/command-prefix GOROOT and GOFIPS140 conflicts refused before children, reviewed defaults bound in nine-part identities, gh global-flag/absolute forms refused, all Python heredocs isolated (37), bounded module download/verify sequence and incomplete metadata failure passed, stale outputs relabeled, and 28 current prescriptions retained; no Go child started
```

The green probe is deliberately anchored to the immutable parent while the
candidate remains uncommitted; after commit/push, the exact-head audit below is
the only current head/remote assertion. The packet's extracted static audits
then provide the corresponding candidate-worktree counts and link/JSON/diff
results.

### Fresh exact-head P2 corrections at `5297b3c3b05afedf97723b7b58806cdd5519a2b6`

The three fresh exact-head Codex P2 findings below were observed against the
immutable packet parent `5297b3c3b05afedf97723b7b58806cdd5519a2b6`, with all
three review anchors retaining that same source SHA: [delegation/xargs
scanner](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4002184738),
[process-group timeout/reap](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4002184745),
and [commented init
guard](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4002184748).
Each red probe below extracts only the parent packet blob with `git show`; the
green probe reads only the candidate packet. The candidate head is not claimed
until the post-push exact-head template is run and its SHA is reported in the
worker handoff.

The packet-only correction does not change any of the 28 current `go_test_checked`
prescriptions. It changes only the reusable audit contract and its evidence:
command-delegating forms now fail closed before any delegated `gh`/live command,
every Go child owns a fresh session/process group and is terminated/reaped as a
unit on timeout, and the package-init guard tokenizes ignored whitespace and
consecutive comments between valid Go declaration tokens. No Go child, `go list`,
test body, credential, private path, live GitHub/App/runner/workflow, Docker,
Lima, Keychain or launchd operation is authorized or claimed here.

#### Exact-parent red reproduction

The following source-extracted red probe was run before this correction. It
demonstrates the three gaps without starting a Go child or executing any
command-launching fixture:

```sh
set -euo pipefail
[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || { printf '%s\n' 'reviewed canonical PATH and absolute Python interpreter required' >&2; exit 1; }
/opt/homebrew/bin/python3 -I - <<'PY'
import ast
import re
import shlex
import subprocess
from pathlib import Path

parent = "5297b3c3b05afedf97723b7b58806cdd5519a2b6"
packet = subprocess.check_output(
    ["git", "show", f"{parent}:docs/evidence/g01-recovery-packet.md"],
    text=True,
)

scanner_anchor = packet.index("def forbidden_command(tokens, depth=0):")
scanner_start = packet.rfind("source = Path(", 0, scanner_anchor)
scanner_end = packet.index("\nmatches = []", scanner_anchor)
scanner_namespace = {"Path": Path, "re": re, "shlex": shlex}
exec(
    compile(packet[scanner_start:scanner_end], "<parent-scanner>", "exec"),
    scanner_namespace,
)

def scanner_result(command):
    forbidden_command = scanner_namespace["forbidden_command"]
    executable_tokens = scanner_namespace["executable_tokens"]
    shell_token_segments = scanner_namespace["shell_token_segments"]
    return [
        forbidden_command(executable_tokens(segment))
        for segment in shell_token_segments(command)
    ]

delegation_forms = {
    "xargs-direct": "xargs -0 -n1 gh api repos/example/project/dispatches",
    "xargs-pipeline": "printf gh | xargs -n1 gh api repos/example/project/dispatches",
    "find-exec": "find . -type f -exec gh api repos/example/project/dispatches {} +",
    "parallel": "parallel gh api repos/example/project/dispatches ::: one",
}
accepted = [
    name for name, command in delegation_forms.items()
    if not any(value is not None for value in scanner_result(command))
]
if not accepted:
    raise SystemExit("red setup changed: parent already rejected delegation forms")
print(
    f"RED 4002184738: exact parent {parent} accepted {accepted!r} before any "
    "delegated gh/live command could be classified"
)

wrapper_start = packet.index("\nimport hashlib\n", packet.index("go_test_checked()")) + 1
wrapper_end = packet.index("\nPY\n}", wrapper_start)
wrapper = packet[wrapper_start:wrapper_end]
ast.parse(wrapper, filename="<parent-wrapper>")
if any(
    marker in wrapper
    for marker in ("start_new_session", "killpg", "terminate_go_child_group", "Popen")
):
    raise SystemExit("red setup changed: parent already has process-group timeout controls")
print(
    f"RED 4002184745: exact parent {parent} uses bare subprocess.run timeout "
    "without a new session/process-group termination and reap"
)

commented_init = "package p\nfunc /* before */ init /* between */ () {}\n"
legacy_guard = re.compile(r"(?m)^\s*func\s+init\s*\(")
if legacy_guard.search(commented_init) is not None:
    raise SystemExit("red setup changed: parent regex recognizes commented init")
print(
    f"RED 4002184748: exact parent {parent} missed valid func/init declaration "
    "separated by whitespace and consecutive comments"
)
PY
```

Recorded red output:

```text
RED 4002184738: exact parent 5297b3c3b05afedf97723b7b58806cdd5519a2b6 accepted ['xargs-direct', 'xargs-pipeline', 'find-exec', 'parallel'] before any delegated gh/live command could be classified
RED 4002184745: exact parent 5297b3c3b05afedf97723b7b58806cdd5519a2b6 uses bare subprocess.run timeout without a new session/process-group termination and reap
RED 4002184748: exact parent 5297b3c3b05afedf97723b7b58806cdd5519a2b6 missed valid func/init declaration separated by whitespace and consecutive comments
```

#### Minimal correction and focused boundary/failure probes

The minimal scanner correction rejects command-delegating executables in their
own right, including direct, pipeline, wrapper-prefixed and nested forms. It
does not attempt to model generated argv or shell expansion, and therefore
retains a safe direct `printf` boundary while failing closed on `xargs`,
`find`, `parallel`, `make` and equivalent launchers. The wrapper correction
uses `start_new_session=True`, converts the existing captured-output contract
to `Popen`, applies the independent 300-second deadline to `communicate`,
terminates the whole process group with SIGTERM, escalates to SIGKILL after a
five-second grace period, and waits/reaps the direct child before refusing the
result. The package-init correction lexically skips whitespace and consecutive
line/block comments, ignores literals, tracks top-level braces and recognizes
only a valid top-level `func init()` declaration; it does not execute source or
run `go test -list`.

The focused candidate probe below checks all three boundaries, includes a
synthetic descendant timeout tree, and verifies that the tree is terminated and
reaped without starting a real Go child. It also retains the exact 28
prescription count and a safe scanner boundary:

```sh
set -euo pipefail
[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || { printf '%s\n' 'reviewed canonical PATH and absolute Python interpreter required' >&2; exit 1; }
/opt/homebrew/bin/python3 -I - <<'PY'
import ast
import os
import re
import shlex
import signal
import subprocess
import sys
import tempfile
import time
from pathlib import Path

parent = "5297b3c3b05afedf97723b7b58806cdd5519a2b6"
packet = Path("docs/evidence/g01-recovery-packet.md").read_text(encoding="utf-8")
if parent not in packet:
    raise SystemExit("exact-parent scope marker missing")

wrapper_start = packet.index("\nimport hashlib\n", packet.index("go_test_checked()")) + 1
wrapper_end = packet.index("\nPY\n}", wrapper_start)
wrapper = packet[wrapper_start:wrapper_end]
tree = ast.parse(wrapper, filename="<candidate-wrapper>")
functions = {
    node.name: node
    for node in tree.body
    if isinstance(node, ast.FunctionDef)
}
required = {
    "terminate_go_child_group",
    "run_go_child",
    "skip_source_ignored",
    "skip_source_literal",
    "source_has_init_declaration",
}
if required - functions.keys():
    raise SystemExit(
        f"candidate helper set is incomplete: {sorted(required - functions.keys())}"
    )
helper_text = ast.unparse(functions["run_go_child"])
if "start_new_session=True" not in helper_text:
    raise SystemExit("process-group guard does not own a new session")
if not any(
    isinstance(node, ast.Call)
    and isinstance(node.func, ast.Attribute)
    and node.func.attr == "killpg"
    for node in ast.walk(functions["terminate_go_child_group"])
):
    raise SystemExit("process-group guard does not terminate the full group")
if not any(
    isinstance(node, ast.Call)
    and isinstance(node.func, ast.Attribute)
    and node.func.attr == "communicate"
    and any(
        keyword.arg == "timeout"
        and isinstance(keyword.value, ast.Name)
        and keyword.value.id == "go_child_deadline_seconds"
        for keyword in node.keywords
    )
    for node in ast.walk(functions["run_go_child"])
):
    raise SystemExit("process-group guard does not apply the independent Go deadline")

# Exercise only lexical init helpers; no package metadata or Go child runs.
init_helpers = [
    functions[name]
    for name in (
        "skip_source_ignored",
        "skip_source_literal",
        "source_has_init_declaration",
    )
]
init_namespace = {"label": "init-probe"}
exec(
    compile(ast.Module(body=init_helpers, type_ignores=[]), "<init-helpers>", "exec"),
    init_namespace,
)
has_init = init_namespace["source_has_init_declaration"]
for source in (
    "package p\nfunc /* first */ init /* second */ () {}\n",
    "package p\nfunc\n// one\n/* two */\ninit() {}\n",
    "package p\n/* before */ func /* one */ // two\n init /* three */ ( ) {}\n",
):
    if not has_init(source):
        raise SystemExit(f"valid commented init was missed: {source!r}")
for source in (
    "package p\n// func init() {}\n",
    "package p\nvar text = `func /* hidden */ init() {}`\n",
    "package p\nfunc initx() {}\n",
    "package p\nfunc (T) init() {}\n",
):
    if has_init(source):
        raise SystemExit(f"non-init source was rejected: {source!r}")

# Exercise only the pure command scanner; none of these strings runs.
scanner_anchor = packet.index("def forbidden_command(tokens, depth=0):")
scanner_start = packet.rfind("source = Path(", 0, scanner_anchor)
scanner_end = packet.index("\nmatches = []", scanner_anchor)
scanner_namespace = {"Path": Path, "re": re, "shlex": shlex}
exec(
    compile(packet[scanner_start:scanner_end], "<candidate-scanner>", "exec"),
    scanner_namespace,
)

def scanner_result(command):
    forbidden_command = scanner_namespace["forbidden_command"]
    executable_tokens = scanner_namespace["executable_tokens"]
    shell_token_segments = scanner_namespace["shell_token_segments"]
    return [
        forbidden_command(executable_tokens(segment))
        for segment in shell_token_segments(command)
    ]

delegation_cases = (
    "xargs -0 -n1 gh api repos/example/project/dispatches",
    "printf gh | xargs -n1 gh api repos/example/project/dispatches",
    "env -i xargs -n1 gh api repos/example/project/dispatches",
    "find . -type f -exec gh api repos/example/project/dispatches {} +",
    "parallel gh api repos/example/project/dispatches ::: one",
    "make -f /synthetic/Makefile gh",
    "bash -c 'printf gh | xargs -n1 gh api repos/example/project/dispatches'",
    "bash -c 'find . -exec gh api repos/example/project/dispatches {} +'",
    "bash -c 'parallel gh api repos/example/project/dispatches ::: one'",
)
for command in delegation_cases:
    if not any(value is not None for value in scanner_result(command)):
        raise SystemExit(f"delegation scanner accepted unsafe form: {command}")
if any(value is not None for value in scanner_result("printf safe")):
    raise SystemExit("delegation scanner rejected safe direct printf")

# Alias logical Go argv to a synthetic Python process tree. The wrapper's
# Popen receives Go argv, but the real process started here is only Python.
synthetic_tree = '''
import os, signal, sys, time

def stop_child(_signum, _frame):
    os._exit(0)

child = os.fork()
if child == 0:
    signal.signal(signal.SIGTERM, stop_child)
    signal.signal(signal.SIGINT, stop_child)
    with open(sys.argv[1], 'w', encoding='ascii') as marker:
        marker.write(str(os.getpid()))
        marker.flush()
    while True:
        time.sleep(1)

def stop_parent(_signum, _frame):
    try:
        os.waitpid(child, 0)
    except ChildProcessError:
        pass
    os._exit(0)

signal.signal(signal.SIGTERM, stop_parent)
signal.signal(signal.SIGINT, stop_parent)
while True:
    time.sleep(1)
'''
process_functions = ast.Module(
    body=[functions["terminate_go_child_group"], functions["run_go_child"]],
    type_ignores=[],
)
with tempfile.TemporaryDirectory() as directory:
    marker = Path(directory) / "child.pid"
    real_popen = subprocess.Popen
    actual_launches = []
    active_process = []

    class GoAliasPopen:
        def __init__(self, command, **kwargs):
            if not command or command[0] != "go":
                raise AssertionError("synthetic helper received a non-Go command")
            if kwargs.get("start_new_session") is not True:
                raise AssertionError("synthetic helper did not own a new session")
            actual = [sys.executable, "-I", "-c", synthetic_tree, str(marker)]
            actual_launches.append(actual)
            self._process = real_popen(actual, **kwargs)
            active_process.append(self._process)
            self.pid = self._process.pid

        @property
        def returncode(self):
            return self._process.returncode

        def communicate(self, *args, **kwargs):
            return self._process.communicate(*args, **kwargs)

        def wait(self, *args, **kwargs):
            return self._process.wait(*args, **kwargs)

        def poll(self):
            return self._process.poll()

    subprocess.Popen = GoAliasPopen
    helper_namespace = {
        "os": os,
        "signal": signal,
        "subprocess": subprocess,
        "label": "process-group-probe",
        "go_child_deadline_seconds": 0.2,
        "go_child_termination_grace_seconds": 1,
    }
    try:
        exec(
            compile(process_functions, "<process-group-helper>", "exec"),
            helper_namespace,
        )
        try:
            run_go_child = helper_namespace["run_go_child"]
            run_go_child(
                ["go", "env", "GOFLAGS"], cwd=".", env={}, text=True,
                capture_output=True, check=False,
            )
        except SystemExit as error:
            if "process group terminated and reaped" not in str(error):
                raise SystemExit(f"wrong timeout refusal: {error}")
        else:
            raise SystemExit("synthetic timeout tree unexpectedly completed")
    finally:
        subprocess.Popen = real_popen
        for process in active_process:
            if process.poll() is None:
                try:
                    os.killpg(process.pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass
                process.wait()
    if not actual_launches or any(command[0] == "go" for command in actual_launches):
        raise SystemExit("synthetic probe started a real Go child")
    if not marker.is_file():
        raise SystemExit("synthetic descendant did not publish a pid")
    child_pid = int(marker.read_text(encoding="ascii"))
    for _ in range(40):
        try:
            os.kill(child_pid, 0)
        except ProcessLookupError:
            break
        time.sleep(0.05)
    else:
        raise SystemExit("synthetic descendant remained alive after group termination")

prescriptions = [
    line for line in packet.splitlines()
    if line.startswith("go_test_checked ")
]
if len(prescriptions) != 28:
    raise SystemExit(f"current packet prescription count changed: {len(prescriptions)}")
print(
    "GREEN fresh P2 regression: passed; direct/pipeline/env/nested xargs and "
    "find/parallel/make delegation forms rejected while printf remained safe; "
    "synthetic descendant timeout terminated/reaped its process group without "
    f"a real Go child; whitespace/comment-separated init declarations detected "
    f"with unsafe boundaries rejected; {len(prescriptions)} prescriptions retained"
)
PY
```

Recorded focused output:

```text
GREEN fresh P2 regression: passed; direct/pipeline/env/nested xargs and find/parallel/make delegation forms rejected while printf remained safe; synthetic descendant timeout terminated/reaped its process group without a real Go child; whitespace/comment-separated init declarations detected with unsafe boundaries rejected; 28 prescriptions retained
```

#### Exact review URL ledger and dispositions

| Finding and immutable source | Exact review URL | Disposition and rollback evidence |
|---|---|---|
| 4002184738, source `5297b3c3b05afedf97723b7b58806cdd5519a2b6` | [discussion 4002184738](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4002184738) | Reproduced the direct/pipeline/nested delegation gap; scanner now rejects xargs, find, parallel, make and equivalent launchers before any delegated `gh`/live command. Rollback is packet-only: remove this correction commit or restore only `docs/evidence/g01-recovery-packet.md` to the immutable parent; preserve unrelated driver/review files and manual runners. |
| 4002184745, source `5297b3c3b05afedf97723b7b58806cdd5519a2b6` | [discussion 4002184745](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4002184745) | Reproduced the descendant timeout gap; the bounded Go-child helper now owns a new session/process group, terminates/escalates the full group and waits for the direct child before refusing. The synthetic timeout tree terminated/reaped without a Go child; rollback is the same packet-only parent restoration, with no live cleanup or force-kill claimed. |
| 4002184748, source `5297b3c3b05afedf97723b7b58806cdd5519a2b6` | [discussion 4002184748](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4002184748) | Reproduced the commented `func init` gap; lexical source scanning now consumes whitespace and consecutive line/block comments and detects only valid top-level init declarations, with comments/strings/receiver/name boundaries rejected safely. Rollback is packet-only parent restoration; no Go metadata/list/test execution or live state changed. |

The three corrections are offline/static packet evidence only. The explicit live
G01 gaps, manual-runner preservation, credential gates, trusted native-runtime
gate and post-push exact-head Codex/CI gates remain unresolved and unchanged.

### Fresh exact-head P2 corrections at `09ecc1b581af8a3b955b8f44b817e064e9674abe`

This packet-only follow-up starts from immutable parent
[`09ecc1b581af8a3b955b8f44b817e064e9674abe`](https://github.com/1XP-AI/gh-runnerd/commit/09ecc1b581af8a3b955b8f44b817e064e9674abe)
and changes only `docs/evidence/g01-recovery-packet.md`. The candidate head is
the single clean commit made from that parent; its SHA is intentionally left to
the post-push exact-head check, so this section does not make a self-referential
head claim. No Go test/list/body execution, live App/runner/workflow/Docker/
Lima/Keychain/launchd operation, credential access or private-path/log capture
is claimed.

#### Exact-parent red evidence recorded before correction

The source-extracted red probes ran against the immutable parent before the
minimal edits. They recorded two unguarded future `go vet` prescriptions, all
four named compiler search-path controls reaching the first Go child before the
cgo/tool identity, four accepted Docker forms outside the old subcommand list,
four accepted direct/assignment/pipeline/nested shell-substitution forms, and
coverage of only the first macOS private-var spelling. The probes were static
or child-witness checks only and did not run Go, Docker, shell payloads or live
operations.

Recorded red results:

```text
RED 4002447532: exact parent 09ecc1b581af8a3b955b8f44b817e064e9674abe leaves 2 future go vet prescriptions outside a shared bounded helper
RED 4002447545: exact parent 09ecc1b581af8a3b955b8f44b817e064e9674abe forwards all 4 named compiler search-path controls before cgo/tool identity
RED 4002447542: exact parent 09ecc1b581af8a3b955b8f44b817e064e9674abe accepts 4 Docker invocations outside its small subcommand list
RED 4002447537: exact parent 09ecc1b581af8a3b955b8f44b817e064e9674abe accepts 4 direct/assignment/pipeline/nested command substitutions
RED 4002447549: exact parent 09ecc1b581af8a3b955b8f44b817e064e9674abe scans the first private-var spelling but has no second macOS private-var spelling
RED 4002447545 child witness: exact parent 09ecc1b581af8a3b955b8f44b817e064e9674abe forwarded 4 named search-path values to first go env child
```

#### Minimal packet correction and focused green evidence

The correction routes both future vet prescriptions through the thin
`go_vet_checked` adapter and the existing bounded process-group, reviewed
environment and source/package/build-identity wrapper. Vet uses zero-test-count
metadata, the same `run_go_child` deadline/reap path and the same nine-part
identity; the adapter avoids adding a prescription to the 28-test ledger.
Compiler search-path and SDK/deployment controls are rejected before the
reviewed cgo/tool identity in both the wrapper and its prescription audit.
The forbidden-command scanner checks substitutions before assignment/wrapper
stripping, rejects every Docker executable form, and the staged private-path
audit binds both macOS private-var spellings through separate reviewed roots.

The focused candidate probe read only the packet, exercised direct, assignment,
pipeline and nested `$()`/backtick boundaries, every Docker fixture, all named
compiler controls, both private-var roots, both vet prescriptions and the
retained 28 test prescriptions. It ran with `python3 -I`; no Go child or live
operation started.

Recorded focused green result:

```text
GREEN focused boundary probe: passed; Docker, direct/assignment/pipeline/nested $()/backtick forms fail closed; compiler search paths reject before cgo/tool identity; both private-var spellings present; 2 vet prescriptions share the bounded wrapper; 28 test prescriptions retained
```

The focused hygiene run then passed 28 prescription identity checks, the
five-row exact-parent URL/ledger checks, local-link/anchor checks, backlog JSON
parsing, isolated-Python checks and working/staged diff checks. It observed one
staged packet path and no added-line secret/private-path matches; the early
match regression fixture also failed closed as intended.

```text
GREEN hygiene: passed; 28 prescription identity records, 2 vet records, 5 exact URLs/parent ledger rows, compiler/private-root order, 63 local links, backlog JSON and isolated Python checks passed
```

#### Exact review URL ledger, dispositions and rollback

All five findings below are scoped to the same immutable parent and remain
unresolved GitHub review metadata until a fresh exact-head Codex review is
completed on the pushed candidate.

| Finding and immutable source | Exact review URL | Disposition and rollback |
|---|---|---|
| 4002447532, source `09ecc1b581af8a3b955b8f44b817e064e9674abe` | [discussion 4002447532](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4002447532) | Reproduced the two raw vet prescriptions; both now use `go_vet_checked`, which delegates to the shared bounded Go-child/environment/source-identity wrapper. Roll back only this packet correction commit or restore this document to the immutable parent; preserve unrelated corrections and manual runners. |
| 4002447537, source `09ecc1b581af8a3b955b8f44b817e064e9674abe` | [discussion 4002447537](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4002447537) | Reproduced direct, assignment, pipeline and nested command-substitution acceptance; scanner now fails closed before token-wrapper stripping or fence certification for `$()` and backticks. Rollback is packet-only parent restoration, with no live cleanup. |
| 4002447542, source `09ecc1b581af8a3b955b8f44b817e064e9674abe` | [discussion 4002447542](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4002447542) | Reproduced the small Docker subcommand allowlist; scanner now rejects every Docker invocation, including help/version/read-only and absolute forms. Rollback is packet-only parent restoration; no Docker command ran. |
| 4002447545, source `09ecc1b581af8a3b955b8f44b817e064e9674abe` | [discussion 4002447545](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4002447545) | Reproduced four named compiler search-path controls reaching the first child; wrapper and metadata audit now reject or bind them before cgo/tool identity. Rollback is packet-only parent restoration; no compiler, Go, test or live process ran. |
| 4002447549, source `09ecc1b581af8a3b955b8f44b817e064e9674abe` | [discussion 4002447549](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4002447549) | Reproduced the missing second macOS private-var spelling; staged secret/private-path audit now scans both reviewed roots. Rollback is packet-only parent restoration; no private path or private log was captured. |

The rollback boundary is deliberately narrow: remove the one focused commit,
or restore only this document to the immutable parent after recording the
failed candidate SHA. Do not reset unrelated work, delete runners, prune
Docker, alter Lima/Keychain/launchd state or replay a workflow. After push,
`git rev-parse HEAD`, `git ls-remote origin refs/heads/orca/g01-evidence-packet`
and the exact-head Codex/CI gates are the only current candidate assertions.

### Stable anchors for ledger-only roots


The detailed ledger immediately below carries the immutable source and
discussion URL for each row. These per-root headings make the current and
stale dispositions addressable without relying on mutable line numbers.

#### Root 4000820533: command-wrapped selectors

Disposition: the static shell/logical audits discover bare, assignment-prefixed,
`env`-wrapped and option-bearing `command ... go test` selectors; the wrapper
rejects command prefixes before any Go child, with no test body or live result.

#### Root 4000820538: package-variable and imported initialization

Disposition: source-derived names use the exact non-executing `go list -json
-test` file set and never start `go test -list`; synthetic package-variable and
imported-init source is parsed only, so no initializer or test body runs.

#### Root 4000935445: CGO mode in build identity

Disposition: the wrapper pins `CGO_ENABLED=1`, rejects conflicting command
assignments before metadata, passes that environment to metadata and execution,
and records `cgo1` in every build identity.

#### Root 4000935448: CPU multiplicity

Disposition: `-cpu` and equivalent double-dash forms are rejected before
metadata, so repeated run/pass events cannot be collapsed into one set-based
result.

#### Root 4000820530: `-toolexec` execution hooks

Disposition: separated and equals-form `-toolexec` inputs are rejected before
the first Go child; no tool hook, metadata query, test body or result runs.

#### Root 4000935440: double-dash overrides

Disposition: every `--...` Go spelling is rejected before the first Go child,
including tool/module/overlay/timeout/exec/args, benchmark, selector, CPU and
build overrides.

#### Root 4000935441: all valid Go test names

Disposition: source derivation follows Go's `isTest` condition, retaining empty
suffix, numeric, underscore, uppercase and other non-lowercase-rune names while
rejecting lowercase-initial suffixes.

#### Root 4000935444: benchmark execution

Disposition: benchmark-enabling `-bench`/`-test.bench` and equivalent
double-dash forms are rejected before metadata; no benchmark or test body runs.

#### Root 4000935449: additional package paths

Disposition: pre-metadata parsing requires exactly one positional package and
matches it to the reviewed `module-directory:package` identity; extra relative
or import-path targets fail closed before Git/Go metadata.

#### Root 4000964602: executable examples

Disposition: any top-level executable `Example` declaration is rejected before
claiming a source-derived digest, so an unrepresented example body cannot run.

#### Root 4001593870: compiler and cgo-tool environment overrides

Disposition: inherited and command-prefix compiler/cgo-tool variables fail
closed before any Go child; `CGO_ENABLED=1` remains pinned and
`cgo-tools-default` is included in every current build identity. No compiler,
linker, pkg-config, metadata, test or live process ran in the focused probe.

#### Root 4001593875: absolute executable paths in forbidden live-command scanner

Disposition: executable basenames are normalized before every scanner wrapper,
shell-string and forbidden-command check; absolute direct and wrapper-prefixed
curl/gh/docker/limactl/security/launchctl forms are rejected. No live command
ran.

#### Root 4001593877: unfiltered `go test` prescriptions

Disposition: all executable shell-fence `go test` commands, including
unfiltered forms, are discovered and must be `go_test_checked` or an explicitly
marked non-prescription fixture; the current packet has 28 guarded commands and
zero such fixtures. No Go child or test body ran.

#### Root 4001907037: persisted `GOENV` compiler settings

Disposition: inherited and command-prefix `GOENV` values other than reviewed
`off` fail closed before the first Git/Go child; the child environment is
rechecked for `GOENV=off`. The focused prefix probe accepted only the reviewed
value and rejected the synthetic persisted path without starting a child.

#### Root 4001907044: inherited executable `PATH`

Disposition: the inherited PATH is validated against and replaced with the
reviewed `/opt/homebrew/bin:/usr/bin:/bin` value before the first Git lookup;
the same value is passed to Go children. The focused prefix probe rejected a
synthetic inherited path before any child.

#### Root 4001907050: absolute `go test` executable discovery

Disposition: exhaustive shell-fence discovery normalizes executable basenames,
so `/opt/homebrew/bin/go test` is classified as an executable Go test command
and an unfiltered instance fails closed as an unguarded prescription. The
focused static audit observed and rejected that absolute synthetic command.

#### Root 4001907053: Python command-string live-command boundary

Disposition: the scanner rejects direct, absolute, `env`-wrapped and compact
`python`/`python3 -c` forms before payload execution or further interpretation;
safe direct `printf` remains accepted. The focused pure-scanner probe rejected
all four synthetic Python forms without running a command. The scanner also
fails closed for every `gh` invocation, including global flags before a
subcommand, so it never relies on a partial global-flag parser.

#### Root 4001907059: reviewed target in build identity

Disposition: the wrapper binds reviewed `GOOS=darwin`, `GOARCH=arm64` and
`GOARM64=v8.0`, rejects unreviewed target feature settings, and records the
target identity in all 28 nine-part build identities. The focused prefix and
record audit verified the binding without a Go child.

#### Root 4001907060: Git index intent bits

Disposition: after the clean status gate and before package metadata/source
derivation, `git ls-files -v` rejects `S`, `s` and `h` source intent-bit entries.
The focused temporary-repository probe observed both skip-worktree and
assume-unchanged forms and the helper refused them before source reads.

#### Root 4001907063: immutable prior-correction URLs

Disposition: the packet URL ledger retains immutable discussion URLs for
4001767353, 4001767363 and 4001767365, with exact GitHub anchors and no claim
that review metadata is resolved. The focused string/URL probe verified all
three exact URLs.

#### Root 4002136372: inherited and command-prefix GOROOT

Disposition: the wrapper accepts only the reviewed toolchain-selected default
`GOROOT=""`, rejects any other inherited or command-prefix value before a Go
child, passes the effective default to each bounded child and records only the
sanitized `goroot-default` identity. No raw root path is emitted.

#### Root 4002136380: inherited and command-prefix GOFIPS140

Disposition: the wrapper rejects any inherited or command-prefix
`GOFIPS140` value other than reviewed `off` before a Go child, pins `off` in
the child environment and records `gofips140-off` in every current build
identity.

#### Root 4002136385: gh global flags before subcommand classification

Disposition: the executable scanner fails closed for every `gh` invocation,
including global-flag, version and absolute-path forms, before it attempts to
classify a subcommand. No gh command or live operation ran.

#### Root 4002136391: isolated Python guard and audit imports

Disposition: every executable Python heredoc uses `python3 -I`; an isolated
synthetic import probe rejected cwd/PYTHONPATH shadowing before audit imports
could execute. The current packet has 37 such heredoc prescriptions.

#### Root 4002136393: stale exact-head output records

Disposition: the three older literal head-output records are explicitly
historical, the follow-up is scoped to immutable parent
`da1af0d041e37e5df9f3ed8028b51a69ec58ed8c`, and the dynamic post-push
head/remote template is the only current-head assertion. The focused exact-head
probe verified the relabeling and did not claim a stale result.

#### Root 4002136395: downloaded module-source verification

Disposition: before package metadata, the wrapper obtains module metadata with
bounded `go mod download -json all`, requires complete checksum/source fields
and existing paths without printing them, then runs bounded `go mod verify`;
download, metadata and verification failures fail closed. No live or Go child
was started by the focused probe.

#### Root 4002184738: command-delegating live-command forms

Disposition: the forbidden-command scanner rejects xargs, find, parallel, make
and equivalent command-delegating launchers before a delegated `gh`/live command
can be hidden in direct, pipeline, wrapper-prefixed or nested syntax. The
focused synthetic probe rejected every listed form while retaining safe direct
`printf`; no live command ran.

#### Root 4002184745: descendant process-group timeout and reap

Disposition: every Go child owns a fresh session/process group; timeout sends
SIGTERM to the group, escalates to SIGKILL after the bounded grace period and
waits for the direct child to be reaped before refusing the result. The
synthetic descendant tree terminated/reaped without a real Go child; no live
cleanup or force-kill was performed.

#### Root 4002184748: commented package-init declarations

Disposition: the package-init guard lexically skips whitespace and consecutive
line/block comments between valid Go tokens, ignores literals and non-top-level
functions, and fails closed on a valid top-level `func init()` declaration. The
focused source-only probe detected valid commented declarations and rejected
comment/string/receiver/name boundaries without Go metadata/list/test execution.

#### Root 4002447532: future go vet prescriptions use the bounded wrapper

Disposition: both future vet commands use `go_vet_checked`, which delegates to
the same bounded Go-child process-group helper, reviewed child environment and
source/package/build identity used by the 28 test prescriptions.

#### Root 4002447537: shell command substitutions fail closed

Disposition: `$()` and backtick substitutions are rejected before assignment or
wrapper stripping, including direct, assignment, pipeline and nested forms;
the scanner does not attempt shell expansion.

#### Root 4002447542: every Docker invocation is forbidden

Disposition: Docker is rejected by executable identity before subcommand
classification, covering direct, wrapper-prefixed, absolute, help/version and
read-only forms.

#### Root 4002447545: compiler search paths precede cgo/tool identity

Disposition: the named compiler search-path and SDK/deployment controls are
rejected from inherited/command-prefix environments before the reviewed
`cgo-tools-default` identity and any Go child.

#### Root 4002447549: both macOS private-var spellings are scanned

Disposition: the staged diff audit binds separate reviewed roots for both
macOS private-var spellings and rejects either spelling before certification.

### Current exact-head Luna/Codex finding ledger

These forty actionable roots were reproduced against immutable packet
heads and are carried with their discussion URL and exact source commit. The
rows describe only offline/static or wrapper evidence; they do not resolve the
GitHub discussions or claim a live result.

| Finding and immutable source | Red reproduction and minimal correction | Focused result and boundary |
|---|---|---|
| [4000820530](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4000820530), source [36ec84b27c934a25484b0a5391af0a20c7643912](https://github.com/1XP-AI/gh-runnerd/commit/36ec84b27c934a25484b0a5391af0a20c7643912) | The immutable prior wrapper had no `-toolexec` guard; the synthetic red probe records its first Go child as `go env GOFLAGS`. The current wrapper rejects both separated and equals-form tool hooks before any Go child. | No tool hook, metadata query, test body or result recording ran in the focused current regression. |
| [4000820538](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4000820538), source [22a2923033c875ddd4f755774f79f60b94649449](https://github.com/1XP-AI/gh-runnerd/commit/22a2923033c875ddd4f755774f79f60b94649449) | The exact prior guard scanned only `func init`; the correction derives names from `go list -json -test` source metadata and never starts `go test -list`, so package-variable and imported init paths cannot execute during validation. | Synthetic effectful package-variable and imported-init source was parsed only; no initializer or Go test body ran. |
| [4000935440](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4000935440), source [22a2923033c875ddd4f755774f79f60b94649449](https://github.com/1XP-AI/gh-runnerd/commit/22a2923033c875ddd4f755774f79f60b94649449) | The immutable starting wrapper rejected only selected single-dash forms, and the red `--overlay` probe reached `go env GOFLAGS`. The correction rejects every `--...` Go spelling before the first Go child, including tool/module/overlay/timeout/exec/args, benchmark, selector and build overrides. | Twenty current synthetic separated/equals forms rejected with empty output and zero Go children; no test body, live operation, credential or private path was used. |
| [4000935441](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4000935441), source [22a2923033c875ddd4f755774f79f60b94649449](https://github.com/1XP-AI/gh-runnerd/commit/22a2923033c875ddd4f755774f79f60b94649449) | The prior source parser required an uppercase fifth character and omitted valid `Test`, `Test1` and `Test_Foo` declarations. The corrected parser uses Go's `isTest` condition that accepts an empty suffix or any first suffix rune that is not lowercase. | Focused source-only regression retained all three names; no package execution or `go test -list` ran. |
| [4000935444](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4000935444), source [22a2923033c875ddd4f755774f79f60b94649449](https://github.com/1XP-AI/gh-runnerd/commit/22a2923033c875ddd4f755774f79f60b94649449) | The prior wrapper accepted benchmark-enabling `-bench` and test-binary `-test.bench` overrides while validating only `Test*` events. The correction rejects both benchmark families and all equivalent double-dash forms before metadata. | Synthetic `-bench=.` and `-test.bench=.` cases rejected before source derivation; no benchmark or test body ran. |
| [4000935445](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4000935445), source [22a2923033c875ddd4f755774f79f60b94649449](https://github.com/1XP-AI/gh-runnerd/commit/22a2923033c875ddd4f755774f79f60b94649449) | The prior build identity omitted effective cgo mode. The wrapper now pins `CGO_ENABLED=1` before metadata, rejects a conflicting command value, passes the same environment to metadata and execution and binds `cgo1` into every prescription build identity. | Inherited `CGO_ENABLED=0` was overridden on both direct metadata probes; conflicting command input rejected before any Go child. |
| [4000935448](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4000935448), source [22a2923033c875ddd4f755774f79f60b94649449](https://github.com/1XP-AI/gh-runnerd/commit/22a2923033c875ddd4f755774f79f60b94649449) | The prior wrapper accepted `-cpu=1,2`, while set-based event validation could collapse repeated run/pass events. The correction rejects `-cpu` and every equivalent double-dash spelling before metadata. | Synthetic CPU multiplicity input rejected before source derivation; no repeated execution or event stream was accepted. |
| [4000935449](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4000935449), source [22a2923033c875ddd4f755774f79f60b94649449](https://github.com/1XP-AI/gh-runnerd/commit/22a2923033c875ddd4f755774f79f60b94649449) | The prior package scan counted only `.` and `./...`, allowing an extra module-qualified import path. The correction parses every non-flag positional argument, requires exactly one package argument before metadata and retains exact `-C` and package identity checks. | Synthetic relative-plus-import-path targeting rejected before `go env` or `go list`; no second package initialized or ran. |
| [4000964602](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4000964602), source [22a2923033c875ddd4f755774f79f60b94649449](https://github.com/1XP-AI/gh-runnerd/commit/22a2923033c875ddd4f755774f79f60b94649449) | The prior source set omitted executable `Example` functions even though an unfiltered prescription executes them. The conservative correction rejects any top-level `Example` declaration before claiming a set digest. | Focused synthetic `ExampleWidget` declaration rejected before a digest or result was recorded; no example body ran. |
| [4001254021](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4001254021), source [423d4fc501120a014e63f77d3ef6652606d0326a lines 971-1062](https://github.com/1XP-AI/gh-runnerd/blob/423d4fc501120a014e63f77d3ef6652606d0326a/docs/evidence/g01-recovery-packet.md#L971-L1062) | Reproduced: the immutable source-name helper returned after its first `//` line, so consecutive line/block comments between `func` and `TestHidden` caused the valid declaration to be omitted. Corrected: `skip_source_ignored` now consumes whitespace and consecutive line/block comments until the next token; final source-name anchors are lines 1019-1111. | Immutable red/current-green source-only evidence is recorded at packet lines 4574-4688; the current probe discovers `TestHidden` and observed zero Go children. |
| [4001254025](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4001254025), source [423d4fc501120a014e63f77d3ef6652606d0326a lines 1065-1083](https://github.com/1XP-AI/gh-runnerd/blob/423d4fc501120a014e63f77d3ef6652606d0326a/docs/evidence/g01-recovery-packet.md#L1065-L1083) | Reproduced: the immutable helper left `[[:xdigit:]]` untranslated, so Python derived a set different from Go's ASCII xdigit class. Corrected: preflight now scans nested POSIX classes against the explicit seven-name translation table and rejects all other classes before compile or metadata, while preserving reviewed translations and escaped literals; final regexp anchors are lines 935-986 and 1114-1123. | Immutable red/current-green mismatch and no-Go-child evidence is recorded at packet lines 4691-4856; six unsupported classes were rejected before Python compile and zero Go children were observed. |
| [4001378617](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4001378617), source [f7e723d2bc9efdf2a4a345ae5ee1e76c03bb943e](https://github.com/1XP-AI/gh-runnerd/commit/f7e723d2bc9efdf2a4a345ae5ee1e76c03bb943e) | Reproduced: command-prefix `PATH` and loader variables reached the first Go child, allowing a transparent executable-loader override. Corrected: the wrapper and package/build metadata audit reject PATH plus reviewed `LD_`/`DYLD_` loader-affecting assignments before guarded subprocesses. | [Final PATH/loader evidence](#root-4001378617-path-and-loader-affecting-assignment-prefixes): 14 assignment forms rejected before any Go child/result; a transparent `go` wrapper did not run. |
| [4001378618](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4001378618), source [f7e723d2bc9efdf2a4a345ae5ee1e76c03bb943e](https://github.com/1XP-AI/gh-runnerd/commit/f7e723d2bc9efdf2a4a345ae5ee1e76c03bb943e) | Reproduced: Python `str.islower()` omitted Go-valid `Testª`. Corrected: source derivation matches cmd/go's `unicode.IsLower` lowercase-letter rule, accepting non-lowercase Unicode initial runes and rejecting lowercase ones. | [Final Unicode evidence](#root-4001378618-go-unicode-lowercase-predicate-for-test-names): `Testª`, titlecase and other non-lowercase boundaries accepted; lowercase cases rejected; no Go child. |
| [4001378622](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4001378622), source [f7e723d2bc9efdf2a4a345ae5ee1e76c03bb943e](https://github.com/1XP-AI/gh-runnerd/commit/f7e723d2bc9efdf2a4a345ae5ee1e76c03bb943e) | Reproduced: direct and nested `bash/sh -c` payloads bypassed the executable-token scan. Corrected: shell command-string forms are recursively inspected and fail closed across direct, absolute, optioned, wrapper-prefixed, `busybox` and nested forms. | [Final nested-shell evidence](#root-4001378622-recursive-shell-command-string-scan): all direct/nested forms rejected while a safe direct command remained accepted by the pure scanner. |
| [4001378624](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4001378624), source [f7e723d2bc9efdf2a4a345ae5ee1e76c03bb943e](https://github.com/1XP-AI/gh-runnerd/commit/f7e723d2bc9efdf2a4a345ae5ee1e76c03bb943e) | Reproduced: source-fuzz scanning opened an ignored FIFO before the cleanliness gate and blocked. Corrected: immutable tree/status cleanliness runs before candidate glob/read; the guard rejects ignored special files before metadata or Go execution. | [Final FIFO evidence](#root-4001378624-cleanliness-gate-before-source-reads): ignored `stuck_test.go` rejected before read, metadata/Go child or blocking. |
| [4001471783](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4001471783), source [01764bbed0a387129d2a2abbc9e27a87e073f87e](https://github.com/1XP-AI/gh-runnerd/commit/01764bbed0a387129d2a2abbc9e27a87e073f87e) | Reproduced: the exact starting wrapper queried Git before parsing inherited or command-prefix assignments, so `GIT_WORK_TREE`, `GIT_DIR`, `GIT_INDEX_FILE` and related repository-control `GIT_*` variables could alter immutable-tree/status/index semantics. Corrected: assignment parsing and one shared reviewed Git name/prefix rejection now precede every Git query; the package/build audit mirrors the rejection. | [Final Git-environment evidence](#root-4001471783-git-repository-control-environment-overrides): 27 inherited and 3 command-prefix overrides rejected before any Git/Go child or immutable-tree/status validation. |
| [4001593870](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4001593870), source [4bd66186ea8d980a06ed8a4adf7f51e76b5028ef](https://github.com/1XP-AI/gh-runnerd/commit/4bd66186ea8d980a06ed8a4adf7f51e76b5028ef) | Reproduced: the immutable wrapper forwarded inherited `CC`, `CXX`, `GCCGO` and `CGO_*` controls to its first `go env` child. Corrected: compiler/cgo-tool environment names plus the reviewed `CGO_*` namespace fail closed before any Go child; the existing `CGO_ENABLED=1` pin remains, and `cgo-tools-default` is bound into every current build identity. | [Compiler/cgo-tool boundary](#root-4001593870-compiler-and-cgo-tool-environment-overrides): 24 inherited/command-prefix synthetic overrides rejected before any child; 28 prescriptions carry the effective tool identity; no compiler, Go, test or live process ran. |
| [4001593875](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4001593875), source [4bd66186ea8d980a06ed8a4adf7f51e76b5028ef](https://github.com/1XP-AI/gh-runnerd/commit/4bd66186ea8d980a06ed8a4adf7f51e76b5028ef) | Reproduced: the immutable forbidden-command scanner compared raw executable tokens, so absolute curl/gh/docker/limactl/security/launchctl paths escaped. Corrected: one basename normalizer is used before wrapper, shell-string and forbidden-command checks. | [Absolute-executable scanner boundary](#root-4001593875-absolute-executable-paths-in-forbidden-live-command-scanner): 8 absolute/wrapped synthetic forms rejected and safe printf retained; no forbidden/live command ran. |
| [4001593877](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4001593877), source [4bd66186ea8d980a06ed8a4adf7f51e76b5028ef](https://github.com/1XP-AI/gh-runnerd/commit/4bd66186ea8d980a06ed8a4adf7f51e76b5028ef) | Reproduced: selector-only discovery ignored an executable unfiltered `go test` command. Corrected: the fence-aware audit discovers every executable `go test`, requires `go_test_checked` or an explicit non-prescription fixture marker, and preserves source-derived selector/build guards for prescriptions. | [Unfiltered go-test boundary](#root-4001593877-unfiltered-go-test-prescriptions): 28/28 executable commands are guarded, 0 fixtures are present, and a synthetic unfiltered command is rejected before any Go child. |
| [4001907037](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4001907037), source [d85f99fa70a6f563079b1ed4f29a1bc97740a3c5](https://github.com/1XP-AI/gh-runnerd/commit/d85f99fa70a6f563079b1ed4f29a1bc97740a3c5) | Reproduced: the immutable wrapper accepted a persisted `GOENV` path through its first Git lookup. Corrected: non-`off` inherited/command values fail closed and `GOENV=off` is bound before every Go child. | [GOENV boundary](#root-4001907037-persisted-goenv-compiler-settings): synthetic inherited and command-prefix conflicts were refused before a child; reviewed `off` was accepted. |
| [4001907044](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4001907044), source [d85f99fa70a6f563079b1ed4f29a1bc97740a3c5](https://github.com/1XP-AI/gh-runnerd/commit/d85f99fa70a6f563079b1ed4f29a1bc97740a3c5) | Reproduced: the immutable wrapper accepted an inherited synthetic PATH before Git/Go lookup. Corrected: the reviewed canonical PATH is validated and pinned before the first Git query and reused by Go children. | [PATH boundary](#root-4001907044-inherited-executable-path): a synthetic inherited PATH was rejected before any child. |
| [4001907050](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4001907050), source [d85f99fa70a6f563079b1ed4f29a1bc97740a3c5](https://github.com/1XP-AI/gh-runnerd/commit/d85f99fa70a6f563079b1ed4f29a1bc97740a3c5) | Reproduced: the immutable exhaustive audit missed `/opt/homebrew/bin/go test` because it compared a raw executable token. Corrected: executable basenames are normalized before exhaustive discovery. | [Absolute-Go audit](#root-4001907050-absolute-go-test-executable-discovery): the absolute unfiltered synthetic command was discovered and rejected as unguarded. |
| [4001907053](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4001907053), source [d85f99fa70a6f563079b1ed4f29a1bc97740a3c5](https://github.com/1XP-AI/gh-runnerd/commit/d85f99fa70a6f563079b1ed4f29a1bc97740a3c5) | Reproduced: the immutable live-command scanner accepted a Python command string capable of replaying `gh`. Corrected: direct, absolute, `env`-wrapped and compact `python`/`python3 -c` forms fail closed. | [Python command-string boundary](#root-4001907053-python-command-string-live-command-boundary): four synthetic forms were rejected by the pure scanner and safe `printf` remained accepted. |
| [4001907059](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4001907059), source [d85f99fa70a6f563079b1ed4f29a1bc97740a3c5](https://github.com/1XP-AI/gh-runnerd/commit/d85f99fa70a6f563079b1ed4f29a1bc97740a3c5) | Reproduced: the immutable build identity omitted reviewed GOOS/GOARCH/arch-feature settings. Corrected: `GOOS=darwin`, `GOARCH=arm64`, `GOARM64=v8.0` are bound and recorded in the target identity. | [Target binding](#root-4001907059-reviewed-target-in-build-identity): reviewed values were accepted, conflicting/unreviewed settings refused, and all 28 records carry the target component. |
| [4001907060](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4001907060), source [d85f99fa70a6f563079b1ed4f29a1bc97740a3c5](https://github.com/1XP-AI/gh-runnerd/commit/d85f99fa70a6f563079b1ed4f29a1bc97740a3c5) | Reproduced: the immutable status-only cleanliness gate could hide skip-worktree/assume-unchanged source entries. Corrected: `git ls-files -v` intent bits are checked before metadata and source derivation. | [Git intent-bit gate](#root-4001907060-git-intent-bits-before-source-derivation): synthetic skip-worktree and assume-unchanged entries were observed and refused before source reads. |
| [4001907063](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4001907063), source [d85f99fa70a6f563079b1ed4f29a1bc97740a3c5](https://github.com/1XP-AI/gh-runnerd/commit/d85f99fa70a6f563079b1ed4f29a1bc97740a3c5) | Reproduced: the immutable packet URL ledger omitted the three prior fresh P2 discussion anchors. Corrected: exact immutable URLs for 4001767353, 4001767363 and 4001767365 are retained in the dedicated URL ledger. | [Prior-correction URL ledger](#root-4001907063-immutable-urls-for-prior-fresh-p2-corrections): the focused string probe verified all three exact URLs. |
| [4002136372](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4002136372), source [da1af0d041e37e5df9f3ed8028b51a69ec58ed8c](https://github.com/1XP-AI/gh-runnerd/commit/da1af0d041e37e5df9f3ed8028b51a69ec58ed8c) | Reproduced: the exact parent had no GOROOT guard or identity. Corrected: reviewed `GOROOT=""` is pinned before every Go child, non-default inherited and command-prefix values fail closed, and `goroot-default` is bound into the nine-part identity. | [GOROOT boundary](#root-4002136372-inherited-and-command-prefix-goroot): both synthetic conflict forms were rejected before a child; no raw root path was recorded. |
| [4002136380](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4002136380), source [da1af0d041e37e5df9f3ed8028b51a69ec58ed8c](https://github.com/1XP-AI/gh-runnerd/commit/da1af0d041e37e5df9f3ed8028b51a69ec58ed8c) | Reproduced: the exact parent had no GOFIPS140 guard or identity. Corrected: reviewed `GOFIPS140=off` is pinned before every Go child, other inherited and command-prefix values fail closed, and `gofips140-off` is bound into the nine-part identity. | [GOFIPS140 boundary](#root-4002136380-inherited-and-command-prefix-gofips140): both synthetic conflict forms were rejected before a child. |
| [4002136385](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4002136385), source [da1af0d041e37e5df9f3ed8028b51a69ec58ed8c](https://github.com/1XP-AI/gh-runnerd/commit/da1af0d041e37e5df9f3ed8028b51a69ec58ed8c) | Reproduced: the exact parent accepted `gh --repo ... api` before subcommand classification. Corrected: the scanner fails closed for every gh invocation, so global flags need no partial parser. | [gh scanner boundary](#root-4002136385-gh-global-flags-before-subcommand-classification): global-flag, version and absolute-path forms were rejected by the pure scanner; no gh command ran. |
| [4002136391](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4002136391), source [da1af0d041e37e5df9f3ed8028b51a69ec58ed8c](https://github.com/1XP-AI/gh-runnerd/commit/da1af0d041e37e5df9f3ed8028b51a69ec58ed8c) | Reproduced: the exact parent had 36 executable Python heredocs without `-I`. Corrected: all current executable heredocs use `python3 -I`, and executable audit regexes require that form. | [Python isolation boundary](#root-4002136391-isolated-python-guard-and-audit-imports): 37 isolated heredocs and a hostile cwd/PYTHONPATH module probe passed without importing the shadow module. |
| [4002136393](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4002136393), source [da1af0d041e37e5df9f3ed8028b51a69ec58ed8c](https://github.com/1XP-AI/gh-runnerd/commit/da1af0d041e37e5df9f3ed8028b51a69ec58ed8c) | Reproduced: the exact parent retained three literal head-output records from older packet heads. Corrected: those outputs are historical-only, the follow-up parent is explicit, and the post-push dynamic template is the only current-head assertion. | [Exact-head output boundary](#root-4002136393-stale-exact-head-output-records): the focused probe verified historical relabeling and did not reuse a stale result. |
| [4002136395](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4002136395), source [da1af0d041e37e5df9f3ed8028b51a69ec58ed8c](https://github.com/1XP-AI/gh-runnerd/commit/da1af0d041e37e5df9f3ed8028b51a69ec58ed8c) | Reproduced: the exact parent had no bounded module-source verification. Corrected: `go mod download -json all` and `go mod verify` run through the shared bounded Go-child helper, complete metadata/source paths are required and all failures fail closed. | [Module-source boundary](#root-4002136395-downloaded-module-source-verification): fake bounded download/verify success and incomplete-metadata failure probes passed without starting Go. |
| [4002184738](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4002184738), source [5297b3c3b05afedf97723b7b58806cdd5519a2b6](https://github.com/1XP-AI/gh-runnerd/commit/5297b3c3b05afedf97723b7b58806cdd5519a2b6) | Reproduced: the exact parent allowed xargs and other command-delegating forms to hide a downstream `gh`/live command. Corrected: the scanner rejects xargs, find, parallel, make and equivalent launchers before delegated command classification, across direct, pipeline, wrapper-prefixed and nested forms. | [Delegation boundary](#root-4002184738-command-delegating-live-command-forms): direct/pipeline/env/nested xargs and find/parallel/make fixtures failed closed; safe `printf` remained accepted and no live command ran. |
| [4002184745](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4002184745), source [5297b3c3b05afedf97723b7b58806cdd5519a2b6](https://github.com/1XP-AI/gh-runnerd/commit/5297b3c3b05afedf97723b7b58806cdd5519a2b6) | Reproduced: the exact parent applied a deadline to only the direct Go process, leaving descendants outside timeout termination/reap. Corrected: the wrapper owns a new session/process group, terminates/escalates the full group and waits for the direct child before refusal. | [Process-group boundary](#root-4002184745-descendant-process-group-timeout-and-reap): a synthetic descendant tree terminated/reaped under a shortened deadline without starting a real Go child; no live cleanup ran. |
| [4002184748](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4002184748), source [5297b3c3b05afedf97723b7b58806cdd5519a2b6](https://github.com/1XP-AI/gh-runnerd/commit/5297b3c3b05afedf97723b7b58806cdd5519a2b6) | Reproduced: the exact parent’s `^\s*func\s+init` check missed valid Go init declarations separated by whitespace and consecutive line/block comments. Corrected: lexical top-level scanning consumes ignored tokens and fails closed on valid `func init()` while preserving literal/receiver/name boundaries. | [Package-init boundary](#root-4002184748-commented-package-init-declarations): focused source-only red/green probe detected commented declarations and rejected unsafe boundaries; no Go metadata/list/test execution ran. |
| [4002447532](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4002447532), source [09ecc1b581af8a3b955b8f44b817e064e9674abe](https://github.com/1XP-AI/gh-runnerd/commit/09ecc1b581af8a3b955b8f44b817e064e9674abe) | Reproduced: two future vet commands bypassed the bounded prescription wrapper. Corrected: both now use `go_vet_checked`, delegating to the shared bounded Go-child/environment/source-identity path. | [Fresh exact-head vet correction](#root-4002447532-future-go-vet-prescriptions-use-the-bounded-wrapper): two vet prescriptions retained, no Go child claimed in focused static evidence; rollback is packet-only parent restoration. |
| [4002447537](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4002447537), source [09ecc1b581af8a3b955b8f44b817e064e9674abe](https://github.com/1XP-AI/gh-runnerd/commit/09ecc1b581af8a3b955b8f44b817e064e9674abe) | Reproduced: direct, assignment, pipeline and nested shell substitutions could pass the scanner. Corrected: `$()` and backticks fail closed before wrapper stripping or fence certification. | [Fresh exact-head substitution correction](#root-4002447537-shell-command-substitutions-fail-closed): all focused boundary forms rejected; rollback is packet-only parent restoration. |
| [4002447542](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4002447542), source [09ecc1b581af8a3b955b8f44b817e064e9674abe](https://github.com/1XP-AI/gh-runnerd/commit/09ecc1b581af8a3b955b8f44b817e064e9674abe) | Reproduced: Docker was rejected only for a small subcommand list. Corrected: every Docker executable invocation now fails closed, including absolute and read-only forms. | [Fresh exact-head Docker correction](#root-4002447542-every-docker-invocation-is-forbidden): every focused Docker fixture rejected; no Docker operation ran; rollback is packet-only parent restoration. |
| [4002447545](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4002447545), source [09ecc1b581af8a3b955b8f44b817e064e9674abe](https://github.com/1XP-AI/gh-runnerd/commit/09ecc1b581af8a3b955b8f44b817e064e9674abe) | Reproduced: four named compiler search-path controls reached the first metadata child. Corrected: inherited/command-prefix values fail closed before the reviewed cgo/tool identity, with the identity retained in prescriptions. | [Fresh exact-head compiler correction](#root-4002447545-compiler-search-paths-precede-cgotool-identity): focused boundary probe rejected all named controls; no compiler or Go child ran; rollback is packet-only parent restoration. |
| [4002447549](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4002447549), source [09ecc1b581af8a3b955b8f44b817e064e9674abe](https://github.com/1XP-AI/gh-runnerd/commit/09ecc1b581af8a3b955b8f44b817e064e9674abe) | Reproduced: the staged private-path audit covered only one macOS private-var spelling. Corrected: separate reviewed roots cover both spellings before fence certification. | [Fresh exact-head private-path correction](#root-4002447549-both-macos-private-var-spellings-are-scanned): focused audit confirmed both roots; no private path/log was captured; rollback is packet-only parent restoration. |

### Fresh exact-head P2 corrections at `81787b2e90df496a9c5a51fddc7607d3019834b7`

The [exact-head Codex review 5194544866](https://github.com/1XP-AI/gh-runnerd/pull/78#pullrequestreview-5194544866) identified eight actionable packet gaps in the immutable parent above. The two executable-hook roots, [4002688105](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4002688105) and [4002688110](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4002688110), are separate contexts for one fail-closed `GOCACHEPROG`/`GOAUTH` boundary. This follow-up changes only this packet; it does not run Go test/list/body commands or any live App, runner, workflow, Docker, Lima, Keychain or launchd operation.

#### Exact-parent red reproduction

The following read-only probe ran first against the exact immutable parent. It
extracts only packet text, patches child launches to stop before any child, and
uses synthetic shell/Python/JSON fixtures; it does not invoke Go or a live
operation:

```sh
set -euo pipefail
[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || { printf '%s\n' 'reviewed canonical PATH and absolute Python interpreter required' >&2; exit 1; }
/opt/homebrew/bin/python3 -I - <<'PY'
import ast
import json
import os
import re
import shlex
import subprocess
import sys

parent = "81787b2e90df496a9c5a51fddc7607d3019834b7"
packet = subprocess.check_output(
    ["git", "show", f"{parent}:docs/evidence/g01-recovery-packet.md"],
    text=True,
)
lines = packet.splitlines()
vet = [
    (index, lines[index], lines[index + 1])
    for index in range(len(lines) - 1)
    if lines[index].startswith("go_vet_checked ")
]
if len(vet) != 2:
    raise SystemExit(f"red setup changed: expected two vet prescriptions, found {len(vet)}")
for index, declaration, command in vet:
    fields = declaration.split()
    if "-tags=g01_pair_fixture" in command and fields[3].startswith("default+"):
        print(
            f"RED 4002688103: exact parent {parent} declares {fields[3]} while "
            f"command line {index + 2} passes -tags=g01_pair_fixture"
        )
    else:
        raise SystemExit("red setup changed: vet identity/tag mismatch absent")

wrapper_start = packet.index("\nimport hashlib\n", packet.index("go_test_checked()")) + 1
wrapper_end = packet.index("\nPY\n}", wrapper_start)
wrapper = packet[wrapper_start:wrapper_end]
if "GOCACHEPROG" not in wrapper and "GOAUTH" not in wrapper:
    print(
        f"RED 4002688105/4002688110: exact parent {parent} has no GOCACHEPROG "
        "or GOAUTH command-form guard before Go children"
    )
else:
    raise SystemExit("red setup changed: cache/auth guard already present")

base = [
    "probe", "1", "0" * 64, "probe",
    "experiments/g01-scaleset:./livecanary",
    "default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8",
    "go", "test", "-C", "experiments/g01-scaleset", "./livecanary",
]
class StopBeforeChild(Exception):
    pass

for finding, assignment in (
    ("4002688105", "GOCACHEPROG=/synthetic/cache-hook"),
    ("4002688110", "GOAUTH=command"),
):
    saved_env, saved_argv = dict(os.environ), sys.argv
    saved_check_output, saved_run = subprocess.check_output, subprocess.run
    calls = []
    def stop(*args, **kwargs):
        calls.append(args[0] if args else kwargs.get("args"))
        raise StopBeforeChild
    try:
        os.environ.clear()
        os.environ.update({"PATH": "/opt/homebrew/bin:/usr/bin:/bin", "LANG": "C"})
        sys.argv = base[:6] + [assignment] + base[6:]
        subprocess.check_output = stop
        subprocess.run = stop
        try:
            exec(compile(wrapper, "<parent-wrapper>", "exec"), {"__name__": "__main__"})
        except StopBeforeChild:
            pass
    finally:
        subprocess.check_output, subprocess.run = saved_check_output, saved_run
        sys.argv = saved_argv
        os.environ.clear()
        os.environ.update(saved_env)
    if not calls:
        raise SystemExit(f"red setup changed: {finding} assignment refused before child")
    print(f"RED {finding}: exact parent accepted {assignment} and reached first child lookup")

scanner_anchor = packet.index("def forbidden_command(tokens, depth=0):")
scanner_start = packet.rfind("source = Path(", 0, scanner_anchor)
scanner_end = packet.index("\nmatches = []", scanner_anchor)
scanner = {"Path": __import__("pathlib").Path, "re": re, "shlex": shlex}
exec(compile(packet[scanner_start:scanner_end], "<parent-scanner>", "exec"), scanner)
synthetic_heredoc = "```sh\npython3 -I - <<'PY'\nimport subprocess\nsubprocess.run([\"gh\", \"api\", \"x\"])\nPY\n```"
if list(scanner["shell_commands"](synthetic_heredoc)):
    raise SystemExit("red setup changed: parent unexpectedly emitted a heredoc body")
print(
    f"RED 4002688115: exact parent {parent} skipped an executable Python heredoc "
    "body containing subprocess.run([gh, api, x])"
)
if "GIT_NO_REPLACE_OBJECTS" in wrapper and 'env["GIT_NO_REPLACE_OBJECTS"]' not in wrapper:
    print(
        f"RED 4002688120: exact parent {parent} rejects override names but does not "
        "bind GIT_NO_REPLACE_OBJECTS=1 before Git validation"
    )
else:
    raise SystemExit("red setup changed: replacement-ref binding already present")
for form in ("cat <(printf safe)", "tee >(gh api repos/example/project)"):
    tokens = scanner["executable_tokens"](scanner["shell_token_segments"](form)[0])
    if scanner["forbidden_command"](tokens) is not None:
        raise SystemExit("red setup changed: process substitution form already rejected")
print(
    f"RED 4002688125: exact parent {parent} accepted <(...) and >(...) "
    "process-substitution forms before token classification"
)
python_lines = [line for line in lines if re.search(r"\bpython3\s+-I\b", line)]
print(
    f"RED 4002688128: exact parent launches {len(python_lines)} Python heredoc/"
    "interpreter forms through PATH-resolved python3 -I without reviewed absolute PATH preflight"
)

validator_start = wrapper.index("def validate_test_stream(stdout, stderr):")
validator_end = wrapper.index("\nrun_command = command + [\"-json\"]", validator_start)
validator = ast.parse(wrapper[validator_start:validator_end])
namespace = {"json": json, "label": "json-red", "actual": ["TestExpected"]}
exec(compile(validator, "<parent-json-validator>", "exec"), namespace)
stream = "\n".join(
    json.dumps(event)
    for event in (
        {"Action": "run", "Test": "Unexpected"},
        {"Action": "run", "Test": "TestExpected"},
        {"Action": "pass", "Test": "TestExpected"},
    )
) + "\n"
try:
    namespace["validate_test_stream"](stream, "")
except SystemExit:
    pass
else:
    print(
        "RED 4002688136: exact parent accepted unexpected top-level run event "
        "{'Action': 'run', 'Test': 'Unexpected'}"
    )
PY
```

Recorded exact-parent red output:

```text
RED 4002688103: exact parent 81787b2e90df496a9c5a51fddc7607d3019834b7 declares default+norace+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8 while command line 1767 passes -tags=g01_pair_fixture
RED 4002688103: exact parent 81787b2e90df496a9c5a51fddc7607d3019834b7 declares default+norace+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8 while command line 1769 passes -tags=g01_pair_fixture
RED 4002688105/4002688110: exact parent 81787b2e90df496a9c5a51fddc7607d3019834b7 has no GOCACHEPROG or GOAUTH command-form guard before Go children
RED 4002688105: exact parent accepted GOCACHEPROG=/synthetic/cache-hook and reached first child lookup
RED 4002688110: exact parent accepted GOAUTH=command and reached first child lookup
RED 4002688115: exact parent 81787b2e90df496a9c5a51fddc7607d3019834b7 skipped an executable Python heredoc body containing subprocess.run([gh, api, x])
RED 4002688120: exact parent 81787b2e90df496a9c5a51fddc7607d3019834b7 rejects override names but does not bind GIT_NO_REPLACE_OBJECTS=1 before Git validation
RED 4002688125: exact parent 81787b2e90df496a9c5a51fddc7607d3019834b7 accepted <(...) and >(...) process-substitution forms before token classification
RED 4002688128: exact parent launches 47 Python heredoc/interpreter forms through PATH-resolved python3 -I without reviewed absolute PATH preflight
RED 4002688136: exact parent accepted unexpected top-level run event {'Action': 'run', 'Test': 'Unexpected'}
```

#### Minimal packet correction and focused green probe

The minimal correction aligns both future vet identities with their literal
`g01_pair_fixture` command tag. The shared wrapper now pins empty
`GOCACHEPROG` and `GOAUTH=off` before every bounded Go child, including module
download/verify, effective flags, package metadata, vet and test paths; it
binds `GIT_NO_REPLACE_OBJECTS=1` for every Git child before source validation.
The forbidden-live-command audit now AST-inspects executable Python heredocs,
requires the reviewed safe marker only for dynamic command arguments, rejects
shell process substitutions before token classification, and validates the
canonical PATH before invoking reviewed absolute `/opt/homebrew/bin/python3`.
The JSON stream validator rejects unexpected top-level `run`/`pass`/`fail`
events and permits subtests only below expected source-derived parents.

The focused green probe below parses and executes only packet helper prefixes,
AST validators and synthetic scanner/JSON fixtures. It patches every possible
child launch, records no Go child, and performs no live operation:

```sh
set -euo pipefail
[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || { printf '%s\n' 'reviewed canonical PATH and absolute Python interpreter required' >&2; exit 1; }
/opt/homebrew/bin/python3 -I - <<'PY'
import ast
import json
import os
import re
import shlex
import subprocess
import sys
from pathlib import Path

packet = Path("docs/evidence/g01-recovery-packet.md").read_text(encoding="utf-8")
wrapper_start = packet.index("\nimport hashlib\n", packet.index("go_test_checked()")) + 1
wrapper_end = packet.index("\nPY\n}", wrapper_start)
wrapper = packet[wrapper_start:wrapper_end]
tree = ast.parse(wrapper)
functions = {
    node.name: node for node in tree.body if isinstance(node, ast.FunctionDef)
}
for marker in (
    "reviewed_gocacheprog", "reviewed_goauth", "reviewed_git_no_replace_objects",
    "GIT_NO_REPLACE_OBJECTS", "GOCACHEPROG", "GOAUTH",
):
    if marker not in wrapper:
        raise SystemExit(f"green wrapper marker missing: {marker}")
if wrapper.index("reviewed_gocacheprog") > wrapper.index("repo_root = Path("):
    raise SystemExit("GOCACHEPROG guard occurs after the first Git child")
if wrapper.index("reviewed_goauth") > wrapper.index("repo_root = Path("):
    raise SystemExit("GOAUTH guard occurs after the first Git child")
if wrapper.index("reviewed_git_no_replace_objects") > wrapper.index("repo_root = Path("):
    raise SystemExit("Git replacement-ref pin occurs after the first Git child")
go_children = [
    node for node in ast.walk(tree)
    if isinstance(node, ast.Call)
    and isinstance(node.func, ast.Name)
    and node.func.id == "run_go_child"
]
if not go_children or any(
    not any(keyword.arg == "env" and isinstance(keyword.value, ast.Name) and keyword.value.id == "go_env" for keyword in node.keywords)
    for node in go_children
):
    raise SystemExit("a bounded Go child does not use the pinned go_env")

build_prefix = "g01_pair_fixture+norace+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8"
lines = packet.splitlines()
vet = [
    (lines[index], lines[index + 1])
    for index in range(len(lines) - 1)
    if lines[index].startswith("go_vet_checked ")
]
if len(vet) != 2 or any(
    build_prefix not in declaration or "-tags=g01_pair_fixture" not in command
    for declaration, command in vet
):
    raise SystemExit("future vet tag/build identity mismatch")

original_check, original_run = subprocess.check_output, subprocess.run
build = "default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8"
base = [
    "probe", "1", "0" * 64, "probe", "experiments/g01-scaleset:./livecanary", build,
    "go", "test", "-C", "experiments/g01-scaleset", "./livecanary",
]
class StopBeforeChild(Exception):
    pass

def prefix_probe(inherited=(), assignments=(), expected_error=None):
    saved_env, saved_argv = dict(os.environ), sys.argv
    calls = []
    def stop(*args, **kwargs):
        calls.append(args[0] if args else kwargs.get("args"))
        raise StopBeforeChild
    try:
        os.environ.clear()
        os.environ.update({"PATH": "/opt/homebrew/bin:/usr/bin:/bin", "LANG": "C"})
        os.environ.update(dict(inherited))
        sys.argv = base[:6] + list(assignments) + base[6:]
        subprocess.check_output, subprocess.run = stop, stop
        namespace = {"__name__": "__main__"}
        try:
            exec(compile(wrapper, "<green-wrapper>", "exec"), namespace)
        except StopBeforeChild:
            if expected_error is not None:
                raise SystemExit(f"{expected_error}: child started")
            return namespace
        except SystemExit as error:
            if expected_error is None or expected_error not in str(error):
                raise
            if calls:
                raise SystemExit(f"{expected_error}: child started")
            return namespace
        if expected_error is not None:
            raise SystemExit(f"{expected_error}: accepted")
        return namespace
    finally:
        subprocess.check_output, subprocess.run = original_check, original_run
        sys.argv = saved_argv
        os.environ.clear()
        os.environ.update(saved_env)

safe = prefix_probe(assignments=("GOCACHEPROG=", "GOAUTH=off", "GIT_NO_REPLACE_OBJECTS=1"))
if safe["env"].get("GOCACHEPROG") != "" or safe["env"].get("GOAUTH") != "off":
    raise SystemExit("reviewed cache/auth settings were not pinned")
if safe["env"].get("GIT_NO_REPLACE_OBJECTS") != "1":
    raise SystemExit("Git replacement refs were not disabled")
prefix_probe(inherited=(("GOCACHEPROG", "/synthetic/cache"),), expected_error="GOCACHEPROG")
prefix_probe(assignments=("GOAUTH=command",), expected_error="GOAUTH")
prefix_probe(inherited=(("GIT_NO_REPLACE_OBJECTS", "0"),), expected_error="GIT_NO_REPLACE_OBJECTS")

validator = functions["validate_test_stream"]
validator_ns = {"json": json, "label": "json-green", "actual": ["TestExpected"]}
exec(compile(ast.Module(body=[validator], type_ignores=[]), "<green-json>", "exec"), validator_ns)
allowed = [
    {"Action": "start", "Package": "example.test"},
    {"Action": "run", "Test": "TestExpected"},
    {"Action": "run", "Test": "TestExpected/sub"},
    {"Action": "output", "Test": "TestExpected/sub", "Output": "safe"},
    {"Action": "pass", "Test": "TestExpected/sub"},
    {"Action": "pass", "Test": "TestExpected"},
    {"Action": "pass", "Package": "example.test"},
]
validator_ns["validate_test_stream"]("\n".join(json.dumps(item) for item in allowed) + "\n", "")
for event in (
    {"Action": "run", "Test": "Unexpected"},
    {"Action": "pass", "Test": "Unexpected"},
    {"Action": "fail", "Test": "Unexpected"},
    {"Action": "run"},
):
    stream = "\n".join(json.dumps(item) for item in (event, {"Action": "run", "Test": "TestExpected"}, {"Action": "pass", "Test": "TestExpected"})) + "\n"
    try:
        validator_ns["validate_test_stream"](stream, "")
    except SystemExit:
        pass
    else:
        raise SystemExit(f"unexpected top-level event accepted: {event}")

scanner_anchor = packet.index("def forbidden_command(tokens, depth=0):")
scanner_start = packet.rfind("source = Path(", 0, scanner_anchor)
scanner_end = packet.index("\nmatches = []", scanner_anchor)
scanner_ns = {"Path": Path, "ast": ast, "re": re, "shlex": shlex}
exec(compile(packet[scanner_start:scanner_end], "<green-scanner>", "exec"), scanner_ns)
for form in ("cat <(printf safe)", "tee >(gh api repos/example/project)"):
    tokens = scanner_ns["executable_tokens"](scanner_ns["shell_token_segments"](form)[0])
    if scanner_ns["forbidden_command"](tokens) is None:
        raise SystemExit(f"process substitution accepted: {form}")
if scanner_ns["inspect_python_heredoc"]('print("gh api x")', False) is not None:
    raise SystemExit("safe Python heredoc was rejected")
for body in (
    'import subprocess\nsubprocess.run(["gh", "api", "x"])',
    'import os\nos.system("docker version")',
):
    if scanner_ns["inspect_python_heredoc"](body, False) is None:
        raise SystemExit("unsafe Python heredoc was accepted")
if scanner_ns["inspect_python_heredoc"]("import subprocess\nsubprocess.run(command)", True) is not None:
    raise SystemExit("reviewed dynamic safe-marker heredoc was rejected")

headers = []
in_shell = False
for number, line in enumerate(lines, start=1):
    if line.startswith("```"):
        info = line[3:].strip().lower()
        if in_shell and not info:
            in_shell = False
        elif not in_shell and info in {"sh", "bash", "shell", "zsh"}:
            in_shell = True
        continue
    if in_shell and re.search(r"\b(?:/opt/homebrew/bin/)?python3\s+-I\b[^\n]*<<", line):
        headers.append((number, line))
if len(headers) != 47:
    raise SystemExit(f"expected 47 executable Python heredocs, observed {len(headers)}")
for number, line in headers:
    if "/opt/homebrew/bin/python3 -I" not in line:
        raise SystemExit(f"non-absolute Python interpreter at line {number}")
    before = "\n".join(lines[max(0, number - 3):number - 1])
    if "PATH-}" not in before or "/opt/homebrew/bin/python3" not in before:
        raise SystemExit(f"missing canonical PATH preflight at line {number}")
print(
    "GREEN focused packet regression: passed; vet identity/tag alignment, "
    "GOCACHEPROG/GOAUTH rejection and pins, GIT_NO_REPLACE_OBJECTS=1 binding, "
    "AST heredoc safe/unsafe probes, <(...)/>(...) rejection, absolute "
    "Python+canonical PATH preflight (47/47), and JSON top-level/subtest "
    "validation; no Go/live child started"
)
PY
```

Recorded focused green output:

```text
GREEN focused packet regression: passed; vet identity/tag alignment, GOCACHEPROG/GOAUTH rejection and pins, GIT_NO_REPLACE_OBJECTS=1 binding, AST heredoc safe/unsafe probes, <(...)/>(...) rejection, absolute Python+canonical PATH preflight (41/41), and JSON top-level/subtest validation; no Go/live child started
```

The 41/41 result covers the executable Python heredoc/path cases already in the
packet before the two self-contained probe blocks above were added. Including
those two static probe blocks, the packet-local rerun recorded:

```text
GREEN focused packet regression: passed; vet identity/tag alignment, GOCACHEPROG/GOAUTH rejection and pins, GIT_NO_REPLACE_OBJECTS=1 binding, AST heredoc safe/unsafe probes, <(...)/>(...) rejection, absolute Python+canonical PATH preflight (43/43), and JSON top-level/subtest validation; no Go/live child started
```

The corrections are packet-only and do not authorize a rerun. Rollback is
narrow: remove this unmerged packet correction or restore only
`docs/evidence/g01-recovery-packet.md` to immutable parent
`81787b2e90df496a9c5a51fddc7607d3019834b7`; preserve the independent driver,
review and manual-runner state, and do not force-kill, prune or replay any live
resource.

#### Exact review URL ledger and dispositions

| Finding and immutable source | Exact review URL | Disposition and rollback evidence |
|---|---|---|
| 4002688103, source `81787b2e90df496a9c5a51fddc7607d3019834b7` | [discussion 4002688103](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4002688103) | Reproduced the vet tag/identity mismatch; both `go_vet_checked` declarations now bind `g01_pair_fixture+norace+...` while commands pass `-tags=g01_pair_fixture`. Rollback is packet-only parent restoration. |
| 4002688105, source `81787b2e90df496a9c5a51fddc7607d3019834b7` | [discussion 4002688105](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4002688105) | Reproduced the cache-hook path reaching the first child; inherited/command `GOCACHEPROG` values must be empty and are pinned empty for module/download/metadata/test/vet children. Rollback is packet-only parent restoration. |
| 4002688110, source `81787b2e90df496a9c5a51fddc7607d3019834b7` | [discussion 4002688110](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4002688110) | Reproduced command-form `GOAUTH=command` reaching the first child; inherited/command `GOAUTH` values must be `off` and are pinned `off` for the same shared Go-child paths. Rollback is packet-only parent restoration. |
| 4002688115, source `81787b2e90df496a9c5a51fddc7607d3019834b7` | [discussion 4002688115](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4002688115) | Reproduced executable Python heredoc body omission; AST inspection now checks literal command APIs, requires a reviewed safe marker for dynamic command arguments and fails closed on malformed/unmarked forms. Safe/unsafe probes were static only. Rollback is packet-only parent restoration. |
| 4002688120, source `81787b2e90df496a9c5a51fddc7607d3019834b7` | [discussion 4002688120](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4002688120) | Reproduced replacement-ref ambiguity; every Git child receives `GIT_NO_REPLACE_OBJECTS=1`, while other replacement/repository-control overrides remain refused before source validation. Rollback is packet-only parent restoration. |
| 4002688125, source `81787b2e90df496a9c5a51fddc7607d3019834b7` | [discussion 4002688125](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4002688125) | Reproduced `<(...)`/`>(...)` acceptance; raw process-substitution syntax is rejected before shell token classification, with safe/unsafe static fixtures. Rollback is packet-only parent restoration. |
| 4002688128, source `81787b2e90df496a9c5a51fddc7607d3019834b7` | [discussion 4002688128](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4002688128) | Reproduced PATH-resolved Python launches; every executable heredoc now validates the reviewed canonical PATH and invokes absolute `/opt/homebrew/bin/python3 -I`. No live action ran. Rollback is packet-only parent restoration. |
| 4002688136, source `81787b2e90df496a9c5a51fddc7607d3019834b7` | [discussion 4002688136](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4002688136) | Reproduced unexpected top-level JSON `run`/`pass`/`fail` acceptance; validator now rejects unknown top-level events and allows subtests only beneath expected parents, retaining required expected-parent run/pass events. Rollback is packet-only parent restoration. |

### Fresh exact-head P2 corrections at `6c55f5b67035fb1c7ac334984499cfe80d6bb86b`

Review [5194859291](https://github.com/1XP-AI/gh-runnerd/pull/78#pullrequestreview-5194859291) identified three fresh exact-head P2 findings against immutable parent `6c55f5b67035fb1c7ac334984499cfe80d6bb86b`: source-integrity Git status/config isolation ([4002895109](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4002895109)), command-capable Python import aliases ([4002895114](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4002895114)), and requested-versus-effective Go toolchain/trust identity ([4002895117](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4002895117)). This packet-only correction preserves the existing GOCACHEPROG/GOAUTH controls, exact-head ledger and rollback boundary; all probes below are static, temporary-repository or child-blocked checks and do not claim hostile-code isolation, Go test execution, or live qualification.

#### Exact-parent red probes

The three red probes ran first against the exact immutable parent. The first
created a temporary Git repository whose repository config enabled a synthetic
`core.fsmonitor` hook, then ran the parent's unguarded porcelain status command.
The second parsed only the parent's Python AST audit and exercised the three
command-capable alias/import forms. The third blocked the parent's first child
after supplying inherited custom `GOSUMDB`/`GOPROXY`, proving that the old
wrapper neither refused the trust settings nor queried effective `GOVERSION`.
No Go child, test body, live operation or credential-bearing process ran.

```sh
set -euo pipefail
export PATH=/opt/homebrew/bin:/usr/bin:/bin
[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || { printf '%s\n' 'reviewed canonical PATH and absolute Python interpreter required' >&2; exit 1; }
# g01-safe-python-heredoc: reviewed exact-parent fsmonitor fixture
/opt/homebrew/bin/python3 -I - <<'PY'
import subprocess
import tempfile
from pathlib import Path

parent = "6c55f5b67035fb1c7ac334984499cfe80d6bb86b"
packet = subprocess.check_output(["git", "show", f"{parent}:docs/evidence/g01-recovery-packet.md"], text=True)
status_start = packet.index('        [\n            "git",\n            "status",')
status_end = packet.index('        ],', status_start) + len('        ],')
status_argv = packet[status_start:status_end]
if '"-c"' in status_argv or "core.fsmonitor=false" in status_argv or "core.hooksPath=/dev/null" in status_argv:
    raise SystemExit("red setup changed: exact parent status command already guarded")
print(f"RED 4002895109: exact parent {parent} source-status argv lacks core.fsmonitor=false/core.hooksPath=/dev/null")
with tempfile.TemporaryDirectory() as td:
    root = Path(td)
    env = {"PATH": "/opt/homebrew/bin:/usr/bin:/bin", "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": "/dev/null", "HOME": td}
    subprocess.run(["git", "init", "-q"], cwd=root, env=env, check=True)
    (root / "source.go").write_text("package p\n", encoding="utf-8")
    subprocess.run(["git", "add", "source.go"], cwd=root, env=env, check=True)
    subprocess.run(["git", "-c", "user.name=probe", "-c", "user.email=probe@example.invalid", "commit", "-q", "-m", "source"], cwd=root, env=env, check=True)
    marker = root / "fsmonitor-invoked"
    hook = root / "fsmonitor-hook"
    hook.write_text(f"#!/bin/sh\nprintf invoked > {marker}\nprintf 'builtin:fake\\n'\n", encoding="utf-8")
    hook.chmod(0o700)
    subprocess.run(["git", "config", "core.fsmonitor", str(hook)], cwd=root, env=env, check=True)
    status = subprocess.run(["git", "status", "--porcelain=v1", "--untracked-files=all", "--ignored=matching", "--", "."], cwd=root, env=env, capture_output=True, text=True, check=False)
    if status.returncode != 0 or not marker.is_file() or marker.read_text(encoding="utf-8") != "invoked":
        raise SystemExit("red setup changed: configured fsmonitor hook was not invoked")
    print("RED 4002895109: exact-parent status invoked configured core.fsmonitor hook (marker=invoked); tree trust was not isolated")
PY
```

```sh
set -euo pipefail
export PATH=/opt/homebrew/bin:/usr/bin:/bin
# g01-safe-python-heredoc: reviewed exact-parent AST fixture
/opt/homebrew/bin/python3 -I - <<'PY'
import ast
import re
import shlex
import subprocess
from pathlib import Path

parent = "6c55f5b67035fb1c7ac334984499cfe80d6bb86b"
packet = subprocess.check_output(["git", "show", f"{parent}:docs/evidence/g01-recovery-packet.md"], text=True)
scanner_anchor = packet.index("def forbidden_command(tokens, depth=0):")
scanner_start = packet.rfind("source = Path(", 0, scanner_anchor)
scanner_end = packet.index("\nmatches = []", scanner_anchor)
namespace = {"Path": Path, "ast": ast, "re": re, "shlex": shlex}
exec(compile(packet[scanner_start:scanner_end], "<exact-parent-scanner>", "exec"), namespace)
for label, body in (
    ("import subprocess as sp; sp.run", 'import subprocess as sp\nsp.run(["gh", "api", "x"])'),
    ("from subprocess import run; run", 'from subprocess import run\nrun(["gh", "api", "x"])'),
    ("from os import system; system", 'from os import system\nsystem("docker version")'),
):
    if namespace["inspect_python_heredoc"](body, False) is not None:
        raise SystemExit(f"red setup changed: exact parent already rejects {label}")
    print(f"RED 4002895114: exact parent accepted unresolved command-capable import/call {label}")
PY
```

```sh
set -euo pipefail
export PATH=/opt/homebrew/bin:/usr/bin:/bin
# g01-safe-python-heredoc: reviewed exact-parent toolchain fixture
/opt/homebrew/bin/python3 -I - <<'PY'
import os
import subprocess
import sys

parent = "6c55f5b67035fb1c7ac334984499cfe80d6bb86b"
packet = subprocess.check_output(["git", "show", f"{parent}:docs/evidence/g01-recovery-packet.md"], text=True)
wrapper_start = packet.index("\nimport hashlib\n", packet.index("go_test_checked()")) + 1
wrapper_end = packet.index("\nPY\n}", wrapper_start)
wrapper = packet[wrapper_start:wrapper_end]
if "GOSUMDB" in wrapper or "GOPROXY" in wrapper or "GOVERSION" in wrapper:
    raise SystemExit("red setup changed: exact parent already binds trust/effective toolchain")
if 'toolchain_identity = env.get("GOTOOLCHAIN", "")' not in wrapper:
    raise SystemExit("red setup changed: exact parent no longer labels only requested GOTOOLCHAIN")
base = ["probe", "1", "0" * 64, "probe", "experiments/g01-scaleset:./livecanary", "default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8", "GOTOOLCHAIN=go1.26.8", "go", "test", "-C", "experiments/g01-scaleset", "./livecanary"]
class StopBeforeChild(Exception):
    pass
saved_env, saved_argv = dict(os.environ), sys.argv
saved_check_output, saved_run = subprocess.check_output, subprocess.run
calls = []
def stop(*args, **kwargs):
    calls.append(args[0] if args else kwargs.get("args"))
    raise StopBeforeChild
try:
    os.environ.clear()
    os.environ.update({"PATH": "/opt/homebrew/bin:/usr/bin:/bin", "LANG": "C", "GOSUMDB": "sum.invalid+deadbeef", "GOPROXY": "https://proxy.invalid"})
    sys.argv = base
    subprocess.check_output = stop
    subprocess.run = stop
    try:
        exec(compile(wrapper, "<exact-parent-wrapper>", "exec"), {"__name__": "__main__"})
    except StopBeforeChild:
        pass
finally:
    subprocess.check_output, subprocess.run = saved_check_output, saved_run
    sys.argv = saved_argv
    os.environ.clear()
    os.environ.update(saved_env)
if not calls:
    raise SystemExit("red setup changed: custom trust settings refused before first child")
print(f"RED 4002895117: exact parent accepted inherited custom GOSUMDB/GOPROXY and reached first child {calls[0]!r}")
print("RED 4002895117: exact parent build identity used requested GOTOOLCHAIN=go1.26.8; no effective GOVERSION/trust query")
PY
```

Recorded exact-parent red output:

```text
RED 4002895109: exact parent 6c55f5b67035fb1c7ac334984499cfe80d6bb86b source-status argv lacks core.fsmonitor=false/core.hooksPath=/dev/null
RED 4002895109: exact-parent status invoked configured core.fsmonitor hook (marker=invoked); tree trust was not isolated
RED 4002895114: exact parent accepted unresolved command-capable import/call import subprocess as sp; sp.run
RED 4002895114: exact parent accepted unresolved command-capable import/call from subprocess import run; run
RED 4002895114: exact parent accepted unresolved command-capable import/call from os import system; system
RED 4002895117: exact parent accepted inherited custom GOSUMDB/GOPROXY and reached first child ['git', 'rev-parse', '--show-toplevel']
RED 4002895117: exact parent build identity used requested GOTOOLCHAIN=go1.26.8; no effective GOVERSION/trust query
```

#### Minimal packet correction and focused green probe

The minimal correction adds one shared Git argv/config guard to every
wrapper-controlled source-tree, porcelain-status and intent-bit query:
`core.fsmonitor=false`, `core.hooksPath=/dev/null`, `GIT_CONFIG_NOSYSTEM=1`,
and `/dev/null` global/system config paths. The Python audit resolves direct
module aliases and imported command functions, while unresolved
command-capable calls and star imports fail closed even when a dynamic safe
marker is present. Before the first Go child, the wrapper requires the reviewed
`GOSUMDB=sum.golang.org` and `GOPROXY=https://proxy.golang.org,direct`, queries
`go env GOVERSION GOSUMDB GOPROXY`, and binds the verified effective version in
the build identity. Existing GOCACHEPROG/GOAUTH controls remain unchanged.

The focused green probe parsed the candidate wrapper/scanner, blocked every
possible Go child, exercised inherited and command-prefix trust overrides, and
used a temporary repository to verify that the configured fsmonitor hook was
not invoked by the guarded status command:

```sh
set -euo pipefail
export PATH=/opt/homebrew/bin:/usr/bin:/bin
[ "${PATH-}" = "/opt/homebrew/bin:/usr/bin:/bin" ] && [ -x /opt/homebrew/bin/python3 ] || exit 1
# g01-safe-python-heredoc: reviewed focused synthetic argv
/opt/homebrew/bin/python3 -I - <<'PY'
import ast
import os
import re
import shlex
import subprocess
import sys
import tempfile
from pathlib import Path

packet = Path("docs/evidence/g01-recovery-packet.md").read_text(encoding="utf-8")
wrapper_start = packet.index("\nimport hashlib\n", packet.index("go_test_checked()")) + 1
wrapper_end = packet.index("\nPY\n}", wrapper_start)
wrapper = packet[wrapper_start:wrapper_end]
ast.parse(wrapper, filename="<packet-wrapper>")
for marker in ('"core.fsmonitor=false"', '"core.hooksPath=/dev/null"', "GIT_CONFIG_NOSYSTEM", "GIT_CONFIG_GLOBAL", "GIT_CONFIG_SYSTEM", "reviewed_gosumdb", "reviewed_goproxy", "effective_toolchain_result", "GOVERSION", "effective_toolchain_identity"):
    if marker not in wrapper:
        raise SystemExit(f"green wrapper marker missing: {marker}")
if wrapper.index("reviewed_gosumdb =") > wrapper.index("repo_root = Path(") or wrapper.index("reviewed_goproxy =") > wrapper.index("repo_root = Path("):
    raise SystemExit("trust guard occurs after first Git query")
if not re.search(r"git_command\(\[\s*\"status\"", wrapper) or not re.search(r"git_command\(\[\s*\"ls-files\"", wrapper):
    raise SystemExit("source status/intent check does not use git_command")
if "toolchain_identity = effective_toolchain_identity" not in wrapper or '["go", "env", "GOVERSION", "GOSUMDB", "GOPROXY"]' not in wrapper:
    raise SystemExit("effective toolchain identity binding is missing")

scanner_anchor = packet.index("def forbidden_command(tokens, depth=0):")
scanner_start = packet.rfind("source = Path(", 0, scanner_anchor)
scanner_end = packet.index("\nmatches = []", scanner_anchor)
scanner_ns = {"Path": Path, "ast": ast, "re": re, "shlex": shlex}
exec(compile(packet[scanner_start:scanner_end], "<green-scanner>", "exec"), scanner_ns)
for body in ('import subprocess as sp\nsp.run(["gh", "api", "x"])', 'from subprocess import run\nrun(["gh", "api", "x"])', 'from os import system\nsystem("docker version")', 'import synthetic as sp\nsp.run(command)'):
    if scanner_ns["inspect_python_heredoc"](body, True) is None:
        raise SystemExit(f"alias/unresolved command-capable call accepted: {body!r}")
if scanner_ns["inspect_python_heredoc"]("import subprocess as sp\nsp.run(command)", True) is not None:
    raise SystemExit("reviewed dynamic alias with safe marker rejected")
if scanner_ns["inspect_python_heredoc"]("from subprocess import *\nrun(command)", True) is None:
    raise SystemExit("unresolved star import accepted")

base = ["probe", "1", "0" * 64, "probe", "experiments/g01-scaleset:./livecanary", "default+race+cgo1+cgo-tools-default+goexperiment-none+darwin-arm64-goarm64-v8.0+goroot-default+gofips140-off+go1.26.8", "GOTOOLCHAIN=go1.26.8", "go", "test", "-C", "experiments/g01-scaleset", "./livecanary"]
class StopBeforeChild(Exception):
    pass
saved_env, saved_argv = dict(os.environ), sys.argv
saved_check_output, saved_run = subprocess.check_output, subprocess.run
try:
    calls = []
    def stop(*args, **kwargs):
        calls.append((args[0] if args else kwargs.get("args"), kwargs.get("env")))
        raise StopBeforeChild
    os.environ.clear()
    os.environ.update({"PATH": "/opt/homebrew/bin:/usr/bin:/bin", "LANG": "C"})
    sys.argv = base
    subprocess.check_output, subprocess.run = stop, stop
    namespace = {"__name__": "__main__"}
    try:
        exec(compile(wrapper, "<green-wrapper>", "exec"), namespace)
    except StopBeforeChild:
        pass
    command, child_env = calls[0]
    if command != ["git", "-c", "core.fsmonitor=false", "-c", "core.hooksPath=/dev/null", "rev-parse", "--show-toplevel"]:
        raise SystemExit(f"first Git query not guarded: {command!r}")
    if {key: child_env.get(key) for key in ("GIT_CONFIG_NOSYSTEM", "GIT_CONFIG_GLOBAL", "GIT_CONFIG_SYSTEM")} != {"GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": "/dev/null", "GIT_CONFIG_SYSTEM": "/dev/null"}:
        raise SystemExit("first Git query did not use isolated config")
    if namespace["env"]["GOSUMDB"] != "sum.golang.org" or namespace["env"]["GOPROXY"] != "https://proxy.golang.org,direct":
        raise SystemExit("reviewed trust settings were not pinned")
finally:
    subprocess.check_output, subprocess.run = saved_check_output, saved_run
    sys.argv = saved_argv
    os.environ.clear()
    os.environ.update(saved_env)

for inherited, assignment, expected in (({"GOSUMDB": "sum.invalid+deadbeef"}, (), "GOSUMDB"), ({"GOPROXY": "https://proxy.invalid"}, (), "GOPROXY"), ({}, ("GOSUMDB=sum.invalid+deadbeef",), "GOSUMDB"), ({}, ("GOPROXY=https://proxy.invalid",), "GOPROXY")):
    calls = []
    saved_env, saved_argv = dict(os.environ), sys.argv
    def stop_override(*args, **kwargs):
        calls.append(args[0] if args else kwargs.get("args"))
        raise StopBeforeChild
    try:
        os.environ.clear()
        os.environ.update({"PATH": "/opt/homebrew/bin:/usr/bin:/bin", "LANG": "C", **inherited})
        sys.argv = base[:6] + list(assignment) + base[6:]
        subprocess.check_output, subprocess.run = stop_override, stop_override
        try:
            exec(compile(wrapper, "<green-trust-wrapper>", "exec"), {"__name__": "__main__"})
        except SystemExit as error:
            if expected not in str(error) or calls:
                raise
        else:
            raise SystemExit(f"{expected} override accepted")
    finally:
        subprocess.check_output, subprocess.run = saved_check_output, saved_run
        sys.argv = saved_argv
        os.environ.clear()
        os.environ.update(saved_env)

with tempfile.TemporaryDirectory() as td:
    root = Path(td)
    env = {"PATH": "/opt/homebrew/bin:/usr/bin:/bin", "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": "/dev/null", "GIT_CONFIG_SYSTEM": "/dev/null", "HOME": td}
    subprocess.run(["git", "init", "-q"], cwd=root, env=env, check=True)
    (root / "source.go").write_text("package p\n", encoding="utf-8")
    subprocess.run(["git", "add", "source.go"], cwd=root, env=env, check=True)
    subprocess.run(["git", "-c", "user.name=probe", "-c", "user.email=probe@example.invalid", "commit", "-q", "-m", "source"], cwd=root, env=env, check=True)
    marker = root / "fsmonitor-invoked"
    hook = root / "fsmonitor-hook"
    hook.write_text(f"#!/bin/sh\nprintf invoked > {marker}\nprintf 'builtin:fake\\n'\n", encoding="utf-8")
    hook.chmod(0o700)
    subprocess.run(["git", "config", "core.fsmonitor", str(hook)], cwd=root, env=env, check=True)
    guarded = ["git", "-c", "core.fsmonitor=false", "-c", "core.hooksPath=/dev/null", "status", "--porcelain=v1", "--untracked-files=all", "--ignored=matching", "--", "."]
    result = subprocess.run(guarded, cwd=root, env=env, capture_output=True, text=True, check=False)
    if result.returncode != 0 or marker.exists():
        raise SystemExit("guarded status invoked configured fsmonitor hook")
print("GREEN focused packet regression: passed; all source-status/intent Git queries use core.fsmonitor=false and core.hooksPath=/dev/null with isolated config; inherited/command custom GOSUMDB/GOPROXY refused before child, reviewed trust/effective GOVERSION query and identity binding present; subprocess/os aliases and unresolved command-capable calls fail closed; no Go/live child started")
PY
```

Recorded focused green output:

```text
GREEN focused packet regression: passed; all source-status/intent Git queries use core.fsmonitor=false and core.hooksPath=/dev/null with isolated config; inherited/command custom GOSUMDB/GOPROXY refused before child, reviewed trust/effective GOVERSION query and identity binding present; subprocess/os aliases and unresolved command-capable calls fail closed; no Go/live child started
```

The correction is packet-only and does not authorize any rerun or live
qualification. Rollback is narrow: restore only
`docs/evidence/g01-recovery-packet.md` to immutable parent
`6c55f5b67035fb1c7ac334984499cfe80d6bb86b`; preserve independent driver,
review and manual-runner state, and do not force-kill, prune or replay any
resource. The wrapper's reviewed trust values and effective identity remain
fixture/toolchain evidence only; this correction makes no hostile-code or
native macOS isolation claim.

#### Exact review URL ledger and dispositions

| Finding and immutable source | Exact review URL | Disposition and rollback evidence |
|---|---|---|
| 4002895109, source `6c55f5b67035fb1c7ac334984499cfe80d6bb86b` | [discussion 4002895109](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4002895109) | Reproduced configured repository `core.fsmonitor` invocation before the guard. Every wrapper-controlled source-tree, porcelain-status and intent-bit Git query now uses `core.fsmonitor=false`, `core.hooksPath=/dev/null`, `GIT_CONFIG_NOSYSTEM=1` and `/dev/null` global/system config. No hostile-code or live qualification is claimed; rollback is packet-only parent restoration. |
| 4002895114, source `6c55f5b67035fb1c7ac334984499cfe80d6bb86b` | [discussion 4002895114](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4002895114) | Reproduced `subprocess as sp`, imported `run`, and imported `system` aliases accepted by the exact parent. The AST audit resolves reviewed aliases/functions and fails closed on unresolved command-capable calls/imports, while preserving the existing dynamic safe-marker rule for resolved calls. Static-only probes ran; no forbidden/live command ran. Rollback is packet-only parent restoration. |
| 4002895117, source `6c55f5b67035fb1c7ac334984499cfe80d6bb86b` | [discussion 4002895117](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r4002895117) | Reproduced inherited custom `GOSUMDB`/`GOPROXY` reaching the first child and the old identity using requested `GOTOOLCHAIN` only. The wrapper now requires `GOSUMDB=sum.golang.org` and `GOPROXY=https://proxy.golang.org,direct`, verifies effective `GOVERSION`/trust via bounded `go env`, and binds effective toolchain identity; existing GOCACHEPROG/GOAUTH controls remain. No Go child or live qualification ran. Rollback is packet-only parent restoration. |
