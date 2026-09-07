# G01 external-review corrections

This work tracks [issue30](https://github.com/1XP-AI/gh-runnerd/issues/30) and
references [G01](https://github.com/1XP-AI/gh-runnerd/issues/1). The audit reproduced
all six external findings from merged PR25/28 against main `f1fe70c`. The PR25
comments were stale by reviewed commit, but remained unresolved defects. No live
credentials, GitHub mutation, Docker operation or worker execution was used.

An adjacent cleanup regression is preserved at red commit `68eb032`: started-,
assigned- and completed-only messages previously took the empty-poll safe-close
path, leaving cleanup possible under stale zero. Unexpected work kinds are now
quarantined before testing for an empty available-job list. Nil/empty polls can
take the no-message path only after earlier evidence permits the controlled
probe; they never clear prior work fences. No terminal reconciliation is inferred
from these unexpected messages.

Independent review also reproduced a nonzero-statistics poll with no job entries
taking that safe-close path. Red commit `c575fc0` preserves the case. Empty polls
now require the complete statistics structure to be zero; positive demand,
acquired/running work or runner counts conservatively retain uncertainty. Those
counts are not treated as identities or ownership proof. The true empty control
intentionally uses all-zero statistics. A nil poll after unsafe session/read
evidence cannot authorize safe close or cleanup; prior demand still blocks cleanup.

## Initial corrections

The immutable red commit `f6fc90b` preserves regressions for these findings:

- [P1: observed jobs allowed cleanup](https://github.com/1XP-AI/gh-runnerd/pull/25#discussion_r3949077101).
  Replay now retains every observed request ID. Cleanup refuses while any remain,
  including after a planned barrier closes its session and aggregate statistics
  report zero. This small harness has no terminal-job reconciliation; inspection
  does not release those requests.
- [P1: multi-job acquisition batch ACKed before refusal](https://github.com/1XP-AI/gh-runnerd/pull/25#discussion_r3949077092).
  Acquisition phases enforce exactly one available request before returning the
  message to the SDK listener, so the listener cannot ACK an invalid batch.
- [P2: missing bridge consumed a worker start](https://github.com/1XP-AI/gh-runnerd/pull/28#discussion_r3949487047).
  Profile verification requires exactly one bridge attachment. Empty and missing
  network maps fail closed; the synthetic runtime now models the required bridge
  for positive cases.

Red command: `GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=30s -run '^TestAuditPR(25Observed|25Multi|28Missing)' ./livecanary ./liveworker`
from `experiments/g01-scaleset`. It failed with one cleanup delete for each of
three unresolved-job barriers, one ACK for each invalid acquisition phase, and
one start for both missing-network cases.

After the initial corrections, fresh package race tests passed: livecanary1.616s
and liveworker2.014s. These are synthetic results and do not close the live gates.

## Directory-sync recovery

Red commit `b0c8059` reproduces the failed-directory-sync restart defect in both
worker and controller journals. An injected initial sync failure stops the first
open, but the former reopen path accepted its valid header without retrying the
failed directory sync. Both implementations now sync on every successful open,
including existing journals, before returning authority to perform effects.
Targeted fresh journal race tests passed for both packages. This is deterministic
failure-injection evidence, not a physical crash or power-loss experiment.

## Explicit recovery authority

Red commit `c2ae474` reproduces both blocked recovery renewal and a raw Driver
caller changing approval fields after journal open. The controller has the same
former whole-approval-digest trap as the worker; both are corrected here.

The new version1 journal header hashes stable ownership with only `expires_at`
and `phases` excluded. The initial authority and each later authority record
include the complete approval digest, expiry and phase list. A changed authority
must extend expiry and may contain only `inspect` and `cleanup`. The caller must
provide the new explicit, currently valid approval. Each renewal is synced before
the journal can authorize an operation. It never resets phase attempts, known
observations, ownership receipts or uncertainty; older authority cannot be reused
after renewal. Source/harness, endpoint/daemon, image, nonce and all other stable
fields remain fixed. Expired authority alone never authorizes recovery.

Both Journal interfaces now require an unexported authorizer at Driver.Run before
preflight. Its exclusive lease refuses concurrent calls on that FileJournal;
Close waits for the active operation. The authorizer checks the current approval,
still-owned journal inode and original directory identity. Mutating Driver fields,
closing the journal, or replacing the file/directory cannot bypass this check.
This is approved-code discipline in a trusted controller account, not isolation
from hostile Go callers or same-UID code.

Old journals without the versioned ownership header are refused and retained.
There is no migration or automatic adoption. No live controller/worker journals
have been created in this experiment. Explicit renewal does not make an unknown
resource safe to delete; the existing cleanup fences remain required.

Additional observed working-tree red cases caught a shared read lease admitting
concurrent phases and replaced journal/directory identities still authorizing
operations. The final exclusive lease and identity checks cover those cases.
Their tests and implementation are committed together, so these are not claims
of separate immutable red commits.

## Decoder-equivalent field names

The adjacent JSON regression at red commit `b77f0b7` showed both strict decoders
accepting duplicate fields through ASCII casing and Unicode long-s aliases.
Duplicate detection now canonicalizes each name by Unicode simple-fold class,
matching the standard decoder's struct-field equivalence without quadratic
pairwise comparisons. Single aliases and separate object scopes remain valid.
These helpers serve fixed approval/credential/journal schemas; arbitrary maps
with case-distinct keys are intentionally outside this strict decoder contract.
This is an adjacent verified defect, not an additional original audit finding.

## Permanent controller experiment admission

Red commit `a41030e` preserves the
[P1 cross-directory cap regression](https://github.com/1XP-AI/gh-runnerd/pull/25#discussion_r3949077105):
two private state directories with different nonces admitted two synthetic scale
sets. It also preserves refusal regressions for copied state, a closed/deleted
first experiment, failed admission sync and unsafe or unknown claim files.

The public controller journal opener now obtains one fixed directory from the
effective UID's OS account home: `<OS-account-home>/.gh-runnerd-g01-experiment`.
The operator must explicitly prepare that directory as an owned, nonsymlink
`0700` directory before a separately approved live invocation. The account home
must be owned and not writable by group/others. No environment variable, CLI
flag, approval field or supplied state directory chooses the admission root.
Missing or unsafe state refuses before remote effects. This work did not prepare
the real account directory; all admission tests use private injected roots.

The `0600`, single-link `admission.json` holds a permanent versioned record of
stable approval ownership and the exact state-directory and journal device/inode
identities. A nonblocking global flock is held for the FileJournal lifetime.
The claim file, root directory and containing directory are synced, including on
reopen. The sealed Driver.Run authorizer checks the current claim, journal and
ownership while holding its exclusive operation lease. Copied state, another
nonce/directory, replaced inventory or concurrent use cannot obtain this lease.
Closing the process, successful scale-set deletion and uncertain outcomes never
remove or reassign the pin. A later distinct experiment requires separately
implemented and reviewed reconciliation; deleting the claim is not a retry path.

An additional observed working-tree red reproduced Go 1.26.8's `osusergo`
current-account fallback selecting a synthetic `HOME` (0.501s). The production
opener now requires `cgo && !osusergo && !android` before even preparing a journal;
unsupported builds return a fixed refusal. The ordinary Darwin OS-backed lookup
is the intended runtime contract. The same synthetic regression passes with
`-tags=osusergo` (race, 1.504s) and `CGO_ENABLED=0` (0.837s). Source inspection of
the pinned standard library's `os/user/lookup.go`, `lookup_stubs.go` and
`cgo_lookup_unix.go` established the lookup distinction. These guard tests and
the fix are committed together, not claimed as a separate immutable red commit.

This is a permanent **controller-UID cap for the bounded scale-set experiment**,
including its session/JIT/acquisition budget. It does not add admission across
independent worker journals: the worker helper still limits one container per
owned journal. Integrated one-worker baseline execution, global worker admission
and production fleet admission remain blocked. Trusted same-UID/admin code can
alter files and is outside this filesystem discipline's threat boundary.

The focused red command is
`GOTOOLCHAIN=go1.26.8 go test -race -count=1 -run 'TestAdmission|TestAuditPR25Distinct' ./livecanary`.
Default controller/worker package race checks passed after the admission change
(2.690s/1.826s). Sync failures are injected; no physical crash, real scale set,
account-root mutation or live resource cleanup is claimed.

## Retaining every work-bearing observation

Final independent review of `cad9bd2` reproduced another path to stale-zero
deletion: initial session running statistics followed by a nil poll closed the
session and allowed cleanup. The source audit also reproduced forgotten create
response statistics, owned scale-set reads, inspection and optional embedded
session-set statistics. Nonempty poll messages with running counts still reached
ACK/acquisition, and a pre-JIT owned read with running counts still issued JIT.
An exact runner reference observed during inspection or pre-JIT lookup was also
forgotten when later lookups returned nil.

Red commits `5c853eb` and `f1c94ec` preserve the statistics regressions and controls;
`5dbfa75` preserves runner-presence regressions. The first source matrix and actual
file-journal reopen tests failed with one delete after later zero inspection
(0.891s). The unsafe-running effect tests observed ACK, acquisition or JIT calls
(0.484s). Runner-presence tests observed a later delete in all eight owned/invalid
reference cases (0.482s). These are synthetic results; `cad9bd2` approval did not
authorize merging or executing this later correction.

The shared work classifier and durable observation contract cover these sources:

| Source | Required evidence and behavior |
| --- | --- |
| Valid create response | Statistics required. Validated set ID and work category share one durable result before any quarantine return. |
| Owned GetScaleSet, including inspect, cleanup and pre-JIT/probe reads | Object and statistics required. Persist read intent and classified result; cleanup replays the newly written evidence before deletion. |
| OpenSession | Top-level statistics required. Optional embedded set may be absent; when present, its statistics are required and its category is combined conservatively. Known session ID and category share one result. The listener receives that checked initial snapshot. |
| Poll | Nil message means no observation. A present message requires statistics; read intent and category are durable before validation/ACK/acquisition. |
| Unowned FindScaleSet discovery | Nil object means no observation. A present object requires statistics and is classified without adopting its ID. Discovery never authorizes ownership or a later creation retry. |
| FindRunner | Nil reference means no observation. Any present reference fences unknown capacity; retain its ID only after exact positive-ID/name/scale-set checks. Unexpected references also fail closed. |

Nonnegative available/assigned-only demand permanently blocks cleanup while
allowing the separately approved controlled probe. Acquired/running jobs, any
registered/busy/idle runner count, negative counts and missing required statistics
permanently quarantine new effects. Counts never prove individual ownership.
Actual runner presence separately records identity only when its binding is valid.
No later zero, nil, session close or successful inspection clears either fence.

Work-bearing reads use fixed `observe-owned`, `observe-poll`, `observe-discovery`
and `observe-runner` intent/result operations. The intent is synced before the
read because a failed result write could otherwise lose observed work. Create
and session effects store identity and category atomically in their existing
result. A failed result leaves its durable intent unresolved across reopen.
Inspection remains remotely read-only and cannot erase an older pending intent.
The update-setting regression now distinguishes these journaled observations
from remote mutation intents, retaining its zero-session/JIT/ACK checks.

The focused synthetic command is
`GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=45s -run 'TestObservation|TestStatistics|TestDemandStatistics|TestUnsafeStatistics|TestZeroStatistics|TestOlderPendingIntent|TestObservedRunner|TestUnownedDiscovery' ./livecanary`.
Additional failure tests cover refusal before a read and failed owned/runner
results across actual private journal close/reopen followed by a successful
zero/absent inspection. Genuine all-zero and optional-absence controls retain
empty cleanup, and demand retains the intended controlled barrier. No live
resource, host crash or power-loss test is claimed.

## Invalid owned-object proof is also durable

The next exact-head external review found
[P1: persist invalid owned-set observations](https://github.com/1XP-AI/gh-runnerd/pull/37#discussion_r3950272982)
at `94e4eff`, `livecanary/driver.go:205`. The read result previously stored zero
statistics as safe before checking the returned ID, name, group and ownership
label. The immediate phase refused, but a later matching zero response could
still authorize cleanup.

Red commit `b29f3ba` reproduces 30 invalid-to-valid transitions: five nonnil
identity failures across inspection, pre-JIT and pre-probe reads, each using a
fresh Driver or an actual private journal close/reopen. Every case reached its
first cleanup with one delete and no uncertainty (race run, 2.289s). Six nil-object
controls were already fenced. The fixture uses the same inventory before and
after the observation, so neither inventory mismatch nor an earlier cleanup
attempt masks the ownership regression.

All existing owned-object predicates now run inside the `observe-owned` result:
nonnull object, exact scale-set ID/name/group and the required ownership label
name. Any failure is durably unresolved before returning quarantine; the original
create receipt is retained. A matching zero inspection cannot clear it. The
existing intent/result failure ordering also covers this invalid result. The
separate DisableUpdate phase gate and otherwise verified empty-cleanup control
remain unchanged.

Focused command:
`GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=45s -run 'TestInvalidOwnedProof|TestUpdateSettingDrift|TestCleanupOnlyForNeverIssuedWorker' ./livecanary`.
Adjacent review checked malformed create/session callbacks, post-poll validation,
runner binding and discovery; those already retain their applicable uncertainty
or cannot adopt/retry. This correction changes only owned-object predicate
placement. It requires fresh exact-head review and CI; earlier clean verdicts
do not cover it, and no live execution is claimed.
