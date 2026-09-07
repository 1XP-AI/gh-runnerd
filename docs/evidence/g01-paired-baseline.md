# G01 paired execution and collection — issue52

This private library connects the reviewed controller and worker under their real
journal leases. It acquires one verified job, requests one JIT, creates/starts one
worker, and records at most eight observation rounds. It has no CLI/broker entry,
new phase, cleanup, admission reset or live authorization. G01 remains open.

## Recorded protocol

`runPairedBaseline` requires concrete controller/worker FileJournals, SDKAPI and
Docker, captures their approvals/dependencies, and holds C before W. W drains and
releases before C. The earliest caller, approval, credential or ten-minute deadline
bounds the original scope. Current authority is checked after durable intents and
before each actual operation. Cached W.Bind verifies the original receipt without
another W write/request; the actual worker-journal control checks exactly one bound
record and 23 total paired records for create/start and nine inspections. Its C-only
checker never enters the listener or W again.
This also works inside the listener's mutex-held acquisition continuation.

The same invocation must record C pair intent → actual W bound → C pair result,
then concrete Docker host/image Preflight and the complete organization-roster
anchor, before the listener's owned-set/session/source/ACK/acquire stages. The
anchor includes the actual count, accepted pages and strict-stable-total-v1
provenance, and must match the earlier create inventory digest. It does not make
that historical digest a complete or atomic remote snapshot. Legacy Inventory
keeps its original reader, errors, cancellation behavior and exact sorted-ID
encoding (including JSON null for explicit empty inventory).

The existing baseline journal branch admits one continuation parent and one serial
child: JIT, handoff, start, then four rounds. It stores exact assigned predecessors,
16 KiB maximum records and a 1 MiB cumulative budget. The C handoff intent leaves
its own HandoffIntent ref zero; after append assigns that ref, the exact handoff is
passed to W and recorded in C's result. Unknown child facts can be followed only by
the parent's unknown closure and a collection summary when storage permits.
Malformed identities, enums and predecessor refs are rejected even in unknown
records. Pending/partial and reopened completed prefixes never resume effects.

JIT validation requires the exact marked POST/status200, bounded unambiguous wire,
SDK agreement, the expected positive runner tuple and base64 input of 16 bytes–1 MiB
(with the existing response-body cap also applying). A bounded expected runner
tuple observed at the transport survives cancellation as unknown evidence; it
cannot authorize create without SDK agreement and current authority. No JIT text,
standalone JIT hash, token, raw response or raw SDK error enters C records. Exact
W container receipts retain the existing EnvDigest and LabelsDigest: EnvDigest
is derived from the full worker environment, which includes JIT. That existing
profile proof is required by W and is not removed or described as secret-free.

## Observation and outstanding work

Each round is one C intent/result with four closed typed reader slots. Nil means
not attempted. A nonnil zero W receipt means Observe returned before an owned
inspect receipt; it is not reported absence. REST not-addressable is distinct
from 404 and sends no request. Reads are serial, with a current pair/capacity check
before each; a contradiction or failed reader prevents later readers. An unknown
round retains every earlier returned fact and the exact W receipt when available.

The first round is immediate. Later rounds wait at least five seconds after the
prior durable round result. Exact per-gap assertions use a private test clock;
the real positive invokes the production timer/guard. Total suite duration is not
a recorded trace of individual gaps. C reads share one 30-second child context. W.Observe
keeps its existing separate preflight/inspect bounds under the outer lifetime;
there is no claimed 30-second bound for the whole round. Four rounds occur inside
the acquisition continuation and four only after durable matching SDK Completed
callbacks, using the outer scopes after listener revocation.

SDK request, opaque job, SDK runner, REST job and REST runner IDs remain separate.
The REST runner target comes from the current or previously recorded positive
REST job association. Tests use SDK runner 81 and REST runner 9001. Later missing/pending/404 facts
do not erase prior positives; contradictions and status/conclusion regressions
stop collection. The assigned round ref plus its fixed reader field identifies
each observation in replay. Numeric equality is never targeting/cleanup authority.

The returned outcome is collected, incomplete or unresolved, with an optional
actual collection ref and explicit none/known-open/open-unknown session facts.
Collected requires eight rounds, matching callbacks and a consistent positive
identity tuple; it does not claim job success, terminal cleanup or absence. A
failed final summary write returns a fixed error, unresolved and no result ref,
while retaining known rounds and outstanding-session refs. There are zero session
close, worker delete or scale-set delete calls here.

## Synthetic TDD evidence

The compiled feature red `1b3d898` used always-unresolved stubs. Actual generated
C/W FileJournals and private TLS/Unix fixtures initialized successfully; paired,
empty-roster and 102-runner roster assertions failed (race exit 1, 0.469s). Embedded
legacy Inventory controls passed. This was a new unimplemented feature, not an
existing product regression. The first actual integrated positive at `caeee8d`
passed race 37.400s with real five-second cadence.

Subsequent immutable behavioral reds and corrections cover:

- `f0268df`: a contradictory REST job allowed a later worker read (2 versus 1
  exact container GETs, 0.964s); per-reader consistency now stops it.
- `2209392`: canceled observed JIT 200 lost its tuple (0.732s), and a real final
  collection fsync failure incorrectly returned collected (matrix 12.096s).
  Bounded unknown facts and explicit storage failure now survive. Independent
  actual full-cadence review reproduced both on the prior immutable checkpoint.
- `3ffe8d6` and `569d802`: actual same-inode reopen accepted malformed unknown
  receipt/sample facts (2.005s and 2.127s); closed replay validation now rejects them.
- `be89071`: a ten-second simulated reader latency consumed all required gaps
  under a start-based anchor (2.119s); the clock now anchors at durable completion.
- `7d35f3d`: the final independent review's present-without-ID finding and six
  adjacent impossible REST job normalization forms reproduced through same-inode
  replay (2.321s). Positive IDs now require exact detail provenance, and present
  requires the complete positive runner association. Empty pending lists,
  unresolved/404 outcomes and partial positive pending associations remain valid.

The matrix uses actual controller stage intent/result fsyncs, selected worker
record fsyncs and same-file reopen; C/W journal and claim inode replacement;
current dependency/cancellation checks; original 200 ms host deadline; actual file
budget exhaustion; malformed/lost JIT and lost Unix create/start responses;
known-create cancellation; concurrent Close/drain; source/association regression;
and retained positives across later empty job listings. A private cadence seam
advances only test cadence; network and authority deadlines always remain real.
One full positive retains actual five-second waits. No full suite or live result
is inferred from a focused test; final combined results are recorded below.

The C fixture seeds inventory/create receipts. It does not execute remote policy
Preflight or prove a live original scale-set creation. Docker fixtures are private
Unix HTTP servers, not a daemon or worker process. No real admission root, account,
credential, runner, workflow, image or runtime is accessed.

## Static verification and remaining gates

The generated-root worker helper and integration tests require
`g01_pair_fixture && !g01_live && !g01_worker`. The helper accepts no caller root or
opener override and can reopen only its own canonical private directories.
Luna max supplied the two-file tooling change, preserving behavioral red
`f4561267` and green `8803fa5d` (integrated as `8272bfd`/`a4f095e`). Its generated
fixture proves a failing tagged test fails actual make check; the positive
control also verifies the Go 1.26.8 pin, 120-second timeout and tagged vet forwarding.
Root independently reproduced that tooling red and green.

From `experiments/g01-scaleset`, required fixture checks are:

```text
GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=120s -tags=g01_pair_fixture ./livecanary
GOTOOLCHAIN=go1.26.8 go vet -tags=g01_pair_fixture ./livecanary
```

At the initial frozen head `6a378ef`, the full tagged livecanary race suite passed
(exit 0, 99.814s), including the real-cadence positive, legacy listener tests and
paired failure matrices. Root also completed full make check on that head.

After the final REST normalization correction, focused replay/cross-round/early-
stop/distinct-ID/canceled-JIT race tests passed (11.265s). The added actual worker
bound/paired-record count control passed (2.976s); it strengthens evidence for
existing cached behavior and is not claimed as a new behavioral red. Tagged vet
for livecanary and the consumed liveworker helper passed; diff/format checks are
clean. No dependency or worker runtime/API changes were made.

The root integrator owns the changed-head full make check, hosted CI and exact-
head independent/external review. These remain pending at this correction
checkpoint. No live or platform evidence is claimed.

The next separately reviewed terminal slice must move the same four post-Completed
rounds into the original-session finalizer before revocation, then establish
eligible completion/roster/zero-work facts and exact non-force deletion/session/set
results. It must not add eight more rounds, retry ambiguous effects, infer absence
from errors, release claims automatically or bypass G04 and the remaining G01 gates.
