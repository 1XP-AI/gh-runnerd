# Goal-driven issue execution

The [GitHub Project](https://github.com/orgs/1XP-AI/projects/2) is the planning/control surface. Each implementation issue has one concrete goal, dependencies, acceptance criteria, TDD evidence and a primary agent setting. Models are labels/Project fields, not GitHub user assignees. No issue starts automatically merely because it appears on the board.

## Model allocation

| Work | Implementer | Review |
|---|---|---|
| Default, including auth, protocol, concurrency, resource admission, lifecycle, providers, architecture, tests, packaging and docs | Luna / `gpt-5.6-luna`, `max` | Independent Luna `max`; use a second independent Luna `max` pass for security/recovery boundaries |
| Explicit current override on [#66](https://github.com/1XP-AI/gh-runnerd/issues/66) and R1 children [#67](https://github.com/1XP-AI/gh-runnerd/issues/67)/[#68](https://github.com/1XP-AI/gh-runnerd/issues/68)/[#69](https://github.com/1XP-AI/gh-runnerd/issues/69) | Grok 4.6 xhigh | Independent Luna `max` |

The repository default is Luna max, including contract-setting evidence gates such as G01, G02 and G04, unless an explicit current user override is recorded on the issue. Do not overwrite that override with the default. Historical Astra/Luna assignments in review and evidence records stay unchanged. The Project `Agent` field currently offers Astra xhigh and Luna max; Grok 4.6 xhigh is recorded in the named issue bodies until a Project option exists. Do not rewrite #1/#2/#60 Agent values to make this planning change look like those issues changed owners.

Verify each issue's contract and dependencies before dependent implementation begins. Escalate discovered architecture, secret handling, process isolation or concurrency changes to an additional independent Luna max review; do not stretch a small issue into an unreviewed redesign. Parallelize only independent issues with non-overlapping file ownership; no simultaneous edits to shared protocol/state definitions.

## Per-issue goal workflow

1. Select a Ready issue whose dependencies are Done; read the plan, relevant ADR and current repository instructions.
2. In the implementation task, create **one active goal** from the issue's Goal statement. Do not invent a token budget. Record issue URL and goal status in that task.
3. Create a dedicated branch/worktree. Keep one issue's behavior in one PR; split only if the issue's acceptance contract requires it.
4. Follow red -> green -> refactor, with meaningful failure evidence before the fix and relevant automated checks afterward.
5. Ask the assigned independent reviewer to check invariants and failure cases. Independent review is Luna max unless the issue records a different reviewer; an implementer override does not change the reviewer. Record reviewer/model and outcomes in the PR. This internal review is separate from the GitHub Codex review.
6. Move Project status to In review. Wait for GitHub Codex to finish reviewing the exact current PR head. Use the configured `codex-review` skill to read both inline reviews and issue-comment findings, including stale/outdated findings. Reproduce each finding; fix it or provide a specific evidence-based rebuttal. After pushing fixes, request `@codex review` and wait for the new result. Merge only when required CI and both review paths are complete, no actionable finding remains unresolved, and the issue's existing authorization permits it. Check the current head immediately before merge and constrain the merge to that SHA.
7. Close the issue and mark its goal complete only when all acceptance criteria and evidence are satisfied. If the goal includes merge, a merely opened PR is not completion.

Keep dependent issues blocked until evidence gates pass. Production implementation of #68 is dependent work under that rule and waits for full G01 #1 and G02 #2; completing #67 does not complete G02. Native #68 blockers remain #60/#66/#67 and authorize only reviewed contract and offline evidence work until those gates pass. A blocked issue needs a concrete blocker and an independently useful next step if one exists. Follow the host's actual goal-tool blocked threshold; do not mark a goal blocked after a single inconvenience. GitHub Project Goal text is a durable work specification, not an active Codex goal or an automatic scheduler.

If review arrives after a PR was merged, audit the finding against current `main`
and use a fresh issue-linked fix PR. Keep the original review thread open until
the correction or rebuttal has concrete evidence; link the reviewed fix and its
validation before resolving it. A moved anchor, stale original commit or an
untimestamped thumbs-up does not establish that a finding was addressed. Record
review/merge timing when auditing a missed review instead of implying the review
was complete at merge time.

## Reusable dispatch text

> Work on ISSUE_URL using the issue's implementer (default Luna max; #66/#67/#68/#69 are Grok 4.6 xhigh) and one active goal equal to its Goal statement. Independent review is Luna max even when the implementer is overridden. Read AGENTS.md and linked design decisions. Verify dependencies first. Use a separate worktree, write the meaningful failing test before implementation, and preserve the no-secrets/no-busy-kill/owned-cleanup invariants. Do not change existing live runners or enroll new Apps unless the issue explicitly authorizes that operation. Open a reviewed PR with commands/results, red evidence, limitations and rollback notes. Update the Project accurately; do not mark the goal complete while required work remains.

## Board fields

Status, Stage, Priority, Agent, Risk, Goal, Dependencies, Test profile and Release. Stage/milestones preserve the original M0–M5 taxonomy. Release is additive sequencing (R1 Internal MVP, R2 Everyday operations, R3 General distribution, Future research) and does not change acceptance text or native dependency gates. Initial gates/bootstrap work is Ready; dependent full-scope work is Backlog. R1 children use their own documented dependencies: #67 is Ready with no production dependency; #68 is natively Blocked by #60/#66/#67, and after those complete authorized work is the independently reviewed minimal contract and offline evidence only until full G01 #1 and G02 #2 pass, with production implementation starting only then; #69 is Blocked by full G01 #1 and #68. Ready on a child is not a bypass of the parent's remaining blockers. Completing #67 does not complete G02 and does not satisfy AGENTS.md G01/G02 gates for #68 production or for G04+ full-scope work. VM/fleet research is explicitly Future. Machine-readable `status` and `test_profile` values are in `backlog.json`; trusted-runtime and trusted-live-github profiles require maintainer-controlled execution and never run on public PR code.

G08 enrollment and G15 service lifecycle remain gated by their contracts and independent review. Default implementation and review are Luna max; recorded overrides such as Grok 4.6 xhigh on #66/#67/#68/#69 still require independent Luna max review. G12 computes pure proposals, while G05/G13 own atomic reservation and live admission under the parent full-scope gates.

Use one issue per bounded outcome and linked dependencies instead of assigning all future work active goals at once.
