# G01g: paired terminal executable and bounded broker handoff

Issue [60](https://github.com/1XP-AI/gh-runnerd/issues/60) connects the reviewed
paired terminal sequence to one tagged `g01-live` executable and a dedicated
`g01-broker` mode. This is an offline experiment continuation, not a live
authorization, production daemon, or closure of G01/G02.

## Implementation boundary

`g01-live` now has a mutually exclusive
`--execute-approved-paired-terminal` mode. It accepts only the controller
approval/state and explicit worker approval/state inputs; phase, controller-only
execution and worker flags are rejected before controller credential stdin is
read. The executable validates both approvals, shared nonce/harness/workflow/
controller identity, workflow-run authority, expiries, required terminal phases,
immutable build revision and distinct private state roots. It then reads one
bounded controller credential payload and invokes the existing terminal sequence
in the same process. Controller and worker journals are acquired in order and
released in reverse order; worker JIT remains an in-memory handoff and is never
sent to a worker stdin or subprocess.

The broker accepts a dedicated `paired-terminal` approval and requires worker
approval/state flags only for that mode. It holds exact worker approval bytes,
approval-file identity and state-root identity through preparation, binds them
into the native-account admission event, requires workflow identity verification
before launch, mints once, and invokes one fixed `g01-live` argv with a minimal
environment. PEM remains in the broker; only bounded controller credentials are
sent to the child. Existing controller-only, discovery and separate worker
paths retain their prior mode/refusal behavior.

Failure boundaries remain fail-stop: changed worker approval/state, mismatched
shared identity, consumed paired admission, invalid phases/expiry/build,
cancellation, storage uncertainty, bounded child output overflow or child
failure returns a fixed refusal/quarantine result without retry or resume.

## TDD evidence and checks

Red tests were preserved in commit `3acd5ab` (`test(g01): preserve paired
handoff red cases`). With the implementation temporarily absent, the meaningful
CLI test failed before input consumption:

```text
TestPairedTerminalModeReadsControllerInputAfterAllGates: code=1 reads=0
output="canary refused; approval, authority or private state requires review"
```

The broker mode test independently failed with the fixed broker refusal. The
green implementation adds the same-process adapter, broker worker binding and
fixed child handoff, plus negative tests for unused flags, missing workflow
verification authority, changed worker approval and mismatched shared identity.

The paired broker behavioral fixture uses only generated temporary files,
synthetic HTTP responses and bounded child test processes. It verifies worker
binding before authenticated work, workflow verification before launch, one
installation-token mint, one handoff, no PEM in the payload or admission
ledger, and the dedicated `paired_terminal_completed` result. Existing tagged
private TLS/Unix fixtures continue to exercise one acquire/JIT/create/start,
original-session close, non-force worker delete with separate absence, owned-set
delete with absence, complete rosters and secret-free journals.

Final offline checks on Go 1.26.8/Darwin ARM64:

```text
GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=90s ./...        # G01 passed
GOTOOLCHAIN=go1.26.8 go vet ./...                                   # G01 passed
GOTOOLCHAIN=go1.26.8 make experiments                               # passed: 2 module(s)
```

The offline gate also passed its tagged `g01-live,g01-worker` CLI tests,
tagged vet, collection/terminal/storage fixture partitions, and both G02
module suites. `git diff --check` passed. No live GitHub endpoint, App,
credential, runner/group/workflow, Docker/Lima configuration, Keychain,
launchd service or existing runner was touched.

The repository-wide `GOTOOLCHAIN=go1.26.8 make check` also passed: formatting,
build, root unit/race tests, the configured fuzz smoke, module verification,
license inventory, both offline experiment modules and pinned `govulncheck`
(`v1.7.0`, no vulnerabilities found).

## Remaining gates and rollback

Live execution remains unperformed and requires the exact reviewed immutable
artifact, explicit maintainer dispatch and separately approved resources. The
same-UID private-file model is not hostile-code isolation; crash recovery,
uncertain acquisition/JIT/session reconciliation and any successor live run
remain separate evidence gates. Independent Luna/max review, hosted CI and
exact-head GitHub Codex review are still required before merge.

Offline rollback is source-only: revert the focused issue-60 commits. The
experiment creates no persistent live resource and requires no runner, Docker,
Keychain, launchd or GitHub cleanup.
