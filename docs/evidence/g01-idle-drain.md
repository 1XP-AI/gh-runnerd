# G01 idle-drain observation evidence

Issue #71 adds an experiment-only observation phase. It does not close the
parent G01 goal, add a production provider, or authorize live execution.

## Invariant and counterexample

The invariant is: withdrawing listener capacity to zero may affect the next
poll, while the poll already admitted by the listener must retain the pinned
SDK's ACK-before-acquisition ordering. Counterexamples include a response that
arrives before a request-written marker, an absent/ambiguous marker, a missing
controlled old message, a replacement/disappearing runner, unknown or
contradictory counters, a duplicate/wrong callback, or a second poll that
returns a message after withdrawal. Each is recorded as
inconclusive/quarantined and never treated as proof of a drain barrier.

The transport hook records `WroteRequest` for each physical attempt in both
bounded polls and holds the first response body before the high-level listener
parser receives it. `WroteRequest` is a client-side transport fact only:
`server_receipt` remains `unproven`, so the synthetic test and any future live
result cannot claim server acceptance or an atomic server-side drain. The
implementation calls the released listener's public `SetMaxRunners(0)`
callback and lets the listener perform its normal ACK-before-acquire sequence.

## Bounded observation

The private journal record contains only fixed categories, the initial/zero
capacities, the poll and next-poll response categories, the seven nonnegative
scale-set counters (available, acquired, assigned, running, registered, busy,
idle), exact owned scale-set identity, exact runner identity when present, a
fixed ordering, sequence and timestamp. It excludes queue URLs, access
tokens, request/response bodies, JIT material and raw SDK errors. A drain event
marks work as observed during replay; an inconclusive event additionally
retains uncertainty. Neither outcome authorizes deletion, replay,
re-acquisition or runner removal.

The prerequisite remains one already-idle, registered, owned canary runner
with zero available/acquired/assigned/running/busy work. The complete bounded
prerequisite snapshot is journaled before the listener starts; a rejected or
ambiguous snapshot writes a fixed quarantine marker, so later zero statistics
cannot authorize cleanup. No idle-worker pre-provision seam was added; if a
future live run needs one, it requires a separate independent design review
and explicit maintainer authorization.

## TDD and verification

The first implementation's historical red was:

```text
cd experiments/g01-scaleset
go test ./livecanary -run 'TestDrainListener' -count=1
```

It failed to compile because the drain hook, listener runner,
boundary/category constants and experiment seam were undefined. This is
retained as chronology only and is not claimed as meaningful behavioral TDD.
A meaningful behavioral counterexample was later reproduced retrospectively
against immutable base `cf67d4a`: `TestIssue71DrainAuthorityIsBehaviorallyAvailable`
failed because the base rejected the new `drain` phase with `approval rejected`.
That is independently inspectable base behavior, not a pre-implementation
run; the original pre-implementation meaningful-red chronology is unavailable.
The equivalent green test is `TestDrainPhaseAuthorityIsAccepted` on fix commit
`82d0d7d`.

Retrospectively, after the correction commit, a temporary test-only checkout
pinned to `6e954cf` reproduced all five independent findings with executable
assertions: poll reservation omitted, rejected idle state not fencing replay,
wrong ACK reaching the inner effect, no-message accepted as observed, and
cancellation missing an explicit marker. Those failures are defect evidence,
not a claim that the tests preceded implementation; the focused regressions
below pass on the current correction head.

The newly queued f482 defects were first reproduced behaviorally on the
current f482 worktree with the added regressions:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go test ./livecanary -run 'TestDrain(RequiresKnownConsistentStatisticsAndControlledMessage|CancellationAfterIntentRejectsEffect|CancellationBeforeSnapshotRecordsMarker)$' -count=1 -v
FAIL: unknown next-poll statistics were accepted; cancellation after ACK intent reached the inner ACK; before-snapshot cancellation had no marker.
```

The minimal correction then made those assertions pass, along with the
bounded adapter field-presence test; the cancellation fence is deliberately
not described as atomic against a remote call that was already issued or
accepted.

### Exact-head follow-up corrections

This bounded follow-up started from exact base
`6e1b2144cb92a1a53db3926dd0e924c646643dfa` for PR #72. The current-head
Codex inventory was read with the local review script using these exact
invocations (the personal script path is intentionally omitted from committed
evidence):

```text
bash /path/to/codex-review.sh all 72 --repo 1XP-AI/gh-runnerd
bash /path/to/codex-review.sh detail-all 72 --repo 1XP-AI/gh-runnerd
```

The actionable findings were [P1 withdrawn-poll physical retries](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3967496285),
[P2 drain-phase fence discharge](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3967496296),
and [P1 missing durable resolution evidence](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3967496305).
The first two were reproduced before the correction with this real failing
regression run:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go test ./livecanary -run 'TestReplayDischargesCompletedDrainPhaseFence|TestDrainListenerRejectsWithdrawnPollPhysicalRetry' -count=1 -v
```

It failed with the valid final observation still reporting `uncertain: true`
and the synthetic listener promoting a transparent retry on the withdrawn
poll to `Outcome: observed` (`WroteRequest` callbacks `[0 1 0]`, `err=<nil>`).
The minimal correction stores the drain phase fence separately, clears only
that fence after a valid matching observed record, and traces both polls;
one successful physical write per poll is required, while error, duplicate and
transparent-retry callbacks remain invalid. The first poll's SDK response and
ACK-before-acquire path are unchanged.

The focused green chronology was:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go test ./livecanary -run 'TestReplayDischargesCompletedDrainPhaseFence|TestReplayDrainFence|TestDrainPollHookRequiresOneSuccessfulWritePerPoll|TestDrainListenerRejectsWithdrawnPollPhysicalRetry' -count=1 -v
GOTOOLCHAIN=go1.26.8 go test -race ./livecanary -run 'TestDrain|TestReplayDischargesCompletedDrainPhaseFence|TestReplayDrainFence' -count=1
GOTOOLCHAIN=go1.26.8 go test ./livecanary -count=1
```

All three commands passed after the correction; the first includes the 4x4
first/second poll matrix (success, write error, duplicate callback and
transparent retry), and the real listener fixture remained inconclusive for a
withdrawn-poll retry. These are offline tests only.

The prior [P1 transparent first-poll retry finding](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3966770569)
and [P1 contradictory poll-counter finding](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3966770561)
remain part of the durable chronology. Their historical red preceded the
earlier correction; the current follow-up re-ran the regression guards with:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go test ./livecanary -run 'TestDrainPollHookRejectsDuplicatePhysicalWrites|TestDrainPollHookRequiresOneSuccessfulWritePerPoll' -count=1 -v
GOTOOLCHAIN=go1.26.8 go test ./livecanary -run 'TestDrainObservationRejectsPollCountersContradictingOwnedRunner|TestDrainClientRejectsPollRunnerPartitionMismatchBeforeEffects|TestDrainRequiresKnownConsistentStatisticsAndControlledMessage' -count=1 -v
```

Both commands passed on the corrected source. The first-poll guard counts
every physical callback and rejects retries; the counter guards reject a
valid-but-contradictory runner partition before ACK or acquisition. The
[duplicate-key regression](https://github.com/1XP-AI/gh-runnerd/commit/6e1b2144cb92a1a53db3926dd0e924c646643dfa)
is also retained and was re-run with:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go test ./livecanary -run 'TestDrainStatisticsRejectsDuplicateJSONFields|TestDrainPollHookRejectsDuplicatePhysicalWrites' -count=1 -v
```

That command passed; duplicate or case-folded JSON keys remain unknown rather
than being accepted as a complete statistics sample. The historical reds are
not relabeled as pre-implementation tests for this follow-up; the actual red
run above is the new TDD regression, followed by the listed green runs.

## Independent correction matrix

| Finding | Correction and evidence |
|---|---|
| Codex P1 [status-only response close](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3965776832) | `drainHeldBody.Close` remains release-gated; `TestDrainHeldBodyHoldsCloseUntilRelease` passes (fix `f44f5f9`). |
| Codex P1 [cancellation join](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3965776826) | all listener context exits release and join the run goroutine; `TestDrainRejectsEffectsAfterCancellationAndRecordsMarker` passes (fixes `f44f5f9`, `82d0d7d`). |
| Codex P1 [drain route unreachable](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3965776815) | `Run` routes `drain` before generic owned/statistics quarantine; `TestDriverRoutesDrainBeforeNoWorkerStatisticsQuarantine` passes (fix `f44f5f9`). |
| Codex P2 [paired verification](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3965957331) | drain is admitted as a verification phase in paired approval/preparation; paired tests pass (fix `945371e`). |
| Codex P1 [runner continuity](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3966125441) | observed requires exact before/after runner tuple; runner mutation is rejected (fix `6e954cf`). |
| Codex P1 [embedded session set](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3966362999) | session-open now requires exact nested set identity, update fence, labels, and statistics equal to the idle before snapshot. `TestDrainRejectsEmbeddedSessionStatisticsMismatch` passes (fix `82d0d7d`). |
| Codex P1 [ambiguous session retention](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3966363009) | non-successful listener outcomes retain the live session, write a quarantine marker, and never invoke session close; `TestDriverRoutesDrainBeforeNoWorkerStatisticsQuarantine` asserts zero close calls (fix `82d0d7d`). |
| Codex P1 [phase crash fence](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3966362990) | replay marks a durable `drain` phase uncertain before polling, so a crash before the final observation cannot authorize cleanup. `TestDrainPhaseStartRetainsCrashUncertainty` passes (fix `82d0d7d`). |
| Codex P1 [transparent first-poll retry](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3966770569) | every first-poll physical `WroteRequest` callback is counted; duplicates/errors remain invalid. `TestDrainPollHookRejectsDuplicatePhysicalWrites` and the 4x4 two-poll matrix pass. |
| Codex P1 [contradictory poll counters](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3966770561) | poll runner partitions must match the owned idle prerequisite before ACK/acquisition; `TestDrainObservationRejectsPollCountersContradictingOwnedRunner` and `TestDrainClientRejectsPollRunnerPartitionMismatchBeforeEffects` pass. |
| Codex P1 [withdrawn-poll retry](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3967496285) | both bounded polls are traced; any second-poll error/duplicate/retry prevents `observed`, and the actual listener fixture remains inconclusive. `TestDrainListenerRejectsWithdrawnPollPhysicalRetry` passes. |
| Codex P2 [drain-phase fence discharge](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3967496296) | replay discharges only a valid matching observed drain record's phase-local fence; unrelated uncertainty, work, reservations, and failed/inconclusive/crashed fences survive. `TestReplayDischargesCompletedDrainPhaseFence`, `TestReplayDrainFencePreservesUnrelatedUncertaintyAndReservations`, and `TestReplayDrainFenceRetainsInconclusiveOutcome` pass. |
| Codex P2 [after-snapshot prerequisite scope](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3969825423) | replay requires the first ordered drain snapshot to prove the owned idle prerequisite, then checks only after-snapshot identity and runner partition while allowing job-counter changes. The real pinned-SDK/FileJournal regression and malformed-stage matrix pass. |
| Codex P1 [durable finding evidence](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3967496305) | this section and the matrix record full finding URLs, exact red/green commands, actual outcomes and rollback scope for the current and prior corrections. |
| Security F1 / Protocol F1 poll reservation | `observe-poll` result journals one verified request ID plus fixed work before ACK; replay sets reservation/work fences. `TestDrainPollJournalsReservationBeforeACK` passes (`82d0d7d`). |
| Security F2 rejected idle prerequisite | complete bounded `DrainSnapshot` plus `prerequisite-failed` marker is retained; replay remains uncertain. `TestDrainRejectedIdlePrerequisiteRetainsFence` passes (`82d0d7d`). |
| Security F3 / Protocol F2 runner and nested set identity | observed requires exact runner continuity; session-open validates nested set ID/name/group/label/update fence. Pinned SDK drain integration and mutation tests pass (`6e954cf`, `82d0d7d`). |
| Security F4 / Protocol F1 no-message and field presence | observed requires a present old controlled message, complete known counters on both polls, successful ACK and acquisition; bodyless 202/no-message and empty/missing statistics objects remain inconclusive. The bounded adapter retains only field presence/scalars ephemerally; `TestDrainListenerNoMessageIsInconclusive`, `TestDrainRequiresKnownConsistentStatisticsAndControlledMessage`, and `TestDrainPollHookPreservesStatisticsFieldPresence` pass on the correction head. |
| Security F5 / Protocol F5 counters and integration | known counters require nonnegative internally consistent busy/idle partition; unknown values cannot masquerade as zero. `TestDrainRequiresKnownConsistentStatisticsAndControlledMessage` and `TestDriverDrainThroughPinnedSDKAndPollHook` pass (`82d0d7d`). |
| Protocol F3 cancellation/WithoutCancel | phase context is bound through the wrapper, checked before every ACK/acquisition/poll effect, and cancellation/deadline/quarantine writes a fixed marker even when close/after inspect fails. `TestDrainRejectsEffectsAfterCancellationAndRecordsMarker` passes (`82d0d7d`). |
| Protocol F4 duplicate/wrong callbacks | exact phase, identity, ACK-before-acquire and one-shot state are validated before inner effects; wrong/duplicate callbacks make zero additional inner calls. `TestDrainRejectsDuplicateOrWrongEffectsBeforeInnerCall` passes (`82d0d7d`). |
| Protocol F6 cancellation race and early snapshot | durable intent is followed by a cancellation fence immediately before the bounded SDK call; a canceled before-snapshot path records its fixed marker. The fence does not claim to revoke bytes already accepted by a remote service. `TestDrainCancellationAfterIntentRejectsEffect` and `TestDrainCancellationBeforeSnapshotRecordsMarker` pass on the correction head. |
| Protocol F9 TDD chronology | the compile-only red and retrospective base/repro executions are now labeled candidly; no later archive reproduction is represented as pre-implementation evidence. |

The independent design review remains respected: the transport marker is
client-side only, `server_receipt` is `unproven`, the real high-level pinned
listener is retained, the hook is bounded to one old and one next poll, and no
pre-provision, recovery, replay, `RemoveRunner` or live operation was added.

Verified locally with the pinned `github.com/actions/scaleset v0.4.0` module:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go test ./livecanary -run 'TestDrain|TestCredentialAttestationMismatchAndExpiredTokenRejected' -count=1
GOTOOLCHAIN=go1.26.8 go test -race ./livecanary -run 'TestDrain|TestDriverDrainThroughPinnedSDK' -count=1
GOTOOLCHAIN=go1.26.8 go vet ./livecanary
cd ../g02-auth
GOTOOLCHAIN=go1.26.8 go test ./... -run 'TestBrokerFinitePhases|TestBrokerControllerApproval' -count=1
cd ../..
git diff --check
bash scripts/gofmt.sh check
bash scripts/check-offline-experiments.sh
```

All commands above passed, as did `GOTOOLCHAIN=go1.26.8 go test ./...` in both
experiment modules and the focused/race drain suites. The offline script
reported `offline experiment checks passed: 2 module(s)`.
The listener tests use `httptest` synthetic transports and the real released
listener; the pinned-SDK test composes `OpenDrainSession` with the actual
`MessageSessionClient` offline. These are offline protocol evidence, not live
qualification. No credentials, runner/session/JIT operation,
workflow operation, app/keychain/launchd/Docker/Lima mutation or cleanup was
performed.

### Exact-head blocker corrections (local uncommitted tree)

Before edits, the independent review probes were re-run against the immutable
`1eb48afb50ffbb10b42d07181f16153df1c494eb` source extraction with:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go test ./livecanary -run 'TestIndependentReplayPhaseBindingBoundaries|TestIndependentFileJournalStoresMismatchedDrainSequence|TestIndependentContradictoryNextPollStatsCannotPromoteDrain' -count=1 -v
GOTOOLCHAIN=go1.26.8 go test ./livecanary -run 'TestSecurityReviewReplayIdentitySequencePhaseBoundaries|TestSecurityReviewValidObservationNeedsExactRequiredFields|TestSecurityReviewDrainObservationDoesNotAuthorizeLaterWorkOrCleanup' -count=1 -v
```

Both commands failed as expected. Replay accepted absent, repeated,
interrupted, foreign-set and `Sequence=999` histories; the durable FileJournal
reopen accepted `Sequence=999`; and the listener promoted a known withdrawn
poll partition of `registered=0,busy=0,idle=0` to `observed`. The probes also
confirmed the existing runner continuity, byte/body budget, retry trace and
ACK-before-acquisition controls remained intact before this correction.

The local red-first correction adds phase-local replay binding and the
withdrawn-poll runner-partition fence. A drain phase records its created
scale-set ID, and its journal-assigned event sequence becomes the only valid
`Drain.Sequence`; replay requires exactly one active phase, the created set ID
in both snapshots, and a later matching observation. Both polls and the final
snapshot compare only `registered`, `busy` and `idle` runner counters, so job
counters remain free to change.

The focused correction and positive controls were run from the experiment
module:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go test ./livecanary -run 'TestReplayDrainRequiresOneMatchingPhaseIdentityAndSequence|TestFileJournalDrainReplayRetainsMismatchedSequenceAndIdentity|TestDrainObservedAllowsNextPollJobCounterChanges|TestDrainListenerRejectsContradictoryWithdrawnPollRunnerPartition|TestReplayDischargesCompletedDrainPhaseFence|TestDriverDrainThroughPinnedSDKAndPollHook' -count=1 -v
GOTOOLCHAIN=go1.26.8 go test ./livecanary -count=1
GOTOOLCHAIN=go1.26.8 go test -race ./livecanary -run 'TestDrain|TestReplayDrainRequiresOneMatchingPhaseIdentityAndSequence|TestFileJournalDrainReplayRetainsMismatchedSequenceAndIdentity|TestDriverDrainThroughPinnedSDKAndPollHook' -count=1
GOTOOLCHAIN=go1.26.8 go vet ./livecanary
cd ../..
bash scripts/gofmt.sh check
bash scripts/check-offline-experiments.sh
```

All commands passed. The first includes the real FileJournal close/reopen
checks and the real pinned `github.com/actions/scaleset v0.4.0` listener path;
the full package, focused race, vet, formatting and offline experiment checks
also passed. These remain offline tests only, and the local correction is
intentionally uncommitted and unpublished for coordinator exact-head review.

### Missing drain-phase SetID correction (current local uncommitted tree)

The red-first regression was added before the implementation change and run
with:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go test ./livecanary -run 'TestReplayDrainPhaseRequiresPositiveSetID|TestFileJournalRejectsNonPositiveDrainPhaseSetID|TestFileJournalForeignDrainPhaseSetIDRetainsFenceAfterReopen' -count=1 -v
```

It failed as expected: replay discharged matching observations for missing and
zero phase IDs, and the real FileJournal accepted missing, zero and negative
drain-phase IDs. Negative and foreign-positive direct replay histories already
remained fenced.

The minimal correction makes `validEvent` require `ID > 0` for
`phase/drain`, prevents replay from inferring a missing, zero or negative ID
from the prior create result, and retains uncertainty for foreign-positive
phase IDs. The Driver's positive phase ID is now asserted through the pinned
SDK journal path together with exact phase-sequence observation binding.

The focused green and verification commands were:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go test ./livecanary -run 'TestReplayDrainPhaseRequiresPositiveSetID|TestFileJournalRejectsNonPositiveDrainPhaseSetID|TestFileJournalForeignDrainPhaseSetIDRetainsFenceAfterReopen|TestReplayDrainRequiresOneMatchingPhaseIdentityAndSequence|TestFileJournalDrainReplayRetainsMismatchedSequenceAndIdentity|TestDrainObservedAllowsNextPollJobCounterChanges|TestDrainListenerRejectsContradictoryWithdrawnPollRunnerPartition|TestReplayDischargesCompletedDrainPhaseFence|TestReplayDrainFencePreservesUnrelatedUncertaintyAndReservations|TestDriverDrainThroughPinnedSDKAndPollHook' -count=1 -v
GOTOOLCHAIN=go1.26.8 go test -race ./livecanary -run 'TestReplayDrainPhaseRequiresPositiveSetID|TestFileJournalRejectsNonPositiveDrainPhaseSetID|TestFileJournalForeignDrainPhaseSetIDRetainsFenceAfterReopen|TestReplayDrainRequiresOneMatchingPhaseIdentityAndSequence|TestFileJournalDrainReplayRetainsMismatchedSequenceAndIdentity|TestDrainListenerRejectsContradictoryWithdrawnPollRunnerPartition|TestReplayDischargesCompletedDrainPhaseFence|TestReplayDrainFencePreservesUnrelatedUncertaintyAndReservations|TestDriverDrainThroughPinnedSDKAndPollHook' -count=1
GOTOOLCHAIN=go1.26.8 go vet ./livecanary
gofmt -l experiments/g01-scaleset/livecanary/journal.go experiments/g01-scaleset/livecanary/driver.go experiments/g01-scaleset/livecanary/drain_followup_test.go experiments/g01-scaleset/livecanary/sdk_integration_test.go
git diff --check
```

All listed verification commands passed; the formatting and diff checks emitted
no output. No broad multi-module gate was rerun for this narrow correction.
These are offline tests only; no live runner, workflow, credential, journal
cleanup, or external operation was performed.

### Security F2 owned-idle prerequisite correction (current local uncommitted tree)

The prior F1 phase-ID correction remains in place; its separate red/green
evidence above is unchanged and was included in the focused verification below.
The new F2 red-first regression was run before its implementation with:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go test ./livecanary -run 'TestSecurityReviewDrainRequiresOwnedIdleBeforeProof|TestSecurityReviewMatchingDrainPhaseDischargesAfterFileJournalReopen|TestSecurityReviewValidOneIdleRunnerDrainObservation|TestSecurityReviewInconclusiveDrainMayHaveMissingOwnedIdentity|TestSecurityReviewObservedDrainAllowsLegitimateJobCounterChanges' -count=1 -v
```

It failed as expected: observed two-runner and busy snapshots were accepted,
and the real FileJournal accepted a positive-ID, correctly sequenced
non-owned-idle observation and discharged the replay fence. Missing owned
identity already remained rejected, while the valid one-idle, inconclusive,
and legitimate job-counter controls passed.

The minimal F2 correction adds the existing `validDrainIdlePrerequisite` to
the `drainOutcomeObserved` branch of `validDrainObservation`; the runner
partition equality checks remain unchanged. Inconclusive observations may
still carry missing runner identity, while observed evidence now requires the
owned one-runner idle proof before journal append or replay can discharge a
phase fence.

The focused green, race, pinned-SDK, vet, formatting, and diff checks were:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go test ./livecanary -run 'TestSecurityReviewDrainRequiresOwnedIdleBeforeProof|TestSecurityReviewMatchingDrainPhaseDischargesAfterFileJournalReopen|TestSecurityReviewValidOneIdleRunnerDrainObservation|TestSecurityReviewInconclusiveDrainMayHaveMissingOwnedIdentity|TestSecurityReviewObservedDrainAllowsLegitimateJobCounterChanges|TestReplayDrainPhaseRequiresPositiveSetID|TestFileJournalRejectsNonPositiveDrainPhaseSetID|TestFileJournalForeignDrainPhaseSetIDRetainsFenceAfterReopen|TestReplayDrainRequiresOneMatchingPhaseIdentityAndSequence|TestFileJournalDrainReplayRetainsMismatchedSequenceAndIdentity|TestDrainObservedAllowsNextPollJobCounterChanges|TestDrainListenerRejectsContradictoryWithdrawnPollRunnerPartition|TestReplayDischargesCompletedDrainPhaseFence|TestReplayDrainFencePreservesUnrelatedUncertaintyAndReservations|TestDriverDrainThroughPinnedSDKAndPollHook' -count=1 -v
GOTOOLCHAIN=go1.26.8 go test -race ./livecanary -run 'TestSecurityReviewDrainRequiresOwnedIdleBeforeProof|TestSecurityReviewMatchingDrainPhaseDischargesAfterFileJournalReopen|TestSecurityReviewValidOneIdleRunnerDrainObservation|TestSecurityReviewInconclusiveDrainMayHaveMissingOwnedIdentity|TestSecurityReviewObservedDrainAllowsLegitimateJobCounterChanges|TestReplayDrainPhaseRequiresPositiveSetID|TestFileJournalRejectsNonPositiveDrainPhaseSetID|TestFileJournalForeignDrainPhaseSetIDRetainsFenceAfterReopen|TestReplayDrainRequiresOneMatchingPhaseIdentityAndSequence|TestFileJournalDrainReplayRetainsMismatchedSequenceAndIdentity|TestDrainListenerRejectsContradictoryWithdrawnPollRunnerPartition|TestReplayDischargesCompletedDrainPhaseFence|TestReplayDrainFencePreservesUnrelatedUncertaintyAndReservations|TestDriverDrainThroughPinnedSDKAndPollHook' -count=1
GOTOOLCHAIN=go1.26.8 go vet ./livecanary
gofmt -l experiments/g01-scaleset/livecanary/drain.go experiments/g01-scaleset/livecanary/journal.go experiments/g01-scaleset/livecanary/driver.go experiments/g01-scaleset/livecanary/drain_followup_test.go experiments/g01-scaleset/livecanary/security_review_extra_test.go experiments/g01-scaleset/livecanary/sdk_integration_test.go
git diff --check
```

All listed checks passed; the race run reported no race, and formatting and
diff checks emitted no output. The pinned `github.com/actions/scaleset v0.4.0`
Driver path remains offline-only, no broad multi-module gate was repeated, and
no live operation or personal path was added.

### Codex r3969825423 phase-aware replay correction

The fresh exact-head finding is [Codex r3969825423](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3969825423): replay was applying the before-only
`validDrainIdlePrerequisite` to the after `DrainSnapshot`, so a legitimate
`TotalAcquiredJobs=1` after snapshot permanently set uncertainty even though the
observed drain contract permits job-counter changes. The historical original
TDD exception and prior clean-history consolidation remain unchanged; this is a
new meaningful red-first regression for the current correction.

Before the implementation change, the real pinned-SDK Driver/FileJournal
regression was run from `experiments/g01-scaleset`:

```text
GOTOOLCHAIN=go1.26.8 go test ./livecanary -run '^TestDriverDrainThroughPinnedSDKAndPollHook$' -count=1 -v
```

It failed after the listener completed and the FileJournal was closed/reopened:
`legitimate after job-counter change retained replay uncertainty` with
`uncertain:true`. The fixture recorded matching drain phase SetID/sequence,
before and after `DrainSnapshot` result records through the real Driver, and
kept the after runner partition/identity unchanged while changing only the
acquired-job counter.

The minimal correction makes replay treat only the first ordered
`result/observe-runner` snapshot in a pending drain phase as the prerequisite;
the second must retain the created set identity, exact runner identity and
registered/busy/idle partition, while its job counters may change. Missing or
invalid before proof, a malformed after snapshot, or a snapshot outside that
phase-local ordering remains uncertain; unrelated reservations, work and
uncertainty are never cleared.

The focused green, race, pinned-SDK, vet, formatting and diff checks were:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go test ./livecanary -run 'TestDriverDrainThroughPinnedSDKAndPollHook|TestFileJournalReplayFencesMalformedDrainSnapshotStages|TestReplayDrainRequiresOneMatchingPhaseIdentityAndSequence|TestFileJournalDrainReplayRetainsMismatchedSequenceAndIdentity|TestDrainObservedAllowsNextPollJobCounterChanges|TestDrainListenerRejectsContradictoryWithdrawnPollRunnerPartition' -count=1 -v
GOTOOLCHAIN=go1.26.8 go test -race ./livecanary -run 'TestDriverDrainThroughPinnedSDKAndPollHook|TestFileJournalReplayFencesMalformedDrainSnapshotStages|TestReplayDrainRequiresOneMatchingPhaseIdentityAndSequence|TestFileJournalDrainReplayRetainsMismatchedSequenceAndIdentity|TestDrainObservedAllowsNextPollJobCounterChanges|TestDrainListenerRejectsContradictoryWithdrawnPollRunnerPartition' -count=1
GOTOOLCHAIN=go1.26.8 go test ./livecanary -count=1
GOTOOLCHAIN=go1.26.8 go vet ./livecanary
cd ../..
bash scripts/gofmt.sh check
git diff --check
```

All listed checks passed: focused green, focused race, full `livecanary`
package, pinned SDK drain replay, vet, formatting and diff checks. The malformed
FileJournal matrix covers missing-before/no unsafe after inference, busy before,
after partition change, after scale-set identity change and after runner
identity change; each remains fenced. These are offline tests only; no live
runner, credential, workflow, session/JIT, cleanup or external operation was
performed.

## Remaining gate and rollback

The live G01 gate remains unresolved until a separately authorized run uses an
immutable reviewed head, the approved private repository/workflow/resources,
one idle owned canary runner and independent review. A timing miss, missing
runner, stale zero, identity/statistics mismatch, unknown response, or
server-receipt ambiguity must remain inconclusive. Rollback of the latest
follow-up is a focused revert of its code/test/documentation commit(s), with
the exact current journal and owned resources retained for inspection. If an
earlier correction must be isolated, revert only the reviewed source slice for
[r3966770569](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3966770569),
[r3966770561](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3966770561),
or the [duplicate-key regression](https://github.com/1XP-AI/gh-runnerd/commit/6e1b2144cb92a1a53db3926dd0e924c646643dfa)
after checking dependent corrections. To roll back only the current
[r3969825423](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3969825423)
correction, revert its focused source/test/evidence commit while retaining the
current journal and owned resources for inspection; do not reset, erase or
replay the journal. No live cleanup, workflow replay, runner mutation or
rollback operation was performed.
