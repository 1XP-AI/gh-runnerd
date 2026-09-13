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
| [Controller phase driver](g01-live-driver.md) | Fixture/source: bounded tagged controller operations; no worker launch or live result. | [candidate head `0e08ce8285846e26a94fb6fcb33bbeede662bf14`](https://github.com/1XP-AI/gh-runnerd/blob/0e08ce8285846e26a94fb6fcb33bbeede662bf14/docs/evidence/g01-live-driver.md) |
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
| Go toolchain | Experiment module directive `go 1.26.3`; verification toolchain `GOTOOLCHAIN=go1.26.8` / `go1.26.8` | Fixture/toolchain evidence only; every rerun records the exact toolchain. |
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
the selected SDK/source, Go toolchain/module, listener/adapter, journal or
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
| Idle drain and withdrawal | Only reuse the authoritative current PR #72 head `f5560ba950f77343e57034cc1cf85dc67f5ac922` and its unchanged fixture/source. | On that authoritative current PR #72 checkout: see the exact command in the block below; repeat with `go test -race` for the same selector. | Fixture/source: record physical-write markers as client facts and inconclusive server receipt; never reuse as live assignment/drain evidence. |
| Paired terminal/worker support | paired journal/lease, worker profile, `liveworker` runtime source/implementation, image/runtime pins and terminal selectors are unchanged. | See the exact controller update-policy and worker runtime commands plus the tagged partition commands below; the worker runtime selector includes the direct Docker state/inspection, mutation-EOF and changed-daemon contracts, while tagged partition commands retain `go1.26.8`, `-race`, `-count=1` and `-timeout=120s`, the complete worker `^TestPaired` partition is included, and the controller-side terminal groups remain exhaustive/disjoint. | Fixture: record receipt/identity checks only; no live worker or terminal success claim. |
| Actual live canary | There is no live evidence to reuse today. A future result is reusable only for the same immutable workflow/run attempt, source/head, resources, authority scope and approved observation boundary. | Rebuild/plan the exact reviewed tagged binary, then run only the explicitly authorized phase from [the driver](g01-live-driver.md); never substitute fixture commands or broaden phases. | Live: record sanitized server observations, authorization and unresolved outcomes; any changed target or boundary requires a fresh approval/rerun. |

The exact focused commands referenced above are recorded here. They are
prescriptions for future reruns, not completed results. None of these
prescriptions reports a completed test or live server success/receipt.

```sh
GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=45s -run '^(TestSDKACKBoundaries|TestSDKDemandAboveFiftyAndPartialAcquisition|TestSDKRepeatedStatisticsAnd202ReuseLastObservation|TestRecoveryAfterACKCallbackCrash|TestRecoveryMissingLifecycleCallback|TestRecoveryErrorsHoldReservationsAndRedact|TestSDKAcquisitionResponseLossAfterACK|TestSDKCapacityWithdrawalDoesNotFenceInFlightAcquisition|TestSDKHTTPFailuresAndSessionRefresh)$' .
GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=45s ./livecanary -run '^(TestSupportedListenerBarriersAndReservation|TestDriverBarriersThroughPinnedSDK|TestForeignIdentityAndUnreviewedWorkNeverACKOrDelete|TestAuditPR25MultiJobAcquisitionMustRefuseBeforeACK|TestNoMessageDoesNotCountAsCompletedBarrier)$'
```

```sh
GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=180s ./livecanary -run '^TestBaseline'
GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=180s ./livecanary -run '^(TestAdmission.*|TestAuditPR25DistinctStateDirectoriesMustShareCap)$'
GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=180s ./liveworker -run '^(TestAdmissionDirectoryUsesOSAccountWithoutEnvironmentFallback|TestAdmissionAuthorityChecksTheCurrentClaim|TestWorkerAdmissionCapsIndependentDirectories|TestWorkerAdmissionRetainsSlotAfterOutcomeAndClose|TestAdmissionRejectsCopiedJournalInDifferentDirectory|TestAdmissionSyncFailureMustBeRetried|TestAdmissionRefusesMissingUnsafeOrUnknownRootState|TestAdmissionInitializationLockPrecedesClaimCreation)$'
GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -tags=osusergo -count=1 -timeout=180s ./liveworker -run '^TestUnsupportedAccountLookupRefusesBeforeJournal$'
GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -tags=osusergo -count=1 -timeout=180s ./livecanary -run '^TestUnsupportedAccountLookupRefusesBeforeJournal$'
```

```sh
GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=45s -run '^(TestSDKJITLookupBeforeCreationDoesNotDiscoverIdentity|TestSDKJITResponseLossWithoutCommitDoesNotDiscoverIdentity|TestSDKJITResponseLossDiscoversIdentityWithoutReissuing)$' .
GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=45s ./livecanary -run '^(TestJITLostResponseIsSecretSafeAndNeverReissued|TestDriverBarriersThroughPinnedSDK)$'
```

```sh
GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=45s ./liveworker -run '^(TestOneWorkerNeverRecreatedAndOnlyJITAddedToEnvironment|TestUncertainStartNeverRetriesAndCannotCleanup|TestEveryRuntimeBoundaryRejectsProfileAndOwnershipMismatch|TestUnverifiedRunnerUpdatePolicyRefusesBeforeRuntime|TestSocketReplacementAfterPreflightCannotReceiveAnyMutation|TestSocketModesAndControllerOwnership|TestSocketPostConnectRecheckClosesBeforeHTTP|TestNoCreateBeforeDurableIntent|TestUnknownCreateNeverRetriesAfterRestart|TestCreationWarningsPreserveKnownIDWithoutAuthorizingStart|TestWorkerPreparationReturnsCanonicalSnapshotAndRejectsPriorEffect|TestUnixInspectRequiresStateFlagsBeforeMutation|TestDockerInspectExact(KnownStatesAndSerializableFacts|StatePresenceAndLegacyRequirements|RejectsMalformedOrAmbiguousBodiesBeforeMutation|NotFoundReportsOnlyTheExactGET|RejectsOtherResponsesAndInvalidTargets|RequiresSupported404Body|CancellationNeverReportsPresenceOrAbsence|RejectsReplacedSocket|EOFCancellationKeepsUnknownOutcome)|TestDockerInspectLegacyCleanupKeepsSignedAndAbsentExitPolicy|TestDockerInspectMapsPreserveCaseSensitiveKeysAndProfile|TestDockerCompletedMutationResponseSurvivesEOFCancellation|TestDockerInspectUnknownOrAbsentStatusCannotAuthorizeMutation|TestUnixRuntimeRejectsWrongIdentityImagesAndUnsupportedLimits|TestUnixRuntimeOneShotCreateStartAndNonForceCleanup|TestUnixRuntimeAmbiguousEffectsNeverRetry|TestChangedDaemonCannotCreate|TestChangedDaemonOrAbsentContainerNeverMeansCleanupComplete|TestUnixTransportRejectsSymlinksAndInheritedTCPDestinations|TestAuditPR28MissingBridgeMustNotStart|TestCleanupRetainsActiveAndUnknownWorkers|TestOwnedTerminalCleanupAndRunningRemovalRace)$'
GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=45s ./livecanary -run '^(TestCreateRequestsDisabledRunnerUpdate|TestUnconfirmedUpdateSettingQuarantinesCreate|TestUpdateSettingDriftStopsBeforeSessionOrJIT|TestUpdateSettingDriftDoesNotBlockSafeEmptyCleanup)$'
GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -tags=g01_worker -count=1 -timeout=45s ./cmd/g01-worker -run '^(TestBlockedJITInputHonorsDeadline|TestOfflinePlanAndRefusalDoNotReadSecretsOrEchoInput)$'
```

```sh
GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=45s ./livecanary -run '^(TestAmbiguousCreateNeverRetriesAfterRestart|TestObserve.*|TestObservationIntentFailureStopsBeforeRead|TestObservationResponseCaptureIsLocalAndRejectsOtherOperations|TestStatistics.*|TestDemandStatisticsAllowControlledProbeButNeverCleanup|TestInvalidOwnedProof.*|TestCanonicalPreparationRecordsNoPhaseOrRemoteIntent|TestCanonicalPreparationRefusesInvalidJournalAndPhase|TestCanonicalPreparationRecoveryAndDriverShareLocalGate|TestJournal.*|TestAuthority.*|TestInventoryStrictPages|TestInventoryMalformedStopsLegacyEffects|TestInventoryTransportRefusalIsBoundedAndSanitized|TestInventoryImpossibleTotalStopsBeforeNextPage|TestRosterActualTLSCompleteObservation|TestRosterPreservesOnlyAcceptedPagePrefix|TestRosterFinalPublicationGuardAfterDigest|TestRosterRefusesInvalidEntryWithoutNetwork|TestStrictJSONRejectsDecoderEquivalentDuplicateFields|TestStrictJSONRejectsDuplicateAuthorityFields|TestFailedDirectorySyncMustBeRetried|TestFileJournalRejectsUnrecordedAuthorityBeforeRawDriverEffect|TestRenewedRecoveryApprovalRetainsOwnedState)$'
GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=45s ./liveworker -run '^(TestPrivateJournalLocksAndRetainsReservationAcrossRestart|TestJournalRejectsChangedApprovalTornTailAndUnsafeFiles|TestAuthorityLeaseRefusesConcurrentRunsAndFencesClose|TestAuthorityRejectsReplacedJournalOrDirectory|TestStrictJSONRejectsDecoderEquivalentDuplicateFields|TestStrictInputRejectsAmbiguousOrExtraAuthorityFields|TestFailedDirectorySyncMustBeRetried|TestFileJournalRejectsUnrecordedAuthorityBeforeRawDriverEffect|TestRenewedRecoveryApprovalRetainsOwnedState)$'
GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=45s ./livecanary -run '^(TestDemandStatisticsAllowControlledProbeButNeverCleanup|TestCleanupOnlyForNeverIssuedWorkerWithExactReceipt|TestZeroStatisticsAndOptionalAbsencePermitEmptyCleanup|TestUnsafeStatisticsStopNewEffectsBeforeControlledMessage|TestEmptyAvailableWithWorkStatisticsStaysQuarantined|TestOlderPendingIntentSurvivesSuccessfulZeroInspection|TestUnownedDiscoveryEvidenceCannotAuthorizeCreationAfterAbsence|TestAuditPR25ObservedJobsMustBlockCleanup|TestUnexpectedWorkMessageQuarantinesBeforeSafeClose|TestObservedRunnerSurvivesLaterAbsenceAndFirstCleanup|TestObservationResultFailureSurvivesFileReopenAndInspection)$'
```

```sh
GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=45s -run '^TestSDKBusyRemovalSentinelAndRawErrorExposure$' .
GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=45s ./livecanary -run '^(TestHTTPErrorsDoNotReturnSecretResponseBody|TestSDKHTTPDebugDoesNotLogCredentials|TestSharedTransportRejectsOversizeSuccessAndErrorBodies|TestResponseReaderConsumesOnlyBudgetPlusOneAndRejectsTruncation|TestResponseBudgetAppliesAfterGzipDecompression|TestRealJournalCreateFailureBlocksRetryAndContainsNoErrorBody)$'
GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=45s ./liveworker -run '^(TestUncertainStartNeverRetriesAndCannotCleanup|TestUnixResponsesAreBoundedAndRedirectsNeverFollowed|TestObservationDoesNotJournalRawRuntimeStatus)$'
```

```sh
GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -tags=g01_live -count=1 -timeout=45s ./cmd/g01-live -run '^(TestPlanAndRefusalsNeverReadCredentialsOrEchoInputs|TestPreparationCommandNeverReadsCredentialsOrRunsRemotePhase|TestPairedTerminalModeReadsControllerInputAfterAllGates|TestPairedTerminalModeRejectsUnusedPhaseAndControllerFlagsBeforeInput|TestPairedTerminalModeRequiresWorkflowVerificationAuthorityBeforeInput|TestInheritedNamedCredentialFIFODelayedEOF|TestInheritedCredentialPipeStopsAtDeadline|TestCredentialInputRejectsNonPipeDescriptor|TestBlockedCredentialPipeStopsAtDeadline|TestCredentialInputAcceptsCompleteAndRejectsOversize)$'
GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -tags=g01_live -count=1 -timeout=45s ./livecanary -run '^(TestAuthoritySplitAndPolicyRejection|TestSDKTransportOwnership|TestCredentialAttestationMismatchAndExpiredTokenRejected|TestTransportRejectsPlaintextOffHostAndProxyBeforeNetwork)$'
```

```sh
GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset ./livecanary -run '^(TestBaselineAcquireTargetIsActionsOnly|TestPinnedSDKDrain.*|TestDriverDrainThroughPinnedSDKAndPollHook|TestDrainListenerWithdrawsWhilePollResponseIsHeld|TestDrainCancellationStopsBeforeReleasingHeldResponse)$' -count=1 -v -timeout=180s
```

```sh
terminal_heavy_tests='^TestPairedTerminal(FinalResultCapacity|PendingChildCapacity|EligibilityUsesFreshExactFacts|CapturedAcknowledgementCancellation|MissingAcknowledgementsAndPostchecks)$'
terminal_remainder_skip='^TestPairedTerminal(FinalResultCapacity|PendingChildCapacity|EligibilityUsesFreshExactFacts|CapturedAcknowledgementCancellation|MissingAcknowledgementsAndPostchecks|Actual(Controller|Worker)SyncFailures|PostIntent(JournalIdentity|AuthorityBoundaries)|ClosedReplayActualFile|WorkerReceiptSurvivesControllerWriteFailure|FixtureStorageFailure)$'
terminal_storage_tests='^TestPairedTerminal(Actual(Controller|Worker)SyncFailures|PostIntent(JournalIdentity|AuthorityBoundaries)|ClosedReplayActualFile|WorkerReceiptSurvivesControllerWriteFailure|FixtureStorageFailure)$'
GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -tags=g01_pair_fixture -race -count=1 -timeout=120s ./livecanary -run '^TestPaired' -skip '^TestPairedTerminal'
GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -tags=g01_pair_fixture -race -count=1 -timeout=120s ./livecanary -skip '^TestPaired'
GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -tags=g01_pair_fixture -race -count=1 -timeout=120s ./liveworker -run '^TestPaired'
GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -tags=g01_pair_fixture -race -count=1 -timeout=120s ./livecanary -run "$terminal_heavy_tests"
GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -tags=g01_pair_fixture -race -count=1 -timeout=120s ./livecanary -run '^TestPairedTerminal' -skip "$terminal_remainder_skip"
GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -tags=g01_pair_fixture -race -count=1 -timeout=120s ./livecanary -run "$terminal_storage_tests"
GOTOOLCHAIN=go1.26.8 go vet -C experiments/g01-scaleset -tags=g01_pair_fixture ./livecanary
GOTOOLCHAIN=go1.26.8 go vet -C experiments/g01-scaleset -tags=g01_pair_fixture ./liveworker
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
directory and source boundary were checked against target head
`ee8df8b7e00204c74a892b27f8b4c0ab278751ba`; the selector declaration audit is
anchored to immutable source commit
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

The following commands are the exact documentation/static checks used for this
correction. Their results are recorded immediately after each check; no command
below runs a test body or performs a live App, runner, Docker, Lima, Keychain,
launchd or workflow operation.

### Markdown links, JSON, and ledger shape

```sh
set -euo pipefail
python3 - <<'PY'
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
ledger_rows=$(awk '
  /^\| Boundary \| Reuse unchanged evidence when \| Exact focused rerun \| Class\/result to record \|$/ { in_table=1; next }
  in_table && /^\|---/ { next }
  in_table && /^\|/ { rows++; next }
  in_table && !/^\|/ { exit }
  END { print rows + 0 }
' docs/evidence/g01-recovery-packet.md)
test "$ledger_rows" -eq 10
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

The link/anchor checker reported 119 local targets with all targets present and
skipped external URLs after syntax recognition. `jq` exited 0, and the ledger
check exited 0 with 10 data rows and four columns in every row.

The packet's controller-side paired-terminal command fragment is compared with
the independently maintained fragment in `g01-paired-terminal.md`; the worker
partition and worker vet command are intentionally packet-only additions.

```sh
set -euo pipefail
diff -u \
  <(awk '/^terminal_heavy_tests=/{capture=1} capture { print; if ($0 ~ /^GOTOOLCHAIN=.*go vet -C experiments\/g01-scaleset -tags=g01_pair_fixture \.\/livecanary$/) exit }' docs/evidence/g01-paired-terminal.md) \
  <(awk '/^terminal_heavy_tests=/{capture=1} capture { print; if ($0 ~ /^GOTOOLCHAIN=.*go vet -C experiments\/g01-scaleset -tags=g01_pair_fixture \.\/livecanary$/) exit }' docs/evidence/g01-recovery-packet.md | grep -v 'liveworker')
printf 'paired-terminal command fragment comparison: passed; packet controller fragment matches g01-paired-terminal.md\n'
```

The paired-terminal fragment comparison exited 0 with no diff and printed the
pass message above.

### Stable checkout and selector audit

```sh
set -euo pipefail
test "$(git rev-parse --show-toplevel)" = "$(pwd -P)"
test -d experiments/g01-scaleset
test "$(git rev-parse --verify ee8df8b7e00204c74a892b27f8b4c0ab278751ba)" = "ee8df8b7e00204c74a892b27f8b4c0ab278751ba"
git diff --quiet ee8df8b7e00204c74a892b27f8b4c0ab278751ba -- experiments/g01-scaleset
test "$(git rev-parse 1396e201d905be204c3ac697be43723820581314:docs/evidence/g01-red.md)" = "c36e0af0c8e9b301f4889a02454c83ece8d5942f"
printf 'stable checkout audit: passed; repo root is current directory, experiments/g01-scaleset exists, target source is unchanged, and g01-red.md resolves to its pinned blob\n'
```

The stable checkout audit exited 0. It confirmed the target head, unchanged
`experiments/g01-scaleset` source, and the `g01-red.md` commit/blob pin; it did
not inspect or execute any live system.

The focused offline selector checks are list-only source checks. They were run
against the unchanged exact source tree above and did not execute test bodies.

```sh
GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=45s ./liveworker -list '^(TestSocketModesAndControllerOwnership|TestSocketPostConnectRecheckClosesBeforeHTTP|TestNoCreateBeforeDurableIntent|TestUnknownCreateNeverRetriesAfterRestart|TestCreationWarningsPreserveKnownIDWithoutAuthorizingStart|TestWorkerPreparationReturnsCanonicalSnapshotAndRejectsPriorEffect|TestUnixInspectRequiresStateFlagsBeforeMutation|TestDockerInspectExact(KnownStatesAndSerializableFacts|StatePresenceAndLegacyRequirements|RejectsMalformedOrAmbiguousBodiesBeforeMutation|NotFoundReportsOnlyTheExactGET|RejectsOtherResponsesAndInvalidTargets|RequiresSupported404Body|CancellationNeverReportsPresenceOrAbsence|RejectsReplacedSocket|EOFCancellationKeepsUnknownOutcome)|TestDockerInspectLegacyCleanupKeepsSignedAndAbsentExitPolicy|TestDockerInspectMapsPreserveCaseSensitiveKeysAndProfile|TestDockerCompletedMutationResponseSurvivesEOFCancellation|TestDockerInspectUnknownOrAbsentStatusCannotAuthorizeMutation|TestPrivateJournalLocksAndRetainsReservationAcrossRestart|TestJournalRejectsChangedApprovalTornTailAndUnsafeFiles|TestAuthorityLeaseRefusesConcurrentRunsAndFencesClose|TestAuthorityRejectsReplacedJournalOrDirectory|TestChangedDaemonCannotCreate)$'
GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=45s ./livecanary -list '^(TestCanonicalPreparationRecordsNoPhaseOrRemoteIntent|TestCanonicalPreparationRefusesInvalidJournalAndPhase|TestCanonicalPreparationRecoveryAndDriverShareLocalGate|TestZeroStatisticsAndOptionalAbsencePermitEmptyCleanup)$'
GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -tags=osusergo -count=1 -timeout=45s ./livecanary -list '^TestUnsupportedAccountLookupRefusesBeforeJournal$'
GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=45s ./livecanary -list '^(TestAmbiguousCreateNeverRetriesAfterRestart|TestObserve.*|TestStatistics.*|TestInvalidOwnedProof.*|TestJournal.*|TestAuthority.*)$'
```

All four commands exited 0 and listed, respectively, 25 `liveworker` names,
4 preparation names, 1 `osusergo` name and 26 reconciliation names; no test
body ran.

The declaration consistency check also avoids self-referential line numbers and
uses immutable source/tree assertions plus quiet presence/absence checks:

```sh
set -euo pipefail
test "$(git rev-parse 95cd9210620c54e098ecbe0df1217af1659f0c74)" = "95cd9210620c54e098ecbe0df1217af1659f0c74"
test "$(git rev-parse '95cd9210620c54e098ecbe0df1217af1659f0c74^{tree}')" = "d8b79cd1ddc6993792a44a8e8ae88985ce7466c0"
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
printf 'selector declaration audit: passed; immutable source/tree, requested declarations, package names, and build constraints matched; no test bodies executed\n'
```

### Diff and staged secret/private-path scan

After staging only this packet file, the final local checks were:

```sh
set -euo pipefail
test "$(git diff --cached --name-only)" = "docs/evidence/g01-recovery-packet.md"
test "$(git diff --cached --name-only | wc -l | tr -d ' ')" -eq 1
git diff --check
git diff --cached --check
private_user_root='/'"Users/"
private_home_root='/'"home/"
private_var_root='/'"private/var/"
secret_private_pattern='^\+.*(-----BEGIN[[:space:]]+[A-Z0-9 ]*PRIVATE KEY|gh[pousr]_[A-Za-z0-9_]+|github_pat_[A-Za-z0-9_]+|AKIA[0-9A-Z]{16}|xox[baprs]-[A-Za-z0-9-]+|'"${private_user_root}"'|'"${private_home_root}"'|'"${private_var_root}"')'
if git diff --cached --unified=0 -- docs/evidence/g01-recovery-packet.md | rg -q "$secret_private_pattern"; then
  printf 'staged secret/private-path scan: FAILED\n'
  exit 1
fi
printf 'diff and staged secret/private-path checks: passed; working/staged diff checks exited 0, one staged packet path, and no added-line matches\n'
```

The staged diff check exited 0, the staged path set was exactly this packet
file, and the added-line secret/private-path scan found no matches. No check is
claimed against an unrecorded SHA. The staged correction diff is distinct from
the cumulative PR #78 diff; the cumulative history still includes the
independent driver correction from `7ce053380003c9260f46bf93ea118893b95f01e7`
and team-review routing corrections from
`f59a30532bfdc62876065dcf8b4997520606e6a6`, which remain outside this staged
change.
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
historical [secret/error finding 3999010986](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999010986)
remains addressed by the dedicated Secret and error handling row and its root
and livecanary commands above; no issue, Project, or Goal state is changed.
