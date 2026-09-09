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

Both required settled Luna/max reports for frozen head
`0ecb06c1755bc3a2f49724c9b7d5fa2bc9a0c3a9` were read as GitHub-backed review
dispositions on [PR 62](https://github.com/1XP-AI/gh-runnerd/pull/62). Private
local report files are not repository artifacts and are not committed.

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

The earlier bounded CI-contract follow-up started at frozen PR head
`1ebf0b1e50ac200a26b69d0352a61e5a622bbeeb` and owns only the offline gate
script, its tooling regression tests, this CI guide, and this evidence record.
It does not change the G01 or G02 runtime.

## TDD red evidence

The current findings were independently reproduced before the fixes. The
actual red commands/results were:

```text
GOTOOLCHAIN=go1.26.8 go test -count=1 -run '^TestBrokerBuildRejectsFixtureCapability$' .
FAIL: fixture-enabled controller build accepted by production broker gate

GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=120s -run '^TestPairedBrokerRejectsMalformedWorkerJournalBeforeMint$' .
FAIL: malformed worker journal crossed pre-mint boundary; mints=1
```

An independent disposable overlay against that prior frozen source reproduced
the tenth-slot reopen refusal expected by
[discussion r3956753229](https://github.com/1XP-AI/gh-runnerd/pull/62#discussion_r3956753229).
The overlay was local to that reviewer, is not a repository artifact, and is
not committed.

The implementation then progressed through focused green tests and normal
commits `6a35fe7`, `9df5d42`, `8c59523`, and `4aab247`; the current source head
before the prior evidence update was `4aab247cb2bc0ec9820e341db84db6b0a4b1743d`;
the frozen PR head for this follow-up is `1ebf0b1e50ac200a26b69d0352a61e5a622bbeeb`.

The CI contract correction also had meaningful red evidence before the script
fix. The prior G02 invocation was not the approved static split, and the new
G02 witness matrix therefore rejected its wrapper log:

```text
GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=120s \
  -run '^TestToolingDefaultG01PartitionsRun$' ./scripts
FAIL: expected named G02 45-second invocation ran 0 times; wrapper log contained
the 45-second unfiltered remainder and the 120-second named invocation

GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=120s \
  -run '^TestToolingDefaultG02PartitionsRun$' ./scripts
FAIL: expected named G02 45-second invocation ran 0 times; wrapper log contained
the 45-second unfiltered remainder and the 120-second named invocation
```

These failures exercise the CI command contract and generated coverage
boundaries, rather than a missing runtime symbol or an unavailable fixture.

The current follow-up at frozen PR head
`bf278729a4e42bc3d2358f9debdbdfa6bf8b2e63` independently reproduced three
still-live findings before the fixes:

```text
GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=60s \
  -run '^TestPairedWorkerPreparationReceiptFencesClaimMutation$' .
FAIL: worker admission claim mutation crossed receipt fence
  (hash, replacement, missing, malformed, and locked)

GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=60s \
  -run '^TestPairedBrokerRejectsWorkerClaimChangeBeforeMint$' .
FAIL: worker admission claim hash crossed pre-mint fence:
  err=<nil> mints=1 launches=1
  (same for replacement, missing, malformed, and locked)

GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=180s \
  -run '^TestToolingDefaultG02PartitionsRun$' ./scripts
FAIL: expected remaining TestPaired 45-second invocation ran 0 times;
wrapper log retained the two-command skip-only-cadence remainder
```

Same-inode mutation and replacement of the worker `admission.json` after
canonical preparation reached mint and launch. That is
[discussion r3957639894](https://github.com/1XP-AI/gh-runnerd/pull/62#discussion_r3957639894).
Committed reviewer-local report/overlay paths are
[discussion r3957639908](https://github.com/1XP-AI/gh-runnerd/pull/62#discussion_r3957639908).

A later coordinator reproduction at `92632dde67a98c31146386ce2ce144326b0bdff9`
showed the same-inode fence was incomplete: `workerClaimPath` searched a
sibling `worker-admission` directory and the account pin for any matching
inode. Moving the prepared claim inode into that sibling and writing `{}` at
the original pathname made `checkPrepared` return nil
([comment 5586268545](https://github.com/1XP-AI/gh-runnerd/pull/62#issuecomment-5586268545)):

```text
GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=60s \
  -run '^TestPairedWorkerPreparationReceiptFencesClaimRelocation$' .
FAIL: relocated receipt inode bypassed changed canonical claim path
  (relocated-inode and relocated-directory)

GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=60s \
  -run '^TestPairedBrokerRejectsWorkerClaimChangeBeforeMint$' .
FAIL: worker admission claim relocated-inode crossed pre-mint fence:
  err=<nil> mints=1 launches=1
  (same for relocated-directory)
```

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
digests. Every later `checkPrepared` reopens the worker journal and the
admission claim bound by the trusted preparation contract: the explicit
prepared directory in tests, otherwise the native-account worker pin. It
compares the named path's inode and digest to the stored receipt and checks
the claim's version-1 ownership/state/journal schema under a short exclusive
file lease. It does not search fixture siblings or other candidate paths for a
matching inode. Same-inode mutation, replacement, missing, malformed, locked,
relocated-inode, and relocated-directory claims fail at the pre-auth, mint,
and launch fences with zero remote/mint/launch as appropriate. Offline
executable fixtures bind the claim directory through the explicit
`resolveWorkerClaimDirectory` / `bindWorkerClaimDirectory` seam, the same
narrow pattern as `brokerBinaryOpener`.
The child later reopens the worker journal and admission claim through the same
canonical parser before worker effects, so those cases cannot mint/launch or
authorize a retry. `TestPairedWorkerPreparationReceiptFencesJournalMutation`
and `TestPairedWorkerPreparationReceiptFencesClaimMutation` cover journal and
claim fences; `TestPairedBrokerRejectsWorkerClaimChangeBeforeAuth` asserts zero
authenticated calls; `TestPairedBrokerRejectsWorkerClaimChangeBeforeMint`
asserts zero mints and zero launches. The real malformed-journal test still
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
TLS/Unix bridge. The following 120-second result is retained as a historical
measurement only; it is not approved CI proof or a public timeout allowance:

```text
GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=120s \
  -run '^TestPairedBrokerRealCadenceChildExceedsThirtySeconds$' -v .
real cadence bridge wall time: 36.196312875s
PASS; package wall time 39.633s
```

No fast clock is used by this regression. Its fixture/test tags are explicitly
rejected by the production binary gate; the bridge's opener override is only a
test seam for the offline endpoint and does not weaken the production path.

The previous two-partition follow-up recorded these 45-second runs at
`1ebf0b1e50ac200a26b69d0352a61e5a622bbeeb` / `bf278729a4e42bc3d2358f9debdbdfa6bf8b2e63`.
They are historical measurements only and are not current proof that the
two-command remainder gate is green:

```text
/usr/bin/time -p env GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=45s \
  -run '^TestPairedBrokerRealCadenceChildExceedsThirtySeconds$' ./...
PASS; g02-auth package wall time 40.151s; process wall time 41.22s

/usr/bin/time -p env GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=45s \
  -skip '^TestPairedBrokerRealCadenceChildExceedsThirtySeconds$' ./...
PASS; g02-auth package wall time 40.471s; process wall time 41.50s
```

Independent exact-head repeats at `bf278729a4e42bc3d2358f9debdbdfa6bf8b2e63`
found the named cadence partition still passing (~40–43s) but the unfiltered
remainder hitting the 45-second test-binary deadline on repeat while generating
an RSA fixture key, and the declared offline gate exiting 1. Those red results
are the current Codex finding
[r3957835501](https://github.com/1XP-AI/gh-runnerd/pull/62#discussion_r3957835501).
The 45-second per-process default was not widened.

## Verification record

Historical module checks at the prior two-partition follow-up (`1ebf0b1` /
`bf27872`) are retained below as historical measurements. They are not current
proof of the three-partition G02 gate:

```text
GOTOOLCHAIN=go1.26.8 go test -count=1 ./...
ok g01-scaleset; livecanary 37.487s; liveworker 10.221s

GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=45s \
  -tags=g01_live,g01_worker ./cmd/g01-live ./cmd/g01-worker
PASS; g01-live 5.320s; g01-worker 1.497s

GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=120s \
  -run '^(TestBrokerRejectsCleanFixtureBinaryBeforeMint|TestBrokerAllowsCleanProductionBinaryArtifact|TestPairedWorkerPreparationReceiptFencesJournalMutation|TestBrokerLedgerCapacityDerivesFromFiniteSlotSchema)$' .
PASS
```

The G02 offline script now keeps the 45-second per-process default and the real
seven-times-five-second cadence, and splits G02 into three static partitions
with `./...` package discovery: the exact cadence name; the remaining
`^TestPaired` family with that exact name skipped; and an unfiltered complement
that skips `^TestPaired`. Synthetic RSA candidates are generated once per
test-binary. The generated positive and failure witness matrix proves cadence,
remaining `TestPaired` family, remainder, other package, same-name cadence,
same-name remaining `TestPaired`, Example Output, and fuzz seed each execute
exactly once and propagate nonzero failures. The historical 120-second cadence
measurement above remains timing evidence only and is not an approved CI proof.

Current exact 45-second G02 partitions after the split:

```text
GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=45s \
  -run '^TestPairedBrokerRealCadenceChildExceedsThirtySeconds$' ./...
PASS; g02-auth 41.338s; process wall time 42.37s

GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=45s \
  -run '^TestPaired' -skip '^TestPairedBrokerRealCadenceChildExceedsThirtySeconds$' ./...
PASS; g02-auth 11.297s; process wall time 12.42s

GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=45s \
  -skip '^TestPaired' ./...
PASS; g02-auth 23.600s; process wall time 24.34s

# Repeat unfiltered complement without changes:
GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=45s \
  -skip '^TestPaired' ./...
PASS; g02-auth 24.639s; process wall time 25.48s
```

Focused claim-fence and related tests after the pathname-binding fix:

```text
GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=90s \
  -run '^(TestPairedWorkerPreparationReceiptFencesClaimRelocation|TestPairedWorkerPreparationReceiptFencesClaimMutation|TestPairedWorkerPreparationReceiptFencesJournalMutation|TestPairedBrokerRejectsWorkerClaimChangeBeforeAuth|TestPairedBrokerRejectsWorkerClaimChangeBeforeMint|TestPairedBrokerRealEntrypointUsesPairedPreparationClosure|TestPairedBrokerRejectsMalformedWorkerJournalBeforeMint|TestBrokerAccountRootIgnoresEnvironmentAndFailsClosed)$' .
PASS; g02-auth 2.514s
```

Current G02 45-second partitions after the pathname-binding fix:

```text
GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=45s \
  -run '^TestPairedBrokerRealCadenceChildExceedsThirtySeconds$' ./...
PASS; g02-auth 40.625s

GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=45s \
  -run '^TestPaired' -skip '^TestPairedBrokerRealCadenceChildExceedsThirtySeconds$' ./...
PASS; g02-auth 11.823s

GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=45s \
  -skip '^TestPaired' ./...
PASS; g02-auth 25.740s
```

Declared offline gate after the pathname-binding fix:

```text
/usr/bin/time -p env GOTOOLCHAIN=go1.26.8 bash scripts/check-offline-experiments.sh
G02 named cadence: g02-auth 40.781s
G02 remaining TestPaired family: g02-auth 11.511s
G02 unfiltered complement: g02-auth 26.138s
offline experiment checks passed: 2 module(s)
exit 0; process wall time 353.67s
```

The focused tooling matrix passed after the three-partition script correction:

```text
GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=180s \
  -run '^TestToolingDefaultG02PartitionsRun$' ./scripts
PASS; TestToolingDefaultG02PartitionsRun 122.103s
```

The declared offline gate passed after these edits:

```text
/usr/bin/time -p env GOTOOLCHAIN=go1.26.8 bash scripts/check-offline-experiments.sh
G02 named cadence: g02-auth 40.421s
G02 remaining TestPaired family: g02-auth 10.994s
G02 unfiltered complement: g02-auth 23.360s
offline experiment checks passed: 2 module(s)
exit 0; process wall time 358.06s

# Repeat without source changes:
G02 named cadence: g02-auth 40.359s
G02 remaining TestPaired family: g02-auth 10.756s
G02 unfiltered complement: g02-auth 24.922s
offline experiment checks passed: 2 module(s)
exit 0; process wall time 352.57s
```

Root validation after the G01 tooling-log assertion for the three-command G02
split:

```text
GOTOOLCHAIN=go1.26.8 go test -count=1 ./...
PASS; scripts 203.625s
GOTOOLCHAIN=go1.26.8 go test -race -count=1 ./...
PASS; scripts 205.307s
GOTOOLCHAIN=go1.26.8 go vet ./...
PASS
git diff --check
PASS
GOTOOLCHAIN=go1.26.8 bash scripts/gofmt.sh check
PASS

GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=180s \
  -run '^(TestToolingDefaultG01PartitionsRun|TestToolingDefaultG02PartitionsRun)$' ./scripts
PASS; scripts 145.223s
```

The named cadence partition remains close to the 45-second budget because the
production seven-times-five-second cadence is preserved (~36s child wall time
plus clone/build/bridge). That timeout was not widened. Remainder headroom is
now the unfiltered complement at ~24s rather than a 45-second near miss.

All fixtures use disposable local files, synthetic nonsecret values, generated
loopback TLS, and a private Unix socket. No live endpoint, App, credential,
runner/group/workflow, Docker/Lima context, Keychain, launchd service, or
manually installed runner was touched. Same-UID ownership and short released
checks are not hostile-code isolation.

## Hosted remaining-TestPaired timeout at 773cccc

Hosted Public CI run 34238428090 job 102102074844 failed on exact head
`773cccc592a8e1f6cd168d80a1e38b384c6668af`. The named cadence partition
passed (`ok g02-auth 38.287s`). The remaining `^TestPaired` 45-second
partition panicked with `TestPairedBrokerRealEntrypointUsesPairedPreparationClosure`
active for 42s. The dump waited in `invokeBrokerPairedTerminal` at
`broker_process.go:224` (`Wait` / Linux `pidfdWait`) with `CommandContext`
`watchCtx` still armed.

This was not an aggregate-timeout miss and was not a Linux-only wait
primitive bug. The remaining-partition entrypoint uses `testBrokerBinary`
(the Go test executable) as the paired child. `TestMain` treated any
`ControllerApprovalSHA256` prefix `e` as overflow output and prefix `f` as
`time.Sleep(time.Minute)`. The real entrypoint hashes live controller
approval bytes, including `ExpiresAt: time.Now().Add(time.Hour)`, so the
digest is wall-clock and timezone dependent. `runBrokerWithAPI` uses
`context.Background()`, so `pairedChildDeadline` grants the 10-minute
maximum child budget and does not kill the sleeper before the 45-second
process deadline. A digest starting with `f` therefore occupied the
remaining partition until timeout. The named cadence partition builds a
real tagged `g01-live` child and does not enter this `TestMain` trap.

The overflow/timeout child sentinels used by
`TestBrokerPairedChildBoundsTimeoutOverflowAndCancel` remain
`e`/`f` plus 63 `a` bytes. Prefix matching was replaced with equality on
those exact sentinel strings only. Cadence, race, count=1, and the
unfiltered complement contract were not widened.

TDD red against the prefix trap, with the new regression test present:

```text
GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=15s \
  -run '^TestBrokerPairedTerminalRealDigestPrefixDoesNotTriggerFixtureSleep$/e$' .
FAIL: ordinary digest prefix "e" refused: broker stopped; retain private intent
and review; no automatic retry

GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=8s \
  -run '^TestBrokerPairedTerminalRealDigestPrefixDoesNotTriggerFixtureSleep$/f$' .
panic: test timed out after 8s
running tests:
TestBrokerPairedTerminalRealDigestPrefixDoesNotTriggerFixtureSleep/f (8s)
invokeBrokerPairedTerminal broker_process.go:224 waiting child
```

Green after matching only the dedicated sentinels:

```text
GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=30s \
  -run '^(TestBrokerPairedTerminalRealDigestPrefixDoesNotTriggerFixtureSleep|TestBrokerPairedChildBoundsTimeoutOverflowAndCancel|TestBrokerPairedChildDeadlineIsBoundedAndLeavesCadenceMargin|TestBrokerPairedTerminalPipeUsesFixedArgsAndOneControllerInput|TestPairedBrokerRealEntrypointUsesPairedPreparationClosure)$' .
PASS; g02-auth 1.827s
  TestPairedBrokerRealEntrypointUsesPairedPreparationClosure 0.87s
  TestBrokerPairedTerminalRealDigestPrefixDoesNotTriggerFixtureSleep 0.25s
  TestBrokerPairedChildBoundsTimeoutOverflowAndCancel 0.03s

GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=45s \
  -run '^TestPaired' -skip '^TestPairedBrokerRealCadenceChildExceedsThirtySeconds$' ./...
PASS; g02-auth 15.197s

GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=90s -skip '^TestPaired' .
PASS; g02-auth 43.161s
  TestBrokerPairedTerminalRealDigestPrefixDoesNotTriggerFixtureSleep 2.35s
  /e 1.30s; /f 1.03s

GOTOOLCHAIN=go1.26.8 go vet ./...
PASS
git diff --check
PASS
GOTOOLCHAIN=go1.26.8 bash scripts/gofmt.sh check
PASS
```

The official 45-second unfiltered complement was not claimed as a local
pass on this loaded host: later tests were still starting when the
process deadline fired, including a 1-second `/f` child start, which is
not the one-minute prefix sleep. Hosted CI remains the complement
measurement. The named 7x5s cadence path was not changed.

## Worker admission root identity

An independent review of `773cccc` reproduced a remaining fence hole:
replace the worker admission directory, restore the original
`admission.json` inode at the same canonical pathname, and
`checkPrepared` returned nil; `brokerExecute` then returned nil with
mints=1 launches=1. File inode/digest/path checks survived because they
did not retain the prepared directory identity. Scope is that proven
same-process replacement, not a broader search or native-account change.

TDD red against current head `6f94ced` before the identity bind:

```text
GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=60s \
  -run '^(TestPairedWorkerPreparationReceiptFencesAdmissionRootReplacement|TestPairedBrokerRejectsWorkerAdmissionRootReplacementBeforeAuth|TestPairedBrokerRejectsWorkerAdmissionRootReplacementBeforeMint)$' .
FAIL: TestPairedWorkerPreparationReceiptFencesAdmissionRootReplacement/replaced-root
  replaced worker admission root crossed receipt fence
FAIL: TestPairedBrokerRejectsWorkerAdmissionRootReplacementBeforeAuth/replaced-root
  err=<nil> mints=1 launches=1
FAIL: TestPairedBrokerRejectsWorkerAdmissionRootReplacementBeforeMint/replaced-root
  err=<nil> mints=1 launches=1
PASS: symlink, mode, and missing roots already refused
```

Green after binding the trusted preparation directory identity and
revalidating it on every claim-path check:

```text
GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=60s \
  -run '^(TestPairedWorkerPreparationReceiptFencesAdmissionRootReplacement|TestPairedBrokerRejectsWorkerAdmissionRootReplacementBeforeAuth|TestPairedBrokerRejectsWorkerAdmissionRootReplacementBeforeMint|TestPairedWorkerPreparationReceiptFencesClaimRelocation|TestPairedWorkerPreparationReceiptFencesClaimMutation|TestPairedWorkerPreparationReceiptFencesJournalMutation|TestPairedBrokerRejectsWorkerClaimChangeBeforeAuth|TestPairedBrokerRejectsWorkerClaimChangeBeforeMint|TestPairedBrokerRealEntrypointUsesPairedPreparationClosure|TestBrokerPairedTerminalRealDigestPrefixDoesNotTriggerFixtureSleep)$' .
PASS; g02-auth 4.501s

GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=45s \
  -run '^TestPaired' -skip '^TestPairedBrokerRealCadenceChildExceedsThirtySeconds$' ./...
PASS; g02-auth 14.169s

GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=30s \
  -run '^TestBrokerAccountRootIgnoresEnvironmentAndFailsClosed$' .
PASS; g02-auth 0.253s

GOTOOLCHAIN=go1.26.8 go vet ./...
PASS
git diff --check
PASS
GOTOOLCHAIN=go1.26.8 bash scripts/gofmt.sh check
PASS
```

Cadence, fixture sentinels, worker-claim path pinning, and native-account
rooting were not changed. The named 7x5s cadence test was not re-run.

## Worker admission root receipt boundary

An independent P1 on `e48f813` showed `prepareJournal` still sampled the
current admission-root inode after the child returned. Modeling the real
child (no in-process `claimDirectoryInfo`) by creating a valid journal and
claim, then replacing the 0700 root while restoring the original claim inode
before returning the receipt, allowed `brokerExecute` to authenticate,
mint once, and launch once. Exact-head Codex clean did not clear that P1.

The trusted root identity is now part of the canonical preparation receipt
(`admission_directory`) produced by G01 liveworker/livecanary and consumed
by G02. Absent, zero, or mismatched root identities are rejected before
auth, mint, or launch. Post-return Lstat is only a comparison against that
receipt, not a source of trust.

Independent overlay red against `e48f813` (overlay not committed):

```text
GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=60s \
  -run '^TestReviewWorkerRootReplacementBetweenReceiptAndRootBind$' .
FAIL: err=<nil> mints=1 launches=1 authenticated GET /app and installation mint

GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=90s \
  -run '^TestReviewWorkerRootReplacementBetweenReceiptAndRootBind$' .
FAIL: mints=1 launches=1
```

Green after the receipt protocol (checked-in tests; overlay rechecked then
removed):

```text
GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=60s \
  -run '^(TestPairedBrokerRejectsWorkerAdmissionRootReplacementBetweenReceiptAndBind|TestPairedWorkerPrepareJournalRejectsReplacedRootBetweenReceiptAndBind|TestPairedWorkerPreparationReceiptRejectsAbsentOrMismatchedAdmissionRoot|TestPairedWorkerPreparationReceiptFencesAdmissionRootReplacement|TestPairedBrokerRejectsWorkerAdmissionRootReplacementBeforeAuth|TestPairedBrokerRejectsWorkerAdmissionRootReplacementBeforeMint)$' .
PASS; g02-auth 1.209s

GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=60s \
  -run '^TestReviewWorkerRootReplacementBetweenReceiptAndRootBind$' .
PASS; mints=0 calls=[] launches=0

GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=45s \
  -run '^TestPaired' \
  -skip '^(TestPairedBrokerRealCadenceChildExceedsThirtySeconds|TestPairedBrokerChainsRealControllerCreatePreparationAndTerminal)$' ./...
PASS; g02-auth 10.192s

GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=30s \
  -run '^TestWorkerPreparationReturnsCanonicalSnapshotAndRejectsPriorEffect$' ./liveworker
PASS
GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=30s \
  -run '^TestCanonicalPreparationRecordsNoPhaseOrRemoteIntent$|^TestPairedPreparationUsesDedicatedPhaseWithoutCleanupAuthority$' ./livecanary
PASS
GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=30s \
  -run '^TestPairedBrokerRealEntrypointUsesPairedPreparationClosure$' .
PASS; g02-auth 0.793s

GOTOOLCHAIN=go1.26.8 go vet ./...
PASS
git diff --check
PASS
GOTOOLCHAIN=go1.26.8 bash scripts/gofmt.sh check
PASS
```

The real g01-live bridge clones committed HEAD, so its producer/consumer
round-trip is recorded after this commit. Timeouts, cadence, race, count=1,
and complement contracts were not widened. Prior fixture-sentinel and
after-bind root checks remain.

## Timeout headroom for the real cadence partition

Historical Codex finding
[r3957835501](https://github.com/1XP-AI/gh-runnerd/pull/62#discussion_r3957835501)
is a headroom finding, not a coverage finding. Applying a 12% slowdown to the
whole 40s wall incorrectly scales the fixed 35-second production wait. The
clone/build/compile overhead is the CPU-bound part. Recorded in-process cadence
wall 40.151s minus 35s wait leaves 5.151s overhead; twice that overhead plus
the fixed wait is a 45.302s model. That arithmetic is not an executed timeout
and is not treated as proof that a 45-second process failed.

The first isolation follow-up at `c628b0b` moved clone/build into
`TestPairedBrokerPrepareReviewedG01LiveBinary` but still prepared only
`g01_live,g01_pair_fixture`. Cadence requests
`g01_live,g01_pair_fixture,g01_pair_real_cadence`, so the cadence process kept
its own clone/build. A process-global tag-only cache also returned a deleted
`t.TempDir` path. Actual red at `c628b0b`, including independent contract and
security reviews, is recorded in
[PR 62 comment 5594788650](https://github.com/1XP-AI/gh-runnerd/pull/62#issuecomment-5594788650).
Current Codex [r3964054895](https://github.com/1XP-AI/gh-runnerd/pull/62#discussion_r3964054895)
is the tag mismatch; historical [r3957835501](https://github.com/1XP-AI/gh-runnerd/pull/62#discussion_r3957835501)
remains open headroom. Public CI green on `c628b0b` does not clear those
findings.

TDD red on `c628b0b` before this correction:

```text
GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=90s \
  -run '^(TestPairedBrokerPrepareReviewedG01LiveBinary|TestPairedBrokerChainsRealControllerCreatePreparationAndTerminal)$' .
FAIL; TestPairedBrokerChainsRealControllerCreatePreparationAndTerminal
      reviewed g01 bridge command failed: ""; package ~2.156s

GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=90s \
  -run '^(TestPairedBrokerPrepareReviewedG01LiveBinary|TestPairedBrokerChainsRealControllerCreatePreparationAndTerminal)$' .
FAIL; same chain failure; package ~1.779s
```

Missing or invalid `G01_PAIR_BRIDGE_PREP_DIR` still compiled inside cadence
(standalone fallback). A prior-commit fixture with matching two-line metadata
was accepted as current-head evidence. Parent-directory symlink plus 0777 root
and 0644 metadata still loaded. Prep failure leaked the owned mktemp directory
because the G02 subshell had no EXIT trap.

The G02 offline script still runs the same four 45-second partitions, race,
count=1, `./...` discovery, and seven real five-second gaps. No 120-second
cadence timeout was added. Preparation now stores both tagged variants under
owned 0700 directories, pins a receipt to current HEAD, clean VCS, exact
tags/target/Go SDK and the SHA-256 of the opened bytes, and uses the existing
private-file helpers. A set `G01_PAIR_BRIDGE_PREP_DIR` is fail-closed and does
not clone/build. Unset remains the standalone compile path. The tag-only
process cache is removed. An EXIT trap removes only the owned mktemp path on
success and command failure. Independent review of `e25a682e` found that
EXIT-only cleanup does not run on default SIGTERM/SIGINT on Bash 5; the
follow-up below adds explicit INT/TERM/HUP handling.

Green after this correction:

```text
GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=90s \
  -run '^(TestPairedBrokerPrepareReviewedG01LiveBinary|TestPairedBrokerChainsRealControllerCreatePreparationAndTerminal)$' .
PASS; g02-auth 5.680s

GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=90s \
  -run '^(TestPairedBrokerPrepareReviewedG01LiveBinary|TestPairedBrokerChainsRealControllerCreatePreparationAndTerminal)$' .
PASS; g02-auth 6.622s

GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=180s \
  -run 'TestPairedBrokerPrepared|TestPreparedBridgeStandaloneUnsetPrepCompilesDistinctVariant' .
PASS; g02-auth 9.164s

GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=60s \
  -run '^TestG02OwnedPrepDirRemovedAfterPrepFailure$' ./scripts
PASS; scripts 10.448s

GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=240s \
  -run '^TestToolingDefaultG02PartitionsRun$' ./scripts
PASS; scripts 179.226s

GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=60s \
  -run '^TestToolingDefaultG01PartitionsRun$' ./scripts
PASS; scripts 27.427s
```

Exact four-command G02 partition harness, all `-race -count=1 -timeout=45s`
`./...`, with an owned prep directory and a compile probe that prep writes and
cadence/remaining must not rewrite:

```text
prep exact name:       PASS; g02-auth 4.470s; process 5.78s; compile probe written
real cadence exact:    PASS; g02-auth 38.115s; process 38.45s; compile probe absent
remaining TestPaired:  PASS; g02-auth 13.122s; process 13.58s; compile probe absent
unfiltered complement: PASS; g02-auth 30.720s; process 31.19s
```

Cadence wall remains the production seven-times-five-second wait plus the
race-compiled test binary and bridge. It no longer clones or rebuilds the
cadence-tagged fixture. Limits: public hosted CI still has no secrets or live
GitHub/App/runner/Docker/Keychain/launchd coverage. Rollback is a source-only
revert of this prepared-receipt follow-up; the earlier cadence-prep isolation,
receipt-boundary, fixture-sentinel and after-bind commits remain independently
revertable.

## G02 owned prep cleanup on INT/TERM/HUP

Independent contract review of `e25a682e` found that
`scripts/check-offline-experiments.sh` installed only an EXIT trap while the
script comment and `docs/CI.md` claimed signal-induced cleanup. On Bash 5, EXIT
does not run for default SIGTERM/SIGINT, so a hosted cancellation can leak the
owned mktemp tree. Ordinary success, `exit 91`, and prep-failure cleanup were
already green. Cleanup is now registered immediately after `mktemp` and before
`chmod`. EXIT still removes only that owned path without changing success or
command-failure status. Explicit INT, TERM, and HUP traps remove the same path,
disarm EXIT, and exit 130, 143, or 129. Tests signal only the exact G02
subshell PID of the generated fixture; they do not kill process groups or
existing runners.

TDD red at `e25a682e` before this correction (Go 1.26.8, darwin/arm64,
`/bin/bash` 3.2.57):

```text
GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=180s \
  -run '^TestG02OwnedPrep(CleanupContract|DirRemovedAfter(Success|Exit91|Signal|PrepFailure))$' ./scripts
FAIL; scripts 59.530s
TestG02OwnedPrepCleanupContract: missing owned EXIT/INT/TERM/HUP cleanup traps
TestG02OwnedPrepDirRemovedAfterSignal/INT: INT status=0
TestG02OwnedPrepDirRemovedAfterSuccess PASS
TestG02OwnedPrepDirRemovedAfterExit91 PASS
TestG02OwnedPrepDirRemovedAfterPrepFailure PASS
TestG02OwnedPrepDirRemovedAfterSignal/TERM PASS
TestG02OwnedPrepDirRemovedAfterSignal/HUP PASS
```

Local Bash 3.2 runs EXIT on TERM/HUP, so the local behavioral red is INT
status 0 plus the missing explicit traps. The independent review's smallest
EXIT-only SIGTERM check was `rc=143 cleanup_marker=no`.

Green after this correction:

```text
bash -n scripts/check-offline-experiments.sh
PASS

GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=180s \
  -run '^TestG02OwnedPrep(CleanupContract|DirRemovedAfter(Success|Exit91|Signal|PrepFailure))$' ./scripts
PASS; scripts 58.693s

GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=180s \
  -run '^TestG02OwnedPrep(CleanupContract|DirRemovedAfter(Success|Exit91|Signal|PrepFailure))$' ./scripts
PASS; scripts 60.054s
```

G02 partition witness plus the cleanup family, run once after the focused
green:

```text
GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=300s \
  -run '^(TestToolingDefaultG02PartitionsRun|TestG02OwnedPrep)' ./scripts
PASS; scripts 223.351s
```

This follow-up does not rerun G01 partitions or the real G02 experiment
harness. Hosted CI run 34306339973 failed on a separate G01 terminal
partition (120s); that investigation is owned by an independent Luna
diagnostic on immutable `e25a682e` and is not addressed here.

Limits: public hosted CI still has no secrets or live
GitHub/App/runner/Docker/Keychain/launchd coverage. Rollback is a source-only
revert of this signal-cleanup follow-up; the prepared-receipt, cadence-prep
isolation, receipt-boundary, fixture-sentinel and after-bind commits remain
independently revertable.

## G01 terminal heavy partition for hosted 120s budget

Hosted Public CI run 34306339973 timed out the previous terminal non-storage
process at package `120.025s` with
`TestPairedTerminalPreParentFailureRetainsReturnedOnlyTerminal` still active.
Independent diagnosis on immutable `e25a682e` measured the same selected family
at local `GOMAXPROCS=2` as `101.060s` package time while that target stayed
about `2.2s` isolated. The evidence favored cumulative partition budget plus
host parallelism/fsync cost, not a proven target deadlock. Residual
host-filesystem uncertainty remains; this follow-up does not claim the hosted
timeout is a proven fixed deadlock.

The gate adds one deterministic heavy terminal partition of those five names
and makes the previous non-storage command an explicit complementary remainder
that also skips storage. Storage, unfiltered non-paired collection, default G01
45s partitions, and G02 partitions are unchanged. Timeouts stay 120s; jobs are
not parallelized.

`go test -list` on current livecanary `TestPairedTerminal` names, classified
with the script regexes, is 26 total: heavy 5, remainder 15, storage 6, each
name once. `-list` does not apply `-skip`; remainder membership is the set
difference, and `-run/-skip` was checked by refusing a heavy name and a storage
name under the remainder skip (`[no tests to run]`).

TDD red before the gate change (Go 1.26.8, darwin/arm64):

```text
GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=90s \
  -run '^TestG01PairedTerminalPartitionRegistry$' ./scripts
FAIL; scripts 0.415s
script missing terminal_heavy_regex assignment

GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=180s \
  -run '^TestToolingTaggedPairFixturePartitionsRun$' ./scripts
FAIL; scripts 53.26s
heavy/remainder/future invocations 0 times; legacy unsplit terminal skip retained
```

Green after this correction:

```text
bash -n scripts/check-offline-experiments.sh
PASS

GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=90s \
  -run '^TestG01PairedTerminalPartitionRegistry$' ./scripts
PASS; scripts 3.325s

GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=180s \
  -run '^TestToolingTaggedPairFixturePartitionsRun$' ./scripts
PASS; scripts 59.419s

GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=180s \
  -run '^(TestG01PairedTerminalPartitionRegistry|TestToolingTaggedPairFixturePartitionsRun)$' ./scripts
PASS; scripts 63.425s

GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=120s \
  -run '^(TestToolingDefaultG01PartitionsRun|TestG02OwnedPrepCleanupContract)$' ./scripts
PASS; scripts 27.376s
```

Actual new partitions once, `-race -count=1 -timeout=120s -tags=g01_pair_fixture`,
`GOMAXPROCS=2`, `-json` top-level counts only:

```text
heavy:     PASS; 5 tests; package 68.852s; wall 69.47s; headroom ~51s
remainder: PASS; 15 tests; package 30.84s; wall 31.70s; headroom ~89s
           TestPairedTerminalPreParentFailureRetainsReturnedOnlyTerminal 2.16s
storage:   PASS; 6 tests; package 87.205s; wall 87.71s; unchanged command
```

Local `GOMAXPROCS=2` therefore moves the previous 101.060s non-storage process
into 68.852s + 30.84s without widening timeouts. Hosted Ubuntu x64 scheduling
and `File.Sync` latency remain unmeasured; a one-off host fsync stall is still
possible. G02 owned INT/TERM/HUP cleanup is unchanged. This worker does not
rerun hosted CI.

Limits: public hosted CI still has no secrets or live
GitHub/App/runner/Docker/Keychain/launchd coverage. Rollback is a source-only
revert of this terminal-partition follow-up; the signal-cleanup,
prepared-receipt, cadence-prep isolation, receipt-boundary, fixture-sentinel
and after-bind commits remain independently revertable.

## Paired controller state lexical validation before claim

Codex P2 [r3964610622](https://github.com/1XP-AI/gh-runnerd/pull/62#discussion_r3964610622)
on `7ff0e96`: an absolute non-clean `ControllerStateDirectory` (trailing slash)
was accepted by `openBrokerPrivateDirectory`, then `openBrokerAdmission`
appended the permanent `paired-terminal` claim, then
`invokeBrokerPairedPreparation` refused `filepath.Clean(path) != path`. No
child, mint, or authenticated call ran, but the corrected canonical path could
not retry. The existing command contract already refuses non-clean paths; this
follow-up applies that refusal before admission instead of rewriting signed
authority. Claims are not cleared. Worker trailing-slash and relative paths
were already refused before claim; f1b receipt/root identity is unchanged.

TDD red at `7ff0e96` before this correction (Go 1.26.8, darwin/arm64), using
real disposable 0700 directories:

```text
GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=90s \
  -run '^TestPairedBrokerRejectsNoncanonicalControllerStateBeforeClaim$' .
FAIL; g02-auth 1.28s
trailing slash: permanent paired claim appended
dot: permanent paired claim appended
dot-dot: permanent paired claim appended
relative PASS; worker trailing slash PASS
```

Green after this correction:

```text
GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=90s \
  -run '^TestPairedBrokerRejectsNoncanonicalControllerStateBeforeClaim$|^TestPairedBrokerAcceptsCanonicalControllerStateDirectory$|^TestBrokerControllerModeKeepsCanonicalStateDirectory$|^TestPairedBrokerRealEntrypointUsesPairedPreparationClosure$|^TestBrokerPrivateFileFrontDoorDiscoveryAndSecretFreeState$|^TestBrokerControllerPayloadUsesActualIssuanceAndPrivateHandoff$' .
PASS; g02-auth 4.100s

GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=90s \
  -run '^TestPairedBrokerRejectsNoncanonicalControllerStateBeforeClaim$|^TestPairedBrokerAcceptsCanonicalControllerStateDirectory$|^TestBrokerControllerModeKeepsCanonicalStateDirectory$|^TestPairedBrokerRealEntrypointUsesPairedPreparationClosure$' .
PASS; g02-auth 27.773s
```

Invalid lexical controller paths now return `errBroker` with no paired claim,
API call, mint, or child; a following canonical attempt on the same admission
root completes once. Controller-only canonical handoff remains compatible.
Rollback is a source-only revert of this entry/path validation follow-up.

## Issue 60 hosted job-cap decomposition

This focused follow-up starts after the path-validation correction at
`7b528a6fc3feca1612bfdc88c721f18147b24050` and owns only the hosted workflow,
its focused tooling contract test, this CI guide, and this evidence record. It
does not change the G01/G02 partition scripts or runtime code.

The coordinator's REST annotation for Public CI run `34310027322`, job
`102334625450`, records a cancellation because the job exceeded its 15-minute
outer budget: it started at `04:12:11` and ended at `04:27:26`. Root/tooling
checks consumed `10m52s`; offline experiments started at `04:23:03` and were
cancelled after `4m21s`. The annotation identifies neither a test-case failure
nor a manual cancellation. Increasing bounded exhaustive fixture coverage made
the former single sequential job's aggregate budget too small.

This outer job-cap finding is separate from hosted run `34306339973`, where a
G01 terminal test package reached its own `120.025s` process deadline with
`TestPairedTerminalPreParentFailureRetainsReturnedOnlyTerminal` active. The
older result was a package-level G01 partition timeout and motivated the
already-recorded terminal split; run `34310027322` is a workflow-level
aggregate-cap cancellation. Neither result is evidence of a new test assertion
failure, and this change does not widen either timeout.

The workflow now runs root/tooling, offline experiments, and the pinned
vulnerability scan in separate hosted `ubuntu-24.04` jobs, each capped at 15
minutes. Every test-bearing job checks out
`${{ github.event.pull_request.head.sha || github.sha }}` with
`persist-credentials: false`, uses the existing reviewed checkout/setup-go
commits, and disables setup-go caching. The required public `Go checks` name is
an explicit `always()` aggregator over all three jobs; it checks every
`needs.<job>.result` and exits nonzero for any result other than `success`,
including `failure`, `cancelled`, and `skipped`. The aggregator has no source
checkout because it only evaluates dependency status.

## TDD and focused verification

Before decomposition, the generated workflow contract test failed against the
single `checks` job:

```text
GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=90s \
  -run '^TestPublicWorkflowCapacityContract$' ./scripts
FAIL; missing required job "root"; got jobs [checks]
```

After decomposition, the same contract test generated status fixtures for all
success, root failure, offline cancellation, and vulnerability skipped cases;
only the all-success fixture passed the extracted aggregator script:

```text
GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=90s \
  -run '^TestPublicWorkflowCapacityContract$' ./scripts
PASS; scripts 0.428s

GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=90s \
  -run '^TestPublicWorkflowCapacityContract$' ./scripts
PASS; scripts 1.446s

GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=420s ./scripts
PASS; scripts 363.880s

ruby -e 'require "yaml"; y=YAML.load_file(".github/workflows/ci.yml"); abort "missing jobs" unless y["jobs"] || y[true]; p(y["jobs"] || y[true]).keys'
PASS; ["root", "offline", "vuln", "checks"]

git diff --check
PASS
```

The contract also requires every existing `make` command exactly once,
including both `make test` and `make test-race`, every worker's 15-minute cap,
the immutable ref and pinned actions, read-only permissions, disabled caches,
no secrets/self-hosted/target workflow, and all required aggregator
dependencies. No workflow manual rerun, self-hosted runner operation, runner
cleanup, action-version warning cleanup, or unrelated code change was used.

Rollback is the exact source-only command
`git revert --no-edit <Issue-60-hosted-job-cap-decomposition-SHA>`; it removes
the four-job workflow and contract/docs follow-up while leaving the pathfix,
G01/G02 partition caps, runtime changes, and manually installed runners
untouched. A rollback does not authorize workflow replay or live runner
operations.

## Remaining gates

This worker does not merge PR 62. After push, request `@codex review` on the
exact new head. Hosted CI, independent Luna review of the new head, and a clean
exact-head Codex verdict remain required before any merge decision. Live
recovery remains unauthorized and unproven. Historical headroom finding
r3957835501 stays open until Codex re-reviews this head; CI green does not
clear it. Hosted 34306339973 is not treated as a proven target deadlock.
Codex P2 r3964610622 is addressed in source on this head and needs an
exact-head re-review.
