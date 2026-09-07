# Contributing

Start with an issue in the implementation Project. Read [the plan](docs/PLAN.md), [agent execution](docs/EXECUTION.md) and [TDD strategy](docs/TEST-STRATEGY.md).

Keep changes scoped to an observable issue goal. Include a reproducing failing test for behavior changes, relevant checks, independent review of security/concurrency changes, and accurate limitations. Documentation-only changes do not require artificial tests.

The public baseline is documented in [CI.md](docs/CI.md). Run `make check` before submitting a change; it uses the pinned Go toolchain and reports absent unit or fuzz suites explicitly.

Do not run unreviewed contributions on privileged self-hosted runners. Do not submit credentials or private workflow logs. Dependencies and assets must have documented licenses compatible with distribution.
