# G01 disposable worker harness

Status: implemented and tested with synthetic Unix HTTP servers only. This is a
bounded continuation of [issue 1](https://github.com/1XP-AI/gh-runnerd/issues/1),
not live approval or a production provider. G01's GitHub protocol, worker and
cleanup evidence gates remain open. No Docker daemon, container, image layer,
runner registration or workflow was used to verify this change.

## Executable scope

`experiments/g01-scaleset/cmd/g01-worker` requires the `g01_worker` build tag and
`--execute-approved-worker`. `--plan` is offline. The command accepts one phase,
an exact-build private approval and an exclusive private journal. It does not
obtain JIT, dispatch a workflow, obtain management credentials or release a
GitHub capacity reservation. The controller and worker journals are separate;
their integration and the final approval must bind the same experiment, runner,
workflow commit and one outstanding JIT reservation.

| Phase | Effect and prerequisite |
| --- | --- |
| `create` | Verify daemon and preloaded image, persist intent, submit one create request, then persist its returned immutable ID. Accept JIT on standard input only. |
| `start` | Inspect only the journaled ID and verify its complete profile. Require `created`, persist start intent and submit one start request. |
| `inspect` | Recheck daemon/image and inspect the journaled ID. Record only an allowlisted status. Observation never clears an uncertain effect. |
| `cleanup` | Recheck exact ID, ownership and profile. Only `created` or `exited` may be removed with `force=false&v=false`; active, unknown or mismatched resources stay reserved. |

No phase pulls/builds an image, discovers containers by label, stops/kills a
worker, restarts it, captures its logs, prunes resources or changes Docker
contexts. The helper never creates a second worker under the same journal, even
after successful cleanup. This is a per-journal limit; the controller's permanent
scale-set admission pin does not cap containers across independent worker
journals. Integrated one-worker/global worker admission remains an open gate.
A lost create/start/delete response or a failed result
write retains the intent and forbids subsequent mutations. A returned create
warning also quarantines the worker, retaining its known ID for inspection.
Unknown create with no durable ID requires private operator reconciliation; this
slice cannot discover or adopt that container. A missing container is an error,
not evidence that GitHub's runner/job is absent.

## Fixed worker profile

The only accepted image reference is the reviewed Linux ARM64 manifest:

```text
ghcr.io/actions/actions-runner@sha256:f5a0d9a3d857315f2aed7075a02a29f46927ad198221c3b1c66585ae9fe36c0d
```

The approval also pins its local image config ID. That ID and the actual image
contents have not yet been measured on an approved runtime. Preflight requires
both IDs, Linux/ARM64 metadata and the exact repository digest to agree; it never
fetches missing image layers. See the separately reviewed canary assets for the
registry metadata provenance.

The invocation is fixed to entrypoint `/home/runner/bin/Runner.Listener`, command
`["run", "--once"]` and working directory `/home/runner`. Runner v2.337.0 reads
`ACTIONS_RUNNER_INPUT_JITCONFIG` through its supported command-input path. Its
listener supports `--once`, although that option is deprecated; this experiment
uses the pinned behavior to exit after one job. The image has no default runner
invocation, and its `run.sh` wrapper contains a restart loop, so that wrapper is
not used. No shell is involved in passing JIT.

The fixed Docker profile uses UID/GID `1001:1001`, 1 CPU, 1 GiB memory,
`MemorySwap=Memory`, 256 PIDs, all capabilities dropped and
`no-new-privileges:true`. It uses a writable disposable root filesystem, private
IPC/cgroup namespaces and the ordinary bridge network. There are no bind mounts,
image volumes, shared PID/IPC namespaces, extra groups, devices, published ports
or Docker socket. Healthchecks are disabled, restart policy is `no`,
`AutoRemove=false`, and the log driver is `none`. Standard masked/read-only
kernel paths are explicit. Before start or cleanup, the complete returned
configuration must match, including exactly one bridge attachment; missing/empty
network maps and unexpected nonzero configuration fields fail closed.
Real daemon normalization of this conservative profile is still unverified.

The image contains sudo and Docker client programs. Dropped capabilities,
no-new-privileges and absence of the daemon socket are part of the required
profile, not proof of hostile-code isolation. The ordinary bridge permits
outbound network access. Only the separately reviewed harmless private workflow
is within scope.

**A pinned image alone does not pin running runner code.** Approval requires
`runner_updates_disabled=true`, an operator/controller attestation that the
owned scale set was created and read back with `DisableUpdate=true` before JIT
issuance. The worker cannot query or prove that GitHub setting. The separate
controller integration and its evidence are required before actual bootstrap;
setting this boolean without that evidence is not a substitute.

## Runtime and secret boundaries

The helper connects directly to one approved absolute Unix socket. It ignores
Docker/environment contexts and proxies, rejects symlinks, requires ownership by
the controller's current UID and denies group/other write bits (`0022`). It accepts
read/execute bits such as mode `0755`: Apple's Unix-domain `connect(2)` contract
uses write access to the named socket. A read-only inventory observed `0755` on
the current Desktop endpoint; synthetic tests verify the permission rule, but
this helper has not contacted that daemon. No socket mode or owner is changed.
This is a private-controller ownership requirement, not proof against an
adversarial socket administrator or ACL changes.

The client pins the socket's file identity on its first preflight connection and
checks identity, owner and mode before every dial and again after connect, before
any HTTP request bytes can be sent. Replacement or permission changes close the
connection and refuse the request; replacement during a mutation retains the
durable uncertain intent without retries. A new command repeats preflight before
using a new socket instance. Keepalive reuse and redirects are disabled; a TCP
daemon is never used. Each phase verifies the approved daemon ID and Docker API
1.45 compatibility.
Preflight requires Linux ARM64, at least 2 configured CPUs and 2 GiB configured
memory, memory/swap/CPU/PID enforcement capabilities and no daemon warnings.
These metadata checks do not measure idle capacity, disk space or other running
workloads. The approved inventory must establish actual headroom separately.

Every HTTP operation has a 30-second timeout; the Unix connection timeout is
5 seconds. Execution after input is bounded by 10 minutes and approval expiry.
Create's standard-input wait is separately bounded by 30 seconds and expiry.
These command deadlines do not kill an already running worker or bound its idle
lifetime. A runner left waiting or busy stays reserved for explicit observation
and later approved resolution. No cancellation/force cleanup is inferred.

All success and error response bodies are capped at 2 MiB plus one overflow
detection byte, including decoded compressed responses. This accommodates the
at-most-1-MiB JIT value present in container inspection. Response headers are
capped at 64 KiB; image environment and labels are each capped at 64 KiB with
at most 64 entries. Malformed, oversized or failed mutation responses become
uncertain state without retries or raw output.

Only `ACTIONS_RUNNER_INPUT_JITCONFIG` is added to the verified image environment.
The helper never copies its own environment into the container. App keys,
installation tokens and workflow-read authority stay controller-side. The JIT
is transmitted in the local Docker API request body, not command arguments,
shell history or the journal. It remains sensitive in daemon container metadata,
process memory and runner-generated credential files; no secure-erasure claim
is made. Do not publish raw inspect bodies or runner diagnostics.

The journal records fixed phase/outcome names, IDs, approval digest and private
environment/label fingerprints. The environment fingerprint is secret-derived
correlation metadata, so the journal is not a public evidence artifact. Use a
controller-owned `0700` directory and a `0600` single-link regular journal.
The file is exclusively locked; intents/results and the directory entry are
synced before mutation, retrying directory sync on every reopen. The Driver.Run
authorizer holds an exclusive operation lease and rechecks current approval and
exact journal/directory ownership before preflight. Changed stable ownership,
torn tails, symlinks, hardlinks and permissive modes are rejected. Local Docker
administrators and processes sharing the controller UID remain trusted; labels
and filesystem modes are not an adversarial boundary against them.

The version1 header separates stable ownership from phase/expiry authority.
An explicit renewal must extend expiry and allow only `inspect` and `cleanup`;
it cannot authorize another create/start, reset attempts or clear uncertainty.
Renewal is appended durably before observation; superseded authority is refused.
All other approval fields, including harness, daemon, endpoint, image and nonce,
must remain unchanged. Legacy journals are retained/refused without migration;
no live legacy journals exist. This same restricted renewal contract also applies
to the separate controller journal. It never makes unresolved resources safe to
delete or confers authority merely because an earlier approval expired.

## Preparing a reviewable invocation

Live use requires separate approval of the exact reviewed harness/workflow SHAs,
private target, Unix endpoint, daemon ID, already prepared image ID, owner nonce,
resource inventory and resolution plan. This invalid example describes the
private approval schema and cannot authorize execution:

```json
{
  "runner_updates_disabled": false,
  "harness_sha": "REVIEWED_40_HEX_COMMIT",
  "workflow_sha": "REVIEWED_40_HEX_WORKFLOW_COMMIT",
  "owner_nonce": "SAME_32_HEX_EXPERIMENT_NONCE",
  "controller": "REVIEWED_CONTROLLER_IDENTIFIER",
  "endpoint": "/PRIVATE/APPROVED/docker.sock",
  "daemon_id": "OBSERVED_APPROVED_DAEMON_ID",
  "image_id": "sha256:OBSERVED_APPROVED_64_HEX_CONFIG_ID",
  "image": "ghcr.io/actions/actions-runner@sha256:f5a0d9a3d857315f2aed7075a02a29f46927ad198221c3b1c66585ae9fe36c0d",
  "expires_at": "2000-01-01T00:00:00Z",
  "phases": ["create", "start", "inspect", "cleanup"]
}
```

Store the completed approval in a controller-owned `0600` regular file. Expiry
must be in the next 24 hours. Each phase must be explicitly listed. Approval and
state locations are private and must not enter public issues or logs.

Build the reviewed commit in a clean **standalone clone**, with its `.git`
directory, and place the binary outside the checkout. The pinned Go toolchain
does not stamp VCS metadata in the observed linked-worktree layout; the command
refuses missing metadata, dirty builds and a mismatching SHA. From
`experiments/g01-scaleset` in that standalone clone:

```sh
GOTOOLCHAIN=go1.26.8 go build -buildvcs=true -trimpath -tags=g01_worker -o "$G01_PRIVATE_BINARY" ./cmd/g01-worker
GOTOOLCHAIN=go1.26.8 go version -m "$G01_PRIVATE_BINARY"
"$G01_PRIVATE_BINARY" --plan
```

Verify `vcs.revision` equals the approved 40-character commit, `vcs.modified=false`
and Go `go1.26.8`. The compile/plan commands above are offline. After actual
approval, each invocation has this shape, using approved private variable values:

```sh
"$G01_PRIVATE_BINARY" --execute-approved-worker --approval "$G01_PRIVATE_APPROVAL" --state-dir "$G01_PRIVATE_STATE" --phase "$G01_APPROVED_PHASE"
```

Only `create` reads standard input, as one strict JSON object with `jit_config`.
A separately reviewed controller broker must pipe the short-lived JIT directly;
do not paste it into a command or save it as a public artifact. No broker or
GitHub integration is implemented by this helper.

## Verification evidence and remaining gate

Red commit `450c30d` demonstrates mutation before durable ownership intent and
retry after uncertain creation. Synthetic regression tests now exercise the
actual Unix HTTP client, journal-before-effect ordering, lost responses,
oversized/error responses, redirect refusal, profile mutations, changed daemon
identity, missing containers, running-to-delete races and refusal to retry.
Private-file crash/restart/locking tests and tagged CLI secret-input/output tests
are included. A blocked input regression failed its timing assertion before the
bounded reader was implemented. All fixtures are isolated local HTTP servers;
they have no Docker backend and cannot pull images or start containers.

Independent review reproduced a socket replacement after preflight. Red commit
`f03555c` captured one request reaching the replacement for create/start/delete,
including synthetic JIT on create. The corrected client sends zero requests to
the replacement, preserves uncertainty and does not retry. Additional synthetic
checks cover replacement/permission changes between connect and transmission,
`0755` acceptance, `0757`/`0775` refusal and foreign-UID metadata refusal.

Default/tagged tests and independent source review are necessary preparation.
They do not establish actual image startup, resource enforcement, GitHub job
assignment, runner self-removal, cleanup completeness or protocol refresh/loss
semantics. Those observations require the still-pending, explicitly approved
private canary. Keep G01 open until its separate evidence plan is satisfied.

Primary contracts:

- [Runner v2.337.0 image definition](https://github.com/actions/runner/blob/v2.337.0/images/Dockerfile).
- [Runner command-input parsing](https://github.com/actions/runner/blob/v2.337.0/src/Runner.Listener/CommandSettings.cs) and [JIT/one-job listener behavior](https://github.com/actions/runner/blob/v2.337.0/src/Runner.Listener/Runner.cs).
- [Runner wrapper restart behavior](https://github.com/actions/runner/blob/v2.337.0/src/Misc/layoutroot/run.sh).
- [Docker Engine API 1.45](https://docs.docker.com/reference/api/engine/version/v1.45/) and [pinned Moby schema](https://github.com/moby/moby/blob/v26.1.5/api/swagger.yaml).
- [Moby non-force removal](https://github.com/moby/moby/blob/v26.1.5/daemon/delete.go) and [start/removal exclusion](https://github.com/moby/moby/blob/v26.1.5/daemon/start.go).
- [Apple Unix-domain connection access checks](https://developer.apple.com/library/archive/documentation/System/Conceptual/ManPages_iPhoneOS/man2/connect.2.html).
