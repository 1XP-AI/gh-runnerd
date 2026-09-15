# Contributing

Start with an issue in the implementation Project. Read [the plan](docs/PLAN.md), [agent execution](docs/EXECUTION.md) and [TDD strategy](docs/TEST-STRATEGY.md).

Keep changes scoped to an observable issue goal. The writer follows red -> minimal
green -> refactor, then runs focused unit/negative checks while iterating;
documentation-only changes record why no artificial test is needed. Use
`make fast` only with explicit `FAST_MODULE`, `FAST_PACKAGE` and `FAST_TEST`
selectors; it fails closed for missing or no-match selectors and is not the full
gate. Keep intermediate commits local. Batch source, documentation and
finding-ledger fixes before pushing one stable review candidate rather than
launching hosted CI and a review for every local commit. Repeat the PR quick gate
only after the head or relevant risk boundary changes.

Independent reviewers use the immutable candidate source and exact-source CI
evidence, adding delta/risk probes instead of repeating the complete suite. A
second independent security/recovery pass remains required when the changed
boundary warrants it. The coordinator audits the contract, ledger and evidence;
the coordinator is not a third full-suite tester. Carry resolved findings forward
with their original URL, source SHA and resolution evidence, and sign off the
final delta against the exact candidate SHA. Group only valuable non-blocking
hardening into follow-up issues; give routine P2/P3/nit findings a one-time
disposition instead of opening speculative work.

The public baseline is documented in [CI.md](docs/CI.md). `make check` remains the
complete local validation suite, while the hosted PR quick workflow is the required
stable candidate gate; neither is an automatic per-commit requirement. The full
Public CI matrix runs once on source-affecting merges to `main` as integration
evidence rather than on every PR push; documentation-only pushes are filtered
out. The final merge gate still requires exact-head GitHub Codex
review, the PR quick check for that same SHA, stale/outdated finding inspection
and triage, resolution or rebuttal of every blocking finding, linked follow-up
issues for valuable non-blocking hardening, one-time dispositions for routine
findings, and any applicable security second pass.

Release, macOS, soak and other trusted/live checks run before the applicable
release or live qualification only, with an immutable reviewed commit and
explicit maintainer authorization. No mandatory security gate is deferred.

Do not run unreviewed contributions on privileged self-hosted runners. Do not submit credentials or private workflow logs. Dependencies and assets must have documented licenses compatible with distribution.
