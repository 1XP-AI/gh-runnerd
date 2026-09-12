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
below pass on the published correction snapshots.

The newly queued f482 defects were first reproduced behaviorally on the
historical f482 snapshot `f482d249e6e5eca7bd03ce55cdaf3b6cde7a671d` with the
added regressions:

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

### Acquire target correction (exact head `9b1f0c214022740d6276b6f9dd417f4f892d0bd5`)

The follow-up review identified a production-boundary leak in the bounded
acquisition capture: `baselineWireCapture.target` accepted a synthetic
queue-URL `/acquirejobs` request, and its suffix-only Actions matcher accepted
the same endpoint shape on an arbitrary host. The pinned SDK source confirms
the real request is `POST /_apis/runtime/runnerscalesets/{setID}/acquirejobs`
with exactly `api-version=6.0-preview`; the strict `count`/`value` decoder and
the prior [acquisition P1 evidence](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3972911056)
remain unchanged.

The meaningful red was run before the correction from the exact head above:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go test ./livecanary -run '^TestBaselineAcquireTargetIsActionsOnly$' -count=1 -v
```

It failed because `queue_endpoint` and `wrong_host` both returned `true`
instead of `false`. The minimal source correction in
`5ffaa604a17f16c5b77aebf088c7fcec47e816bd` removes the queue compatibility
branch, requires the Actions runtime path and an approved/API host, and moves
the synthetic fixture to the pinned endpoint; queue query material is not
copied into that test-only request. The complete poll-statistics preflight
before `VerifyRun`, strict bounded acquisition decoding, and all six prior P1
fixes remain in place.

The focused green and safety checks were:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go test ./livecanary -run '^(TestBaselineAcquireTargetIsActionsOnly|TestDrainListenerWithdrawsWhilePollResponseIsHeld|TestPinnedSDKDrainAcquisitionRequiresStrictWireResponse)$' -count=1 -v -timeout=180s
GOTOOLCHAIN=go1.26.8 go test ./livecanary -run '^(TestBaselineAcquireTargetIsActionsOnly|TestPinnedSDKDrain.*|TestDriverDrainThroughPinnedSDKAndPollHook|TestDrainListenerWithdrawsWhilePollResponseIsHeld|TestDrainCancellationStopsBeforeReleasingHeldResponse)$' -count=1 -v -timeout=180s
GOTOOLCHAIN=go1.26.8 go test -race ./livecanary -run '^(TestBaselineAcquireTargetIsActionsOnly|TestPinnedSDKDrain.*|TestDriverDrainThroughPinnedSDKAndPollHook|TestDrainListenerWithdrawsWhilePollResponseIsHeld|TestDrainCancellationStopsBeforeReleasingHeldResponse)$' -count=1 -timeout=180s
GOTOOLCHAIN=go1.26.8 go vet ./livecanary
cd ../..
test -z "$(gofmt -l experiments/g01-scaleset/livecanary/baseline_wire.go experiments/g01-scaleset/livecanary/baseline_listener.go experiments/g01-scaleset/livecanary/drain_driver.go experiments/g01-scaleset/livecanary/sdk.go experiments/g01-scaleset/livecanary/drain_test.go experiments/g01-scaleset/livecanary/drain_p1_followup_test.go)"
git diff --check
```

All focused normal/race tests passed, vet was silent and successful, and the
format/diff checks were clean. The queue endpoint, wrong host, wrong set/path,
and wrong method cases remain rejected while the actual pinned SDK endpoint and
the synthetic fixture endpoint are accepted; these are offline checks only.
No live operation, credential, runner, Docker, Keychain, launchd, workflow or
personal-path mutation occurred, and historical snapshots remain unchanged.
Rollback is recoverable with normal `git revert --no-edit
5ffaa604a17f16c5b77aebf088c7fcec47e816bd`, which returns the source to the
published `9b1f0c214022740d6276b6f9dd417f4f892d0bd5` correction head; the
documentation-only commit that records this evidence can be reverted
separately without rewriting history.

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

## Exact-head P1 follow-up: marked boundaries and runtime-origin binding

Date: 2026-09-13. This section records the current exact-head follow-up against
starting snapshot `6357865735034ff326401c9d535afe5d07ba3433`. The independent
finding URLs are [acquisition mismatch pre-forwarding](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3976535985),
[Scale Set origin binding](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3976535994),
[session-open body ownership](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3976536000),
and [durable finding evidence](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3976536008).
The durable-evidence finding specifically named the two preceding exact-head
findings, so their URLs are retained here as well: [acquisition origin](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3976171401)
and [session-open route binding](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3976171403).

### Red-first reproductions

After the boundary tests were added and before the corresponding source
corrections, this focused command exited 1:

```text
cd experiments/g01-scaleset
GOWORK=off GOTOOLCHAIN=go1.26.8 go test ./livecanary -run '^(TestBaselineAcquireTargetMismatchStopsBeforeInner|TestBaselineSessionOpenBodyMustMatchOwnerBeforeInner|TestBaselineSnapshotRequestsRequireExactOriginBeforeInner)$' -count=1 -v -timeout=60s
```

The current-head red result was meaningful: the case-folded and wrong-route
acquisition mutations, every invalid/ambiguous session-open body, and the
wrong-origin Scale Set and runner requests reached the inner transport instead
of being rejected. The test retained only counters and fixed error outcomes;
no request body, token, URL, response error, or private log was recorded.

The pinned-SDK body regression was independently run before the body fix:

```text
cd experiments/g01-scaleset
GOWORK=off GOTOOLCHAIN=go1.26.8 go test ./livecanary -run '^TestPinnedSDKDrainRejectsAmbiguousSessionRequestBeforeFixture$' -count=1 -v -timeout=180s
```

It exited 1 because all four mutated bodies (wrong owner, case-fold duplicate,
unknown field, and malformed JSON) were accepted and reached the fixture. The
pinned Scale Set origin regression was also run before its source fix:

```text
cd experiments/g01-scaleset
GOWORK=off GOTOOLCHAIN=go1.26.8 go test ./livecanary -run '^TestPinnedSDKDrainRejectsSnapshotOriginMismatchBeforeEffects$' -count=1 -v -timeout=180s
```

It exited 1 because the second snapshot on the other runtime origin reached the
fixture/listener path. The direct boundary red test above also covered the
runner-origin mismatch; the pinned runner regression was added after that red
reproduction and then verified against the pinned SDK.

### Corrections and resolution evidence

Marked acquisition captures now reject every request unless the exact Actions
route, method, query, approved HTTPS origin and one expected request-ID set are
present. A marked acquisition with no IDs is itself quarantined, and there is
no acquisition bootstrap exception; case-folded route spellings, wrong route
families, duplicate/extra query pairs and second requests stop before the inner
transport.

Marked session-open POSTs are decoded with strict unknown-field and duplicate-key
handling. The approved pinned SDK v0.4.0 request must contain the all-zero
session ID and an exact `ownerName` equal to the approved owner; absent, wrong,
case-folded, duplicate, unknown or malformed bodies are rejected before
forwarding. A rejected body never captures an origin or session identity, so a
later close cannot claim a safe identity; the pinned fixture counter proves the
ambiguous request does not reach the fixture.

Scale Set and runner snapshot captures now require an approved HTTPS runtime
origin. The before Scale Set request learns one exact canonical origin; the
before runner, after Scale Set and after runner observations all require that
same origin. `drainSnapshotWithOrigin` quarantines incomplete wire-reader pairs,
missing endpoint host allowlists, status/fact mismatches and any changed origin.
The production `SDKAPI` implements both wire readers and the endpoint-host
reader; the no-wire branch remains only for pre-existing synthetic API tests and
does not claim a runtime origin or serve as pinned-SDK evidence.

The focused green command after the corrections exited 0:

```text
cd experiments/g01-scaleset
GOWORK=off GOTOOLCHAIN=go1.26.8 go test ./livecanary -run '^(TestBaselineAcquireTargetIsActionsOnly|TestBaselineAcquireTargetMismatchStopsBeforeInner|TestBaselineMarkedAcquireWithoutIDsStopsBeforeInner|TestBaselineSessionOpenTargetMismatchStopsBeforeInner|TestBaselineSessionOpenBodyMustMatchOwnerBeforeInner|TestBaselineSnapshotRequestsRequireExactOriginBeforeInner|TestBaselineAcquireTargetRequiresCapturedSessionOrigin|TestBaselineAcquireOriginMismatchStopsBeforeInner|TestBaselineSessionCloseTargetRequiresExactOrigin|TestPinnedSDKDrainRejectsAmbiguousSessionRequestBeforeFixture|TestPinnedSDKDrainRejectsSnapshotOriginMismatchBeforeEffects|TestPinnedSDKDrainRejectsRunnerSnapshotOriginMismatchBeforeListener|TestDriverDrainThroughPinnedSDKAndPollHook|TestPinnedSDKDrainAcceptsUnrelatedRunnerMetadata|TestPinnedSDKDrainSnapshotsRequireStrictWireFacts)$' -count=1 -timeout=240s
```

The command passed in 0.427s. The same expression with `go test -race` also
passed with no race diagnostics. The runner-origin pinned test specifically
observed zero listener polls, while the Scale Set-origin test observed exactly
one fixture snapshot read and no observed drain.

The required full package gates then passed:

```text
cd experiments/g01-scaleset
GOWORK=off GOTOOLCHAIN=go1.26.8 go test ./livecanary -count=1 -timeout=300s
```

This passed in 27.859s.

```text
cd experiments/g01-scaleset
GOWORK=off GOTOOLCHAIN=go1.26.8 go test -race ./livecanary -count=1 -timeout=300s
```

This passed in 39.211s with no race diagnostics. `GOWORK=off
GOTOOLCHAIN=go1.26.8 go vet ./livecanary`, `gofmt -d` over the seven touched
Go files, and `git diff --check` all exited 0.

### Finding matrix and rollback

| Finding | Reproduction and resolution | Rollback evidence |
|---|---|---|
| [r3976535985](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3976535985) acquisition mismatch | Red command above; exact marked acquisition/no-ID tests now reject before inner transport. | Revert this candidate source/test/evidence commit as one unit; no live rollback was run. |
| [r3976535994](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3976535994) Scale Set origin | Red command above plus pinned second-origin test; before/after Scale Set and runner snapshots now share one exact origin. | The pinned wrong-origin test quarantines before the second fixture snapshot; focused revert only. |
| [r3976536000](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3976536000) session-open body | Pinned body red command above; strict owner/body validation and zero fixture opens now pass. | Ambiguity retains no close identity; focused revert only, with no live close/cleanup. |
| [r3976536008](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3976536008) durable evidence | This section records all four current URLs, the two prior URLs named by the finding, red/green results, resolution and rollback scope. | Documentation is included in the same candidate commit and can be reverted with it. |
| Independently reproduced runner-origin gap | Direct runner wrong-origin red case and pinned `TestPinnedSDKDrainRejectsRunnerSnapshotOriginMismatchBeforeListener` green regression; no separate review URL was supplied. | Runner mismatch quarantines before listener effects; focused revert only. |

No live GitHub App, runner, workflow, Docker/Lima, Keychain, launchd, network
resource, cleanup or rollback operation was performed. The live G01 gate,
independent exact-head Codex review and CI remain coordinator-owned.

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

### Exact-head follow-up: snapshot-bound session origin and marked listener polls

Date: 2026-09-13. This correction started from exact head
`8439c12e431bb25bd229c9783119bee125b0c0bd`. The current-head Codex findings
were [session origin not bound to the before snapshot](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3996893012)
and [marked listener poll mismatch forwarded before rejection](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3996893019).
No Project or Issue ownership/status/goal/dependency field was changed.

#### Red-first reproductions

After adding the two behavioral regressions and before changing production
code, this focused command exited 1:

```text
cd experiments/g01-scaleset
GOWORK=off GOTOOLCHAIN=go1.26.8 go test ./livecanary -run '^(TestPinnedSDKDrainRejectsSessionOriginMismatchBeforeListener|TestDrainListenerRejectsMarkedPollTargetMismatchBeforeInner)$' -count=1 -v -timeout=240s
```

The session-origin regression observed one listener poll (`polls=1`) instead
of quarantining before listener start. The marked-poll regression observed
the wrong-host, wrong-path and wrong-scheme mutations reach the inner
transport (`calls=1` in each case); the already-existing query validator
rejected the wrong queue-proof case, so that subcase passed in the red run.
The failing output retained only fixed counters and error categories; no
request body, bearer, URL query, response error or private log was recorded.

#### Corrections and boundary evidence

The drain driver now initializes the poll hook with the exact origin captured
by the before Scale Set snapshot, and `OpenDrainSession` refuses a different
session-open origin. The driver independently compares `hook.origin` with the
captured origin after session-open and before `runDrainListener`; a mismatch
quarantines without starting a poll. Synthetic API fakes keep the prior
no-wire path, while the pinned SDK path remains origin-bound.

Listener polls now carry a private per-hook approval marker through the
listener call context. Only a marked poll is admitted to the poll hook's
physical validation: exact queue target/path/query, canonical origin,
capacity and cursor are checked before calling the inner transport, and a
foreign marker or any mismatch is rejected/quarantined before forwarding.
Unmarked session-open, ACK and acquisition requests continue through their
existing baseline wire boundaries.

The focused green normal and boundary run passed after the corrections:

```text
cd experiments/g01-scaleset
GOWORK=off GOTOOLCHAIN=go1.26.8 go test ./livecanary -run '^(TestPinnedSDKDrainRejectsSessionOriginMismatchBeforeListener|TestDrainListenerRejectsMarkedPollTargetMismatchBeforeInner|TestDrainPollHookRejectsApprovalFromDifferentHook|TestDrainPollHookForwardsUnmarkedNonPollRequest)$' -count=1 -v -timeout=240s
```

It passed with all four marked-poll mutations rejected before the inner
transport, the foreign marker rejected before the inner transport, and the
unmarked session request forwarded once. The focused race expression covering
the drain, pinned-SDK and baseline boundary families also passed with no race
diagnostics:

```text
cd experiments/g01-scaleset
GOWORK=off GOTOOLCHAIN=go1.26.8 go test -race ./livecanary -run 'TestDrain|TestPinnedSDKDrain|TestDriverDrainThroughPinnedSDKAndPollHook|TestBaseline.*(Acquire|SessionOpen|Snapshot)' -count=1 -timeout=300s
```

The focused normal run passed in 2.358s and the focused race run passed in
4.700s. The full package gates also passed:

```text
cd experiments/g01-scaleset
GOWORK=off GOTOOLCHAIN=go1.26.8 go test ./livecanary -count=1 -timeout=300s
GOWORK=off GOTOOLCHAIN=go1.26.8 go test -race ./livecanary -count=1 -timeout=300s
GOWORK=off GOTOOLCHAIN=go1.26.8 go vet ./livecanary
cd ../..
bash scripts/gofmt.sh check
git diff --check
set -e
if git diff --text | rg -n '(/Users/|/home/|-----BEGIN (RSA|OPENSSH|EC|PRIVATE)|github_pat_[A-Za-z0-9_]+|gh[pousr]_[A-Za-z0-9_]{20,}|Authorization[^\n]{0,20}Bearer[[:space:]]+[A-Za-z0-9._-]{20,})'; then exit 1; fi
```

The full normal package passed in 27.179s, the full race package passed in
40.157s, vet was silent and successful, gofmt/diff were clean, and the
secret/private-path scan printed `diff secret/private-path scan passed`.
The repository offline gate was also run from the repository root; it passed
all bounded G01/G02 partitions and printed `offline experiment checks passed:
2 module(s)`.

```text
GOTOOLCHAIN=go1.26.8 bash scripts/check-offline-experiments.sh
```

#### Finding matrix, exact head and rollback

| Finding | Reproduction and resolution | Rollback scope |
|---|---|---|
| [r3996893012](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3996893012) session origin | Red regression above reproduced a poll after a mismatched session-open origin; the pinned regression now quarantines with zero listener polls, and the driver/SDK checks bind the origin to the before snapshot. | Revert the correction source/test commit only; no live close, cleanup or rollback operation was run. |
| [r3996893019](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3996893019) marked poll forwarding | Red regression above reproduced wrong host/path/scheme forwarding; marked target/path/origin/query and foreign-marker boundaries now reject before inner transport while unmarked non-poll requests retain prior behavior. | Revert the correction source/test commit only; no network or runner rollback was run. |

The source/test correction is commit
`cbf711cb582ccd6a68eca13312697be21cea580a`; it is the exact implementation
head before this evidence-only follow-up commit and is independently
recoverable with `git revert --no-edit cbf711cb582ccd6a68eca13312697be21cea580a`.
The evidence-only follow-up commit is immediately on that head. The unresolved live gap is the
coordinator-owned exact-head Codex review, required CI and maintainer-authorized
live G01 gate. No live GitHub App, runner, workflow, Docker/Lima, Keychain,
launchd, credential, cleanup or canary operation was performed.

### Historical exact-head blocker corrections (snapshot `1eb48afb...`; published correction `5d715c486...`)

The following entries preserve the historical chronology of the blocker
corrections. The independent probes targeted source snapshot
`1eb48afb50ffbb10b42d07181f16153df1c494eb`; the corresponding correction was
published at full commit
`5d715c486c959ae67ca615c4eaea3e7e89ede556`, with subsequent replay and
capacity snapshots `f5020e8de32f8641e3aa80dfbc3e74e108277197`,
`1769da60bfbfb261f6e08d862465975e733cc176` and
`3a18028efec69eccfc40879a6f845c37d399e65f`. These entries identify historical
snapshots and do not describe the present branch.

The maintainer approval recorded at [Issue #71 comment](https://github.com/1XP-AI/gh-runnerd/issues/71#issuecomment-5603758575) is limited to the original Issue #71 chronology gap: because the original meaningful pre-implementation behavioral red could not be recovered, independent test-only probes may be run against immutable historical snapshots to diagnose that already-implemented behavior. This historical exception does not waive TDD for this follow-up or any future change: each new implementation correction still requires a meaningful failing test before the fix; retrospective archive reproductions are diagnostic evidence only and must not be presented as pre-implementation red or substituted for future TDD.

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

The historical red-first correction adds phase-local replay binding and the
withdrawn-poll runner-partition fence. A drain phase records its created
scale-set ID, and its journal-assigned event sequence becomes the only valid
`Drain.Sequence`; replay requires exactly one active phase, the created set ID
in both snapshots, and a later matching observation. Both polls and the final
snapshot compare only `registered`, `busy` and `idle` runner counters, so job
counters remain free to change.

The focused historical correction and positive controls were run from the experiment
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
also passed. These remain offline tests only; that correction is published in
`5d715c486c959ae67ca615c4eaea3e7e89ede556`.

### Missing drain-phase SetID correction (historical snapshot; published in `5d715c486...`)

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

### Security F2 owned-idle prerequisite correction (historical snapshot; published in `5d715c486...`)

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
new meaningful red-first regression for the then-reviewed correction snapshot.

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

### PR72 residual replay-contract correction (security F1–F3 / protocol P1–P2)

The exact-head security and protocol reviews of
`f5020e8de32f8641e3aa80dfbc3e74e108277197` identified residual
replay gaps: missing, single, out-of-order, or post-completion snapshot stages
could discharge a drain phase; the final observation was not correlated to the
durable before/after records; and same-ID foreign set/runner metadata was
accepted. The existing harness set `workObserved=true` for a drain observation,
so the demonstrated histories remained blocked from destructive cleanup, but
they incorrectly removed the phase-local uncertainty fence and could affect
later non-cleanup authorization.

Meaningful red probes were added before the implementation change and run
against the current exact head with real `FileJournal` close/reopen boundaries:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go test ./livecanary -run '^TestReplayContract' -count=1 -v
```

The run failed for missing before/after stages, an extra post-completion
snapshot, a final runner mismatch, and foreign metadata sharing the phase SetID.
The red assertions required uncertainty after reopen; the old implementation
returned `uncertain:false` for those malformed histories.

The minimal correction records explicit `before`/`after` roles on the two
`observe-runner` results, requires exactly one valid ordered pair, and requires
the final observation to follow the after result and match both durable records
for set identity, runner tuple, and registered/busy/idle partition. Job
counters remain excluded from that equality, so legitimate after-snapshot
counter changes remain accepted. A completed or interrupted phase rejects any
later drain snapshot, and production replay derives the expected set/runner
identity from the current approval rather than trusting candidate metadata.

The correction coverage includes complete histories and every crash prefix,
missing/extra/outside-phase stages, swapped/substituted runners, stable-set
ownership, malformed phase identity/sequence, unrelated reservations and
unknown/work fences, the real pinned-SDK/FileJournal path, and cleanup fencing.
The focused green and bounded race results are recorded below with the final
verification commands. Rollback is a focused revert of this correction's
source/test/evidence commit, retaining the current journal and owned resources
for inspection; do not reset, erase, replay, or run live cleanup.

The bounded verification commands completed successfully after the correction:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go test ./livecanary -run 'TestReplayContract|TestReplayDrain|TestFileJournal.*Drain|TestSecurityReview.*Drain|TestDrainObservedAllowsNextPollJobCounterChanges|TestDrainListenerRejectsContradictoryWithdrawnPollRunner|TestDriverDrainThroughPinnedSDKAndPollHook' -count=1 -v
GOTOOLCHAIN=go1.26.8 go test -race ./livecanary -run 'TestReplayContract|TestReplayDrain|TestFileJournal.*Drain|TestSecurityReview.*Drain|TestDrainObservedAllowsNextPollJobCounterChanges|TestDrainListenerRejectsContradictoryWithdrawnPollRunner|TestDriverDrainThroughPinnedSDKAndPollHook' -count=1
GOTOOLCHAIN=go1.26.8 go vet ./livecanary
gofmt -l livecanary/baseline_journal.go livecanary/drain.go livecanary/drain_driver.go livecanary/drain_followup_test.go livecanary/drain_test.go livecanary/driver.go livecanary/journal.go livecanary/preparation.go livecanary/replay_contract_red_test.go livecanary/security_review_extra_test.go livecanary/sdk_integration_test.go
cd ../..
git diff --check
bash scripts/gofmt.sh check
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go test ./livecanary -count=1
cd ../..
bash scripts/check-offline-experiments.sh
```

The focused normal suite, focused race suite, vet, formatting, diff, and final
full `livecanary` package run passed; race emitted no report and formatting/diff
checks emitted no diagnostics. The offline experiment check also passed for the
scaleset, livecanary, and liveworker modules. These remain offline fixture
checks only.

### Codex r3972112659 capacity-ordinal correction

The exact-head P1 finding is [Codex r3972112659](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3972112659): `drainClient.GetMessage` accepted either capacity `0` or `1` for either of its two bounded calls. A malformed first poll could therefore reach the SDK with withdrawn capacity, and a malformed second poll could reach the SDK with capacity still set to `1`; the existing two-poll fence did not establish the required `1 -> 0` transition.

The meaningful red regression was added before the implementation and run against the current exact head:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go test ./livecanary -run '^TestDrainRejectsCapacityOrdinalBeforeInnerEffects$' -count=1
```

It failed as expected: the first capacity-`0` call returned `<nil>` after reaching the inner session, and the rejected second capacity-`1` call left the inner poll count at `2` (with ACK/acquisition still at zero only because the later message fence stopped those effects). The regression controls also cover first capacity `-1` and `2`, a valid `1 -> 0` sequence, and a third call; rejected calls assert unchanged inner poll, ACK, and acquisition counts.

The minimal green correction validates the ordinal under the existing client mutex before the inner SDK call: poll one must use capacity `1`, poll two must use capacity `0`, and any third poll or wrong capacity returns `ErrQuarantine` without advancing the ordinal or invoking the inner session. The existing pinned SDK/FileJournal test continues to exercise the legitimate `1 -> 0` HTTP header sequence, and no replay, persistence, cleanup, ownership, reservation, or unrelated fence behavior changed.

Bounded verification completed successfully:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go test ./livecanary -run 'TestDrainRejectsCapacityOrdinalBeforeInnerEffects|TestDrainListenerWithdrawsWhilePollResponseIsHeld|TestDriverDrainThroughPinnedSDKAndPollHook' -count=1 -v
GOTOOLCHAIN=go1.26.8 go test -race ./livecanary -run 'TestDrainRejectsCapacityOrdinalBeforeInnerEffects|TestDrainListenerWithdrawsWhilePollResponseIsHeld|TestDriverDrainThroughPinnedSDKAndPollHook' -count=1
GOTOOLCHAIN=go1.26.8 go vet ./livecanary
gofmt -l livecanary/drain.go livecanary/drain_test.go
git diff --check
GOTOOLCHAIN=go1.26.8 go test ./livecanary -count=1
```

The focused normal suite, focused race suite, pinned SDK path, vet, formatting,
diff, and one full `livecanary` package run passed; the formatting and diff
commands emitted no diagnostics. All checks were offline fixture tests; no live
runner, workflow, credential, cleanup, or external operation was performed.
Rollback is a focused revert of this correction's source/test/evidence commit,
retaining the current journal and owned resources for inspection; do not reset,
erase, replay, or run live cleanup.

### Exact-head follow-up: poll cursor and embedded wire identity

Three new exact-head Codex findings were addressed from reviewed head
`3a18028efec69eccfc40879a6f845c37d399e65f`:

* [P2 durable chronology](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3972526860)
  — the historical evidence text incorrectly described an obsolete local
  tree. The historical section above now names the old source
  snapshot and the published correction
  `5d715c486c959ae67ca615c4eaea3e7e89ede556`, preserves the initial Issue #71
  chronology exception described above, and contains no personal machine
  path.
* [P1 poll cursor binding](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3972526869)
  — the first `GetMessage` must use `last=0`; the second must use the first
  acknowledged message ID, and both checks occur before the inner SDK call.
  A no-message first poll cannot create a fake future-cursor empty observation.
* [P1 embedded-body identity](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3972526881)
  — outer-envelope and JSON-encoded body fields are now decoded strictly,
  including case-folded duplicate-key rejection, and bounded decoded job
  facts must match the SDK message before `VerifyRun`, ACK or acquisition.

The meaningful red was run first against the actual pinned
`github.com/actions/scaleset v0.4.0` loopback HTTP fixture:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go test ./livecanary -run 'TestPinnedSDKDrain(RejectsAmbiguousEmbeddedJobIdentityBeforeEffects|BindsPollCursorBeforeInnerCall)' -count=1 -v
```

It exited 1 before the correction. The case-folded owner duplicate was
accepted as an observed message and reached ACK/acquisition; the wrong first
cursor and wrong second cursor both reached the inner SDK poll instead of
being quarantined. This red used atomic poll/ACK/acquisition counters in the
fixture, so it exercised the effect boundary rather than only a pure parser.

The minimal correction uses the existing strict adapter parser at the
transport boundary. It retains only bounded structured `baselineBatch` facts
in the poll hook, clears the ephemeral body bytes, and journals no raw body or
SDK error. The journaled client compares those facts with the decoded SDK
message immediately after the inner poll returns and before `VerifyRun`; the
listener wrapper validates cursor/capacity/ACK state before invoking that inner
poll. Malformed, exact-duplicate, case-folded-duplicate and wrong-cursor
controls quarantine with zero forbidden effects, while the legitimate exact
SDK path still performs exactly one old poll, ACK, acquisition and withdrawn
poll.

The read-only security archive also reported a same-name foreign numeric
runner ID that cannot be rejected without a trusted approved runtime ID. No
static ID was invented in this correction. The feasible persisted-evidence
gap was separately closed by requiring `Event.ID` to equal
`DrainSnapshot.Runner.ID`; the mismatch is covered by
`TestSecurityReviewDrainSnapshotEventIDMustMatchRunner` through a real
FileJournal append/rejection check.

The source/test correction is commit
`668c361578c7cec749a650e452b85fd0ecf8e5ae`. Green verification completed as
follows:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go test ./livecanary -run '^(TestPinnedSDKDrainRejectsAmbiguousEmbeddedJobIdentityBeforeEffects|TestPinnedSDKDrainBindsPollCursorBeforeInnerCall|TestPinnedSDKDrainMatchesWireBeforeVerifyRun|TestDriverDrainThroughPinnedSDKAndPollHook|TestDrainListenerWithdrawsWhilePollResponseIsHeld|TestDrainListenerRejectsWithdrawnPollPhysicalRetry|TestDrainListenerRejectsContradictoryWithdrawnPollRunnerPartition|TestDrainListenerNoMessageIsInconclusive|TestDrainRejectsCapacityOrdinalBeforeInnerEffects)$' -count=1 -v -timeout=180s
GOTOOLCHAIN=go1.26.8 go test -race ./livecanary -run '^(TestPinnedSDKDrainRejectsAmbiguousEmbeddedJobIdentityBeforeEffects|TestPinnedSDKDrainBindsPollCursorBeforeInnerCall|TestPinnedSDKDrainMatchesWireBeforeVerifyRun|TestDriverDrainThroughPinnedSDKAndPollHook|TestDrainListenerWithdrawsWhilePollResponseIsHeld|TestDrainListenerRejectsWithdrawnPollPhysicalRetry|TestDrainListenerRejectsContradictoryWithdrawnPollRunnerPartition|TestDrainListenerNoMessageIsInconclusive|TestDrainRejectsCapacityOrdinalBeforeInnerEffects|TestSecurityReviewDrainSnapshotEventIDMustMatchRunner)$' -count=1 -v -timeout=180s
GOTOOLCHAIN=go1.26.8 go vet ./livecanary
gofmt -l livecanary/drain.go livecanary/drain_driver.go livecanary/drain_followup_test.go livecanary/drain_test.go livecanary/driver.go livecanary/journal.go livecanary/sdk_integration_test.go livecanary/security_review_extra_test.go
git diff --check
GOTOOLCHAIN=go1.26.8 go test ./livecanary -count=1
```

All listed commands passed; the race run emitted no report and formatting and
diff checks emitted no diagnostics. The full package run was the single broad
`livecanary` verification for this follow-up; all HTTP traffic stayed inside
the bounded loopback fixture and no live runner, workflow, credential, cleanup
or other external operation was performed. To roll back the source/test
correction, use `git revert --no-edit
668c361578c7cec749a650e452b85fd0ecf8e5ae` on the exact branch, retaining any
current journal and owned resources for inspection; do not reset, erase,
replay or run live cleanup.

### Exact-head follow-up: physical poll and strict remote facts

The six latest exact-head Codex findings were reproduced from
`4d9043c456a957be2dd5db367f6d5d636483501a` before implementation and are
tracked at [physical poll capacity/header/cursor](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3972911079), [withdrawn 202 body](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3972911049), [cancel-before-release ordering](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3972911070), [strict VerifyRun fields](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3972911082), [strict acquirejobs count/value](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3972911056), and [strict scale-set snapshot facts](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3972911063).

The meaningful red ran against the real pinned `github.com/actions/scaleset v0.4.0` loopback fixture before the corresponding source corrections:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go test ./livecanary -run '^TestPinnedSDKDrain(RejectsPhysicalPollMutationBeforeInner|RejectsWithdrawnPollBodyBeforeAbsent|RequiresCompletePollStatsBeforeVerifyRun|VerifyRunRejectsAmbiguousWireFieldsBeforeEffects|AcquisitionRequiresStrictWireResponse|SnapshotsRequireStrictWireFacts)$' -count=1 -v -timeout=180s
```

It exited 1 as intended. Physical header and cursor mutations were promoted
and reached the second poll; a withdrawn 202 carrying a complete message body
was classified as absent; incomplete outer statistics crossed `VerifyRun`; and
lossy SDK decoding accepted ambiguous exact/case-fold duplicate fields in
VerifyRun, acquisition, and snapshot responses (with the matrix also covering
malformed, null, missing, and contradictory values, some of which already
failed closed). The deterministic cancellation test was added red-first and
then fixed to require cancel before response release and listener join; no
remote effect is authorized by that helper.

The correction inventories each evidence-bearing remote fact at its adapter
boundary and reuses the existing strict readers. Poll requests now compare the
SDK header and cursor against the callback ordinal and prior strict message ID,
using token-bearing query values only ephemerally; poll bodies distinguish an
unambiguous no-message wire shape from a lossy SDK nil; complete strict poll
statistics are checked before `VerifyRun`; VerifyRun, acquisition, and
scale-set snapshots compare strict bounded wire facts before any later effect.
An ambiguous post-acquire response records the effect as unknown through the
existing journal path, retains the owned reservation/uncertainty, and cannot
produce an observed drain. ACK-before-acquisition order, pinned SDK behavior,
and no raw credential/body/journal payload retention remain unchanged.

Green verification for the source/test correction was:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go test ./livecanary -run '^(TestPinnedSDKDrainRejectsPhysicalPollMutationBeforeInner|TestDrainCancellationStopsBeforeReleasingHeldResponse|TestPinnedSDKDrainRejectsWithdrawnPollBodyBeforeAbsent|TestPinnedSDKDrainRequiresCompletePollStatsBeforeVerifyRun|TestPinnedSDKDrainVerifyRunRejectsAmbiguousWireFieldsBeforeEffects|TestPinnedSDKDrainAcquisitionRequiresStrictWireResponse|TestPinnedSDKDrainSnapshotsRequireStrictWireFacts|TestDrainListenerWithdrawsWhilePollResponseIsHeld|TestDrainPollHookRequiresOneSuccessfulWritePerPoll|TestDriverDrainThroughPinnedSDKAndPollHook)$' -count=1 -v -timeout=180s
GOTOOLCHAIN=go1.26.8 go test -race ./livecanary -run '^(TestPinnedSDKDrainRejectsPhysicalPollMutationBeforeInner|TestDrainCancellationStopsBeforeReleasingHeldResponse|TestPinnedSDKDrainRejectsWithdrawnPollBodyBeforeAbsent|TestPinnedSDKDrainRequiresCompletePollStatsBeforeVerifyRun|TestPinnedSDKDrainVerifyRunRejectsAmbiguousWireFieldsBeforeEffects|TestPinnedSDKDrainAcquisitionRequiresStrictWireResponse|TestPinnedSDKDrainSnapshotsRequireStrictWireFacts|TestDrainListenerWithdrawsWhilePollResponseIsHeld|TestDrainPollHookRequiresOneSuccessfulWritePerPoll|TestDriverDrainThroughPinnedSDKAndPollHook)$' -count=1 -timeout=180s
GOTOOLCHAIN=go1.26.8 go vet ./livecanary
gofmt -l livecanary/drain.go livecanary/drain_driver.go livecanary/observer_http.go livecanary/sdk.go livecanary/baseline_message.go livecanary/baseline_wire.go livecanary/baseline_listener.go livecanary/drain_test.go livecanary/sdk_integration_test.go livecanary/drain_p1_followup_test.go
git diff --check
GOTOOLCHAIN=go1.26.8 go test ./livecanary -count=1
```

The focused normal/race runs, vet, formatting, diff check, and one final full
`livecanary` run passed; race emitted no report and formatting/diff emitted no
diagnostics. The source/test correction is intentionally offline and makes no
live runner, workflow, credential, Docker/Lima, Keychain, launchd, cleanup or
GitHub write operation. Rollback is a focused `git revert --no-edit` of the
source/test correction (and its documentation commit, if separate), retaining
the current journal, owned reservation and uncertainty for inspection; never
reset, erase, replay or run live cleanup. Historical
`5d715c486c959ae67ca615c4eaea3e7e89ede556` and its later consolidated snapshots
`f5020e8de32f8641e3aa80dfbc3e74e108277197`,
`1769da60bfbfb261f6e08d862465975e733cc176`, and
`3a18028efec69eccfc40879a6f845c37d399e65f` remain unchanged, and this
evidence section contains no personal machine paths or raw secret-bearing
payloads.

### Exact-head follow-up: SDK drain response and transport boundary closure

The four latest exact-head Codex P1 findings were read from the PR review
record with `gh` against the immutable baseline
`9904048a8250f2445f5fee4c2141b3f9f1642e3a` (the local review wrapper was not
present, so the equivalent `gh api` detail/all reads were used):

```text
gh api --paginate repos/1XP-AI/gh-runnerd/pulls/72/comments --jq '.[] | [.id,.path,.line,.html_url] | @tsv'
gh api repos/1XP-AI/gh-runnerd/pulls/comments/3973406578 --jq '{html_url,path,line,body}'
gh api repos/1XP-AI/gh-runnerd/pulls/comments/3973406571 --jq '{html_url,path,line,body}'
gh api repos/1XP-AI/gh-runnerd/pulls/comments/3973406564 --jq '{html_url,path,line,body}'
gh api repos/1XP-AI/gh-runnerd/pulls/comments/3973406553 --jq '{html_url,path,line,body}'
gh api --paginate repos/1XP-AI/gh-runnerd/issues/72/comments --jq '.[] | [.id,.html_url] | @tsv'
```

The findings are [non-EOF poll read error](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3973406578), [ACK physical DELETE binding](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3973406571), [runner snapshot decoding](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3973406564), and [session-open decoding](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3973406553). The historical TDD exception remains only the original [Issue #71 authorization](https://github.com/1XP-AI/gh-runnerd/issues/71#issuecomment-5603758575); it does not waive this follow-up or any future correction.

Meaningful red regressions were added and run before implementation against
the real pinned `github.com/actions/scaleset v0.4.0` loopback fixture:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go test ./livecanary -run 'TestPinnedSDKDrainRejectsNonEOFPollReadError|TestPinnedSDKDrainBindsACKToPhysicalDelete|TestPinnedSDKDrainRejectsAmbiguousRunnerSnapshot|TestPinnedSDKDrainRejectsAmbiguousSessionResponse' -count=1 -v
```

The command exited 1 as intended. The old code reported known poll facts after
a complete JSON body followed by a non-EOF read error; accepted a mutated ACK
DELETE; accepted duplicate runner-set identity; and accepted duplicate session
identity/queue fields. The red assertions exercised the pinned SDK and loopback
wire path, used effect counters where applicable, and did not disclose private
response values.

The minimal source/test correction is commit
`19f53d4e7af9578c7719c7897aa032124df6df6b`. `drainObservedBody` now records a
non-EOF read failure and clears all derived facts, so a lossy SDK success cannot
promote the response to known. The existing bounded `baselineWireCapture`
adapter is reused for session-open and ACK; session-open validates canonical
session identity, owner, nested set, complete statistics, and exact ephemeral
queue URL before assigning the poll hook, while ACK requires one exact physical
`DELETE` for the captured queue and message ID with status 204. A runner
equivalent uses the same strict duplicate-key decoder and compares the bounded
count/value identity to the pinned SDK result; an ambiguous runner or session
response is rejected before downstream effects, and a failed session-open
leaves the created remote session unclosed for quarantine inspection.

The complete evidence-bearing drain inventory is now:

| Boundary | Bounded evidence and gate |
| --- | --- |
| Before/after scale-set snapshot | `set-observe` strict bounded identity, labels, update fence, statistics, status 200, and SDK comparison. |
| Before/after runner lookup | `runner-observe` strict count/value (`nil` only for an exact empty list) and exact ID/name/scale-set comparison. |
| Session creation | `session-open` strict identity, nested set/statistics and queue URL comparison; queue URL and token are ephemeral and never journaled. |
| Old and withdrawn polls | Exact queue target, cursor, capacity and physical-write ordinal; bounded body/statistics/job facts; non-EOF body errors remain unknown. |
| VerifyRun | Existing strict bounded REST reader rejects read errors, duplicate keys and mismatched approved run fields before effects. |
| ACK | Exact captured queue plus message ID, one physical DELETE and status 204 before recording success. |
| Acquisition | Existing strict bounded `acquire` reader compares the one-shot request, status 200, count and IDs. |
| Session close | SDK close remains a status-only 204 effect; the response-budget transport blocks refresh PATCH, and no response body is decoded or persisted. |

No remaining SDK lossy JSON decode or transport read-error bypass was found in
the evidence-bearing set, runner, session, poll, VerifyRun or acquisition
boundaries. The session-close response carries no evidence-bearing body and is
not used to infer drain state; production persistence/reconciliation remains
out of scope. Unknown poll/session/ACK states retain the existing journal
reservation and quarantine markers, including when a remote session was
created before the uncertainty was discovered; ACK-before-acquisition remains
unchanged. Prior six P1 corrections and replay/ordinal fixes remain in place.

Green verification was:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go test ./livecanary -run 'TestPinnedSDKDrainRejectsNonEOFPollReadError|TestPinnedSDKDrainBindsACKToPhysicalDelete|TestPinnedSDKDrainRejectsAmbiguousRunnerSnapshot|TestPinnedSDKDrainRejectsAmbiguousSessionResponse' -count=1 -v
GOTOOLCHAIN=go1.26.8 go test ./livecanary -run '^(TestPinnedSDKDrain|TestDriverDrainThroughPinnedSDKAndPollHook|TestDrain|TestSecurityReview.*Drain|TestReplay.*Drain|TestFileJournal.*Drain)' -count=1 -v -timeout=180s
GOTOOLCHAIN=go1.26.8 go test -race ./livecanary -run '^(TestPinnedSDKDrain|TestDriverDrainThroughPinnedSDKAndPollHook|TestDrain|TestSecurityReview.*Drain|TestReplay.*Drain|TestFileJournal.*Drain)' -count=1 -timeout=180s
GOTOOLCHAIN=go1.26.8 go vet ./livecanary
cd ../..
bash scripts/gofmt.sh check
git diff --check
bash scripts/check-offline-experiments.sh
```

All commands passed: the four pinned-SDK regressions, wider normal suite,
wider race suite, vet, formatting, diff check, and both-module offline gate.
The tests used only bounded loopback fixtures; no live GitHub workflow,
runner, App credential, Keychain, launchd, Docker/Lima or cleanup operation was
performed. Rollback is a focused normal `git revert --no-edit
19f53d4e7af9578c7719c7897aa032124df6df6b` plus a separate revert of this
documentation append if needed; retain any journal, reservation and
uncertainty for inspection, and never reset, erase, replay or run live cleanup.

### Exact-head follow-up: runner metadata and withdrawal completion

The current-head Codex detail was read against the immutable source baseline
`88d37abea8ba4d6b793c849b38bcf797f2dbb503` with the installed review wrapper
using the separated repository argument:

```text
bash /path/to/codex-review.sh detail 72 --repo 1XP-AI/gh-runnerd
```

The actionable findings were [P1 unrelated runner metadata](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3973755454)
and [P1 withdrawal completion race](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3973755448).

The new regressions were added before the source correction and run against
that exact baseline at `2026-09-09T23:08:11Z` UTC:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go test ./livecanary -run 'TestPinnedSDKDrainAcceptsUnrelatedRunnerMetadata|TestDrainListenerDoesNotReleaseBeforeWithdrawalCompletes|TestDrainListenerCancellationWhileWithdrawalBlockedDoesNotDeadlock' -count=1 -v -timeout=30s
```

The command exited 1 with sanitized outcomes: the runner metadata response was
quarantined; the deterministic blocked-callback response race observed `ack`
before withdrawal completion; and the cancellation case completed without a
deadlock. No response body, token, URL or raw SDK error was recorded.

The follow-up source/test correction was then appended as
`64c6fce0f9f86ece48a7b28ee51b1aef9774f2cb`, with no reset, rebase, amend or
force operation. `decodeBaselineRunner` now retains the recursive
case-folded duplicate-key guard while using ordinary `json.Unmarshal` for
bounded `count`/`value` facts, so unrelated status/version metadata is
tolerated without relaxing required-field or count/value identity bounds.
The drain hook records `withdrawalCompleted` only after the capacity callback
returns, closes a separate first-callback completion signal, requires that
fact for a proven boundary, and waits for it before releasing a response that
arrived first; cancellation still force-releases and joins the listener so an
in-progress callback cannot deadlock cleanup. ACK-before-acquisition,
non-EOF/duplicate/retry guards, unknown-state retention and the unproven
server-receipt category remain unchanged.

Post-correction focused verification on that exact SHA was:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go test ./livecanary -run '^(TestPinnedSDKDrain.*|TestDriverDrainThroughPinnedSDKAndPollHook|TestDrainListenerDoesNotReleaseBeforeWithdrawalCompletes|TestDrainListenerCancellationWhileWithdrawalBlockedDoesNotDeadlock|TestDrainPollHookRejectsDuplicatePhysicalWrites|TestDrainPollHookRequiresOneSuccessfulWritePerPoll)$' -count=1 -timeout=180s
```

At `2026-09-09T23:13:11Z` UTC this passed in 0.509s, covering the new
metadata-positive and blocked-race paths plus existing pinned-SDK positive and
negative runner/session/ACK/non-EOF and capacity/retry checks.

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go test -race ./livecanary -run '^(TestPinnedSDKDrain.*|TestDriverDrainThroughPinnedSDKAndPollHook|TestDrainListenerDoesNotReleaseBeforeWithdrawalCompletes|TestDrainListenerCancellationWhileWithdrawalBlockedDoesNotDeadlock|TestDrainPollHookRejectsDuplicatePhysicalWrites|TestDrainPollHookRequiresOneSuccessfulWritePerPoll)$' -count=1 -timeout=180s
```

At `2026-09-09T23:13:12Z` UTC this passed in 1.614s with no race report.
The relevant package gate then passed at `2026-09-09T23:13:20Z` UTC:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go test ./livecanary -count=1 -timeout=180s
GOTOOLCHAIN=go1.26.8 go vet ./livecanary
bash ../../scripts/gofmt.sh check
git -C ../.. diff --check
```

The full `livecanary` package passed in 28.409s; vet, formatting and diff
checks emitted no diagnostics. These remain offline loopback tests only: no
live workflow, runner, credential, App, Keychain, launchd, Docker/Lima or
cleanup operation was performed. Rollback is a focused normal
`git revert --no-edit 64c6fce0f9f86ece48a7b28ee51b1aef9774f2cb` (and a
separate documentation revert if desired), retaining journal, reservation and
uncertainty for inspection.

### Exact-head follow-up: queue destination host boundary

The additional security P1 sent at `2026-09-09T23:06:52Z` UTC identified that
`validDrainQueueURL` accepted the control-plane `api.github.com` origin as a
session `MessageQueueURL`. Because the pinned SDK uses that URL for queue GET
and ACK DELETE requests with the session bearer, accepting the API origin could
send queue credentials outside the exact approved Actions destination. The
required policy is HTTPS plus an exact approved `Approval.ActionsHosts`
host/port pair; the control-plane API host remains permitted only by the
general control-plane transport path and is not reused for queue validation.

The new pinned `OpenDrainSession` regression was added and run before the
production correction at `2026-09-09T23:21:25Z` UTC, with source baseline
`cb2c1ba7c58fb5faeab6eeaa42b4564686a9c61c`:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go test ./livecanary -run '^TestPinnedSDKDrainRequiresApprovedHTTPSQueueHost$' -count=1 -v -timeout=180s
```

The command exited 1 as required. The approved HTTPS fixture case passed, while
the API control-plane host, an unapproved Actions-like host and a plain HTTP
host were each accepted by the old production code; the failure also showed
that the old path would return a session instead of rejecting before assigning
the poll target. No raw response body, bearer, private path or SDK error was
recorded in this evidence.

The minimal source/test correction was then appended and pushed as
`b3d45cb648965913090dcb08c3674341f76f483a`. `validDrainQueueURL` now requires
HTTPS, a valid bounded port, and exact host/port membership in the supplied
`Approval.ActionsHosts`; a bare approved hostname means the default HTTPS port,
while an explicit fixture host/port is accepted only when that exact pair is
listed. `validDrainSessionWire` performs this check before `OpenDrainSession`
assigns `hook.target`, so rejected queue URLs cannot become poll or ACK
destinations. The pinned loopback fixture now uses a test-only TLS server and
explicitly lists its listener host/port in its fixture approval; production
`Approval.Validate` and the general API-host transport allowlist were not
weakened or reused for queue identity.

Focused post-fix normal verification at `2026-09-09T23:24:36Z` UTC was:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go test ./livecanary -run '^(TestPinnedSDKDrain.*|TestValidDrainQueueURLRequiresExactApprovedHostPort|TestDriverDrainThroughPinnedSDKAndPollHook|TestDrainListenerDoesNotReleaseBeforeWithdrawalCompletes|TestDrainListenerCancellationWhileWithdrawalBlockedDoesNotDeadlock|TestDrainPollHookRejectsDuplicatePhysicalWrites|TestDrainPollHookRequiresOneSuccessfulWritePerPoll)$' -count=1 -timeout=180s
```

The command passed in 0.594s. It covered the approved/rejected queue-host
matrix, unrelated runner metadata acceptance, duplicate/case-folded session
and runner identity rejection, non-EOF poll failure, physical ACK binding,
withdrawn-poll/capacity/cursor/retry fences, and the blocked withdrawal
response race plus cancellation join.

The corresponding race verification at `2026-09-09T23:24:45Z` UTC was:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go test -race ./livecanary -run '^(TestPinnedSDKDrain.*|TestValidDrainQueueURLRequiresExactApprovedHostPort|TestDriverDrainThroughPinnedSDKAndPollHook|TestDrainListenerDoesNotReleaseBeforeWithdrawalCompletes|TestDrainListenerCancellationWhileWithdrawalBlockedDoesNotDeadlock|TestDrainPollHookRejectsDuplicatePhysicalWrites|TestDrainPollHookRequiresOneSuccessfulWritePerPoll)$' -count=1 -timeout=180s
```

The race command passed in 2.097s with no race report. The full relevant package
run passed at `2026-09-09T23:24:55Z` UTC in 27.336s:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go test ./livecanary -count=1 -timeout=180s
```

At `2026-09-09T23:25:31Z` UTC, `GOTOOLCHAIN=go1.26.8 go vet ./livecanary`,
`bash ../../scripts/gofmt.sh check` and `git diff --check` all exited 0 with no
diagnostics. These remain offline loopback tests only; no live workflow,
runner, credential, App, Keychain, launchd, Docker, Lima or cleanup operation
was performed, and no repeated full-repository gate was run.

This correction does not change the unknown-session contract: an ambiguous
session-open remains `uncertain=true`, `sessionID=""`, `reserved=false`, and
`authorizePhase` refuses every subsequent non-inspect phase. No known
reservation or session ID is invented, no production persistence/recovery
redesign is introduced, and ACK-before-acquisition, unknown-state retention,
duplicate/casefold guards and the unproven server-receipt category remain
unchanged.

Rollback is a focused normal `git revert --no-edit
b3d45cb648965913090dcb08c3674341f76f483a` followed by a separate revert of
this documentation append if needed; retain the current journal and any
uncertainty for inspection, and never reset, erase, replay or run live cleanup.

### Exact-head follow-up: acquisition request body and session-close wire binding

The fresh exact-head Codex detail for PR72 was read against immutable baseline
`d85213a2123ed260d9dd1fc01771705bc2b96ae`. It identified [P1 acquisition
request-body binding](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3974007599)
and [P1 session-close DELETE binding](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3974007586).
The review command was the repository's `codex-review` detail path for PR72;
no raw review payload, token or private machine path is included here.

The meaningful red regressions were added before the source correction and run
against the exact baseline with the real pinned `github.com/actions/scaleset
v0.4.0` SDK and an offline TLS loopback fixture:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go test ./livecanary -run '^TestPinnedSDKDrainBindsAcquireToPhysicalRequestBody$' -count=1 -v -timeout=180s
```

At `2026-09-09T23:53:39Z` UTC this exited 1. The valid SDK-array control
passed, while mismatched, duplicate, case-folded, malformed and oversized
request bodies were forwarded and accepted by the old response-only capture;
the read-error control also reached the loopback acquisition handler
(`acquires=1`). The sanitized failing assertion was `want quarantine before
forwarding`, and no request body, bearer, URL or SDK error text was retained.

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go test ./livecanary -run '^TestPinnedSDKDrainBindsSessionCloseToPhysicalDelete$' -count=1 -v -timeout=180s
```

At `2026-09-09T23:53:46Z` UTC this exited 1: both wrong-session and wrong-
scale-set DELETE controls returned SDK `nil` from the permissive loopback
handler, so the old direct `session.Close` path recorded success instead of
retaining uncertainty.

The minimal source/test correction was committed as
`05b5fffa9b6798e20d5454838252dd21de412fdc`. The pinned SDK's actual acquisition
schema is a JSON array of `int64` IDs. The innermost request transport capture
receives the final request after the physical-mutation seam, reads at most the
existing response budget, rejects malformed, duplicate, case-folded, semantic
duplicate, mismatched, oversized and read-error bodies before forwarding, and
replaces a valid body with the same bounded bytes for the real SDK transport;
the forwarding copy is cleared after the synchronous request and on close.
Expected IDs are cloned into the ephemeral capture, while no raw request body,
token or response error is journaled. The response-side acquisition status and
accepted IDs remain required, and ACK-before-acquisition and unknown/replay
guards are unchanged.

Drain session close now reuses the existing `terminal-session-close` exact
wire capture with the approved SetID/session ID and requires one matching
DELETE plus HTTP 204 before `Driver.effect` can persist a successful
`session-close` result. A wrong target that nevertheless receives 204 is
therefore recorded as unknown and leaves the session/reservation fence for
inspection; it is never retried or cleaned up automatically.

Focused green verification completed on the corrected source:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go test ./livecanary -run '^(TestPinnedSDKDrain.*|TestDriverDrainThroughPinnedSDKAndPollHook)$' -count=1 -v -timeout=180s
```

At `2026-09-10T00:02:24Z` UTC this passed in 0.630s, including the valid and
negative request-body matrix, wrong set/session close paths, the existing
pinned-SDK poll/ACK/acquisition/identity/queue controls and the legitimate
observed drain.

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go test -race ./livecanary -run '^(TestPinnedSDKDrain.*|TestDriverDrainThroughPinnedSDKAndPollHook)$' -count=1 -v -timeout=180s
```

At `2026-09-10T00:02:33Z` UTC this passed in 2.019s with no race report. At
`2026-09-10T00:02:42Z` UTC, `GOTOOLCHAIN=go1.26.8 go vet ./livecanary`,
relevant-file `gofmt -l`, and `git diff --check` all exited 0; gofmt emitted no
paths. Package-wide `livecanary` normal and race runs also passed before this
commit (28.241s and 40.834s respectively). All checks remained offline TLS
loopback evidence: no live runner, workflow, credential, App, Keychain,
launchd, Docker/Lima or cleanup operation was performed.

The remaining gap is that a client-side request write and a matching response
still do not claim server receipt or an atomic remote drain barrier. A close
target mismatch may have changed remote state before it became unknown, so the
durable fence intentionally retains the session and requires operator
inspection. G01's full live evidence gate and parent goal remain open.

Rollback is a focused normal `git revert --no-edit
05b5fffa9b6798e20d5454838252dd21de412fdc` followed by a separate revert of
this evidence append if needed; retain the current journal, reservation and
uncertainty, and never reset, erase, replay or run live cleanup.

### Exact-head follow-up: acquisition preflight, session-close origin, and request-body lifetime

This follow-up was developed from immutable baseline
`116beda04dc2bf69280cdefc4de4ef2fef397ef3`. Fresh Codex detail for PR72
identified [acquisition target preflight](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3974234277)
and [session-close endpoint origin binding](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3974234284)
as P1 findings. The review detail was read through the repository wrapper; no
raw review payload, token, private path, request body or SDK error was retained.

The meaningful normal regressions were added before the source correction. At
`2026-09-10T00:32:14Z` UTC, this command was run against the immutable baseline:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go test ./livecanary -run 'TestBaselineAcquireTargetMismatchStopsBeforeInner|TestBaselineAcquireForwardingBodySurvivesAsyncRoundTripClose|TestPinnedSDKDrainBindsSessionCloseToSessionOpenOrigin' -count=1 -v -timeout=180s
```

It exited 1: all four acquisition host, scale-set, query and endpoint
mutations reached the inner transport; the asynchronous forwarding control
observed an empty body after the old wrapper returned; and the wrong allowed
Actions origin was not quarantined. The failing assertions were sanitized and
no remote payload, bearer, private path or raw SDK error was recorded.

The additional body-lifetime regression isolated the standard RoundTripper
contract race after the target/origin corrections were present. At
`2026-09-10T00:46:54Z` UTC, the new body mutex was temporarily removed from the
worktree while the target/origin corrections remained, and this race command
exited 1 with a `bytes.Reader.Reset`/`Read` data race:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go test -race ./livecanary -run '^TestBaselineAcquireForwardingBodyConcurrentReadCloseIsSafe$' -count=1 -v -timeout=180s
```

This was a focused isolation red, not a claim that the entire pristine
baseline had been rerun. The mutex was restored immediately; no reviewer
successfully modified the owned tests or source.

Source/test commit `1d25d31061b0be6dcad50a67a7263c5157b69a6f` makes the minimal
corrections. Acquisition-shaped non-target requests are rejected before the
inner transport, while bootstrap requests outside that shape remain
untouched; the valid acquisition target still has strict host, path, method,
query and ID/body checks. Session-open capture now validates the actual HTTPS
scheme/host/port against the approved runtime identity and records that exact
origin; terminal session close requires the same origin while retaining the
legitimate dynamic Actions API base path. The capture transport no longer
closes the replacement body after the inner RoundTripper returns, and the
bounded forwarding body's `Read` and `Close` operations are synchronized for
the asynchronous ownership permitted by `net/http.RoundTripper`. No blanket
response clean-EOF close-error quarantine was added.

Fresh focused normal verification at `2026-09-10T00:50:24Z` UTC was:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go test ./livecanary -run '^(TestBaselineAcquireTargetMismatchStopsBeforeInner|TestBaselineSessionCloseTargetRequiresExactOrigin|TestBaselineAcquireForwardingBodySurvivesAsyncRoundTripClose|TestBaselineAcquireForwardingBodyConcurrentReadCloseIsSafe|TestPinnedSDKDrainRejectsAcquireTargetMutationBeforeFixture|TestPinnedSDKDrainBindsSessionCloseToSessionOpenOrigin|TestPinnedSDKDrainBindsAcquireToPhysicalRequestBody)$' -count=1 -timeout=180s
```

It passed in 0.529s. The corresponding focused race command at
`2026-09-10T00:50:45Z` UTC passed in 1.432s with no race report. Both suites
used the real pinned `github.com/actions/scaleset v0.4.0` SDK and offline TLS
loopback fixtures, including the legitimate dynamic `/tenant/v2/` path and a
wrong-but-otherwise-allowed close origin.

The complete relevant package normal run began at `2026-09-10T00:51:11Z` UTC
and passed in 26.637s:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go test ./livecanary -count=1 -timeout=240s
```

The package-wide race command was also run in the final validation window and
the captured successful result was 39.272s:

```text
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go test -race ./livecanary -count=1 -timeout=300s
```

At `2026-09-10T00:53:42Z` UTC, `GOTOOLCHAIN=go1.26.8 go vet ./livecanary` exited
0. At `2026-09-10T00:53:50Z` UTC, `bash scripts/gofmt.sh check` exited 0;
`git diff --check` also exited 0. No additional root-plus-G01/G02 offline gate
was rerun after the coordinator's throughput direction; those exact broader
gates remain coordinator/CI evidence. All work here stayed offline: no live
runner, workflow, credential, App, Keychain, launchd, Docker, Lima, ScaleSet,
JIT or cleanup operation was performed.

The bounded security adjudication independently confirmed the standard HTTP
early-response/body-truncation race and found no actionable clean-EOF response
close-error contract issue. Final independent delta review, exact-head Codex
review and CI remain coordinator gates; this worker does not claim those gates
are complete. The durable unknown/replay fence remains unchanged whenever a
target or response cannot be proven, and the live G01 evidence gate and parent
goal remain open.

Rollback is a focused normal `git revert --no-edit
1d25d31061b0be6dcad50a67a7263c5157b69a6f` followed by a separate revert of
this evidence append if needed; retain the current journal, reservation and
uncertainty, and never reset, erase, replay or run live cleanup.

### Exact-head follow-up: fail-closed marked session-open target binding

Fresh exact-head Codex detail for PR72 identified [P1 session-open target
pre-forwarding](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3974562078)
on `f08e9f3e0b1b7cb5d028e437160f7afa3a838f00`. The finding covers marked
session-open host, path, scale-set ID, method and query mutations: the old
capture returned success for a non-target request, allowing the innermost
transport to run before `OpenDrainSession` rejected the response and discarded
the remote session identity.

The meaningful red regression was added before the source correction:

```text
cd experiments/g01-scaleset
GOWORK=off GOTOOLCHAIN=go1.26.8 go test ./livecanary -run '^TestBaselineSessionOpenTargetMismatchStopsBeforeInner$' -count=1 -v -timeout=60s
```

It exited 1. The valid session-open and both SDK bootstrap controls passed, while
the wrong-host, wrong-scale-set, wrong-endpoint-path, wrong-method and wrong-
query cases returned nil instead of rejecting before the inner transport; the
counter assertions therefore exposed the pre-forwarding gap. No request body,
token, URL, response error or private path was retained.

The minimal source/test correction adds a narrow session-open candidate check to
the marked capture: requests in the `/runnerscalesets/` route family that fail
the exact host/path/method/query target are rejected before the inner transport.
The two pinned SDK bootstrap POST paths remain unclassified and continue through
the transport, while the valid session-open request remains accepted. Existing
acquisition target preflight, session-close origin binding and asynchronous
request-body lifetime fixes are unchanged.

The batched focused normal verification was:

```text
cd experiments/g01-scaleset
GOWORK=off GOTOOLCHAIN=go1.26.8 go test ./livecanary -run '^(TestBaselineSessionOpenTargetMismatchStopsBeforeInner|TestBaselineAcquireTargetMismatchStopsBeforeInner|TestBaselineSessionCloseTargetRequiresExactOrigin|TestBaselineAcquireForwardingBodySurvivesAsyncRoundTripClose|TestBaselineAcquireForwardingBodyConcurrentReadCloseIsSafe|TestPinnedSDKDrainRejectsAcquireTargetMutationBeforeFixture|TestPinnedSDKDrainBindsSessionCloseToSessionOpenOrigin|TestPinnedSDKDrainBindsAcquireToPhysicalRequestBody|TestPinnedSDKDrainRejectsAmbiguousSessionResponse|TestPinnedSDKDrainRequiresApprovedHTTPSQueueHost|TestDriverDrainThroughPinnedSDKAndPollHook)$' -count=1 -v -timeout=180s
```

This passed. The corresponding focused race command passed with no race report;
the valid session-open/bootstrap controls, all five session-open mutation
boundaries, prior acquisition/origin/body-lifetime regressions and pinned SDK
drain integration all remained green. These are offline loopback tests only;
the full G01 live evidence gate and parent goal remain open.

```text
cd experiments/g01-scaleset
GOWORK=off GOTOOLCHAIN=go1.26.8 go test -race ./livecanary -run '^(TestBaselineSessionOpenTargetMismatchStopsBeforeInner|TestBaselineAcquireTargetMismatchStopsBeforeInner|TestBaselineSessionCloseTargetRequiresExactOrigin|TestBaselineAcquireForwardingBodySurvivesAsyncRoundTripClose|TestBaselineAcquireForwardingBodyConcurrentReadCloseIsSafe|TestPinnedSDKDrainRejectsAcquireTargetMutationBeforeFixture|TestPinnedSDKDrainBindsSessionCloseToSessionOpenOrigin|TestPinnedSDKDrainBindsAcquireToPhysicalRequestBody|TestPinnedSDKDrainRejectsAmbiguousSessionResponse|TestPinnedSDKDrainRequiresApprovedHTTPSQueueHost|TestDriverDrainThroughPinnedSDKAndPollHook)$' -count=1 -timeout=180s
```

This passed in 1.838s with no race report.

Rollback is a focused normal `git revert --no-edit` of this correction commit;
retain the journal, reservation and uncertainty, and never reset, erase, replay,
or run live cleanup.

### Exact-head follow-up: marked ACK and session-close DELETE pre-forward fences

Date: 2026-09-13. This correction started from exact head
`1bb0c878fe97b81cae61f3416cd074670ec2292d` for PR #72. The fresh Codex
findings were [marked ACK DELETE validation](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3997097883)
and [marked session-close DELETE fall-through](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3997097890).
No Project, Issue, goal, dependency, branch-ownership or live-operation state
was changed.

#### Red-first reproductions

The meaningful red-first command ran after adding the behavioral regressions
and before changing production code:

```text
cd experiments/g01-scaleset
GOWORK=off GOTOOLCHAIN=go1.26.8 go test ./livecanary -run '^(TestBaselineMarkedACKMismatchStopsBeforeInner|TestBaselineMarkedACKRequiresIdentityAndOneShotCardinality|TestBaselineMarkedSessionCloseMismatchStopsBeforeInner|TestBaselineMarkedSessionCloseRequiresIdentityAndOneShotCardinality|TestUnmarkedDeletePreservesInnerTransport)$' -count=1 -v -timeout=120s
```

It exited 1 as intended. Wrong ACK message/path and the omitted
`runnerscalesets` session-close route returned nil instead of a pre-forward
`ErrRemote`; duplicate marked ACK and close requests were also accepted by the
old transport boundary, while the unmarked DELETE control passed. The
regressions count inner calls and retain only fixed outcomes/counters; no
request body, bearer, queue URL, response error or private log is recorded.

#### Corrections and boundary evidence

Marked ACK captures now require the exact captured queue URL plus message ID,
canonical expected origin, an approved queue host when an allowlist is
available, a captured tenant prefix, and positive scale-set/session identity
before reserving the one-shot physical request. The request reservation is
made before the inner SDK transport and rejects a second physical DELETE;
`baseline_listener` now carries its session identity into the same marked
capture, and the drain client carries origin, tenant prefix, set/session ID and
approved host metadata into ACK captures. Existing ACK-before-acquisition and
response-status checks remain unchanged.

Marked session-close DELETEs now take a dedicated branch before
`snapshotRequestCandidate`, so a mismatch cannot fall through merely because
its route omits the `runnerscalesets` family. The branch requires exact HTTPS
origin, approved host, tenant prefix, scale-set ID, session ID, method, route,
API-version query and one-shot cardinality before forwarding; unmarked
non-G01 requests retain the prior inner-transport behavior.

The focused green normal command passed in 0.478s:

```text
cd experiments/g01-scaleset
GOWORK=off GOTOOLCHAIN=go1.26.8 go test ./livecanary -run '^(TestBaselineMarkedACKMismatchStopsBeforeInner|TestBaselineMarkedACKRequiresIdentityAndOneShotCardinality|TestBaselineMarkedSessionCloseMismatchStopsBeforeInner|TestBaselineMarkedSessionCloseRequiresIdentityAndOneShotCardinality|TestUnmarkedDeletePreservesInnerTransport|TestPinnedSDKDrainBindsACKToPhysicalDelete|TestPinnedSDKDrainBindsSessionCloseToPhysicalDelete)$' -count=1 -v -timeout=240s
```

The corresponding focused race suite passed in 1.571s with no race
diagnostics. The pinned-SDK controls now assert zero fixture calls for a
rewritten ACK and for wrong session-close session/set/route-family paths; the
direct boundary matrix covers missing/wrong origin, tenant prefix, set/session
identity and duplicate physical requests, while the unmarked control asserts
one forwarded inner call.

The full offline package and related preservation checks passed after the
correction:

```text
cd experiments/g01-scaleset
GOWORK=off GOTOOLCHAIN=go1.26.8 go test ./livecanary -count=1 -timeout=300s
GOWORK=off GOTOOLCHAIN=go1.26.8 go test -race ./livecanary -count=1 -timeout=360s
GOWORK=off GOTOOLCHAIN=go1.26.8 go vet ./livecanary
GOWORK=off GOTOOLCHAIN=go1.26.8 go test -tags=g01_pair_fixture ./livecanary -run '^(TestPairedTerminalActualJournalsFinalize|TestPairedTerminalCompletionCadenceAndReceiptSeparation|TestPairedTerminalFinalResultCapacity|TestPairedTerminalPendingChildCapacity|TestPairedTerminalEligibilityUsesFreshExactFacts|TestPairedTerminalCapturedAcknowledgementCancellation|TestPairedTerminalMissingAcknowledgementsAndPostchecks)$' -count=1 -v -timeout=300s
cd ../..
GOTOOLCHAIN=go1.26.8 bash scripts/check-offline-experiments.sh
bash scripts/gofmt.sh check
git diff --check
```

The normal package exited 0 in 29.692s, the race package exited 0 in 45.666s
with no race diagnostics, vet and the tagged paired-terminal matrix exited 0,
and the two-module offline gate exited 0; format/diff checks were clean. The
exact implementation/test head is
`a0702639247bc1f97f030d3c9adbecdc38af37da`, comprising source correction
`a1f20770dd4f13d3a5979489607447503cf82ff4` and test assertion refinement
`a0702639247bc1f97f030d3c9adbecdc38af37da`.

Rollback is recoverable with focused normal
`git revert --no-edit a0702639247bc1f97f030d3c9adbecdc38af37da a1f20770dd4f13d3a5979489607447503cf82ff4`;
the documentation-only append can be reverted separately. No live GitHub App,
runner, Scale Set, JIT, workflow, Docker/Lima, Keychain, launchd, credential,
network, cleanup or rollback operation was performed. The remaining gap is
the coordinator-owned exact-head Codex review and required CI/live G01 gate;
this worker did not request review or merge.

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
after checking dependent corrections. To roll back only the current queue-host
correction, revert the focused `b3d45cb648965913090dcb08c3674341f76f483a`
source/test/evidence commits while retaining the current journal and owned
resources for inspection; do not reset, erase or replay the journal. No live
cleanup, workflow replay, runner mutation or rollback operation was performed.

### Exact-head correction: captured origin, bootstrap boundaries, and paired close

Date: 2026-09-10. This correction addresses the two independent private review
findings on exact head c0e8a2e: acquisition was bound to the approved-host set
rather than the session-open origin, and the marked session-open classifier
could forward route-family escapes or reject an organization named
"runnerscalesets". It also addresses the paired terminal cleanup finding: the
paired close capture omitted the origin learned during its own session-open, so
the DELETE could occur while the local close receipt remained unknown.

The red-first chronology was:

1. The required c0 paired regression was run first with
   GOWORK=off GOTOOLCHAIN=go1.26.8 go test -tags=g01_pair_fixture ./livecanary
   -run '^TestPairedTerminalCapturedAcknowledgementCancellation$/session$'
   -count=1 -v -timeout=180s. It exited 1 with
   "captured close acknowledgement/history lost"; this is the c0 failure where
   the close DELETE reached the fixture but the paired capture had no origin
   and recorded no close response.
2. After adding the request-boundary regression cases, the focused red command
   was GOWORK=off GOTOOLCHAIN=go1.26.8 go test ./livecanary -run
   '^(TestBaselineSessionOpenTargetMismatchStopsBeforeInner|TestBaselineAcquireTargetRequiresCapturedSessionOrigin|TestBaselineAcquireOriginMismatchStopsBeforeInner)$'
   -count=1 -v -timeout=60s. It exited 1 as expected: malformed-percent,
   semicolon, duplicate, case, delimiter, and omitted-route session-open cases
   were forwarded; the colliding organization bootstrap cases were rejected; a
   second approved acquisition origin and a missing captured origin were
   accepted; and the acquisition origin mismatch reached the inner transport.

The minimal correction then made the following boundaries explicit. RawQuery is
parsed through the error-returning parser and requires the exact key/value set,
rejecting malformed percent escapes, semicolon syntax, duplicates, empty or
unexpected pairs. A marked session-open accepts its exact target or only the
known organization registration-token and runner-registration bootstrap paths
for the approved organization, including the hosted and /api/v3 API prefixes;
all other marked non-target requests reject before the inner transport. The
organization is carried into the capture so an organization slug equal to
"runnerscalesets" remains compatible without restoring substring matching.
Acquisition now copies the exact canonical HTTPS origin captured at session-open
and requires equality with the physical acquisition request while retaining the
approved-host check. The paired listener records that session-open origin, the
paired terminal close refuses an absent origin before invoking the SDK, and the
close capture receives the recorded origin; the drain path copies its hook
origin as well. Redirects, missing origins, and ambiguous origins remain
quarantined, and the asynchronous request-body ownership and error-redaction
behavior remains unchanged.

The first focused green command was
GOWORK=off GOTOOLCHAIN=go1.26.8 go test ./livecanary -run
'^(TestBaselineSessionOpenTargetMismatchStopsBeforeInner|TestBaselineAcquireTargetMismatchStopsBeforeInner|TestBaselineAcquireTargetRequiresCapturedSessionOrigin|TestBaselineAcquireOriginMismatchStopsBeforeInner|TestBaselineSessionCloseTargetRequiresExactOrigin|TestBaselineAcquireTargetIsActionsOnly|TestBaselineAcquireForwardingBodySurvivesAsyncRoundTripClose|TestBaselineAcquireForwardingBodyConcurrentReadCloseIsSafe|TestPinnedSDKDrainRejectsAcquireTargetMutationBeforeFixture|TestPinnedSDKDrainBindsSessionCloseToSessionOpenOrigin|TestPinnedSDKDrainBindsAcquireToPhysicalRequestBody|TestPinnedSDKDrainRejectsAmbiguousSessionResponse|TestPinnedSDKDrainRequiresApprovedHTTPSQueueHost|TestDriverDrainThroughPinnedSDKAndPollHook)$'
-count=1 -timeout=180s; it exited 0. The same expression with go test -race
exited 0 in 1.873s with no race diagnostics.

The tagged required paired matrix was then run with
GOWORK=off GOTOOLCHAIN=go1.26.8 go test -tags=g01_pair_fixture ./livecanary
-run
'^(TestPairedTerminalFinalResultCapacity|TestPairedTerminalPendingChildCapacity|TestPairedTerminalEligibilityUsesFreshExactFacts|TestPairedTerminalCapturedAcknowledgementCancellation|TestPairedTerminalMissingAcknowledgementsAndPostchecks)$'
-count=1 -v -timeout=180s; it exited 0 in 12.247s. The pinned SDK subset
was initially run with
GOWORK=off GOTOOLCHAIN=go1.26.8 go test ./livecanary -run
'^(TestPinnedSDKDrainRejectsAcquireTargetMutationBeforeFixture|TestPinnedSDKDrainBindsAcquireToPhysicalRequestBody|TestPinnedSDKDrainBindsSessionCloseToSessionOpenOrigin|TestPinnedSDKDrainRequiresApprovedHTTPSQueueHost|TestDriverDrainThroughPinnedSDKAndPollHook)$'
-count=1 -v -timeout=180s; that run failed because the explicit bootstrap
allowlist did not yet include the valid /api/v3 prefix. After adding that known
prefix, the same command exited 0 in 0.774s.

The expanded tagged paired command
GOWORK=off GOTOOLCHAIN=go1.26.8 go test -tags=g01_pair_fixture ./livecanary
-run
'^(TestPairedTerminalActualJournalsFinalize|TestPairedTerminalCompletionCadenceAndReceiptSeparation|TestPairedTerminalFinalResultCapacity|TestPairedTerminalPendingChildCapacity|TestPairedTerminalEligibilityUsesFreshExactFacts|TestPairedTerminalCapturedAcknowledgementCancellation|TestPairedTerminalMissingAcknowledgementsAndPostchecks)$'
-count=1 -v -timeout=240s exited 0 in 13.518s, including actual paired
terminal finalize and cleanup. The final untagged focused normal expression
covering the same request boundaries plus all pinned SDK integration cases
exited 0 in 0.509s; its exact fourteen-test expression is recorded in the
correction report. The corresponding final untagged go test -race expression
exited 0 in 1.873s with no race diagnostics. The final tagged paired race
command used the seven-test expression above with go test -race; it exited 0 in
44.187s with no race diagnostics.

Focused verification also passed: GOWORK=off GOTOOLCHAIN=go1.26.8 go vet
./livecanary exited 0; gofmt reported no files for the touched implementation
and test files; and git diff --check exited 0. Tests named above cover the c0
paired close root cause, exact acquisition-origin binding, session-open
host/path/method/query and route-family pre-forward rejection, organization
collision compatibility, strict raw-query parsing, dynamic session-close
paths, physical request-body binding, terminal
capacity/eligibility/cancellation/missing-receipt behavior, and pinned SDK
drain integration. Verification was offline and focused on the permitted
package and tagged fixture; no full root, G01, G02, makecheck, GitHub,
workflow, runner, credential, App, Keychain, launchd, Docker, Lima, live
cleanup, or rollback operation was performed. Independent final review,
exact-head Codex review, CI, and the live G01 evidence gate remain
coordinator-owned, and the parent G01 goal remains active.

### Exact-head follow-up: runtime tenant-prefix binding and session-open cardinality

Date: 2026-09-12 UTC. This correction addresses the fresh exact-head Codex
P1 findings [runtime path-prefix binding](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3996992590)
and [duplicate marked session-open cardinality](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3996992592).

The meaningful red-first command was run before the implementation change:

```text
cd experiments/g01-scaleset
GOWORK=off GOTOOLCHAIN=go1.26.8 go test ./livecanary -run '^(TestBaselineSessionOpenRejectsDuplicateMarkedPOSTBeforeInner|TestPinnedSDKDrainRejectsSnapshotTenantPrefixMismatchBeforeEffects|TestPinnedSDKDrainRejectsSessionTenantPrefixMismatchBeforeListener)$' -count=1 -v -timeout=180s
```

It exited 1. The duplicate marked session-open returned nil on its second
request instead of `ErrRemote`; the same-origin foreign-prefix after-snapshot
and session-open cases returned nil instead of quarantine. The failing cases
also showed the pre-fix request could reach the inner/fixture boundary, while
the existing origin and queue-target controls remained separate green
regressions.

The minimal correction captures the exact private runtime path prefix from the
first approved scale-set snapshot and carries it through the before/after
scale-set and runner snapshots, session-open, listener hook, ACK, acquisition,
and session-close captures. A same-origin different tenant prefix is rejected
at the marked request boundary before inner transport effects; the exact
origin and private approval marker semantics remain unchanged, and the prefix
is never journaled. Marked session-open request cardinality is reserved before
body forwarding, so a second marked POST is rejected before the inner transport
and cannot create a duplicate live session.

The focused normal green command was:

```text
cd experiments/g01-scaleset
GOWORK=off GOTOOLCHAIN=go1.26.8 go test ./livecanary -run '^(TestBaselineSessionOpenRejectsDuplicateMarkedPOSTBeforeInner|TestBaselineRuntimePathPrefixMismatchStopsBeforeInner|TestPinnedSDKDrainRejectsSnapshotTenantPrefixMismatchBeforeEffects|TestPinnedSDKDrainRejectsSessionTenantPrefixMismatchBeforeListener|TestPinnedSDKDrainRejectsSnapshotOriginMismatchBeforeEffects|TestPinnedSDKDrainRejectsSessionOriginMismatchBeforeListener|TestPinnedSDKDrainBindsAcquireToPhysicalRequestBody|TestPinnedSDKDrainBindsSessionCloseToSessionOpenOrigin|TestDrainListenerRejectsMarkedPollTargetMismatchBeforeInner)$' -count=1 -v -timeout=180s
```

It exited 0 in 0.481s. The corresponding focused race command exited 0 in
1.528s with no race report. The new boundary matrix covers same-origin
foreign-prefix scale-set, runner, session-open, acquisition and close requests
and asserts zero inner calls; the duplicate session-open regression asserts
exactly one inner call.

The complete offline package normal run exited 0 in 32.259s:

```text
cd experiments/g01-scaleset
GOWORK=off GOTOOLCHAIN=go1.26.8 go test ./livecanary -count=1 -timeout=240s
```

The complete package race run exited 0 in 42.067s with no race report:

```text
cd experiments/g01-scaleset
GOWORK=off GOTOOLCHAIN=go1.26.8 go test -race ./livecanary -count=1 -timeout=300s
```

The tagged paired-terminal preservation matrix exited 0 in 14.716s:

```text
cd experiments/g01-scaleset
GOWORK=off GOTOOLCHAIN=go1.26.8 go test -tags=g01_pair_fixture ./livecanary -run '^(TestPairedTerminalActualJournalsFinalize|TestPairedTerminalCompletionCadenceAndReceiptSeparation|TestPairedTerminalFinalResultCapacity|TestPairedTerminalPendingChildCapacity|TestPairedTerminalEligibilityUsesFreshExactFacts|TestPairedTerminalCapturedAcknowledgementCancellation|TestPairedTerminalMissingAcknowledgementsAndPostchecks)$' -count=1 -v -timeout=240s
```

`GOWORK=off GOTOOLCHAIN=go1.26.8 go vet ./livecanary`, `bash
scripts/gofmt.sh check`, and `git diff --check` each exited 0. The exact
tested implementation head is
`1f12e11309b273ca54b2b222f702539425ac91b1` (`fix G01 runtime tenant binding
and session cardinality`); the evidence append is documentation-only and
follows that head. No live runner, workflow, credential, App, Keychain,
launchd, Docker, Lima, ScaleSet, JIT, cleanup, or canary operation was run.

Files changed: `experiments/g01-scaleset/livecanary/baseline_listener.go`,
`baseline_terminal.go`, `baseline_wire.go`, `drain.go`, `drain_driver.go`,
`drain_p1_followup_test.go`, `sdk.go`, `sdk_integration_test.go`, and this
evidence file.

The remaining live gap is the separately authorized G01 canary/evidence gate;
exact-head Codex review, CI and merge remain coordinator-owned. Rollback is a
focused `git revert --no-edit 1f12e11309b273ca54b2b222f702539425ac91b1`
followed, if needed, by a separate revert of this evidence append; retain the
current journal, reservation and uncertainty, and never reset, erase, replay,
or run live cleanup.

### Exact-head follow-up: physical Host authority at marked wire boundaries

Date: 2026-09-13. This correction started from exact head
`1e19ec56acbeda044acaf80c2b8ada0e5d66c192` and addresses the fresh Codex P1
[request Host override finding](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3997211059).
No review request, merge, Project/Issue/goal/status change, live App, runner,
workflow, GitHub API, credential, or cleanup operation was performed.

#### Red-first reproductions

After adding the marked-operation and marked-poll Host regressions, but before
the source correction, this focused command exited 1:

```text
cd experiments/g01-scaleset
GOWORK=off GOTOOLCHAIN=go1.26.8 go test ./livecanary -run '^(TestMarkedRequestHostOverrideStopsBeforeInner|TestDrainListenerRejectsMarkedPollTargetMismatchBeforeInner|TestMarkedRequestHostMatchingURLHostPreservesForwarding|TestUnmarkedDeletePreservesInnerTransport)$' -count=1 -v -timeout=120s
```

The overridden Host poll case reached its inner transport once, and all four
marked session-open, ACK, acquisition and session-close cases returned nil and
would have reached their inner transport; the exact-Host and unmarked controls
passed. The test retained only fixed error categories and inner-call counters.

#### Minimal correction and green evidence

The final baseline wire boundary now accepts only an empty `req.Host` (the
normal SDK form) or exact equality with `req.URL.Host`; a mismatch is rejected
before any marked session-open, ACK, acquisition or session-close inner call.
The marked poll hook applies the same check before its inner transport, while
unmarked requests retain their prior forwarding behavior. The explicit
`req.Host == req.URL.Host` marked control and unmarked overridden-Host controls
pass.

The focused normal command exited 0 in 0.517s:

```text
cd experiments/g01-scaleset
GOWORK=off GOTOOLCHAIN=go1.26.8 go test ./livecanary -run '^(TestDrain|TestPinnedSDKDrain|TestBaseline.*(Acquire|SessionOpen|Snapshot|SessionClose)|TestMarkedRequestHost.*|TestUnmarkedDeletePreservesInnerTransport)$' -count=1 -timeout=300s
```

The corresponding focused race command exited 0 in 1.912s with no race
diagnostics:

```text
cd experiments/g01-scaleset
GOWORK=off GOTOOLCHAIN=go1.26.8 go test -race ./livecanary -run '^(TestDrain|TestPinnedSDKDrain|TestBaseline.*(Acquire|SessionOpen|Snapshot|SessionClose)|TestMarkedRequestHost.*|TestUnmarkedDeletePreservesInnerTransport)$' -count=1 -timeout=300s
```

The complete `livecanary` package normal run exited 0 in 30.030s, and the
complete race run exited 0 in 41.306s with no race diagnostics:

```text
cd experiments/g01-scaleset
GOWORK=off GOTOOLCHAIN=go1.26.8 go test ./livecanary -count=1 -timeout=300s
GOWORK=off GOTOOLCHAIN=go1.26.8 go test -race ./livecanary -count=1 -timeout=300s
```

The repository offline gate then exited 0 and printed
`offline experiment checks passed: 2 module(s)`:

```text
GOTOOLCHAIN=go1.26.8 bash scripts/check-offline-experiments.sh
```

`GOWORK=off GOTOOLCHAIN=go1.26.8 go vet ./livecanary`,
`GOWORK=off GOTOOLCHAIN=go1.26.8 bash ../../scripts/gofmt.sh check`, and
`git diff --check` each exited 0. The final diff secret/private-path scan
exited 0 and printed `diff secret/private-path scan passed`:

```text
set -e
path_pattern="$(printf '/%s/|/%s/' Users home)"
if git diff --text | rg -n "(${path_pattern}|-----BEGIN (RSA|OPENSSH|EC|PRIVATE)|github_pat_[A-Za-z0-9_]+|gh[pousr]_[A-Za-z0-9_]{20,}|Authorization[^\n]{0,20}Bearer[[:space:]]+[A-Za-z0-9._-]{20,})"; then exit 1; fi
printf '%s\n' 'diff secret/private-path scan passed'
```

The source/test correction is commit
`44524b9f290689378348951a722d7eb2ffb3d070`; this evidence append is the
documentation-only commit immediately after it. Rollback is recoverable with
`git revert --no-edit 44524b9f290689378348951a722d7eb2ffb3d070`; the evidence
append can be reverted separately if required. No live rollback was run, and
the unresolved live G01 gate, exact-head Codex review, CI and merge remain
coordinator-owned.

### Exact-head follow-up: opaque marked URLs and snapshot bootstrap allowlist

Date: 2026-09-13. This correction started from exact head
`d58224fe9a6b20b45b61eeb839fd84642ec587a2` and addresses the fresh Codex P1
findings [non-empty marked URL.Opaque forwarding](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3997312693)
and [marked snapshot non-target rewrite](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3997312695).
No review request, merge, Project/Issue/goal/status change, live App, runner,
workflow, GitHub API, credential, cleanup or canary operation was performed.

#### Red-first reproductions

After adding the two behavioral regressions and before changing source, this
focused command exited 1:

```text
cd experiments/g01-scaleset
GOWORK=off GOTOOLCHAIN=go1.26.8 go test ./livecanary -run '^(TestMarkedRequestOpaqueStopsBeforeInner|TestMarkedPollOpaqueStopsBeforeInner|TestMarkedSnapshotRewriteAllowsOnlyRequiredBootstrap|TestUnmarkedOpaqueRequestPreservesInnerForwarding)$' -count=1 -v -timeout=120s
```

The marked set/runner snapshot, session-open, ACK, acquisition, JIT,
session-close and poll cases with non-empty `URL.Opaque` returned nil and
reached the inner transport; the repository-token and dispatch snapshot
rewrites also returned nil and reached the inner transport. The exact required
bootstrap routes and unmarked opaque control passed. The regressions retain
only fixed error categories and inner-call counters; no request body, token,
URL, response error or private log is recorded.

#### Minimal correction and green evidence

The final marked request boundary now rejects every non-empty `URL.Opaque`
before forwarding, covering both the baseline marked transport and the marked
drain poll hook through the shared hierarchical-URL/Host check. Snapshot
captures now allow non-candidate requests only when they match the existing
exact approved registration-token or Actions runner-registration bootstrap
allowlist; all other marked non-target requests, including state-changing or
wrong-scope routes, reject before the inner transport. The production snapshot
captures carry the approved organization needed to validate those bootstrap
routes. Unmarked requests remain outside the marked check and retain prior
forwarding behavior.

The focused normal command exited 0 in 0.513s:

```text
cd experiments/g01-scaleset
GOWORK=off GOTOOLCHAIN=go1.26.8 go test ./livecanary -run '^(TestMarkedRequestOpaqueStopsBeforeInner|TestMarkedPollOpaqueStopsBeforeInner|TestMarkedSnapshotRewriteAllowsOnlyRequiredBootstrap|TestUnmarkedOpaqueRequestPreservesInnerForwarding)$' -count=1 -v -timeout=120s
```

The corresponding focused race command exited 0 in 1.542s with no race
diagnostics. The complete `livecanary` package normal run exited 0 in 28.951s,
and the complete race run exited 0 in 41.493s with no race diagnostics:

```text
cd experiments/g01-scaleset
GOWORK=off GOTOOLCHAIN=go1.26.8 go test -race ./livecanary -run '^(TestMarkedRequestOpaqueStopsBeforeInner|TestMarkedPollOpaqueStopsBeforeInner|TestMarkedSnapshotRewriteAllowsOnlyRequiredBootstrap|TestUnmarkedOpaqueRequestPreservesInnerForwarding)$' -count=1 -v -timeout=120s
GOWORK=off GOTOOLCHAIN=go1.26.8 go test ./livecanary -count=1 -timeout=360s
GOWORK=off GOTOOLCHAIN=go1.26.8 go test -race ./livecanary -count=1 -timeout=420s
```

`GOWORK=off GOTOOLCHAIN=go1.26.8 go vet ./livecanary`,
`GOTOOLCHAIN=go1.26.8 bash scripts/gofmt.sh check`, and `git diff --check`
each exited 0. The repository offline gate exited 0 and printed
`offline experiment checks passed: 2 module(s)`:

```text
GOTOOLCHAIN=go1.26.8 bash scripts/check-offline-experiments.sh
```

The final diff secret/private-path scan exited 0 and printed
`diff secret/private-path scan passed`:

```text
set -e
path_pattern="$(printf '/%s/|/%s/' Users home)"
if git diff --text | rg -n "(${path_pattern}|-----BEGIN (RSA|OPENSSH|EC|PRIVATE)|github_pat_[A-Za-z0-9_]+|gh[pousr]_[A-Za-z0-9_]{20,}|Authorization[^\n]{0,20}Bearer[[:space:]]+[A-Za-z0-9._-]{20,})"; then exit 1; fi
printf '%s\n' 'diff secret/private-path scan passed'
```

The exact tested implementation head is
`bf45b498db8bd35150d0f75181336663ea4cb027` (`fix G01 marked opaque and snapshot boundaries`),
and this evidence append is documentation-only. Rollback is recoverable with
`git revert --no-edit bf45b498db8bd35150d0f75181336663ea4cb027`; this evidence
append can be reverted separately if required. No live rollback was run, and
the unresolved live G01 gate, exact-head Codex review, CI and merge remain
coordinator-owned.

### Exact-head follow-up: marked session-open origin binding

Date: 2026-09-13. This correction started from exact head
`91aa852cc2f66102aad54c402b7da8e552163828`. The fresh Codex P1 finding is
[marked session-open origin mismatch forwarded before rejection](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3997399947).
No Project, Issue, goal, dependency or status field was changed.

#### Red-first reproduction

After adding the regression and before changing production code, this focused
command exited 1:

```text
cd experiments/g01-scaleset
GOWORK=off GOTOOLCHAIN=go1.26.8 go test ./livecanary -run '^TestBaselineSessionOpenOriginBindsToBeforeSnapshot$' -count=1 -v -timeout=60s
```

The same-origin control passed, while the different-but-approved origin failed
with `err=<nil> inner calls=1`, proving the marked POST reached the inner
transport before the fix. The regression retained only fixed counters and
error categories; no request body, token, URL, response error or private log
was recorded.

#### Correction and verification

The before Scale Set snapshot origin now initializes the paired listener's
session-open capture, and the pinned drain session capture inherits the
before-snapshot origin from its poll hook. A marked session-open target now
rejects a different origin before body handling or inner transport forwarding;
same-origin session-open, the explicit bootstrap allowlist, unmarked requests,
and the existing tenant-prefix, Host and Opaque checks remain intact. The
correction source/test commit is
`3be170f82e4b173baf808f22e3637dfd533fe7d4`.

The focused normal run exited 0 in 0.513s, including the new regression, the
pinned-SDK mismatch regression, same-origin session-open, bootstrap,
tenant-prefix, Opaque, Host and unmarked-forwarding controls:

```text
cd experiments/g01-scaleset
GOWORK=off GOTOOLCHAIN=go1.26.8 go test ./livecanary -run '^(TestBaselineSessionOpenOriginBindsToBeforeSnapshot|TestPinnedSDKDrainRejectsSessionOriginMismatchBeforeListener|TestBaselineSessionOpenTargetMismatchStopsBeforeInner|TestBaselineSessionOpenBodyMustMatchOwnerBeforeInner|TestBaselineRuntimePathPrefixMismatchStopsBeforeInner|TestMarkedRequestOpaqueStopsBeforeInner|TestMarkedSnapshotRewriteAllowsOnlyRequiredBootstrap|TestUnmarkedOpaqueRequestPreservesInnerForwarding|TestDrainPollHookForwardsUnmarkedNonPollRequest)$' -count=1 -v -timeout=240s
```

The focused race run exited 0 in 4.737s with no race diagnostics:

```text
cd experiments/g01-scaleset
GOWORK=off GOTOOLCHAIN=go1.26.8 go test -race ./livecanary -run '^(TestDrain|TestPinnedSDKDrain|TestDriverDrainThroughPinnedSDKAndPollHook|TestBaseline.*(Acquire|SessionOpen|Snapshot)|TestMarked|TestUnmarked)' -count=1 -timeout=300s
```

The full package normal run exited 0 in 28.681s and the full race run exited 0
in 40.923s with no race diagnostics:

```text
cd experiments/g01-scaleset
GOWORK=off GOTOOLCHAIN=go1.26.8 go test ./livecanary -count=1 -timeout=300s
GOWORK=off GOTOOLCHAIN=go1.26.8 go test -race ./livecanary -count=1 -timeout=300s
```

`GOWORK=off GOTOOLCHAIN=go1.26.8 go vet ./livecanary`,
`bash scripts/gofmt.sh check`, and `git diff --check` each exited 0. The
diff secret/private-path scan exited 0 and printed
`diff secret/private-path scan passed`:

```text
set -e
if git diff --text | rg -n '(/Users/|/home/|-----BEGIN (RSA|OPENSSH|EC|PRIVATE)|github_pat_[A-Za-z0-9_]+|gh[pousr]_[A-Za-z0-9_]{20,}|Authorization[^\n]{0,20}Bearer[[:space:]]+[A-Za-z0-9._-]{20,})'; then exit 1; fi
printf '%s\n' 'diff secret/private-path scan passed'
```

The repository offline gate exited 0 and printed
`offline experiment checks passed: 2 module(s)`:

```text
GOTOOLCHAIN=go1.26.8 bash scripts/check-offline-experiments.sh
```

The exact tested implementation head is
`3be170f82e4b173baf808f22e3637dfd533fe7d4`; this evidence append is a separate
documentation-only commit. Rollback is recoverable with
`git revert --no-edit 3be170f82e4b173baf808f22e3637dfd533fe7d4`; no live
rollback, App, runner, workflow, canary, Docker/Lima, Keychain, launchd,
credential or cleanup operation was performed. The unresolved live G01 gate,
independent exact-head Codex review, CI and merge remain coordinator-owned.

### Exact-head follow-up: case-folded duplicate poll capacity header

Date: 2026-09-13. This correction started from exact head
`2c3227780a5b5ef1ae4424ecbc50b53b795fbf96` and addresses the fresh Codex P1
[case-folded duplicate marked poll capacity finding](https://github.com/1XP-AI/gh-runnerd/pull/72#discussion_r3997489022).
The pinned SDK v0.4.0 header is `X-ScaleSetMaxCapacity`; no review request,
merge, Project/Issue/goal/status change, live App, runner, workflow, GitHub
API, credential, cleanup, or canary operation was performed.

#### Red-first reproduction

After adding `TestPinnedSDKDrainRejectsCaseFoldedDuplicateCapacityBeforeFixture`
and before changing the source boundary, this focused pinned-SDK TLS loopback
command exited 1:

```text
cd experiments/g01-scaleset
GOWORK=off GOTOOLCHAIN=go1.26.8 go test ./livecanary -run '^TestPinnedSDKDrainRejectsCaseFoldedDuplicateCapacityBeforeFixture$' -count=1 -v -timeout=120s
```

The canonical-only check accepted the lower-case map-key duplicate injected by
the intervening wrapper; the fixture saw both polls, the observation was
`Outcome:observed`, and the returned error was `err=<nil>`. The regression keeps
only fixed outcome/counter evidence and does not retain a request body, token,
URL, response error, or private log.

#### Minimal correction and green evidence

The marked poll boundary now scans every `http.Header` map key with
`strings.EqualFold`, collects all matching values, and requires exactly one
value equal to the intended capacity (`1` on the first poll and `0` on the
withdrawn poll). This rejects case-folded duplicate keys before the inner
transport while preserving the pinned SDK's valid canonical header, exact
origin/tenant/Host/Opaque/session/ACK/acquisition/close guards, unmarked
forwarding, and the existing snapshot bootstrap allowlist.

The focused normal command exited 0 in 0.464s:

```text
cd experiments/g01-scaleset
GOWORK=off GOTOOLCHAIN=go1.26.8 go test ./livecanary -run '^(TestPinnedSDKDrainRejectsCaseFoldedDuplicateCapacityBeforeFixture|TestPinnedSDKDrainRejectsPhysicalPollMutationBeforeInner|TestPinnedSDKDrainVerifyRunRejectsAmbiguousWireFieldsBeforeEffects|TestPinnedSDKDrainAcquisitionRequiresStrictWireResponse|TestDrainPollHookRejectsDuplicatePhysicalWrites|TestDrainPollHookRequiresOneSuccessfulWritePerPoll|TestDriverDrainThroughPinnedSDKAndPollHook)$' -count=1 -v -timeout=300s
```

The corresponding focused race command exited 0 in 1.962s with no race
diagnostics:

```text
cd experiments/g01-scaleset
GOWORK=off GOTOOLCHAIN=go1.26.8 go test -race ./livecanary -run '^(TestPinnedSDKDrainRejectsCaseFoldedDuplicateCapacityBeforeFixture|TestPinnedSDKDrainRejectsPhysicalPollMutationBeforeInner|TestPinnedSDKDrainVerifyRunRejectsAmbiguousWireFieldsBeforeEffects|TestPinnedSDKDrainAcquisitionRequiresStrictWireResponse|TestDrainPollHookRejectsDuplicatePhysicalWrites|TestDrainPollHookRequiresOneSuccessfulWritePerPoll|TestDriverDrainThroughPinnedSDKAndPollHook)$' -count=1 -v -timeout=300s
```

The complete offline `livecanary` package normal run exited 0 in 28.402s and
the race run exited 0 in 38.714s with no race diagnostics:

```text
cd experiments/g01-scaleset
GOWORK=off GOTOOLCHAIN=go1.26.8 go test ./livecanary -count=1 -timeout=360s
GOWORK=off GOTOOLCHAIN=go1.26.8 go test -race ./livecanary -count=1 -timeout=420s
```

`GOWORK=off GOTOOLCHAIN=go1.26.8 go vet ./livecanary`,
`GOTOOLCHAIN=go1.26.8 bash ../../scripts/gofmt.sh check`, and `git diff --check`
each exited 0. The final diff secret/private-path scan exited 0 and printed
`diff secret/private-path scan passed`:

```text
set -e
path_pattern="$(printf '/%s/|/%s/' Users home)"
if git diff --text | rg -n "(${path_pattern}|-----BEGIN (RSA|OPENSSH|EC|PRIVATE)|github_pat_[A-Za-z0-9_]+|gh[pousr]_[A-Za-z0-9_]{20,}|Authorization[^\n]{0,20}Bearer[[:space:]]+[A-Za-z0-9._-]{20,})"; then exit 1; fi
printf '%s\n' 'diff secret/private-path scan passed'
```

The two-module offline gate exited 0 and printed
`offline experiment checks passed: 2 module(s)`:

```text
GOTOOLCHAIN=go1.26.8 bash scripts/check-offline-experiments.sh
```

The exact tested source/evidence implementation head is
`e4e2a28fd24c767db51a83869369631e86b3c919` (`fix(g01): reject case-folded
duplicate poll capacity`); this evidence append is documentation-only on top
of that source commit. Rollback is recoverable with
`git revert --no-edit e4e2a28fd24c767db51a83869369631e86b3c919`; revert this
documentation append separately if needed. No live rollback was run, and the
unresolved live G01 gate, exact-head Codex review, CI and merge remain
coordinator-owned.
