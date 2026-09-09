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

This bounded CI-contract follow-up starts at frozen PR head
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
the fixed wait is 45.302s, which exceeds the 45-second process budget.

TDD red before isolating fixture preparation:

```text
GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=30s \
  -run '^TestG02CadenceHeadroomSeparatesFixturePrepFromFixedWait$' ./scripts
FAIL: cadence partition still absorbs fixture clone/build instead of a bounded
preparation stage
```

The G02 offline script now runs a bounded 45-second
`TestPairedBrokerPrepareReviewedG01LiveBinary` process, then the unchanged
45-second cadence name, then remaining `^TestPaired` skipping prep and cadence,
then the unfiltered `^TestPaired` complement. The 45-second default, race,
count=1, `./...` discovery, and seven real five-second gaps are unchanged. No
120-second cadence timeout was added.

Green:

```text
GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=30s \
  -run '^TestG02CadenceHeadroomSeparatesFixturePrepFromFixedWait$' ./scripts
PASS

GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=180s \
  -run '^TestToolingDefaultG02PartitionsRun$' ./scripts
PASS; scripts 164.882s

GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=180s \
  -run '^TestToolingDefaultG01PartitionsRun$' ./scripts
PASS; scripts 25.775s

GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=45s \
  -run '^TestPairedBrokerPrepareReviewedG01LiveBinary$' .
PASS; g02-auth 2.957s

GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=45s \
  -run '^TestPairedBrokerRealCadenceChildExceedsThirtySeconds$' .
PASS; g02-auth 39.054s

GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=45s \
  -run '^TestPaired' \
  -skip '^TestPairedBroker(PrepareReviewedG01LiveBinary|RealCadenceChildExceedsThirtySeconds)$' ./...
PASS; g02-auth 12.055s

git diff --check
PASS
GOTOOLCHAIN=go1.26.8 bash scripts/gofmt.sh check
PASS
```

Stressed model after the split: fixture clone/build occupies its own 45-second
process (measured 2.957s). Cadence retains the 35-second wait plus module race
compile/bridge (measured 39.054s). Twice the remaining non-wait overhead still
fits the 45-second cadence budget; twice the combined in-process overhead did
not. Residual cadence headroom is the race compile of the test binary, not
hidden unbounded work.

## Remaining gates

This worker does not merge PR 62. After push, request `@codex review` on the
exact new head. Hosted CI, independent review of the new head, and a clean
exact-head Codex verdict remain required before any merge decision. Live
recovery remains unauthorized and unproven. Rollback is a source-only revert
of this cadence-prep isolation follow-up; earlier receipt-boundary,
fixture-sentinel and after-bind commits remain independently revertable.
