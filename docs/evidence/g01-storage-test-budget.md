# G01 storage-test process budget

Writer checkpoint: 2026-10-25, base
`98e950394d2d2075dee969361fb5337fba67a71e`. This is the single change-impact
ledger for the bounded offline-checker correction following PR #109. It changes
test orchestration only, not production authentication, protocol or runner code.

## Repeated failure and preserved scope

The [PR #109 main run](https://github.com/1XP-AI/gh-runnerd/actions/runs/40172002974/attempts/1)
passed root/tooling, root race, G02, R1 and vulnerability jobs. Its last G01
tagged storage invocation hit the existing 120-second package timeout while
`TestPairedTerminalPostIntentJournalIdentity` subcases were active. Earlier G01
partitions passed; the aggregate correctly failed. No assertion identified an
authentication-scope failure. The [postmerge record](https://github.com/1XP-AI/gh-runnerd/pull/109#issuecomment-6394855855)
keeps that failure explicit and schedules no retry.

The same storage boundary had already timed out on the preceding image-pin main
in [PR #108](https://github.com/1XP-AI/gh-runnerd/pull/108#issuecomment-6394411863).
That source's local diagnostic and one unchanged hosted retry passed, but did
not prove an infrastructure-only cause or eliminate the recurring budget issue.
The present correction isolates the observed family rather than increasing the
deadline, weakening assertions or rerunning until green.

The storage union is unchanged. Journal identity now runs alone, followed
serially by the same union with only that exact family skipped. Both commands
retain `-race -count=1 -timeout=120s -tags=g01_pair_fixture`. Existing heavy,
terminal remainder, collection, default, CLI and vet selections are unchanged.
Module inventory, fixed Go 1.27.3, source discovery and G02 resource cleanup are
also unchanged. The checker retains `set -euo pipefail`; either new process's
failure stops the gate before a success marker.

## Coverage and process evidence

The registry lists actual compiled terminal tests and assigns each once:

| Partition | Actual top-level names |
| --- | ---: |
| Existing heavy | 5 |
| Existing terminal remainder | 15 |
| Storage journal identity | 1 |
| Storage remainder | 5 |
| Total, unchanged | 26 |

The registry requires the journal name to belong to the existing storage union,
proves no overlap/omission and verifies its exclusion from the storage remainder.
The existing executable witness test now includes a journal-only failing
witness, a positive journal marker and the new exact invocation. It continues
to prove failure propagation and exactly-once positive markers for examples,
fuzz seeds, collection, heavy/remainder, future terminal names and storage.

A new independent positive witness records its actual process IDs from the
journal and another storage family. The old checker executed both exactly once
in the same process: **RED**, exit 1, package 12.646s (test 12.29s). This is an
executed isolation failure, not merely a textual regex expectation or a planned
timing experiment. After splitting the checker, this focused race command
passed, exit 0, package 99.193s:

```text
go test -race -count=1 -timeout=240s -run '^(TestToolingG01StoragePartitionsUseSeparateProcesses|TestG01PairedTerminalPartitionRegistry|TestToolingTaggedPairFixturePartitionsRun)$' ./scripts
```

The 240-second limit belongs only to this aggregate tooling regression process,
which repeatedly runs disposable synthetic modules. It does not change any
120-second G01 partition or per-child tooling deadline. `go vet ./scripts`,
`bash -n scripts/check-offline-experiments.sh` and `git diff --check` also passed.
The real storage families were then run serially from the G01 module, without
other local validation in parallel:

```text
go test -race -count=1 -timeout=120s -tags=g01_pair_fixture -run '^TestPairedTerminalPostIntentJournalIdentity$' ./livecanary
# PASS, exit 0, 25.297s
go test -race -count=1 -timeout=120s -tags=g01_pair_fixture -run '^TestPairedTerminal(Actual(Controller|Worker)SyncFailures|PostIntent(JournalIdentity|AuthorityBoundaries)|ClosedReplayActualFile|WorkerReceiptSurvivesControllerWriteFailure|FixtureStorageFailure)$' -skip '^TestPairedTerminalPostIntentJournalIdentity$' ./livecanary
# PASS, exit 0, 85.884s
```

These are actual synthetic storage/recovery test results, not an inference from
the tooling witnesses or a new full-module/main-matrix pass. No real canary
resource or server-side recovery behavior was exercised.

## Environment, review and rollback

Local commands use the existing allowlisted stock Go 1.27.3 Darwin/ARM64
environment, empty `GOEXPERIMENT`, `GOENV=off`, `GOWORK=off`, `GOAUTH=off`,
`GOTELEMETRY=off`, `GOMAXPROCS=2`, `GOFLAGS=-p=2`, cgo and fixed trusted
compiler/cache/toolchain locations. No ambient credentials or private proxy
were inherited. Tests use only inspected synthetic fixture sources and owned
temporary files, not live canary resources.

Independent review, GitHub Codex review and same-head PR quick remain pending
for this new tooling candidate. Record their receipts and postmerge main matrix
in the PR rather than creating another source head solely to copy results.
The prior authentication-scope reviews are reused only for unchanged code.

Rollback is reverting this checker/test/ledger change as one unit; the previous
combined storage budget would then return. There is no state migration, token
issuance, workflow dispatch, runner/container/service change or new cleanup
operation. G01/full G02 and live authorization gates remain unresolved.
