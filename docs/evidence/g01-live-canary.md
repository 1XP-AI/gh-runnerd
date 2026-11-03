# G01 minimal private live-canary plan

Status: **not run; not authorized by the offline spike**. This is the concrete
experiment to arrange and review. Registering live resources, minting JIT and
running workflows require explicit authorization for the named environment and
reviewed immutable harness/workflow commit. Do not mark these checkboxes passed
because the offline fixture supplied the desired behavior.

Current follow-up: the caller-bound workflow review and runner-group readback
are recorded below. No runner is registered and no post-policy canary run
exists; this document does not authorize execution.

A separate [tagged controller phase driver](g01-live-driver.md) now provides a
reviewable executable for bounded controller operations. It has not run live and
does not launch workers or complete the phases below. The original fixture
remains loopback-only.

## Workflow and controller-phase provenance boundary

The inactive canary template is phase-agnostic: it keeps manual
`workflow_dispatch` but defines no dispatch inputs at all, and does not put a
phase in `run-name`. Do not infer a controller phase from `run-name` or workflow
inputs.
In the offline contract, the controller approval identifies one exact workflow
run ID and an allowed phase set; the signed broker provenance receipt binds the
approval digest, that run ID, and the selected controller phase for an
invocation. This fixture contract is not production provenance evidence. The
production authenticated provenance source and reviewed trust-root
provisioning/rotation remain unresolved, as does the provider-side conditional
delete gate. This template correction closes neither gate and authorizes no live
execution.

Release/source and byte qualification (2026-10-25): the named private target
offers Linux ARM64 runner `v2.338.0`. Its official archive and all ten image
layers have been hash-verified, and selected image runner/Node bytes match the
archive. The current experimental pin selects that immutable ARM64 manifest;
see the [runner checkpoint](g01-runner-release-checkpoint.md#2026-10-25-target-offer-archive-and-image-bytes).
These static results do not establish runtime behavior or authorize a canary.

## Required environment and authorization record

### Read-only preparation checkpoint — 2026-10-22

The maintainer authorized preparation, not execution. A dedicated private,
non-archived repository was located through `gh`; its name and local endpoint
remain in the private approval conversation, not this public record. Its current
workflow is manual-only, has empty token permissions, one ten-minute job and
non-cancelling concurrency. However, it targets macOS ARM64 with a fixed label
and prints the runner name. It is **not** the nonce-scoped Linux workload in
`experiments/g01-canary-assets`, and is not approved for this experiment.

The preparation host is macOS ARM64 and has a configured Docker Desktop Linux
context. Only local context metadata was inspected: daemon availability, worker
isolation and resources have not been verified. No context was switched, image
pulled, container started or runner changed. Approximately 7.5 GiB free disk was
observed, so runtime preparation must wait for adequate measured headroom.
The proposed first scope is one disposable Linux ARM64 worker (1 vCPU / 1 GiB),
one controller/session/scale set and one baseline job, with no host socket or
controller mounts in the worker. No current native runner is a fallback target.

The organization runner-group metadata request returned HTTP 403. Selected-only
repository policy, group identity and existing-runner inventory therefore remain
unverified; no authentication scopes or App permissions were changed. A reviewed
nonce-bound workflow commit, immutable executor/runtime inputs, separate
controller/broker identities, authenticated provenance/trust roots and supported
conditional cleanup are still required. A configured context or a successful
source review cannot clear these live gates. Later fault phases need their own
explicit approval; do not dispatch even the baseline from this checkpoint.

### Execution checklist

The 2026-10-25 read-only preparation additionally verified the target's offered
runner package and static distribution/image bytes. The earlier authenticated
UI inspection confirmed a dedicated, nondefault group selecting only the private
target with public repositories disabled and zero configured runners; its
All-workflows policy still needs the approved workflow restriction. Current
`gh` runner-group reads remain HTTP 403. Neither the earlier UI observation nor
the successful package-list read proves authority of the eventual execution
principal. Exact workflow/runtime/controller identities and the provenance and
cleanup capabilities below remain required.

### Read-only revalidation — 2026-11-01

Private observation record: `G01-CANARY-READONLY-2026-11-01`; captured
2026-11-01 (date-only; the exact query time was not retained in this public
record). The private GitHub metadata, workflow, prior-run, current-head, and
separate-review claims below are coordinator-supplied from the private approval
record; this document review did not query GitHub or independently verify them.
Private repository/workflow identities and raw responses remain omitted here.

The target remains private and non-forked. Its dedicated runner group is
selected-only, selects one private repository, blocks public repositories, and
currently has zero registered runners. The group reports
`restricted_to_workflows: false` with no selected workflow entries, so the
current policy is not execution-ready. The runner-group `GroupPolicy` v2
preflight requires workflow restriction to be enabled and exactly one selected
entry matching the independently approved full-SHA reusable-callee workflow;
the observed policy fails that gate. No runner-group setting was changed.

The private approval record says the workflow uses a manual caller pinned to a
reusable callee by full commit SHA, and that a separate Luna Max static review
returned HOLD because the group policy and runtime readiness do not qualify. It
also records an earlier successful baseline run on an older macOS ARM64 workflow;
that run does not qualify the current Linux ARM64 nonce workflow. The record
reports no run for the current private workflow head.

Public source at merge `f41f782527f30fddb254165625ec7f772a63b2bc` contains
reusable-callee `Request`/signed `Receipt` v2 and `PublicPolicy` v3 provenance
checks, plus the runner-group `GroupPolicy` v2 preflight;
[Public CI #41304144352](https://github.com/1XP-AI/gh-runnerd/actions/runs/41304144352)
is coordinator-reported as passed on merge
`f41f782527f30fddb254165625ec7f772a63b2bc` using hosted runners; this document
review did not query the remote run status.
This source/CI result does not establish production trust-root provisioning,
runner/JIT readiness, or server-side conditional cleanup.

The public [actual-host checkpoint dated 2026-10-28](https://github.com/1XP-AI/gh-runnerd/issues/1#issuecomment-6395166330)
separately reports Docker Engine metadata on the test host and successful
download and digest verification of the selected immutable ARM64 runner image.
No container was started. These are coordinator-reported observations from
that date; this review did not repeat them. They establish endpoint metadata
and image presence at that time, not current readiness, worker isolation, or
effective resource enforcement.

That actual-host checkpoint is distinct from the development host's configured
Docker Desktop Linux context. For that separate context, daemon availability,
worker isolation, and resource limits have not been verified. The
private-target revalidation made read-only GitHub metadata requests only.
During that revalidation, no canary workflow was dispatched; no self-hosted
runner or JIT operation, canary credential mint/read, Docker
daemon/image/container operation, or Scale Set operation occurred. Live
acceptance remains open.

### Fresh Luna Max exact-head readiness review — 2026-11-01

Disposition: **HOLD / NO-GO**. This fresh read-only review evaluated public
main source SHA `f90b2d173d8ea4a39f7d3b54be15de0d900412bf`. The review itself
performed no workflow dispatch, runner/JIT, credential, Docker/host,
runner-group, App, or file mutation. The preceding 2026-11-01 checkpoint is
preserved as historical; this subsection is its subsequent documentation-only
record.

The current GitHub review reports that private target runner group `#3` selects
the target repository but has `restricted_to_workflows: false`, an empty
`selected_workflows` list, and zero runners. The current Linux ARM64/JIT
workflow has no run. Historical run `35095628204` was a macOS ARM64
runner-baseline, not current Scale Set/JIT evidence. The App installation
membership recheck returned `403`, so the last user-confirmed exclusion is not
current verification. Private repository/workflow identity, workflow
path/SHA, installation ID, raw responses, host paths, credentials, and tokens
remain omitted.

The five historical findings remain bounded as follows; the anchors were
checked against the exact source SHA above:

| # | Finding and current disposition | Current source anchors |
|---|---|---|
| 1 | Durable one-shot `inspect` consumption was corrected offline: preparation rejects a previously seen phase, and the driver records `inspect` before preflight/read. This is not live evidence. | `experiments/g01-scaleset/livecanary/preparation.go:43-57`; `experiments/g01-scaleset/livecanary/driver.go:580-592` |
| 2 | FIFO/named-pipe shape is not authenticated caller provenance. Approved and paired execute paths remain quarantined; production broker provenance and trust-root authority remain unresolved. | `experiments/g01-scaleset/livecanary/controller_process_handoff.go:181-187`; `experiments/g01-scaleset/cmd/g01-live/main.go:67-72,231-234`; `experiments/g01-scaleset/livecanary/controller_experiment.go:11-12` |
| 3 | The experimental FD3 controller path is not live-cleared without broker-only policy/launch authority and trust roots; its offline entry requires the compiled policy/grant and exact approval, with independent roots checked before input. | `experiments/g01-scaleset/livecanary/controller_experiment.go:23-65,68-97`; `experiments/g01-scaleset/livecanary/controller_process_handoff.go:27-28,59-66` |
| 4 | The selected phase is bound into the derived request and signed receipt/binding, then checked before credential input. This remains offline-only evidence. | `experiments/g01-scaleset/livecanary/controller_handoff.go:250-266,386-415,452-477` |
| 5 | The driver requires `ConditionalScaleSetDeleter` with a server-side version/ETag freshness fence, but pinned `actions/scaleset` v0.5.0 is called through ID-only `DeleteRunnerScaleSet`; atomic cleanup remains a live blocker. | `experiments/g01-scaleset/go.mod:5-8`; `experiments/g01-scaleset/livecanary/cleanup_fence.go:39-49`; `experiments/g01-scaleset/livecanary/driver.go:542-553,658-669`; `experiments/g01-scaleset/livecanary/sdk.go:383-384` |

No canary was performed, and this re-review does not authorize one. The
reviewer made no source changes, tests, GitHub writes, commit, or push. Rollback:
revert this subsection only; it changes no source, runtime, or GitHub state.

### Read-only host runtime recheck — 2026-11-01

Coordinator-observed facts from the existing connected SSH terminal during this
turn: the host was Darwin/arm64; the Docker context was `colima`; and Docker
reported Linux/aarch64, 4 CPUs, `MemTotal` 8307113984 bytes, and
`ServerVersion` 29.5.2. Host-root disk reporting showed 460 GiB total, 12 GiB
used, and 222 GiB available; this is the host root filesystem, not the Docker
VM disk. An image inspection for
`ghcr.io/actions/actions-runner@sha256:9644537b29b4799cb752a2f23a45a1e603cf74c3c5998241a1533f0e8c3dc649`
reported the expected image reference absent from this Docker context.

No image pull, container start, runner/JIT operation, workflow dispatch, Scale
Set operation, or resource/configuration change was performed. This
point-in-time observation does not prove current VM disk headroom,
isolation/resource enforcement, trusted runtime readiness, or live Scale Set
behavior. The Oct 28 report remains historical; this newer observation
supersedes it only for current Docker version and image presence. Rollback:
remove this subsection only; it is documentation-only and changes no source,
runtime, or GitHub state.

### Authorized runner-group workflow policy — 2026-11-03

After explicit maintainer authorization, runner group `#3` was updated through
the GitHub CLI to require workflow restrictions and allow exactly one
same-repository reusable workflow pinned to a full commit SHA. The private
repository path and workflow SHA are omitted from this public evidence. The
existing repository scope was intended to remain one selected private
repository, with public repositories disabled.

The initial update enabled workflow restrictions but also cleared the selected
repository list. Read-after-write immediately found zero selected repositories
and zero runners. The original private repository selection was restored using
the group repository-access endpoint. Final read-back confirmed one selected
private repository, public repositories disabled, workflow restriction enabled,
one full-SHA workflow entry, and zero runners. A read-only workflow-run listing
at 2026-11-03 02:22 UTC showed only historical run `35095628204` from the
2026-09-16 macOS runner baseline; no post-policy canary run was present.

This is a permission-policy result only. At the time of this policy update, the
pinned reusable workflow was still a static proposal without the caller guard
recorded below. This policy change did not qualify its runtime, broker
provenance, trust roots, worker isolation/resource limits, or cleanup safety.
It does not authorize workflow dispatch, runner registration, JIT issuance, or
host/Docker operations. Full G01 remains HOLD / NO-GO. Rollback to the previous
workflow-access setting requires explicit maintainer authorization; no canary
execution state was created.

### Read-only Colima storage recheck — 2026-11-03 02:27 UTC

From the existing test-host SSH shell, `colima status` reported a running
macOS Virtualization.Framework VM with a Docker runtime on `aarch64` using
`virtiofs`. `colima list` reported 4 CPUs, 8 GiB configured memory and a
60 GiB configured disk. Docker reported Linux/aarch64, 4 CPUs,
`MemTotal` 8307113984 bytes, Engine 29.5.2, and Docker root `/var/lib/docker`.
Inside that VM, `df -h /var/lib/docker` reported 59G total, 17G used and 40G
available. Host-root `df -h /` reported 460 GiB total, 12 GiB used and 222 GiB
available. Inspecting the pinned runner image still returned “No such image”;
inspection did not pull it.

No image pull, container start, runner/JIT operation, workflow dispatch, Scale
Set operation or host/Docker configuration change occurred. This is a
point-in-time storage/capacity observation only; it does not prove worker
isolation, enforcement of the intended per-job resource limit, trusted runtime
readiness or live Scale Set behavior. The runner image remains absent. Full G01
is still HOLD / NO-GO; execution needs separate authorization. Rollback: remove
this subsection only; it is documentation-only.

### Caller-bound workflow and policy recheck — 2026-11-03

The coordinator directly rechecked the private workflow and runner-group state
with `gh` after the policy update. A private companion workflow change binds the
reusable job to a manual caller from the intended repository on its `main`
branch and pins the guarded callee by full commit SHA. The repository, workflow
path, immutable SHA and private PR identity are intentionally omitted here. The
contract suite (five tests), standalone contract check, `actionlint`, and PR
quick-check passed. GitHub Codex completed review of the exact PR head without
findings, and the change was merged. This review covers the caller guard and
static workflow contract; it is not a fresh review of every live harness phase,
authority handoff, cleanup barrier or fault-injection step.

Read-after-merge confirms runner group `#3` is selected-only, public repository
access is disabled, workflow restriction is enabled, exactly one full-SHA
workflow is selected, exactly one private repository is selected, and zero
runners are registered. A read-only workflow-run query found only historical
run `35095628204` from the 2026-09-16 macOS ARM64 baseline; no post-policy
canary run exists. This closes the static caller-binding and workflow-selection
checks only. It does not qualify the host/runtime, authenticated production
provenance or trust roots, worker isolation/resource enforcement, provider-side
conditional cleanup, or the Linux ARM64/JIT behavior. No workflow was
dispatched and no runner, JIT, credential, Scale Set, Docker or host operation
was performed.

### Read-only protected-environment status — 2026-11-03

A read-only GitHub CLI listing for Actions environments in the private canary
repository returned `total_count: 0`. No GitHub Actions environment or
environment-protection rule is configured. This metadata check changed no
environment, workflow, secret or repository setting. A protected approval
environment remains an open pre-execution gate; configuring one requires its
own explicit authorization and does not qualify the live harness or runtime.

The caller's `main` ref does not currently meet the reusable callee's required
`github.ref_protected == true` condition, so the caller remains fail-closed. The
candidate identity mapping described below is static preparation only:
the controller approval is still an invalid placeholder, no workflow run ID or
experiment approval exists, and no Scale Set, session, runner, credential or
host resource was created or changed. The driver permanently pins the first
experiment's ownership and journal; the candidate identity must not be reused
for a later experiment without the separately reviewed reconciliation described
in the [driver record](g01-live-driver.md#private-approval-and-invocation).

- [x] Maintainer identified one disposable **private** repository in the
  organization. The reviewed workflow identity and full SHA are recorded in
  the private repository and group configuration; no public fork/PR execution
  is allowed.
- [x] Runner group `#3` selects only that private repository, disables public
  repository access, and restricts execution to exactly one full-SHA reusable
  workflow. The private identifiers are omitted from this public record.
- [x] A fresh read-only `gh` recheck confirmed the runner-group repository ID
  matches the private caller repository, the group selects only that
  repository and the exact pinned reusable workflow, public repositories are
  disabled, and the group currently has zero runners. The callee names both
  this group and the nonce-bound label, so runners in other groups cannot
  receive its jobs; there are no existing runners in the selected group at this
  checkpoint. Recheck inventory immediately before any separately authorized
  dispatch because group membership can change.
- [x] Assign and statically verify the candidate canary identity before creating
  resources. The private manual caller's fixed 32-lowercase-hex `run-name`
  suffix is the reusable callee's runner-label suffix; `Approval.setName()`
  derives the scale-set and session-owner name as `g01-<owner_nonce>`. The
  controller approval must use that same suffix as `owner_nonce`, making its
  scale-set name equal to the workflow's required runner label. Private names,
  suffix and immutable workflow SHA remain in the private record.
- [x] A read-only `gh api` recheck on 2026-11-03 confirmed that the
  maintainer-supplied existing organization App installation has
  `organization_self_hosted_runners: write`. No App permission, installation or
  repository setting was changed. Private App identifiers are omitted here.
- [ ] Verify that the separate secret broker keeps management credentials on the
  controller. The organization-only App does not need repository access. Broker
  and controller approvals must explicitly select the reviewed
  [organization-only management scope](g01-organization-management-scope.md),
  with separate workflow verification authority where required. Scope support
  alone does not qualify issuance, provenance or execution.
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
- [x] The offline adoption profile selects SDK `v0.5.0`/Go `1.27.3` and the
  byte-verified runner `2.338.0` ARM64 image. The archive, manifest and config
  digests are in the [runner checkpoint](g01-runner-release-checkpoint.md#2026-10-25-target-offer-archive-and-image-bytes);
  see the [adoption ledger](g01-sdk-v05-adoption.md) and PR #107 for SDK review
  and verification. The frozen v0.4 recovery packet (Go 1.26.9 wrapper and older
  Go 1.26.8 historical/specimen text) is not the current execution profile and
  must not be relabeled by replacing version strings.
- [ ] Approve and qualify the actual daemon/runtime, recheck support/security,
  and verify effective isolation and resource enforcement before execution.
  This checklist does not authorize a canary or live runner operation.
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

The private `Event` journal is not a public report format. Convert independently
verified observations to `livecanary.PublicEvidenceObservation` and serialize
them only with `livecanary.MarshalPublicEvidence`. The serializer emits the
versioned allowlist, requires fixed phase/operation/category values, bounds time,
input size and counters, aliases resource IDs within the report, and reduces HTTP
status to its class. Their input fields are excluded from direct JSON output.
Rejected input returns a generic error without echoing values. This offline
serializer does not execute a canary or establish that an observation occurred.

## Minimal execution phases

Run phases serially. Each phase begins by verifying that only its owned resources
exist and that no previous worker is busy/unknown. Bound each observation window
to ten minutes; timeout means unresolved/quarantine, not success or forced cleanup.

| Phase | Concrete intervention and observations | Required decision evidence |
|---|---|---|
| Baseline | Register the dedicated scale set/session. Launch one runner with worker-only JIT environment and execute a reviewed no-op job with a heartbeat and normal completion. Inspect allowlisted booleans for JIT variable absence in the job and expected private credential-file existence, without printing values. | Exact SDK/runner/runtime inputs, one-job execution, normal deregistration/process exit and no management secret inheritance. |
| Before ACK | On a new controlled job message, stop the listener at the barrier after poll receipt and before DELETE. Restart using the reviewed session recovery path. | Whether/how the unacknowledged message/request ID is redelivered, session owner/ID behavior, request cursor handling and aggregate demand. Do not assume the documented replay timing. |
| After ACK / continuation loss | Allow DELETE to return success, then stop before caller acquisition/lifecycle continuation. The v0.5 handler has already validated the durable poll/source records; it is not a legacy SDK callback. Resume with independent statistics/reference reads. | Whether aggregate demand converges, which request identities survive in the journal, whether availability redelivers, and what remains quarantined. Restored count alone is not complete recovery. |
| Acquisition ambiguity | Journal request IDs and intent. First stop before POST; separately allow POST success, suppress its result to the application, then restart. Observe GitHub state and owned workers without automatic retry. | Distinguish not-attempted from accepted/unknown. Any experiment repeating the same acquired request ID needs review of that exact step and may affect only the disposable no-op job. Establish reoffer/expiry/reacquisition behavior or keep it unresolved. |
| JIT ambiguity | Journal one stable worker identity. Stop before JIT POST; separately issue JIT once, suppress its result and stop before launch. Lookup the stable runner name/ID. No new worker identity or JIT retry. [Signed offline physical-boundary evidence](g01-jit-boundary.md) covers only the dedicated zero-work prelaunch probe. | Confirm live discoverability and metadata scope; show the unknown worker reservation remains held. Determine safe owned deregistration/reissue only after the original runner's inactive state is established. Workload/demand/drain evidence remains separate. No claim that the secret can be recovered. |
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
