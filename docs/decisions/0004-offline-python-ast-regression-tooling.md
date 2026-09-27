# ADR 0004: Narrow Python exception for offline Python-AST evidence tests

Status: accepted for the issue #79 evidence-tooling scope upon merge of PR #103;
the candidate remains subject to independent review and exact-head Codex review.

## Context and evidence

[ADR 0001](0001-language-and-boundaries.md) selects Go for the CLI, background
service, scheduler and state reconciliation. That product decision is unchanged.
The [G01 evidence packet](../evidence/g01-recovery-packet.md) already contains a
Python AST-based audit of Python and shell prescriptions. Issue #79 needs tests
of that existing implementation, including Python loop/comprehension targets,
call aliases and definition-time expressions.

At candidate `387a647355e48d44333954fadf48d5b02335290c`, the standard-library
Python harness reproduced the seven recorded finding classes and passed eight
focused cases after their initial corrections. Independent review then found
additional reader/iterator gaps; [Codex review of that exact candidate](https://github.com/1XP-AI/gh-runnerd/pull/103#pullrequestreview-5325780932)
also identified loader and language-policy gaps. Those findings remain blockers
until their correction or evidence-based disposition is reviewed; the eight-test
result is not comprehensive safety proof. The local interpreter used for this
evidence is CPython 3.14.3; no other interpreter/platform coverage is implied.

The harness uses Python's standard-library `ast`, `unittest` and local Git
fixtures, without third-party Python packages. Implementing a separate Python
parser in Go would duplicate the language semantics under test or introduce a
parser dependency. A Go wrapper that invokes the same Python checks would not
remove the interpreter dependency. No comparative maintenance or performance
benchmark has been run; this decision rests on testing the existing AST audit
directly and keeping the exception bounded.

## Decision

Permit Python only for
`scripts/evidence_packet/issue79_regression_test.py` and its offline regression
fixtures for the existing packet audit. This is not permission to implement
product behavior or general repository tooling in Python. Any broader use needs
a separately reviewed decision. The CLI, daemon and production adapters remain
Go; no Python interpreter or package is bundled into release artifacts.

The harness must use only the standard library and explicitly selected local
Git fixture operations. Invoke it with `python3 -B` to avoid bytecode artifacts
and record the actual interpreter and test results. Adding dependencies,
automatic hosted execution, or a broader supported interpreter matrix requires
separate review; this ADR does not claim those checks have run.

## Trust and execution boundaries

- Python/shell regression specimens remain data for AST/token inspection; they
  must not be evaluated or launched.
- The scanner functions are the reviewed implementation under test, not
  untrusted executable test data. This harness is not a Python sandbox or a
  replacement for source review and runner trust policy.
- Loading packet definitions must reject unreviewed imports, decorators and
  non-reviewed definition-time expressions before evaluation. A modified packet
  must not acquire an import/decorator/default-expression execution path merely
  because it contains a matching scanner fence.
- Local Git fixtures use task-owned temporary repositories and synthetic values.
  The harness must not contact GitHub, dispatch workflows, access credentials,
  or operate existing runners, Docker, Keychain or launchd.
- Repository/PR review, exact-head identity and live-operation authorization
  gates still apply. Neither this exception nor green synthetic tests complete
  G01/G02 or establish hostile-code isolation.

## Consequences and rollback

Maintainers now have one explicitly scoped interpreter-dependent evidence test.
Its manual focused results must be reported separately from Go/hosted checks;
passing Public CI does not imply this Python harness ran. Reconsider the exception
if the packet audit is extracted or replaced by a reviewed implementation with
equivalent regression coverage.

Rollback is a reviewed revert of the harness and this exception, retaining Go
product code and unrelated evidence. Do not remove an existing safety check
without an explicit replacement or a documented reopening of the affected gate.
