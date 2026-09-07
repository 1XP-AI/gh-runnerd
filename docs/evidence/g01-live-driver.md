# G01 controller phase driver

Status: **implemented and tested offline; no live authorization or live result**.
This is a controller slice of the [live-canary plan](g01-live-canary.md), not
completion of G01. The original fixture remains permanently loopback-only. The
separate command requires `-tags=g01_live` at build time, an explicit execution
flag, an exact private approval and controller-side broker input. Public CI has
no credentials. SDK `v0.4.0` and Go `1.26.8` remain pinned.

## Executable scope

One command executes one named phase. The driver never launches a worker, opens
Docker, pulls an image, dispatches/cancels/reruns a workflow, enrolls an App, calls
`RemoveRunner`, or invokes another executable. No environment variables supply
credentials. JIT is never printed, saved or forwarded. One stable worker name
and one unknown reservation exhaust the budget.

| Phase | Behavior |
|---|---|
| `create` | Verify private repository/group policy, record the initial runner-ID inventory, prove the nonce name absent, journal intent and create one scale set. Lost results quarantine; discovery never authorizes adoption/retry. |
| `before-ack` | Create one owned session, run the supported listener at capacity 1, verify a controlled job, stop before DELETE, then close only that known owned session. |
| `after-ack` | Stop after successful ACK, before acquisition/callbacks; record the barrier and close the known owned session. |
| `before-acquire` | ACK, then stop before acquisition POST; close the known owned session. |
| `acquire-loss` | Journal exactly one request ID/intent, call `AcquireJobs` once, record only a response success boolean, suppress its application result, retain session/reservation and quarantine. |
| `jit-loss` | Verify the stable worker name absent, journal one JIT intent, call once, record a response success boolean, discard JIT/result identity and quarantine the reservation. No worker starts. |
| `inspect` | Read owned statistics and the stable runner reference; record only assigned count/reference ID. No observation releases reservations or uncertainty. |
| `cleanup` | Delete only with an exact create receipt, nonce name/label and group, no JIT/acquisition attempt, no unresolved session/intent, all-zero statistics and an unchanged complete runner-ID inventory. Verify inventory again afterward. |

Each SDK/REST operation has a 30-second deadline; each phase is bounded by ten
minutes and approval expiry, whichever comes first. Each phase is one-shot. An
empty poll is **unresolved**, never a passed barrier. The high-level listener
retains upstream ACK ordering; fault barriers use its public client interface.
Adapter deadlines remain effective after the listener removes cancellation.

Known session closure is automatic only at a safe planned barrier. These probes
measure close/new-session behavior, **not crash restart of the same session**.
Rehydration and general session replacement remain unimplemented. A surviving
session receipt blocks subsequent mutations after restart. A changed session ID
also quarantines; the driver never deletes the replacement session.

## Authority split and its limits

Before scale-set mutation, read-only GitHub API checks require an installation
token scoped to exactly one matching private, non-fork repository and a
nondefault, noninherited runner group with selected-only visibility, public
repositories disallowed and exactly that repository selected.

Before ACK, a **separate controller-only Actions-read authority** verifies the
explicit workflow run ID, reviewed head SHA/path, first run attempt,
`workflow_dispatch` event and same private repository/head-repository IDs. This
does not broaden the G02 production App. Supply a separate temporary canary App
token or another separately approved read-only verification authority. Dispatch
remains outside the driver. The workload must be reviewed at that SHA; see the
inactive [workload asset](../../experiments/g01-canary-assets/README.md).

The trusted broker supplies issuance metadata: App/installation IDs, organization,
expiry and permission facts. Mismatches, expired tokens and a shared token for
both authorities are rejected. **These issuance facts are broker attestations.**
GitHub's installation repository API independently proves installation access
and repository scope, but does not introspect the token's App ID, installation
ID or complete permission set. The broker and its issuance record still need
review before execution; do not describe those IDs as independently observed.

The SDK constructor is named `NewClientWithPersonalAccessToken`; pinned source
passes its supplied token unchanged as Bearer authentication to the registration
endpoint. This driver supplies an existing installation token, not a PAT.
GitHub documents installation-token support for
[organization registration tokens](https://docs.github.com/en/rest/actions/self-hosted-runners#create-a-registration-token-for-an-organization)
and [installation repository access](https://docs.github.com/en/rest/apps/installations#list-repositories-accessible-to-the-app-installation).
This source/REST compatibility has not yet been exercised live here. The SDK
internally obtains temporary registration/Actions admin credentials even when
its first public method is a read; approval must include this controller-only
authentication flow. It never enrolls an App.

Transport permits only direct TLS to `api.github.com` or exact approved Actions
hostnames. No proxy, redirect, raw SDK error or retry logger is allowed. Unknown
hosts fail closed. HTTP retries are explicitly zero; the SDK's one queue-token
401 refresh/retry remains and is distinct from retrying a transport-ambiguous
effect. Public output uses fixed categories, never input/error values.

## Private approval and invocation

Use a new controller-owned directory with mode `0700`. The approval is a regular,
non-symlink, single-link file with mode `0600`. The placeholder below is invalid
and grants no authorization. Fill actual independently reviewed values and only
the approved phases; expiry must be within 24 hours. Changed approvals cannot
adopt an existing journal.

```json
{
  "app_id": 0,
  "installation_id": 0,
  "organization": "APPROVED_ORGANIZATION",
  "repository": "APPROVED_PRIVATE_REPOSITORY",
  "repository_id": 0,
  "runner_group_id": 0,
  "owner_nonce": "NEW_32_LOWERCASE_HEX_NONCE",
  "harness_sha": "REVIEWED_40_HEX_COMMIT",
  "workflow_sha": "REVIEWED_40_HEX_WORKFLOW_COMMIT",
  "workflow_path": ".github/workflows/canary.yml",
  "workflow_run_id": 0,
  "controller": "APPROVED_TRUSTED_CONTROLLER_ALIAS",
  "expires_at": "2000-01-01T00:00:00Z",
  "actions_hosts": ["EXACT_APPROVED_ACTIONS_HOST"],
  "phases": ["create", "before-ack", "inspect", "cleanup"]
}
```

The controller alias is maintainer attestation, not machine identity/isolation
enforcement. The controller account must be trusted. A same-UID attacker can
alter state; permissions/locks do not isolate hostile jobs. Keep approval,
journal and credentials outside repository/shared directories and worker mounts.

Build from a clean **standalone clone with a real `.git` directory**, detached
at the reviewed SHA, writing the binary outside the checkout. On the measured
Go 1.26.8 toolchain, a linked worktree's `.git` file is not recognized for VCS
stamping, even with `-buildvcs=true`; that binary correctly fails the live gate.
A local clone of the reviewed repository is sufficient and needs no live API.

```sh
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go build -buildvcs=true -trimpath -tags=g01_live -o "$G01_PRIVATE_BINARY" ./cmd/g01-live
"$G01_PRIVATE_BINARY" --plan
go version -m "$G01_PRIVATE_BINARY"
```

Live execution rejects a mismatched Go version, SDK pin, dirty build or embedded
VCS revision. Building, merging or running `--plan` is not authorization. The
build metadata must show the reviewed `vcs.revision` and `vcs.modified=false`.
This was verified offline in a clean local clone at `17a63ffc4604fbec4cc043abd5602a230d35fa8b`;
the linked-worktree build was correctly missing a usable revision stamp. The
reviewed controller broker (not implemented here) must provide this private JSON
on stdin without shell tracing, terminal echo, argv credentials or env dumps:

```json
{
  "installation_token": "PRIVATE_TEMPORARY_VALUE",
  "verification_token": "DISTINCT_PRIVATE_ACTIONS_READ_VALUE_IF_PROBING",
  "app_id": 0,
  "installation_id": 0,
  "organization": "APPROVED_ORGANIZATION",
  "expires_at": "2000-01-01T00:00:00Z",
  "organization_self_hosted_runners": "write",
  "metadata": "read"
}
```

After exact authorization, with broker stdin supplied, invoke one phase:

```sh
"$G01_PRIVATE_BINARY" --execute-approved-canary \
  --approval "$G01_PRIVATE_APPROVAL" --state-dir "$G01_PRIVATE_STATE" \
  --phase before-ack
```

Those variables contain paths, never tokens. Credential strings remain in Go/SDK
memory; clearing one buffer does not erase every copy. This slice has no worker
environment. The runner JIT transport and residual exposure remain as described
in ADR 0002 for later worker implementation.

## Journal and evidence

`journal.jsonl` is private controller state, not a public report. It contains raw
numeric request/resource IDs, counts, session UUIDs, operation ordering and
approval/inventory digests. An exclusive inode lock serializes processes. The
new file and directory entry are synced before effects; every intent/result is
synced. Torn tails, symlinks, hardlinks, permissive modes and changed approvals
fail closed without repair/truncation. Crash after intent or failed result write
retains uncertainty. Only publish manually reviewed aliases/sanitized timelines.

TDD red commit `889d5f3` compiled and failed creation without durable intent,
retry after ambiguous creation across restart, and failed authority bypass.
Green checks cover those cases plus pinned SDK listener barriers, one reservation,
locking/restart/torn tails, ownership, workload policy, secret-bearing failures,
empty cleanup, replaced sessions, TLS restrictions and offline CLI refusal. A
later failing check caught empty polls counted as success and missing distinction
between server receipt and application suppression; both were fixed.

Validation (all offline):

```sh
GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=45s ./...
GOTOOLCHAIN=go1.26.8 go test -race -tags=g01_live -count=1 -timeout=45s ./...
GOTOOLCHAIN=go1.26.8 go vet -tags=g01_live ./...
```

Actual commands/results and reviewed head are recorded in the focused PR. No
live phase has run. Worker baseline/JIT bootstrap, idle/busy assignment and
deregistration races, session restart/replacement, runtime ownership/cleanup and
sanitized server evidence remain outstanding. No override permits cleanup after
JIT/acquisition ambiguity. A separate exact owned-resource cleanup procedure
requires review/authorization; never delete the journal to obtain a retry.
G01 and dependent production integration remain unresolved.
