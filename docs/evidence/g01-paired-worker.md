# G01 paired worker scope — issue #46

This bounded worker-side feature follows merged issue #44. It does not implement
controller pairing, acquisition/JIT, a command phase, broker slot or live execution.
Synthetic controller checks prove worker behavior only; actual two-journal
integration remains required before live wiring.

## Compiled feature red

Starting from main `f676e8cc83625b7090b0d62d70fdeefacca42e28`, added the approved
seven-method API and concrete receipt types with a fail-closed implementation.
Both actual private FileJournal tests compile and fail behaviorally:

```text
GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=45s ./liveworker -run '^TestPairedActualFile'
```

The lifecycle test cannot enter the lease-held callback or obtain its durable
binding/create/start/observation/deletion receipts. The bound-state test cannot
produce the original binding needed to test copied-receiver revocation and
reopen refusal. The command exits 1; this is not a missing-symbol compilation
failure. Fixtures use actual temporary worker journals/claims and a synthetic
runtime/controller checker, without any real account admission root or runtime.

At the red commit, implementation and green verification were still pending.

## Implemented contract

The seven methods are `Driver.WithPairedExecution` and `PairedWorker.Binding`,
`Bind`, `Create`, `Start`, `Observe`, `DeleteTerminal`. They require an actual
worker FileJournal, its held execution lease and permanent admission claim, a
runtime supporting the private exact-inspect extension, and the controller's
current-state checker. Existing Runtime, Container, Driver.Run signatures and
Docker decoding/requests are unchanged. Standalone create/start/cleanup refuse a
paired journal before preflight; legacy inspection cannot clear its reservation.

The scope captures approval, journal and runtime before invoking the caller.
Worker inode/claim/current authority and the original bounded context are checked
before requests, independently of the controller callback. Each method enforces
its own worker phase. Entry validates the captured controller inputs and original
set receipt; only Bind can check the full computed binding against the controller's
actual pair intent. The controller must hold its own real lease, validate full
source/set/file binding and exact predecessor records at every stage, and persist
its pair result before later acquisition/JIT. The worker Create checker receives
an independent copy of the complete typed handoff to compare against those records.
These callbacks follow approved-code discipline; they do not independently prove
controller authorization or isolate a hostile Go caller.

Pair order is controller intent, worker bound, controller result. Receipts contain
positive assigned sequences and hashes; they are facts, not standalone authority.
The binding hash is SHA-256 of `gh-runnerd/g01-pair/binding/v1\x00` followed by the
canonical JSON binding. Worker event hashes use
`gh-runnerd/g01-pair/worker-event/v1\x00`, canonical worker JournalIdentity JSON,
a NUL separator, then the canonical assigned Event JSON. Fields have explicit
snake_case names and stable struct order. Stable identity excludes expiry/phases.

New event branches have shared append/reopen shape and order validation, exact
predecessors and full create-profile digests. References return only after assigned
sequence, write and fsync. A private shared append layer preserves OpenJournal's
existing authority renewal before claim initialization; receipt hashing requires
the initialized claim. Event payloads, authority slices and returned state/absence
pointers are copied. Failed writes/fsync poison the live scope. Complete bytes may
still exist after a failed sync, so no claim is made that a poison marker always
survives a crash. Reopen never restores create/start; interrupted operations retain
uncertainty through fresh reporting. Eligible fully known terminal history can
still be cleaned up after a cleanup-only renewal.

Canonical binding is at most 8 KiB, each paired record including its newline at
most 16 KiB, and the entire journal including header/renewals at most 1 MiB. Checks
reserve room for maximum intent/result pairs before the first request and again
before later intents/effects. Create/Observe require 32 KiB, Start 64 KiB for its
verification and start pairs, and DeleteTerminal 96 KiB for inspect/delete/post-read.
This is serialized capacity checking, not a promise that disk writes cannot fail.

Value copies share revocation. Concurrent/reentrant methods refuse promptly.
The original context is capped by approval expiry, parent deadline and ten minutes;
requests are capped at thirty seconds. Return/panic revokes and cancels, drains
in-flight result recording, then releases the lease. Close waits for that lease;
synchronous Close inside the callback is unsupported. Known create/start/delete
responses remain recorded when cancellation arrives at response EOF.

Identical completed receipts are reusable only within their original scope after
current checks; they do not issue another request. Create compares the supplied
handoff and existing environment/labels digests computed with the captured image
profile, so changed JIT refuses without a new preflight. Raw JIT is never retained
in scope or records. Observe always produces a new read/receipt. Deletion requires
an exact controller decision, exact full profile, exited status, explicit exit 0
and all four activity flags present false. DELETE remains non-force and preserves
volumes; its successful result and later exact GET/404 report are separate receipts.
The 404 is named `not-found-reported`, not an independent daemon absence proof.

## Local verification

Compiled feature red is commit `124f0e11de17a54e1b54335b994ba8497a76260b`.
An independent Astra reviewer reproduced both actual FileJournal feature failures.
The focused green suite covers actual worker files/claims, reference hashes after
fsync, copied handoff/event/receipt values, phase/context/current inode/authority
fences, scope copies, cancellation/panic/draining/Close, initial and late capacity
exhaustion, malformed/oversized/torn replay, failed fsync and recovery renewal.

Actual private Unix listeners exercise the production Docker client with actual
worker FileJournal receipts: complete lifecycle, cleanup-only renewal, exited-zero
and presence-aware flag requirements, wrong profile/ID, missing/null/nonzero exit,
controller denial, 409 deletion failure, separate 404, repeated historical results,
and known responses retained after EOF cancellation. A follow-up test exposed
replacement of the cached first absence reference by a later observation; replay
now keeps the first completed absence reference while every Observe gets its own
new receipt. A typed-nil runtime entry test also failed before the narrow scope
validation fix. Fixture setup never accesses a real account admission root.

Run from `experiments/g01-scaleset`:

```text
GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=45s ./liveworker
```

The complete liveworker race suite passed (exit 0, 5.638 seconds).
Full repository checks and independent green-head review are recorded separately
when completed. No real Docker daemon, GitHub API/App, Keychain, account root or
live effect was used. These tests do not prove two-file controller pairing,
controller acquire/JIT ordering or terminal GitHub eligibility. Actual controller
FileJournal integration with its real lease/checker is required before live wiring.
