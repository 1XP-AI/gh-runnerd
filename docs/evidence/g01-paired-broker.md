# G01g: paired terminal executable and bounded broker handoff

Issue [60](https://github.com/1XP-AI/gh-runnerd/issues/60) connects the reviewed
paired terminal sequence to one tagged `g01-live` executable and a dedicated
`g01-broker` mode. This is an offline experiment continuation, not a live
authorization, production daemon, or closure of G01/G02.

## Implementation boundary

The paired broker approval now requires the explicit `paired-terminal` phase.
The canonical `runBrokerWithAPI` path invokes a dedicated fixed
`--prepare-approved-paired-journal` child contract and accepts only its
paired-terminal preparation receipt. The preparation contract proves a fresh
controller journal and admission claim under controller authority; it does
not borrow cleanup authority, read credentials, contact a remote service, or
authorize worker effects.

After preparation, the broker captures the exact controller snapshot and
worker approval byte hashes plus controller/worker approval and state-root
device/inode identities. The fixed child argv carries only those paths and a
bounded credential-free binding; the controller-only credential payload is
bounded and contains no PEM. The child validates the binding before reading
controller credentials and before constructing SDK/Docker adapters, compares
the argv binding with the broker-supplied payload binding, and revalidates the
same identities throughout the terminal sequence and before completion. The
canonical controller approval and journal-derived `PairInput` remain the only
pairing authority; the binding is identity evidence, not a second pairing
manifest.

The exported `livecanary.RunPairedTerminal` path now owns paired journal/API/
Unix adapter construction. A private `g01_pair_fixture` seam redirects only
generated temporary journal/admission roots, the synthetic private TLS API,
and the Unix fixture; it does not expose runtime authority or alter account,
Keychain, runner, Docker, or service state. The tagged fixture calls the
exported entrypoint and verifies two session acknowledgements, one acquire,
JIT, create/start, original-session close, non-force worker deletion plus
absence, owned-set deletion plus absence, complete rosters, real journal/lease
behavior, and secret-free journals. Cancellation, lost response, reopened
history, changed approval bytes/inodes, changed state roots, and symlinked
roots fail before new effects.

Paired child execution has a fixed argv and `LANG=C`/`LC_ALL=C` environment,
bounded output, a 30-second child timeout, cancellation handling, no retry,
and one logical controller stdin consumption. Existing controller-only,
discovery, and separate-worker paths retain their prior refusal and authority
boundaries.

## TDD evidence and checks

The immutable review baseline retained meaningful red behavior in commit
`3acd5ab`:

```text
GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=60s -tags=g01_live -run '^TestPairedTerminalMode' ./cmd/g01-live
exit 1: paired mode refused before its credential input gate (reads=0)

GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=60s -run '^TestPairedTerminalBrokerApprovalUsesDedicatedMode$' ./...
exit 1: paired terminal broker approval was refused
```

Before implementing the canonical entrypoint fix, the new behavioral
regression test was run against the frozen implementation:

```text
GOTOOLCHAIN=go1.26.8 go test -count=1 -run '^TestPairedBrokerRealEntrypointUsesPairedPreparationClosure$' .
exit 1: real paired entrypoint did not complete one handoff ... mints=0
```

The focused green checks then passed:

```text
GOTOOLCHAIN=go1.26.8 go test -count=1 -run '^TestPairedBrokerRealEntrypointUsesPairedPreparationClosure$|^TestBrokerPaired' .
ok   github.com/1XP-AI/gh-runnerd/experiments/g02-auth  1.665s

GOTOOLCHAIN=go1.26.8 go test -tags g01_live -count=1 ./cmd/g01-live
ok   github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/cmd/g01-live  0.846s

GOTOOLCHAIN=go1.26.8 go test -tags g01_pair_fixture -count=1 -timeout=120s -run '^TestPairedTerminalExported|^TestPairedTerminalBinding' ./livecanary
ok   github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset/livecanary
```

Pinned verification completed without live resources:

```text
GOTOOLCHAIN=go1.26.8 bash scripts/check-offline-experiments.sh
offline experiment checks passed: 2 module(s)

GOTOOLCHAIN=go1.26.8 go test -race -count=1 ./...
ok: root module

GOTOOLCHAIN=go1.26.8 go vet ./...
ok: root module

git diff --check
ok
```

The offline gate covers both module race/vet suites, tagged `g01-live`/
`g01-worker` CLI tests, tagged paired fixture partitions, and the G02 module
suites on Go 1.26.8. The command and tooling files owned by issue #61 were not
edited; their canonical command updates still need integration by that task.

No live GitHub endpoint, App, credential, runner/group/workflow, Docker/Lima
configuration, Keychain, launchd service, reboot, or manually installed
runner was touched. The same-UID private-file model is not hostile-code
isolation, and no production daemon or G01 recovery/live completion is
claimed.

## Remaining gates and rollback

Independent Luna/max review, hosted CI, and exact-head GitHub Codex review are
still required. The coordinator owns those review, stale-finding, CI, and
merge gates; a pending or unreviewed exact head blocks merge.

Offline rollback is source-only: revert the focused issue-60 commit(s). The
tests create only temporary local TLS/Unix fixtures and require no runner,
Docker, Keychain, launchd, or GitHub cleanup.
