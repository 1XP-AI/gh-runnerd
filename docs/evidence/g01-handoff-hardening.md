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
- The independent contract review of prior candidate
  `19e31e46c1a8a092a16ab8619b4facd88a097d8a` found that a valid `unknown/create`
  event appended during Inventory or Discovery could arrive after the
  post-phase gate and still reach Create. The new
  `TestControllerHandoffUnknownHistoryDuringReadsCannotReachCreate` reproduced
  both paths before correction: each subtest failed with one fake Create call
  and a nil error. The correction checks the exact phase/inventory/discovery
  event grammar after those reads and again after the durable create intent,
  immediately before the API call.

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

The commands above passed after the create-boundary correction. The final
changed-boundary race check also passed:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 GOWORK=off go test -race -count=1 -timeout=120s ./livecanary
```

No full repository matrix was run. No workflow was dispatched or replayed.
Public CI on the base SHA is not candidate evidence.

## Review and remaining gates

The independent GPT-6-Luna max contract review of the prior candidate found the
P1 described above. Its correction has a failing-then-passing synthetic test;
fresh exact-head contract and security/recovery delta reviews of the corrected
candidate remain pending. No finding is claimed resolved by writer tests alone.

The independent GPT-6-Luna max security/recovery review of the prior candidate
also reported one P2: because the durable nonce is intentionally committed
before credential input, a credential-reader failure leaves the create slot
unusable in that journal; a later nonce would have sequence 2 and cannot form
the exact sequence-1 create proof. This is confirmed fail-closed behavior: no
create is authorized, no nonce rollback occurs, and there is no same-journal
retry. It is triaged once as a nonblocking limitation of this offline,
unconnected helper, consistent with fsync-before-credentials and replay
rejection. Any future controller/CLI wiring must define and review a controlled
re-provisioning/recovery procedure before using this path; none is implemented
or authorized here. The reviewer also noted that the legacy no-proof
`Driver.Run` create path remains available; that is an explicit boundary of the
accepted offline slice, not production signed-handoff enforcement.

The candidate must remain offline-only and unpushed until the maintainer
authorizes the next publication step. If later authorized, obtain the required
independent reviews on the final exact head, hosted PR quick check, and exact-head
GitHub Codex review; record all finding triage. Only a reviewed, authorized merge
can produce the separate automatic main Public CI evidence.

The previous HOLD draft `2b40bd6` remains preserved on its original branch.
No credential access, App/token enrollment, live runner/Scale Set operation,
workflow dispatch, service/Keychain/Docker/Lima mutation, privileged action, or
destructive cleanup occurred in this correction.
