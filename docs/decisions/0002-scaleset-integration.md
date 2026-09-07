# ADR 0002: Released Scale Set listener with independent reconciliation

Status: **provisional integration selection; G01 remains unresolved** pending
independent Astra protocol review and authorized live contract evidence.

Issue: [G01](https://github.com/1XP-AI/gh-runnerd/issues/1).
Checked: 2026-09-07. See [measured offline evidence](../evidence/g01-contract.md)
and [required live experiment](../evidence/g01-live-canary.md).

## Decision

Pin `github.com/actions/scaleset v0.4.0` (source
`6ce025902cd964747a078c2aabe7340ebc667eca`) behind an adapter. Use its high-level
listener plus independent reconciliation. Keep one serialized session owner per
pool. Do not copy the example shutdown logic or fork/reorder the ACK loop.

The released and audited implementations pass the same offline contract suite.
Their listener source is byte-for-byte unchanged: statistics are recorded, the
message is ACKed, available jobs are acquired, then started/completed/desired
callbacks run. The audited commit's lock changes and error sentinels do not
repair the delivery gap. Prefer the release over adding unreleased concurrency
changes to this initial contract. Revisit the pin on a reviewed release, a
relevant security advisory or a measured need; there is no lifetime compatibility
or support guarantee for this public-preview API.

The lower-level public `GetMessage`, `DeleteMessage` and `AcquireJobs` methods
are available, but moving ACK after callbacks does not prove acquisition retry,
JIT idempotency or durable job execution. This spike does not select a custom
protocol loop merely to reverse two calls.

## Recovery contract for later implementation

Callbacks are observations, not the source of truth. Independently read
`GetRunnerScaleSetByID().Statistics.TotalAssignedJobs` at startup, after listener
failure/session replacement, and on a bounded reconciliation schedule. Apply the
configured pool/global caps. Counts are snapshots, never event increments.

Persist non-secret creation intent and stable manager/pool/worker identity before
external creation. Journal message/request IDs and acquisition intent/results in
the adapter's supported client wrapper when observed; storage failure must stop
progress before the next external operation. This is a **future G04/G05/G07
requirement**, not implemented persistence in the experiment. The wrapper must
not claim that journaled IDs prove GitHub accepted or executed a job.

On restart, use supported runner ID/name reads and independently inspect only
verifiably owned provider resources. A matching runner reference can restore an
external ID, but it contains no busy/idle state or JIT secret. An absent reference
does not prove the local worker/process is gone. Retain unknown reservations and
quarantine ambiguous request IDs, missing callback history, uncertain acquisition
results, lost JIT results and ownership mismatches. Never automatically rerun a
workflow, mint replacement JIT under a different identity, or infer exactly-once
execution. Restoring desired demand is distinct from restoring lost job identity.

The production adapter must fence observations by a local session generation,
discard late responses from replaced sessions, and reset cursor state for a
newly created session. Message IDs are not a global ordering clock. A 202 has no
new statistics; the listener reuses its last snapshot. Even a later GET has no
server revision in the SDK statistic type. No stale observation may authorize
deletion; periodic reads provide convergence candidates, not linearizability.

Use bounded operation deadlines and sanitized error categories. The listener
removes cancellation while processing a message, so callbacks need their own
deadline. Keep SDK/retry logging discarded; raw SDK errors include URL/body
details. Configure retry explicitly using the SDK's public HTTP options: do not
blindly retry uncertain side-effecting JIT/acquisition requests. The experiment
uses `WithRetryableHTTPClint` with HTTP `RetryMax=0`; SDK queue-token 401 refresh
still retries once. Production 429/permission handling, backoff and safe status
classification require adapter tests, not raw error-string parsing.

## Drain and bootstrap constraints

Setting maximum capacity to zero affects the next poll; the offline barrier test
shows an existing poll can still ACK/acquire work. GitHub also documents automatic
assignment to idle registered runners. Neither an old idle callback nor aggregate
zero is a safe deletion signal. Stop new local admission, retain busy/unknown
workers, and keep polling/reconciling existing work. A safe deregistration versus
assignment barrier must be proven live before idle drain can be implemented.

Use the pinned runner's supported `ACTIONS_RUNNER_INPUT_JITCONFIG` input for the
first canary, injected only into its worker at launch. Do not pass management
credentials to the worker. This avoids JIT in argv but does not eliminate secret
exposure: the initial environment/process memory, container configuration if
used, and runner-written credential files remain sensitive. The runner removes
the environment variable during parsing; that does not erase all original
copies. No stdin, FD or arbitrary secret-file input is established. The exact
runner pin and source evidence are in the evidence record.

## Consequences and rollback

The isolated experiment may be reviewed/merged as evidence while G01 stays open;
it does not authorize dependent production integration. Future callers need a
journal, ownership checks, bounded retries, generation fencing and live evidence.

Rollback of this spike is reverting its isolated module and documents. It has no
production state or resources. For the canary, pause new admission, allow the
owned running job to finish, quarantine uncertainty and remove only individually
verified disposable resources. Preserve existing manual runners throughout.
