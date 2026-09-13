# G01 reusable recovery evidence packet

Date: 2026-09-13. Issue: [G01 / #1](https://github.com/1XP-AI/gh-runnerd/issues/1).
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
`6ce025902cd964747a078c2aabe7340ebc667eca` behind an adapter. Use the released
high-level listener with independent reconciliation, keep one serialized
session owner per pool, and preserve the upstream message order:

```text
statistics observation -> message ACK (DeleteMessage) -> available-job acquisition -> lifecycle callbacks
```

The packet records the SDK's order; it does **not** reorder ACK after callbacks
or acquisition, and it does not claim exactly-once delivery, idempotent
acquisition/JIT, durable execution, linearizability or server-side receipt.
Callbacks are observations. Reconciliation reads
`TotalAssignedJobs` at startup, after listener failure/session replacement and
on a bounded schedule, with session-generation fencing and conservative
ownership/quarantine rules. A later zero or absent observation cannot erase an
earlier work-bearing or uncertain observation.

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
| [JIT causality follow-up](g01-jit-causality.md) | Fixture: committed-vs-requested synthetic JIT creation and response-loss controls; no live bug claim. | [ba113977d03fc209ca147edbe10b766eed2c0fe3](https://github.com/1XP-AI/gh-runnerd/blob/ba113977d03fc209ca147edbe10b766eed2c0fe3/docs/evidence/g01-jit-causality.md); causal fix [7079b255d09ce0456b5843a8695277689d52e54e](https://github.com/1XP-AI/gh-runnerd/commit/7079b255d09ce0456b5843a8695277689d52e54e) |
| [Live-canary plan](g01-live-canary.md) | Live plan and authorization checklist; every phase remains unchecked/not run. | [d0afc56d471e8d63d153f6d26aeb95915d141d87](https://github.com/1XP-AI/gh-runnerd/blob/d0afc56d471e8d63d153f6d26aeb95915d141d87/docs/evidence/g01-live-canary.md) |
| [Controller phase driver](g01-live-driver.md) | Fixture/source: bounded tagged controller operations; no worker launch or live result. | [2fd5844f3f5c248479d0a97e642113dcf9c46a9a](https://github.com/1XP-AI/gh-runnerd/blob/2fd5844f3f5c248479d0a97e642113dcf9c46a9a/docs/evidence/g01-live-driver.md) |
| [Idle-drain observation](https://github.com/1XP-AI/gh-runnerd/blob/f5560ba950f77343e57034cc1cf85dc67f5ac922/docs/evidence/g01-idle-drain.md) | Fixture + source at the authoritative current PR #72 head; PR #72 remains open/pending CI, and this is offline evidence only. | [PR #72 head `f5560ba950f77343e57034cc1cf85dc67f5ac922`](https://github.com/1XP-AI/gh-runnerd/commit/f5560ba950f77343e57034cc1cf85dc67f5ac922) |
| [Identity/reconciliation](g01-identity-reconciliation.md) | Source + fixture limits for runner, request, job, REST and terminal identity; no live equality claim. | [9fe1b43cceb10c3ce92b26f8262c72148ded6c60](https://github.com/1XP-AI/gh-runnerd/blob/9fe1b43cceb10c3ce92b26f8262c72148ded6c60/docs/evidence/g01-identity-reconciliation.md) |
| [Exact observation adapter](g01-exact-observations.md) | Fixture: bounded exact-ID/status/provenance readers; no affirmative live recovery or cleanup. | [d62242b07b779ff6f7414e97096d2c8828634a5d](https://github.com/1XP-AI/gh-runnerd/blob/d62242b07b779ff6f7414e97096d2c8828634a5d/docs/evidence/g01-exact-observations.md) |
| [Worker/JIT harness](g01-worker-harness.md) | Source + fixture: worker profile and secret boundary; no image startup, JIT mint, workflow or Docker result. | [c1c0b6a6f2f94651546f533576eb3bbe8a4089b4](https://github.com/1XP-AI/gh-runnerd/blob/c1c0b6a6f2f94651546f533576eb3bbe8a4089b4/docs/evidence/g01-worker-harness.md) |
| [Paired collection](g01-paired-baseline.md) and [terminal path](g01-paired-terminal.md) | Fixture: serialized controller/worker receipts and terminal correlation; no live cleanup or success claim. | [8dd64adc551ba5174892807a678e8bc614d0a474](https://github.com/1XP-AI/gh-runnerd/blob/8dd64adc551ba5174892807a678e8bc614d0a474/docs/evidence/g01-paired-baseline.md), [cf67d4aeb511116fee0de31a4ac38409f28fa29f](https://github.com/1XP-AI/gh-runnerd/blob/cf67d4aeb511116fee0de31a4ac38409f28fa29f/docs/evidence/g01-paired-terminal.md) |
| Current source anchor | This packet is based on clean `origin/main` at the immutable source tree below. | [dce795a871865a8a2ee728151cb161e55081c8b7](https://github.com/1XP-AI/gh-runnerd/commit/dce795a871865a8a2ee728151cb161e55081c8b7) |

The idle-drain link intentionally points at the authoritative current PR #72
head and is not copied into this base worktree. PR #72 remains open/pending CI;
do not turn its offline result into a live result or claim that PR #72 is
mergeable from this packet.

## Exact pins and transport choices

| Boundary | Exact selection | Classification / exposure limit |
|---|---|---|
| Scale Set SDK | `github.com/actions/scaleset v0.4.0`; tag/source `6ce025902cd964747a078c2aabe7340ebc667eca`; [release](https://github.com/actions/scaleset/releases/tag/v0.4.0), [listener source](https://github.com/actions/scaleset/blob/6ce025902cd964747a078c2aabe7340ebc667eca/listener/listener.go), [module](https://github.com/actions/scaleset/blob/6ce025902cd964747a078c2aabe7340ebc667eca/go.mod) | Source + fixture pin. The module declares Go `1.25.3`; the audited comparison is not the selected release. |
| Go toolchain | Experiment module directive `go 1.26.3`; verification toolchain `GOTOOLCHAIN=go1.26.8` / `go1.26.8` | Fixture/toolchain evidence only; every rerun records the exact toolchain. |
| Runner | `v2.337.0`, source `397b032cbf865e9c3ddfab89d533ec19325e1273`; [release](https://github.com/actions/runner/releases/tag/v2.337.0), [command parser](https://github.com/actions/runner/blob/397b032cbf865e9c3ddfab89d533ec19325e1273/src/Runner.Listener/CommandSettings.cs), [JIT materialization](https://github.com/actions/runner/blob/397b032cbf865e9c3ddfab89d533ec19325e1273/src/Runner.Listener/Runner.cs) | Source pin; no runner binary was downloaded or executed by the contract evidence. |
| JIT bootstrap | Select worker-only `ACTIONS_RUNNER_INPUT_JITCONFIG` as parsed by runner `v2.337.0`; [environment parsing](https://github.com/actions/runner/blob/397b032cbf865e9c3ddfab89d533ec19325e1273/src/Runner.Listener/CommandSettings.cs) | Source-supported fallback, with no separate transport version. It avoids argv but remains sensitive in initial environment, process memory, container metadata and runner-written files. `--jitconfig` exposes argv; stdin/FD/file transport is not established and is not claimed. |
| REST observation | GitHub REST API header `2022-11-28` for exact runner/job reads; Scale Set acquisition uses SDK's `api-version=6.0-preview` | Source/fixture request contract, not live acceptance or freshness evidence. |
| Worker runtime support | Docker Engine API `1.45`, Moby schema `v26.1.5`, and the worker-harness image reference `ghcr.io/actions/actions-runner@sha256:f5a0d9a3d857315f2aed7075a02a29f46927ad198221c3b1c66585ae9fe36c0d` | Supporting fixture profile only; actual daemon/image normalization and runner startup remain live gaps. |

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
| ACK/callback loss recovery | `g01-contract.md` and `g01-red.md` recovery rows; ADR 0002 recovery contract. | Fixture + source | Independent statistics reads can restore aggregate demand; missing callback/request identity remains quarantined. No process-kill/restart or durable production journal result is claimed. |
| Acquisition intent and response loss | `TestSDKAcquisitionResponseLossAfterACK`; driver `before-acquire`/`acquire-loss`; [live-canary acquisition phase](g01-live-canary.md#minimal-execution-phases). | Source + fixture + live plan | ACK precedes exactly one `AcquireJobs` request; a suppressed response retains reservation and quarantines. No retry, server reoffer, accepted-request read-back or idempotency claim. A live disposable acquisition observation is still gated. |
| JIT intent and response loss | [JIT causality follow-up](g01-jit-causality.md), `TestSDKJITLookupBeforeCreationDoesNotDiscoverIdentity`, `TestSDKJITResponseLossWithoutCommitDoesNotDiscoverIdentity` and committed response-loss control. | Fixture + source | Synthetic lookup is causally tied to fixture-side commit; JIT value/result identity is discarded and reservation quarantined. No JIT-secret recovery, duplicate-name guarantee, idempotency or real runner launch is claimed. |
| Reconciliation | ADR 0002; [identity evidence](g01-identity-reconciliation.md); exact observation and baseline listener records. | Source + fixture | Read `TotalAssignedJobs` at bounded recovery points, fence session generations, and keep counts as snapshots. A matching ID is only a candidate; no revision, atomic cross-service snapshot or linearizability is established. |
| Quarantine and ownership | [controller driver](g01-live-driver.md), [audit corrections](g01-audit-corrections.md), [worker admission](g01-worker-admission.md). | Fixture + source | Unknown create/session/request/JIT/acquisition/callback outcomes, foreign identities, missing/invalid stats and stale work retain reservations and stop new effects. Name, label, zero count or elapsed time never authorizes adoption/deletion. |
| Idle assignment and drain | [idle-drain evidence at PR #72 head](https://github.com/1XP-AI/gh-runnerd/blob/f5560ba950f77343e57034cc1cf85dc67f5ac922/docs/evidence/g01-idle-drain.md); [live-canary drain phases](g01-live-canary.md#minimal-execution-phases). | Source + fixture + live gap | Offline hook uses the released listener's `SetMaxRunners(0)` and retains ACK-before-acquisition; client-side request markers do not prove server receipt or an atomic drain. Real old-poll acquisition/idle assignment and busy-safe removal remain unverified. |
| Terminal identity and completion | [identity reconciliation](g01-identity-reconciliation.md), [exact observations](g01-exact-observations.md), [paired baseline](g01-paired-baseline.md) and [terminal path](g01-paired-terminal.md). | Source + fixture | Keep SDK request/job IDs, REST job/runner IDs, run attempt, runner identity and local exit distinct; bind an exact tuple before classifying. No live job eligibility, successful execution, per-request release or cleanup proof is claimed. |
| Secret and error boundary | Contract runner/JIT section, [worker harness](g01-worker-harness.md), and driver redaction rules. | Source + fixture | No secrets, JIT values, raw bodies or raw SDK errors belong in this packet. Environment transport remains exposed to trusted same-user/process/container surfaces; modes and same-user ownership are not hostile-code isolation. |
| Rollback and preservation | ADR 0002 and [live-canary exit/cleanup](g01-live-canary.md#exit-evidence-and-cleanup). | Source + plan | Documentation rollback is a reviewed revert of this packet/correction commit. For any future live run: stop new admission, let owned busy work finish, quarantine uncertainty, and remove only individually verified disposable resources; preserve manual runners and never force-kill, prune or replay. |

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
the selected SDK/source, Go toolchain/module, listener/adapter, journal or
authority code, fixture endpoint/decoder, test tags/selectors, runner/parser,
JIT transport/image, or live target/approval changes. A documentation-only
wording/link correction does not invalidate an unchanged offline boundary.

| Boundary | Reuse unchanged evidence when | Exact focused rerun | Class/result to record |
|---|---|---|---|
| ACK, callback loss and acquisition order | SDK commit, listener source, adapter call order and root protocol fixtures are unchanged. | `cd experiments/g01-scaleset && GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=45s -run '^(TestSDKACKBoundaries|TestRecoveryAfterACKCallbackCrash|TestSDKAcquisitionResponseLossAfterACK|TestSDKCapacityWithdrawalDoesNotFenceInFlightAcquisition)$' .` | Fixture: record ACK-before-acquisition and reservation/quarantine behavior; do not add server or exactly-once claims. |
| Baseline listener/admission | listener, strict source reader, journal/lease, caps and selectors are unchanged. | `cd experiments/g01-scaleset && GOTOOLCHAIN=go1.26.8 go test -race ./livecanary -run '^TestBaseline' -count=1` | Fixture: record actual test result and pinned SDK; no live/worker result. |
| JIT causality and transport | runner source/parser, JIT transport, causal fixture and response-loss controls are unchanged. | `cd experiments/g01-scaleset && GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=45s -run '^(TestSDKJITLookupBeforeCreationDoesNotDiscoverIdentity|TestSDKJITResponseLossWithoutCommitDoesNotDiscoverIdentity|TestSDKJITResponseLossDiscoversIdentityWithoutReissuing)$' .` | Fixture/source: record one-request controls and quarantine; never print or retain JIT. |
| Reconciliation/identity/quarantine | statistics readers, generation fences, ownership checks and journal schema are unchanged. | `cd experiments/g01-scaleset && GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=45s ./livecanary -run '^(TestObserve|TestStatistics|TestInvalidOwnedProof|TestJournal|TestAuthority)'` | Fixture: record normalized facts and retained uncertainty; no live equality/absence claim. |
| Tagged controller/JIT input boundary | `g01_live` source, input reader and refusal tests are unchanged. | `cd experiments/g01-scaleset && GOTOOLCHAIN=go1.26.8 go test -race -tags=g01_live -count=1 -timeout=180s ./cmd/g01-live` | Fixture: record no-secret/no-echo/refusal result; no credential or live phase. |
| Idle drain and withdrawal | Only reuse the authoritative current PR #72 head `f5560ba950f77343e57034cc1cf85dc67f5ac922` and its unchanged fixture/source. | On that authoritative current PR #72 checkout: `cd experiments/g01-scaleset && GOTOOLCHAIN=go1.26.8 go test ./livecanary -run '^(TestBaselineAcquireTargetIsActionsOnly|TestPinnedSDKDrain.*|TestDriverDrainThroughPinnedSDKAndPollHook|TestDrainListenerWithdrawsWhilePollResponseIsHeld|TestDrainCancellationStopsBeforeReleasingHeldResponse)$' -count=1 -v -timeout=180s`; repeat with `go test -race` for the same selector. | Fixture/source: record physical-write markers as client facts and inconclusive server receipt; never reuse as live assignment/drain evidence. |
| Paired terminal/worker support | paired journal/lease, worker profile, image/runtime pins and terminal selectors are unchanged. | `GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -tags=g01_pair_fixture -race -count=1 -timeout=120s ./livecanary -run '^TestPaired' -skip '^TestPairedTerminal'` | Fixture: record receipt/identity checks only; no live worker or terminal success claim. |
| Actual live canary | There is no live evidence to reuse today. A future result is reusable only for the same immutable workflow/run attempt, source/head, resources, authority scope and approved observation boundary. | Rebuild/plan the exact reviewed tagged binary, then run only the explicitly authorized phase from [the driver](g01-live-driver.md); never substitute fixture commands or broaden phases. | Live: record sanitized server observations, authorization and unresolved outcomes; any changed target or boundary requires a fresh approval/rerun. |

For every rerun, retain the exact source/evidence SHA, command, exit status,
elapsed time where useful, classification and limitations. Do not convert a
skipped, unavailable or unauthorized live phase into “passed.”

## Documentation-only validation

This packet and the two linked wording corrections require markdown/link-target,
JSON syntax, diff, and secret/private-path checks only. No artificial Go red or
green test is created for documentation changes. On this candidate, the local
markdown link/anchor checker reported all local targets present (external URLs
were syntax-skipped), `jq empty docs/backlog.json` exited 0, `git diff --check`
and `git diff --cached --check` exited 0, and the staged added-line
secret/private-path scan reported no matches. The staged diff was inspected as
three documentation files (172 insertions, 2 deletions); no issue, Project, PR
or Goal state is changed.
