# Ordered implementation backlog

Every original row is a full-scope goal. G01/G02 remain evidence gates; G03 is done. See [execution policy](EXECUTION.md), [approved delivery releases](PLAN.md#approved-delivery-releases) and each GitHub issue for acceptance and TDD details.

**Release placement names which user-visible release needs the complete original scope.** Historical M0–M5 stages, original Goal text, Agent history and native `blockedBy` edges are retained. Relabeling does not make a blocked full-scope issue Ready, does not close a parent when a child lands, and does not make an R2 original-acceptance set independently shippable while it is natively blocked by R3-placed issues.

The live 36-item map is in [ISSUES.md](ISSUES.md). Default implementer is Luna max unless the issue records an explicit current user override; #66 and R1 children #67/#68/#69 are Grok 4.6 xhigh with independent Luna max review.

## R1 critical path

Foreground MVP on this Mac: one org, one private repository, existing Linux-container backend, capacity one, manual App. Daemon/install/service are not implemented and are not R1.

| Key | Issue | Role | Native blockers | Live status |
|---|---|---|---|---|
| G01 | [#1](https://github.com/1XP-AI/gh-runnerd/issues/1) | **Full original** ACK/acquisition/JIT recovery Goal; no subset and no false completion | none | In progress |
| G02-R1 | [#67](https://github.com/1XP-AI/gh-runnerd/issues/67) | R1 subset of G02 #2 (manual single-org credentials). Parent #2 stays R3 In progress | none | Ready |
| G01g | [#60](https://github.com/1XP-AI/gh-runnerd/issues/60) | Bounded broker handoff for the paired Linux-container path | #54 (Done) | In progress |
| P66 | [#66](https://github.com/1XP-AI/gh-runnerd/issues/66) | Delivery-plan documentation | none | In progress |
| G13-R1 | [#68](https://github.com/1XP-AI/gh-runnerd/issues/68) | R1 subset of G13 #13. Native blockers #60, #66, #67. Until full G01 #1 and G02 #2 pass, authorized work is a reviewed minimal contract and offline evidence only; production implementation starts only then. Not a G01/G02/G04 bypass. Completing #67 does not complete G02. Live recovery stays on #1→#69 | #60, #66, #67 | Blocked |
| G16-R1 | [#69](https://github.com/1XP-AI/gh-runnerd/issues/69) | R1 subset of G16 #16: authorized real job plus required recovery | #1, #68 | Blocked |

#68 must not start production implementation while natively blocked or while full G01 #1 or G02 #2 remain open. Native `blockedBy` stays #60/#66/#67; do not add #1/#2 without an explicit ask. Completing a child does not complete G02, G04–G15 or G16.

## Original full-scope backlog (M0–M5 preserved)

| Key | Goal area | Stage | Release (full original) | Primary agent | Depends on |
|---|---|---|---|---|---|
| G01 | Prove Scale Set delivery, acquisition and drain contracts | M0 - Evidence gates | R1 | Luna max | None — originally Ready; live In progress |
| G02 | Prove local App enrollment and launchd credential access | M0 - Evidence gates | R3 | Luna max | None — originally Ready; live In progress. R1 subset is #67 |
| G03 | Bootstrap Go module and independent public CI | M0 - Evidence gates | R1 | Luna max | None — Done |
| G04 | Freeze CLI configuration, provider and scaling contracts | M0 - Evidence gates | R2 | Luna max | G01, G02, G03 (unchanged; not bypassed by #68) |
| G05 | Implement durable state and operation journal | M1 - Control plane | R2 | Luna max | G03, G04 |
| G06 | Implement the supervised daemon and authorized local IPC | M1 - Control plane | R2 | Luna max | G05. Not in R1; not implemented |
| G07 | Implement GitHub App credential and installation binding | M1 - Control plane | R2 | Luna max | G02, G05 |
| G08 | Implement guided init and optional Manifest enrollment | M1 - Control plane | R3 | Luna max | G06, G07 |
| G09 | Implement the recoverable Scale Set adapter | M1 - Control plane | R2 | Luna max | G01, G05, G07 |
| G10 | Implement disposable Linux workers with isolated Docker services | M2 - Execution and scaling | R2 | Luna max | G04, G05, G07. R1 reuses the existing paired Linux-container path; full G10 remains |
| G11 | Implement trusted native macOS worker identity and lifecycle | M2 - Execution and scaling | R3 | Luna max | G02, G04, G05, G07. Not in R1 |
| G12 | Implement deterministic shared capacity and fair scheduling | M2 - Execution and scaling | R2 | Luna max | G04, G05 |
| G12a | Pure scaling targets and capacity arithmetic (child of G12) | M2 - Execution and scaling | R2 | Luna max | G03 + reviewed numeric contract; Done |
| G13 | Integrate reconciliation, drain and safe restart | M2 - Execution and scaling | R2 | Luna max | G06, G09, G10, G11, G12 (unchanged). R1 subset is #68 |
| G14 | Implement status, logs and sanitized diagnostics | M3 - Reliability qualification | R2 | Luna max | G06, G13 |
| G15 | Implement startup, shutdown and safe configuration updates | M3 - Reliability qualification | R2 | Luna max | G02, G06, G13 |
| G16 | Build repeatable two-organization runtime qualification | M3 - Reliability qualification | R3 | Luna max | G08, G13, G14, G15. R1 subset is #69 |
| G17 | Qualify crash recovery, soak and resource budgets | M3 - Reliability qualification | R3 | Luna max | G16, G19 |
| G18 | Package signed releases and reproducible distribution | M4 - Pilot and release | R3 | Luna max | G03, G15, G17 |
| G19 | Audit trust admission and cross-worker secret boundaries | M3 - Reliability qualification | R3 | Luna max | G08, G10, G11, G13 |
| G20 | Pilot migration with reversible legacy runner handoff | M4 - Pilot and release | R2 | Luna max | G17, G18 (R3). Full original R2 placement cannot close until those R3 blockers finish; no bypass |
| G21 | Evaluate optional macOS VM and multi-host providers | M5 - Future | Future | Luna max | G17 |

The machine-readable source is [backlog.json](backlog.json). Published issue numbers and links are in [ISSUES.md](ISSUES.md); all original dependencies remain native GitHub blocking relationships. Full-scope first-release implementation goals except G03/G12a remain open.

## Parallel work boundaries

- Continue G01 (full recovery Goal) and G02 (full Manifest/multi-org/launchd Goal) independently; they are not replaced by #67.
- #67 may proceed as isolated R1 credential evidence without waiting on G02's remaining R3 criteria; it does not authorize live App/Keychain/launchd mutation.
- G04 still integrates the full gates before shared full-scope implementation. #68 is not a G04 bypass: until full G01 #1 and G02 #2 pass, authorized work is a reviewed R1 contract and offline evidence only; production implementation starts only then. Completing #67 does not complete G02. Live G01 recovery remains #69 blocked by #1.
- After G05, IPC, auth, and pure scheduling can proceed with separate file ownership for R2/R3 work.
- Linux and native macOS providers can proceed independently once shared contracts/credentials exist; native macOS is R3.
- Integration, security verdict, soak and rollout remain sequential evidence gates.
- Do not edit #60/#62 or #59 runtime/CI/evidence surfaces from the #66 documentation branch.

## Independent G12 arithmetic slice

[G12a #40](https://github.com/1XP-AI/gh-runnerd/issues/40) extracts only the
already specified numeric rules from G12 after the independent Astra contract
review. It depends on completed G03 and its issue's numeric contract. It can
proceed alongside G01/G02 because it has no GitHub, credential, provider,
configuration, persistence or worker-operation dependency. G04's shared
contracts, G05's durable reservations and the remaining G12/G13 work retain
their original gates. The parent remains open after this child is complete.
