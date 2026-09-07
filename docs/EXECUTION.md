# Goal-driven issue execution

The GitHub Project is the planning/control surface. Each implementation issue has one concrete goal, dependencies, acceptance criteria, TDD evidence and a primary agent setting. Models are labels/Project fields, not GitHub user assignees. No issue starts automatically merely because it appears on the board.

## Model allocation

| Work | Implementer | Review |
|---|---|---|
| Auth, protocol, concurrency, resource admission, destructive lifecycle, providers and architecture | Astra / `gpt-6-astra`, `xhigh` | Independent Astra `xhigh` on security/recovery boundaries |
| Bounded CLI presentation, pure policy proposals under an Astra-approved contract, test harnesses, packaging/docs | Luna / `gpt-5.6-luna`, `max` | Astra `xhigh` for changed security/recovery interfaces; otherwise an independent contract review |

Use Luna only after the contract and dependencies are settled. Escalate discovered architecture, secret handling, process isolation or concurrency changes to Astra; do not stretch a small issue into an unreviewed redesign. Parallelize only independent issues with non-overlapping file ownership; no simultaneous edits to shared protocol/state definitions.

## Per-issue goal workflow

1. Select a Ready issue whose dependencies are Done; read the plan, relevant ADR and current repository instructions.
2. In the implementation task, create **one active goal** from the issue's Goal statement. Do not invent a token budget. Record issue URL and goal status in that task.
3. Create a dedicated branch/worktree. Keep one issue's behavior in one PR; split only if the issue's acceptance contract requires it.
4. Follow red -> green -> refactor, with meaningful failure evidence before the fix and relevant automated checks afterward.
5. Ask the assigned independent reviewer to check invariants and failure cases. Record reviewer/model and outcomes in the PR.
6. Move Project status to In review. Merge only after required checks/review and the issue's existing authorization/policy permits it.
7. Close the issue and mark its goal complete only when all acceptance criteria and evidence are satisfied. If the goal includes merge, a merely opened PR is not completion.

Keep dependent issues blocked until evidence gates pass. A blocked issue needs a concrete blocker and an independently useful next step if one exists. Follow the host's actual goal-tool blocked threshold; do not mark a goal blocked after a single inconvenience. GitHub Project Goal text is a durable work specification, not an active Codex goal or an automatic scheduler.

## Reusable dispatch text

> Work on ISSUE_URL using the issue's assigned model/effort and one active goal equal to its Goal statement. Read AGENTS.md and linked design decisions. Verify dependencies first. Use a separate worktree, write the meaningful failing test before implementation, and preserve the no-secrets/no-busy-kill/owned-cleanup invariants. Do not change existing live runners or enroll new Apps unless the issue explicitly authorizes that operation. Open a reviewed PR with commands/results, red evidence, limitations and rollback notes. Update the Project accurately; do not mark the goal complete while required work remains.

## Board fields

Status, Stage, Priority, Agent, Risk, Goal, Dependencies and Test profile. Initial gates/bootstrap work is Ready; dependent work is Backlog. VM/fleet research is explicitly Future. Machine-readable `status` and `test_profile` values are in `backlog.json`; trusted-runtime and trusted-live-github profiles require maintainer-controlled execution and never run on public PR code.

G08 enrollment and G15 service lifecycle are Astra-owned. Luna G12 computes pure proposals only; Astra G05/G13 owns atomic reservation and live admission.

Use one issue per bounded outcome and linked dependencies instead of assigning all future work active goals at once.
