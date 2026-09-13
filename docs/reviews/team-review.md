# Agent team planning review

Reviewed 2026-09-07. This summarizes independent research and code/document inspection, not implementation or live infrastructure verification.

| Review | Model / effort | Principal findings |
|---|---|---|
| Architecture and language | gpt-6-astra / xhigh | Go official SDK integration, native supervisor, explicit Docker adapter, ACK-before-callback risk, min-total semantics, native trust limits |
| Authentication and security | gpt-6-astra / xhigh | Manifest loopback unproven, App install permission checks, launchd Keychain identity, same-user risk, SDK error-body leakage, owned cleanup |
| TDD and delivery | gpt-5.6-luna / max | Deterministic reducer/fake clock, fault barriers, real SQLite and DB-service tests, public-PR isolation, bootstrap independence, evidence gates |

The Astra entries above are historical authorship and historical review scope;
they do not assign current work or current review ownership. New work follows
the routing in [EXECUTION](../EXECUTION.md).

## Resolutions by the integrator

- Choose Go based on integration/operational complexity; acknowledge Rust's stronger compile-time concurrency guarantees and require race/fuzz/fault tests. No performance benchmark is claimed.
- Use minTotal/maxTotal for autoscale. Do not silently mix minimum-idle and minimum-total semantics.
- The official listener is not an exactly-once durable event bus. G01 proves recovery and safe acquisition/drain ordering before integration.
- Use separate controller/job identities for the protected native profile. Same-user Keychain/workdirs are not an isolation boundary. Native jobs remain trusted-only even with distinct UIDs.
- Linux services need a worker-specific Docker daemon. The outer runner is launched unprivileged; only the reviewed worker-specific DinD service receives manager-runtime privilege. Trusted jobs retain broad control of their worker daemon, including inner privileged containers unless separately restricted. This does not certify hostile multi-tenancy. G10/G19 must resolve the exact topology.
- Credential import is valid for a local-only v0.1. Manifest creation ships only if its live loopback/disabled-webhook gate passes.
- Startup is login-scoped until actual service identity and Keychain evidence support more. Do not disable FileVault or infer execution before disk unlock.
- Existing Docker Desktop or Lima engines remain selectable. Runtime ownership/migration is separate from runner management; no current infrastructure changes are necessary to plan this product.
- Optional VM isolation and multiple hosts remain a later research gate, not a feature implied by the first-release CLI.

The resulting backlog contains 21 bounded goals with primary model, risk, dependencies, TDD evidence and acceptance criteria. Historical Astra reviews covered security/protocol/lifecycle boundaries, including Luna changes that touched them; current review ownership follows [EXECUTION](../EXECUTION.md).

Final read-only review corrections: runner freshness policy now has provider and soak acceptance coverage; quarantined workers retain reservations; Docker daemon privilege is described accurately; browser redirects do not incorrectly require Origin; external IDs are recorded after creation/discovery, following durable intent; bootstrap invariants distinguish management credentials from approved per-worker JIT transport; executable G04 contracts depend on the G03 Go bootstrap.

TDD publication review: G03 validates tooling without ceremonial application tests; first behavior contracts begin in G04. G08 enrollment and G15 launchd lifecycle are Astra-owned; Luna G12 is limited to pure versioned proposals and Astra G05/G13 owns atomic live admission. Every backlog entry carries initial status and test profiles.
