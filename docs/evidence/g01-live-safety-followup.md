# G01 live safety follow-up

This follow-up starts from the exact PR #72 head `f5560ba950f77343e57034cc1cf85dc67f5ac922`. It addresses the five independent-review NO-GO findings recorded on G01 issue #1 and PR #72. It is an offline safety change only; it does not authorize GitHub access, workflow dispatch, runner creation, scale-set creation, credential minting, or live evidence.

## Safety decisions

| Finding | Decision |
|---|---|
| Repeated `inspect` despite one slot | `inspect` now consumes a durable `phase` record before its read path. Replay rejects a second inspect as quarantined, including after a failed or partially observed attempt. |
| Direct FIFO/controller input without broker provenance | Tagged controller and paired-terminal execution are quarantined before reading stdin. The existing stdin boundary cannot independently attest that its caller is the broker. No synthetic environment, flag, token field, or process convention was added as an attestation. |
| Tagged controller entry outside the broker-only path | The tagged execution flags remain disabled at the command boundary. Preparation and plan-only paths remain local and non-executing. |
| Workflow phase not bound to controller phase | The change adds only a structural approval-shape guard: a non-paired controller approval carries one stable multi-phase authority membership set, and the selected outer phase must be a member of that set. It does not verify that workflow input selected that phase; workflow input-to-controller-phase binding remains an explicit unresolved gap, so tagged controller execution stays quarantined. Paired-terminal keeps its separate fixed sequence but is also blocked at the tagged controller entry. |
| Cleanup lacks atomic ownership/freshness fence | The current adapter exposes only unconditional `DeleteScaleSet(context, id)`, so controller cleanup is quarantined before journal/API access. Re-enabling requires a reviewed adapter capability that performs ownership and freshness validation atomically with deletion; inventory-before-delete and inventory-after-delete reads are not such a fence. |

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
Missing capability, mismatched identity/nonce/inventory, missing freshness or
an adapter error quarantines the journal and prevents retry. Inventory reads
before and after deletion remain supporting evidence only, not the atomic fence.

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
```
