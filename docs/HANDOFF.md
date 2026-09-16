# gh-runnerd agent handoff

Latest continuation checkpoint: [2026-09-12](handoffs/2026-09-12.md).
Read that checkpoint before the older snapshot below; recheck live GitHub state
before acting on either document.

This is the operational handoff for an agent continuing the `1XP-AI/gh-runnerd`
project. It combines the repository rules, the GitHub Project control loop, the
runner pilot context, the current gates and the exact review/merge discipline.
Treat the live GitHub Project and the issue body as authoritative when they differ
from this snapshot. Re-read this file, `AGENTS.md` and the linked design documents
before changing code.

Historical baseline snapshot: 2026-09-08 KST, documentation branch `orca/release-reframe` based on `main` `31ae8102f6f20f8e79258eb824af1400eba21954` (`[CI] Split default G01 deadline checks (#65)`). That baseline had 36 Project items after the maintainer-accepted release reorganization ([#66](https://github.com/1XP-AI/gh-runnerd/issues/66)). The current read-only Project snapshot is 38 items; the live Project and issue bodies win when they differ from either snapshot.

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
  Full G02 #2 remains a mandatory pre-release evidence gate for the R1 command
  path and stays classified R3; R1 is not independently deliverable until that
  gate passes. Completing #67 does not complete G02. Daemon/install/launchd
  service are not implemented.
- **R2 Everyday operations:** install/start/stop/status, restart recovery and
  bounded scaling.
- **R3 General distribution:** multi-organization support, trusted native macOS
  backend, automated onboarding, signing/update/diagnostics. Full G02 original
  acceptance remains here and is also a pre-release gate for R1 #68 production.
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
research in G21. Issue #1's current active Goal is the optimized statement in
the [G01 Goal synchronization](#g01-goal-synchronization) section below. The
pre-optimization wording remains only as explicitly labeled historical
provenance; it is not the objective for a resumed active goal.

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
| Route `gpt-luna-max` → Agent: Luna max | `9317c27f` (repository default dispatch) |
| Route `grok-high` → Agent: Grok high | `ab0d6d0d` (explicit override only) |
| Agent: Astra xhigh | `cf617d72` (historical only; never selected for new work) |
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
New work uses only `gpt-luna-max` or `grok-high`. The Project Agent field must
provide `Luna max` and `Grok high`; the Astra option remains only for historical
cards. Verify the live option ID before editing a Ready item. Do not overwrite a
currently owned In progress item during routing changes.

### Live board snapshot

Run `gh project item-list 2 --owner 1XP-AI --limit 1000 --format json` before
dispatch. The latest read-only snapshot was verified 2026-09-13 KST and has 38
items. The full map is in
[ISSUES.md](ISSUES.md). Compact routing:

| Issues | Project status | Release | Current routing |
|---|---|---|---|
| #1 G01 | In progress | R1 | Luna max; **full** ACK/acquisition/JIT Goal unresolved |
| #2 G02 | In progress | R3 | Luna max; Manifest/multi-org/launchd remains here; also pre-release gate for R1 #68 production |
| #67 | Ready | R1 | Project Agent `Grok high` (`grok-high`); independent `gpt-luna-max` review; child of #2; no native blockers |
| #60 G01g | Done | R1 | Luna max; merged PR #62; its evidence is a slice and does not close G01 |
| #71 G01h | In progress | R1 | Luna max; experiment-only idle-drain observation under the single current G01 Goal; do not create a second goal |
| #66 | Done | R1 | Planning-only release map; historical `grok-high` / `gpt-luna-max` review |
| #73 | Done | — | Luna max; workflow validation/process improvement merged as PR #74 |
| #68 | Blocked | R1 | child of #13; native blockedBy #60/#66/#67; contract/offline evidence only until full #1/#2; production implementation only then |
| #69 | Blocked | R1 | child of #16; blocked by #1/#68 |
| #3 G03, #30, #40 G12a, #44, #46, #47, #50, #52, #54 G01f, #61, #64 | Done | R1 except #40 R2 | historical records preserved; #54 Done does not close #1 |
| #4–#7, #9, #10, #12–#15, #20 | Backlog | R2 | Luna max; original dependencies unchanged |
| #8, #11, #16–#19 | Backlog | R3 | Luna max |
| #21 G21 | Future | Future | optional macOS VM/multi-host research |

Issue labels and the initial planning inventory can retain historical Astra values.
For current dispatch, use the Project `Agent` field and the issue's current
execution contract. Original `backlog.json` `status`/`agent` values are the same
class of historical snapshot; do not redispatch from historical Ready. Do not
rewrite historical records to make them look like new work. Do not add further
child issues or native edges without an explicit ask; the R1 mapping is already
applied.

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

## G01 Goal synchronization

The current active Goal for issue [#1](https://github.com/1XP-AI/gh-runnerd/issues/1)
and its one parent active goal is:

> Select and pin one supported Scale Set integration path and produce a reusable evidence packet for recovery at message acknowledgement, acquisition and JIT boundaries, rerunning only changed-boundary checks while keeping unchanged evidence and live gaps explicit.

The pre-optimization wording below is retained for historical provenance only;
operators must not copy it into a new or resumed active goal:

> Select and pin a supported Scale Set integration path with demonstrated recovery at message acknowledgement, acquisition and JIT boundaries.

Child [#71](https://github.com/1XP-AI/gh-runnerd/issues/71) is bounded
experiment-only work under this parent Goal. It does not replace the parent
Goal, close the G01 evidence gate, or authorize a second active goal.

## Model and delegation policy

The repository default routes **implementation, contract work, architecture,
authentication, protocol, concurrency, documentation, coordination and independent
review through `gpt-luna-max`** (`gpt-5.6-luna` with `max` reasoning). An explicit
current user override may select **`grok-high`** (Grok 4.6 with `xhigh` reasoning).
These are the only routes for new work. Historical Astra/Luna assignments in old
review and evidence records are facts about who did that work and must stay
unchanged.

The current documented override for [#66](https://github.com/1XP-AI/gh-runnerd/issues/66)
and R1 children [#67](https://github.com/1XP-AI/gh-runnerd/issues/67)/[#68](https://github.com/1XP-AI/gh-runnerd/issues/68)/[#69](https://github.com/1XP-AI/gh-runnerd/issues/69)
is `grok-high` with independent `gpt-luna-max` review. Do not create a second
native Goal on #1 while this planning work runs.

For every new issue, after checking for an explicit current user override:

- set `IMPLEMENTER_OVERRIDE` to the inspected canonical route key (`gpt-luna-max`
  or `grok-high`), or to an empty string when there is no override (repository
  default `gpt-luna-max`);
- normalize that key to its Project Agent display label (`Luna max` or `Grok
  high`), then look up that label in live `gh project field-list` options by exact
  match;
- when the normalized display label matches one option, write that option unless the item is not Ready
  or already has a conflicting Agent;
- when an override is `grok-high`, require the live `Grok high` Project option;
  missing allowed options are a routing configuration error, not a reason to
  substitute Astra or silently write Luna. Independent review remains
  `gpt-luna-max`. Do not keep a permanent issue-number whitelist;
- use one active goal whose objective is exactly the issue's `Goal` statement;
- do not invent a token budget;
- use an independent contract review with the current selected review model/effort
  (the default is Luna max), plus a second independent Luna max pass for security,
  recovery, secret handling, ownership or concurrency boundaries;
- parallelize only independent issues with explicit non-overlapping file ownership;
  one integrator owns shared protocol/state definitions;
- a Project field never launches work by itself.

## Incremental validation and review handoff

Use focused validation while the candidate is changing and the complete gate once
the candidate is stable. Keep intermediate commits local. The writer owns
meaningful red -> minimal green -> refactor evidence and focused unit/negative
checks; an explicit `make fast` module/package/test selector may help with local
iteration, but it is fail-closed and never represents `make check` or CI success.
Push one batched stable candidate, then run the hosted full matrix once. After a
review fix, batch all actionable fixes before the next candidate push. Do not
repeat a full suite for an unchanged SHA, unchanged risk boundary or already
conclusive result.

| Role | Handoff contract |
|---|---|
| Writer | Batch all source, documentation and finding-ledger changes before one candidate push; include exact commands/results and `git diff --check`. |
| Independent reviewers | Inspect the immutable candidate and shared exact-source hosted CI evidence; run delta/risk probes only. Add the independent Luna max security/recovery pass for applicable boundaries. |
| Coordinator | Audit the issue contract, changed surface, ledger and exact-head evidence; coordinate fixes. Do not act as a third full-suite tester. |
| CI / merge | Hosted PR CI is the complete premerge source gate. GitHub Codex must review the exact final HEAD, including stale/outdated findings, and required CI must pass for that SHA. |
| Main / release | `main` CI is postmerge integration evidence. Release, macOS, soak and trusted/live checks are required only for their applicable qualification, on a reviewed immutable commit with explicit maintainer authorization; no mandatory security gate is deferred. |

Carry resolved findings into each candidate ledger with the original finding URL,
immutable source SHA and resolution evidence, then record final delta sign-off for
the exact candidate SHA. New internal full review is limited to a changed risk or
interface boundary; ordinary fixes receive delta review. After a fix push, obtain
fresh exact-head Codex review and CI; do not request repetitive Codex reviews
mid-edit. The full candidate gate remains separate from internal review, and the
postmerge `main` run never substitutes for premerge evidence.

Suggested handoff prompt to give the next agent:

> Work on ISSUE_URL in `1XP-AI/gh-runnerd`. Use `gpt-luna-max` by default or the issue's explicit `grok-high` override, and one active goal exactly equal to the issue's Goal statement; do not invent a token budget. Independent review uses `gpt-luna-max` unless the user explicitly selects another allowed route. Read `AGENTS.md`, `docs/EXECUTION.md`, the plan, the linked ADRs and the current Project item. Verify dependencies first. Create the goal and isolated branch/worktree, then set the item to In progress. Follow meaningful red test -> minimal green implementation -> refactor -> boundary/failure tests. Keep intermediate commits local and push one batched candidate; do not run the full suite or request Codex review for every commit. Preserve no-secrets, no-busy-kill, owned-cleanup, stable-idempotency and trusted-native invariants. Do not change live runners, Docker context, App/Keychain/launchd state or GitHub credentials without explicit maintainer authorization. Open one focused PR with exact commands/results, red evidence, gaps and rollback notes. Obtain independent `gpt-luna-max` review, then the exact-head GitHub Codex review before merge. Update the Project, issue and goal only when their actual state changes. A missing allowed Project Agent option is a routing failure; do not substitute Astra or silently fall back.

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
that new scope. Record the inspected current canonical route key as
`IMPLEMENTER_OVERRIDE` (empty string when there is none). Do not infer it from
the issue number and do not scrape issue prose automatically into the Agent write.

### 2. Resolve the Project item and route the Agent field

```sh
PROJECT_ID=PVT_kwDOD2M2gs4Bismw
STATUS_FIELD_ID=PVTSSF_lADOD2M2gs4BismwzhhjoZA
AGENT_FIELD_ID=PVTSSF_lADOD2M2gs4Bismwzhhjobs
STATUS_IN_PROGRESS_ID=7a569f61
DEFAULT_ROUTE_KEY="gpt-luna-max"

# Operator-inspected current contract. Must be set: empty string means no
# override (repository default gpt-luna-max). Unset is fail-closed. Canonical
# route keys are normalized to Project display labels below; only the two
# allowlisted keys may select new work. Do not branch on issue numbers.
: "${IMPLEMENTER_OVERRIDE?set IMPLEMENTER_OVERRIDE after inspecting the current issue override; empty string means no override}"

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

normalize_route_key() {
  case "$1" in
    gpt-luna-max|grok-high) printf '%s\n' "$1" ;;
    *) return 1 ;;
  esac
}

route_project_label() {
  case "$1" in
    gpt-luna-max) printf '%s\n' 'Luna max' ;;
    grok-high) printf '%s\n' 'Grok high' ;;
    *) return 1 ;;
  esac
}

ROUTE_INPUT="${IMPLEMENTER_OVERRIDE:-$DEFAULT_ROUTE_KEY}"
if ! ROUTE_KEY="$(normalize_route_key "$ROUTE_INPUT")"; then
  printf 'route %s is not an allowed canonical new-work route; fail closed\n' \
    "$ROUTE_INPUT" >&2
  exit 1
fi
if ! INTENDED_IMPLEMENTER="$(route_project_label "$ROUTE_KEY")"; then
  printf 'route %s has no Project Agent display label; fail closed\n' \
    "$ROUTE_KEY" >&2
  exit 1
fi

AGENT_OPTIONS_JSON="$(gh project field-list "$PROJECT_NUMBER" --owner "$OWNER" --format json)"
if ! AGENT_OPTION_ID="$(printf '%s' "$AGENT_OPTIONS_JSON" | jq -r --arg name "$INTENDED_IMPLEMENTER" --arg fid "$AGENT_FIELD_ID" '
  [.fields[] | select(.id == $fid) | .options[]? | select(.name == $name) | .id]
  | if length == 1 then .[0]
    elif length == 0 then empty
    else error("ambiguous Agent option name")
    end')"; then
  printf 'Agent option lookup failed; fail closed\n' >&2
  exit 1
fi

if [ -z "$AGENT_OPTION_ID" ]; then
  printf 'route %s requires Project Agent option %s; fail closed\n' \
    "$ROUTE_KEY" "$INTENDED_IMPLEMENTER" >&2
  exit 1
fi

if [ "$ITEM_AGENT" != "(unset)" ] && [ "$ITEM_AGENT" != "$INTENDED_IMPLEMENTER" ]; then
  printf 'selected issue already has Agent %s; will not overwrite with %s\n' \
    "$ITEM_AGENT" "$INTENDED_IMPLEMENTER" >&2
  exit 1
fi

gh project item-edit --id "$ITEM_ID" --project-id "$PROJECT_ID" \
  --field-id "$AGENT_FIELD_ID" --single-select-option-id "$AGENT_OPTION_ID"
```

If the item is already In progress or otherwise not Ready, the guard above stops
before any field edit; coordinate instead of starting a second implementation or
overwriting a currently owned Agent. Keep the issue's durable Goal/Dependencies
fields and `docs/backlog.json` aligned only when the contract actually changes.
Do not rewrite original JSON `status`/`agent` snapshots to look live; the Project
is dispatch authority. The script does not branch on issue numbers.
`IMPLEMENTER_OVERRIDE` is the operator-inspected canonical route key, not an LLM
scrape and not a permanent whitelist. The resolver allowlists only
`gpt-luna-max` and `grok-high`, normalizes them to the Project display labels
`Luna max` and `Grok high`, and then obtains the option from
`gh project field-list`. Exact match writes; a missing allowed option fails
closed; non-Ready and conflicting Agent fail closed. Historical `Astra xhigh`
is never a valid new-work key. Independent review uses `gpt-luna-max`.

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

Run focused checks appropriate to the changed surface while editing, then batch
source, docs and finding-ledger changes into one stable review candidate. Public
CI must not require live App credentials, local runner access or the unreleased
manager. The hosted PR workflow supplies the complete candidate gate; local
`make check` is available on demand but is not an automatic per-commit
requirement. Trusted hardware/live tests require a reviewed immutable commit and
explicit maintainer-triggered execution on the scoped runner group.

```sh
git diff --check
make check                    # optional local confidence; hosted PR CI is the full candidate gate
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
| G01 | #1 | Current active Goal: pin one supported Scale Set path and produce reusable ACK/acquisition/JIT recovery evidence; the historical pre-optimization wording is provenance only |
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
| G01h | #71 | Experiment-only idle-drain observation under the single current G01 Goal (child of #1; In progress) |
| G13-R1 | #68 | R1 foreground command, capacity one (child of #13; native blockedBy #60/#66/#67; production waits on full G01/G02) |
| G16-R1 | #69 | R1 authorized real job plus required recovery (child of #16; blocked by #1/#68) |
| P66 | #66 | Delivery-plan documentation; not a runtime Goal |

G01 and G02 are evidence gates. G01's exact Goal and full ACK/acquisition/JIT
recovery remain required; child merges are not completion. G04 and all dependent
full-scope implementation remain behind their acceptance evidence. #68 is not a
G04/G13 bypass: until full G01 #1 and G02 #2 pass, its authorized work is
contract and offline evidence only; production implementation starts only then.
Completing #67 does not complete G02. Because full G02 remains classified R3,
R1 is not independently deliverable until that gate passes. G10/G16 require real ARM64 Docker and private test repositories.
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

- [#60](https://github.com/1XP-AI/gh-runnerd/issues/60) G01g / merged PR [#62](https://github.com/1XP-AI/gh-runnerd/pull/62): bounded broker handoff. Its completion does not close G01 or authorize dependent production.
- [#2](https://github.com/1XP-AI/gh-runnerd/issues/2) readiness review / open PR [#59](https://github.com/1XP-AI/gh-runnerd/pull/59): G02 macOS live-validation packet. Do not treat it as R1 completion of #67.

Inspect those PRs in their own worktrees. This documentation change must not
modify runtime, CI or evidence docs belonging to those PRs.

### This planning change

Issue [#66](https://github.com/1XP-AI/gh-runnerd/issues/66) on `orca/release-reframe`
is a completed historical planning record. Its author route was Grok 4.6 xhigh;
independent Luna max review and exact-head Codex+CI preceded merge. #68 stays natively
Blocked until #60, #66 and #67 are complete. After that,
authorized work is the reviewed minimal contract and offline evidence only until
full G01 #1 and G02 #2 pass; production implementation starts only then.
Completing #67 does not complete G02. Because full G02 remains classified R3,
R1 is not independently deliverable until that gate passes. No live authorization
and no merge of blocked implementation are implied.

## PR #76 documentation finding ledger

This focused correction addresses the three actionable inline findings on review
commit `3901c9f8ad676a12b8f700ae60c242901111e0fd`:

- [route-key normalization](https://github.com/1XP-AI/gh-runnerd/pull/76#discussion_r3996581696): the documented resolver now accepts only canonical `gpt-luna-max`/`grok-high` keys, maps them to the exact Project labels `Luna max`/`Grok high`, and rejects Astra for new work.
- [G01 Goal synchronization](https://github.com/1XP-AI/gh-runnerd/pull/76#discussion_r3996581698): the optimized current Goal is repeated in planning docs, while the prior wording is explicitly historical and not an active objective.
- [live map refresh](https://github.com/1XP-AI/gh-runnerd/pull/76#discussion_r3996581700): `ISSUES.md` and this compact map use the read-only 2026-09-13 KST Project snapshot of 38 items, including #71 In progress and #73 Done.

Documentation-only validation run on 2026-09-13 KST (no live App, runner,
workflow, Docker/Lima, Keychain or launchd operation):

- `git diff --check` — PASS (exit 0).
- `jq empty docs/backlog.json` — PASS (exit 0).
- `current_goal="$(jq -r '.issues[] | select(.key == "G01") | .goal' docs/backlog.json)"; for file in docs/HANDOFF.md docs/PLAN.md docs/BACKLOG.md docs/ISSUES.md; do rg -F -q "> $current_goal" "$file"; done` — PASS (current optimized Goal matches all four docs; historical predecessor is explicitly labeled in each).
- `route_functions="$(sed -n '361,374p' docs/HANDOFF.md)"; eval "$route_functions";` canonical-key/display-label assertions against `gh project field-list` plus Astra rejection — PASS (exit 0).
- `project_json="$(gh project item-list 2 --owner 1XP-AI --limit 1000 --format json)";` map-count, status, Agent and Release assertions — PASS (38 Project items, 38 map rows; #71 In progress/Luna max/R1; #73 Done/Luna max/—).
- `p1="$(printf '/'; printf 'Users/')"; p2="$(printf '/'; printf 'private/')"; p3="$(printf '%s' '-----BE' 'GIN')"; p4="$(printf '%s' 'github_' 'pat_')"; p5="$(printf '%s' 'Bear' 'er ')"; git diff --unified=0 -- docs/HANDOFF.md docs/PLAN.md docs/BACKLOG.md docs/ISSUES.md | rg -n -e "$p1" -e "$p2" -e "$p3" -e "$p4" -e "$p5"` — PASS (no matches).
- `p1="$(printf '36'; printf '-item')"; p2="$(printf 'last verified snapshot is '; printf '36')"; p3="$(printf 'live board now has '; printf '36')"; p4="$(printf 'Verified '; printf '2026-09-08 against Project #2')"; rg -n -e "^## Live $p1" -e "$p2" -e "$p3" -e "$p4" docs/HANDOFF.md docs/PLAN.md docs/BACKLOG.md docs/ISSUES.md` — PASS (no unlabelled stale-count matches; historical baseline remains explicitly labeled).

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
- [ ] Implementer matches the inspected current user override (default
      `gpt-luna-max` / `Luna max` if none), and the Project Agent option is one
      of `Luna max` or `Grok high`. Historical records are not rewritten.
- [ ] A meaningful red case, minimal green fix and relevant boundary tests are
      recorded, with actual commands/results and remaining gaps.
- [ ] Source, documentation and finding-ledger changes were batched before the
      candidate push; focused checks are not represented as full-gate evidence,
      and unchanged full runs were not repeated without a recorded reason.
- [ ] Previously resolved findings retain original URLs, immutable source SHAs and
      resolution evidence, with final delta sign-off on the exact candidate SHA.
- [ ] No secrets, personal paths, raw SDK errors, live tokens or unreviewed runner
      operations entered files, issues, logs or artifacts.
- [ ] Busy work was never killed and cleanup is ownership-bound.
- [ ] Internal independent `gpt-luna-max` review is recorded. An implementer override
      does not change the reviewer.
- [ ] Codex reviewed the exact current PR head; inline and issue-comment findings
      were read and resolved/rebutted; post-fix review was requested and awaited.
- [ ] Required CI is green for the SHA being merged.
- [ ] Issue, Project, PR, branch and goal states match reality.
