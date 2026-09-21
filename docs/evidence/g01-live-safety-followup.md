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

## PR #81 exact-head Codex P2 follow-up

The source change boundary started at the requested exact PR #81 head
`e124872d13d8fd8d3cf5032b0b13ff20fd8bb55f`. The implementation and regression
tests are in commit `026833ac82908b0a453a0e96683a3a094039f1b6`; this evidence
update is a separate documentation boundary so the source SHA remains exact.

| Finding | RED reproduction at the exact starting head | Correction and triage |
|---|---|---|
| Codex P2 `r4022996388` ([discussion](https://github.com/1XP-AI/gh-runnerd/pull/81#discussion_r4022996388)) | `GOTOOLCHAIN=go1.26.8 go test -count=1 -run '^TestBrokerControllerClaimsAttemptBeforeProvenanceSideEffects$' .` failed: the provenance adapter observed `broker.jsonl` absent/unlocked (`attest=false`, `verify=false`) because `brokerAttemptUnused` was only an absence check and `openBrokerJournal` happened later. | `runBrokerWithAPI` now requires a canonical absolute attempt path, opens and locks the durable journal before provenance, checks that claim around both adapter calls and before/after credential input, and passes the held journal into execution. `openBrokerJournal` rejects noncanonical paths and documents the claim-before-input boundary. The regression uses only a signed offline fixture adapter and a synthetic private input. |
| Codex P2 `r4022996398` ([discussion](https://github.com/1XP-AI/gh-runnerd/pull/81#discussion_r4022996398)) | `GOTOOLCHAIN=go1.26.8 go test -count=1 -run '^TestCleanupPreparationRereadsFinalOwnershipAndZeroStatistics$' ./livecanary` failed: mutating the final ownership label or idle statistics immediately before preparation still returned nil and crossed to conditional deletion. | `ScaleSetDeletionExpectation` and `ScaleSetDeletionFence` now carry the final ownership label and exact all-zero statistics. The synthetic conditional adapter rereads the set in `PrepareScaleSetDeletion` and verifies those predicates before returning a fence; the driver requires the fence to bind both values, and the adapter's conditional delete checks them again. The pinned SDK remains without this capability and therefore remains quarantined. |

The RED commands above were run before commit `026833a`; their failures are the
prior unsafe behaviors, not skipped tests. After the implementation, focused
normal and race checks passed:

```text
GOTOOLCHAIN=go1.26.8 go test -count=1 -run '^TestBroker(ControllerClaimsAttemptBeforeProvenanceSideEffects|ControllerFrontDoorRequiresBrokerProvenance|ControllerFrontDoorRejectsDirectFIFOBeforeRead|WorkflowReceiptBounds(Attest|Verify)ToApprovalDeadline)$' .
ok   github.com/1XP-AI/gh-runnerd/experiments/g02-auth  1.371s

GOTOOLCHAIN=go1.26.8 go test -race -count=1 -run '^TestBroker(ControllerClaimsAttemptBeforeProvenanceSideEffects|ControllerFrontDoorRequiresBrokerProvenance|ControllerFrontDoorRejectsDirectFIFOBeforeRead|WorkflowReceiptBounds(Attest|Verify)ToApprovalDeadline)$' .
ok   github.com/1XP-AI/gh-runnerd/experiments/g02-auth  5.538s

GOTOOLCHAIN=go1.26.8 go test -count=1 -run '^Test(CleanupPreparationRereadsFinalOwnershipAndZeroStatistics|CleanupConditionalFence|CleanupCancellationDuringPrepare|InspectQuarantinesSurvivingSetAfterDurableDelete|PinnedSDKDoesNotAdvertiseConditionalCleanup)$' ./livecanary
ok   github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/livecanary  0.301s

GOTOOLCHAIN=go1.26.8 go test -race -count=1 -run '^Test(CleanupPreparationRereadsFinalOwnershipAndZeroStatistics|CleanupConditionalFence|CleanupCancellationDuringPrepare|InspectQuarantinesSurvivingSetAfterDurableDelete|PinnedSDKDoesNotAdvertiseConditionalCleanup)$' ./livecanary
ok   github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/livecanary  1.320s
```

The complete offline normal package checks also passed at this change boundary:
`GOTOOLCHAIN=go1.26.8 go test -count=1 ./...` in `experiments/g02-auth`
(`46.735s`, including `cmd/g01-broker` and `cmd/g02-enroll`) and in
`experiments/g01-scaleset` (`0.300s` root, `34.654s` livecanary, `8.269s`
liveworker). Targeted formatting, vet, diff, and secret/private-path checks
were run locally with these results:

```text
gofmt -l experiments/g01-scaleset/livecanary/cleanup_fence.go experiments/g01-scaleset/livecanary/cleanup_fence_test.go experiments/g01-scaleset/livecanary/driver.go experiments/g02-auth/broker.go experiments/g02-auth/broker_entry.go experiments/g02-auth/broker_journal.go experiments/g02-auth/broker_provenance_test.go
(empty output; exit 0)
GOTOOLCHAIN=go1.26.8 go vet ./...   # experiments/g02-auth
(empty output; exit 0)
GOTOOLCHAIN=go1.26.8 go vet ./...   # experiments/g01-scaleset
(empty output; exit 0)
git diff --check
(empty output; exit 0)
secret/private-path scan over the changed Go/evidence files
secret/private-path scan passed: no raw secret or personal path patterns
```

None of these checks exercised GitHub, Actions, Scale Set, runner, Docker,
Lima, Keychain, launchd or self-hosted-runner operations.

Remaining live gaps are unchanged and explicit: the provenance adapter is only
an offline signed fixture and has no broker-authenticated workflow-input source;
tagged controller and paired entrypoints remain quarantined. The pinned
`github.com/actions/scaleset v0.4.0` adapter still exposes only unconditional
`DeleteScaleSet`, so no live conditional cleanup authorization or live
verification is claimed.

## G01 trust-root contract slice — 2026-09-21

The first red test for the next blocker was added before implementation:

```text
GOWORK=off GOTOOLCHAIN=go1.26.8 go test . -run '^TestBrokerProvenanceRequiresExplicitPinnedTrustRoot$' -count=1
FAIL: broker_provenance_test.go:287: undefined: NewBrokerProvenanceTrustRoot
```

The minimal offline contract now provides `BrokerProvenanceTrustRoot`, which
copies one explicitly supplied Ed25519 public key and key ID and verifies the
receipt against the controller-derived request. The request therefore binds
the controller-approval digest, repository/ref/workflow/run tuple, selected
phase, owner nonce, source and receipt expiry; the receipt cannot select its
own verification key. The broker requires a valid root before invoking the
provenance adapter and before reading credential input. A rootless adapter is
rejected before a blocking FIFO read, token mint or API call.

Focused normal and race checks passed:

```text
GOWORK=off GOTOOLCHAIN=go1.26.8 go test . -run '^TestBroker(Provenance|WorkflowReceipt|Controller(FrontDoor|ClaimsAttempt))' -count=1
PASS
GOWORK=off GOTOOLCHAIN=go1.26.8 go test -race . -run '^TestBroker(Provenance|WorkflowReceipt|Controller(FrontDoor|ClaimsAttempt))' -count=1
PASS
```

This does not close the live gate. The repository has no production
broker-authenticated workflow-input source, no reviewed key provisioning or
rotation authority, and no authenticated controller handoff that delivers and
verifies this receipt before credential parsing. The concrete next contract is
an operator-approved broker authority that pins the root key ID/fingerprint,
attests the verified private repository/ref/workflow/run and selected phase,
and hands the signed receipt to a controller that has the same pinned root;
until that contract and source exist, tagged controller and paired execution
remain quarantined.

The cleanup blocker is likewise unchanged: pinned Scale Set SDK `v0.4.0`
exposes only unconditional `DeleteRunnerScaleSet(ctx, id)` and no version/ETag
read or conditional delete operation. The existing offline
`ConditionalScaleSetDeleter` remains the safe boundary. Its concrete next
contract is a reviewed provider adapter whose final owned read returns a server
revision and whose delete submits that revision plus the durable owner identity
as one server-side conditional operation; no observe-then-delete fallback is
authorized. No live API, credential, runner, Scale Set, workflow, Docker,
Lima, Keychain or launchd operation was performed.

## Codex P1 trust-root triage and fix — 2026-09-22

Codex review of the first PR head reproduced a P1: the adapter supplied
`TrustRoot()`, so an adapter could sign with a replacement key and nominate
that same key as the verification root. The first red test for the fix was
added before implementation:

```text
GOWORK=off GOTOOLCHAIN=go1.26.8 go test . -run '^TestBrokerWorkflowReceiptRejectsAdapterSelectedRootBeforeCredentialInput$' -count=1
./broker_provenance_test.go:395:8: e.api.provenanceRoot undefined (type *brokerAPI has no field or method provenanceRoot)
FAIL: build failed
```

The minimal fix moves the pinned root into broker-owned `brokerAPI` configuration
through a separate constructor, removes `TrustRoot()` from the adapter contract,
and verifies receipts only against that independently configured root. The
attacker-signed fixture receipt is now rejected before the credential FIFO read,
token mint or API call. The production constructor still has an empty root and
therefore preserves controller quarantine until a reviewed provisioning and
handoff protocol exists.

The focused regression passed after the fix:

```text
GOWORK=off GOTOOLCHAIN=go1.26.8 go test . -run '^TestBrokerWorkflowReceiptRejectsAdapterSelectedRootBeforeCredentialInput$' -count=1
ok   github.com/1XP-AI/gh-runnerd/experiments/g02-auth  0.465s
```
