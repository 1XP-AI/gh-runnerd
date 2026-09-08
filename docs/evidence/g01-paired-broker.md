# G01g: paired terminal executable and bounded broker handoff

Issue [60](https://github.com/1XP-AI/gh-runnerd/issues/60) and PR
[62](https://github.com/1XP-AI/gh-runnerd/pull/62) remain an offline experiment
continuation. This evidence does not authorize live GitHub, runner, Docker,
Keychain, launchd, or recovery operations and does not claim G01/G02 closure.

## Review inputs and baseline

The implementation started at frozen head
`0ecb06c1755bc3a2f49724c9b7d5fa2bc9a0c3a9`. Reviewed `origin/main` at
`31ae8102f6f20f8e79258eb824af1400eba21954` was not an ancestor, so it was
integrated with a normal merge as `74efbdef37fba91b91d2315dae9c7e01cfd1b34b`.
No rebase, amend, force update, workflow replay, or live operation was used.

Both required settled Luna/max reports were read:

- `/tmp/g01-paired-broker-review-0ecb06c.md`
- `/tmp/g01-paired-broker-independent-review-0ecb06c.md`

The Codex wrapper inventory, including stale inline and issue-comment findings,
was read. The three live findings at the frozen head were:

- P1 fixture-enabled executable accepted by the production gate:
  [discussion r3956753241](https://github.com/1XP-AI/gh-runnerd/pull/62#discussion_r3956753241)
- P2 tenth finite ledger slot exceeded the structural line bound:
  [discussion r3956753229](https://github.com/1XP-AI/gh-runnerd/pull/62#discussion_r3956753229)
- P2 worker journal/admission was not prepared before mint:
  [discussion r3956753245](https://github.com/1XP-AI/gh-runnerd/pull/62#discussion_r3956753245)

The stale findings were also retained in review history and checked against
the current fixes: canonical controller history
([r3955590270](https://github.com/1XP-AI/gh-runnerd/pull/62#discussion_r3955590270)),
historical claims
([r3955069682](https://github.com/1XP-AI/gh-runnerd/pull/62#discussion_r3955069682)),
paired preparation phase
([r3955069674](https://github.com/1XP-AI/gh-runnerd/pull/62#discussion_r3955069674)),
worker daemon-ID boundaries
([r3955069689](https://github.com/1XP-AI/gh-runnerd/pull/62#discussion_r3955069689)),
and bounded child authority
([r3955590276](https://github.com/1XP-AI/gh-runnerd/pull/62#discussion_r3955590276)).

## TDD red evidence

The current findings were independently reproduced before the fixes. The
actual red commands/results were:

```text
GOTOOLCHAIN=go1.26.8 go test -count=1 -run '^TestBrokerBuildRejectsFixtureCapability$' .
FAIL: fixture-enabled controller build accepted by production broker gate

GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=120s -run '^TestPairedBrokerRejectsMalformedWorkerJournalBeforeMint$' .
FAIL: malformed worker journal crossed pre-mint boundary; mints=1
```

The independent report's clean temporary overlay also reproduced the tenth-slot
failure with:

```text
GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=120s \
  -overlay=/tmp/g01-review-overlay.json \
  -run '^TestReviewPairedTenthSlotLedgerBound$' .
ok: the test expected reopen rejection of the valid 21-line ledger
```

The implementation then progressed through focused green tests and normal
commits `6a35fe7`, `9df5d42`, `8c59523`, and `4aab247`; the current source head
before this evidence update is `4aab247cb2bc0ec9820e341db84db6b0a4b1743d`.

## Implemented boundaries

Production `validBrokerBuild` now requires the exact reviewed tag set
`g01_live`; any fixture or unreviewed test tag is rejected, even when the
revision, SDK, VCS cleanliness, OS, architecture, and CGO metadata are valid.
The checked-in bridge still uses an explicit `brokerBinaryOpener` seam for its
offline fixture binary, and the new `TestBrokerRejectsCleanFixtureBinaryBeforeMint`
proves the real production opener rejects that clean fixture artifact before
any API call or token mint. `TestBrokerAllowsCleanProductionBinaryArtifact`
proves a clean `g01_live` artifact is accepted. Fixture support remains absent
from ordinary production-tag builds.

The broker ledger derives its structural line limit from the finite schema:
eight controller phases plus `discover-actions-host` and `paired-terminal`.
The bound is one header, two records per slot, and the required trailing split
element: `1 + 2*10 + 1 = 22` lines. The existing byte bound remains in force;
unknown slots, malformed/duplicate records, oversize records, and invalid
authority transitions remain rejected. `TestBrokerLedgerCapacityDerivesFromFiniteSlotSchema`
locks the formula and the finite slot set.

Worker preparation now crosses the actual G01 boundary. The fixed
`--prepare-approved-paired-worker-journal` child command calls canonical
`liveworker.PrepareJournal` (or its explicitly nonproduction fixture adapter),
which validates approval, replays the complete journal, holds the canonical
worker authority/admission lease, rejects prior effects/uncertainty/reservation
histories, and returns only a credential-free receipt. The broker binds the
receipt's approval digest, state/journal/claim identities, and journal/claim
digests; it also checks the journal inode and bytes without duplicating G01's
event schema. The child later reopens the worker journal and admission claim
through the same canonical parser before worker effects, so replacement,
symlink, hash, reopen, and prior-history cases cannot mint/launch or authorize
a retry. `TestPairedWorkerPreparationReceiptFencesJournalMutation` covers
same-inode mutation and replacement, while the real malformed-journal test
asserts zero mints and zero authenticated calls.

The singleton JIT/acquire/start/ACK/session-close sequence, owned non-force
worker deletion plus set cleanup, historical claims, controller snapshot
receipts, and one-shot reopen behavior remain unchanged.

## Real offline bridge and cadence evidence

`TestPairedBrokerChainsRealControllerCreatePreparationAndTerminal` builds a
clean clone with VCS metadata, runs the real controller-create executable, the
real paired-preparation child, the broker entrypoint, and exported
`RunPairedTerminal` through generated loopback TLS and a private Unix socket.
It asserts:

```text
create=1 start=1 JIT=1 acquire=1 acknowledgements=2
session-open=1 session-close=1 worker-delete=1 worker-absence=1
set-create=1 set-delete=1 set-absence=1 complete-rosters=4 unexpected=0
broker installation-token mints=1
```

It also checks original controller-journal continuation, separate worker
journal, secret-free private roots, non-force worker deletion, and no second
mint/effect after reopening the completed claim.

`TestPairedBrokerRealCadenceChildExceedsThirtySeconds` builds the explicit
`g01_live,g01_pair_fixture,g01_pair_real_cadence` offline artifact, selects the
production wall clock (seven five-second cadence gaps), and runs the same
TLS/Unix bridge. The measured checked-in result was:

```text
GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=120s \
  -run '^TestPairedBrokerRealCadenceChildExceedsThirtySeconds$' -v .
real cadence bridge wall time: 36.196312875s
PASS; package wall time 39.633s
```

No fast clock is used by this regression. Its fixture/test tags are explicitly
rejected by the production binary gate; the bridge's opener override is only a
test seam for the offline endpoint and does not weaken the production path.

## Verification record

Focused and module checks that passed on the current source include:

```text
GOTOOLCHAIN=go1.26.8 go test -count=1 ./...
ok g01-scaleset; livecanary 37.487s; liveworker 10.221s

GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=45s \
  -tags=g01_live,g01_worker ./cmd/g01-live ./cmd/g01-worker
PASS; g01-live 5.320s; g01-worker 1.497s

GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=45s \
  -skip '^TestPairedBrokerRealCadenceChildExceedsThirtySeconds$' ./...
PASS; g02-auth 46.113s; all G02 command packages passed

GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=120s \
  -run '^(TestBrokerRejectsCleanFixtureBinaryBeforeMint|TestBrokerAllowsCleanProductionBinaryArtifact|TestPairedWorkerPreparationReceiptFencesJournalMutation|TestBrokerLedgerCapacityDerivesFromFiniteSlotSchema)$' .
PASS
```

The G02 offline script now runs the ordinary suite with the exact real-cadence
test excluded from its historical 45-second package budget, then runs only that
named test with a 120-second budget. This preserves coverage and records the
long test explicitly; it is not a blind rerun or a skipped security test.

The declared offline gate itself passed after these edits:

```text
GOTOOLCHAIN=go1.26.8 bash scripts/check-offline-experiments.sh
exit 0: both modules, command-tag tests, four G01 fixture partitions, vets,
the bounded G02 suite, the named real-cadence regression, and CLI packages;
offline experiment checks passed: 2 module(s)
```

Root validation also passed after updating the tooling-log assertion for the
two-command G02 split:

```text
GOTOOLCHAIN=go1.26.8 go test -count=1 ./...
PASS; scripts 84.373s
GOTOOLCHAIN=go1.26.8 go test -race -count=1 ./...
PASS; scripts 85.227s
GOTOOLCHAIN=go1.26.8 go vet ./...
PASS
git diff --check
PASS
GOTOOLCHAIN=go1.26.8 bash scripts/gofmt.sh check
PASS
```

All fixtures use disposable local files, synthetic nonsecret values, generated
loopback TLS, and a private Unix socket. No live endpoint, App, credential,
runner/group/workflow, Docker/Lima context, Keychain, launchd service, or
manually installed runner was touched. Same-UID ownership and short released
checks are not hostile-code isolation.

## Remaining gates

This worker does not merge PR 62. The coordinator must push the frozen final
head, request two fresh independent reviews and `@codex review`, wait for
completion, read inline and issue-comment findings including stale/outdated
ones, verify hosted CI, and confirm an exact-head clean Codex review before any
merge decision. Live recovery remains unauthorized and unproven.
