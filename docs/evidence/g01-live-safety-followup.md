# G01 live safety follow-up

This follow-up starts from the exact PR #72 head `f5560ba950f77343e57034cc1cf85dc67f5ac922`. It addresses the five independent-review NO-GO findings recorded on G01 issue #1 and PR #72. It is an offline safety change only; it does not authorize GitHub access, workflow dispatch, runner creation, scale-set creation, credential minting, or live evidence.

## Safety decisions

| Finding | Decision |
|---|---|
| Repeated `inspect` despite one slot | `inspect` now consumes a durable `phase` record before its read path. Replay rejects a second inspect as quarantined, including after a failed or partially observed attempt. |
| Direct FIFO/controller input without broker provenance | Tagged controller and paired-terminal execution are quarantined before reading stdin. The existing stdin boundary cannot independently attest that its caller is the broker. No synthetic environment, flag, token field, or process convention was added as an attestation. |
| Tagged controller entry outside the broker-only path | The tagged execution flags remain disabled at the command boundary. Preparation and plan-only paths remain local and non-executing. |
| Workflow phase not bound to controller phase | The change adds only a structural approval-shape guard: a non-paired controller approval carries one stable multi-phase authority membership set, and the selected outer phase must be a member of that set. It does not verify that workflow input selected that phase; workflow input-to-controller-phase binding remains an explicit unresolved gap, so tagged controller execution stays quarantined. Paired-terminal keeps its separate fixed sequence but is also blocked at the tagged controller entry. |
| Cleanup lacks atomic ownership/freshness fence | The current adapter exposes only unconditional `DeleteScaleSet(context, id)`. The driver rejects a missing `ConditionalScaleSetDeleter` before journal authorization or API access, so this pre-journal refusal does not write a quarantine record or claim to quarantine a journal. Once the capability is present and cleanup has authorized the journal, a failed preparation/conditional delete records unresolved intent and quarantines the journal; inventory-before-delete and inventory-after-delete reads are not such a fence. |

The exact blockers are therefore capability-level, not unverified attestation claims: the controller has no authenticated broker provenance channel, workflow input is not yet bound to the controller phase, and the SDK adapter has no conditional owner/freshness delete operation. Those paths remain disabled until a new reviewed design and adapter contract provide these properties.

## TDD and offline verification

The focused red tests were added before the implementation and reproduced the original behavior:

```text
go test . -run 'Test(InspectConsumesSingleDurablePhaseSlot|CleanupRequiresAtomicOwnershipFreshnessFence)$' -count=1
FAIL: repeated inspect returned nil; cleanup returned nil.

go test -tags='g01_live g01_pair_fixture' . -run TestTaggedControllerExecutionRequiresBrokerProvenanceBeforeInput -count=1
FAIL: controller input was read 2 times before broker provenance.
```

The broker red was observed before the shape guard in the predecessor test
`TestBrokerControllerApprovalBindsPhaseAuthorityAndSource`: a controller
approval with multiple phase authorities was accepted. The current regression
is intentionally named `TestBrokerControllerApprovalShapeMatchesAuthorityAndSource`;
it verifies only that structural guard and does not claim workflow-input
verification.

The structural guard is not workflow-input verification. That verification remains an explicit unresolved gap and is not claimed by this document. The final command/result and commit are recorded with the local handoff. No live operation or private evidence is represented as passing.

## Exact-head Codex finding audit

The inline finding on PR #81 (`discussion_r4021079691`) was audited against its
named exact head `4ecd46d`. Its premise is stale: admission reconstructs
`provenanceNonces` from every prior ledger claim, builds the new
`brokerProvenanceBinding`, rejects `provenanceNonces[binding.ReceiptNonce]`, and
only then appends `c.event`. The sequential regression
`TestBrokerAdmissionRejectsDuplicateProvenanceNonceBeforeAppend` fails in both
normal and `-race` modes under a controlled mutant that removes that
before-append check, with the observed assertion `duplicate provenance nonce
accepted`; both modes pass with the exact-head guard restored:

```text
GOTOOLCHAIN=go1.26.8 go test -count=1 -run '^TestBrokerAdmissionRejectsDuplicateProvenanceNonceBeforeAppend$' .
FAIL: broker_admission_test.go:177: duplicate provenance nonce accepted

GOTOOLCHAIN=go1.26.8 go test -race -count=1 -run '^TestBrokerAdmissionRejectsDuplicateProvenanceNonceBeforeAppend$' .
FAIL: broker_admission_test.go:177: duplicate provenance nonce accepted

# Guard restored:
GOTOOLCHAIN=go1.26.8 go test -count=1 -run '^TestBrokerAdmissionRejectsDuplicateProvenanceNonceBeforeAppend$' .
ok
GOTOOLCHAIN=go1.26.8 go test -race -count=1 -run '^TestBrokerAdmissionRejectsDuplicateProvenanceNonceBeforeAppend$' .
ok
```

This is an evidence-based stale-finding rebuttal; no production guard change
was required.

## PR #81 exact-head Codex finding follow-up

The following corrections were reproduced against the exact review head
`92f60da67b7b31646f492ae229c3327436d7e0ad` with offline fixtures only:

| Discussion | Reproduction and correction |
|---|---|
| `discussion_r4021411205` | `head_branch` alone cannot distinguish `refs/heads/main` from `refs/tags/main`. The workflow-run adapter now accepts only attested `refs/heads/...` until an authoritative ref-type field exists; the branch/tag-collision regression rejects the tag. |
| `discussion_r4021411208` | `TestCleanupMissingConditionalDeleterStopsBeforeJournalAuthorization` confirms the missing capability returns quarantine with no journal authorization, event or delete. Post-authorization preparation/conditional-delete failures remain durable unknown state and cannot retry; the evidence below does not call the pre-journal type assertion a journal quarantine. |
| `discussion_r4021411210` | The live controller approval schema now recognizes the broker's optional `workflow_ref` field under strict decoding, retains it in the approval digest, and has a focused `ReadApproval` regression. This changes no live gate: the tagged controller and paired entrypoints remain quarantined. |
| `discussion_r4021411215` | The tagged live command emits the explicit fixed result `canary quarantined; broker provenance is required` only after local approval/state validation. The paired bridge checks that result; `TestPairedBridgeDoesNotTreatGenericRejectAsQuarantine` proves the generic refusal cannot silently produce a skip. |
| `discussion_r4021411222` | Provenance `Attest` and `Verify` receive a derived context bounded by the parent deadline, approval/controller expiry and a ten-minute maximum. Timeout and canceled-context regressions verify that blocked adapters cannot outlive the authority context. |

The earlier stale nonce audit (`discussion_r4021079691`) and the exact
live-entrypoint quarantine remain in force. No workflow replay, GitHub App/API
call, credential mint, runner operation or live validation was performed.

## Remaining external G01 gates

The workflow-input gate remains blocked by a capability, not by the signed
fixture tests: `BrokerProvenanceAdapter` has no broker-authenticated workflow
source, and the GitHub workflow-run response cannot attest which input selected
the controller phase. Re-enabling tagged controller or paired execution
requires a reviewed broker channel that authenticates that input-to-phase
binding before credential input and API effects; production therefore remains
quarantined.

The cleanup gate remains blocked by the pinned `github.com/actions/scaleset
v0.4.0` client: `SDKAPI.DeleteScaleSet` is unconditional and exposes no
provider version, ETag, or equivalent atomic conditional-delete operation. The
existing `ConditionalScaleSetDeleter` contract and synthetic tests are the
minimal offline evidence; a reviewed provider adapter implementing the
server-side owner/freshness condition in the delete operation is required
before any live cleanup authorization. No credentials or live operation were
used here.

## Conditional cleanup contract follow-up

The controller driver now has a narrow `ConditionalScaleSetDeleter` contract.
Cleanup first requires that capability, then binds the exact created scale-set
ID, nonce-derived name, runner-group ID, owner nonce and fresh runner-inventory
digest to a provider version or ETag returned by `PrepareScaleSetDeletion`.
`DeleteScaleSetIfOwned` must submit that revision as a server-side conditional
delete in the same operation; it must never delegate to the existing
unconditional `DeleteScaleSet` or substitute an observe-then-delete sequence.
Missing capability is a separate pre-journal refusal: `Run("cleanup")` checks
for `ConditionalScaleSetDeleter` before `authorizePhase`, so it does not append
an event or quarantine a journal. After journal authorization and durable
delete intent, mismatched identity/nonce/inventory, missing freshness or an
adapter error records unresolved state, quarantines the journal and prevents
retry. Inventory reads before and after deletion remain supporting evidence
only, not the atomic fence.

The pinned `github.com/actions/scaleset v0.4.0` adapter still exposes no
conditional delete, version or ETag contract, so it deliberately does not
implement `ConditionalScaleSetDeleter`; the real SDK cleanup path remains
quarantined. Offline tests use a synthetic adapter to prove the exact-match
path and reject replacement between fence preparation and deletion; no GitHub,
credential, runner, Docker, Lima, Keychain or launchd operation was run.

The focused TDD evidence for this contract was:

```text
GOTOOLCHAIN=go1.26.8 go test -count=1 -run 'TestCleanupConditionalFence(AcceptsExactOwnerAndFreshnessMatch|RejectsConcurrentReplacement)$' ./livecanary
FAIL: exact conditional cleanup was still quarantined; replacement adapter was never called

GOTOOLCHAIN=go1.26.8 go test -race -count=1 -run 'Test(CleanupConditionalFence|PinnedSDKDoesNotAdvertiseConditionalCleanup)' ./livecanary
ok: exact version/ETag match, concurrent replacement, missing freshness, owner mismatch and SDK capability boundary

GOTOOLCHAIN=go1.26.8 go test -count=1 -run '^TestCleanupMissingConditionalDeleterStopsBeforeJournalAuthorization$' ./livecanary
ok: missing capability returned quarantine before journal authorization, with no journal event or unconditional delete
```

## PR #81 cancellation-during-prepare follow-up

The exact-head finding `discussion_r4022781337` identified a gap after
`PrepareScaleSetDeletion` returned: the cleanup callback passed its context
directly to `DeleteScaleSetIfOwned`, even when preparation had canceled or
expired that context. The offline regression cancels the parent context from
inside a synthetic prepare adapter that otherwise returns a valid fence. The
driver now checks `c.Err()` between preparation and conditional deletion; a
canceled context returns through `effect`, which records the durable unknown
delete intent and quarantines the journal before the adapter's delete method
can run.

The required TDD red/green evidence was:

```text
GOTOOLCHAIN=go1.26.8 go test -count=1 -run '^TestCleanupCancellationDuringPrepareDoesNotDeleteAndRetainsUncertainty$' ./livecanary
FAIL: cleanup_fence_test.go:195: canceled prepare cleanup = <nil>, want quarantine

GOTOOLCHAIN=go1.26.8 go test -count=1 -run '^Test(CleanupConditionalFence|CleanupCancellationDuringPrepare|PinnedSDKDoesNotAdvertiseConditionalCleanup)' ./livecanary
ok: github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/livecanary 0.365s

GOTOOLCHAIN=go1.26.8 go test -race -count=1 -run '^Test(CleanupConditionalFence|CleanupCancellationDuringPrepare|PinnedSDKDoesNotAdvertiseConditionalCleanup)' ./livecanary
ok: github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/livecanary 1.323s
```

The regression also verifies that `DeleteScaleSetIfOwned` and the legacy
unconditional delete are both called zero times, replay retains durable
uncertainty, and a subsequent cleanup attempt is rejected without retrying
preparation. Rollback is a normal revert of the follow-up commit (for example,
`git revert <follow-up-commit-sha>` after verifying the exact target head); do
not reset or replay cleanup, and do not alter live runners, credentials,
provider context, or other external state.

## PR #81 surviving-set-after-delete follow-up

The exact-head finding `discussion_r4022877174` identified a contradiction in
the one allowed recovery inspect: after a successful conditional delete result,
the durable state permits that inspect slot, but a surviving owned scale set
could be recorded as an ordinary successful inspection. The offline regression
performs a successful fenced cleanup while the synthetic API intentionally keeps
the owned zero-stat set, then verifies that inspect returns `ErrQuarantine`,
retains the `observe-owned` read receipt, and does not append a safe inspect
observation. The driver now checks `s.deleted && set != nil` immediately after
the owned read and quarantines this contradiction without discarding the read
evidence.

The required TDD red/green evidence was:

```text
GOTOOLCHAIN=go1.26.8 go test -count=1 -run '^TestInspectQuarantinesSurvivingSetAfterDurableDelete$' .
FAIL: safety_gate_followup_test.go:105: inspect accepted a surviving set after durable delete: <nil>

GOTOOLCHAIN=go1.26.8 go test -count=1 -run '^TestInspectQuarantinesSurvivingSetAfterDurableDelete$' .
ok  github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/livecanary  0.350s
```

Focused normal and race verification after the fix was:

```text
GOTOOLCHAIN=go1.26.8 go test -count=1 -run '^Test(Inspect|CleanupConditionalFence|PinnedSDKDoesNotAdvertiseConditionalCleanup)' .
ok  github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/livecanary  0.126s

GOTOOLCHAIN=go1.26.8 go test -race -count=1 -run '^Test(Inspect|CleanupConditionalFence|PinnedSDKDoesNotAdvertiseConditionalCleanup)' .
ok  github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/livecanary  1.320s
```

Rollback is a normal revert of the follow-up commit (for example, `git revert
<follow-up-commit-sha>` after verifying the exact target head); do not reset or
replay cleanup, and do not alter live runners, credentials, provider context,
or other external state. No live operation, GitHub access, credential mint,
runner operation, Docker, Lima, Keychain or launchd operation was performed.
