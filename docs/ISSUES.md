# Published issue goals

[Project](https://github.com/orgs/1XP-AI/projects/2) · [Ready work](https://github.com/orgs/1XP-AI/projects/2/views/2) · [Repository](https://github.com/1XP-AI/gh-runnerd) · [Architecture plan](PLAN.md) · [Execution policy](EXECUTION.md)

These are issue goals. The table records the initial planning inventory; the live GitHub issue/Project is authoritative after work begins. No future goal is automatically dispatched.

| Goal | Issue | Primary agent | Initial state | Test profiles |
|---|---|---|---|---|
| G01 | [#1 Prove Scale Set delivery, acquisition and drain contracts](https://github.com/1XP-AI/gh-runnerd/issues/1) | Astra xhigh | Ready | offline, trusted-live-github |
| G02 | [#2 Prove local App enrollment and launchd credential access](https://github.com/1XP-AI/gh-runnerd/issues/2) | Astra xhigh | Ready | offline, trusted-runtime, trusted-live-github |
| G03 | [#3 Bootstrap Go module and independent public CI](https://github.com/1XP-AI/gh-runnerd/issues/3) | Luna max | Ready | offline |
| G04 | [#4 Freeze CLI configuration, provider and scaling contracts](https://github.com/1XP-AI/gh-runnerd/issues/4) | Astra xhigh | Backlog | offline |
| G05 | [#5 Implement durable state and operation journal](https://github.com/1XP-AI/gh-runnerd/issues/5) | Astra xhigh | Backlog | offline |
| G06 | [#6 Implement the supervised daemon and authorized local IPC](https://github.com/1XP-AI/gh-runnerd/issues/6) | Astra xhigh | Backlog | offline, trusted-runtime |
| G07 | [#7 Implement GitHub App credential and installation binding](https://github.com/1XP-AI/gh-runnerd/issues/7) | Astra xhigh | Backlog | offline, trusted-runtime, trusted-live-github |
| G08 | [#8 Implement guided init and optional Manifest enrollment](https://github.com/1XP-AI/gh-runnerd/issues/8) | Astra xhigh | Backlog | offline, trusted-live-github |
| G09 | [#9 Implement the recoverable Scale Set adapter](https://github.com/1XP-AI/gh-runnerd/issues/9) | Astra xhigh | Backlog | offline, trusted-live-github |
| G10 | [#10 Implement disposable Linux workers with isolated Docker services](https://github.com/1XP-AI/gh-runnerd/issues/10) | Astra xhigh | Backlog | offline, trusted-runtime, trusted-live-github |
| G11 | [#11 Implement trusted native macOS worker identity and lifecycle](https://github.com/1XP-AI/gh-runnerd/issues/11) | Astra xhigh | Backlog | offline, trusted-runtime, trusted-live-github |
| G12 | [#12 Implement deterministic shared capacity and fair scheduling](https://github.com/1XP-AI/gh-runnerd/issues/12) | Luna max | Backlog | offline |
| G12a | [#40 Implement pure scaling targets and capacity arithmetic](https://github.com/1XP-AI/gh-runnerd/issues/40) | Luna max | Ready (independent arithmetic child) | offline |
| G13 | [#13 Integrate reconciliation, drain and safe restart](https://github.com/1XP-AI/gh-runnerd/issues/13) | Astra xhigh | Backlog | offline, trusted-runtime |
| G14 | [#14 Implement status, logs and sanitized diagnostics](https://github.com/1XP-AI/gh-runnerd/issues/14) | Luna max | Backlog | offline, trusted-runtime |
| G15 | [#15 Implement startup, shutdown and safe configuration updates](https://github.com/1XP-AI/gh-runnerd/issues/15) | Astra xhigh | Backlog | offline, trusted-runtime |
| G16 | [#16 Build repeatable two-organization runtime qualification](https://github.com/1XP-AI/gh-runnerd/issues/16) | Luna max | Backlog | offline, trusted-runtime, trusted-live-github |
| G17 | [#17 Qualify crash recovery, soak and resource budgets](https://github.com/1XP-AI/gh-runnerd/issues/17) | Astra xhigh | Backlog | offline, trusted-runtime, trusted-live-github |
| G18 | [#18 Package signed releases and reproducible distribution](https://github.com/1XP-AI/gh-runnerd/issues/18) | Luna max | Backlog | offline, trusted-runtime |
| G19 | [#19 Audit trust admission and cross-worker secret boundaries](https://github.com/1XP-AI/gh-runnerd/issues/19) | Astra xhigh | Backlog | offline, trusted-runtime, trusted-live-github |
| G20 | [#20 Pilot migration with reversible legacy runner handoff](https://github.com/1XP-AI/gh-runnerd/issues/20) | Astra xhigh | Backlog | offline, trusted-runtime, trusted-live-github |
| G21 | [#21 Evaluate optional macOS VM and multi-host providers](https://github.com/1XP-AI/gh-runnerd/issues/21) | Astra xhigh | Future | offline |

The original plan's 48 dependency edges are recorded as native GitHub blocking relationships and linked in issue bodies. Start G01/G02/G03; evidence gates control dependent work.

This table intentionally preserves the initial planning inventory. Current dispatch is recorded in the live Project Agent field and the open issue execution contracts; all future dispatches use Luna max.

Each issue has one Goal statement, scope, TDD cases, acceptance criteria, model/effort, risk and dependencies. See [backlog.json](backlog.json) for machine-readable specifications.

## Project views

- [Goals](https://github.com/orgs/1XP-AI/projects/2/views/1): every issue with status, stage, model, Goal, risk, dependencies and test profiles.
- [Ready](https://github.com/orgs/1XP-AI/projects/2/views/2): only work whose initial dependencies are clear; its initial entries were G01/G02/G03 and independent G12a. Dispatch only issues currently marked Ready in the live Project.
- [Board](https://github.com/orgs/1XP-AI/projects/2/views/3): status columns with stage, priority and model on cards.

The initial board contains 4 Ready, 17 Backlog and 1 Future issue, including the independent G12a child. These are initial planning counts; consult the live Project for current status. Move later issues to Ready only when their dependencies are Done. Model metadata is a dispatch instruction, not an automatic agent scheduler.

## Parallel arithmetic child

Issue [#40](https://github.com/1XP-AI/gh-runnerd/issues/40) is a native sub-issue
of G12 #12 with a native dependency on completed G03 #3. Its independently
reviewed numeric contract permits isolated target/resource arithmetic now;
existing parent dependencies and live evidence gates remain in place. The live
Project records its current progress. See the child issue for exact scope and
acceptance; this addition does not mark G12 or its integration complete.
