# Contributing

Start with an issue in the implementation Project. Read [the plan](docs/PLAN.md), [agent execution](docs/EXECUTION.md) and [TDD strategy](docs/TEST-STRATEGY.md).

Keep changes scoped to an observable issue goal. The writer follows red -> minimal
green -> refactor, then runs focused unit/negative checks while iterating;
documentation-only changes record why no artificial test is needed. Use
`make fast` only with explicit `FAST_MODULE`, `FAST_PACKAGE` and `FAST_TEST`
selectors; it fails closed for missing or no-match selectors and is not the full
gate. Batch source, documentation and finding-ledger fixes before pushing one
stable review candidate rather than launching a review for every local commit.

Independent reviewers use the immutable candidate source and exact-source CI
evidence, adding delta/risk probes instead of repeating the complete suite. A
second independent security/recovery pass remains required when the changed
boundary warrants it. The coordinator audits the contract, ledger and evidence;
the coordinator is not a third full-suite tester. Carry resolved findings forward
with their original URL, source SHA and resolution evidence, and sign off the
final delta against the exact candidate SHA.

The public baseline is documented in [CI.md](docs/CI.md). `make check` remains the
complete public validation suite and hosted PR CI remains the required stable
candidate gate; it is not an automatic per-commit requirement. The final merge
gate still requires exact-head GitHub Codex review, required CI for that same SHA,
stale/outdated finding resolution or rebuttal, and any applicable security second
pass. Main's postmerge integration run does not replace those premerge checks.

Release, macOS, soak and other trusted/live checks run before the applicable
release or live qualification only, with an immutable reviewed commit and
explicit maintainer authorization. No mandatory security gate is deferred.

Do not run unreviewed contributions on privileged self-hosted runners. Do not submit credentials or private workflow logs. Dependencies and assets must have documented licenses compatible with distribution.
