# TDD, reliability and performance evidence

This document specifies future tests. None of these product tests has run in this planning repository.

## Every implementation issue

1. Derive observable invariants and a counterexample from the issue goal.
2. Write a failing test and capture why it fails (behavior absent/broken, not bad setup).
3. Implement the smallest behavior that satisfies it.
4. Refactor while keeping the contract green.
5. Add boundary/failure tests justified by the issue's risks.
6. Record exact commands, relevant output, environment, remaining gaps and a reviewed PR.

Do not manufacture red evidence after implementation, count snapshots as safety evidence or require tests for prose-only edits. Tests should survive internal refactors and catch meaningful regressions.

## Layers

| Layer | Required evidence |
|---|---|
| Pure logic | Fake clocks/randomness; state transitions; caps; fairness; no unbounded loops or sleeps |
| Persistence | Real temporary SQLite DB; rollback/migrations; injected crash at each intent/side-effect/commit boundary |
| GitHub protocol | Fake HTTP/session service; pagination, 401/403/429, reconnect, replay, ACK timing, stale stats, capacity and JIT lifecycle |
| Provider contract | Common create/inspect/drain/cleanup suite; observed external IDs; idempotence; unknown state retention |
| Linux integration | Real Docker; parallel PostgreSQL/Redis services; job containers; mounts and ports; no host-socket exposure |
| macOS integration | Native processes, launchd identity, process-tree cleanup, same-UID trust caveat, locked Keychain and startup behavior |
| End to end | Private test repositories/Apps, two org installations, actual workflow routing and one-job teardown |
| Reliability | Kill/restart manager, Docker unavailable, network outage, token rotation, disk pressure, drain under load and restart reconciliation |
| Release | Reproducible build inputs, checksums/provenance, license inventory, install/uninstall/upgrade rollback and clear preview status |

## Invariants

- Busy jobs are never killed by ordinary scale-down. Unknown activity is not treated as idle.
- Reservations cover creating, ready, busy and unresolved workers; shared caps cannot be exceeded by concurrent orgs.
- Retried side effects use stable ownership/idempotency keys and do not create uncontrolled duplicate workers.
- Crashes after message ACK do not permanently strand desired capacity: reconcile from statistics and owned resources.
- A worker executes at most one job before disposal; job rerun policy stays with GitHub/operator.
- Cleanup deletes only owned resources. Cross-pool fixed port/workdir collisions are rejected or avoided.
- Secrets and raw secret-bearing SDK errors do not reach public logs, diagnostic bundles or fixtures. Management credentials never enter job bootstrap environments; only per-worker JIT data may reach its worker through the G01-approved transport, with residual exposure documented.
- Fair admission makes progress for equally eligible pools when slots become available.
- No exact-once execution, arbitrary-job sandbox or restart checkpointing claim is inferred from green unit tests.

## CI policy

Bootstrap public PR checks on GitHub-hosted standard Linux: format/vet, meaningful unit tests, race tests where supported, fuzz smoke, dependency/license checks and `govulncheck`. Pin action commits and dependency versions. Split deterministic tests from credentialed hardware suites; skipped tests must be visible and never count as passing evidence.

Trusted hardware tests use reviewed immutable commits, maintainer-triggered runs and scoped credentials on a dedicated pool. No public-fork `pull_request_target` checkout onto local runners. Test the Linux provider on ARM64 hardware. A hosted check must not require the unreleased runner manager itself to work.

## Release candidate gate

Before calling a release candidate stable, complete a planned 24-hour mixed-workload soak with at least 100 jobs across both org installations; repeated scale up/down; a controlled manager crash/restart; token refresh; network and runtime outage; parallel DB jobs; and host restart evidence. The threshold is a release gate, not a test result already obtained. Record failures, expected interrupted jobs and what must be rerun manually. Require no unexplained orphan workers, cap violations, unintended busy-job termination or credential leakage.

Measure manager CPU/RSS, admission latency, API requests, provision-to-ready time, queue duration and disk growth separately from user job costs. Set an initial regression budget from the prototype baseline before optimization; keep hardware/workload identical for comparisons. Tests must not expand runner counts until measured resource headroom exists.

Runner freshness cases: simulate a stale runner/image denied new jobs and a security-required update while a job is busy. New admission pauses, replacement workers use the verified version, busy work drains, and rollback never selects a prohibited old version. Record runner-update ownership and scheduling impact.
