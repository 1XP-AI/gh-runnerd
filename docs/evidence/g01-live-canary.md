# G01 minimal private live-canary plan

Status: **not run; not authorized by the offline spike**. This is the concrete
experiment to arrange and review. Registering live resources, minting JIT and
running workflows require explicit authorization for the named environment and
reviewed immutable harness/workflow commit. Do not mark these checkboxes passed
because the offline fixture supplied the desired behavior.

A separate [tagged controller phase driver](g01-live-driver.md) now provides a
reviewable executable for bounded controller operations. It has not run live and
does not launch workers or complete the phases below. The original fixture
remains loopback-only.

## Required environment and authorization record

- [ ] Maintainer identifies one disposable **private** repository and one
  organization; no public fork/PR execution. Record the reviewed workflow SHA.
- [ ] Dedicated runner group allows only that repository, and a unique canary
  scale-set name/label and owner nonce cannot target existing manual runners.
- [ ] Maintainer supplies an existing user-owned App installation with
  organization self-hosted runners write permission. G01 does not create an App,
  change its permissions or borrow credentials from existing runner installs.
  A separate secret broker keeps management credentials on the controller.
- [ ] Dedicated trusted controller and disposable worker environment is named.
  Prefer one Linux ARM64 worker on an explicitly chosen Docker endpoint/runtime;
  Docker Desktop presence alone is not authorization to use it. Native macOS
  requires a separately approved trusted execution identity. Do not share the
  management credential context with job code.
- [ ] Resource budget: **one active worker maximum**, one scale set, one session
  owner, at most two small queued canary jobs per phase, 1 vCPU / 1 GiB worker
  target after host headroom is checked, 10-minute job deadline. Increase any
  limit only with new concrete authorization. No service-container workloads in
  this protocol canary.
- [ ] Use SDK `v0.4.0`, Go `1.26.8` and runner `2.337.0`; verify runner bytes
  against [published digests](g01-contract.md#runner-and-jit-transport) and record
  exact runtime/image digest. Recheck support/security status before execution.
- [ ] Independent Luna max reviews the immutable live harness, authority
  checks, cleanup, fault barriers and sanitized telemetry. The offline fixture
  must not be pointed at GitHub by replacing its URL. The separate tagged driver
  and remaining worker steps require their own exact-commit review/approval.
- [ ] Maintainer authorizes the named scale-set/session creation, bounded JIT
  issuance, workflow dispatch, manager interruption, response suppression and
  owned cleanup operations below. Record authorization without secret values.

Use a dedicated experiment owner nonce and durable non-secret operation log.
Inventory existing runners read-only before/after; do not modify them. Record
only operation kind, relative time, attempt/session generation, anonymized stable
resource identifiers, response status/category, booleans and counts. Never write
HTTP bodies/URLs, auth headers, JIT values, environment dumps, full runner logs,
private names or personal paths into the public report. Suppression barriers must
operate inside the controller adapter after receipt of a response; do not use a
credential-recording HTTPS interception proxy.

## Minimal execution phases

Run phases serially. Each phase begins by verifying that only its owned resources
exist and that no previous worker is busy/unknown. Bound each observation window
to ten minutes; timeout means unresolved/quarantine, not success or forced cleanup.

| Phase | Concrete intervention and observations | Required decision evidence |
|---|---|---|
| Baseline | Register the dedicated scale set/session. Launch one runner with worker-only JIT environment and execute a reviewed no-op job with a heartbeat and normal completion. Inspect allowlisted booleans for JIT variable absence in the job and expected private credential-file existence, without printing values. | Exact SDK/runner/runtime inputs, one-job execution, normal deregistration/process exit and no management secret inheritance. |
| Before ACK | On a new controlled job message, stop the listener at the barrier after poll receipt and before DELETE. Restart using the reviewed session recovery path. | Whether/how the unacknowledged message/request ID is redelivered, session owner/ID behavior, request cursor handling and aggregate demand. Do not assume the documented replay timing. |
| After ACK / callback loss | Allow DELETE to return success, then stop the listener before the first callback or acquisition. Resume with independent statistics/reference reads. | Whether aggregate demand converges, which request identities are lost, whether availability redelivers, and what remains quarantined. Restored count alone is not complete recovery. |
| Acquisition ambiguity | Journal request IDs and intent. First stop before POST; separately allow POST success, suppress its result to the application, then restart. Observe GitHub state and owned workers without automatic retry. | Distinguish not-attempted from accepted/unknown. Any experiment repeating the same acquired request ID needs review of that exact step and may affect only the disposable no-op job. Establish reoffer/expiry/reacquisition behavior or keep it unresolved. |
| JIT ambiguity | Journal one stable worker identity. Stop before JIT POST; separately issue JIT once, suppress its result and stop before launch. Lookup the stable runner name/ID. No new worker identity or JIT retry. | Confirm discoverability and metadata scope; show the unknown worker reservation remains held. Determine safe owned deregistration/reissue only after the original runner's inactive state is established. No claim that the secret can be recovered. |
| Idle assignment during drain | Create one idle canary runner; hold one scale-set poll in flight, request local drain/zero capacity and dispatch one no-op job. Record whether the old poll acquires and whether the registered idle runner is assigned despite withdrawal. Never stop a worker based on an old idle observation. | Real ordering of withdrawal, acquisition and assignment. A fake server cannot supply this result. |
| Busy-safe deregistration | With the owned canary job running and emitting heartbeat, call the supported `RemoveRunner` once, while allowing the runner/process to continue normally. In a separate idle-to-assigned race, attempt deregistration of only that owned ID while the controlled job is offered. | A busy refusal must preserve heartbeat/job completion. If a successful deletion can race with assignment or affect running work, stop this phase, retain the evidence and reject that drain algorithm. This potentially disruptive disposable-job probe needs explicit authorization above. |
| Session replacement | Stop the session owner at an operation barrier, close/recreate only its own session through supported methods, start a new local generation with a reset cursor, independently reconcile. | Real replacement/lease conflict behavior, message identity scope, late response fencing, surviving demand and unknown reservations. Never delete another owner's session. |

A workflow that merely sleeps for a short fixed duration and records a harmless
completion marker is sufficient. It needs no repository secrets, deployment,
package publication or external side effects. The canary does not cancel or
automatically rerun jobs; any disposable job interrupted by a fault is reported
with its GitHub outcome and left for explicit maintainer handling.

## Exit evidence and cleanup

- [ ] Publish a sanitized timeline of each phase with actual results and
  unresolved observations, distinguishing server observations from inferred
  controller state. Attach immutable reviewed commit references.
- [ ] Record absent/held reservations and every owned runner/process/container
  ID using public aliases. No unexplained duplicate worker or orphan is allowed.
- [ ] Show initial manual-runner inventory remains unchanged.
- [ ] After any busy canary job completes, stop new admission and remove only
  individually verified owned runners, session, scale set and disposable worker
  resources. If activity or ownership is ambiguous, quarantine and request
  maintainer intervention. No broad process kill or Docker prune.
- [ ] Revoke/delete only temporary credentials belonging to this experiment
  according to the maintainer's authorization; preserve the existing App and
  unrelated installations.
- [ ] Independent Luna max reviews the sanitized results and the final recovery /
  drain decision. If acquisition, JIT ambiguity or safe drain remains unknown,
  keep G01 unresolved and dependent production implementation blocked.

Rollback is stopping further canary admission, waiting for owned busy work,
retaining uncertain resources for diagnosis and removing only verified disposable
resources. Existing manual runners remain the operational fallback. This plan
does not implement production providers or adopt the upstream demo's forced
busy-runner shutdown.
