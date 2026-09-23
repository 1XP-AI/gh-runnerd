# G01 Handoff Hardening — Offline Evidence

Issue: [#94](https://github.com/1XP-AI/gh-runnerd/issues/94)

Base: `7e4d46e8762f327c03f1f80d726e040b9ba7f0a6`

Scope: offline receipt acquisition, approval-bound create-prefix validation,
and dependent G01 compile selection for G02 root metadata.

This record began as evidence for a locally committed candidate. It now tracks
PR [#97](https://github.com/1XP-AI/gh-runnerd/pull/97), opened after the
maintainer authorized publication. Its pre-correction head
`b53820a6a59850ad85943ebc5e5b1bb1547fc3f5` passed the hosted PR quick check and
received exact-head GitHub Codex review; the review findings and current
dispositions are recorded below. The PR remains unmerged, and merge was not
authorized. The correction remains unconnected to a live controller, CLI,
receipt transport, credential store, App, runner, or Scale Set. Full G01/G02
evidence and live-operation authorization gates remain open.

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
- A fresh independent security/recovery review of exact HEAD
  `1c98292c40cb67e1e766228c5826c29edfd05c0f` found a second P1 window: the
  final `Events()` snapshot released `FileJournal.mu` before
  `CreateScaleSet`, allowing a concurrent same-process `Append` to persist
  `unknown/create` between validation and the call. The reviewer identified
  this interleaving by source inspection and had not dynamically reproduced it.
  `TestControllerHandoffCreateRechecksHistoryAtomicallyWithEffect` was added
  before the source correction and failed on that source with one fake Create
  call. Its append runs concurrently after the snapshot but before the
  validation callback returns. Exact red command from
  `experiments/g01-scaleset`:

  ```text
  GOTOOLCHAIN=go1.26.8 GOWORK=off go test -count=1 -timeout=30s -run '^TestControllerHandoffCreateRechecksHistoryAtomicallyWithEffect$' ./livecanary
  ```

  Observed failure: `unknown/create appended after the final history snapshot
  reached 1 create calls`.

These are synthetic offline tests and controlled source mutations; no remote
operation was attempted. The create-prefix mutations are regression evidence,
not evidence that those tests were originally run on the unchanged main source.

## PR #97 exact-head review findings and correction

GitHub Codex reviewed exact head
`b53820a6a59850ad85943ebc5e5b1bb1547fc3f5` and reported:

- **P1, failed journal append could still reach CreateScaleSet**
  ([finding](https://github.com/1XP-AI/gh-runnerd/pull/97#discussion_r4082160685)).
  After the durable create intent, a concurrent append could fail and poison
  `FileJournal`; `withEventsLocked` nevertheless passed the old canonical
  snapshot to the effect callback. A real-`FileJournal` regression now injects
  that write failure before the locked-history callback. Before the source fix
  it failed with `createCalls=1`; after the fix it passes with the journal
  poisoned and `createCalls=0`. `withEventsLocked` now rejects a poisoned
  journal while holding `mu`, before exposing history or invoking the effect.
- **P1, review model attribution/route**
  ([finding](https://github.com/1XP-AI/gh-runnerd/pull/97#discussion_r4082160672)).
  Historical GPT-6-Luna max review attribution is retained as actually
  produced. For this task the maintainer explicitly required GPT-6-Luna max
  for worktree agents; this is the task-local user setting referenced by the
  final “Do not override an explicit current user setting” instruction in
  `AGENTS.md`, while `docs/EXECUTION.md` continues to describe GPT-5.6-Luna max
  as the repository default and canonical route. The attribution is therefore
  not silently relabeled. Fresh review-task launch requests must specify
  `gpt-6-luna`/`max`, and the effective launch model/effort must be verified
  before their findings count as review evidence.
- **P2, hosted workflow omitted its path-predicate regression test**
  ([finding](https://github.com/1XP-AI/gh-runnerd/pull/97#discussion_r4082160691)).
  `TestPullRequestQuickWorkflowContract` now requires
  `TestG01WorkflowModuleSelectionPredicate` in the hosted workflow selector;
  that contract test failed against the previous workflow command. The hosted
  selector now runs the predicate test with the other workflow contract tests.

These are offline code/test changes only. They do not expand production or
live-operation authority.

Correction validation on the current local delta:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 GOWORK=off go test -count=1 -timeout=120s ./...
GOTOOLCHAIN=go1.26.8 GOWORK=off go test -race -count=1 -timeout=120s ./livecanary
GOTOOLCHAIN=go1.26.8 GOWORK=off go test -count=1 -timeout=30s -run '^TestFailedConcurrentAppendStopsBeforeRealJournalCreate$' ./livecanary

cd experiments/g02-auth
GOTOOLCHAIN=go1.26.8 GOWORK=off go test -run '^$' -count=1 -timeout=120s ./...

cd <repository-root>
GOTOOLCHAIN=go1.26.8 GOWORK=off go test -count=1 -timeout=30s -run '^(TestPublicWorkflowCapacityContract|TestPullRequestQuickWorkflowContract|TestG01WorkflowModuleSelectionPredicate)$' ./scripts
git diff --check
```

All commands in this block passed. The targeted journal test was first observed
failing before the guard (`createCalls=1`) and passed afterward (`createCalls=0`).
An additional full `go test ./scripts` attempt with a 120-second package
timeout did **not** pass: it timed out in the unrelated
`TestToolingTaggedPairFixturePartitionsRun` while waiting for its nested
tooling process. The hosted workflow invokes only the three named contract
tests above; the timeout is retained as an unpassed broader-suite result, not
reported as a regression pass.

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
- The final create-history check, `CreateScaleSet` call, and durable create
  result are serialized under the journal event mutex. A concurrent append is
  therefore either in the locked snapshot and rejected or ordered after the
  create result; it cannot land between the check and call. This is cooperative
  same-process serialization, not hostile-code isolation. The journal append
  mutex remains held across the bounded Create API call, so the adapter must
  honor its operation context, must not synchronously reenter the Driver's
  Journal, and concurrent journal writers wait for the call to return. The
  `API.CreateScaleSet` contract now states this restriction. The configured
  `SDKAPI` forwards the bounded context to the SDK and has no journal reference.
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

The commands above passed after the first create-boundary correction. After
the additional journal-append serialization fix, these checks also passed:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 GOWORK=off go test -count=1 -timeout=120s ./...
GOTOOLCHAIN=go1.26.8 GOWORK=off go test -race -count=1 -timeout=120s ./livecanary

cd <repository-root>
GOTOOLCHAIN=go1.26.8 GOWORK=off go test -count=1 -timeout=30s -run '^(TestG01WorkflowModuleSelectionPredicate|TestPullRequestQuickWorkflowContract)$' ./scripts
git diff --check
```

The new interleaving regression was red on the pre-fix source with one fake
Create call, then passed after the locked-history correction. The separate
concurrent-append test verifies a writer cannot append while Create is in
flight and that the create result is durable first. No full repository matrix
was run. No workflow was dispatched or replayed. Public CI on the base SHA is
not candidate evidence.

The two changed-boundary tests were also repeated 20 times:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 GOWORK=off go test -count=20 -timeout=120s -run '^TestControllerHandoffCreate(RechecksHistoryAtomicallyWithEffect|SerializesConcurrentJournalAppend)$' ./livecanary
```

They passed in 2.048s; the same two tests passed under `-race` in 1.468s.

No full repository matrix was run. No workflow was dispatched or replayed.
Public CI on the base SHA is not candidate evidence.

## Review and remaining gates

The independent GPT-6-Luna max contract review of exact HEAD
`1c98292c40cb67e1e766228c5826c29edfd05c0f` reported no new P0-P3 findings and
confirmed the synchronous Inventory/Discovery P1 was resolved. The independent
security/recovery review of that same exact HEAD confirmed those synchronous
cases but found the additional concurrent-append P1 above. The new regression
reproduced it against the pre-fix source, and source plus the serializing test
were added afterward. Fresh exact-head contract and security/recovery reviews
of the new candidate are still required; writer tests alone do not close the
finding.

The security/recovery reviewer ran
`GOTOOLCHAIN=go1.26.8 GOWORK=off go test -race -count=1 -timeout=120s -run '^TestControllerHandoff' ./livecanary` on the reviewed
HEAD; it passed in 3.159s but did not exercise the identified interleaving.

Fresh independent GPT-6-Luna max reviews of exact HEAD
`58ace4594c428b827b299faab2fddd64b6aa4350` both confirmed that the concurrent-
append P1 is resolved. The contract reviewer additionally classified
reentrant-adapter deadlock as P2; the security/recovery reviewer classified the
same source-inferred risk as P3. Neither dynamically reproduced it. The trace
is that `CreateScaleSet` runs inside `FileJournal.withEventsLocked`, so a custom
adapter that synchronously calls `Events` or `Append` on that same journal
waits on the mutex it is already inside. The configured `SDKAPI` has no journal
reference and simply forwards the bounded context and create request to the
pinned SDK. This is triaged once as an accepted nonblocking adapter-contract
restriction for the offline, unconnected slice: the interface now explicitly
forbids synchronous journal reentry. No follow-up issue is warranted for the
current adapter; any future adapter/controller wiring must preserve that
restriction or replace and re-review the lock boundary first.

The reviewers also source-traced cancellation visible at the final check,
ambiguous Create errors, and partial result-write/fsync failures. A visible
cancellation prevents the call and records unknown/create; cancellation can
still race after that check and is passed to the bounded API context. Ambiguous
remote errors quarantine without retry. A failed result append leaves the
journal unusable or torn/pending so replay cannot retry, but there is no
automated reconciliation if the remote create already happened. The review
did not dynamically exercise those narrow failure timings.

On `58ace4594c428b827b299faab2fddd64b6aa4350`, the contract reviewer passed
the focused create/handoff, atomic-history race, G02 workflow-selection, and
PR-workflow-contract checks; `git diff --check` passed and the worktree was
clean. The security/recovery reviewer passed the same two changed-boundary
tests under `-race` in 1.366s and also verified a clean exact HEAD. Neither ran
the full module suite or live operations; the coordinator's full module and
`livecanary` race results are recorded above.

Fresh independent contract and security/recovery delta reviews then verified
the API-contract/evidence-only change at exact HEAD
`cf9ace198bb1cf36bbc01dc6e71744aacf5a32d9`, with a clean worktree. Both found
no open P0/P1 and confirmed the concurrent-append P1 remains resolved. The
contract reviewer retained a conditional P2 classification for a violating
reentrant adapter; the security reviewer classified that same source-inferred
hazard as P3 and accepted it for the configured `SDKAPI`, which has no Journal
reference. Both reviews retained the fail-closed P2 limitation that credential
input failure after durable nonce consumption burns that journal's signed-create
slot. These limits are accepted only for this offline, unconnected slice; the
reentry contract and future recovery gate remain explicit. Since this final
review delta changed only an API comment and evidence, the reviewers ran no
tests. Their exact-head dispositions were recorded in the [#94 update](https://github.com/1XP-AI/gh-runnerd/issues/94#issuecomment-5792259911).

The coordinator revalidated exact HEAD `cf9ace198bb1cf36bbc01dc6e71744aacf5a32d9`
on 2026-09-23. From `experiments/g01-scaleset`, these both passed:

```text
GOTOOLCHAIN=go1.26.8 GOWORK=off go test -count=1 -timeout=120s ./...
GOTOOLCHAIN=go1.26.8 GOWORK=off go test -race -count=1 -timeout=120s ./livecanary
```

From the repository root, the metadata-selection and PR-workflow-contract tests
also passed:

```text
GOTOOLCHAIN=go1.26.8 GOWORK=off go test -count=1 -timeout=30s -run '^(TestG01WorkflowModuleSelectionPredicate|TestPullRequestQuickWorkflowContract)$' ./scripts
git diff --check 7e4d46e8762f327c03f1f80d726e040b9ba7f0a6...HEAD
```

The worktree was clean after these checks. This does not include the full
repository matrix, hosted PR quick check, GitHub Codex review, live operations,
or post-merge Public CI.

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

The maintainer authorized pushing this correction and opening PR #97, but did
not authorize merge. Obtain fresh independent contract and security/recovery
reviews on the corrected exact head, push the reviewed candidate, wait for its
hosted PR quick check, then request and complete GitHub Codex review of that
exact pushed head. Record all new finding triage before asking about merge
authorization. Only a separately authorized, reviewed merge can produce the
automatic main Public CI evidence.

The previous HOLD draft `2b40bd6` remains preserved on its original branch.
No credential access, App/token enrollment, live runner/Scale Set operation,
workflow dispatch, service/Keychain/Docker/Lima mutation, privileged action, or
destructive cleanup occurred in this correction.
