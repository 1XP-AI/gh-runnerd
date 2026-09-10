# Goal-driven issue execution

The [GitHub Project](https://github.com/orgs/1XP-AI/projects/2) is the planning/control surface. Each implementation issue has one concrete goal, dependencies, acceptance criteria, TDD evidence and a primary agent setting. Models are labels/Project fields, not GitHub user assignees. No issue starts automatically merely because it appears on the board.

## Model allocation

| Work | Implementer | Review |
|---|---|---|
| Default, including auth, protocol, concurrency, resource admission, lifecycle, providers, architecture, tests, packaging and docs | Luna / `gpt-5.6-luna`, `max` | Independent Luna `max`; use a second independent Luna `max` pass for security/recovery boundaries |
| Explicit current override on [#66](https://github.com/1XP-AI/gh-runnerd/issues/66) and R1 children [#67](https://github.com/1XP-AI/gh-runnerd/issues/67)/[#68](https://github.com/1XP-AI/gh-runnerd/issues/68)/[#69](https://github.com/1XP-AI/gh-runnerd/issues/69) | Grok 4.6 xhigh | Independent Luna `max` |

The repository default is Luna max, including contract-setting evidence gates such as G01, G02 and G04, unless an explicit current user override is recorded on the issue. Do not overwrite that override with the default. Historical Astra/Luna assignments in review and evidence records stay unchanged. The Project `Agent` field currently offers Astra xhigh and Luna max; Grok 4.6 xhigh is recorded in the named issue bodies until a Project option exists. Do not rewrite #1/#2/#60 Agent values to make this planning change look like those issues changed owners.

Verify each issue's contract and dependencies before dependent implementation begins. Escalate discovered architecture, secret handling, process isolation or concurrency changes to an additional independent Luna max review; do not stretch a small issue into an unreviewed redesign. Parallelize only independent issues with non-overlapping file ownership; no simultaneous edits to shared protocol/state definitions.

## Incremental validation and review

Validation is incremental during editing and complete at the stable merge
candidate. Do not require or automatically run the whole suite for every local
commit. The writer records a meaningful red case for behavior changes, applies a
minimal green fix, and runs the explicit focused unit/negative checks relevant to
the changed surface; documentation-only changes record why no artificial test is
needed. The opt-in `make fast` entry point requires an explicit module, package,
and test selector, fails closed for missing/invalid selectors, and never stands in
for the complete gate.

| Role/tier | Required work and evidence |
|---|---|
| Writer / edit | Red -> minimal green -> refactor; focused checks and `git diff --check` while iterating. Batch source, docs and finding-ledger changes before one review candidate push. |
| Independent reviewer(s) | Review the immutable candidate source and shared exact-source CI evidence; run only delta/risk probes. Add the independent Luna max security/recovery pass when the changed boundary warrants it. Do not duplicate the full suite by default. |
| Coordinator | Audit the contract, changed surface, finding ledger and exact-source CI/review records; coordinate resolution. The coordinator is not a third full-suite tester. |
| Candidate / premerge | The pushed PR head receives the complete hosted CI matrix once stable. Preserve required job/check names, complete coverage, exact pull-request-head checkout, cache policy and timeouts. |
| Main / postmerge | The same workflow on `main` is integration evidence after merge; it does not replace the premerge candidate gate or authorize a merge. |
| Release/live qualification | Run release, macOS, soak and other trusted/live profiles only before the applicable release or live qualification, on a reviewed immutable commit with explicit maintainer authorization. Do not defer a mandatory security gate. |

Use one finding ledger per candidate: carry each previously resolved finding with
its immutable source SHA, original finding URL and resolution evidence, then record
the final delta sign-off against the exact candidate SHA. A new internal full review
is needed only when the changed diff crosses a new risk or interface boundary;
focused delta review remains required for ordinary fixes. GitHub Codex review and
the required CI gate are separate and stricter: immediately before merge, Codex
must have reviewed the exact final HEAD, stale/outdated findings must be read and
resolved or rebutted, required CI must pass for that same SHA, and a security
second pass remains required where applicable. Do not request repetitive Codex
reviews mid-edit; after a fix push, request and await the fresh exact-head review.

## Per-issue goal workflow

1. Select a Ready issue from the live GitHub Project whose dependencies are Done; read the plan, relevant ADR and current repository instructions. Do not treat `backlog.json` `status` as live Ready.
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

> Work on ISSUE_URL using the issue's implementer (default Luna max unless an explicit current user override is recorded) and one active goal equal to its Goal statement. Independent review is Luna max even when the implementer is overridden. Read AGENTS.md and linked design decisions. Verify dependencies first. Use a separate worktree, write the meaningful failing test before implementation, and preserve the no-secrets/no-busy-kill/owned-cleanup invariants. Do not change existing live runners or enroll new Apps unless the issue explicitly authorizes that operation. Open a reviewed PR with commands/results, red evidence, limitations and rollback notes. Update the Project accurately; do not mark the goal complete while required work remains. Do not write Luna over an inspected override that has no Project Agent option; preserve the existing Agent value.

## Board fields

Status, Stage, Priority, Agent, Risk, Goal, Dependencies, Test profile and Release. Stage/milestones preserve the original M0–M5 taxonomy. Release is additive sequencing (R1 Internal MVP, R2 Everyday operations, R3 General distribution, Future research) and does not change acceptance text or native dependency gates. Initial gates/bootstrap work is Ready; dependent full-scope work is Backlog. R1 children use their own documented dependencies: #67 is Ready with no production dependency; #68 is natively Blocked by #60/#66/#67, and after those complete authorized work is the independently reviewed minimal contract and offline evidence only until full G01 #1 and G02 #2 pass, with production implementation starting only then; #69 is Blocked by full G01 #1 and #68. Ready on a child is not a bypass of the parent's remaining blockers. Completing #67 does not complete G02 and does not satisfy AGENTS.md G01/G02 gates for #68 production or for G04+ full-scope work. R1 is not independently deliverable until the R3-placed full G02 #2 gate passes; do not move full G02 onto R1. VM/fleet research is explicitly Future. Machine-readable `test_profile` values are in `backlog.json`. JSON `status` and `agent` on original records are the initial/historical planning snapshot, not live dispatch authority; see `field_semantics` in that file. The live GitHub Project is authoritative after work begins. Do not redispatch from historical Ready values. trusted-runtime and trusted-live-github profiles require maintainer-controlled execution and never run on public PR code.

G08 enrollment and G15 service lifecycle remain gated by their contracts and independent review. Default implementation and review are Luna max; recorded overrides such as Grok 4.6 xhigh on #66/#67/#68/#69 still require independent Luna max review. G12 computes pure proposals, while G05/G13 own atomic reservation and live admission under the parent full-scope gates.

Use one issue per bounded outcome and linked dependencies instead of assigning all future work active goals at once.
