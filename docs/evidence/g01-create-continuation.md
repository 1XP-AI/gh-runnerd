# G01 completed-create continuation

This offline correction lets the private paired preparation and baseline
consumers recognize the history written by authenticated controller create.
It grants no live authorization and does not complete G01 #1.

`parseCompletedCreateHistory` is a pure structural parser shared by paired
preparation, held baseline listener construction, and replay of the prefix at
the first baseline record. It accepts the six ordered empty-work create records
(phase, inventory, discovery intent/result, create intent/result), optionally
preceded by one structurally valid create consumption at physical sequence 1.
Payloads must match the exact typed shapes, inventory is a lowercase SHA-256
hex digest, and the create result has a positive ID. Authority records remain
structural here; the existing journal opener validates their renewal chain.

The parser returns the original inventory/create events and replay state,
never a capability. The receipt-backed writer's inventory sequence 3 and create
result sequence 7 retain their original `controllerEventRef` hashes. Legacy
six-record journals remain eligible under the existing ownership/current
approval and trusted-journal policy. Short listener fixtures have been corrected
to use the actual six-record grammar. Ordinary empty and partial journals still
reopen for nonbaseline inspection/recovery; existing suffix replay and generic
phase/work/uncertainty/deletion fences are unchanged.

Stored consumption has nonce, hashes and key ID, but no signed receipt bytes,
trust root, receipt expiry, or source. Historical signature/provenance checks,
raw snapshot equality against typed `Approval`, and reconstruction of the
per-open proof's identity/parent/deadline are impossible from those records.
Structural acceptance is not cryptographic authentication. A well-shaped
fabricated hash cannot authorize credential reads, proof creation, or another
create. Completed history survives the original invocation, but create cannot
resume with the same receipt, a fresh nonce, or a reopened proof. Continuation
still independently requires its current authority, credentials, context and
worker bindings. Inspect/cleanup-only renewal cannot restore paired/create
permission. Same-user filesystem access is not hostile-code isolation.

Behavioral tests start with actual `runControllerHandoffCreate`, a synthetic
signed receipt, strict credentials and a counted fake API: exactly one reader,
factory and create, and seven fsynced events. They reopen through the same
private fixture admission root and call actual paired preparation without
rewriting journal bytes. Baseline and paired fake-HTTP collection/reopen tests
retain physical references, two distinct singleton ACKs, one acquisition and
one continuation, and refuse repeated create/protocol effects. Negative cases
cover strict metadata/payload shapes, handoff duplication/phase/order, each
missing/duplicate create step, physical positions, durable prelude mutations,
raw snapshot hash differences, foreign identities, and renewal limits.

The actual initial test failed at paired preparation on the old parser. The
new receipt-backed continuation tests passed, rejected the same history when
optional-consumption acceptance was temporarily removed, and passed again
after restoration. A focused normal batch passed in 151.251 seconds before a
first-baseline physical-sequence correction. Its added regression reproduced
acceptance of sequences 0/1/7/9 where 8 was required; the corrected source then
passed all continuation tests in normal mode (2.325 seconds). The implementation
report separates that earlier broad batch from the final delta evidence. The
focused race batch passed on the final Go source (370.876 seconds); tagged CLI
quarantine checks passed, and ordinary builds still exclude both live commands.
The report records exact commands, source SHA and limits.
No real service, workflow dispatch, Docker daemon, SSH, credentials, packet body
or live cleanup is exercised. Paired terminal's direct set delete remains its
existing experimental route, separate from generic conditional cleanup and
production provider qualification. Independent review, exact-head GitHub Codex
review and PR quick checks remain subsequent gates.
