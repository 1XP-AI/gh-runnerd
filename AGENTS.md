# gh-runnerd contributor/agent instructions

This is a plan-first repository. Do not treat example CLI commands as implemented features.

- Work from a GitHub issue and its Goal, acceptance criteria and dependencies. One issue, one branch/worktree, one active goal when the task requests goal execution.
- Follow docs/EXECUTION.md. User-authorized model settings are Astra (`gpt-6-astra`, `xhigh`) for risky architecture/auth/protocol/lifecycle code and Luna (`gpt-5.6-luna`, `max`) for bounded work after contracts are approved. Do not override an explicit current user setting.
- Use TDD for implementation: meaningful failing test -> minimal implementation -> refactor -> relevant verification. Document actual results; never claim planned/skipped/live tests passed.
- Preserve live manually installed runners during development. No global Docker prune/context change, broad process kill, unreviewed destructive cleanup or automatic workflow replay.
- Credentials, raw SDK response errors, JIT configs, personal machine paths and private test logs must not enter commits/issues/diagnostic bundles.
- Native macOS runners are for explicitly trusted code. Do not claim same-user workdirs or Keychain provide hostile-code isolation.
- Keep GitHub SDK behavior behind an adapter and pin versions. Resolve G01/G02 evidence gates before dependent implementation.
- Limit concurrency and resources globally across pools; ordinary scale-down drains busy work.
- Public PR tests use hosted environments without secrets. Real Mac/self-hosted tests require reviewed commits and explicit maintainer dispatch under runner-group policy.
- Use Go unless an ADR supported by evidence changes the decision. Keep dependency count small and review licenses. No restricted virtualization binary or macOS image bundled by default.
- Independent agents may work in parallel only when an active task authorizes delegation and file ownership/dependencies are clear. Integrator owns shared interfaces.
