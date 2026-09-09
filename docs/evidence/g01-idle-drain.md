# G01 idle-drain observation evidence

Issue #71 adds an experiment-only observation phase. It does not close the
parent G01 goal, add a production provider, or authorize live execution.

## Invariant and counterexample

The invariant is: withdrawing listener capacity to zero may affect the next
poll, while the poll already admitted by the listener must retain the pinned
SDK's ACK-before-acquisition ordering. The counterexample is a response that
arrives before a request-written marker, an absent/ambiguous marker, or a
second poll that returns a message after withdrawal; each case is recorded as
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
with zero available/acquired/assigned/running/busy work. No idle-worker
pre-provision seam was added; if a future live run needs one, it requires a
separate independent design review and explicit maintainer authorization.

## TDD and verification

Meaningful behavioral red was captured before the implementation existed:

```text
cd experiments/g01-scaleset
go test ./livecanary -run 'TestDrainListener' -count=1
```

The module failed to compile because the drain hook, listener runner,
boundary/category constants and experiment seam were undefined. After the
minimal implementation, the same behavior was green, followed by failure and
boundary coverage for missing request-written evidence, negative counters,
identity changes, duplicate ordering, response-before-write observations,
idle prerequisites, replay uncertainty and missing verification authority.

Verified locally with the pinned `github.com/actions/scaleset v0.4.0` module:

```text
cd experiments/g01-scaleset
go test ./livecanary -run 'TestDrain|TestCredentialAttestationMismatchAndExpiredTokenRejected' -count=1
go test -race ./livecanary -run 'TestDrainListener' -count=1
cd ../g02-auth
go test ./... -run 'TestBrokerFinitePhases|TestBrokerControllerApproval' -count=1
```

All commands above passed. The listener test uses an `httptest` synthetic
transport and the real released listener; it is offline protocol evidence,
not live qualification. No credentials, runner/session/JIT operation,
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
