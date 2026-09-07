# G01 identity and terminal reconciliation evidence

Date: 2026-09-07. **Offline source evidence only; the live gate remains unresolved.**
This condenses independent Astra primary-source research into supported SDK,
runner and documented REST facts, expectations, and live-observation limits. It
is not a cleanup implementation, successor ledger or live approval.

## Scope and source pins

No production code, runtime, workflow, approval, dependency or broker state was
changed; no live API, runner, worker, Docker endpoint or credential was used.
See the [G01 contract](g01-contract.md) and [live plan](g01-live-canary.md) for
adjacent evidence and a future observation procedure. The selected source pins:

| Component | Pin used for the source reading |
|---|---|
| Scale Set SDK | v0.4.0, commit `6ce025902cd964747a078c2aabe7340ebc667eca` |
| Runner application | v2.337.0, commit `397b032cbf865e9c3ddfab89d533ec19325e1273` |
| ARC reference | gha-runner-scale-set-0.14.2, commit `9bb16ae49d0ce585d8e682aa7e2668a6e832d5d8` |

The accompanying source capture contains the exact retrieved-file SHA-256
manifest. Links use the pins above for source. REST pages were inspected on
2026-09-07 and render current documented examples even with
`apiVersion=2022-11-28`; that is documented capability, not proof that the
harness used that header. A live probe must verify its selected API version.

## Identity facts and the constrained tuple

The strongest source-backed expectation for one isolated baseline is one exact
identity tuple:

```text
JIT RunnerReference
  -> SDK GetRunner(reference.ID)
  -> REST runner with the same numeric ID
  -> terminal job.runner_id, runner_name and runner_group_id
```

Each arrow is an assertion to test on the selected run and attempt, not a
server invariant proven by this source review. A positive ID that differs
between SDK and REST, a matching name in the wrong scale set/group, a changed
run attempt or source, or multiple candidate jobs leaves the observation
unresolved. A name-only join cannot recover it.

| Identity | Established fact | Limit that remains |
|---|---|---|
| SDK runner | JIT returns `RunnerReference{ID, Name, RunnerScaleSetID}`. `GetRunner(ID)` addresses that exact agent; `GetRunnerByName(name)` returns the same reference type. | Read-back has no busy state, JIT secret, creation nonce or creation time. Name lookup does not show that this controller created the runner. |
| SDK request/job | `RunnerRequestID` is `int64`; `JobID` is an independent opaque string. Available, Assigned, Started and Completed messages carry both plus owner/repository/workflow reference/run ID. Started and Completed add runner ID/name; Assigned has no runner ID. | No numeric conversion or equality with the public REST job ID is specified. The SDK payload has no run-attempt or execution-head field. |
| REST workflow job | Attempt-specific listing supplies the numeric job identity. Exact job GET supplies status/conclusion, run/head, runner ID/name/group and check-run URL. | REST has no SDK request ID or opaque SDK `JobID`. Names and labels are not identity. Enumeration must stay within the approved attempt and bounded scope. |
| Runner application | The pinned runner copies `Runner.Id` to `TaskAgent.Id` in its REST list conversion and copies the agent ID into `RunnerSettings.AgentId` during registration. | This supports a shared agent/REST runner ID, but does not prove that a Scale Set JIT reference is visible at the organization REST endpoint or that every service flow preserves equality. |
| Executed job context | `job.check_run_id` is documented; the pinned runner copies the server-supplied context and its `JobContext` supports the numeric field. | It identifies a check run, not the opaque SDK job string. `runner.name` is explicitly not globally unique. Missing `check_run_id` cannot be represented as `0`. |

Keep these fields separate throughout any future journal:
`SDKRunnerRequestID`, `SDKJobID`, `RESTJobID`, `check_run_id`, SDK runner ID,
REST runner ID, run ID and run attempt. For a strictly isolated one-job
baseline, exact run/attempt/source plus the JIT/Started/Completed runner
identity provides a constrained correlation. An optional workflow identity
step can expose `job.check_run_id` through the existing jobs API, but no
official contract says that Checks `external_id` contains `SDKJobID`; that
would be a live-only probe requiring separately approved Checks-read authority.

Sources: [SDK types](https://github.com/actions/scaleset/blob/6ce025902cd964747a078c2aabe7340ebc667eca/types.go#L21-L62), [SDK JIT and exact/name reads](https://github.com/actions/scaleset/blob/6ce025902cd964747a078c2aabe7340ebc667eca/client.go#L604-L705), [workflow jobs API](https://docs.github.com/en/rest/actions/workflow-jobs), [runner-to-agent conversion](https://github.com/actions/runner/blob/397b032cbf865e9c3ddfab89d533ec19325e1273/src/Sdk/DTWebApi/WebApi/ListRunnersResponse.cs#L42-L45), [runner registration identity](https://github.com/actions/runner/blob/397b032cbf865e9c3ddfab89d533ec19325e1273/src/Runner.Listener/Configuration/ConfigurationManager.cs#L343-L397), [job context](https://docs.github.com/en/actions/reference/workflows-and-actions/contexts#job-context), and [pinned JobContext](https://github.com/actions/runner/blob/397b032cbf865e9c3ddfab89d533ec19325e1273/src/Runner.Worker/JobContext.cs#L60-L83).

## Error provenance and ownership rules

SDK v0.4.0 exports `RunnerNotFoundError`, `RunnerExistsError`,
`JobStillRunningError` and `MessageQueueTokenExpiredError`; its decoder
recognizes the service exception names. This gives a supported category for an
exact SDK operation, but the decoder classifies by exception-name matching and
does not require a particular HTTP status. A typed error or `errors.Is` result
alone is therefore not destructive absence proof.

An adapter may treat an exact owned `GetRunner(ID)` observation as a candidate
absence only when it retains normalized endpoint, expected status and category
provenance for that request. Generic 404, permission, network, malformed-body
or wrong-endpoint failures stay unresolved. Do not persist raw SDK errors or
response bodies; they can contain URLs or body data. The unrelated
scale-set-by-ID path has no dedicated scale-set-not-found sentinel.

ARC provides a supported upstream example of uncertainty handling: after a JIT
conflict it looks up the intended name, verifies the returned scale-set ID,
removes that runner and requeues configuration; ordinary cleanup retains a
runner when the SDK reports `JobStillRunningError`. This demonstrates useful
upstream facts, not permission for this harness to adopt by name, regenerate a
JIT, or retry a finite request. ARC's completed callback marks desired state
dirty; it does not itself use `JobCompleted` as a pod-delete proof.

The runner has a request model with request ID, opaque GUID job ID, nullable
result/finish time and lease fields, and its listener can read that request
under runner-server authority. The Scale Set SDK does not expose a controller
read-by-request operation. Do not extract worker OAuth credentials or call an
undocumented controller endpoint to obtain these fields.

Strict ownership remains unchanged:

- Cleanup needs an immutable receipt for the exact owned resource and its
  unchanged profile/ownership tuple. A name, label, prefix or empty inventory
  cannot adopt a resource.
- Unknown create, start, delete, JIT or callback outcomes retain the
  reservation and enter quarantine until a separately reviewed transition or
  explicit operator resolution exists.
- Zero statistics, a nil poll, elapsed time, an absent callback or a fresh
  approval cannot erase an earlier work-bearing observation.
- Never persist, print or recover a JIT secret, raw response, worker OAuth
  credential or broad job-context dump. Base64 JIT data is encoding, not
  protection.

## Terminal evidence

SDK history and local process state answer different questions:

| Observation | It can establish | It cannot establish |
|---|---|---|
| SDK `JobCompleted` | A matching SDK history event, when its IDs and source bind exactly | Successful execution: cancellation can be followed by requeue. The pinned README documents repeated Assigned/Completed(`canceled`) for one workflow job. |
| REST exact attempt/job is `completed` | GitHub's public workflow result for that exact job and attempt | That the SDK event was the same job unless the independent tuple binds it; a changed attempt/head/source must be refused. |
| Container is exited with a present code | Local termination of the exact owned process/container under an unchanged profile | GitHub success. `Runner.Listener` can return exit 0 after a run-once message without translating job result and can exit successfully for other service messages. |
| Runner status is idle/absent | A point-in-time public status or absence observation | A drain barrier, per-request release, or safe deletion while an assignment race is possible. REST deletion force-removes and is not the SDK busy-sensitive contract. |

For baseline success, require the exact REST workflow job/attempt to be
`completed` with conclusion `success`, with source/head/run identity unchanged.
Record cancellation, failure and timeout as terminal outcomes, not success. A
202 or no-message response adds no new statistic revision; no SDK revision
proves an atomic snapshot across APIs. GitHub's eventual ephemeral
deregistration, unused-JIT removal and 24-hour unassignment support eventual
observation only; they provide no exact per-request release receipt or local
worker-absence proof.

## Uncertainty reconciliation matrix

| Loss case | Supported later evidence | Still unavailable / required boundary |
|---|---|---|
| Clean baseline | Original set/session/message/request and JIT/runner/container receipts; matching Started/Completed; exact terminal REST job; exact local exit; exact runner absence with complete inventory and fresh owned-set statistics. | Cross-service ordering, absence propagation and safe busy/assignment behavior still require live validation. |
| Acquire response lost before JIT | Durable acquire intent, known request/run identity and a fence proving no JIT or worker handoff followed. Later observations can retire the experiment without claiming acquisition never succeeded. | No read-back of the accepted request-ID set, per-request release/cancel API or idempotency guarantee. Zero counters alone cannot recover acceptance or ownership. |
| JIT response lost before worker handoff | Name lookup may find a candidate and exact set ID. Under the strict contract, require an immutable creation receipt or explicit external operator resolution plus exact-ID absence/ownership evidence before any owned cleanup. | The old JIT secret cannot be recovered. Name/set equality is not a creation receipt. Never regenerate to discover what happened. |
| Known runner/container, callback missing | Exact terminal REST job bound to that runner, verified worker exit and exact runner absence can support a separately classified completion path after review. | Do not fabricate callback success; SDK completion history may not be replayable. Cleanup still needs its own exact ownership and busy-safe evidence. |
| Unknown create/start/delete result | If an exact ID was retained, inspect it with unchanged ownership/profile and pair state/local evidence. A controlled fault witness may identify a deliberately suppressed return without entering the driver's retry path. | Names, empty statistics and elapsed time do not establish ownership. A true network loss with no witness remains unresolved. |

The later paths are reviewed-transition options, not authorized state changes;
retain original evidence and uncertainty. A future observer receipt may record
only normalized IDs/source and response category privately before a deliberate
fault suppresses the driver's return; it must never store the JIT or raw body,
trigger retry, or be presented as automatic recovery.

## Broker and live-gate boundary

The current broker is a **global finite ledger with one slot per named phase,
including `inspect` and `cleanup`**. A fresh attempt directory does not grant
another inspection, reset a claim or authorize another JIT/acquisition. A
successor experiment needs an explicit pair/epoch, finite budget and separate
review. A proposed future sample/poll budget inside an approved phase is not
implemented here.

The smallest prerequisite for a future live observation is one reviewed
baseline contract:

1. Fix the private workflow run, attempt, source/head/path/event/group and
   exclusive nonce before the controller journal; wait only inside an approved
   bounded observation budget for the exact queued job.
2. Add normalized exact-ID SDK runner observations and read-only REST
   runner/job observations using existing separate controller authorities;
   synthetic tests must cover status/category provenance and secret-safe
   failures.
3. For one worker, collect the JIT -> SDK runner -> REST runner ->
   Started/Completed -> REST job tuple before automatic deregistration. An
   optional `job.check_run_id` step may strengthen the binding. Keep the pinned
   runner/image, `DisableUpdate`, exact container receipt and one-worker limit.
4. Define the finite sample/poll budget and manual reconciliation path before
   execution. Current one-use phases cannot be rerun by choosing a new
   directory. No baseline without a reviewed completion/cleanup or explicit
   external-resolution path should be requested.

## Sources and validation limits

Primary sources: [SDK errors](https://github.com/actions/scaleset/blob/6ce025902cd964747a078c2aabe7340ebc667eca/errors.go#L18-L101), [SDK message contract](https://github.com/actions/scaleset/blob/6ce025902cd964747a078c2aabe7340ebc667eca/README.md#L88-L116), [ARC JIT recovery and cleanup](https://github.com/actions/actions-runner-controller/blob/9bb16ae49d0ce585d8e682aa7e2668a6e832d5d8/controllers/actions.github.com/ephemeralrunner_controller.go#L649-L699), [ARC completion callback](https://github.com/actions/actions-runner-controller/blob/9bb16ae49d0ce585d8e682aa7e2668a6e832d5d8/cmd/ghalistener/scaler/scaler.go#L159-L162), [runner job request model](https://github.com/actions/runner/blob/397b032cbf865e9c3ddfab89d533ec19325e1273/src/Sdk/DTWebApi/WebApi/TaskAgentJobRequest.cs#L72-L237), [runner request check](https://github.com/actions/runner/blob/397b032cbf865e9c3ddfab89d533ec19325e1273/src/Runner.Listener/JobDispatcher.cs#L249-L303), [workflow run attempt](https://docs.github.com/en/rest/actions/workflow-runs#get-a-workflow-run-attempt), [Checks API](https://docs.github.com/en/rest/checks/runs#get-a-check-run), [pinned listener](https://github.com/actions/runner/blob/397b032cbf865e9c3ddfab89d533ec19325e1273/src/Runner.Listener/Runner.cs#L573-L603), [self-hosted runner API](https://docs.github.com/en/rest/actions/self-hosted-runners#delete-a-self-hosted-runner-from-an-organization), [ephemeral runners](https://docs.github.com/en/actions/reference/runners/self-hosted-runners#ephemeral-runners-for-autoscaling), [JIT removal](https://docs.github.com/en/actions/how-tos/manage-runners/self-hosted-runners/remove-runners), and [ARC lifecycle](https://docs.github.com/en/actions/concepts/runners/actions-runner-controller).

These are source/doc contracts, not server equality, propagation-bound,
lease-cancellation or race-barrier measurements. No live runner, workflow or
REST API behavior is claimed. The document does not implement cleanup,
successor budgeting, reconciliation storage, workflow changes or approvals.
It contains no credential, JIT value, raw response, private machine path or private test log.
