# G01 live safety follow-up

This follow-up starts from the exact PR #72 head `f5560ba950f77343e57034cc1cf85dc67f5ac922`. It addresses the five independent-review NO-GO findings recorded on G01 issue #1 and PR #72. It is an offline safety change only; it does not authorize GitHub access, workflow dispatch, runner creation, scale-set creation, credential minting, or live evidence.

## Safety decisions

| Finding | Decision |
|---|---|
| Repeated `inspect` despite one slot | `inspect` now consumes a durable `phase` record before its read path. Replay rejects a second inspect as quarantined, including after a failed or partially observed attempt. |
| Direct FIFO/controller input without broker provenance | Tagged controller and paired-terminal execution are quarantined before reading stdin. The existing stdin boundary cannot independently attest that its caller is the broker. No synthetic environment, flag, token field, or process convention was added as an attestation. |
| Tagged controller entry outside the broker-only path | The tagged execution flags remain disabled at the command boundary. Preparation and plan-only paths remain local and non-executing. |
| Workflow phase not bound to controller phase | The change adds only a structural singleton approval-shape guard: a non-paired controller approval must contain one declared phase matching the outer approval. It does not verify that workflow input selected that phase; workflow input-to-controller-phase binding remains an explicit unresolved gap, so tagged controller execution stays quarantined. Paired-terminal keeps its separate fixed sequence but is also blocked at the tagged controller entry. |
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
