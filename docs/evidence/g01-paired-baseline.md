# G01 paired execution and collection — issue52

This private collection entry connects the reviewed controller and worker under
their real journal leases. It acquires one verified job, requests one JIT,
creates/starts one worker, and records at most eight observation rounds. The
`runPairedBaseline` entry has no public CLI/broker entry, approval phase/API,
terminal cleanup, admission reset or live authorization. Issue54 adds private
terminal journal stages through a separate `runPairedTerminal` entry; see the
[terminal evidence](g01-paired-terminal.md). G01 remains open.

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

The collection-only baseline journal branch admits one continuation parent and one
serial child: JIT, handoff, start, then four rounds. It stores exact assigned predecessors,
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

For `runPairedBaseline`, the first round is immediate. Later rounds wait at least five seconds after the
prior durable round result. Exact per-gap assertions use a private test clock;
the real positive invokes the production timer/guard. Total suite duration is not
a recorded trace of individual gaps. C reads share one 30-second child context. W.Observe
keeps its existing separate preflight/inspect bounds under the outer lifetime;
there is no claimed 30-second bound for the whole round. Four rounds occur inside
the acquisition continuation and four only after durable matching SDK Completed
callbacks, using the outer scopes after listener revocation. The separate terminal
entry and its additional journal stages are described in the [terminal evidence](g01-paired-terminal.md).

SDK request, opaque job, SDK runner, REST job and REST runner IDs remain separate.
The REST runner target comes from the current or previously recorded positive
REST job association. Tests use SDK runner 81 and REST runner 9001. Later missing/pending/404 facts
do not erase prior positives; contradictions and status/conclusion regressions
stop collection. The assigned round ref plus its fixed reader field identifies
each observation in replay. Numeric equality is never targeting/cleanup authority.

The collection-only result is collected, incomplete or unresolved, with an
optional actual collection ref and explicit none/known-open/open-unknown session
facts. The separate terminal path may additionally report
`close-acknowledged204` after its session close; see the [terminal evidence](g01-paired-terminal.md).
Collected requires eight rounds, matching callbacks and a consistent positive
identity tuple; this collection outcome does not claim job success, terminal
cleanup or absence. A
failed final summary write returns a fixed error, unresolved and no result ref,
while retaining known rounds and outstanding-session refs. There are zero session
close, worker delete or scale-set delete calls in `runPairedBaseline`.

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
current dependency/cancellation checks; original 200 ms bootstrap deadline; actual file
budget exhaustion; malformed/lost JIT and lost Unix create/start responses;
known-create cancellation; concurrent Close/drain; source/association regression;
and retained positives across later empty job listings. A private cadence seam
advances only test cadence; network and authority deadlines always remain real.
One full collection positive retains actual five-second waits. No full suite or live result
is inferred from a focused test; final combined results are recorded below.

The original 200 ms deadline may expire during valid local preparation before
the host request starts. Both early expiry and an entered blocked request must
return unresolved and prevent later operations. Fixture red `924f032` reproduced
the former case at an actual C intent sync (race 1.013 seconds); requiring host
entry incorrectly rejected that safe outcome. The corrected timer test and a
separate synchronized original-parent cancellation test passed with race in
2.158 seconds. The latter waits for actual `/version` entry, then cancels the
parent and observes request exit and zero later operations; it does not claim
to prove timed expiry inside an active request. No production timeout changed.

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
GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=120s -tags=g01_pair_fixture -skip '^TestPairedTerminal' ./livecanary
```

The terminal behavior and terminal persistence/identity partitions, including
their exact storage expression, are maintained in the canonical
[terminal evidence guide](g01-paired-terminal.md). Use the repository's
`scripts/check-offline-experiments.sh` for the complete three-way invocation;
the collection/listener command above is shown here only to identify this
collection entry's partition.

### Current correction checkpoint

The parent `8a848ad5b373866e1a6a01ce2a462ecfe37655df` is intentionally not
described as a schema red: its named `TestPairedTerminalClosedReplayActualFile`
test passes because the schema witness was added only in `03c1b649`. Running that
parent test alone therefore cannot reproduce the finding. The reproducible red
uses the uncommitted, test-only patch
[`g01-terminal-reference-witness.patch`](g01-terminal-reference-witness.patch)
in a detached temporary snapshot; it adds a separate assertion and does not
alter the parent ref, branch, published commits or production files.

From the repository root, this exact recipe checks the parent red and the frozen
03c1 green:

```sh
set -eu
repo="$(git rev-parse --show-toplevel)"
tmp_root="$(mktemp -d "${TMPDIR:-/tmp}/g01-terminal-schema.XXXXXX")"
old_wt="$tmp_root/old"
new_wt="$tmp_root/new"
cleanup() {
	git worktree remove --force "$old_wt" >/dev/null 2>&1 || true
	git worktree remove --force "$new_wt" >/dev/null 2>&1 || true
	rmdir "$tmp_root" >/dev/null 2>&1 || true
}
trap cleanup EXIT

git worktree add --detach "$old_wt" 8a848ad5b373866e1a6a01ce2a462ecfe37655df
test "$(git -C "$old_wt" rev-parse HEAD)" = 8a848ad5b373866e1a6a01ce2a462ecfe37655df
git -C "$old_wt" apply --check "$repo/docs/evidence/g01-terminal-reference-witness.patch"
git -C "$old_wt" apply "$repo/docs/evidence/g01-terminal-reference-witness.patch"
set +e
GOTOOLCHAIN=go1.26.8 go test -C "$old_wt/experiments/g01-scaleset" -race -count=1 -timeout=120s -tags=g01_pair_fixture ./livecanary -run '^TestPairedTerminalPersistedReferenceSchema$' -v
old_schema_status=$?
set -e
test "$old_schema_status" -eq 1

git worktree add --detach "$new_wt" 03c1b649b6081b7ac868bb830f02a6d07319dc95
test "$(git -C "$new_wt" rev-parse HEAD)" = 03c1b649b6081b7ac868bb830f02a6d07319dc95
git -C "$new_wt" apply --check "$repo/docs/evidence/g01-terminal-reference-witness.patch"
git -C "$new_wt" apply "$repo/docs/evidence/g01-terminal-reference-witness.patch"
GOTOOLCHAIN=go1.26.8 go test -C "$new_wt/experiments/g01-scaleset" -race -count=1 -timeout=120s -tags=g01_pair_fixture ./livecanary -run '^TestPairedTerminalPersistedReferenceSchema$' -v

G01_TERMINAL_JOURNAL_OUT="$tmp_root/old-journal.jsonl" G01_TERMINAL_REFS_OUT="$tmp_root/old-refs.txt" GOTOOLCHAIN=go1.26.8 go test -C "$old_wt/experiments/g01-scaleset" -race -count=1 -timeout=120s -tags=g01_pair_fixture ./livecanary -run '^TestPairedTerminalReferenceJournalBytes$'
G01_TERMINAL_JOURNAL_OUT="$tmp_root/new-journal.jsonl" G01_TERMINAL_REFS_OUT="$tmp_root/new-refs.txt" GOTOOLCHAIN=go1.26.8 go test -C "$new_wt/experiments/g01-scaleset" -race -count=1 -timeout=120s -tags=g01_pair_fixture ./livecanary -run '^TestPairedTerminalReferenceJournalBytes$'
if cmp -s "$tmp_root/old-journal.jsonl" "$tmp_root/new-journal.jsonl"; then
	echo 'unexpected identical old/new terminal journal bytes' >&2
	exit 1
fi
printf 'old refs: '; sed -n 's/^.*ref=//p' "$tmp_root/old-refs.txt"
printf 'new refs: '; sed -n 's/^.*ref=//p' "$tmp_root/new-refs.txt"
```

The parent run of the old named test is green (race, 5.992 seconds), which is
why it is not used as red evidence. With the temporary witness patch, the old
snapshot fails (race, 0.502 seconds) with CamelCase `Pair`/`Acquire`/`JIT` and
the other untagged keys; the same witness passes on 03c1 (race, 1.431 seconds).
The fixture's deterministic terminal and collection-summary JSONL events also
produce different `controllerEventRef` values: old
`23:3afde60c183b38bffd4035e7d71f3f05bc91f1cece0f9c844dfe3406eafaf9c5`,
`24:1c0efa1b52eae355be78bc02e43f5068e3e4c6a33aab28e4065ea021ad48393c`;
new `23:242dbb81ea3443a21e9c111d9f0e224c872212cb36686436c537c3816bedc71f`,
`24:88cef33e2f0f772e200e2aefecdbc8560c7315fe868861b5022b78eddef0c6b4`.
The recipe compares the complete bytes, not only the displayed hashes, and
observes `cmp` status 1.

The unchanged correction witness on the frozen head is:

```text
GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=120s -tags=g01_pair_fixture ./livecanary -run '^TestPairedTerminalClosedReplayActualFile$' -v
```

After explicit tags were added to both persisted terminal-reference structs,
the unchanged focused correction command passed in 5.867 seconds. The complete
terminal persistence, authority-boundary, replay and worker-receipt group passed
in 42.087 seconds, and tagged vet, shell syntax and `git diff --check` passed.
The full offline gate also passed with the unchanged pins and bounds:

```text
GOTOOLCHAIN=go1.26.8 GO=go bash scripts/check-offline-experiments.sh
```

The run completed all three G01 partitions and G02's checks: G01 collection,
terminal behavior and terminal storage took 105.568, 54.361 and 42.889 seconds
respectively; G02's library and CLI checks took 27.225, 2.258 and 1.513 seconds.
`GOTOOLCHAIN=go1.26.8 make check` also passed build, vet, unit/race tests, fuzz
smoke, dependency/license checks, the offline gate and pinned govulncheck (no
vulnerabilities found).

Public CI run `34168590856`, attempt 2, job `101885714771` remains a gap. Its
preceding checks passed, but the offline step's first G01 package command hit
the package-wide `-timeout=45s`; the failure log listed
`TestResponseBudgetAppliesAfterGzipDecompression` while the stack was blocked
in `client.Get`/`responseBudgetTransport.RoundTrip`. Local exact single-test
and full-package race checks passed, so this does not establish an individual
test timeout and no limit or response-budget code was changed. The two current
P2 findings are locally reproduced and corrected; external exact-head Codex
re-review and hosted CI remain separate gates, while earlier findings remain
historically stale/outdated per the review reader.

Rollback is source-file-scoped and journal-state-offline-only. `controllerEventRef`
hashes the domain prefix, serialized journal identity, a NUL separator and the
`json.Marshal(Event)` bytes; changing the terminal-reference tags therefore
changes both the persisted JSONL and every affected event hash. The old/new
fixture witness above proves that the old and new formats have different
serialized bytes and hashes, so cross-version replay compatibility cannot be
assumed in either direction. This slice supplies no migration, translation,
reset or online rollback path: use a fresh offline fixture/journal for the
selected source revision and quarantine any existing cross-version journal for
reviewed inspection. Never rewrite, truncate, reset or delete a journal/claim as
rollback. If a future live resource or authority is unknown, retain its
journal, claim and session/worker/set references and the resource itself for a
separately reviewed reconciliation; no live resource was touched here. The
published implementation history, test partitions, limits and prior evidence
remain unchanged.

At the initial frozen head `6a378ef`, the pre-issue54 collection-era tagged
livecanary race suite passed (exit 0, 99.814s), including the real-cadence positive,
legacy listener tests and paired failure matrices. Root also completed full make
check on that historical head.

After the final REST normalization correction, focused replay/cross-round/early-
stop/distinct-ID/canceled-JIT race tests passed (11.265s). The added actual worker
bound/paired-record count control passed (2.976s); it strengthens evidence for
existing cached behavior and is not claimed as a new behavioral red. Tagged vet
for livecanary and the consumed liveworker helper passed; diff/format checks are
clean. No dependency or worker runtime/API changes were made.

The root integrator owns the changed-head full make check, hosted CI and exact-
head independent/external review. These remain pending at this correction
checkpoint. No live or platform evidence is claimed.

Issue54's private terminal journal stages and terminal result are documented in the
[terminal evidence](g01-paired-terminal.md). This collection-only record does not
infer terminal cleanup, live behavior or the terminal failure matrix from its
historical checks; G01 and its remaining live/platform gates stay open.
