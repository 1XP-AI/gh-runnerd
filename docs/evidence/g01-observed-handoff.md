# G01 observed receipt to single create: offline evidence

Issue #1, based on `eba9312ac309dacccd1450b8fc431c605fb8c0fa`.
Implementation: Codex `gpt-6.1-sol`, high reasoning, explicit user override.

The private `runControllerHandoffCreate` composition owns the raw approval and
bounded receipt snapshots, consumes a shared v1 receipt using independently
supplied source/root and the existing journal, then strictly decodes and validates
credential bytes before invoking a required network-lazy API factory. It clears
the raw bytes and passes the original per-open proof directly to
`Driver.runWithHandoff` for one create. It returns only a fixed error or success.

The `g01_pair_fixture`-only G02 producer invokes the actual workflow observer and
grant adapter with a fixed in-memory HTTP 200 response, caller-provided synthetic
key and independent grant, candidate and response facts. It selects no controller
root, reads no external credential, provisions no file/key and launches no
process. The adapter's credential callback returns a fixed synthetic token; its
attempt guard is an in-memory fixture, not a broker process claim. Phase and full
ref remain operator grant policy, not authenticated GitHub workflow inputs.

One existing-source change is necessary: create consumption now requires empty
journal history under the existing nonce-append mutex. The generic consumer
previously allowed a fresh nonce to read credentials after a failed create,
although create's proof requires sequence one. Non-create consumption is unchanged.

## Behavioral evidence

- Red: a compiling provisional composition that discarded the proof refused the
  valid create (`reads=1`, `factories=1`, `creates=0`); missing semantic credential
  validation let invalid identity/scope/token/expiry/permissions reach the factory.
- Red: a fresh nonce after malformed credentials produced `reads=2`, `events=2`.
- Green: actual adapter receipt -> strict consumer -> stored nonce and exact raw
  approval/receipt hashes -> validated credentials -> original proof -> one fake
  create. Caller buffer mutation cannot change the owned approval/receipt.
- Negative checks cover independent candidate/grant and observed run mismatches;
  raw approval versus canonical journal authority; phase/source/root/signature;
  ambiguous/unknown JSON; credential shape, identity, scope, permissions and expiry;
  cancellation; journal identity/close/reopen; replay, fresh nonce and concurrent
  entries. Inappropriate credential-reader/factory/create calls stay zero.
- Malformed input and cancellation retain consumption across journal reopen.
  Credential bytes are cleared on success and failure; decoded Go strings may
  retain copies. No proof or credential is returned by the composition.

## Focused verification

All commands used the pre-existing Go 1.27.3 toolchain and cache with
`GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOEXPERIMENT= GOFLAGS=-mod=readonly`.
The commands below are relative to the indicated experiment module; all passed.

| Module | Command |
| --- | --- |
| G01 | `go test ./livecanary -run '^TestControllerHandoff' -count=1` |
| G01 | `go test -tags g01_pair_fixture ./livecanary -run '^TestControllerHandoff(Entry\|Observed)' -count=1` |
| G01 | `go test -race ./livecanary -run '^TestControllerHandoff' -count=1` |
| G01 | `go test -race -tags g01_pair_fixture ./livecanary -run '^TestControllerHandoff(Entry\|Observed)' -count=1` |
| G02 | `go test -tags g01_pair_fixture . -run '^TestWorkflow(Provenance\|RunObserver)' -count=1` (provenance tests; observer names covered by the next command) |
| G02 | `go test -race -tags g01_pair_fixture . -run '^TestWorkflow(Provenance\|Observer\|Transport)' -count=1` |
| G02 | `go test . -run '^TestBrokerBuild(MustMatchReviewedController\|RejectsFixtureCapability)$' -count=1` |
| G01 | `go test -tags 'g01_live,g01_pair_fixture' ./cmd/g01-live -run '^Test(TaggedControllerExecutionRequiresBrokerProvenanceBeforeInput\|PlanAndRefusalsNeverReadCredentialsOrEchoInputs\|PairedTerminalModeQuarantinesBeforeControllerInput)$' -count=1` |

`go list -json` confirmed the producer file is ignored for untagged and
`g01_live` builds and selected only with `g01_pair_fixture`. The existing broker
build gate requires exactly `g01_live`. Gofmt, diff hygiene and changed-file
private-pattern checks passed. No dependency or schema change was needed.

## Remaining gates

This is synthetic in-process integration evidence only. The normal CLI and
`RunBroker` do not activate this helper; quarantine and the empty default root
remain in place. No GitHub, Docker, SSH, credential, process-launch or live test
was performed. Full suites, independent review, exact-head GitHub Codex review
and hosted PR checks were not performed in this local implementation task.

Live use still needs reviewed signing/grant ownership, independent controller
root provisioning/rotation, separate private-repository observation authority,
receipt-before-credential transport and issuance/handoff integrity, reviewed
executable/build configuration and explicit live authorization. Conditional
cleanup remains a separate provider capability gap. G01/G02 remain incomplete
and dependent production G04+ work remains gated.
