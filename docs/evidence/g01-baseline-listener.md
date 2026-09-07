# G01 private baseline listener evidence

Issue [#47](https://github.com/1XP-AI/gh-runnerd/issues/47) adds a private collection
listener under the concrete controller journal lease. It uses Scale Set SDK v0.4.0
and the existing strict REST source reader. This listener-only slice adds no public
CLI/broker entry or approval phase/API, JIT request, worker invocation, automatic
session close or live execution. Issue54 adds private terminal journal stages
through a separate caller; see the [terminal evidence](g01-paired-terminal.md).
G01 remains open: live job/runner reconciliation and safe production cleanup still
require their own implementation and evidence.

## Implemented boundary

`newBaselineListenerHeld` is network-lazy. Approved package code must already hold
the real controller execution lease, supply the matching captured SDK approval and
separate verification credential, and have the original assigned scale-set creation
record. A future caller must additionally prove completed controller/worker pairing
and host preflight. Merely constructing or entering this private listener does not
supply that integration proof or create public phase authority. The later paired
collection and terminal callers keep that proof outside this listener; see the
[paired collection](g01-paired-baseline.md) and [terminal evidence](g01-paired-terminal.md).

`run` first journals an exact owned-set GET and checks its ID, name, group, owned
label, updates-disabled setting and all seven statistics. It then records the
session, whole message, strict REST source check, ACK and singleton acquisition,
with intent/result pairs. Before acquisition, acquired/running/registered/busy/idle
statistics must be present zero; available/assigned may be zero or one. Supplied
nested session statistics are subject to the same rule. Missing/null, negative and
above-singleton values refuse. After acquisition, each supplied count remains
bounded to zero or one and is only an observation, not ownership or success proof.
A nil/202 poll contributes no new statistics.

The request-local transport guard sees the exact operation once, after the shared
1 MiB response limit. It normalizes a maximum of four items in wire order, rejects
malformed items, unknown kinds, exact/folded key collisions, trailing values and
contradictory acquire envelopes, and passes unchanged bytes to the SDK. A lone
case alias which both decoders interpret identically remains accepted. Bootstrap
responses do not satisfy the operation capture. Existing TLS destination,
HTTP/1, proxy, redirect, retry-count and PATCH-refusal controls are retained.

This addresses two pinned-SDK behaviors: its message decoder partitions known
kinds and drops unknown kinds, and its acquisition decoder returns `value` without
checking `count`. See the pinned [message decoder](https://github.com/actions/scaleset/blob/v0.4.0/client.go),
[session client](https://github.com/actions/scaleset/blob/v0.4.0/session_client.go)
and [listener](https://github.com/actions/scaleset/blob/v0.4.0/listener/listener.go).
The SDK listener performs ACK before acquisition and passes `WithoutCancel` to
message handling; this adapter checks its captured original context instead.

Available binds the singleton request and opaque job ID to the approved
owner/repository/run/event and a bounded nonempty opaque workflow ref. The exact
REST run independently corroborates the approved repository/path/head/event and
attempt 1, including private/non-fork base and head repositories. The workflow ref
is retained as observed data, without an assumed format or a new approval field.
Sparse later source fields retain their missing/null or explicit-zero states;
populated contradictions refuse. SDK runner facts remain distinct from REST job
and runner IDs. The first batch cannot contain Started/Completed before acquisition;
this is experiment eligibility, not a claim about service ordering. Assigned can
use an Available anchor in the same batch.

The synchronous continuation runs once only after the actual SDK acquisition
method has returned and released its session mutex, and count 1 plus exactly the
requested singleton has been durably recorded. Its value receipt links assigned
whole-batch/source/ACK/acquire references and the Available item index. Callback
delivery records link their original wire indices; the SDK's grouped callback
order does not replace wire order. Matching Completed ends collection, and does
not establish GitHub job success, runner deletion permission or local worker exit.

## Persistence and interruption

The controller reference domain is `gh-runnerd/g01-pair/controller-event/v1`, then
NUL, JSON(stable journal identity), NUL, JSON(assigned Event). Field order and tags
match the approved pair contract without importing worker types. Events and
reference payloads are copied. Authority renewal still appends before claim
initialization; existing event references retain the same stable identity.

Every effect has room reserved for a 16 KiB intent and maximum result within the
1 MiB journal limit. Actual file fsync precedes the effect, followed by another
current-context/authority/file-identity check. Failure after intent retains a
pending attempt. Valid wire responses already received when the SDK reports
original-context cancellation are retained as known facts; cancellation still
prevents the next effect or continuation. If result persistence itself fails, the
intent remains the conservative boundary; a fully written result may be found on
reopen. No branch repairs a torn tail or grants a retry.

The run is limited to 16 polls, four items per message, thirty seconds per request
and the earliest caller/approval/credential deadline, capped at ten minutes.
Operations serialize and retained/reentrant receivers refuse. The caller's real
lease prevents concurrent Close from releasing journal/claim ownership. Returned
session objects remain with the private owner; when an SDK error prevents returning
an object, any captured session ID stays in the normalized record. This slice
provides no resume or teardown path. Generic replay retains the work/uncertainty
fences, and a mixed subsequent legacy action history is refused. Operator
reconciliation and finite ACK/acquire/JIT fault experiments remain future work.

Raw bodies, queue and acquisition URLs/tokens, arbitrary display strings, credential
values and raw SDK errors are excluded from the journal, acquisition receipt and
returned errors. These are trusted controller-process boundaries; they do not
isolate hostile same-user Go code or an administrator changing the host.

## Actual synthetic red and green evidence

All commands below run in `experiments/g01-scaleset`. Tests use real SDK requests
to a private loopback TLS fixture and actual temporary controller journals with an
injected private admission root. They create no real account admission root,
GitHub resource, runner, container or Keychain item.

| Checkpoint | Observed result |
|---|---|
| `f0a98f4` — `go test -race ./livecanary -run '^TestBaselinePinnedSDKAdmission$' -count=1` | Compiled new-feature red: valid control passed; unknown sibling, pre-acquire Started and wrong envelope count incorrectly returned success with ACK 2/acquire 1/continuation 1 (0.574s). Independent reviewer reproduced the same counts (0.736s). This scaffold is not a claim about the unchanged fault probe. |
| `ba9322c` — post-intent and callback probes | Real fsync boundary red: replacing the journal or claim after owned-set intent still issued one GET. Cancellation replaced the pending intent; the first session cancellation exposed a typed-nil SDK interface panic. A separate callback probe accepted changed owner/run/ref/time as original wire facts; its changed-runner negative control already refused. |
| `c3db628` — known-response cancellation probe | After the fixture delivered valid responses, original-context cancellation made SDK poll/ACK/acquire errors overwrite their captured known facts with unknown. The positive response-loss/reopen and scope controls remained green. |
| `d8744d3` — `GOTOOLCHAIN=go1.26.8 go test -race ./livecanary -run '^TestBaseline' -count=1` | Passed in 28.095s. Includes the boundary corrections and the matrix below. |

The independent final review then reproduced an acquisition input-snapshot defect
at `d8744d3`. The adopted actual SDK/TLS test is retained at red `d06edba`:
a synchronous response-boundary mutation of the caller's request slice from 42 to
43 incorrectly changed the known response42 to unknown (acquire1/continuation0,
0.612s). The correction snapshots the input at entry for all response comparisons
and receipt fields; it does not rely on caller-owned storage after the SDK call.
The focused regression passed in 1.689s, the fresh full `go test -race ./livecanary
-count=1` suite passed in 31.507s, and `go vet ./livecanary` passed with
`GOTOOLCHAIN=go1.26.8`. This is synchronous input mutation, not a claim of isolation
from hostile concurrent Go memory access.

The final matrix covers owned-set/update/statistics drift before session POST;
all seven presence/value fields in set, session, nested session and both poll
positions; Available and strict REST source mismatches; sparse lifecycle positive
controls and positive-tuple contradictions; first-batch eligibility; hidden
unknown kinds and ambiguous JSON; count/value conflicts; actual receipt/cursor
ordering and distinct IDs; callback mutation; real post-fsync identity/cancellation
and deadline boundaries; cancelled in-flight SDK requests; response loss/401 and
result-write uncertainty with reopened files; repeated operations; nil-poll
exhaustion; capacity and actual read-only-descriptor write failure; torn/corrupt
history; stable renewal/reference encoding; closed/retained scope; and secret
canaries. Loss fixtures observed one session/ACK/acquire request as applicable,
zero replacement/PATCH or unrelated requests, and no continuation after refusal.

Additional checks passed at `d8744d3` before that narrow snapshot correction:

- `GOTOOLCHAIN=go1.26.8 go test -race ./...` — core 1.455s,
  livecanary 43.711s and existing liveworker 4.985s.
- `GOTOOLCHAIN=go1.26.8 go test -race -tags=g01_live ./cmd/g01-live` — 4.834s.
- `GOTOOLCHAIN=go1.26.8 go vet -tags=g01_live ./...` — passed.
- `git diff --check` — passed.

Root-level combined checks, independent final review, exact-head external Codex
review and hosted CI remain integrator gates.

## Rollback

Before integration there is no production caller to disable. Revert this focused
slice if necessary; do not erase a baseline journal or reuse its resource identity
to make a failed attempt appear fresh. No real state was created by these tests.
