# G01 external-review corrections

This work tracks [issue30](https://github.com/1XP-AI/gh-runnerd/issues/30) and
references [G01](https://github.com/1XP-AI/gh-runnerd/issues/1). The audit reproduced
all six external findings from merged PR25/28 against main `f1fe70c`. The PR25
comments were stale by reviewed commit, but remained unresolved defects. No live
credentials, GitHub mutation, Docker operation or worker execution was used.

An adjacent cleanup regression is preserved at red commit `68eb032`: started-,
assigned- and completed-only messages previously took the empty-poll safe-close
path, leaving cleanup possible under stale zero. Unexpected work kinds are now
quarantined before testing for an empty available-job list. Genuine nil/empty
poll controls retain the intended no-message behavior; no terminal reconciliation
is inferred from these unexpected messages.

## Initial corrections

The immutable red commit `f6fc90b` preserves regressions for these findings:

- [P1: observed jobs allowed cleanup](https://github.com/1XP-AI/gh-runnerd/pull/25#discussion_r3949077101).
  Replay now retains every observed request ID. Cleanup refuses while any remain,
  including after a planned barrier closes its session and aggregate statistics
  report zero. This small harness has no terminal-job reconciliation; inspection
  does not release those requests.
- [P1: multi-job acquisition batch ACKed before refusal](https://github.com/1XP-AI/gh-runnerd/pull/25#discussion_r3949077092).
  Acquisition phases enforce exactly one available request before returning the
  message to the SDK listener, so the listener cannot ACK an invalid batch.
- [P2: missing bridge consumed a worker start](https://github.com/1XP-AI/gh-runnerd/pull/28#discussion_r3949487047).
  Profile verification requires exactly one bridge attachment. Empty and missing
  network maps fail closed; the synthetic runtime now models the required bridge
  for positive cases.

Red command: `GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=30s -run '^TestAuditPR(25Observed|25Multi|28Missing)' ./livecanary ./liveworker`
from `experiments/g01-scaleset`. It failed with one cleanup delete for each of
three unresolved-job barriers, one ACK for each invalid acquisition phase, and
one start for both missing-network cases.

After the initial corrections, fresh package race tests passed: livecanary1.616s
and liveworker2.014s. These are synthetic results and do not close the live gates.
Admission across directories is still required on this branch before final review.

## Directory-sync recovery

Red commit `b0c8059` reproduces the failed-directory-sync restart defect in both
worker and controller journals. An injected initial sync failure stops the first
open, but the former reopen path accepted its valid header without retrying the
failed directory sync. Both implementations now sync on every successful open,
including existing journals, before returning authority to perform effects.
Targeted fresh journal race tests passed for both packages. This is deterministic
failure-injection evidence, not a physical crash or power-loss experiment.

## Explicit recovery authority

Red commit `c2ae474` reproduces both blocked recovery renewal and a raw Driver
caller changing approval fields after journal open. The controller has the same
former whole-approval-digest trap as the worker; both are corrected here.

The new version1 journal header hashes stable ownership with only `expires_at`
and `phases` excluded. The initial authority and each later authority record
include the complete approval digest, expiry and phase list. A changed authority
must extend expiry and may contain only `inspect` and `cleanup`. The caller must
provide the new explicit, currently valid approval. Each renewal is synced before
the journal can authorize an operation. It never resets phase attempts, known
observations, ownership receipts or uncertainty; older authority cannot be reused
after renewal. Source/harness, endpoint/daemon, image, nonce and all other stable
fields remain fixed. Expired authority alone never authorizes recovery.

Both Journal interfaces now require an unexported authorizer at Driver.Run before
preflight. Its exclusive lease refuses concurrent calls on that FileJournal;
Close waits for the active operation. The authorizer checks the current approval,
still-owned journal inode and original directory identity. Mutating Driver fields,
closing the journal, or replacing the file/directory cannot bypass this check.
This is approved-code discipline in a trusted controller account, not isolation
from hostile Go callers or same-UID code.

Old journals without the versioned ownership header are refused and retained.
There is no migration or automatic adoption. No live controller/worker journals
have been created in this experiment. Explicit renewal does not make an unknown
resource safe to delete; the existing cleanup fences remain required.

Additional observed working-tree red cases caught a shared read lease admitting
concurrent phases and replaced journal/directory identities still authorizing
operations. The final exclusive lease and identity checks cover those cases.
Their tests and implementation are committed together, so these are not claims
of separate immutable red commits.
