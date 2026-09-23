# G01 Handoff Hardening — Offline Evidence

Issue: [#94](https://github.com/1XP-AI/gh-runnerd/issues/94)

Base: `7e4d46e8762f327c03f1f80d726e040b9ba7f0a6`

Scope: offline receipt acquisition, approval-bound create-prefix validation,
and dependent G01 compile selection for G02 root metadata.

This evidence is for a locally committed but unpublished candidate. It does not
establish independent review, a PR, merge, hosted quick check, exact-head GitHub
Codex review, or post-merge Public CI. The correction remains unconnected to a live
controller, CLI, receipt transport, credential store, App, runner, or Scale Set.
The full G01/G02 evidence and live-operation authorization gates remain open.

## Red evidence

The initial main-based correction added failing tests before the corresponding
source changes:

- `TestControllerHandoffCancellationClosesOwnedSourceAndReleasesLease` and
  `TestControllerHandoffPreReadRefusalClosesOwnedPipe` failed because a canceled
  read did not close/join the owned source and an early refusal left the pipe
  producer blocked.
- `TestControllerHandoffRejectsSecondOpenerAndParallelSameHandle` failed its
  added lease assertion because credential input retained the journal lifecycle
  lease.
- `TestG01WorkflowModuleSelectionPredicate` rejected the required G02 root
  `go.mod` and `go.sum` paths before the workflow predicate was extended.

The following focused regression mutations were also run and restored; each
produced its expected failure:

- Disabling original-parent propagation made
  `TestControllerHandoffOriginalParentCancellationFencesCreate` observe one
  fake Create call and a nil error.
- Sampling the nonce append clock before taking `journal.mu` made
  `TestControllerHandoffNonceAppendResamplesAfterJournalLock` fail because the
  authorization time was sampled while the journal was locked and expired
  before append.
- Rejecting the verified handoff at the pre-phase gate made
  `TestControllerHandoffCreatePrefixAllowsVerifiedCreate` fail with quarantine.
- Allowing a third unrelated event in the post-phase prefix made
  `TestControllerHandoffCreatePrefixRechecksAfterPhaseAppend` fail because
  changed history reached the create boundary.

These are synthetic offline tests and controlled source mutations; no remote
operation was attempted. The create-prefix mutations are regression evidence,
not evidence that those tests were originally run on the unchanged main source.

## Implemented contract and checks

- Receipt input is a concrete private source: bounded memory bytes for offline
  fixtures or an owned in-memory `io.Pipe` used by tests. The helper no longer
  accepts an arbitrary `io.Reader`. Cancellation closes both pipe endpoints and
  joins the watcher; receipt reads remain synchronous and capped at 16 KiB + 1.
- Receipt/credential input occurs without holding the journal lifecycle lease.
  The approval snapshot is cloned and bounded; the nonce append resamples
  cancellation, expiry, root, tuple, and current ownership while holding the
  journal mutex. Credentials are read only after the durable fsynced nonce
  event, then authority and event identity are rechecked.
- A nonserialized create proof binds the exact approval/receipt tuple, original
  parent context, effective deadline, successful credential return, durable
  event, and current per-open journal identity. Both create gates use the same
  exact event grammar. Unknown, pending, replayed, mismatched, expired,
  canceled, or reopened proof paths stay quarantined.
- The PR quick-check path predicate selects G01 for G02 handoff code and root
  `go.mod`/`go.sum` changes, with positive and near-miss negative path tests.

Latest checks after the concrete-source and prefix changes:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 GOWORK=off go test -count=1 -timeout=60s -run '^TestControllerHandoff' ./livecanary
GOTOOLCHAIN=go1.26.8 GOWORK=off go test -count=1 -timeout=120s ./...

cd <repository-root>
GOTOOLCHAIN=go1.26.8 GOWORK=off go test -count=1 -timeout=30s -run '^(TestG01WorkflowModuleSelectionPredicate|TestPullRequestQuickWorkflowContract)$' ./scripts
git diff --check
```

The commands above passed. The final changed-boundary race check also passed:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 GOWORK=off go test -race -count=1 -timeout=120s ./livecanary
```

No full repository matrix was run. No workflow was dispatched or replayed.
Public CI on the base SHA is not candidate evidence.

## Review and remaining gates

Independent contract and security/recovery review are pending for the stable
candidate. No findings are claimed resolved by internal tests alone. The
candidate must remain offline-only and unpushed until the maintainer authorizes
the next publication step. If later authorized, batch the stable source/evidence
candidate, obtain the required independent reviews, hosted PR quick check, and
exact-head GitHub Codex review; record all finding triage. Only a reviewed,
authorized merge can produce the separate automatic main Public CI evidence.

The previous HOLD draft `2b40bd6` remains preserved on its original branch.
No credential access, App/token enrollment, live runner/Scale Set operation,
workflow dispatch, service/Keychain/Docker/Lima mutation, privileged action, or
destructive cleanup occurred in this correction.
