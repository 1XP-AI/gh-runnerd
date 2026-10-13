# Remaining Project work: bounded parallel dispatch

Planning checkpoint: 2026-10-14 Asia/Seoul. The live
[GitHub Project](https://github.com/orgs/1XP-AI/projects/2) has 44 cards:
20 Done, 3 In progress, 17 Backlog, 3 Blocked, and 1 Future. The remaining
24 cards are not 24 completed implementations. There are no Ready cards in
this snapshot. This plan neither changes the board nor dispatches gated work.

## Current execution choice

- Cursor Agent: Muse Spark 1.3 Max.
- Codex: GPT-6.1 Sol, medium reasoning; do not reuse Luna Max for new work.
- At most two active workers. One issue per branch/worktree. Reuse suitable
  existing checkouts; create another only for an eligible distinct issue.
- Local implementation, relevant red/green checks, and coherent commits only.
  Independent review and GitHub review are deferred by current user choice.
  No push, PR creation, merge, issue closure, or live operation is implied.
- The coordinator owns shared interfaces and integration. No concurrent
  changes to common configuration, protocol/state types, or the Git index.
- Deferred reviews are not passing reviews. Local completion does not satisfy
  an issue's remaining merge, qualification, or live evidence criteria.

## Wave and ownership map

The owner column is a proposed allocation, not an active agent or completion
claim. Before each dispatch, read that issue's current acceptance contract and
Project status; dependencies must be Done. Path boundaries are assigned then,
not invented here as already implemented features.

| Wave / lane | Issues | Proposed owner | Scope boundary / entry condition |
|---|---|---|---|
| Gate A | #1 G01, #79 supporting evidence | Codex Sol medium, one at a time | Scale Set delivery/acquisition/JIT/drain evidence. Preserve the two existing local branches; no repeated unchanged checks or specimen execution. Full G01 remains open. |
| Gate B | #2 G02, #67 R1 credentials | Muse Max, one at a time | Enrollment, credential/service identity, and foreground credential evidence. #67 does not complete #2. Keep credential work separate from Gate A's protocol files. |
| 1: contracts | #4 G04 | Codex Sol medium | Requires #1/#2/#3 Done. Freeze configuration/provider/ownership/scaling contracts before parallel consumers; coordinator owns shared interfaces. |
| R1 lane | #68 foreground lifecycle -> #69 qualification | Muse Max, serial | #68 requires #60/#66/#67 and full #1/#2 before production implementation. Its isolated contract/offline scope is not production authorization. #69 requires #1/#68; real-job evidence needs concrete authorization. |
| 2: state | #5 G05 | Codex Sol medium | Requires #3/#4. Durable state and operation journal; owns persistence interfaces. |
| 2: scaling | #12 G12 | Muse Max | Requires #4/#5. Pure deterministic proposals, not live reservations. Existing completed #40 arithmetic is reused, not reimplemented. Can run beside #6 after #5 settles. |
| 3: daemon | #6 G06 | Codex Sol medium | Requires #5. Supervision and authorized local IPC; owns daemon/IPC surface. |
| 3: credentials | #7 G07 | Muse Max | Requires #2/#5. Credential and installation-binding implementation; separate from daemon/IPC files. Queue after #12 if the two-worker cap is reached. |
| 4: providers / enrollment | #8 G08, #9 G09, #10 G10, #11 G11 | Codex: #9/#11; Muse: #8/#10 | Queue pairs, never four workers. #8 needs #6/#7; #9 needs #1/#5/#7; #10 needs #4/#5/#7; #11 needs #2/#4/#5/#7. Each issue owns its adapter/enrollment files, not shared contract definitions. |
| 5: integration | #13 G13 | Codex Sol medium | Requires #6/#9/#10/#11/#12. One integrator for reconciliation, drain, restart and atomic admission. |
| 6: diagnostics | #14 G14 | Muse Max | Requires #6/#13. Sanitized status/logs/diagnostics, separate from service lifecycle. |
| 6: service / trust | #15 G15, #19 G19 | Codex Sol medium, serial | #15 needs #2/#6/#13; #19 needs #8/#10/#11/#13. Service/startup and trust-boundary work do not authorize installing services or changing host identities. |
| 7: runtime qualification | #16 G16 | Codex Sol medium | Requires #8/#13/#14/#15. Two-organization runtime evidence; concrete trusted environment and maintainer authorization required. |
| 8: reliability | #17 G17 | Codex Sol medium | Requires #16/#19. Recovery, soak and resource budgets; actual authorized evidence, not planned passes. |
| 9: release | #18 G18 | Muse Max | Requires #3/#15/#17. Reproducible packaging/signing/distribution; no publication without authorization. |
| 10: migration | #20 G20 | Codex Sol medium | Requires #17/#18. Reversible legacy-runner handoff; preserve manually installed runners. |
| Future | #21 G21 | Not dispatched | Requires #17 and remains Future. Optional VM/multi-host research is not part of current production implementation. |

All 24 unfinished cards appear exactly once. The R1 lane is a separate delivery
lane, not a bypass of the full G01/G02 gates. The nominal wave numbers do not
override the dependency column; in particular #12 starts only after #5.

## Immediate boundary: what actually unlocks implementation

Gate A: G01 still lacks the authorized product canary and its real server-side
recovery/drain observations. See the
[canary authority checklist](../evidence/g01-live-canary.md#required-environment-and-authorization-record).
Its local offline evidence and Go remediation do not authorize registering
resources, issuing JIT, dispatching jobs, fault injection, or cleanup.
The #79 local evidence-hardening branch is preserved; remote closure/merge is
not part of the current local-only workflow.

Gate B: G02 already records a disposable enrollment/manual recovery and bounded
source-build/same-account platform observations. Do not repeat those as though
they were never collected. The
[G02 readiness packet](../evidence/g02-mac-live-readiness.md#acceptance-status)
distinguishes those results from remaining service identity/support and
qualification boundaries; its retired execution procedure must not be run.
The [R1 foreground evidence](../evidence/g02-r1-foreground.md) is offline evidence,
not #67's authorized real manual-credential result or full G02 completion.

Concrete live targets, resource limits, phases and ownership must be authorized
separately. Do not request secret values in chat. Until the relevant evidence
and dependencies are satisfied and the live Project permits dispatch, later
waves are queued plans, not background implementations.

## Exit accounting

Record per issue: changed files, actual red/green and focused checks, local
commit, remaining criteria, and worktree cleanup decision. A finished local
unit can be committed while the parent issue stays open. Clean up only fully
finished, inactive, recoverable worktrees; preserve unpushed work and uncertain
ownership. This planning change has no application behavior change and needs
no artificial Go test or additional review loop.
