# G01 complete runner inventory — issue #50

The existing create and cleanup phases used the inventory digest as a prerequisite
for effects. Missing response fields previously decoded to a known empty inventory:
`{}` before create allowed CREATE, and `{}` after a valid empty initial inventory
allowed cleanup DELETE and a successful return. Counts that changed between pages
could also produce a successful digest assembled from inconsistent declarations.

## Implemented contract

Inventory now uses a private page decoder through the unchanged shared HTTP reader.
Each page requires a non-null object, a present non-null integer `total_count`, a
present non-null `runners` array, and runner objects with positive integer IDs.
There are at most 100 records per page, 1,000 total runners and ten requests. The
first declared total must remain unchanged; the cumulative IDs must be unique and
exactly match it before any digest is returned. Empty unfinished pages, excess
records and exhausted pagination refuse. Impossible totals stop on the first page.

The existing object-key collision scan rejects exact duplicates and case/Unicode
folded collisions, including escaped keys and nested metadata. Lone aliases decoded
consistently by Go remain accepted. Unrelated metadata remains forward compatible;
the token scan uses `UseNumber` so ignored numbers outside float64 range are not
accidentally rejected. Integer runner IDs retain their exact int64 precision.

The API signature, fixed endpoint, installation credential, HTTP/context behavior,
response limits and normalized errors are unchanged. Valid inventories retain the
exact SHA-256 digest of their sorted JSON ID list. Explicit empty inventory retains
the previous digest of JSON `null`, preserving valid existing journal compatibility.
No journal is cleared or rewritten. This cannot retroactively validate a historical
digest originally produced from a malformed response.

A stable count and complete bounded enumeration do not provide an atomic snapshot
against concurrent same-count membership changes. This correction adds no terminal
reconciliation, paired roster anchor, reservation release or live authorization.

## Meaningful red

Commit `85ed788bd030ecc07f41b5e220d88af1b1efbb7a` adds the actual private TLS
reader tests and the pinned SDK/TLS/private FileJournal create/cleanup witness.
The focused race command compiled and exited 1 in 0.843 s: fifteen reader cases
accepted incomplete or ambiguous facts, CREATE occurred once after malformed input,
and DELETE occurred once after a valid initial empty response followed by malformed
cleanup input. Both effect calls returned nil errors. Valid controls passed.

Commit `1d2bc4a74c8af375c4d61a9618791bd187669f07` extends that red with page/total
bounds and collision coverage, additional missing/null/collision effect witnesses,
exact valid digest checks, and transport controls. The command exited 1 in 1.838 s.
The added impossible-total probe observed ten GETs instead of refusing after one.

```text
GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=45s ./livecanary -run '^TestInventory'
```

## Actual fixture coverage

The focused green race suite passed in 2.688 s before integration. Private TLS
fixtures cover empty/nonempty and maximum 100-by-ten enumeration, unchanged sorted
and maximum-int64 ID digests, lone case/Unicode aliases, ignored large-number
metadata, missing/null/wrong-type fields, duplicate/escaped/folded keys, changing
counts, duplicate IDs across pages, incomplete/excess/over-budget pages and records.

Pre-cancelled and in-flight cancelled calls, a bounded deadline, expired credentials,
non-200 status, redirects, closed connections, truncated responses, ordinary and
gzip-decoded oversized bodies, and trailing secret-canary documents all return
sanitized failure. No raw response canary enters the actual journal.

The legacy effect fixtures use actual temporary FileJournal/admission files, the
production Inventory reader, and actual pinned SDK discovery/create/owned-read/delete
calls over the private TLS listener. They assert zero CREATE or cleanup DELETE for
malformed responses and preserve the valid empty lifecycle. Only the separately
tested remote policy Preflight is suppressed; this is not evidence of live GitHub
or full production authority verification. Transport dialing is restricted to the
fixture listener and no real account admission root is used.

Merged main `1fece28a33e3abf1545060a710af9e5612b55953` was integrated without
conflict. Combined module checks and final review results are recorded below.

```text
GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=120s ./...
GOTOOLCHAIN=go1.26.8 go vet -C experiments/g01-scaleset ./...
git diff --check
```

All passed after integration: core 1.609 s, livecanary 35.908 s, liveworker 8.760 s;
vet and whitespace checks exited 0. These are local fixture results. Independent
review, full repository checks, hosted CI and external exact-head review were
pending when this author evidence was committed; the PR records their outcomes.
