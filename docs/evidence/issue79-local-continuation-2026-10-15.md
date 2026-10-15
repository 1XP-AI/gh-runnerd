# Issue #79 local continuation — 2026-10-15

Issue: https://github.com/1XP-AI/gh-runnerd/issues/79

## Source and scope

The resumed local inspection started from Cursor's clean branch head
`1407f88a58a796a7bec8b1736ca1e179dc8678d3`. That head contains 17 commits
after the Go 1.26.9 packet-profile commit `f362d54`. Inspection covered the
changed scanner boundaries and regression specimens, the trusted definition
loader, and the current-packet static scan. This is an implementer's local
review, not an independent review or a GitHub exact-head approval.

The packet under test is `docs/evidence/g01-recovery-packet.md`, Git blob
`6f80dc5bfe2c351ec39814234d82e07de7e6324c`. This continuation does not change
that packet. Recipes and negative specimens remain inert text/AST; none were
executed. Only inspected trusted harness/scanner definitions ran. No private
credentials, live runners, GitHub writes, push, PR, or merge were involved.

## Finding and minimal fix

The harness previously entered scanner analysis on Python 3.9.6 despite missing
structural-pattern AST classes. An initial focused invocation produced 62
`AttributeError` errors across 11 test methods. That is a runtime setup failure,
not the behavior-change RED evidence below. The scanner also references
`ast.TryStar`, so its required AST surface needs Python 3.11 or later.

The fix checks the five required AST node classes before extracting/loading
scanner definitions. Both the loader and CLI reject missing support; the CLI
prints a single diagnostic and exits 1. Supported runtimes retain the loader.
No scanner acceptance rule or toolchain pin was relaxed.

## Actual TDD and verification

All supported-runtime checks below used Python 3.14.3 with isolated mode `-I`,
no bytecode writes `-B`, and an allowlisted PATH/locale-only environment.

- RED, before the implementation: `ScannerRuntimeTests` ran 2 methods in
  0.007 seconds, with 5 failing missing-feature subcases. Each reached the mocked
  definition loader instead of refusing first. The supported-runtime positive
  control passed.
- GREEN, after the implementation: the same 2 methods passed in 0.001 seconds.
- Post-fix focused integration: those 2 methods plus the 11 newly added Cursor
  boundary methods passed together (13 methods, 1.708 seconds). Boundaries cover
  imported OS aliases, call-site resolution, callable defaults, nested defaults,
  quoted heredocs, implicit exception disclosure, complex conversions,
  conversion aliases, argument flow, mutated mapping keys, and sensitive key
  representations. Negative specimens were inspected, never executed.
- Post-fix packet toolchain profile: 4 tests passed in 0.023 seconds.
- Additional changed-risk controls: reviewed-path readers, concatenated
  reflective reader names, and unrelated nested function-name collisions
  passed (3 methods, 0.303 seconds). Both rejecting and accepting specimens
  were inspected as data.
- Direct unsupported-runtime CLI check on Python 3.9.6: asserted exit 1,
  exactly one stderr line naming the missing AST support, and no traceback.
- Current-packet static scan: 1 test passed in 364.828 seconds (wrapper wall
  time 365.445 seconds), exit 0; 261 shell commands and 86 Python heredoc bodies
  inspected with zero violations. This ran once against the unchanged packet
  blob above, using the post-fix harness. Packet examples were not executed.
- `git diff --check`: passed during editing; repeated before the local commit.

## Limitations and rollback

The full regression suite, hosted checks, independent review, GitHub Codex
review, and live qualification were not run in this continuation. This local
result does not complete all acceptance criteria, close #79, merge its PR, or
satisfy G01/G02 production gates. The live board had no Ready issues when checked;
historical backlog values do not authorize starting dependent implementation.

Rollback is a local revert of this continuation's commit. Cursor's existing
packet and all historical evidence remain unchanged.
