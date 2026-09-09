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

The original plan's 48 dependency edges are recorded as native GitHub blocking relationships and linked in issue bodies. Start G01/G02/G03; evidence gates control dependent full-scope work.

This table intentionally preserves the initial planning inventory. Current dispatch is recorded in the live Project Agent field and the open issue execution contracts. The repository default is Luna max; #66 and R1 children #67/#68/#69 record main author Grok 4.6 xhigh with independent Luna max review. Do not rewrite historical Astra records.

Each original issue has one Goal statement, scope, TDD cases, acceptance criteria, model/effort, risk and dependencies. See [backlog.json](backlog.json) for machine-readable specifications. Release placement is additive; see [approved delivery releases](PLAN.md#approved-delivery-releases).

## Live 36-item release map

Verified 2026-09-08 against Project #2 (`36` items) and native GitHub parent/`blockedBy` relations. **Release** is the stage that needs the complete original scope; it is not a closed DAG that can ship before natively blocking issues on later releases. **R1 subset** is an explicit child when the parent mixes MVP and later acceptance. Children do not close parents. Native edges below were not added by this documentation change.

| Issue | Live status | Release (full original) | R1 subset / notes | Live Agent |
|---|---|---|---|---|
| [#1](https://github.com/1XP-AI/gh-runnerd/issues/1) G01 | In progress | R1 | Full original Goal retained; children #44/#46/#47/#50/#52/#54 are Done slices and do not complete G01 | Luna max |
| [#2](https://github.com/1XP-AI/gh-runnerd/issues/2) G02 | In progress | R3 | Broad Manifest/multi-org/launchd remains here. R1 subset is #67 | Luna max |
| [#3](https://github.com/1XP-AI/gh-runnerd/issues/3) G03 | Done | R1 | Full original bootstrap | Luna max |
| [#4](https://github.com/1XP-AI/gh-runnerd/issues/4) G04 | Backlog | R2 | Full contracts. Still blocked by #1/#2/#3. Not bypassed by #68 | Luna max |
| [#5](https://github.com/1XP-AI/gh-runnerd/issues/5) G05 | Backlog | R2 | Full durable journal | Luna max |
| [#6](https://github.com/1XP-AI/gh-runnerd/issues/6) G06 | Backlog | R2 | Supervised daemon/IPC; not implemented; not R1 | Luna max |
| [#7](https://github.com/1XP-AI/gh-runnerd/issues/7) G07 | Backlog | R2 | Full App credential binding | Luna max |
| [#8](https://github.com/1XP-AI/gh-runnerd/issues/8) G08 | Backlog | R3 | Guided init / Manifest enrollment | Luna max |
| [#9](https://github.com/1XP-AI/gh-runnerd/issues/9) G09 | Backlog | R2 | Full recoverable Scale Set adapter | Luna max |
| [#10](https://github.com/1XP-AI/gh-runnerd/issues/10) G10 | Backlog | R2 | Full Linux Docker workers. R1 reuses the existing paired Linux-container path | Luna max |
| [#11](https://github.com/1XP-AI/gh-runnerd/issues/11) G11 | Backlog | R3 | Trusted native macOS backend; not R1 | Luna max |
| [#12](https://github.com/1XP-AI/gh-runnerd/issues/12) G12 | Backlog | R2 | Full shared capacity/fairness | Luna max |
| [#13](https://github.com/1XP-AI/gh-runnerd/issues/13) G13 | Backlog | R2 | Full reconcile/drain/restart. R1 subset is #68. Original blockers including #11 remain | Luna max |
| [#14](https://github.com/1XP-AI/gh-runnerd/issues/14) G14 | Backlog | R2 | Full status/logs | Luna max |
| [#15](https://github.com/1XP-AI/gh-runnerd/issues/15) G15 | Backlog | R2 | Full service lifecycle | Luna max |
| [#16](https://github.com/1XP-AI/gh-runnerd/issues/16) G16 | Backlog | R3 | Full two-organization qualification. R1 subset is #69 | Luna max |
| [#17](https://github.com/1XP-AI/gh-runnerd/issues/17) G17 | Backlog | R3 | Soak/recovery qualification | Luna max |
| [#18](https://github.com/1XP-AI/gh-runnerd/issues/18) G18 | Backlog | R3 | Signed distribution | Luna max |
| [#19](https://github.com/1XP-AI/gh-runnerd/issues/19) G19 | Backlog | R3 | Trust admission audit | Luna max |
| [#20](https://github.com/1XP-AI/gh-runnerd/issues/20) G20 | Backlog | R2 | Full original placement R2; still natively blocked by #17/#18. No bypass | Luna max |
| [#21](https://github.com/1XP-AI/gh-runnerd/issues/21) G21 | Future | Future | Optional VM/multi-host research | Luna max |
| [#30](https://github.com/1XP-AI/gh-runnerd/issues/30) | Done | R1 | Merged-PR audit | unset |
| [#40](https://github.com/1XP-AI/gh-runnerd/issues/40) G12a | Done | R2 | Child of #12; numeric arithmetic only | Luna max |
| [#44](https://github.com/1XP-AI/gh-runnerd/issues/44) G01a | Done | R1 | Child of #1 | Astra xhigh |
| [#46](https://github.com/1XP-AI/gh-runnerd/issues/46) G01b | Done | R1 | Child of #1 | Astra xhigh |
| [#47](https://github.com/1XP-AI/gh-runnerd/issues/47) G01c | Done | R1 | Child of #1 | Astra xhigh |
| [#50](https://github.com/1XP-AI/gh-runnerd/issues/50) G01d | Done | R1 | Child of #1 | Astra xhigh |
| [#52](https://github.com/1XP-AI/gh-runnerd/issues/52) G01e | Done | R1 | Child of #1 | Astra xhigh |
| [#54](https://github.com/1XP-AI/gh-runnerd/issues/54) G01f | Done | R1 | Child of #1; merged paired execution | Luna max |
| [#60](https://github.com/1XP-AI/gh-runnerd/issues/60) G01g | In progress | R1 | Native parent unset; `blockedBy` #54; blocks #68. Do not retarget from this docs change | Luna max |
| [#61](https://github.com/1XP-AI/gh-runnerd/issues/61) | Done | R1 | CI paired-fixture deadline | Luna max |
| [#64](https://github.com/1XP-AI/gh-runnerd/issues/64) | Done | R1 | CI default G01 deadline coverage | Luna max |
| [#66](https://github.com/1XP-AI/gh-runnerd/issues/66) | In progress | R1 | Planning only. Main author Grok 4.6 xhigh; independent Luna max review | unset |
| [#67](https://github.com/1XP-AI/gh-runnerd/issues/67) | Ready | R1 | Child of #2. Manual single-org credentials. No native blockers | unset |
| [#68](https://github.com/1XP-AI/gh-runnerd/issues/68) | Blocked | R1 | Child of #13. Native `blockedBy` #60/#66/#67. Contract/offline evidence only until full G01 #1 and G02 #2 pass; production implementation only then. Not a G01/G02/G04 bypass. Completing #67 does not complete G02. Live recovery is #69←#1 | unset |
| [#69](https://github.com/1XP-AI/gh-runnerd/issues/69) | Blocked | R1 | Child of #16. `blockedBy` #1/#68 | unset |

### Verified native relations (do not duplicate)

| Issue | Parent | `blockedBy` | Blocks |
|---|---|---|---|
| #67 | #2 | none | #68 |
| #68 | #13 | #60, #66, #67 | #69 |
| #69 | #16 | #1, #68 | none |
| #66 | none | none | #68 |
| #60 | none | #54 | #68 |
| #40 | #12 | #3 | none |
| #44/#46/#47/#50/#52/#54 | #1 | none for these children | #54 blocks #60/#61 |

Original G04 `blockedBy` #1/#2/#3 and original G13 `blockedBy` #6/#9/#10/#11/#12 are unchanged. Further new issues or edge edits need an explicit ask; the mapping above is already applied.

## Project views

- [Goals](https://github.com/orgs/1XP-AI/projects/2/views/1): every issue with status, stage, model, Goal, risk, dependencies, test profiles and Release.
- [Ready](https://github.com/orgs/1XP-AI/projects/2/views/2): dispatch only issues currently marked Ready in the live Project. As of this snapshot that includes #67; it does not include #68 or #69.
- [Board](https://github.com/orgs/1XP-AI/projects/2/views/3): status columns with stage, priority, model and Release on cards.

The initial board contained 4 Ready, 17 Backlog and 1 Future issue, including the independent G12a child. The live board now has 36 items after additive Release labels and R1 children. Consult the live Project for current status. Move later full-scope issues to Ready only when their dependencies are Done. Model metadata is a dispatch instruction, not an automatic agent scheduler.

## Parallel arithmetic child

Issue [#40](https://github.com/1XP-AI/gh-runnerd/issues/40) is a native sub-issue
of G12 #12 with a native dependency on completed G03 #3. Its independently
reviewed numeric contract permits isolated target/resource arithmetic now;
existing parent dependencies and live evidence gates remain in place. The live
Project records G12a as Done. See the child issue for exact scope and
acceptance; this addition does not mark G12 or its integration complete.
