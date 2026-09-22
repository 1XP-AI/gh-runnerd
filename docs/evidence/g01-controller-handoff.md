# G01 Controller Handoff Phase B Evidence

Date: 2026-09-22

## Scope

This offline implementation extracts the existing broker provenance v1 request,
receipt, signing payload, pinned trust root, and strict receipt decoder into the
stdlib-only `experiments/g02-auth/handoff` package. G01 adds an unexported,
fixture-driven consumption helper that verifies a separately supplied receipt,
durably records its credential-free consumption event in the existing owned
journal, and only then calls a separate credential-reader callback.

The helper is not wired to a CLI, live controller, transport, or production
receipt source. It establishes no production provenance authority. G01's
controller quarantine and G02's empty-root/no-adapter defaults remain in place.
The G01 module's `go` directive is aligned to the already-selected Go 1.26.8
metadata to permit the approved local G02 module edge; the SDK selection remains
v0.4.0, with no new third-party dependency.

## TDD And Verification

The first behavior test was run before implementation:

```text
cd experiments/g01-scaleset
GOWORK=off go test ./livecanary -run '^TestControllerHandoffConsumptionEventHasStrictTypedJournalShape$' -count=1
```

It failed because the credential-free typed consumption event was rejected by
journal validation. After adding the typed event validation and persistence
path, the same test passed. The focused handoff suite also passes, covering the
exact shared v1 approval digest and signing bytes, strict parser and trust
failures before credential reading, durable fsync ordering, global nonce replay
after reopen and across phases, fresh nonce use in the same phase, concurrent
consumers, cancellation/expiry, and journal ownership/write failures.

Final focused and compatibility commands run in `experiments/g01-scaleset`:

```text
GOWORK=off go test ./livecanary -run '^TestControllerHandoff' -count=1
GOWORK=off go test -race ./livecanary -run '^TestControllerHandoff' -count=1
GOWORK=off go test -tags g01_live ./cmd/g01-live -run '^Test(PlanAndRefusalsNeverReadCredentialsOrEchoInputs|PairedTerminalModeQuarantinesBeforeControllerInput|PairedTerminalModeRejectsUnusedPhaseAndControllerFlagsBeforeInput)$' -count=1
GOWORK=off go test -tags g01_pair_fixture ./livecanary -run '^$' -count=1
GOWORK=off go test -run '^$' ./...
```

Commands run in `experiments/g02-auth`:

```text
GOWORK=off go test ./handoff -count=1
GOWORK=off go test . -run '^Test(BrokerProvenanceAdapterPreservesSharedDeterministicV1Fixture|BrokerWorkflowReceiptRejectsMissingPinnedTrustRootBeforeCredentialInput|BrokerWorkflowReceiptRejectsAdapterSelectedRootBeforeCredentialInput|BrokerControllerReceiptRequiresExplicitSourceBeforeInput|BrokerControllerFrontDoorRequiresBrokerProvenance|BrokerControllerFrontDoorRejectsDirectFIFOBeforeRead|BrokerControllerClaimsAttemptBeforeProvenanceSideEffects)$' -count=1
GOWORK=off go test -run '^$' ./...
```

Command run from the repository root:

```text
GOWORK=off go test ./scripts -run '^(TestPullRequestQuickWorkflowContract|TestG01SDKComparisonDropsUnusedLocalHandoffModuleEdge)$' -count=1
```

The focused G01/G02 suites, G01 live-entrypoint refusal/quarantine checks,
paired-fixture compile-only check, module compile-only checks, and workflow
contract tests passed. `compare-sdk.sh` removes the unused local G02 edge from
its isolated scratch `go.mod` before `go mod tidy`; the regression test verifies
that ordering and the retained SDK version selection. The SDK comparison matrix
itself was not run.

## Remaining Evidence Gaps

No live GitHub workflow input, Scale Set operation, credential issuance, real
runner, or macOS test was run. No production receipt transport/source or G01
CLI integration is established by this slice. The full repository suites and
the paired bridge executable fixture were not run; compile-only checks confirm
the local module closure without executing those tests. These gaps remain
explicit and require separate authorization and evidence.
