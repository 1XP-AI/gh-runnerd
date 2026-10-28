# G01 observed provenance: bounded offline slice

Date: 2026-10-28. Issue: #1. Base: `ab4adaa366a57e3b89339c00af7c2681ec40643b`.
Writer: explicitly dispatched Codex `gpt-6.1-sol/high`. This is experimental
implementation evidence; independent review and live qualification remain open.

The private observer performs only the exact approved workflow-run GET. It uses
shared fixed-origin, direct TLS, no-proxy/no-redirect transport policy and rejects
upstream read/close errors, compressed/decompressed or oversized responses,
ambiguous JSON, cancellation and mismatched private nonfork numeric identities.
The existing verification-token corroboration shares its exact run matcher.

The adapter copies one independently configured run/phase grant, checks the held
attempt and deadline around token/HTTP/signing boundaries, then signs canonical
shared v1 bytes with a fixed source/key ID. GitHub observation establishes run
facts; the reviewed grant authorizes the broker phase. No GitHub workflow-input
phase claim is made. Default nonces use 128 random bits and differ from the owner;
receipt expiry equals grant expiry. The broker rejects insufficient receipt
expiry without extending the grant, and independently verifies the pinned root.
The injection-only factory receives the actual held claim, uses a local adapter,
and rejects rootless/conflicting configuration before factory callbacks.

## Behavioral TDD and verification

Actual red failures, followed by green fixes:

- `TestWorkflowTransportRejectsUpstreamCloseError`: accepted upstream close failure.
- `TestWorkflowProvenanceSignsObservedExactGrant`: valid observed grant yielded no
  independently verifiable receipt against the initial adapter stub.
- `TestWorkflowObserverRejectsAlreadyDecompressedResponse`: accepted an already
  decompressed response.
- `TestWorkflowProvenanceRejectsCancellationDuringAttemptCheck`: cancellation
  during the attempt guard still invoked the observation credential reader.

All checks use exact Go 1.27.3, the existing build cache, and
`GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOEXPERIMENT=`. In `experiments/g02-auth`,
`$G01_GO` below denotes the directly selected exact toolchain binary:

```sh
$G01_GO test -count=1 -timeout=60s -run '^Test(Workflow|BrokerWorkflow|BrokerProvenance|BrokerController(FrontDoor|Claims|Receipt)|BrokerRejectsRedirects|BrokerBoundsEveryResponse|BrokerDuplicateAPIJSON|BrokerFolded|BrokerOrganizationOnlyMint|BrokerManagementScopeRefuses)' .
$G01_GO test -race -count=1 -timeout=90s -run '^Test(Workflow|BrokerWorkflow|BrokerProvenance|BrokerController(FrontDoor|Claims|Receipt)|BrokerRejectsRedirects|BrokerBoundsEveryResponse|BrokerDuplicateAPIJSON|BrokerFolded|BrokerOrganizationOnlyMint|BrokerManagementScopeRefuses)' .
$G01_GO test -count=1 -timeout=30s ./handoff
$G01_GO vet . ./handoff
$G01_GO test -count=1 -timeout=30s -run '^TestWorkflowProvenance(BoundsCallbacksToGrantDeadline|BrokerFactoryEnforcesCredentialBoundary|RejectsCancellationDuringAttemptCheck)$' .
$G01_GO test -race -count=1 -timeout=60s -run '^TestWorkflowProvenance(BoundsCallbacksToGrantDeadline|BrokerFactoryEnforcesCredentialBoundary|RejectsCancellationDuringAttemptCheck)$' .
$G01_GO test -race -count=1 -timeout=30s -run '^TestBrokerAdmissionRejectsDuplicateProvenanceNonceBeforeAppend$' .
```

Passed: focused normal/race regressions, actual broker-factory no-management-input
cases, callback deadline delta checks, shared receipt contract, admission nonce
regression, vet, gofmt, diff hygiene and changed-file private-pattern scan. Only
synthetic keys/tokens and RoundTripper fixtures were used for these tests.

## Limits and remaining gates

The tagged G01 CLI quarantine regression was attempted offline but could not start:
its existing SDK/UUID dependencies were absent from the default module cache and
module lookup was disabled. No passing result is claimed for that invocation.
The quarantine sources, shared signed bytes and conditional-cleanup fence are
unchanged. Full suites were intentionally not rerun.

A toolchain version probe accidentally triggered automatic Go 1.27.3 retrieval,
contrary to the worker's no-network constraint; this was reported immediately.
Subsequent verification invoked the installed binary directly with downloads off.
No application network, real credential, packet/run body, runtime or GitHub write
was used. No production root/config/CLI, controller receipt bridge or worker
metadata command was added. Normal constructors retain an empty provenance root.
Signing-authority/grant ownership, credential provisioning, handoff/root deployment,
exact-head independent and GitHub Codex reviews, PR quick checks, explicit canary
authorization, SDK atomic conditional cleanup and G01/G02/G04+ gates remain open.
