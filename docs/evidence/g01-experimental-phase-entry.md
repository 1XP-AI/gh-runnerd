# G01 offline experimental phase entry

This path supplies authenticated experimental command dispatch. It does not
complete G01/G02, authorize a live canary, select a persistent signer backend,
provision keys, or replace the production implementation gates.

`g01-broker --execute-experimental-broker` selects sealed `RunBrokerExperiment`.
`g01-live --execute-experimental-phase --mode MODE --phase PHASE` selects sealed
`RunControllerExperiment`. The legacy `--execute-approved-broker` route is
rejected, and the exported `RunBroker` API is an inert fail-closed compatibility
stub. Experimental broker selection requires PublicPolicy v3 and a nonzero
GroupPolicy v2 before credential input or API construction. The legacy direct
controller and standalone worker execution flags are hard refusals. The signed
receiver reaches the actual Driver or private same-process paired terminal
implementation only after its controller handoff checks; public `Driver.Run` and
`RunPairedTerminal` do not provide a local bypass.

For the v3 broker path, provider state is first checked with an exact
repository-scoped read-only installation token (`organization_self_hosted_runners:read`, plus
`metadata:read` when repository-scoped). A failed group/repository/workflow
policy check stops before management write-token issuance. After the read-only
preflight succeeds, the broker repeats the final reusable-callee workflow GET,
requests the exact management write token, and repeats provider-state preflight
before handoff. This narrows credential issuance on observed policy failures;
it does not eliminate provider-state races or revoke a token after issuance.

Each executable independently links a public registry string:
`enrollment.brokerExperimentPolicyJSON` and
`livecanary.controllerExperimentPolicyJSON`, respectively. Both default to empty.
No argv, environment, payload, adapter or exported production setter can supply
roots or grants. The bounded JSON has version, exact build role, fixed workflow
source, separate workflow/issuer IDs and public Ed25519 keys, and a finite grant
array. Each grant binds mode, exact raw controller approval digest, repository,
run, ref/SHA/path, phase, owner nonce, source, organization/repository IDs and
expiry. Parsing rejects duplicate/unknown/missing/null fields, including nested
case substitutions, role/key collisions, duplicate grants and expired authority.
Build inputs, actual public pins/grants and deployment remain separately reviewed
external decisions. Both the `g01-live` front door and broker require Go 1.27.3,
the exact command import path, native `GOOS`/`GOARCH`, `CGO_ENABLED=1`, only the
`g01_live` build tag, a clean embedded VCS revision and the unreplaced Scale Set
SDK v0.5.0. Fixture builds remain inadmissible. These metadata gates do not
establish live readiness or authorize execution.

The fixed broker inputs are owned management stdin, FD3 independent workflow
observation credential, FD4 workflow signer seed, and FD5 issuer signer seed.
The receiver owns control FD3 plus credential stdin. Policy/request validation
precedes wrapping inherited descriptors. Every protected original is marked
close-on-exec before preparation children; aliases are rejected by Fstat identity.
Owned duplicates use ForkLock-aware dup plus close-on-exec, nonblocking polling,
bounded EOF-required reads and joined cancellation watchers. Named FIFOs require
private permissions; UID-owned anonymous pipes have no filesystem link (Darwin
reports mode 0660). Both derived public keys must match their fixed independent
roles before observation or management effects. Seeds are ephemeral experimental
inputs, not a persistent custody mechanism. All tests use synthetic material.

Create retains v1 receipt/binding/signing bytes and its 30-second ceiling.
Other phases use version 2, kind `g01-issued-phase-binding`, signing domain
`gh-runnerd/g01-issued-phase-binding/v2`, exact mode/phase audience, raw approval
and receipt digests, final payload digest/length, nonce, role IDs and times.
Paired identity binding is inserted before final payload serialization/signing.
The original budget is chosen once, narrowed through signing/launch/input/effects
by approval, grant, receipt, worker, parent and token-minus-minute deadlines.
Fault/drain remains capped at ten minutes; paired requires over sixty seconds
remaining. A shorter input timeout does not reset that budget.

Durable consumption atomically burns both the nonce and one phase/paired slot
before credential input. Decode, factory or cancellation failure cannot be retried
with a fresh nonce or reopen, and preparation rejects the burned slot before
reminting. The per-open proof retains concrete journal identity/pointer, original
parent, mode/phase, receipt and deadline; replay never recreates it. Create keeps
its sequence-one grammar. Completed-create parsing remains strict; paired replay
accepts only the narrow trailing paired-consumption prelude without filtering or
renumbering any physical event. The consuming controller journal stays open into
paired execution. Recovery-only renewals cannot restore create/pair authority.

Generic cleanup refuses before intent/input/mint because the current SDK has no
conditional deletion capability. The pinned SDK's ID-only
`DeleteRunnerScaleSet` is intentionally not exposed by `SDKAPI`. The paired
production route also refuses before Docker when its API lacks
`ConditionalScaleSetDeleter`; no unconditional fallback remains. A
`g01_pair_fixture && !g01_worker` adapter is available only for synthetic tests,
requires a TLS loopback endpoint and sends `If-Match` to that fixture. Its
replacement test proves the fake provider rejects a changed ETag; it does not
establish GitHub's versioning/conditional-delete semantics or atomic coupling to
runner inventory. Live cleanup remains unresolved.

The separately bounded [session replacement experiment](g01-session-replacement.md)
uses its exact signed phase, original create prefix and v2 audience. Its two
same-invocation session closes confer no legacy cleanup or recovery authority.

The official offline checker prepares current-source synthetic TEST dispatchers
once in an owned temporary directory, with serial bounded race builds, supplies
them to the existing fixture partition and removes only its owned directory.
Focused tests cover real command dispatch, broker fake HTTP observation/mint,
receiver durable consumption, a before-ACK barrier, inspect, and full paired SDK
and fake Unix-runtime sequencing. Exact source manifests, actual red/refail and
normal/race commands/results belong in each candidate's implementation report.
Independent security/recovery review, exact-head Codex/quick checks, integration
CI, real custody/pins/policy/headroom, fault cleanup/reconciliation, explicit live
authority and the remaining full acceptance evidence stay open.

### Current source validation — 2026-11-03

At implementation commit `f854496` (with only the two evidence documents
modified in the worktree), the official G01 `core` and `jit` profiles both
exited 0 with the pinned local Go 1.27.3 darwin/arm64 toolchain, `GOTOOLCHAIN=local`,
`GOPROXY=off`, `GOSUMDB=off`, and task-scoped temporary/build storage on the
external worktree volume. The machine-specific absolute toolchain/cache paths
are omitted; both invocations used `GO=/path/to/go1.27.3/bin/go`, with
`TMPDIR` and `GOCACHE` set to a task-scoped directory on that volume and
`GOMODCACHE` set to a pre-populated local module cache. Module downloads were
disabled. Both reported `offline experiment checks passed: 1 module(s)`.
These are offline fixture results only; they do not qualify a provider ETag,
runner/JIT behavior or a live canary. GitHub CLI read-only checks confirmed
issue #1 OPEN and Project #2 In progress.
