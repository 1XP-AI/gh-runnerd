# gh-runnerd agent handoff

This is the operational handoff for an agent continuing the `1XP-AI/gh-runnerd`
project. It combines the repository rules, the GitHub Project control loop, the
runner pilot context, the current gates and the exact review/merge discipline.
Treat the live GitHub Project and the issue body as authoritative when they differ
from this snapshot. Re-read this file, `AGENTS.md` and the linked design documents
before changing code.

Snapshot: 2026-09-08 KST, documentation branch `orca/release-reframe` based on `main` `31ae8102f6f20f8e79258eb824af1400eba21954` (`[CI] Split default G01 deadline checks (#65)`). Live Project #2 has 36 items after the maintainer-accepted release reorganization ([#66](https://github.com/1XP-AI/gh-runnerd/issues/66)). This file is a snapshot; the live Project and issue bodies win when they differ.

## Mission and product boundary

`gh-runnerd` is a planned Go CLI and supervised local daemon for one Apple Silicon
Mac. It should manage multiple GitHub organizations and both trusted native macOS
and disposable Linux ARM64 runner pools, with fixed capacity, demand-based scaling,
safe drain, durable reconciliation and a shared host resource budget.

The repository is still a design and implementation backlog. The commands in the
README are intended interfaces, not a runnable product or a stability claim.

The original product envelope is deliberately bounded and is now sequenced as
three maintainer-accepted releases ([PLAN.md](PLAN.md#approved-delivery-releases)):

- **R1 Internal MVP:** this Mac, one organization, one private test repository,
  the existing reviewed Linux-container backend, concurrency one, manual App,
  foreground command. Full G01 ACK/acquisition/JIT recovery remains required.
  Daemon/install/launchd service are not implemented.
- **R2 Everyday operations:** install/start/stop/status, restart recovery and
  bounded scaling.
- **R3 General distribution:** multi-organization support, trusted native macOS
  backend, automated onboarding, signing/update/diagnostics.
- **Future:** G21 optional macOS VM / multi-host research.

Shared envelope limits that still apply:

- one macOS ARM64 manager on one Mac for R1/R2/R3;
- Linux workers through an explicitly selected Docker Engine connection (an existing
  Lima or Docker Desktop engine may be used without changing the global Docker
  context);
- one-job Linux workers with a worker-specific Docker daemon for service/container
  actions;
- native macOS one-job processes only for an explicitly trusted repository/domain
  (R3, not R1);
- GitHub App authentication owned by the operator and outbound GitHub connections;
- no public inbound endpoint, management SaaS, Kubernetes requirement, Windows
  provider, cloud overflow, fleet controller, billing service or macOS VM provider
  in this envelope;
- no automatic replay of an interrupted workflow and no claim that reconciliation
  is checkpointing or exactly-once execution;
- no implicit installation of software or global Docker context changes.

Linux still needs a Linux kernel/runtime on macOS. A native macOS runner cannot run
a Linux Actions job, and a Linux container cannot provide a macOS runner. Native
macOS execution remains a host process; optional VM/multi-host work is future
research in G21. Issue #1's exact Goal is unchanged.

## Host and current fallback runners

The pilot host is a 2024 Mac mini with an M4 Pro and 48 GiB RAM running macOS Tahoe
26.6.2. Existing manual runners are a fallback and must remain intact until a
controlled pilot succeeds.

The current fallback layout is:

| Scope | Runner/pool | Runtime | Current observation |
|---|---|---|---|
| 1XP-AI | `Mac` | native macOS ARM64, launchd service | self-hosted, macOS, ARM64, Default group |
| 1XP-AI | `linux-arm64-1xp-ai` | Lima VM `ci-linux`, Docker ARM64 | Linux service registered and active |
| 1XP-Inc | `linux-arm64-1xp-inc` | same Lima VM, Docker ARM64 | Linux service registered and active |

The Linux VM has previously reported `aarch64`; an ARM64 `hello-world` container
completed successfully. The two Linux registrations are separate organization
registrations and separate services. Do not put registration tokens, JIT tokens,
private keys, App IDs or personal machine paths in commits, issues, logs or this
handoff.

Runners generally need only outbound HTTPS to GitHub; a public inbound IP is not a
normal requirement. Runner registration is per organization and per worker
identity. Short-lived registration/JIT values must be created at the time they are
needed, never hard-coded. Generated runner names are implementation identities;
workflows must target stable pool labels, never a fixed name such as `ci`.

Do not change the current Lima/Docker context, stop or remove these manual services,
prune Docker globally, kill broad process trees, alter account/Keychain/launchd
state, enroll a live GitHub App, or create/remove live runners without explicit
maintainer authorization for the concrete operation.

## Repository source of truth

Read these in order:

1. [`AGENTS.md`](../AGENTS.md) — execution, safety and review rules.
2. [`docs/EXECUTION.md`](EXECUTION.md) — goal workflow, model routing and Project
   semantics.
3. [`docs/PLAN.md`](PLAN.md) — architecture, lifecycle, scaling and evidence gates.
4. [`docs/TEST-STRATEGY.md`](TEST-STRATEGY.md) — TDD layers, invariants, CI and
   release evidence.
5. [`docs/SECURITY-DESIGN.md`](SECURITY-DESIGN.md) — App, IPC, Docker and native
   trust boundaries.
6. The issue's linked ADRs, acceptance criteria and current discussion.

The repository uses Go. Proposed packages are `cmd/gh-runnerd`, `internal/config`,
`internal/control`, `internal/store`, `internal/github`, `internal/auth`,
`internal/scheduler`, `internal/worker/docker`, `internal/worker/native` and
`internal/telemetry`. Freeze shared interfaces in G04 before parallel provider
work. Keep the GitHub SDK behind a small adapter, pin versions, keep dependencies
small and do not introduce a plugin ABI in v0.1.

## GitHub Project control surface

The planning and dispatch surface is [Project #2, `gh-runnerd`](https://github.com/orgs/1XP-AI/projects/2)
in `1XP-AI`. Its GraphQL project ID and field IDs are stable references for the
current board; verify them with `gh project field-list` if GitHub reports a mismatch.

| Value | ID or URL |
|---|---|
| Repository | `1XP-AI/gh-runnerd` |
| Project | `2` / `PVT_kwDOD2M2gs4Bismw` |
| Agent field | `PVTSSF_lADOD2M2gs4Bismwzhhjobs` |
| Agent: Astra xhigh | `cf617d72` (historical only for completed work) |
| Agent: Luna max | `9317c27f` (repository default dispatch) |
| Release field | `PVTSSF_lADOD2M2gs4Bismwzhhr3bc` |
| Release: R1 - Internal MVP | `a0a78eb0` |
| Release: R2 - Everyday operations | `58b696ee` |
| Release: R3 - General distribution | `428d845e` |
| Release: Future research | `c52ca8fd` |
| Status field | `PVTSSF_lADOD2M2gs4BismwzhhjoZA` |
| Status: Backlog | `a4ee9f7f` |
| Status: Ready | `b2cc6cc3` |
| Status: In progress | `7a569f61` |
| Status: In review | `eff80b07` |
| Status: Blocked | `e15688b3` |
| Status: Done | `98236657` |
| Status: Future | `036ca5f5` |

`Agent` is routing metadata. Setting it does not start a Codex agent. A goal in a
Project item is a durable work specification; it is not an active Codex goal.
Grok 4.6 xhigh is recorded on #66/#67/#68/#69 issue bodies; it is not currently a
Project Agent option. Do not overwrite #1/#2/#60 Agent values for this planning
change.

### Live board snapshot

Run `gh project item-list 2 --owner 1XP-AI --limit 1000 --format json` before
dispatch. The last verified snapshot is 36 items. The full map is in
[ISSUES.md](ISSUES.md). Compact routing:

| Issues | Project status | Release | Current routing |
|---|---|---|---|
| #1 G01 | In progress | R1 | Luna max; **full** ACK/acquisition/JIT Goal unresolved |
| #2 G02 | In progress | R3 | Luna max; Manifest/multi-org/launchd remains here |
| #67 | Ready | R1 | Grok 4.6 xhigh / Luna review; child of #2; no native blockers |
| #60 G01g | In progress | R1 | Luna max; separate worktree/PR #62; do not edit from #66 |
| #66 | In progress | R1 | Grok 4.6 xhigh / Luna review; this documentation change |
| #68 | Blocked | R1 | child of #13; blocked by #60/#66/#67; contract-before-implementation |
| #69 | Blocked | R1 | child of #16; blocked by #1/#68 |
| #3 G03, #30, #40 G12a, #44, #46, #47, #50, #52, #54 G01f, #61, #64 | Done | R1 except #40 R2 | historical records preserved; #54 Done does not close #1 |
| #4–#7, #9, #10, #12–#15, #20 | Backlog | R2 | Luna max; original dependencies unchanged |
| #8, #11, #16–#19 | Backlog | R3 | Luna max |
| #21 G21 | Future | Future | optional macOS VM/multi-host research |

Issue labels and the initial planning inventory can retain historical Astra values.
For current dispatch, use the Project `Agent` field and the issue's current
execution contract. Do not rewrite historical records to make them look like new
work. Do not add further child issues or native edges without an explicit ask;
the R1 mapping is already applied.

### Status meanings

| Status | Use when |
|---|---|
| Future | Explicitly deferred research; never dispatch as a release dependency |
| Backlog | Known work whose dependencies or start conditions are not ready |
| Ready | All listed dependencies are Done and the issue can be picked up |
| In progress | One active goal and one issue branch/worktree are being worked |
| In review | Focused PR exists, local/internal review is complete and GitHub review is pending or active |
| Blocked | The issue cannot progress because of a concrete blocker; update the Project immediately, while the active goal follows its own repeated-blocker threshold |
| Done | Acceptance evidence, required reviews and merge are complete |

Do not move an issue to Ready merely because an agent is available. Do not move it
to Done when only a PR was opened or a test was planned.

## Model and delegation policy

The repository default routes **implementation, contract work, architecture,
authentication, protocol, concurrency, documentation, coordination and independent
review through `gpt-5.6-luna` with `max` reasoning**. This includes G01/G02/G04
evidence gates unless an issue records an explicit current user override.
Historical Astra/Luna assignments in old review and evidence records are facts
about who did that work and must stay unchanged. If a later task contains an
explicit current user model or effort override, that override takes precedence.

The maintainer-authorized override for [#66](https://github.com/1XP-AI/gh-runnerd/issues/66)
and R1 children [#67](https://github.com/1XP-AI/gh-runnerd/issues/67)/[#68](https://github.com/1XP-AI/gh-runnerd/issues/68)/[#69](https://github.com/1XP-AI/gh-runnerd/issues/69)
is main author Grok 4.6 xhigh with independent Luna max review. Do not create a
second native Goal on #1 while this planning work runs.

For every new issue, after checking for an explicit current user override:

- set the Project Agent field to `Luna max` when no override exists;
- when an override exists and the Project Agent field has that option, set it to
  that option; when the override has **no** Project option (today: Grok 4.6 xhigh
  on #66/#67/#68/#69), **skip the Agent edit** and leave the field unset — do not
  write Luna as a substitute implementer. Independent review remains Luna max;
- use one active goal whose objective is exactly the issue's `Goal` statement;
- do not invent a token budget;
- use an independent contract review with the current selected review model/effort
  (the default is Luna max), plus a second independent Luna max pass for security,
  recovery, secret handling, ownership or concurrency boundaries;
- parallelize only independent issues with explicit non-overlapping file ownership;
  one integrator owns shared protocol/state definitions;
- a Project field never launches work by itself.

Suggested handoff prompt to give the next agent:

> Work on ISSUE_URL in `1XP-AI/gh-runnerd`. Use the issue's implementer (default `gpt-5.6-luna` with `max`; #66/#67/#68/#69 are Grok 4.6 xhigh) and one active goal exactly equal to the issue's Goal statement; do not invent a token budget. Independent review is Luna max even when the implementer is overridden. Read `AGENTS.md`, `docs/EXECUTION.md`, the plan, the linked ADRs and the current Project item. Verify dependencies first. Create the goal and isolated branch/worktree, then set the item to In progress. Follow meaningful red test -> minimal green implementation -> refactor -> boundary/failure tests. Preserve no-secrets, no-busy-kill, owned-cleanup, stable-idempotency and trusted-native invariants. Do not change live runners, Docker context, App/Keychain/launchd state or GitHub credentials without explicit maintainer authorization. Open one focused PR with exact commands/results, red evidence, gaps and rollback notes. Obtain independent Luna max review, then the exact-head GitHub Codex review before merge. Update the Project, issue and goal only when their actual state changes. Do not write Luna into the Project Agent field over an issue-body-only Grok override.

## Per-issue Project workflow

The following loop is the required control path. Replace `ISSUE` with the issue
number being dispatched; never guess a Project item ID.

### 1. Inspect the issue and board

```sh
REPO=1XP-AI/gh-runnerd
OWNER=1XP-AI
PROJECT_NUMBER=2

gh auth status
gh project view "$PROJECT_NUMBER" --owner "$OWNER"
gh project item-list "$PROJECT_NUMBER" --owner "$OWNER" --limit 1000 --format json \
  | jq -r '.items[] | select(.content.number != null) |
      [.content.number, .content.title, .status, .agent] | @tsv' | sort -n

ISSUE=4
gh issue view "$ISSUE" --repo "$REPO" \
  --json number,title,body,comments,labels,url

# Native GitHub blocking relationships are separate from the issue body.
gh api graphql -f query='query($owner:String!,$repo:String!,$number:Int!){repository(owner:$owner,name:$repo){issue(number:$number){blockedBy(first:100){nodes{number title state url}} blocking(first:100){nodes{number title state url}}}}}' \
  -F owner="$OWNER" -F repo="${REPO#*/}" -F number="$ISSUE"
```

Read the complete issue body and comments. Inspect every native `blockedBy` issue
from the GraphQL response as well as the issue's documented Dependencies field;
every blocker must be closed and its acceptance actually complete before dispatch.
Then read the exact Goal, TDD cases, acceptance checklist, test profile and safety
invariants. If a blocker is open, stop and report that state for the authorized
issue. Select another issue only when a separate user task explicitly authorizes
that new scope.

### 2. Resolve the Project item and route the Agent field

```sh
PROJECT_ID=PVT_kwDOD2M2gs4Bismw
STATUS_FIELD_ID=PVTSSF_lADOD2M2gs4BismwzhhjoZA
AGENT_FIELD_ID=PVTSSF_lADOD2M2gs4Bismwzhhjobs
STATUS_IN_PROGRESS_ID=7a569f61
AGENT_LUNA_MAX_ID=9317c27f
# Default implementer is Luna max. Issue-body-only overrides with no Project
# Agent option must not be rewritten to Luna; leave Agent unset.
SKIP_AGENT_EDIT=0
AGENT_OPTION_ID="$AGENT_LUNA_MAX_ID"
case "$ISSUE" in
  66|67|68|69)
    SKIP_AGENT_EDIT=1
    AGENT_OPTION_ID=""
    ;;
esac

ITEM_JSON="$(gh project item-list "$PROJECT_NUMBER" --owner "$OWNER" \
  --limit 1000 --format json)"
ITEM_RECORD="$(printf '%s' "$ITEM_JSON" | jq -c --arg n "$ISSUE" \
  --arg repo "$REPO" '
    [.items[]
      | select((.content.number | tostring) == $n
          and .content.repository == $repo)]
    | if length == 1 then .[0]
      else error("expected exactly one Project item for the selected repository/issue")
      end')"
ITEM_ID="$(printf '%s' "$ITEM_RECORD" | jq -r .id)"
ITEM_STATUS="$(printf '%s' "$ITEM_RECORD" | jq -r .status)"
ITEM_AGENT="$(printf '%s' "$ITEM_RECORD" | jq -r '.agent // "(unset)"')"
test -n "$ITEM_ID" && test "$ITEM_ID" != null

if [ "$ITEM_STATUS" != "Ready" ]; then
  printf 'selected issue is not Ready (status=%s, agent=%s); coordinate and stop\n' \
    "$ITEM_STATUS" "$ITEM_AGENT" >&2
  exit 1
fi

if [ "$SKIP_AGENT_EDIT" = 1 ]; then
  printf 'issue %s has a Grok 4.6 xhigh override with no Project Agent option; leaving Agent unset (was %s)\n' \
    "$ISSUE" "$ITEM_AGENT"
else
  gh project item-edit --id "$ITEM_ID" --project-id "$PROJECT_ID" \
    --field-id "$AGENT_FIELD_ID" --single-select-option-id "$AGENT_OPTION_ID"
fi
```

If the item is already In progress for another active agent, the guard above stops
before any field edit; coordinate instead of starting a second implementation. Keep
the issue's durable Goal/Dependencies fields and `docs/backlog.json` aligned only
when the contract actually changes. If
the current user selected another supported model/effort, resolve its Project Agent
option ID with `gh project field-list` and replace `AGENT_OPTION_ID`; never overwrite
an explicit current selection with the default, and never write Luna over an
issue-body-only override that has no Agent option. Independent review stays Luna max.

### 3. Start one active goal and an isolated worktree

Create one active goal with the issue Goal copied exactly. The goal tool's token
budget is omitted unless the user explicitly supplies one. Link the issue URL and
record the starting Project status in the agent task. Use a branch/worktree named
for the issue, for example `feat/g04-contracts`; do not edit a shared checkout from
two agents. Set the branch variable at creation time and use that same value for
the worktree, push and PR:

```sh
BRANCH=feat/g04-contracts  # replace with the selected issue's unique branch
```

Before implementation, write down:

- the observable invariant and a counterexample;
- the meaningful failing test or negative check and why it represents missing
  behavior rather than a bad setup;
- the files owned by this issue and the files deliberately left to the integrator;
- the test profile (`offline`, `trusted-runtime`, `trusted-live-github`, etc.) and
  whether explicit maintainer authorization exists.

Only after the goal and isolated worktree/branch exist, claim the live execution
state in the Project:

```sh
gh project item-edit --id "$ITEM_ID" --project-id "$PROJECT_ID" \
  --field-id "$STATUS_FIELD_ID" --single-select-option-id "$STATUS_IN_PROGRESS_ID"
```

This ordering prevents an interrupted dispatch from advertising an active issue
without an active goal and workspace. If the goal or worktree cannot be created,
leave the item in its prior status and record the reason.

### 4. Implement with TDD and bounded resources

Follow red -> green -> refactor. Keep persistence tests on real temporary SQLite,
protocol tests on a fake HTTP/session service, and provider tests on the actual
boundary they claim to cover. Add only risk-justified failure/boundary cases.
Record exact commands, actual results, environment and gaps; “planned,” “skipped”
and “not reproduced” are not passing evidence.

Preserve these invariants:

- ordinary scale-down never kills busy work; unknown activity is not idle;
- reservations include creating, ready, busy and unresolved workers;
- retries use stable ownership/idempotency keys and do not create uncontrolled
  duplicates;
- a post-ACK manager crash reconciles from durable intent, owned resources and
  GitHub statistics without an exactly-once claim;
- every worker runs at most one job before disposal;
- cleanup deletes only resources whose ownership is proven;
- management credentials and raw secret-bearing SDK errors never enter job
  environments, fixtures, public logs or diagnostic bundles;
- native macOS execution is trusted-only; same-UID workdirs, Keychain and process
  groups are not hostile-code isolation;
- a shared host arbiter accounts for Docker daemons/services, creating workers,
  native processes and host reserve; stop admission under pressure rather than
  silently exceeding caps;
- workflows select stable pool labels, never generated runner names.

### 5. Review, PR and exact-head gate

Run the repository checks appropriate to the issue and create one focused PR. Public
CI must not require live App credentials, local runner access or the unreleased
manager. Trusted hardware/live tests require a reviewed immutable commit and
explicit maintainer-triggered execution on the scoped runner group.

```sh
git diff --check
make check                    # when the issue's files and environment support it
git add path/to/changed/files
git commit -m "..."
git push -u origin "$BRANCH"
gh pr create --repo "$REPO" --base main --head "$BRANCH" \
  --title "..." --body-file /path/to/reviewable-body.md
```

Set the Project item to In review only after the PR exists and the local/independent
review has checked the acceptance contract. Independent review is separate from
GitHub Codex review.

For Codex review, load the installed `codex-review` skill and run its
`scripts/codex-review.sh` wrapper from that skill directory. The wrapper is an
agent-environment tool and is not part of this repository. The ordinary
`gh pr view --json comments` output is incomplete and must not be used as the
merge gate. Resolve the skill directory from the local skill catalog rather than
copying the wrapper into the product repository.

```sh
CODEX_REVIEW=/path/to/installed/codex-review-skill/scripts/codex-review.sh
"$CODEX_REVIEW" status PR_NUMBER
"$CODEX_REVIEW" findings PR_NUMBER
"$CODEX_REVIEW" detail PR_NUMBER
"$CODEX_REVIEW" all PR_NUMBER
"$CODEX_REVIEW" request PR_NUMBER
"$CODEX_REVIEW" wait PR_NUMBER
```

Reproduce every actionable finding before fixing it. Report severity, file/line,
reproduction result and the fix or evidence-based rebuttal. The wrapper reads both
inline review comments and issue-comment findings, including stale/outdated ones.
After pushing a fix, request a new review and wait for it. A clean result must name
the current exact head SHA; an old clean verdict or an untimestamped reaction does
not clear a newly pushed commit. If a finding arrives after merge, create a fresh
issue-linked fix PR against current `main`; do not rewrite the historical PR.

Immediately before merging:

1. fetch the PR and verify the head SHA has not changed;
2. verify required CI is successful for that same SHA;
3. verify the exact-head Codex review is clean and all internal findings are
   resolved or rebutted;
4. copy the SHA named by the clean exact-head verdict and use the server-side
   conditional merge guard:

   ```sh
   PR=57
   REVIEWED_SHA=the-clean-verdict-sha
   CURRENT_SHA="$(gh pr view "$PR" --repo "$REPO" --json headRefOid --jq .headRefOid)"
   test "$CURRENT_SHA" = "$REVIEWED_SHA"
   gh pr merge "$PR" --repo "$REPO" --squash \
     --match-head-commit "$REVIEWED_SHA"
   ```

   This prevents a new unreviewed push from being merged between the check and the
   merge action.
5. verify `main`, the issue, Project status, linked PR and goal all reflect the
   actual outcome.

### 6. Close out or record a blocker

After merge, record the exact validation and review links in the PR/issue, close the
issue if its acceptance criteria are complete, set Project status to Done and mark
the active goal complete. If work remains, leave the issue In review or In progress
with a concrete next step. If a real blocker prevents progress, set the Project item
to Blocked immediately and record the blocker and next step; keep the active goal
open until the goal tool's own repeated-blocker threshold permits marking that goal
blocked. Do not call a transient CI timeout “blocked.”

## Goal roadmap and dependencies

The ordered roadmap is in [`docs/BACKLOG.md`](BACKLOG.md) and the full issue bodies.
Use this compact map to orient a new agent; the live Project decides what is Ready.

| Goal | Issue | Outcome |
|---|---:|---|
| G01 | #1 | Scale Set delivery, acquisition, drain and crash/replay evidence gate |
| G02 | #2 | App Manifest enrollment and macOS launchd credential identity gate |
| G03 | #3 | Go module, public CI and toolchain baseline (Done) |
| G04 | #4 | Versioned CLI/config/provider/resource/scaling contracts |
| G05 | #5 | Durable SQLite state and operation journal |
| G06 | #6 | Supervised daemon and authorized local Unix IPC |
| G07 | #7 | GitHub App credentials and installation binding |
| G08 | #8 | Guided init and optional Manifest enrollment |
| G09 | #9 | Recoverable Scale Set adapter |
| G10 | #10 | Disposable Linux Docker workers and isolated services |
| G11 | #11 | Trusted native macOS worker identity/lifecycle |
| G12 | #12 | Deterministic shared capacity and fair scheduling |
| G12a | #40 | Pure scaling targets and capacity arithmetic (Done) |
| G13 | #13 | Reconciliation, drain and safe restart |
| G14 | #14 | Status, logs and sanitized diagnostics |
| G15 | #15 | Startup, shutdown and safe configuration updates |
| G16 | #16 | Repeatable two-organization runtime qualification |
| G17 | #17 | Crash recovery, soak and resource-budget qualification |
| G18 | #18 | Signed, reproducible release distribution |
| G19 | #19 | Trust admission and cross-worker secret boundaries |
| G20 | #20 | Reversible pilot/migration with legacy fallback |
| G21 | #21 | Optional macOS VM and multi-host provider research (Future) |
| G02-R1 | #67 | R1 manual single-organization credentials (child of #2; parent remains R3) |
| G13-R1 | #68 | R1 foreground command, capacity one (child of #13; blocked by #60/#66/#67) |
| G16-R1 | #69 | R1 authorized real job plus required recovery (child of #16; blocked by #1/#68) |
| P66 | #66 | Delivery-plan documentation; not a runtime Goal |

G01 and G02 are evidence gates. G01's exact Goal and full ACK/acquisition/JIT
recovery remain required; child merges are not completion. G04 and all dependent
full-scope implementation remain behind their acceptance evidence. #68 is not a
G04/G13 bypass. G10/G16 require real ARM64 Docker and private test repositories.
G11/G19 require trusted native-process evidence and are R3. G17 is the reliability
release gate. G20 is the reversible pilot, not a license to remove the fallback
runners early. G20 remains natively blocked by #17/#18.

## Current review state and next action

### Merged policy and G01f

PR [#56](https://github.com/1XP-AI/gh-runnerd/pull/56) (`docs: route future work through Luna max`)
aligned default routing to Luna max and preserved historical Astra records.
PR [#55](https://github.com/1XP-AI/gh-runnerd/pull/55) / issue [#54](https://github.com/1XP-AI/gh-runnerd/issues/54)
G01f is merged and Done. Do not start a second G01f implementation. #1 remains
open because its full recovery Goal is not complete.

### Active runtime work in other worktrees (do not edit from #66)

- [#60](https://github.com/1XP-AI/gh-runnerd/issues/60) G01g / open PR [#62](https://github.com/1XP-AI/gh-runnerd/pull/62): bounded broker handoff. Blocks #68.
- [#2](https://github.com/1XP-AI/gh-runnerd/issues/2) readiness review / open PR [#59](https://github.com/1XP-AI/gh-runnerd/pull/59): G02 macOS live-validation packet. Do not treat it as R1 completion of #67.

Inspect those PRs in their own worktrees. This documentation change must not
modify runtime, CI or evidence docs belonging to those PRs.

### This planning change

Issue [#66](https://github.com/1XP-AI/gh-runnerd/issues/66) on `orca/release-reframe`.
Main author Grok 4.6 xhigh; independent Luna max review; exact-head Codex+CI before
merge. #68 stays Blocked until #60, #66 and #67 are complete and the minimal
foreground contract is reviewed. No live authorization and no merge of blocked
implementation are implied.

## Live-operation gate

The next live integration draft remains a paired private canary after current
G01 recovery evidence and the R1 command path exist. PR #55 / G01f is already
merged and does not by itself authorize live work. It must use a reviewed
immutable commit, a disposable private repository/App installation, scoped
runner-group access and explicit maintainer authorization for the concrete
target. No live authorization is present in this handoff. Issue #69 tracks R1
qualification and stays blocked by #1 and #68.

Until that authorization exists, an agent may build offline fakes, local temporary
SQLite journals, private TLS/Unix fixtures and static tooling checks. It may not
mint or paste App/registration/JIT credentials, touch Keychain or launchd identity,
alter the Lima VM, change Docker contexts, register/delete runners or trigger a
real workflow. Sanitize all evidence before committing it.

## Fast verification checklist

Before handing work onward, confirm:

- [ ] `git status` is clean or the changes are on the declared issue branch.
- [ ] The issue Goal was copied exactly into one active goal; no invented budget.
- [ ] Dependencies and Project status were checked live.
- [ ] Implementer matches the current user-selected model/effort (default:
      `Luna max`; #66/#67/#68/#69 are Grok 4.6 xhigh with Agent left unset).
      Historical records and #1/#2/#60 Agent values were not rewritten.
- [ ] A meaningful red case, minimal green fix and relevant boundary tests are
      recorded, with actual commands/results and remaining gaps.
- [ ] No secrets, personal paths, raw SDK errors, live tokens or unreviewed runner
      operations entered files, issues, logs or artifacts.
- [ ] Busy work was never killed and cleanup is ownership-bound.
- [ ] Internal independent Luna max review is recorded. An implementer override
      does not change the reviewer.
- [ ] Codex reviewed the exact current PR head; inline and issue-comment findings
      were read and resolved/rebutted; post-fix review was requested and awaited.
- [ ] Required CI is green for the SHA being merged.
- [ ] Issue, Project, PR, branch and goal states match reality.
