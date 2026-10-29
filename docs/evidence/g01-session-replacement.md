# G01 offline session replacement experiment

Issue #1 has an executable, synthetic offline replacement experiment. This does
not complete G01 or full G02, qualify a live session operation, or implement
production recovery. Both compiled experimental registries remain empty by
default. Existing create-v1 bytes, 30-second create budget, paired normal-success
policy, legacy quarantines and SDK v0.5.0 pin are preserved.

The exact signed phase is `session-replacement`, using the existing v2 binding
and audience `g01-controller-session-replacement`. Broker and controller must
independently admit an exact finite grant and independent workflow/issuer roots.
The original controller approval must include this phase and separate workflow
verification authority. The finite controller phase set has ten entries; broker
ledger capacity is derived from those slots plus its two special slots.

Admission requires exactly the authenticated create consumption followed by six
completed-create events. Original canonical approval, exact raw approval bytes,
physical journal/claim/state identity and the original create consumption must
agree. Preparation requires the original approval file, rather than only its
decoded value. The same prefix is checked atomically before phase consumption
and credential input. Only the current per-open proof may enter the experiment;
legacy Driver.Run, fresh nonces, renewal and reopening cannot restore it.

Within one parent lifecycle of at most ten minutes, each request has a bounded
30-second child context. The experiment independently reads the exact owned set
and absent approved worker, opens SDK object 1, and starts a fresh listener with
cursor 0 and capacity 1. A strict controlled JobAvailable response is held inside
GetMessage, before any real callback or ACK. Present real message ID 0 is valid;
missing/null IDs and synthetic initial ID -1 cannot stand in for it.

A durable retirement fence precedes one exact object-1 Close/204. Its known,
persisted result permits one new session with a different UUID. A completely
fresh listener polls with argument 0 and no inherited cursor query, records a
controlled response or explicit 202, then stops before a real callback. The old
held result passes through a return-generation fence and both listeners join.
Independent final set/worker reads precede the explicitly authorized exact
object-2 Close/204 and a durable closed-session summary. The set and all observed
work reservations remain retained. Neither zero statistics nor absent worker
evidence releases them or establishes server freshness/redelivery timing.

The dedicated physical adapter gates every request, including unmarked traffic,
SDK retries and bootstrap. It binds stage, exact origin, tenant path, URL/query,
Request.Host, authorization bytes, unique capacity header and one-shot count.
Only the pinned registration-token/runner-registration bootstrap is admitted;
ACK, acquire, generate-JIT, PATCH, runner deletion and set deletion are refused.

Replacement journal version 1 is isolated from baseline records, with a maximum
of 32 records of 16 KiB each inside the 1 MiB journal. Capacity is reserved before
effects. Records link exact stored event bytes, original create consumption,
complete create-prefix digest, current phase consumption, approval and physical
identity. Append, clone, strict decode and OpenJournal replay enforce the same
grammar and zero-presence rules. Only bounded typed work/statistics/status/cursor
facts are retained; URLs, tokens, raw bodies and raw errors are excluded.

Cancellation, expiry, invalid identity, ambiguous results or unhealthy storage
stop subsequent network operations. A lost first-close result forbids recreate;
a lost recreate forbids all closes; a failed final-close result remains unknown.
All exit paths retire local delivery, release barriers and join without network
cleanup. Replacement history fences legacy effects; the existing independent
inspect slot remains a single read-only observation.

Offline coverage uses the actual TEST broker and fixed TEST controller command,
protected descriptors, real FileJournal and pinned SDK over synthetic TLS. The
focused family covers present-zero/41, redelivery/202, missing/null identity,
close/open losses, every append/sync boundary, cancellation/expiry, physical
identity/auth/header/retry drift, replay and raw-byte corruption. These synthetic
observations make no live service or runner claim. Exact candidate review,
GitHub Codex review and PR quick check remain coordinator-owned gates.
