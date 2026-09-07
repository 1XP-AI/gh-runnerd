# Ordered implementation backlog

Every row is an unimplemented goal. G01/G02 are evidence gates; G03 is independently ready. See [execution policy](EXECUTION.md) and each GitHub issue for acceptance and TDD details.

| Key | Goal area | Stage | Primary agent | Depends on |
|---|---|---|---|---|
| G01 | Prove Scale Set delivery, acquisition and drain contracts | M0 - Evidence gates | Astra xhigh | None — Ready |
| G02 | Prove local App enrollment and launchd credential access | M0 - Evidence gates | Astra xhigh | None — Ready |
| G03 | Bootstrap Go module and independent public CI | M0 - Evidence gates | Luna max | None — Ready |
| G04 | Freeze CLI configuration, provider and scaling contracts | M0 - Evidence gates | Astra xhigh | G01, G02, G03 |
| G05 | Implement durable state and operation journal | M1 - Control plane | Astra xhigh | G03, G04 |
| G06 | Implement the supervised daemon and authorized local IPC | M1 - Control plane | Astra xhigh | G05 |
| G07 | Implement GitHub App credential and installation binding | M1 - Control plane | Astra xhigh | G02, G05 |
| G08 | Implement guided init and optional Manifest enrollment | M1 - Control plane | Luna max | G06, G07 |
| G09 | Implement the recoverable Scale Set adapter | M1 - Control plane | Astra xhigh | G01, G05, G07 |
| G10 | Implement disposable Linux workers with isolated Docker services | M2 - Execution and scaling | Astra xhigh | G04, G05, G07 |
| G11 | Implement trusted native macOS worker identity and lifecycle | M2 - Execution and scaling | Astra xhigh | G02, G04, G05, G07 |
| G12 | Implement deterministic shared capacity and fair scheduling | M2 - Execution and scaling | Luna max | G04, G05 |
| G13 | Integrate reconciliation, drain and safe restart | M2 - Execution and scaling | Astra xhigh | G06, G09, G10, G11, G12 |
| G14 | Implement status, logs and sanitized diagnostics | M3 - Reliability qualification | Luna max | G06, G13 |
| G15 | Implement startup, shutdown and safe configuration updates | M3 - Reliability qualification | Luna max | G02, G06, G13 |
| G16 | Build repeatable two-organization runtime qualification | M3 - Reliability qualification | Luna max | G08, G13, G14, G15 |
| G17 | Qualify crash recovery, soak and resource budgets | M3 - Reliability qualification | Astra xhigh | G16, G19 |
| G18 | Package signed releases and reproducible distribution | M4 - Pilot and release | Luna max | G03, G15, G17 |
| G19 | Audit trust admission and cross-worker secret boundaries | M3 - Reliability qualification | Astra xhigh | G08, G10, G11, G13 |
| G20 | Pilot migration with reversible legacy runner handoff | M4 - Pilot and release | Astra xhigh | G17, G18 |
| G21 | Evaluate optional macOS VM and multi-host providers | M5 - Future | Astra xhigh | G17 |

The machine-readable source is [backlog.json](backlog.json). GitHub issue numbers and Project links are recorded after publication. All first-release implementation goals remain open.

## Parallel work boundaries

- Start G01 (Astra xhigh), G02 (Astra xhigh), and G03 (Luna max) independently.
- G04 integrates the gates and freezes contracts before shared implementation.
- After G05, IPC, auth, and pure scheduling can proceed with separate file ownership.
- Linux and native macOS providers can proceed independently once shared contracts/credentials exist.
- Integration, security verdict, soak and rollout are sequential evidence gates.
