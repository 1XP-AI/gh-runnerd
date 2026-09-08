# G01 exact observation adapter

Issue [#1](https://github.com/1XP-AI/gh-runnerd/issues/1) remains open. This is an
offline-tested prerequisite, not a live baseline or production G04 interface.
Only package-local tests in this adapter slice call the three new private SDKAPI
helpers. This slice adds no public driver approval phase/API, CLI/broker slot,
journal schema, admission claim or worker operation. The later paired collection
and private terminal journal stages are documented in the [paired baseline](g01-paired-baseline.md)
and [terminal evidence](g01-paired-terminal.md).

## Facts and limits

- `observeSDKRunner` uses the pinned SDK's exact-ID read and requires its returned
  ID/name/scale-set to match the supplied expected reference. A context-local
  transport observer records only the target GET's status. Bootstrap errors
  cannot stand in for that target. A target 404 plus the SDK not-found category
  means only `not_found_reported`; its exception-name classification is not
  proof of absence, ownership or safe removal. There is no name fallback.
- `observeRESTRunner` reads the exact organization runner using controller runner
  authority. ID/name must match and status/busy must be present and recognized.
  A reported false busy value is a fact, not a drain barrier.
- `observeRESTJob` verifies the approved source through a new strict decoder
  sharing the existing run-policy predicate. It reads only attempt 1, one page
  with `per_page=2`, and requires an explicit complete singleton before exact job
  GET. A previously captured numeric job ID, if supplied, must match. Only an
  actual empty array with explicit zero count is pending; missing/null arrays
  remain unresolved. It rejects pagination, identity contradictions, retracted
  positive associations, status regression and changed terminal conclusions
  between list and detail. Forward status progression is allowed.
  The exact job detail must explicitly report `run_attempt: 1`; missing/null
  detail attempts remain unresolved. The attempt-specific list may omit that
  optional field, but a populated contradiction refuses before the detail read.
  This is a stricter experiment evidence policy, not a GitHub wire-presence
  guarantee. Both base and head repository fork flags must be false.

The readers are stateless. SDK runner IDs, REST runner IDs and numeric REST job
IDs have distinct private types. No unused SDK job/request fields, history merger
or identity-join mechanism is added. `present` means validated response facts,
not correspondence with an SDK job or owned worker. Missing/unassigned job
runner fields remain pending. Unknown status/conclusion strings produce a fixed
unresolved result instead of being copied into results. Complete REST job facts
still do not establish worker exit, completion of the SDK request or cleanup.

New REST decoders reject malformed/trailing JSON and duplicate/folded keys,
including nested approved-source identities, while accepting unrelated fields.
The shared run predicate also rejects a forked base repository in the existing
`VerifyRun` path. The original fault-harness decoder and ownership/statistics
fences remain unchanged. The supported SDK decoder remains unchanged; no stricter SDK
wire-validation guarantee is implied.

## Authority and request bounds

The helpers reuse the existing SDK v0.4.0 client, REST API version `2022-11-28`,
1 MiB decoded-body budget, concrete transport, HTTP/1, approved destinations,
redirect/proxy refusal, PATCH refusal and zero automatic HTTP retries. SDK
runner reads can perform the existing credential-bootstrap POSTs before their
GET; these are not credential-free or GET-only transactions.

Each invocation shares one original context/network deadline bounded by 30
seconds, caller cancellation and approval/credential expiry. The approved SDK
usage is serial: its internal client mutex is not cancellable, so this is not a
hard return-time guarantee for arbitrary concurrent SDK callers. No background
request or replacement client hides that limitation.

Run/job endpoints use the separate Actions-read verification token; runner reads
use controller runner authority. Missing/invalid or non-distinct required
verification authority refuses before network, even with an inspect-only
approval. No G02 permission expansion, environment credential lookup, worker
credential extraction or response-URL traversal is added. Results are private,
bounded facts; public errors contain fixed categories, never raw SDK errors,
response bodies, source names, URLs or tokens.

## TDD evidence

Go `1.26.8`, SDK `v0.4.0`; all network fixtures are synthetic and fenced to their
own loopback endpoint. No account admission root, GitHub credential, Docker
endpoint, image, worker or workflow was used.

- Behavioral red `880e003e541accbc7a433d17ee3d9a4555f47aa7` compiled and failed
  `go test -count=1 ./livecanary -run '^TestObserve'` in 0.552s: stub readers could
  not return exact SDK/REST facts or normalized target status. This was a new
  feature red, not a claim of a pre-existing product regression.
- Intermediate red `dff1c4940931553eb75eb5c61173994a0d3e8110` reproduced four
  same-sample failures in 0.541s: missing/null job arrays looked empty, and a
  completed result could change conclusion or regress to in-progress. The fix
  preserves array presence and checks local status progression. The tests also
  cover the complete queued/in-progress/completed transition matrix.
- Focused `go test -race -count=1 ./livecanary -run '^TestObserv'` passed in
  2.114s, followed by `go vet ./livecanary`. Coverage includes actual SDK
  misleading/bootstrap error categories and cancellation, source/runner/job
  identity and decoding faults, IDs beyond floating-point exactness, missing
  authority, single deadlines, context-local provenance, no retries/redirects,
  fixed/chunked/gzip body limits and secret-safe errors.

A disk-space interruption affected an independent review link step; it is an
infrastructure interruption, not a failed application assertion. Source and
red evidence were preserved while only reproducible Go caches were cleared.
After integration of main `bb4a8fee0025e7f3c6ac7974cdb416bafde7bf78`,
`GOTOOLCHAIN=go1.26.8 make check` passed at
`a6c1eff0b463ab1df93837919a87607cfd9e2384`. It included root build/vet/tests/race,
the configured root fuzz target, module/license checks, both offline experiment
modules and their tagged commands, and the configured vulnerability scan. G01
livecanary passed in 6.055s; G02 auth passed in 28.630s. No vulnerability was
reported by that configured scan. This final documentation update changes no
implementation or tests at that head.

GitHub review subsequently found two source/attempt contradictions. Behavioral
red `7747531` failed in 0.570s: a base-only fork passed both source readers, and
missing or conflicting job attempts were accepted and reported as attempt 1.
The test fixtures now use independent base/head repository maps, preserving
isolated fork mutations. Complete matching attempt facts and omitted/null list
attempts are positive controls. The correction checks both fork values,
rejects any populated job-attempt contradiction and requires explicit exact-job
corroboration before returning an attempt fact. Final corrected-head checks and
reviews are recorded in the PR; the earlier full check does not cover this fix.

## Work still required

A future call site must acquire the existing current-authority/journal/admission
lease, derive expected identities from immutable receipts, and durably record
intent before each work-bearing read and its result before decisions. This
adapter does not make caller-supplied IDs owned, erase uncertainty, or authorize
additional inspection through a new directory. Finite broker sampling and paired
worker/controller handoff require their own review.

Live SDK/REST identity correspondence, lifecycle field population/order,
missing-callback reconciliation, exact worker execution/exit/deregistration,
assignment/busy/absence behavior and safe terminal or explicit external
resolution remain unverified. This adapter slice implements no affirmative
recovery, cleanup or successor transition; the private terminal path is covered
separately and does not establish live behavior. The rest of G01's
ACK/acquisition/JIT/session/drain matrix and dependent gates remain open. See the
[identity evidence](g01-identity-reconciliation.md) for the supported-source
facts and unresolved service behavior.
