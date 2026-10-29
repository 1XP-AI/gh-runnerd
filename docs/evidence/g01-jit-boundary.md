# G01 signed offline JIT physical boundary

This unit is offline synthetic evidence for issue #1, based on
`3cd3cd9e997dbf2f916d333fd5bda44e72667543`. G01 remains open and In progress.
It does not authorize a live experiment or dependent production recovery.

The signed experimental broker and fixed TEST controller execute both
`before-jit` and authenticated `jit-loss` through protected descriptors, the
receiver, a real FileJournal and the pinned scaleset SDK v0.5.0 over private
synthetic TLS. Both modes require the original raw approval and exactly seven
authenticated completed-create events before preparation/remint and again at
receiver consumption, under the journal mutex before credential input. The
broker has already read its management input before preparation; this is not
evidence of zero broker credential reads.

The dedicated probe deliberately requires an already-created owned set with
all seven statistics explicitly zero and an explicit empty stable-name runner
list. It supplies no workload, positive-demand, acquisition or drain evidence.
Those separate remaining G01 phases are still required.

Each invocation creates a fresh HTTP/1 transport with connection reuse, SDK
retries, redirects and SDK logging disabled. Its finite physical gate checks
all requests, including bootstrap and unmarked traffic. Controller request
totals are 9/10 for legacy management scope and 7/8 for organization-only scope
(`before-jit`/`jit-loss`); these exclude broker and original create requests.
The actual SDK constructs and validates the JIT POST. `before-jit` persists
known-not-attempted evidence and returns a private planned sentinel before
physical entry. `jit-loss` persists possible-forward intent before its sole
physical entry, then validates and discards a bounded successful response
inside the adapter before generic JIT capture or SDK decoding. The SDK returns
a nil DTO and private suppression sentinel. No response runner ID is retained.
Go buffer clearing is best effort and makes no secure-memory-erasure claim.

Only an expected local stop or validated suppression permits the final stable
name GET. Its identity is independent of the hidden response: the focused
fixture deliberately uses different response and lookup IDs. Inventory derives
only from the fixture's own committed creation, following
[the causal fixture contract](g01-jit-causality.md). A physical disconnect
without commit records one possible-forward attempt, discovers nothing and
causes no automatic post-error GET. Successful suppression can observe either
presence or absence; neither establishes live freshness or noncommit.

Version 1 `jit_boundary` records form an isolated 13/14-record grammar. Physical
references hash exact prior lines, including newlines, and the original header
and create prefix under new domains. Required zero, false and nested members
are strict. Records remain within 16 KiB per raw physical line and the unchanged
1 MiB journal limit, with 16-slot worst-case capacity reserved before effects.
Every incomplete prefix, consumption-only history and complete result retains
the worker reservation and quarantine on reopen. There is no partial resume,
fresh-nonce retry, alternate-mode escape, renewal, release or cleanup authority.
The existing one read-only inspect slot remains available. Direct ordinary
`Run(jit-loss)` keeps its historical harness behavior and is never upgraded to
this evidence; direct `Run(before-jit)` refuses.

The behavioral RED used the actual TEST broker/controller commands. Minimal
finite-phase scaffolding was needed to serialize valid fixtures; this was not
an untouched-base execution. The admitted commands failed on missing typed
physical evidence, with physical JIT counts 0 and 1 respectively. Setup/compiler
failures are recorded separately and are not behavioral RED evidence.

Both mandatory omission checks failed causally. Removing only the final
pre-send stop allowed the fixture's real POST and failed `physical JIT count =
1, want 0`. Removing only interception let a nonnil DTO cross the immediately
observed SDK return boundary, triggering a safe boolean/count assertion before
any later defensive error. Both safeguards were restored and the protocol
tests passed. Synthetic secret values are excluded from command output and
journal evidence. No session, ACK, acquisition, worker/container launch or
delete operation is admitted.

Focused tests cover physical tuple/header/body drift, finite bootstrap/status
and retry failures, complete response and observation presence, raw approval
drift before remint, atomic pre-input history races, identity/capacity failures,
every append's write/short-write/fsync/cancellation/expiry boundary, strict
schema/raw-reference replay, cloning, reopen and cross-open proof refusal.
Current-source race TEST helpers carry source/tool/binary hashes; file existence
alone is not provenance. The offline checker runs the JIT family in its own
serial storage/cancellation, expiry, admission/history, physical-mutation and
complementary remainder partitions, each retaining
the 120-second limit and disjoint from the old families; actual command
cases remain in their existing family exactly once. The original combined race
family exceeded its cumulative 120-second package budget; that run is a failure,
not a pass, and motivated only this bounded routing correction. The original
180 ms expiry fixture also expired before later target hooks under race; it
remains failed evidence. Its corrected standard two-second context deadline
must reach each selected fsync hook, expire there with `DeadlineExceeded`, and
permit no later physical traffic; a pre-target expiry still fails.

The measured combined core race run passed with a reported package duration of
120.832 seconds (including race exit delay), leaving insufficient timeout
headroom. Physical mutations therefore receive their own final partition.
The measured constituent test-body sums of 43.16 and 76.44 seconds project the
two new partition budgets; they are not separately executed partition timings.
All assertions and the 120-second package limits remain unchanged.

Live discoverability, consistency, name uniqueness, ownership/activity metadata,
safe deregistration/reissue and service timing remain unverified. Prior PR116
and main CI results are supplied historical evidence, not validation of this
unit. Independent source/security review, exact-head GitHub Codex review and PR
quick checks, merge and resulting main CI remain separate coordinator gates.
