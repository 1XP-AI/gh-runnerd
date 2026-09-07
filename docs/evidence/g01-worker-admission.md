# Bounded worker admission

This continues [G01](https://github.com/1XP-AI/gh-runnerd/issues/1) after the review
audit. A worker journal already prevented a second create in that directory,
but independently locked directories admitted separate containers. The finite
experiment requires one worker across the trusted controller account.

Red commit `495d078` records two concurrent approved state directories producing
two synthetic create calls. Further cases accepted another directory after the
first worker was created, deleted or left uncertain. The actual race test failed
in0.578s. A private admission entry seam delegated to the existing journal opener
in that red commit; it did not alter production admission behavior. The corrected
fixture canonicalizes macOS temporary-directory aliases because admission roots
must be canonical; its original behavioral failures remain preserved.

The implementation applies the previously reviewed controller admission pattern
to a distinct fixed worker claim: native effective-UID account home plus
`.gh-runnerd-g01-worker-experiment`. The operator must prepare its private root.
No approval, flag, environment variable or state directory selects it. Supported
native account lookup is required before journal preparation. The claim pins the
stable approval digest (only expiry/phases excluded for existing restricted
renewal) and exact state/journal device and inode. Its lifetime flock serializes
competing processes; file/root/parent synchronization is retried on every open.
Authorization rechecks named handles and claim content under the journal's
exclusive execution lease.

No outcome, close or process crash removes the claim. A later distinct experiment
requires separately designed and reviewed reconciliation; deleting state is not
a retry path. Existing trusted-UID/admin limitations apply. Separate controller
accounts are outside this account-scoped experiment cap. The independent
controller and worker pins do not by themselves prove their approvals share a
nonce or that GitHub work completed; the baseline integration must establish that
binding before live execution. This change does not implement production scaling
or close G01.

Verification uses synthetic runtimes and private filesystem fixtures only:
concurrent directories, created/deleted/unknown outcomes, owned reopen without
retry, copied journal refusal, unsafe/missing/tampered claims, file/directory
identity changes and repeated admission-directory sync failure. Unsupported
account lookup profiles are checked with `-tags=osusergo` and `CGO_ENABLED=0`.
No real account admission root, daemon, image, worker, credentials or workflow
was created or changed. Physical power-loss and actual worker lifecycle evidence
remain uncollected.

Author verification after the correction: liveworker race passed2.550s;
unsupported `osusergo` race guard passed1.405s and CGO-disabled guard passed0.919s.
Full `make check` passed, including both experiments and tagged commands, build,
vet/race, dependencies/licenses and the configured root vulnerability scan.
Fuzz explicitly skipped because the repository has no fuzz targets. Independent
review and hosted/external final-head gates are still required before merge.
