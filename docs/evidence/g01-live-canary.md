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

### Read-only repository-plan check — 2026-11-03

Additional read-only `gh api` checks against the private canary repository's
`main` branch-protection and repository-ruleset endpoints returned HTTP 403
with GitHub's plan-entitlement response requiring GitHub Pro or public
visibility. The repository is intentionally private, so the `main` ref cannot
currently satisfy `github.ref_protected`; retain the caller's fail-closed
guard. No branch rule, ruleset, environment, visibility, workflow, runner or
other repository setting was changed. See the [branch-protection API
documentation](https://docs.github.com/en/rest/branches/branch-protection#get-branch-protection).

GitHub's [deployment-environment documentation](https://docs.github.com/en/actions/reference/workflows-and-actions/deployments-and-environments)
states that required-reviewer protection for private-repository environments
is available only with GitHub Enterprise; Free, Pro and Team provide that
protection only for public repositories. Therefore the private canary needs an
eligible Enterprise-backed owner before both protected-ref and human-review
gates can be configured. Do not make the canary public or weaken the caller
guard to work around this constraint. Selecting another private repository
would require separately rechecking its owner plan, App access and runner-group
scope. No canary workflow was dispatched; execution still requires its own
concrete maintainer authorization.

### Historical Luna Max static harness review — 2026-11-03

Disposition: **HOLD** at exact public source HEAD
`686df326b911184b9c3df2c9bd2a9f7ba65b5b49`. The review performed no tests,
program execution, file mutation, GitHub write, credential, runner, host or
Scale Set operation.

Finding F1 was the mismatch between legacy broker launch flags and the
`g01-live` command's unconditional rejection of direct execution. The rebuttal
is limited to the selected GroupPolicy v2 reusable-callee route: ordinary
`RunBroker` rejects a v2 approval without the experimental signed assembly
before making GitHub API calls; the selected route validates the signed
reusable-callee binding and launches the explicit experimental mode. The
direct-execution CLI flags remain rejected.

The legacy `g01-broker --execute-approved-broker` / `RunBroker` route remains
callable when `GroupPolicy` is omitted and has its zero value. Explicit
non-v2 `GroupPolicy` JSON is rejected; the legacy approval is unversioned. It
can make authenticated identity calls and mint an installation token before
launching `g01-live` with the incompatible direct flags, which `g01-live`
rejects before reading its input. This route therefore has pre-rejection live
effects; it is not side-effect-free and must not be used for the selected
canary. The finding is rebutted only within the selected v2 route's scope.
This PR does not disable the legacy route; any broad G01 live authorization
must explicitly exclude it or separately block that path before use.

Finding F2 was the absence of a production caller for the public evidence
allowlist. The candidate adds the tagged `--sanitize-public-evidence` output
mode described below. At the reviewed parent, the new focused CLI test failed
because this mode was not recognized; after the change,
the focused test passed under Go 1.27.3:

```sh
GOTOOLCHAIN=auto go test -tags g01_live ./cmd/g01-live -run 'TestPublicEvidenceSanitizer' -count=1
```

The sanitizer closes the output-path gap for manually verified observations
but does not establish their truth or close the live checklist.

The historical review could not close checklist items 353–356 offline: the rendered
private workflow and worker/executor are not in this checkout, and current
runtime, isolation, resource enforcement, live fault outcomes, actual sanitized
timeline, authorization and owned cleanup remain unqualified. Preserve the
checklist as open. Re-review the final immutable candidate after its changes are
committed.

### Follow-up Luna Max candidate review — 2026-11-03

Disposition at exact public source HEAD
`9483727b42054c94ba4511d86ab9343f831f6c01`: **APPROVE offline/static;
HOLD live**. The source and documentation review ran no tests, programs, or
live actions. It found no P0/P1 issue in the sanitizer or selected v2 route.
The review confirmed the bounded single-document parser and allowlisted output,
and identified a P2: the recursive duplicate-key scan lacked an explicit JSON
nesting-depth cap. A focused test reproduced that the 17-level input was
accepted at this head; the candidate now caps container nesting at 16 and the
boundary test passes at 16 and rejects 17. A fresh exact-head review of this
correction was pending at that time; the following section records its result.

The same review noted that the preceding section recorded only the historical
`686df326` review. This follow-up is the exact-head record for `9483727`; neither
review closes the live runtime, authority, authorization, fault-outcome,
sanitized-timeline or owned-cleanup gates. The legacy zero-GroupPolicy route
remains outside the selected v2 canary and is a no-go for any broader G01 live
authorization unless explicitly excluded or separately disabled.

### Exact-head Luna Max delta review — 2026-11-03

Disposition at exact public source HEAD
`88331bdb7fc81a4b390bf0d62502755ea009337d`: **APPROVE offline/static;
HOLD live**. The read-only review ran no tests, programs, or live actions and
reported no P0/P1/P2/P3 findings. It confirmed the scanner starts at depth zero
and rejects objects or arrays at depth 16 or greater, the boundary test accepts
16 containers and rejects 17, and the exact-case, duplicate-key, null
prevalidation and allowlisted-serialization checks remain present. Previously
recorded focused-test results were not independently rerun by this review and
remain attributed to their original execution.

This review resolves the pending exact-head delta for the depth-cap correction;
it does not qualify runtime behavior or authorize a canary. Provenance/trust,
isolation and resource enforcement, cleanup, concrete live authorization and
the remaining checklist gates remain open. The omitted zero-GroupPolicy legacy
route remains excluded from the selected v2 canary and a no-go for broader G01
live authorization unless explicitly excluded or separately disabled.

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
- [x] Offline source/fixture evidence verifies the v3 broker builds the management
  credential payload from authenticated issuance and sends it through the bounded
  handoff to the selected controller child; the controller validates the signed
  receipt/binding before reading credentials. The controller validates the
  approved organization-only scope and keeps the workflow-verification token
  distinct where a phase requires it. See the [broker evidence](g01-broker.md),
  [organization-only scope ledger](g01-organization-management-scope.md), and
  passing separate-process fixtures
  TestControllerProcessHandoffActualBrokerSeparateProcess and
  TestControllerProcessHandoffV3ActualBrokerChild. This closes source-level
  custody evidence only. The current controller launches no worker child, so
  actual worker credential inheritance, deployed identity, runtime isolation,
  live issuance and production provenance remain unqualified.
- [ ] Dedicated trusted controller and disposable worker environment is named.
  Prefer one Linux ARM64 worker on an explicitly chosen Docker endpoint/runtime;
  Docker Desktop presence alone is not authorization to use it. Native macOS
  requires a separately approved trusted execution identity. Do not share the
  management credential context with job code.
- [x] Plan a maximum budget of **one active worker**, one scale set, one session
  owner, at most two small queued canary jobs per phase, 1 vCPU / 1 GiB worker
  target, and a 10-minute job deadline; no service-container workloads. Raise
  any limit only with new concrete authorization.
- [ ] Measure available host headroom and verify effective resource enforcement
  on the selected trusted environment before execution; the budget above is a
  target and does not establish current capacity.
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
The tagged `g01-live --sanitize-public-evidence` mode is the output boundary for
manually assembled, independently verified observations. It accepts one JSON
array from stdin, limits the input to 1 MiB, rejects unknown fields, trailing
documents, duplicate keys, case-folded field names, explicit null values and
mixed execution flags, caps nested JSON containers at 16 levels, then writes
only the serializer's allowlisted document.
Required fields must be present. The exact schema check avoids Go decoder
case-folding, duplicate-key overwrite and null-as-zero behavior. It does not
open the private journal, credentials or a network connection, and it does not
attest that the supplied observations occurred. Use it only after the
observations have been independently verified; sanitized output alone is not
canary evidence. Treat any nonzero command result as invalid output: a failed
or short stdout write returns nonzero but cannot retract bytes already accepted
by the output sink. Each input item can provide
`elapsed_ms`, `attempt`, `session_generation`, `phase`, `operation`, `outcome`,
`resources` (`kind` and private `id`), `http_status`, `succeeded`, and bounded
`counts`; private resource IDs are replaced by stable aliases in the output.

### Offline authority and build identity update — 2026-11-03

Local candidate source commit `7e87850e181e958cae21043ba47e1eae0b7cf1c3`
closes two additional offline admission gaps. Public broker selection now
requires PublicPolicy v3 plus a nonzero valid GroupPolicy v2; exported
`RunBroker` is an inert compatibility stub, and the legacy approved-broker and
host-discovery CLI routes are disabled. For the selected v3 reusable-callee
route, exact read-only installation-token permissions are used for repository,
runner-group and workflow preflight. A failed preflight cannot request the
management write token. After it succeeds, the broker repeats the final
reusable-callee GET, requests the write token, and repeats provider preflight
before handoff. A read-only token issued before a later rejection is not
revoked and can remain valid until expiry. No real GitHub token was requested
or issued; tests used synthetic API responses.

The `g01-live` build admission check now requires Go `1.27.3`, its exact command
import path, native `GOOS`/`GOARCH`, `CGO_ENABLED=1`, only the `g01_live` tag, a
clean lowercase 40-character VCS revision, and the unreplaced
`github.com/actions/scaleset v0.5.0` dependency. A clean build of the candidate
recorded those exact values, revision
`7e87850e181e958cae21043ba47e1eae0b7cf1c3`, and `vcs.modified=false`; the binary
was inspected but not run. Go 1.27.3 offline checks passed:
`go test ./... -count=1` and `go vet ./...` in `experiments/g02-auth`, plus
`go test -tags=g01_live ./... -count=1` and `go vet -tags=g01_live ./...` in
`experiments/g01-scaleset`. These tests use fixtures and do not establish live
GitHub, credential, Scale Set, runner or host behavior.

As checked on 2026-11-03, the upstream [actions/scaleset release page](https://github.com/actions/scaleset/releases)
marks `v0.5.0` as its latest release, and the candidate build uses its exact
module checksum. The pinned [v0.5.0 client source](https://github.com/actions/scaleset/blob/v0.5.0/client.go)
deletes a scale set by numeric ID and exposes no server-side version/ETag
precondition. Therefore the existing `ConditionalScaleSetDeleter` requirement
still has no supported implementation; cleanup remains quarantined and no
delete fallback is enabled. The canary remains unrun and unauthorized. Issue
#1 and Project #2 must stay In Progress pending the remaining live-readiness
evidence, concrete canary authorization, and a safe conditional cleanup path.

### Independent offline review triage — 2026-11-03

The exact-head Luna Max review at `f88f0c8d4eb3365f7d52d9267e59ec4801f778b2`
approved the offline/static v3 broker implementation and held live qualification.
Three P2 implementation gaps were fixed in subsequent local commits
`d5859f5093893fd7aa22a7d3a45a4f0e9ac6c3e1` and
`432f0ce6ecce886eed2857e2c189e22d1ecb9d87`: reject legacy controller policy
selection, repeat the final workflow-run read after write-token provider
preflight, and verify the pinned SDK checksum in both broker and child build
admission. The remaining token lifetime and non-atomic provider race are recorded
as live limitations; no live token, provider, runner, or host operation occurred.

At this premerge checkpoint, the issue remained OPEN and Project #2 remained In
progress. Conditional Scale Set cleanup, final-head review/quick check, trusted
runtime and custody evidence, and concrete separately authorized canary
execution were still gates. The offline tests do not satisfy any live acceptance
criterion.

### Post-merge reconciliation — 2026-11-03

PR #153 merged as `0b7741c045b1da55e50f758b3feaf179eb053cea` from reviewed head
`a82764feafcf1dd3f081a14074efb1efed655f05`. GitHub Codex reviewed that exact
head and reported no major issues; the hosted `Go checks` run passed for the
same SHA. The PR records the completed G01/G02 offline suites, vet checks, RED /
GREEN evidence, and exact clean Go 1.27.3 build metadata. The binary was inspected
but not executed.

This closes the PR #153 exact-head Codex review and hosted quick-check gates. It
does not close G01 or authorize execution: issue #1 remains OPEN and Project #2
remains In progress. The pinned v0.5.0 SDK has no server-side version/ETag
precondition for Scale Set deletion, so cleanup remains quarantined. Installation
tokens may remain valid until expiry after rejection, and the final provider read
is not atomic with handoff. Authenticated production provenance and trust-root
provisioning, broker-only execution and trusted credential custody, qualified
runtime isolation and resource enforcement, concrete separate canary
authorization, and sanitized live evidence remain open. No canary dispatch,
credential issuance, Scale Set, runner, Docker, or host-service action occurred
as part of PR #153. A separate historical macOS baseline execution is verified
in the next section; it does not qualify the current Linux ARM64 experiment.

### Canary policy recheck — 2026-11-03

An authenticated, read-only `gh` recheck now finds the dedicated runner group
limited to one selected private repository, with public repositories disabled,
workflow restrictions enabled for one immutable workflow reference, and zero
registered runners. This supersedes the 2026-11-01 observation that workflow
restriction was disabled. Private repository, workflow, and commit identities
are omitted from this public record. No group or repository setting changed.

The manual caller is on the private target's `main` branch and calls the
SHA-pinned reusable workflow. The called workflow is explicitly marked as a
local static proposal that needs separate review and authorization. Its job
guard requires `github.ref_protected == true`; the current `main` branch reports
`protected: false`, so dispatching from that ref would not run the canary job.
GitHub documents that a reusable workflow uses the caller's `github` context and
that `ref_protected` is true only when branch protections or rulesets are
configured ([contexts](https://docs.github.com/en/actions/reference/workflows-and-actions/contexts#github-context),
[reusable workflows](https://docs.github.com/en/actions/reference/workflows-and-actions/reusing-workflow-configurations#github-context)).
No workflow was dispatched during this recheck.

A read-only Actions history query independently found one completed
`workflow_dispatch` on 2026-09-16 with a successful result. Its only job ran on
a self-hosted macOS ARM64 runner in group #3. This corroborates the older
baseline run noted above; it does not qualify the current Linux ARM64 workflow,
current runner configuration, or current runtime isolation. No run logs were
read, and no new dispatch occurred during this recheck.

The GitHub App installation's access to the selected private repository was not
independently verified in this recheck. No additional authentication scope was
requested or added. Branch protection, App installation selection, live harness
review, runtime/isolation/resource qualification, conditional cleanup, separate
canary authorization, and all live execution evidence remain open. This
configuration read does not authorize the experiment.

### Private branch-protection entitlement — 2026-11-03

Read-only `gh api orgs/1XP-AI` reports the organization plan as `free`, and the
canary repository's `main` branch reports `protected: false`. The branch
protection and repository-rulesets GET endpoints each return HTTP 403 with the
message that GitHub Pro or a public repository is required. The organization
rulesets endpoint returns HTTP 403 stating that GitHub Team is required. GitHub's
current documentation likewise lists protected branches and repository rulesets
for private repositories on Pro, Team, or Enterprise plans, and organization
rulesets on Team or Enterprise
([protected branches](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-protected-branches),
[repository rulesets](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets),
[organization rulesets](https://docs.github.com/en/organizations/managing-organization-settings/creating-rulesets-for-repositories-in-your-organization)).

The current private-repository plan therefore cannot provide the protection
required by the canary job's `github.ref_protected` guard. No plan, repository
visibility, branch, ruleset, or workflow setting changed. Making the repository
public is not an approved workaround. Even after a maintainer-authorized plan
and branch-policy change, the live canary still requires its separate concrete
execution authorization.

The current `gh auth status` scope list omits `read:user`; no scope was added.
The GitHub App installation's selected-repository access remains unverified.

### Broker read-token revocation offline follow-up — 2026-11-03

On base `8537b8ba93ced1cea995dbd4d0402b98ef85897b`, local implementation
commit `47182b560b57ae6e9340e013c4a964a71ba6715c` adds a single revocation of the
v3 broker's narrowly scoped read-only installation token after its provider
preflight attempt. The broker durably records the revocation intent before
`DELETE /installation/token`; only an empty `204` response is confirmed. An
ambiguous result is journaled and stops before the final workflow check and
management-token mint. The G01 separate-process fixture now covers the read and
write mint scopes and verifies one read-token revocation. The broker journal
fixture also rejects persistence of the synthetic read token.

The meaningful RED was
`go test -run '^TestBrokerExperimentV3StopsWhenReadOnlyTokenRevocationIsUncertain$' -count=1 .`
against the parent source. It failed because the synthetic revocation failure
still returned `controller_completed` after two mints and three workflow reads.
After the fix, the same case passes and the broker stops before the write
boundary. Go 1.27.3 results, with build/module caches and temporary build files
on the external worktree volume:

| Check | Result |
|---|---|
| G02 `go test -run '^TestBrokerExperimentV3StopsWhenReadOnlyTokenRevocationIsUncertain$' -count=1 .` | PASS |
| G02 `go test -run '^TestBrokerExperimentV3' -count=1 .` | PASS (5.798s) |
| G02 v3 suite rerun after the journal redaction assertion | PASS (5.344s) |
| G02 `go test -count=1 -timeout=120s ./...` | PASS (all packages; library 48.110s) |
| G02 `go test -race -run '^TestBrokerExperimentV3(ReadOnlyPreflightRejectsBeforeWriteMint|StopsWhenReadOnlyTokenRevocationIsUncertain|FinalGETFencesCalleeBeforeMintAndHandoff)$' -count=1 -timeout=60s .` | PASS (17.057s) |
| G02 `go test -race -count=1 -timeout=120s ./...` | PASS (all packages; library 120.491s) |
| G02 `go vet ./...` | PASS |
| G01 `go test -tags=g01_pair_fixture -run '^(TestControllerProcessHandoffActualBrokerSeparateProcess|TestControllerProcessHandoffV3ActualBrokerChild)$' -count=1 -timeout=120s ./livecanary` | PASS (3.001s) |
| `git diff --check` | PASS |
| `git diff --unified=0 | rg -n 'BEGIN (RSA|OPENSSH|EC|PRIVATE)|ghs_[A-Za-z0-9_]{20,}|github_pat_[A-Za-z0-9_]{20,}|/Users/|/private/var|/tmp/'` | No matches (rg exit 1) |

The initial G02 full-race attempt at a 45-second timeout expired while the
standalone compile test was running; the same full suite passed with a
120-second timeout. The first G01 process-fixture run exposed that its synthetic
server only modeled the previous single write-token flow; the fixture was
updated and both v1 and v3 process-handoff cases then passed.

This is synthetic offline evidence only. No real GitHub token was minted or revoked,
and no provider, runner, Scale Set, Docker or host operation occurred. An
uncertain read-token revocation may still leave that token valid until expiry;
the later management token is not automatically revoked. The provider-read to
handoff race, trust-root/custody, runtime/isolation, conditional deletion and
separately authorized live canary gates remain open. Issue #1 and Project #2
remain OPEN / In progress.

### Read-token rejection and journal uncertainty regression — 2026-11-03

The independent Luna Max exact-head review of PR #159 at
`d8f387fdc7e367ac65e23a3b449ee7202aff2163` identified two P1 findings: rejected read-token scope/expiry
responses discarded a usable
token before cleanup, and a token equal to the caller verification credential
could reach provider-state preflight and DELETE. It also identified a P2 gap:
a successful DELETE followed by failed revoked-record sync did not append an
uncertainty marker. Local regression tests reproduced the P1s. The correction
retains only a lexically valid, bounded read-token candidate for one journaled
revocation attempt, never preflights rejected issuance, records/skips a
caller-token collision before provider-state preflight, and appends terminal
uncertainty after a failed revoked-result sync. Verification on the corrected
source commit `4f3d84f52402f21ec5b1031a19af92aaeb60b424`, using Go 1.27.3
and Go caches on the external worktree volume:

| Check | Result |
|---|---|
| RED on the pre-fix tree: rejected scope/expiry cleanup and caller-token collision regressions | FAIL as expected |
| G02 focused test `go test -run 'TestBroker(ReadOnlyRevocationRecordsUncertainWhenRevokedSyncFails|ExperimentV3RevokesTokenFromRejectedReadIssuance|ExperimentV3DoesNotRevokeVerificationTokenCollision)$'` | PASS (1.777s) |
| G02 `go test -count=1 -timeout=120s ./...` | PASS (all packages; library 47.211s) |
| G02 `go vet ./...` | PASS |
| G01 `go test -tags=g01_pair_fixture -run '^(TestControllerProcessHandoffActualBrokerSeparateProcess|TestControllerProcessHandoffV3ActualBrokerChild)$' -count=1 -timeout=120s ./livecanary` | PASS (3.773s) |

No GitHub token, provider, runner, Scale Set or host operation was performed.
This offline correction does not change the selected SDK `v0.5.0` path or
close the still-unsupported conditional Scale Set deletion requirement; issue
#1 and Project #2 remain OPEN / In progress.

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

### Account-wide read-token revocation uncertainty fence — 2026-11-03

The Luna Max review of PR #159 identified a compound journal failure: GitHub
DELETE succeeds, then both the per-attempt `read_only_token_revoked` record and
its uncertainty record fail to sync. The per-attempt durable intent stopped the
same invocation, but an approved later `inspect` could open a new attempt and
mint a management token. A regression run against base `1ea35587f5df5fa6`
reproduced the gap: the later inspect returned `controller_completed` after two
token mints.

The correction adds a second intent/confirmation pair to the fixed account
admission ledger. It binds only phase plus attempt and journal device/inode,
contains no credential, and is synced before DELETE. Confirmation is written
only after the exact successful DELETE response and the per-attempt success
record are durable. Admission replay requires a unique start for the current
claim, no intervening event, and a matching confirmation. An unmatched start
refuses every later phase, including `inspect` and `cleanup`, before management
token issuance or child handoff. A visible confirmation is re-synced before the
next invocation may rely on it. The finite admission-ledger line bound now
includes this event pair for every allowed slot.

Verification on the corrected local candidate, with Go 1.27.3 and the
gh-runnerd Go cache, module cache and temporary directory on the external
worktree volume:

| Check | Result |
|---|---|
| RED at base `1ea3558`: double per-attempt sync failure followed by inspect | FAIL as expected; inspect completed after two token mints |
| Focused revocation, confirmed-history replay, inspect-fence and ledger-bound tests | PASS |
| G02 `go test -count=1 -timeout=120s ./...` | PASS (all packages; library 46.635s) |
| G02 `go vet ./...` | PASS |
| G02 focused `go test -race` for compound failure, confirmed-history replay and revocation-sync failure | PASS (4.635s) |
| `git diff --check` | PASS |

These are synthetic offline results; no real GitHub token was minted or
revoked, and no provider, runner, Scale Set, Docker or host operation occurred.
This closes the identified offline admission-fence gap. The read-to-handoff
race, management-token expiry/revocation, conditional Scale Set deletion,
trusted runtime and custody, production provenance, live readiness, and
separately authorized canary remain unresolved. Issue #1 and Project #2 remain
OPEN / In progress.

### Read-token issuance response-loss fence — 2026-11-03

The independent Luna Max review of PR #159 at `a35220b089d3a3bd79ab1575620892f138739a42`
identified a separate P1: the account admission ledger did not record the
read-token lifecycle until after a usable token response existed. If GitHub
accepted the POST but its response was lost, no token candidate was available
for DELETE and a later `inspect` could mint management authority over an
unresolved read-token issuance. The synthetic regression reproduced that
sequence: no account-level intent was present and the later inspect completed.

The local correction syncs `read_token_lifecycle_started` immediately before
the read-token HTTP request is dispatched. A lost response therefore leaves a
durable account-wide barrier. If a v3 attempt ends before the dispatch boundary,
it records `read_token_request_not_dispatched`, allowing safe later recovery.
An incomplete historical reusable-callee claim with neither lifecycle nor
no-dispatch evidence now fails closed. Correctly paired historical
`read_token_revocation_started` / `read_token_revocation_confirmed` records
remain valid. The finding has a causal regression for accepted-POST/response
loss, sync failure before request dispatch, no-dispatch recovery, legacy
incomplete claims and both current and historical confirmed-pair replay.

Local verification on the candidate based on `a35220b` used Go 1.27.3 and
external-volume Go caches:

| Check | Result |
|---|---|
| RED at `a35220b`: accepted read-token POST with lost response, then inspect | FAIL as expected; no account intent, inspect completed and minted again |
| Focused lifecycle, failure-boundary, replay and recovery tests | PASS (3.113s) |
| G02 `go test -race -count=1 -timeout=210s ./...` | PASS (all packages; library 129.994s) |
| G02 `go vet ./...` | PASS |
| `git diff --check` | PASS |

The source and docs are still a local candidate; fresh independent and
GitHub Codex review of its exact head and hosted PR quick checks remain pending.
GitHub issue, PR and Project metadata were read through `gh`; no provider
credential/token, runner, Scale Set, Docker or host operation was used. The
selected SDK path is unchanged. Production provenance and trust
roots, broker-only execution, management-token lifecycle, conditional Scale Set
deletion, trusted runtime/isolation/resource qualification, the authorized
Linux ARM64/JIT canary and sanitized live exit evidence remain open. Issue #1
stays OPEN and Project #2 stays In progress.

### Workflow-run provenance GET accounting fence — 2026-11-04

Issue #160 starts from the exact local base `a4d6f03b81cbc77f51bf9eb0d37fdf47d67c02f3`.
The initial authenticated workflow-run GET now has a two-record lifecycle in
the fixed account ledger, bound to the already-open per-attempt journal,
attempt/journal device+inode, controller-approval digest, source and exact run
identity. The per-attempt intent is durable first; the account start is synced
before dispatch; the account confirmation follows the validated GET and durable
per-attempt observation while the account lock remains held. Timeout, transport
failure, lost response, crash-visible start and confirmation failure remain
pending and block later replay before any installation-token mint. Only an
explicitly durable pre-dispatch outcome permits a later different recovery
slot; the consumed attempt and same phase are not automatically retried.

The causal fixture records the workflow GET itself in its ordered event stream,
including the synthetic lost-response and timeout cases, while its API fixture
rejects any installation-token mint before the workflow fence. A journal-boundary
failure records `workflow_provenance_get_not_dispatched` and permits a later
inspect; ambiguous GET outcomes leave `workflow_provenance_get_started` without
confirmation and block the inspect before any API call.

Local offline verification at exact candidate head
`6950a80639931e497b55e665049af4d4956eacdf` used the cached Go 1.27.3 toolchain
with external GOCACHE/GOMODCACHE/GOTMPDIR and downloads disabled:

The RED fixture files were applied to a clean archive of exact base
`a4d6f03b81cbc77f51bf9eb0d37fdf47d67c02f3`; no implementation files were
changed in that archive. The identical selector was rerun on a fresh archive
of the same base. Both runs showed no account workflow intent, then `inspect`
completed and minted twice.

| Check | Result |
|---|---|
| `go test -race -count=1 -timeout=120s -run '^TestBrokerExperimentV3LostWorkflowProvenanceResponseBlocksLaterInspect$' .` on exact base | FAIL as expected; `inspect` completed (`controller_completed`) and minted twice |
| Same RED command on a fresh archive of exact base | Same failure and behavior |
| Focused lost-response, timeout, no-dispatch and replay tests on candidate | PASS (4.573s) |
| G02 `go test -race -p=1 -count=1 -timeout=300s ./...` | PASS (all packages; library 138.676s) |
| G02 `go vet -p=1 ./...` | PASS |
| Provider/credential/workflow/runner/Scale Set/Docker/host/live-canary actions | Not run |

These are synthetic offline fixture results only. No provider credential or
installation token was minted, no workflow or runner was invoked, and no live
canary operation was attempted. Independent exact-head review, hosted quick
checks and all live authorization gates remain pending.
