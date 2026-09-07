# G01f: paired normal-success terminal path

Issue [54](https://github.com/1XP-AI/gh-runnerd/issues/54) adds a private,
same-invocation terminal entry over the paired execution library. It is a
synthetic experiment implementation, not a live command or completion of G01.
`runPairedBaseline` retains its collection-only behavior and performs no cleanup.
The fixed `runPairedTerminal` entry additionally requires the original controller
`cleanup` phase and all four original worker phases (`create`, `start`, `inspect`,
`cleanup`) before any pair prefix, host
request, session or acquisition. No public approval phase, worker method, CLI,
broker operation, recovery entry or admission-root behavior is added.
These phase checks are necessary constraints, not standalone paired or live
authority. The collection-only entry does not require controller cleanup.

## Invocation, evidence and ordering

The controller lease is held before the worker lease. Both approvals, original
credentials, exact journal/claim identities, captured API/runtime dependencies and
original context remain in force. The outer deadline is the earliest caller,
approval or credential deadline, capped at ten minutes. The C-only worker checker
never calls the worker or listener recursively. Worker operations drain and its
lease releases before the controller lease on every return.

The existing acquisition continuation performs JIT, worker create/start and rounds
1–4. After the actual source-anchored SDK Started and Completed callbacks reach the
listener's completion sentinel, terminal mode enters a finalizing state. Retained
methods or interface references to that listener refuse new protocol calls. The
original SDK session object and both leases remain held; no listener mutex is
held across worker calls. Rounds 5–8 run in this finalizer before listener
revocation. There are still eight rounds: the first is immediate, and each next
round starts at least five seconds after the preceding durable result. Each C
round retains its original shared 30-second budget; worker observation retains
its existing separate preflight/inspect bounds.

A fixed terminal parent follows the eight durable rounds. Its evidence resolves
actual pair, acquisition, JIT, handoff, start, approved-source, Started callback,
Completed callback and round-eight references. The last round's four actual
slots determine terminal eligibility; retained earlier positives provide identity
anchors and do not substitute for current absence or completion.

The serial child sequence is:

1. A fresh complete roster equal to the original strict roster anchor, followed
   by a fresh exact owned-set read. Ownership requires the original ID, name,
   group, label and `DisableUpdate=true`. All seven statistics must be present
   and explicitly zero; the earlier demand-permitting predicate is insufficient.
2. An assigned decision resolving those reads, matching Started/Completed
   identities and succeeded callback, current REST job `completed/success`,
   separate SDK and REST runner absence reports, and the worker's current full
   profile with `exited`, explicit exit zero and all four state flags false.
3. Exactly one close of the original session, requiring captured exact-target
   HTTP 204. The SDK object remains populated; `close-acknowledged204` reports
   the acknowledgement and its references, not inferred session absence.
4. One `W.DeleteTerminal` call with the actual pair, container, creation and C
   decision references. The real C-only delete checker resolves the decision
   and durable session-close acknowledgement during the expected terminal child.
   Existing worker preflight/profile/exit checks remain. Its DELETE uses
   `force=false&v=false`; the exact post-inspect 404 receipt is separate.
5. Fresh owned-set and complete-roster rechecks after the worker absence. Then
   one exact original-set DELETE, requiring captured 204.
6. Exactly one marked exact-ID set GET, with no retry. This strict experiment's
   passing row requires a reported 404 before one final fresh complete roster
   read equal to the anchor. The existing roster bound is ten pages, 1,000
   entries and 30 seconds. Missing, 200, 5xx, lost or canceled postchecks retain
   any known DELETE 204 while leaving terminal cleanup unresolved.

SDK runner ID 81 and REST runner ID 9001 are deliberately distinct in the fixture;
no cast establishes their association. The SDK absence fixture supplies the
pinned SDK's `AgentNotFoundException` along with 404; an empty generic 404 remains
unresolved. The Docker 404 fixture supplies its required error-response shape.
Reported 404s, roster equality and zero statistics do not provide an atomic drain
proof. This narrow normal-success row assumes a reviewed exclusive one-job
experiment. It does not establish a general SDK scale-set absence guarantee.

## Failure and persistence reporting

Terminal children use the existing private baseline event branch and the existing
single parent/serial child slots. Each effect is preceded by a durable intent and
fresh authority/capacity checks. Actual replay determines the remaining bounded
records: four before a child intent (intent/result, parent closure and summary),
three once that child intent is durable, and two after the final child result
while its parent remains open. The limits remain 16 KiB per record and 1 MiB per
journal. Every boundary retains the same current C/W authority check. A later
child still requires its own four-record reserve. No effect follows uncertainty, lost authority,
insufficient capacity or persistence failure. A child unknown can close its
actual parent unknown; it cannot resume collection or legacy cleanup.

Historical measurement evidence is derived when the actual eighth result is
replayed, before the terminal parent is written. Its collected measurement and
exact references therefore survive a failed parent write or later cleanup
failure. Cleanup has its own outcome and references. Overall failure remains a
fixed error and unresolved result; a failed final summary has no summary reference.
Eight prior rounds alone never imply terminal success.

The session acknowledgement, worker DELETE receipt, worker absence receipt,
set DELETE acknowledgement and set absence are separate facts. Captured session
or set 204 followed by cancellation remains recorded, but the next operation is
refused. A known worker deletion without its post-inspect absence remains
unresolved and blocks set deletion. If W's deletion/absence records are durable
but C's bridge write fails, the exact W-returned receipt survives in returned
failure evidence only. Failed C bridge/summary references stay zero, and that
returned-only copy is not inserted into a C replay or collection record.

No raw JIT, queue URL/token or remote error is added to terminal records. Existing
worker profile receipts still contain the environment digest derived from their
full environment, including JIT; this is not a claim that all secret-derived
hashes are absent. No standalone JIT digest is added.

Reopen is deliberately non-executing for this entry. A crash loses the supported
same-invocation session-close path; neither matching names, zero counts, a copied
journal nor later observations reconstruct the original session or authorize
another effect. Even a fully acknowledged cleanup does not release permanent
experiment claims. Partial states require separately reviewed operator
reconciliation before any future live entry or successor experiment. There is
no automatic resolution, retry, broad cleanup, stop/kill or claim reset here.

## Synthetic validation and limits

The combined fixture uses actual private C/W `FileJournal`s, the pinned SDK over
private TLS, and concrete Docker HTTP over a private Unix socket. It seeds the
original inventory/create journal history rather than creating a remote set.
Its handlers simulate the runner and daemon: no worker process, Docker daemon,
image pull, GitHub account, App, real admission root or credentials were used.

Recorded checkpoints:

- Compiled feature red `9e185ef`: terminal assertion failed at the fail-closed
  stub, race 0.591 seconds; constructed fixtures did not yet execute the path.
- First actual terminal path `17d1b33`: race 4.104 seconds. One acquisition,
  JIT/create/start, eight rounds, one session close, one non-force worker delete
  with separate absence, one set delete with separate absence, four complete
  roster reads, and zero unexpected calls.
- Actual terminal-parent write failure red `c740d05`, race 1.996 seconds; minimal
  replay-derived measurement fix `857c23d`, complete C sync matrix 11.238 seconds.
- Actual W-receipt/C-bridge failure red `4c86fe4`, race 2.542 seconds; returned-only
  receipt fix `0d75256`, focused receipt and original-phase controls 3.732 seconds.
- Final capacity red `a999883`, race 4.540 seconds: the real last roster result
  consumed part of a valid four-record reserve, leaving 64,562 bytes, but the
  obsolete four-record check rejected completion. Exact 32,768 bytes also
  wrongly failed. Legal whitespace within existing bounded JSONL lines preserves
  decoded events and the pinned inode; every fixture explicitly reopens and
  replays successfully without new requests. The correction's four capacity/
  cancellation cases and five existing final-child/parent/summary sync cases
  passed with race in 9.202 seconds. Less than two maximum records still refuses.
- Original controller-authority red `d1d297c`, race 4.405 seconds: C approval
  without cleanup, captured before journal/API setup, still reached all three
  deletes. The guard now refuses before the prefix. Its missing-C-cleanup,
  unchanged collection-only, four original W-phase and full terminal positive
  controls passed with race in 5.697 seconds.
- Pending-child capacity red `94f6558`, race 5.937 seconds: legal four-record
  reserves passed before real child intents, but their actual bytes reduced
  remaining space to 64,805/64,803 bytes and incorrectly prevented set DELETE or
  the final roster read. Exact three-record cases also failed. Replay-derived
  four/three/two accounting passed focused valid-file capacity, cancellation and
  storage controls with race in 20.246 seconds. Insufficient-three cases still
  refuse; successful set DELETE cannot start another child without four records.

The terminal exact-set GET and DELETE calls use the existing captured `SDKAPI`
methods with the same marked context and target. Those methods forward to the
same pinned SDK calls; this routing refactor adds no behavior or injection seam.
Existing actual eligibility, acknowledgement/cancellation, failed postcheck and
full terminal controls passed with race in 19.320 seconds after the refactor.

The bounded matrix exercises missing original controller cleanup and all four
missing original worker phases; fresh
job/runner/local eligibility; assigned/running nonzero and missing statistics,
ownership/update/roster drift; captured-204 cancellation, original-context
expiry, lost and rejected deletes, failed/absent postchecks; actual C and W sync
failures; capacity after intent; C/W journal and claim replacement at effect
intents; same-inode replay of malformed predecessors, self-references, premature
parent completion and false success shapes; and no new requests after reopen.
All-seven explicit-zero enforcement is a code invariant; the wire matrix does
not separately inject every possible missing/nonzero statistic field.

Terminal cadence assertions use the private clock seam. They are not a
wall-clock spacing trace, and suite durations do not prove per-gap spacing. The
existing collection positive separately invokes the unchanged production
sampler's real timer/guard; that historical control is not a live terminal run.
Sync-error fixtures inject failures at actual file-write/sync boundaries and do
not claim physical power-loss durability. The trust model remains reviewed Go
code and private local files, not hostile same-UID code or copying a used mutex.

The independently reviewed Luna tooling fragment runs complementary tagged
partitions, each with the exact toolchain, race detector, count one and a
120-second timeout, plus one tagged vet pass:

```sh
GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -tags=g01_pair_fixture -race -count=1 -timeout=120s ./livecanary -skip '^TestPairedTerminal'
GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -tags=g01_pair_fixture -race -count=1 -timeout=120s ./livecanary -run '^TestPairedTerminal'
GOTOOLCHAIN=go1.26.8 go vet -C experiments/g01-scaleset -tags=g01_pair_fixture ./livecanary
```

The fixture tag remains excluded with either live command tag. Tooling red
`78a8b3e` and fixes `5f419c9`/`ff70853` were independently verified by the
integrator, including actual partition invocation and failing-test witnesses.
The earlier frozen `cfea2048` source/test tree passed the entire tagged terminal
partition with race in 58.043 seconds. The unchanged distinct-ID/cadence and pinned
SDK listener admission controls passed with race in 3.366 seconds; tagged vet and
diff checks passed. These author checks did not repeat the repository-wide check.
The integrator owns final full-repository, hosted CI, independent and external
review at the frozen head; earlier checkpoint timings are not final-head results.
G01's live baseline, failure matrix, recovery/successor evidence and G04 gates
remain open.
