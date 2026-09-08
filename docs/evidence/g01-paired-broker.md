# G01g: paired terminal executable and bounded broker handoff

Issue [60](https://github.com/1XP-AI/gh-runnerd/issues/60) and PR
[62](https://github.com/1XP-AI/gh-runnerd/pull/62) connect the reviewed paired
terminal sequence to one tagged `g01-live` executable and a dedicated
`g01-broker` mode. This is an offline experiment continuation, not a live
authorization, production daemon, or closure of G01/G02.

## Reviewed baseline and finding ledger

The required first step was a normal fetch and merge of reviewed
`origin/main` at `8dd64adc551ba5174892807a678e8bc614d0a474` (the merged CI fix).
It produced merge commit `a5fcffd`; no rebase, amend, force update, workflow
replay, or live operation was performed. The implementation and focused tests
were then completed through source head
`895a478eb1ed894710c75b3426e23cb3b1bceac9` (the evidence-only commit follows
this tested source head).

Both settled Luna/max reports were read: `/tmp/g01-paired-broker-review-ad7c2cf.md`
and `/tmp/g01-paired-review-ad7c2cf.md`.

The exact-head Codex surfaces were also read, including the stale inline
finding, all current inline findings, review summaries, and the prior issue
comment:

- stale phase-receipt finding: [discussion 3955069674](https://github.com/1XP-AI/gh-runnerd/pull/62#discussion_r3955069674)
- historical ledger mode: [discussion 3955069682](https://github.com/1XP-AI/gh-runnerd/pull/62#discussion_r3955069682)
- daemon-ID contract: [discussion 3955069689](https://github.com/1XP-AI/gh-runnerd/pull/62#discussion_r3955069689)
- canonical prerequisite history: [discussion 3955590270](https://github.com/1XP-AI/gh-runnerd/pull/62#discussion_r3955590270)
- child deadline: [discussion 3955590276](https://github.com/1XP-AI/gh-runnerd/pull/62#discussion_r3955590276)
- old Codex review: [review 5138884210](https://github.com/1XP-AI/gh-runnerd/pull/62#pullrequestreview-5138884210)
- Codex review summary: [comment 5580386456](https://github.com/1XP-AI/gh-runnerd/pull/62#issuecomment-5580386456)
- prior integrator note: [comment 5580442642](https://github.com/1XP-AI/gh-runnerd/pull/62#issuecomment-5580442642)

The stale phase-receipt finding is resolved by a dedicated paired preparation
phase and paired receipt validation; the newer prerequisite-history finding
was the deeper version of that contract and is resolved below. The historical
ledger finding is reproduced by
`TestBrokerPairedAdmissionAcceptsHistoricalControllerClaim` and
`TestPairedFailureAllowsAuthorizedInspectWithoutPairedRetry`; both now accept
a prior controller claim under a paired request and a failed paired claim
under an authorized controller inspect while preserving one-shot slots. The
daemon-ID finding is reproduced at the colon and 128-byte boundaries by
`TestPairedWorkerDaemonIDMatchesCanonicalBoundaries`. The history and deadline
findings were reproduced by the red tests in `a0df276` and `278d8e9`, then fixed
in `419f9cd` and subsequent focused commits.

## Implementation boundary

`PreparePairedJournal` now requires the exact canonical controller-create
prefix: create phase, nonempty lowercase SHA-256 inventory, discovery intent
and result, create intent and successful create result. It preserves those
events byte-for-byte and only captures the intended preparation receipt under
the existing controller claim. Fresh, pending, deleted, uncertain, reserved,
previous-paired, malformed, or noncanonical histories are rejected; cleanup
authority is never borrowed and no remote effect or credential read occurs.

Broker admission replays every historical ledger event against the mode,
phase, schema, and authority recorded in that event's slot. Current paired
mode therefore does not reject a historical controller create, and current
controller inspect/cleanup can inspect a retained failed paired claim. The
cross-identity, tamper, ownership, authority-transition, incomplete-claim,
and one-shot current-attempt checks remain in force.

Paired approval validation reserves a complete terminal budget. The child
deadline is the minimum of parent, broker, controller, and worker authority,
then capped at ten minutes; it must leave a 35-second production cadence plus
25 seconds of child margin and a separate credential margin. Insufficient
remaining authority fails before mint/launch. Cancellation, expiry, deadline
overflow, output overflow, lost response, and retry paths remain fail-stop
with no automatic cleanup or retry; the ordinary 30-second preparation bound
is unchanged.

The paired worker daemon ID uses the authoritative worker contract
`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`; unrelated controller fields retain their
narrower validators.

The tagged offline bridge starts a fresh private TLS server and private Unix
Docker endpoint. It builds the reviewed `g01-live` executable from a clean
temporary clone with VCS metadata, then runs the real controller-create CLI,
real paired-preparation child, broker entrypoint, and exported
`RunPairedTerminal`. The fixture seam only supplies generated private roots,
loopback TLS CA, and the Unix endpoint; it cannot select production account,
Keychain, runner, Docker, service, or GitHub state.

## End-to-end evidence

The critical chain is
`TestPairedBrokerChainsRealControllerCreatePreparationAndTerminal` in
`experiments/g02-auth/broker_paired_bridge_test.go`. The controller-create
child first produced the same six canonical history records used by paired
execution; the broker then ran the real paired preparation contract against
that history and launched the actual tagged executable through the private
TLS/Unix bridge. The terminal child appended baseline records to the original
controller journal and created the separate worker paired journal.

The successful run asserted these exact bridge counts:

```text
create=1 start=1 JIT=1 acquire=1 acknowledgements=2
session-open=1 session-close=1 worker-delete=1 worker-absence=1
set-create=1 set-delete=1 set-absence=1 complete-rosters=4 unexpected=0
broker installation-token mints=1
```

It also asserted the broker ledger's paired controller/worker claim, original
session close, non-force worker deletion plus absence, set deletion plus
absence, canonical journal continuation, separate worker journal, and
secret-free attempt/controller/worker/admission roots. Reopening the completed
real broker entrypoint stopped before a second mint or terminal effect.

The failed-paired recovery test separately proves an incomplete paired claim
is retained, one explicitly authorized controller inspect can proceed in its
own slot, and a repeated inspect cannot mint or launch again. Hash/inode and
symlink replacement fences are covered by the paired binding and snapshot
tests. Existing paired terminal partitions cover cancellation, expired
authority, lost responses, journal uncertainty, reopened histories, receipt
separation, worker/set absence, and no-replay behavior.

The fast cadence used only by the tagged bridge is a deterministic test clock;
it advances the same seven five-second waits as production. The production
cadence proof is `TestPairedDistinctIDsAndOriginalCadence`, which requires
eight rounds and seven waits of at least five seconds (at least 35 seconds).
Together with the real child bridge run and
`TestBrokerPairedChildDeadlineIsBoundedAndLeavesCadenceMargin`, this proves a
bounded child may complete beyond the old 30-second limit while retaining a
finite authority cap. No production timeout was made unbounded.

## TDD and verification record

The meaningful red tests were committed before implementation:

```text
GOTOOLCHAIN=go1.26.8 go test -count=1 -run '^TestPairedPreparationPreservesCanonicalControllerHistory|^TestPairedPreparationRejectsFreshAndNonCanonicalHistory$' ./livecanary
exit 1 on the pre-fix implementation: the canonical history contract was absent.

GOTOOLCHAIN=go1.26.8 go test -count=1 -run '^TestBrokerPairedAdmissionAcceptsHistoricalControllerClaim|^TestPairedWorkerDaemonIDMatchesCanonicalBoundaries|^TestPairedApprovalRejectsInsufficientTerminalAuthority$' .
exit 1 on the pre-fix implementation: historical mode, daemon-ID boundaries, and authority budget were wrong.
```

Focused green checks on the tested source head were:

```text
GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=180s ./livecanary
ok  27.089s

GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=120s -tags='g01_live,g01_pair_fixture' ./cmd/g01-live
ok  0.844s

GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=300s -run 'Test(Paired|BrokerPaired|BrokerChild|BrokerBuild|BrokerPipe)' .
ok  11.240s before the final recovery-only test; the added recovery and parent-authority tests also passed in 0.679s.

GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=240s -run '^TestPairedBrokerChainsRealControllerCreatePreparationAndTerminal$' .
ok  3.337s after fixture cleanup; the same test passed at 3.147s before cleanup.

gofmt -d experiments/g01-scaleset/cmd/g01-live/main_test.go
no output
git diff --check
ok
```

The mandated full root `make check` result must be recorded here after the
final evidence edit; it includes formatting, build, vet, root tests and race
tests, fuzz smoke, dependency/license checks, both offline experiment modules,
and the pinned vulnerability check. No claim of full-goal completion is made
until the coordinator confirms independent exact-head review and hosted CI.

## Safety limits and remaining gates

All tests use disposable local files, synthetic nonsecret credentials, private
loopback TLS, and private Unix sockets. No live GitHub endpoint, App,
credential, runner/group/workflow, Docker/Lima context, Keychain, launchd
service, reboot, or manually installed runner was touched. Same-UID private
file ownership is not hostile-code isolation; same-UID races after released
short checks remain outside the proof.

The coordinator owns pushing evidence, requesting two independent reviews of
the exact final head, reading all inline and issue-comment findings including
stale ones, waiting for fresh Codex review and hosted CI, and merge gating.
