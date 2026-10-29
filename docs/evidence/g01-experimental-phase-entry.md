# G01 offline experimental phase entry

This path supplies authenticated experimental command dispatch. It does not
complete G01/G02, authorize a live canary, select a persistent signer backend,
provision keys, or replace the production implementation gates.

`g01-broker --execute-experimental-broker` selects sealed `RunBrokerExperiment`.
`g01-live --execute-experimental-phase --mode MODE --phase PHASE` selects sealed
`RunControllerExperiment`. Existing ordinary broker behavior and both legacy
controller execution quarantines remain. The receiver uses the actual Driver or
the complete same-process paired terminal implementation; there is no dispatcher
callback or legacy launch fallback.

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
external decisions. The exact existing controller binary metadata/tag gate is
unchanged; fixture builds remain inadmissible.

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
conditional deletion capability. The paired normal-success direct deletion
remains its existing exclusive experiment policy; no fault reconciliation or
unconditional fallback was added.

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
