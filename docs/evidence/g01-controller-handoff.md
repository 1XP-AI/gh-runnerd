# G01 Controller Handoff Phase B Evidence

Date: 2026-09-22

## Scope

This offline implementation extracts the existing broker provenance v1 request
and receipt types, signing payload, trust-root setup, and pinned-root verification
into the stdlib-only `experiments/g02-auth/handoff` package. The strict receipt
decoder (`DecodeStrictReceipt`) is new in this slice; the immutable main/base
source had no receipt decoder under `experiments/g02-auth`. G01 adds an
unexported, fixture-driven consumption helper that verifies a separately
supplied receipt, durably records its credential-free consumption event in the
existing owned journal, and only then calls a separate credential-reader
callback.

The helper is not wired to a CLI, live controller, transport, or production
receipt source. It establishes no production provenance authority. G01's
controller quarantine and G02's empty-root/no-adapter defaults remain in place.
The G01 module's `go` directive is aligned to the already-selected Go 1.26.8
metadata to permit the approved local G02 module edge; the SDK selection remains
v0.4.0, with no new third-party dependency.

## TDD And Verification

The contemporaneous preimplementation behavior red established in this packet
is for journal-event acceptance, not receipt parsing:

```text
cd experiments/g01-scaleset
GOWORK=off go test ./livecanary -run '^TestControllerHandoffConsumptionEventHasStrictTypedJournalShape$' -count=1
```

It failed because the credential-free typed consumption event was rejected by
journal validation. After adding the typed event validation and persistence
path, the same test passed. No contemporaneous parser-specific failing red
result is available or established in this evidence packet, so the new strict
receipt decoder is not evidenced as fully test-first. The postimplementation
focused handoff suite includes positive and negative parser/trust checks before
credential reading, as well as the exact shared v1 approval digest and signing
bytes, durable fsync ordering, global nonce replay after reopen and across
phases, fresh nonce use in the same phase, concurrent consumers,
cancellation/expiry, and journal ownership/write failures. These later checks
are evidence about the implementation, not retroactive parser-red evidence.

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

The recorded writer selector above uses
`TestBrokerWorkflowReceiptRejectsMissingPinnedTrustRootBeforeCredentialInput`.
That selector did not match or run the actual test,
`TestBrokerWorkflowReceiptRejectsMissingPinnedRootBeforeCredentialInput`; the
writer command is preserved as actually run. The independent security reviewer
ran the exact actual selector separately:

```text
cd experiments/g02-auth
GOWORK=off go test . -run '^TestBrokerWorkflowReceiptRejectsMissingPinnedRootBeforeCredentialInput$' -count=1
```

It passed (`ok`, 0.617s). This independent result is not attributed to the
writer's recorded command.

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

## Independent Review And Probe Evidence

The review lineage is writer source `2e43ff4f1a44645698065a9b54e4d0e94831f008`,
integrated review candidate `66d3cc75141b1170f30e6d17d9af5ab18bb11a64`, and
main/base `76848ac186d6aa584a7c217511846399c73375cd`. These identify the reviewed
source lineage; they do not claim a merge. The current correction changes this
evidence document only.

The independent contract review tested a copied candidate tree with pinned Go
1.26.8: four targeted G01 journal/approval tests passed (`livecanary`, 0.752s),
and the deterministic v1 signing/trust and strict-decoder tests passed
(`handoff`, 0.289s). The decoder tests and additional probes were postimplementation
checks, not a parser-specific preimplementation red. The supplemental signature
probe rejected invalid encoding (`!`) and a validly encoded 63-byte signature.
The existing all-`A` mutation is a validly encoded 64-byte value and tests
cryptographic rejection, not malformed encoding or length coverage.

The independent security/recovery review used a synthetic overlay to reproduce
that a blocked receipt reader does not return on context cancellation while
holding the journal lifecycle lease; journal authorization could not reacquire
the lease until the reader was released. The canceled call returned without
invoking the credential-reader callback. This is a nonblocking P2 for the
current unexported, quarantined offline helper, but cancellation-aware receipt
acquisition and lease handling are required before any OS-backed or live-source
use. No actual credentials or live source were involved.

The contract review also found a future P2 workflow compile-selection gap:
changing `experiments/g02-auth/go.mod` selects G02 but not dependent G01, unlike
a change under `experiments/g02-auth/handoff/`. This does not affect the actual
handoff source diff reviewed here. No candidate PR quick check, exact-head
Codex review, candidate Public CI, or merge is established by these reviews.

## Candidate Finding Ledger

All findings below are classified against immutable integrated review candidate
`66d3cc75141b1170f30e6d17d9af5ab18bb11a64`.

- **P1 — Evidence/provenance correction:** the prior scope described the strict
  decoder as existing, and the journal-event red could be misread as parser-red
  evidence. This document now distinguishes existing v1 material from the new
  decoder and explicitly records that no parser-specific red is established.
  This is a documentation correction, not code or merge approval.
- **P2 — Receipt-reader cancellation and lease:** byte-bounded receipt input is
  not cancellation-aware while the journal lifecycle lease is held, as
  reproduced above without a credential callback. Required before OS-backed or
  live-source use; not permission to connect this helper to such a source.
- **P2 — G02 module compile selection:** a change to the G02 root `go.mod` does
  not select dependent G01 compilation. This is a future-change coverage gap,
  not a failure for this source diff.
- **P2 — Signature-shape permanent coverage:** malformed encoding and 63-byte
  signatures were independently probed and correctly rejected, but are not
  permanent committed negative cases. The all-`A` 64-byte cryptographic-failure
  case does not cover those shapes; no verifier defect was reproduced.

No public follow-up issue URLs are recorded in this packet; the coordinator will
record appropriate follow-ups in the PR conversation. G01 issue #1 and its full
parent/live evidence gates remain open. No production implementation or live
operation is authorized by this offline slice; OS/native inputs remain
unapproved. No actual credential, API, runner, Scale Set, App, Keychain,
launchd, Docker, Lima, privileged, or workflow operation was performed.

## Remaining Evidence Gaps

No live GitHub workflow input, Scale Set operation, credential issuance, real
runner, or macOS test was run. No production receipt transport/source or G01
CLI integration is established by this slice. The full repository suites and
the paired bridge executable fixture were not run; compile-only checks confirm
the local module closure without executing those tests. These gaps remain
explicit and require separate authorization and evidence.
