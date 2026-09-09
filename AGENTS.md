# gh-runnerd contributor/agent instructions

This is a plan-first repository. Do not treat example CLI commands as implemented features.

- Work from a GitHub issue and its Goal, acceptance criteria and dependencies. One issue, one branch/worktree, one active goal when the task requests goal execution. Live GitHub Project status is dispatch authority; do not redispatch from historical `docs/backlog.json` Ready values.
- Follow docs/EXECUTION.md. The repository default routes implementation, review and coordination through Luna (`gpt-5.6-luna`, `max`) unless an explicit current user override is recorded on the issue. The maintainer-authorized override for delivery-reframe issue #66 and R1 children #67/#68/#69 is main author Grok 4.6 xhigh with independent Luna max review. Preserve the TDD, independent-review, exact-head Codex+CI and live-operation authorization gates. Historical records keep the model that actually produced them. Do not override an explicit current user setting.
- Use TDD for implementation: meaningful failing test -> minimal implementation -> refactor -> relevant verification. Document actual results; never claim planned/skipped/live tests passed.
- Preserve live manually installed runners during development. No global Docker prune/context change, broad process kill, unreviewed destructive cleanup or automatic workflow replay.
- Credentials, raw SDK response errors, JIT configs, personal machine paths and private test logs must not enter commits/issues/diagnostic bundles.
- Native macOS runners are for explicitly trusted code. Do not claim same-user workdirs or Keychain provide hostile-code isolation.
- Keep GitHub SDK behavior behind an adapter and pin versions. Resolve G01/G02 evidence gates before dependent production implementation. Completing the G02 R1 subset does not complete G02.
- Limit concurrency and resources globally across pools; ordinary scale-down drains busy work.
- Public PR tests use hosted environments without secrets. Real Mac/self-hosted tests require reviewed commits and explicit maintainer dispatch under runner-group policy.
- Use Go unless an ADR supported by evidence changes the decision. Keep dependency count small and review licenses. No restricted virtualization binary or macOS image bundled by default.
- Independent agents may work in parallel only when an active task authorizes delegation and file ownership/dependencies are clear. Integrator owns shared interfaces.
- Internal agent approval and passing CI do not replace GitHub Codex review. Before merge, confirm Codex completed review of the exact current PR head; read both inline and issue-comment findings, including stale/outdated ones, and address each actionable finding with a reproduced fix or an evidence-based rebuttal. Never treat staleness or an untimestamped reaction as proof of resolution.
- After fixes are pushed, request `@codex review` and wait for completion before checking the final head again. Unresolved findings, pending review or an unreviewed head block merge. Record finding URLs, reproduction results and resolution evidence; use a fresh fix PR for a finding in already-merged code.
