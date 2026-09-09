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

The transport hook records `WroteRequest` and holds the first response body
before the high-level listener parser receives it. `WroteRequest` is a
client-side transport fact only: `server_receipt` remains `unproven`, so the
synthetic test and any future live result cannot claim server acceptance or an
atomic server-side drain. The implementation calls the released listener's
public `SetMaxRunners(0)` callback and lets the listener perform its normal
ACK-before-acquire sequence.

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
The meaningful behavioral red was reproduced against the immutable base
`cf67d4a`: `TestIssue71DrainAuthorityIsBehaviorallyAvailable` failed because
the base rejected the new `drain` phase with `approval rejected`. The equivalent
green test is `TestDrainPhaseAuthorityIsAccepted` on fix commit
`82d0d7d`.

Before the correction commit, a temporary test-only checkout pinned to
`6e954cf` reproduced all five independent findings with executable assertions:
poll reservation omitted, rejected idle state not fencing replay, wrong ACK
reaching the inner effect, no-message accepted as observed, and cancellation
missing an explicit marker. All five assertions failed on that pre-correction
head; the focused tests below pass on `82d0d7d`.

## Independent correction matrix

| Finding | Correction and evidence |
|---|---|
| Codex P1 status-only response close (`r3965776832`) | `drainHeldBody.Close` remains release-gated; `TestDrainHeldBodyHoldsCloseUntilRelease` passes (fix `f44f5f9`). |
| Codex P1 cancellation join (`r3965776826`) | all listener context exits release and join the run goroutine; `TestDrainRejectsEffectsAfterCancellationAndRecordsMarker` passes (fixes `f44f5f9`, `82d0d7d`). |
| Codex P1 drain route unreachable (`r3965776815`) | `Run` routes `drain` before generic owned/statistics quarantine; `TestDriverRoutesDrainBeforeNoWorkerStatisticsQuarantine` passes (fix `f44f5f9`). |
| Codex P2 paired verification (`r3965957331`) | drain is admitted as a verification phase in paired approval/preparation; paired tests pass (fix `945371e`). |
| Codex P1 runner continuity (`r3966125441`) | observed requires exact before/after runner tuple; runner mutation is rejected (fix `6e954cf`). |
| Codex P1 embedded session set (`r3966362999`) | session-open now requires exact nested set identity, update fence, labels, and statistics equal to the idle before snapshot. `TestDrainRejectsEmbeddedSessionStatisticsMismatch` passes (fix `82d0d7d`). |
| Codex P1 ambiguous session retention (`r3966363009`) | non-successful listener outcomes retain the live session, write a quarantine marker, and never invoke session close; `TestDriverRoutesDrainBeforeNoWorkerStatisticsQuarantine` asserts zero close calls (fix `82d0d7d`). |
| Codex P1 phase crash fence (`r3966362990`) | replay marks a durable `drain` phase uncertain before polling, so a crash before the final observation cannot authorize cleanup. `TestDrainPhaseStartRetainsCrashUncertainty` passes (fix `82d0d7d`). |
| Security F1 / Protocol F1 poll reservation | `observe-poll` result journals one verified request ID plus fixed work before ACK; replay sets reservation/work fences. `TestDrainPollJournalsReservationBeforeACK` passes (`82d0d7d`). |
| Security F2 rejected idle prerequisite | complete bounded `DrainSnapshot` plus `prerequisite-failed` marker is retained; replay remains uncertain. `TestDrainRejectedIdlePrerequisiteRetainsFence` passes (`82d0d7d`). |
| Security F3 / Protocol F2 runner and nested set identity | observed requires exact runner continuity; session-open validates nested set ID/name/group/label/update fence. Pinned SDK drain integration and mutation tests pass (`6e954cf`, `82d0d7d`). |
| Security F4 / Protocol F1 no-message | observed requires a present old controlled message, known counters, successful ACK and acquisition; 202/no-message is inconclusive. `TestDrainListenerNoMessageIsInconclusive` passes (`82d0d7d`). |
| Security F5 / Protocol F5 counters and integration | known counters require nonnegative internally consistent busy/idle partition; unknown values cannot masquerade as zero. `TestDrainRequiresKnownConsistentStatisticsAndControlledMessage` and `TestDriverDrainThroughPinnedSDKAndPollHook` pass (`82d0d7d`). |
| Protocol F3 cancellation/WithoutCancel | phase context is bound through the wrapper, checked before every ACK/acquisition/poll effect, and cancellation/deadline/quarantine writes a fixed marker even when close/after inspect fails. Focused race test passes (`82d0d7d`). |
| Protocol F4 duplicate/wrong callbacks | exact phase, identity, ACK-before-acquire and one-shot state are validated before inner effects; wrong/duplicate callbacks make zero additional inner calls. `TestDrainRejectsDuplicateOrWrongEffectsBeforeInnerCall` passes (`82d0d7d`). |

The independent design review remains respected: the transport marker is
client-side only, `server_receipt` is `unproven`, the real high-level pinned
listener is retained, the hook is bounded to one old and one next poll, and no
pre-provision, recovery, replay, `RemoveRunner` or live operation was added.

Verified locally with the pinned `github.com/actions/scaleset v0.4.0` module:

```text
cd experiments/g01-scaleset
go test ./livecanary -run 'TestDrain|TestCredentialAttestationMismatchAndExpiredTokenRejected' -count=1
go test -race ./livecanary -run 'TestDrain|TestDriverDrainThroughPinnedSDK' -count=1
cd ../g02-auth
go test ./... -run 'TestBrokerFinitePhases|TestBrokerControllerApproval' -count=1
```

All commands above passed, as did `go test ./...` in both experiment modules.
The listener tests use `httptest` synthetic transports and the real released
listener; the pinned-SDK test composes `OpenDrainSession` with the actual
`MessageSessionClient` offline. These are offline protocol evidence, not live
qualification. No credentials, runner/session/JIT operation,
workflow operation, app/keychain/launchd/Docker/Lima mutation or cleanup was
performed.

## Remaining gate and rollback

The live G01 gate remains unresolved until a separately authorized run uses an
immutable reviewed head, the approved private repository/workflow/resources,
one idle owned canary runner and independent review. A timing miss, missing
runner, stale zero, identity/statistics mismatch, unknown response, or
server-receipt ambiguity must remain inconclusive. Rollback is the focused
revert of the issue-71 commit/PR; no live cleanup or workflow replay is part
of rollback.
