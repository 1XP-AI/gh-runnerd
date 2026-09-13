# G01 reusable recovery evidence packet

Date: 2026-09-13. Issue: [G01 / #1](https://github.com/1XP-AI/gh-runnerd/issues/1).
Status: **selected path recorded; offline/source evidence only; live gate
unresolved**. This packet is a sanitized index and changed-boundary ledger. It
does not replace the linked records, authorize a canary, or claim production
completion.

The active G01 Goal is:

> Select and pin one supported Scale Set integration path and produce a reusable
> evidence packet for recovery at message acknowledgement, acquisition and JIT
> boundaries, rerunning only changed-boundary checks while keeping unchanged
> evidence and live gaps explicit.

## Selected path

Pin `github.com/actions/scaleset v0.4.0` at source commit
`6ce025902cd964747a078c2aabe7340ebc667eca` behind an adapter. The selected path
is a future production contract: use the released high-level listener with
independent reconciliation, keep one serialized session owner per pool, and
preserve the upstream message order:

```text
statistics observation -> message ACK (DeleteMessage) -> available-job acquisition -> lifecycle callbacks
```

The packet records the SDK's order; it does **not** reorder ACK after callbacks
or acquisition, and it does not claim exactly-once delivery, idempotent
acquisition/JIT, durable execution, linearizability or server-side receipt.
Callbacks are observations. The future production recovery contract is to read
`TotalAssignedJobs` at startup, after listener failure/session replacement and
on a bounded schedule, with session-generation fencing and conservative
ownership/quarantine rules. The current offline experiment does not implement
that full path: `recovery.go` performs one synthetic read and explicitly defers
persistence, freshness fencing, caps and provider ownership validation to later
gates, while rehydration and general session replacement remain unimplemented
in the driver. Bounded fixture rules preserve a later zero or absent
observation from erasing an earlier work-bearing or uncertain observation; that
is not a live or production reconciliation result.

## Classification and evidence index

Use these classifications throughout the packet:

- **Source** — upstream code or documentation at a pinned URL. It establishes
  what the inspected implementation or documented API says, not live service
  behavior.
- **Fixture** — an offline synthetic HTTP, TLS, Unix-socket, subprocess or file
  test. It can exercise the pinned client and local recovery code, but fixture
  queue/response/commit behavior is not a GitHub service result.
- **Live** — an authorized private GitHub/runner/Docker observation. No live
  result is recorded in this packet.

| Existing record | Classification and reusable scope | Immutable evidence reference |
|---|---|---|
| [ADR 0002](../decisions/0002-scaleset-integration.md) | Source-backed decision and future recovery contract; G01 remains provisional. | [d0afc56d471e8d63d153f6d26aeb95915d141d87](https://github.com/1XP-AI/gh-runnerd/blob/d0afc56d471e8d63d153f6d26aeb95915d141d87/docs/decisions/0002-scaleset-integration.md) |
| [G01 contract](g01-contract.md) | Source + fixture: ACK/acquisition/statistics/error/JIT boundaries, SDK and runner pins. | [ba113977d03fc209ca147edbe10b766eed2c0fe3](https://github.com/1XP-AI/gh-runnerd/blob/ba113977d03fc209ca147edbe10b766eed2c0fe3/docs/evidence/g01-contract.md) |
| [G01 red evidence at historical commit `1396e201d905be204c3ac697be43723820581314`](https://github.com/1XP-AI/gh-runnerd/blob/1396e201d905be204c3ac697be43723820581314/docs/evidence/g01-red.md) | Fixture: pre-fix ACK/callback-loss failures; no live result. | Immutable file blob `c36e0af0c8e9b301f4889a02454c83ece8d5942f`. |
| [JIT causality follow-up](g01-jit-causality.md) | Fixture: committed-vs-requested synthetic JIT creation and response-loss controls; no live bug claim. | [ba113977d03fc209ca147edbe10b766eed2c0fe3](https://github.com/1XP-AI/gh-runnerd/blob/ba113977d03fc209ca147edbe10b766eed2c0fe3/docs/evidence/g01-jit-causality.md); causal fix [7079b255d09ce0456b5843a8695277689d52e54e](https://github.com/1XP-AI/gh-runnerd/commit/7079b255d09ce0456b5843a8695277689d52e54e) |
| [Live-canary plan](g01-live-canary.md) | Live plan and authorization checklist; every phase remains unchecked/not run. | [d0afc56d471e8d63d153f6d26aeb95915d141d87](https://github.com/1XP-AI/gh-runnerd/blob/d0afc56d471e8d63d153f6d26aeb95915d141d87/docs/evidence/g01-live-canary.md) |
| [Controller phase driver](g01-live-driver.md) | Fixture/source: bounded tagged controller operations; no worker launch or live result. | [controller driver evidence revision `0e08ce8285846e26a94fb6fcb33bbeede662bf14`](https://github.com/1XP-AI/gh-runnerd/blob/0e08ce8285846e26a94fb6fcb33bbeede662bf14/docs/evidence/g01-live-driver.md) |
| [Idle-drain observation](https://github.com/1XP-AI/gh-runnerd/blob/f5560ba950f77343e57034cc1cf85dc67f5ac922/docs/evidence/g01-idle-drain.md) | Fixture + source at the authoritative current PR #72 head; PR #72 is open with no checks reported, and this is offline evidence only. | [PR #72 head `f5560ba950f77343e57034cc1cf85dc67f5ac922`](https://github.com/1XP-AI/gh-runnerd/commit/f5560ba950f77343e57034cc1cf85dc67f5ac922) |
| [Identity/reconciliation](g01-identity-reconciliation.md) | Source + fixture limits for runner, request, job, REST and terminal identity; no live equality claim. | [9fe1b43cceb10c3ce92b26f8262c72148ded6c60](https://github.com/1XP-AI/gh-runnerd/blob/9fe1b43cceb10c3ce92b26f8262c72148ded6c60/docs/evidence/g01-identity-reconciliation.md) |
| [Exact observation adapter](g01-exact-observations.md) | Fixture: bounded exact-ID/status/provenance readers; no affirmative live recovery or cleanup. | [d62242b07b779ff6f7414e97096d2c8828634a5d](https://github.com/1XP-AI/gh-runnerd/blob/d62242b07b779ff6f7414e97096d2c8828634a5d/docs/evidence/g01-exact-observations.md) |
| [Worker/JIT harness](g01-worker-harness.md) | Source + fixture: worker profile and secret boundary; no image startup, JIT mint, workflow or Docker result. | [c1c0b6a6f2f94651546f533576eb3bbe8a4089b4](https://github.com/1XP-AI/gh-runnerd/blob/c1c0b6a6f2f94651546f533576eb3bbe8a4089b4/docs/evidence/g01-worker-harness.md) |
| [Paired collection](g01-paired-baseline.md) and [terminal path](g01-paired-terminal.md) | Fixture: serialized controller/worker receipts and terminal correlation; no live cleanup or success claim. | [8dd64adc551ba5174892807a678e8bc614d0a474](https://github.com/1XP-AI/gh-runnerd/blob/8dd64adc551ba5174892807a678e8bc614d0a474/docs/evidence/g01-paired-baseline.md), [cf67d4aeb511116fee0de31a4ac38409f28fa29f](https://github.com/1XP-AI/gh-runnerd/blob/cf67d4aeb511116fee0de31a4ac38409f28fa29f/docs/evidence/g01-paired-terminal.md) |
| Current source anchor | This packet is based on clean `origin/main` at the immutable source tree below. | [dce795a871865a8a2ee728151cb161e55081c8b7](https://github.com/1XP-AI/gh-runnerd/commit/dce795a871865a8a2ee728151cb161e55081c8b7) |

The idle-drain link intentionally points at the authoritative current PR #72
head and is not copied into this base worktree. PR #72 is currently open with no
checks reported (GitHub reports merge state `CLEAN`); do not turn its offline
result into a live result or claim that PR #72 is mergeable from this packet.

## Exact pins and transport choices

| Boundary | Exact selection | Classification / exposure limit |
|---|---|---|
| Scale Set SDK | `github.com/actions/scaleset v0.4.0`; tag/source `6ce025902cd964747a078c2aabe7340ebc667eca`; [release](https://github.com/actions/scaleset/releases/tag/v0.4.0), [listener source](https://github.com/actions/scaleset/blob/6ce025902cd964747a078c2aabe7340ebc667eca/listener/listener.go), [module](https://github.com/actions/scaleset/blob/6ce025902cd964747a078c2aabe7340ebc667eca/go.mod) | Source + fixture pin. The module declares Go `1.25.3`; the audited comparison is not the selected release. |
| Go toolchain | Experiment module directive `go 1.26.3`; verification toolchain `GOTOOLCHAIN=go1.26.8` / `go1.26.8` | Fixture/toolchain evidence only; every rerun records the exact toolchain. |
| Runner | `v2.337.0`, source `397b032cbf865e9c3ddfab89d533ec19325e1273`; [release](https://github.com/actions/runner/releases/tag/v2.337.0), [command parser](https://github.com/actions/runner/blob/397b032cbf865e9c3ddfab89d533ec19325e1273/src/Runner.Listener/CommandSettings.cs), [JIT materialization](https://github.com/actions/runner/blob/397b032cbf865e9c3ddfab89d533ec19325e1273/src/Runner.Listener/Runner.cs) | Source pin; no runner binary was downloaded or executed by the contract evidence. |
| JIT bootstrap | Select worker-only `ACTIONS_RUNNER_INPUT_JITCONFIG` as parsed by runner `v2.337.0`; [environment parsing](https://github.com/actions/runner/blob/397b032cbf865e9c3ddfab89d533ec19325e1273/src/Runner.Listener/CommandSettings.cs) | Source-supported fallback, with no separate transport version. It avoids argv but remains sensitive in initial environment, process memory, container metadata and runner-written files. `--jitconfig` exposes argv; stdin/FD/file transport is not established and is not claimed. |
| REST observation | GitHub REST API header `2022-11-28` for exact runner/job reads; Scale Set acquisition uses SDK's `api-version=6.0-preview` | Source/fixture request contract, not live acceptance or freshness evidence. |
| Worker runtime support | Docker Engine API `1.45`, Moby schema `v26.1.5`, and the worker-harness image reference `ghcr.io/actions/actions-runner@sha256:f5a0d9a3d857315f2aed7075a02a29f46927ad198221c3b1c66585ae9fe36c0d` | Supporting fixture profile only; actual daemon/image normalization and runner startup remain live gaps. |

### Runner pin/source-audit gate

The Runner parser and JIT materialization are part of the pinned source
contract, not evidence supplied by the local Go fixtures. Any proposed Runner
version or source-commit change opens a mandatory immutable source-audit gate:
inspect `CommandSettings.cs` and `Runner.cs` at the candidate commit, record
that commit and the two immutable upstream URLs, and compare the accepted
parser/input modes and JIT materialization behavior with this packet's
transport boundary. Local Go fixture results may supplement that audit but
cannot be reused as its substitute; a changed Runner pin remains an open
source gate until the audit (and any required Runner-level fixture) is recorded.

The current correction preserves Runner `v2.337.0` at source commit
`397b032cbf865e9c3ddfab89d533ec19325e1273`; no Runner source audit, binary
execution or Runner-level fixture result is claimed or required for this
documentation-only selector correction.

Published runner archive digests retained from the contract record are
`9b1dc70626422526e3c94767cf024896beb15da5342a3f4819bf2feac13e0393`
(Linux ARM64) and
`5a2cd92908a93d7276a194e1de6008099f3e7946f3f8e14aa7a1a7b4a31fdec2`
(macOS ARM64). Verify bytes again before any trusted execution; a digest or
version pin alone does not prove the running binary or disable runner updates.

## Requirement/evidence matrix

| Requirement / boundary | Reusable evidence | Classification | Current disposition and limit |
|---|---|---|---|
| Message acknowledgement order | `g01-contract.md`: `TestSDKACKBoundaries`, `TestRecoveryAfterACKCallbackCrash`; pinned listener source above. | Source + fixture | Offline evidence observes ACK before callback and before acquisition. Preserve this order; do not infer GitHub redelivery/retention, server receipt or durable exactly-once behavior. Live pre/post-ACK and session-restart behavior require authorization. |
| ACK/callback loss recovery | `g01-contract.md` and [G01 red evidence at historical commit `1396e201d905be204c3ac697be43723820581314`](https://github.com/1XP-AI/gh-runnerd/blob/1396e201d905be204c3ac697be43723820581314/docs/evidence/g01-red.md) (immutable blob `c36e0af0c8e9b301f4889a02454c83ece8d5942f`); ADR 0002 recovery contract. | Fixture + source | Independent statistics reads can restore aggregate demand; missing callback/request identity remains quarantined. No process-kill/restart or durable production journal result is claimed. |
| Acquisition intent and response loss | `TestSDKAcquisitionResponseLossAfterACK`; driver `before-acquire`/`acquire-loss`; [live-canary acquisition phase](g01-live-canary.md#minimal-execution-phases). | Source + fixture + live plan | ACK precedes exactly one `AcquireJobs` request; a suppressed response retains reservation and quarantines. No retry, server reoffer, accepted-request read-back or idempotency claim. A live disposable acquisition observation is still gated. |
| JIT intent and response loss | [JIT causality follow-up](g01-jit-causality.md), `TestSDKJITLookupBeforeCreationDoesNotDiscoverIdentity`, `TestSDKJITResponseLossWithoutCommitDoesNotDiscoverIdentity` and committed response-loss control. | Fixture + source | Synthetic lookup is causally tied to fixture-side commit; JIT value/result identity is discarded and reservation quarantined. No JIT-secret recovery, duplicate-name guarantee, idempotency or real runner launch is claimed. |
| Reconciliation | ADR 0002; [identity evidence](g01-identity-reconciliation.md); exact observation and baseline listener records. | Source + fixture | Read `TotalAssignedJobs` at bounded recovery points, fence session generations, and keep counts as snapshots. A matching ID is only a candidate; no revision, atomic cross-service snapshot or linearizability is established. |
| Quarantine and ownership | [controller driver](g01-live-driver.md), [audit corrections](g01-audit-corrections.md), [worker admission](g01-worker-admission.md). | Fixture + source | Unknown create/session/request/JIT/acquisition/callback outcomes, foreign identities, missing/invalid stats and stale work retain reservations and stop new effects. Name, label, zero count or elapsed time never authorizes adoption/deletion. |
| Idle assignment and drain | [idle-drain evidence at PR #72 head](https://github.com/1XP-AI/gh-runnerd/blob/f5560ba950f77343e57034cc1cf85dc67f5ac922/docs/evidence/g01-idle-drain.md); [live-canary drain phases](g01-live-canary.md#minimal-execution-phases). | Source + fixture + live gap | Offline hook uses the released listener's `SetMaxRunners(0)` and retains ACK-before-acquisition; client-side request markers do not prove server receipt or an atomic drain. Real old-poll acquisition/idle assignment and busy-safe removal remain unverified. |
| Terminal identity and completion | [identity reconciliation](g01-identity-reconciliation.md), [exact observations](g01-exact-observations.md), [paired baseline](g01-paired-baseline.md) and [terminal path](g01-paired-terminal.md). | Source + fixture | Keep SDK request/job IDs, REST job/runner IDs, run attempt, runner identity and local exit distinct; bind an exact tuple before classifying. No live job eligibility, successful execution, per-request release or cleanup proof is claimed. |
| Secret and error boundary | Contract runner/JIT section, [worker harness](g01-worker-harness.md), and driver redaction rules. | Source + fixture | No secrets, JIT values, raw bodies or raw SDK errors belong in this packet. Environment transport remains exposed to trusted same-user/process/container surfaces; modes and same-user ownership are not hostile-code isolation. |
| Rollback and preservation | ADR 0002 and [live-canary exit/cleanup](g01-live-canary.md#exit-evidence-and-cleanup). | Source + plan | Base `dce795a871865a8a2ee728151cb161e55081c8b7` lacks this added packet path, so rollback is to close/delete the unmerged correction or run `git rm -- docs/evidence/g01-recovery-packet.md` in a review branch. Reserve `git restore --source=<reviewed-parent> -- docs/evidence/g01-recovery-packet.md` for restoring an existing parent file; preserve independent corrections in `g01-live-driver.md` and `docs/reviews/team-review.md`. For any future live run: stop new admission, let owned busy work finish, quarantine uncertainty, and remove only individually verified disposable resources; preserve manual runners and never force-kill, prune or replay. |

No row above is a live result. In particular, this packet makes no exactly-once
or idempotency claim.

## Live gaps and authorization gates

The following are explicit open gates, not implied by the fixture records:

1. Obtain an independent `gpt-luna-max` protocol/recovery review of the exact
   candidate source and this packet. New work follows the current
   `gpt-luna-max` route; historical Astra authorship is not current routing.
2. Name one disposable private repository, selected-only runner group,
   reviewed workflow/run attempt/source/head/path, owner nonce and immutable
   harness/workflow commits. Record an unchanged manual-runner inventory.
3. Approve distinct controller and Actions-read authorities, exact App/org
   installation permissions, broker issuance metadata and expiry. Keep
   credentials outside commits, logs, worker input and public evidence.
4. Approve one scale set, one serialized session owner, one active worker,
   bounded jobs/CPU/memory/deadlines, pinned SDK/Go/runner/JIT transport and
   the worker runtime/image. A native macOS run additionally needs trusted
   execution approval; trusted same-user execution is not hostile-code
   isolation.
5. Explicitly authorize create, ACK/callback fault barriers, acquisition/JIT
   response suppression, workflow execution, manager interruption, idle-drain
   assignment/removal probes and owned cleanup. The offline fixture must never
   be pointed at GitHub by changing its endpoint.
6. Publish sanitized live results for ACK/redelivery, acquisition acceptance or
   reoffer, JIT discoverability, session replacement, runner/job identity,
   idle assignment, busy-safe deregistration and cleanup. Until then, G01 and
   dependent production integration remain unresolved.

## Changed-boundary ledger

This ledger is the reuse rule for future evidence. **Reuse** means cite the
immutable record as historical evidence and label it reused; it never means
that a historical fixture pass became a current or live pass. **Rerun** when
the selected SDK/source, Go toolchain/module, listener/adapter, journal or
authority code, fixture endpoint/decoder, test tags/selectors, runner/parser,
JIT transport/image, or live target/approval changes. A documentation-only
wording/link correction does not invalidate an unchanged offline boundary.
The current table contains **10 data rows** (header and separator excluded);
the count includes the actual live-canary gate row.

| Boundary | Reuse unchanged evidence when | Exact focused rerun | Class/result to record |
|---|---|---|---|
| ACK, callback loss and acquisition order | SDK commit, listener source, adapter call order and root protocol fixtures are unchanged. | See the exact ACK/callback selector in the command block below, including `TestNoMessageDoesNotCountAsCompletedBarrier`. | Fixture: record ACK-before-acquisition and reservation/quarantine behavior; do not add server or exactly-once claims. |
| Baseline listener/admission | listener, strict source reader, journal/lease, caps and selectors are unchanged. | See the exact baseline and admission selectors in the command block below, including the worker admission integrity suite and the tagged `TestUnsupportedAccountLookupRefusesBeforeJournal` selector. | Fixture: record actual test result and pinned SDK; no live result or production worker claim. |
| Runner parser/JIT causality and transport | Runner version/source commit, parser/materialization behavior, JIT transport, causal fixture and response-loss controls are unchanged. | A Runner pin or source-commit change first requires the immutable source-audit gate above; then rerun the root-package and livecanary selectors in the command blocks below, with tagged worker input/refusal and worker transport selectors as applicable. Local Go fixture results cannot substitute for the source audit. | Source + fixture: record parser/materialization comparison, one-request controls and quarantine; never print or retain JIT. |
| Controller update policy | pinned SDK update-setting request, confirmation and drift guards are unchanged. | See the exact `./livecanary` update-policy selectors in the worker/runtime command block below: `TestCreateRequestsDisabledRunnerUpdate`, `TestUnconfirmedUpdateSettingQuarantinesCreate`, `TestUpdateSettingDriftStopsBeforeSessionOrJIT` and `TestUpdateSettingDriftDoesNotBlockSafeEmptyCleanup`. | Source + fixture: record request/confirmation and pre-session/JIT refusal; no live service-setting result. |
| Reconciliation, inventory/identity/quarantine | statistics readers, paged inventory normalization, generation fences, ownership checks and journal/authority schema are unchanged. | Retain the prior reconciliation selector and add the exact focused `TestObservation*` and `TestRoster*` families, inventory/quarantine and journal/authority selectors in the command block below, including controller-side `TestAmbiguousCreateNeverRetriesAfterRestart`, the four `TestInventory*` cases, `TestDemandStatisticsAllowControlledProbeButNeverCleanup`, `TestCleanupOnlyForNeverIssuedWorkerWithExactReceipt`, `TestWorkerPreparationReturnsCanonicalSnapshotAndRejectsPriorEffect`, and the named journal/authority contracts in both `./livecanary` and `./liveworker`. | Fixture: record normalized facts and retained uncertainty; no live equality/absence claim. |
| Secret and error handling | SDK error sentinels, HTTP body/debug redaction and shared response-budget controls are unchanged. | See the exact root, livecanary and liveworker selectors in the secret/error command block below, including decoded-body truncation, gzip-budget, uncertain-start, bounded Unix-response/redirect, journal-create-failure and raw-runtime-status tests. | Fixture: record normalized sentinel/error and body-budget behavior; no secret-bearing output or live result is claimed. |
| Tagged controller/JIT input and credential transport boundary | `g01_live` source, input reader, credential attestation and transport refusal tests are unchanged. | Run the tagged command below plus the livecanary credential/transport selector below, including `TestAuthoritySplitAndPolicyRejection`, `TestSDKTransportOwnership`, `TestCredentialAttestationMismatchAndExpiredTokenRejected` and `TestTransportRejectsPlaintextOffHostAndProxyBeforeNetwork`. | Fixture: record no-secret/no-echo/refusal result; no credential or live phase. |
| Idle drain and withdrawal | Only reuse the authoritative current PR #72 head `f5560ba950f77343e57034cc1cf85dc67f5ac922` and its unchanged fixture/source. | On that authoritative current PR #72 checkout, run the guarded `drain-pr72` and `drain-pr72-race` commands below; both carry the same exact selector/package digest, with `default+norace+go1.26.8` and `default+race+go1.26.8` metadata respectively. | Fixture/source: record physical-write markers as client facts and inconclusive server receipt; never reuse as live assignment/drain evidence. |
| Paired terminal/worker support | paired journal/lease, worker profile, `liveworker` runtime source/implementation, image/runtime pins and terminal selectors are unchanged. | See the exact controller update-policy and worker runtime commands plus the tagged partition commands below; the worker runtime selector includes the direct Docker state/inspection, mutation-EOF and changed-daemon contracts, while tagged partition commands retain `go1.26.8`, `-race`, `-count=1` and `-timeout=120s`, the complete worker `^TestPaired` partition is included, and the controller-side terminal groups remain exhaustive/disjoint. | Fixture: record receipt/identity checks only; no live worker or terminal success claim. |
| Actual live canary | There is no live evidence to reuse today. A future result is reusable only for the same immutable workflow/run attempt, source/head, resources, authority scope and approved observation boundary. | Rebuild/plan the exact reviewed tagged binary, then run only the explicitly authorized phase from [the driver](g01-live-driver.md); never substitute fixture commands or broaden phases. | Live: record sanitized server observations, authorization and unresolved outcomes; any changed target or boundary requires a fresh approval/rerun. |

The exact focused commands referenced above are recorded here. They are
prescriptions for future reruns, not completed results. None of these
prescriptions reports a completed test or live server success/receipt.

Every runnable test prescription below uses the reusable wrapper in this first
block. It derives a list-only command from the same package, build flags and
selector, then compares the observed count and SHA-256 of the sorted executed
test-name set before it invokes the original command. Go's `-list` mode reports
the pre-`-skip` candidates, so the wrapper removes either `-skip REGEXP` or
`-skip=REGEXP` from the first list probe, asks Go's own RE2 regexp engine for
the names matched by that skip expression, and subtracts those names before
count and digest validation. Each prescription also carries an expected
package identity (`module-directory:package`) and build configuration (tag set,
race mode and `GOTOOLCHAIN`); the wrapper queries the effective `go env
GOFLAGS`, including GOENV/configuration, rejects non-empty output, and then
pins `GOFLAGS=` for both list probes and the original command. The wrapper
recomputes both from the original command and fails closed before listing if
either differs. A command failure, unexpected list output, zero expected names,
invalid Go/RE2 syntax, count mismatch or set mismatch stops before any test body
runs; the set digest and metadata are recorded beside each prescription so
renamed, removed, build-tagged, newly unskipped or cross-package tests fail
closed.

```sh
set -euo pipefail

# The digest is SHA-256 of sorted test names joined with one trailing newline.
# Invocation metadata is: package ID module-dir:package, then build ID
# tags+race-mode+toolchain (for example,
# experiments/g01-scaleset:./livecanary default+race+go1.26.8).
# The wrapper lists with the same build flags before it runs the original command.
go_test_checked() {
  python3 - "$@" <<'PY'
import hashlib
import os
import re
import subprocess
import sys
from pathlib import Path

if len(sys.argv) < 7:
    raise SystemExit(
        "usage: go_test_checked COUNT SET_SHA LABEL PACKAGE_ID BUILD_ID "
        "ENV=VALUE... go test ..."
    )
expected_count = int(sys.argv[1])
expected_digest = sys.argv[2]
label = sys.argv[3]
expected_package = sys.argv[4]
expected_build = sys.argv[5]
command = list(sys.argv[6:])
if expected_count <= 0 or not re.fullmatch(r"[0-9a-f]{64}", expected_digest):
    raise SystemExit(f"{label}: invalid expected count/digest")
if not re.fullmatch(
    r"experiments/g01-scaleset:(?:\.|\./[A-Za-z0-9._/-]+)",
    expected_package,
):
    raise SystemExit(f"{label}: invalid expected package identity")
build_parts = expected_build.split("+")
if len(build_parts) != 3:
    raise SystemExit(f"{label}: invalid expected build configuration")
expected_tags, expected_race, expected_toolchain = build_parts
if expected_tags != "default" and not re.fullmatch(
    r"[A-Za-z0-9_]+(?:,[A-Za-z0-9_]+)*", expected_tags
):
    raise SystemExit(f"{label}: invalid expected build tags")
if expected_race not in {"race", "norace"}:
    raise SystemExit(f"{label}: invalid expected race mode")
if not re.fullmatch(r"go[A-Za-z0-9._-]+", expected_toolchain):
    raise SystemExit(f"{label}: invalid expected toolchain")

invocation_root = Path.cwd().resolve()
repo_root = Path(
    subprocess.check_output(
        ["git", "rev-parse", "--show-toplevel"],
        cwd=invocation_root,
        text=True,
    ).strip()
).resolve()
if invocation_root != repo_root:
    raise SystemExit(f"{label}: run this selector from the repository root")

env = dict(os.environ)
while command and re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*=.*", command[0]):
    key, value = command.pop(0).split("=", 1)
    env[key] = value
if len(command) < 3 or command[:2] != ["go", "test"]:
    raise SystemExit(f"{label}: expected a go test command")

effective_goflags = subprocess.run(
    ["go", "env", "GOFLAGS"],
    cwd=repo_root,
    env=env,
    text=True,
    capture_output=True,
    check=False,
)
if effective_goflags.returncode != 0 or effective_goflags.stderr.strip():
    raise SystemExit(f"{label}: effective GOFLAGS query failed")
effective_lines = effective_goflags.stdout.splitlines()
if len(effective_lines) > 1:
    raise SystemExit(f"{label}: effective GOFLAGS query was not single-line")
if effective_lines and effective_lines[0].strip():
    raise SystemExit(f"{label}: effective GOFLAGS must be empty")
env["GOFLAGS"] = ""

test_args = command[2:]

def flag_values(args, flag):
    values = []
    index = 0
    while index < len(args):
        value = args[index]
        if value == flag:
            if index + 1 >= len(args):
                raise SystemExit(f"{label}: {flag} requires a value")
            values.append(args[index + 1])
            index += 2
            continue
        if value.startswith(f"{flag}="):
            values.append(value[len(flag) + 1:])
        index += 1
    return values

module_values = flag_values(test_args, "-C")
if len(module_values) != 1:
    raise SystemExit(f"{label}: expected one -C module directory")
module_dir = module_values[0]
if module_dir != "experiments/g01-scaleset":
    raise SystemExit(f"{label}: unexpected module directory {module_dir!r}")

tag_values = flag_values(test_args, "-tags")
if len(tag_values) > 1:
    raise SystemExit(f"{label}: expected at most one -tags flag")
if tag_values:
    tags = tag_values[0].split(",")
    if not tags or any(not re.fullmatch(r"[A-Za-z0-9_]+", tag) for tag in tags):
        raise SystemExit(f"{label}: invalid -tags value")
    if len(tags) != len(set(tags)):
        raise SystemExit(f"{label}: duplicate build tag")
    tag_identity = ",".join(sorted(tags))
else:
    tag_identity = "default"

race_modes = []
for value in test_args:
    if value == "-race":
        race_modes.append("race")
    elif value.startswith("-race="):
        race_value = value[len("-race="):]
        if race_value == "true":
            race_modes.append("race")
        elif race_value == "false":
            race_modes.append("norace")
        else:
            raise SystemExit(f"{label}: invalid -race value")
if len(race_modes) > 1:
    raise SystemExit(f"{label}: expected at most one -race flag")
race_identity = race_modes[0] if race_modes else "norace"
toolchain_identity = env.get("GOTOOLCHAIN", "")
actual_build = f"{tag_identity}+{race_identity}+{toolchain_identity}"
if actual_build != expected_build:
    raise SystemExit(
        f"{label}: expected build {expected_build}, observed {actual_build}"
    )

skip_patterns = []
list_base_args = []
index = 0
while index < len(test_args):
    value = test_args[index]
    if value == "-skip":
        if index + 1 >= len(test_args):
            raise SystemExit(f"{label}: -skip requires an expression")
        skip_patterns.append(test_args[index + 1])
        index += 2
        continue
    if value.startswith("-skip="):
        skip_patterns.append(value[len("-skip="):])
        index += 1
        continue
    list_base_args.append(value)
    index += 1
if len(skip_patterns) > 1:
    raise SystemExit(f"{label}: expected at most one -skip flag")

package_value_flags = {"-C", "-tags", "-run", "-list", "-count", "-timeout"}
package_indices = []
index = 0
while index < len(list_base_args):
    value = list_base_args[index]
    if value in package_value_flags:
        if index + 1 >= len(list_base_args):
            raise SystemExit(f"{label}: {value} requires a value")
        index += 2
        continue
    if any(value.startswith(flag + "=") for flag in package_value_flags):
        index += 1
        continue
    if value == "." or value.startswith("./"):
        package_indices.append(index)
    index += 1
if len(package_indices) != 1:
    raise SystemExit(f"{label}: expected one package argument")
actual_package = f"{module_dir}:{list_base_args[package_indices[0]]}"
if actual_package != expected_package:
    raise SystemExit(
        f"{label}: expected package {expected_package}, observed {actual_package}"
    )

run_indices = [
    index for index, value in enumerate(list_base_args)
    if value == "-run" or value.startswith("-run=")
]
if len(run_indices) > 1:
    raise SystemExit(f"{label}: expected at most one -run flag")
if flag_values(test_args, "-list"):
    raise SystemExit(f"{label}: original command must not contain -list")

def list_args_for(pattern):
    args = list_base_args[:]
    if run_indices:
        run_index = run_indices[0]
        if args[run_index] == "-run":
            if run_index + 1 >= len(args):
                raise SystemExit(f"{label}: -run requires an expression")
            args[run_index] = "-list"
            args[run_index + 1] = pattern
        else:
            args[run_index] = "-list=" + pattern
    else:
        package_index = package_indices[0]
        args[package_index:package_index] = ["-list", pattern]
    return args

original_run_pattern = "."
if run_indices:
    run_index = run_indices[0]
    original_run_pattern = (
        list_base_args[run_index + 1]
        if list_base_args[run_index] == "-run"
        else list_base_args[run_index][len("-run="):]
    )
list_args = list_args_for(original_run_pattern)

list_result = subprocess.run(
    ["go", "test", *list_args],
    cwd=repo_root,
    env=env,
    text=True,
    capture_output=True,
    check=False,
)
if list_result.returncode != 0:
    raise SystemExit(f"{label}: list validation exited {list_result.returncode}")
test_name = re.compile(r"Test[A-Za-z0-9_]+$")
go_status = re.compile(r"ok\s+\S+\s+[0-9.]+s(?:\s+\(cached\))?$")

def listed_names(result, phase):
    output = [line for line in result.stdout.splitlines() if line]
    if any(not test_name.fullmatch(line) and not go_status.fullmatch(line) for line in output):
        raise SystemExit(f"{label}: {phase} emitted unexpected output")
    names = [line for line in output if test_name.fullmatch(line)]
    if len(names) != len(set(names)):
        raise SystemExit(f"{label}: {phase} emitted duplicate test names")
    return names

listed = listed_names(list_result, "list validation")
skipped = set()
if skip_patterns:
    skip_result = subprocess.run(
        ["go", "test", *list_args_for(skip_patterns[0])],
        cwd=repo_root,
        env=env,
        text=True,
        capture_output=True,
        check=False,
    )
    if skip_result.returncode != 0:
        raise SystemExit(f"{label}: Go/RE2 skip validation exited {skip_result.returncode}")
    skipped = set(listed_names(skip_result, "skip validation"))
actual = [name for name in listed if name not in skipped]
actual_digest = hashlib.sha256(
    ("\n".join(sorted(actual)) + "\n").encode()
).hexdigest()
if len(actual) != expected_count or len(set(actual)) != expected_count:
    raise SystemExit(
        f"{label}: expected {expected_count} names, observed {len(actual)}"
    )
if actual_digest != expected_digest:
    raise SystemExit(
        f"{label}: expected set {expected_digest}, observed {actual_digest}"
    )
print(
    f"{label}: list validation passed; package {actual_package}; "
    f"build {actual_build}; raw listed {len(listed)}; "
    f"filtered executed {expected_count} names; "
    f"set-sha256 {actual_digest}"
)

run_result = subprocess.run(command, cwd=repo_root, env=env, check=False)
if run_result.returncode:
    raise SystemExit(run_result.returncode)
PY
}

go_test_checked 9 9aef95c84ffd42ad632040498c628c78072ce94f5cc2f6af8493fcffc233b707 ack-root experiments/g01-scaleset:. default+race+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=45s -run '^(TestSDKACKBoundaries|TestSDKDemandAboveFiftyAndPartialAcquisition|TestSDKRepeatedStatisticsAnd202ReuseLastObservation|TestRecoveryAfterACKCallbackCrash|TestRecoveryMissingLifecycleCallback|TestRecoveryErrorsHoldReservationsAndRedact|TestSDKAcquisitionResponseLossAfterACK|TestSDKCapacityWithdrawalDoesNotFenceInFlightAcquisition|TestSDKHTTPFailuresAndSessionRefresh)$' .
go_test_checked 5 f95a296947af0fb862e8b447d3e27b7ce9f01e726659f5f87393789b62b783c9 ack-livecanary experiments/g01-scaleset:./livecanary default+race+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=45s ./livecanary -run '^(TestSupportedListenerBarriersAndReservation|TestDriverBarriersThroughPinnedSDK|TestForeignIdentityAndUnreviewedWorkNeverACKOrDelete|TestAuditPR25MultiJobAcquisitionMustRefuseBeforeACK|TestNoMessageDoesNotCountAsCompletedBarrier)$'

go_test_checked 24 eae489a7d743c942dca803c9b13cb618dcd87fbd9a53634f022bc5381c7d6436 baseline experiments/g01-scaleset:./livecanary default+race+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=180s ./livecanary -run '^TestBaseline'
go_test_checked 9 b6a378d192ba2c614cd82c20eab7cffcdbbb00115a1bba4da803fc262a1c3d8f admission-livecanary experiments/g01-scaleset:./livecanary default+race+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=180s ./livecanary -run '^(TestAdmission.*|TestAuditPR25DistinctStateDirectoriesMustShareCap)$'
go_test_checked 8 4bce3c998c4f2e6806888de2ea9936c6dd9ea877c72b190d47f4f47e24cdb559 admission-liveworker experiments/g01-scaleset:./liveworker default+race+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=180s ./liveworker -run '^(TestAdmissionDirectoryUsesOSAccountWithoutEnvironmentFallback|TestAdmissionAuthorityChecksTheCurrentClaim|TestWorkerAdmissionCapsIndependentDirectories|TestWorkerAdmissionRetainsSlotAfterOutcomeAndClose|TestAdmissionRejectsCopiedJournalInDifferentDirectory|TestAdmissionSyncFailureMustBeRetried|TestAdmissionRefusesMissingUnsafeOrUnknownRootState|TestAdmissionInitializationLockPrecedesClaimCreation)$'
go_test_checked 1 e7cdff09074beb49efb17b8e68201798140ea6f0e83ad10f3fc096aed98823ce unsupported-liveworker experiments/g01-scaleset:./liveworker osusergo+race+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -tags=osusergo -count=1 -timeout=180s ./liveworker -run '^TestUnsupportedAccountLookupRefusesBeforeJournal$'
go_test_checked 1 e7cdff09074beb49efb17b8e68201798140ea6f0e83ad10f3fc096aed98823ce unsupported-livecanary experiments/g01-scaleset:./livecanary osusergo+race+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -tags=osusergo -count=1 -timeout=180s ./livecanary -run '^TestUnsupportedAccountLookupRefusesBeforeJournal$'

go_test_checked 3 0f5a8b0633982f8cc04a541cb396c46736c6eec486622354a6456554c52cfc75 jit-root experiments/g01-scaleset:. default+race+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=45s -run '^(TestSDKJITLookupBeforeCreationDoesNotDiscoverIdentity|TestSDKJITResponseLossWithoutCommitDoesNotDiscoverIdentity|TestSDKJITResponseLossDiscoversIdentityWithoutReissuing)$' .
go_test_checked 2 42eac255f830436a6fba9457bcb1964e8f9f596761fecfdb721c2d34b684f4cb jit-livecanary experiments/g01-scaleset:./livecanary default+race+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=45s ./livecanary -run '^(TestJITLostResponseIsSecretSafeAndNeverReissued|TestDriverBarriersThroughPinnedSDK)$'

go_test_checked 34 fcdcde4fce2efa204fdb352c035329a010f7a41b4b9746bf748b2ebc22b4d330 worker-runtime experiments/g01-scaleset:./liveworker default+race+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=45s ./liveworker -run '^(TestOneWorkerNeverRecreatedAndOnlyJITAddedToEnvironment|TestUncertainStartNeverRetriesAndCannotCleanup|TestEveryRuntimeBoundaryRejectsProfileAndOwnershipMismatch|TestUnverifiedRunnerUpdatePolicyRefusesBeforeRuntime|TestSocketReplacementAfterPreflightCannotReceiveAnyMutation|TestSocketModesAndControllerOwnership|TestSocketPostConnectRecheckClosesBeforeHTTP|TestNoCreateBeforeDurableIntent|TestUnknownCreateNeverRetriesAfterRestart|TestCreationWarningsPreserveKnownIDWithoutAuthorizingStart|TestWorkerPreparationReturnsCanonicalSnapshotAndRejectsPriorEffect|TestUnixInspectRequiresStateFlagsBeforeMutation|TestDockerInspectExact(KnownStatesAndSerializableFacts|StatePresenceAndLegacyRequirements|RejectsMalformedOrAmbiguousBodiesBeforeMutation|NotFoundReportsOnlyTheExactGET|RejectsOtherResponsesAndInvalidTargets|RequiresSupported404Body|CancellationNeverReportsPresenceOrAbsence|RejectsReplacedSocket|EOFCancellationKeepsUnknownOutcome)|TestDockerInspectLegacyCleanupKeepsSignedAndAbsentExitPolicy|TestDockerInspectMapsPreserveCaseSensitiveKeysAndProfile|TestDockerCompletedMutationResponseSurvivesEOFCancellation|TestDockerInspectUnknownOrAbsentStatusCannotAuthorizeMutation|TestUnixRuntimeRejectsWrongIdentityImagesAndUnsupportedLimits|TestUnixRuntimeOneShotCreateStartAndNonForceCleanup|TestUnixRuntimeAmbiguousEffectsNeverRetry|TestChangedDaemonCannotCreate|TestChangedDaemonOrAbsentContainerNeverMeansCleanupComplete|TestUnixTransportRejectsSymlinksAndInheritedTCPDestinations|TestAuditPR28MissingBridgeMustNotStart|TestCleanupRetainsActiveAndUnknownWorkers|TestOwnedTerminalCleanupAndRunningRemovalRace)$'
go_test_checked 4 8a0a576036768baff9a9b7d53b3487ef84b2e95f502b3ca9dd453463f396cade update-policy experiments/g01-scaleset:./livecanary default+race+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=45s ./livecanary -run '^(TestCreateRequestsDisabledRunnerUpdate|TestUnconfirmedUpdateSettingQuarantinesCreate|TestUpdateSettingDriftStopsBeforeSessionOrJIT|TestUpdateSettingDriftDoesNotBlockSafeEmptyCleanup)$'
go_test_checked 2 2614438aa4873bd044d1f0d6247b929403e2225c2e5112c13787bf790af42413 worker-command experiments/g01-scaleset:./cmd/g01-worker g01_worker+race+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -tags=g01_worker -count=1 -timeout=45s ./cmd/g01-worker -run '^(TestBlockedJITInputHonorsDeadline|TestOfflinePlanAndRefusalDoNotReadSecretsOrEchoInput)$'

go_test_checked 45 2e182d6aeb4c278eddbe272be1693e6bf0143759c23c03348435da59addfde53 reconciliation experiments/g01-scaleset:./livecanary default+race+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=45s ./livecanary -run '^(TestAmbiguousCreateNeverRetriesAfterRestart|TestObserve.*|TestObservationIntentFailureStopsBeforeRead|TestObservationResponseCaptureIsLocalAndRejectsOtherOperations|TestStatistics.*|TestDemandStatisticsAllowControlledProbeButNeverCleanup|TestInvalidOwnedProof.*|TestCanonicalPreparationRecordsNoPhaseOrRemoteIntent|TestCanonicalPreparationRefusesInvalidJournalAndPhase|TestCanonicalPreparationRecoveryAndDriverShareLocalGate|TestJournal.*|TestAuthority.*|TestInventoryStrictPages|TestInventoryMalformedStopsLegacyEffects|TestInventoryTransportRefusalIsBoundedAndSanitized|TestInventoryImpossibleTotalStopsBeforeNextPage|TestRosterActualTLSCompleteObservation|TestRosterPreservesOnlyAcceptedPagePrefix|TestRosterFinalPublicationGuardAfterDigest|TestRosterRefusesInvalidEntryWithoutNetwork|TestStrictJSONRejectsDecoderEquivalentDuplicateFields|TestStrictJSONRejectsDuplicateAuthorityFields|TestFailedDirectorySyncMustBeRetried|TestFileJournalRejectsUnrecordedAuthorityBeforeRawDriverEffect|TestRenewedRecoveryApprovalRetainsOwnedState)$'
go_test_checked 9 47a97f1f9083abbe72dfc21a2477bd25580684506fbf71bf5aba9e7f6f1a40e8 reconciliation-worker experiments/g01-scaleset:./liveworker default+race+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=45s ./liveworker -run '^(TestPrivateJournalLocksAndRetainsReservationAcrossRestart|TestJournalRejectsChangedApprovalTornTailAndUnsafeFiles|TestAuthorityLeaseRefusesConcurrentRunsAndFencesClose|TestAuthorityRejectsReplacedJournalOrDirectory|TestStrictJSONRejectsDecoderEquivalentDuplicateFields|TestStrictInputRejectsAmbiguousOrExtraAuthorityFields|TestFailedDirectorySyncMustBeRetried|TestFileJournalRejectsUnrecordedAuthorityBeforeRawDriverEffect|TestRenewedRecoveryApprovalRetainsOwnedState)$'
go_test_checked 11 84b872130057cf2103feaa8f7c6cdb69a4d779e135eebf11cc04cb00590eacb9 reconciliation-quarantine experiments/g01-scaleset:./livecanary default+race+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=45s ./livecanary -run '^(TestDemandStatisticsAllowControlledProbeButNeverCleanup|TestCleanupOnlyForNeverIssuedWorkerWithExactReceipt|TestZeroStatisticsAndOptionalAbsencePermitEmptyCleanup|TestUnsafeStatisticsStopNewEffectsBeforeControlledMessage|TestEmptyAvailableWithWorkStatisticsStaysQuarantined|TestOlderPendingIntentSurvivesSuccessfulZeroInspection|TestUnownedDiscoveryEvidenceCannotAuthorizeCreationAfterAbsence|TestAuditPR25ObservedJobsMustBlockCleanup|TestUnexpectedWorkMessageQuarantinesBeforeSafeClose|TestObservedRunnerSurvivesLaterAbsenceAndFirstCleanup|TestObservationResultFailureSurvivesFileReopenAndInspection)$'

go_test_checked 1 be0f21a92ce13d75e85a1182706e84b84884a8cf0ec01172e14e5ac2e04b600f secret-root experiments/g01-scaleset:. default+race+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=45s -run '^TestSDKBusyRemovalSentinelAndRawErrorExposure$' .
go_test_checked 6 6d0b3fc255928a6c7a5f7ae087d25715f96c0bad9f9298b5fb08ea7bc5fc34df secret-livecanary experiments/g01-scaleset:./livecanary default+race+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=45s ./livecanary -run '^(TestHTTPErrorsDoNotReturnSecretResponseBody|TestSDKHTTPDebugDoesNotLogCredentials|TestSharedTransportRejectsOversizeSuccessAndErrorBodies|TestResponseReaderConsumesOnlyBudgetPlusOneAndRejectsTruncation|TestResponseBudgetAppliesAfterGzipDecompression|TestRealJournalCreateFailureBlocksRetryAndContainsNoErrorBody)$'
go_test_checked 3 feda1bf98e323aef3e209cc4ea2765131ca7d17d70dfd7bb4828c0faab14dff4 secret-liveworker experiments/g01-scaleset:./liveworker default+race+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -count=1 -timeout=45s ./liveworker -run '^(TestUncertainStartNeverRetriesAndCannotCleanup|TestUnixResponsesAreBoundedAndRedirectsNeverFollowed|TestObservationDoesNotJournalRawRuntimeStatus)$'

go_test_checked 10 7ced37498c6790e0a1276ffece4cf0bcb2cad09f39da3ebdd0e28694f752596c live-command experiments/g01-scaleset:./cmd/g01-live g01_live+race+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -tags=g01_live -count=1 -timeout=45s ./cmd/g01-live -run '^(TestPlanAndRefusalsNeverReadCredentialsOrEchoInputs|TestPreparationCommandNeverReadsCredentialsOrRunsRemotePhase|TestPairedTerminalModeReadsControllerInputAfterAllGates|TestPairedTerminalModeRejectsUnusedPhaseAndControllerFlagsBeforeInput|TestPairedTerminalModeRequiresWorkflowVerificationAuthorityBeforeInput|TestInheritedNamedCredentialFIFODelayedEOF|TestInheritedCredentialPipeStopsAtDeadline|TestCredentialInputRejectsNonPipeDescriptor|TestBlockedCredentialPipeStopsAtDeadline|TestCredentialInputAcceptsCompleteAndRejectsOversize)$'
go_test_checked 4 a751c8b1a6a22d2f4ce76d90d6d79c48ab377355deed0f3bf189496c22b5492e live-transport experiments/g01-scaleset:./livecanary g01_live+race+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race -tags=g01_live -count=1 -timeout=45s ./livecanary -run '^(TestAuthoritySplitAndPolicyRejection|TestSDKTransportOwnership|TestCredentialAttestationMismatchAndExpiredTokenRejected|TestTransportRejectsPlaintextOffHostAndProxyBeforeNetwork)$'

go_test_checked 34 a79b7fa367d8eb1e7fe4ee4ef637696518946dab6f2f25410f6e04bfba137298 drain-pr72 experiments/g01-scaleset:./livecanary default+norace+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset ./livecanary -run '^(TestBaselineAcquireTargetIsActionsOnly|TestPinnedSDKDrain.*|TestDriverDrainThroughPinnedSDKAndPollHook|TestDrainListenerWithdrawsWhilePollResponseIsHeld|TestDrainCancellationStopsBeforeReleasingHeldResponse)$' -count=1 -v -timeout=180s
go_test_checked 34 a79b7fa367d8eb1e7fe4ee4ef637696518946dab6f2f25410f6e04bfba137298 drain-pr72-race experiments/g01-scaleset:./livecanary default+race+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -race ./livecanary -run '^(TestBaselineAcquireTargetIsActionsOnly|TestPinnedSDKDrain.*|TestDriverDrainThroughPinnedSDKAndPollHook|TestDrainListenerWithdrawsWhilePollResponseIsHeld|TestDrainCancellationStopsBeforeReleasingHeldResponse)$' -count=1 -v -timeout=180s

terminal_heavy_tests='^TestPairedTerminal(FinalResultCapacity|PendingChildCapacity|EligibilityUsesFreshExactFacts|CapturedAcknowledgementCancellation|MissingAcknowledgementsAndPostchecks)$'
terminal_remainder_skip='^TestPairedTerminal(FinalResultCapacity|PendingChildCapacity|EligibilityUsesFreshExactFacts|CapturedAcknowledgementCancellation|MissingAcknowledgementsAndPostchecks|Actual(Controller|Worker)SyncFailures|PostIntent(JournalIdentity|AuthorityBoundaries)|ClosedReplayActualFile|WorkerReceiptSurvivesControllerWriteFailure|FixtureStorageFailure)$'
terminal_storage_tests='^TestPairedTerminal(Actual(Controller|Worker)SyncFailures|PostIntent(JournalIdentity|AuthorityBoundaries)|ClosedReplayActualFile|WorkerReceiptSurvivesControllerWriteFailure|FixtureStorageFailure)$'
go_test_checked 22 f3efc29451112b27d2cb590763612790db48f05b54718b1e023196ead6625222 paired-collection experiments/g01-scaleset:./livecanary g01_pair_fixture+race+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -tags=g01_pair_fixture -race -count=1 -timeout=120s ./livecanary -run '^TestPaired' -skip '^TestPairedTerminal'
go_test_checked 106 bdeef850cfe820297e3896477219f0fcf90046bc30fb0bd454bfd0f8fe6db2ff paired-all-except experiments/g01-scaleset:./livecanary g01_pair_fixture+race+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -tags=g01_pair_fixture -race -count=1 -timeout=120s ./livecanary -skip '^TestPaired'
go_test_checked 24 b7f253f33da157adba67fe4312eddb0ee90172e24f46d4f48502890ab5f080f5 paired-worker experiments/g01-scaleset:./liveworker g01_pair_fixture+race+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -tags=g01_pair_fixture -race -count=1 -timeout=120s ./liveworker -run '^TestPaired'
go_test_checked 5 9e1a2e099c0fc48bbbd83dd0dc798d3c954910cd150b210037741b2c70211952 paired-heavy experiments/g01-scaleset:./livecanary g01_pair_fixture+race+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -tags=g01_pair_fixture -race -count=1 -timeout=120s ./livecanary -run "$terminal_heavy_tests"
go_test_checked 15 5e5dd1ee80d3d303271ab17b17aed22d5084e15684bcbe9f31152c1092ec1f73 paired-remainder experiments/g01-scaleset:./livecanary g01_pair_fixture+race+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -tags=g01_pair_fixture -race -count=1 -timeout=120s ./livecanary -run '^TestPairedTerminal' -skip "$terminal_remainder_skip"
go_test_checked 6 458f77f55209a59338a63bfc27697d85ebe5e0c3c7d1b959a0b56b2527f3ead5 paired-storage experiments/g01-scaleset:./livecanary g01_pair_fixture+race+go1.26.8 \
  GOTOOLCHAIN=go1.26.8 go test -C experiments/g01-scaleset -tags=g01_pair_fixture -race -count=1 -timeout=120s ./livecanary -run "$terminal_storage_tests"
GOTOOLCHAIN=go1.26.8 go vet -C experiments/g01-scaleset -tags=g01_pair_fixture ./livecanary
GOTOOLCHAIN=go1.26.8 go vet -C experiments/g01-scaleset -tags=g01_pair_fixture ./liveworker
```

The packet also audits every future shell `go test` selector prescription
containing either spelling of `-run`/`-skip` (`-run REGEXP` or `-run=REGEXP`, and
the corresponding `-skip` forms), whether or not the command starts with a
`GOTOOLCHAIN=` assignment. The guard audit discovers all selector-bearing
commands first and treats an unguarded line as an error rather than relying on
a visual review of the selector block. The package/build metadata audit below
is a separate gate: every guarded prescription must still carry explicit
`GOTOOLCHAIN`, tag and race metadata.

```sh
set -euo pipefail
selector_pattern='^[[:space:]]*(?:[A-Za-z_][A-Za-z0-9_]*=[^[:space:]]+[[:space:]]+)*go test .*[[:space:]]-(run|skip)(=|[[:space:]])'
selector_lines="$(rg -n "$selector_pattern" docs/evidence/g01-recovery-packet.md)"
test -n "$selector_lines"
selector_probe=$'GOTOOLCHAIN=go1.26.8 go test ./livecanary -run=^TestProbe$\n  GOTOOLCHAIN=go1.26.8 go test ./livecanary -skip ^TestProbe$'
rg -q "$selector_pattern" <<< "$selector_probe"
python3 - <<'PY'
from pathlib import Path
import re
import tempfile

lines = Path("docs/evidence/g01-recovery-packet.md").read_text(encoding="utf-8").splitlines()
selector_command = re.compile(
    r"^\s*(?:(?:[A-Za-z_][A-Za-z0-9_]*=[^\s]+)\s+)*go test .*\s-(?:run|skip)(?:=|\s)"
)
guarded = 0
for index, line in enumerate(lines):
    if line.lstrip().startswith("selector_probe="):
        continue
    if not selector_command.search(line):
        continue
    if index == 0 or not lines[index - 1].lstrip().startswith("go_test_checked "):
        raise SystemExit(f"unguarded future selector at line {index + 1}")
    guarded += 1
if guarded == 0:
    raise SystemExit("no future go test selector prescriptions found")
with tempfile.NamedTemporaryFile("w+", encoding="utf-8", suffix=".md") as probe:
    probe.write(
        Path("docs/evidence/g01-recovery-packet.md").read_text(encoding="utf-8")
        + "\ngo test ./livecanary -run ^TestZeroIndentProbe$\n"
    )
    probe.flush()
    try:
        probe_lines = Path(probe.name).read_text(encoding="utf-8").splitlines()
        for index, line in enumerate(probe_lines):
            if not selector_command.search(line):
                continue
            if index == 0 or not probe_lines[index - 1].lstrip().startswith("go_test_checked "):
                raise SystemExit(f"unguarded future selector at line {index + 1}")
    except SystemExit as error:
        if "unguarded future selector" not in str(error):
            raise
        print(
            "zero-indent selector probe: passed; an unguarded column-zero "
            "`go test -run` without GOTOOLCHAIN was discovered and rejected"
        )
    else:
        raise SystemExit("zero-indent selector probe: unguarded command was accepted")
print(
    f"future selector guard audit: passed; {guarded} go test selector "
    "prescriptions are wrapper-guarded; column-zero, prefixed and indented "
    "discovery forms recognized"
)
PY
```

The selector guard audit exited 0 and found 28 future `go test` selector
prescriptions containing `-run` or `-skip`, each immediately preceded by
`go_test_checked`; the focused zero-indent probe discovered and rejected an
unguarded column-zero selector without a leading `GOTOOLCHAIN=` assignment,
while the explicit read-only samples cover both separated and equals spellings
and both zero-indent and indented shell forms. No test body was run by this
grep/audit. For the paired partitions, the wrapper validates
the filtered executed sets: collection
22/22 (raw list 48), all-except 106/106 (raw list 154), worker 24/24, heavy
5/5, terminal remainder 15/15 (raw list 26), and storage 6/6; their recorded
SHA-256 values are the sorted filtered-name digests on the corresponding
prescription lines above.

The recorded selector-audit output was:

```text
zero-indent selector probe: passed; an unguarded column-zero `go test -run` without GOTOOLCHAIN was discovered and rejected
future selector guard audit: passed; 28 go test selector prescriptions are wrapper-guarded; column-zero, prefixed and indented discovery forms recognized
```

The wrapper metadata is independently checked against each command's literal
package and build flags so a copied digest cannot target the other package or a
different tag/race configuration:

```sh
set -euo pipefail
python3 - <<'PY'
import re
import shlex
from pathlib import Path

lines = Path("docs/evidence/g01-recovery-packet.md").read_text(encoding="utf-8").splitlines()
seen = []
for index, line in enumerate(lines):
    fields = line.split()
    if (
        len(fields) != 7
        or fields[0] != "go_test_checked"
        or fields[-1] != "\\"
        or not re.fullmatch(r"[0-9]+", fields[1])
        or not re.fullmatch(r"[0-9a-f]{64}", fields[2])
    ):
        continue
    if index + 1 >= len(lines):
        raise SystemExit(f"line {index + 1}: missing wrapped command")
    command = shlex.split(lines[index + 1].strip())
    env = {}
    while command and re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*=.*", command[0]):
        key, value = command.pop(0).split("=", 1)
        env[key] = value
    if command[:2] != ["go", "test"]:
        raise SystemExit(f"line {index + 2}: not a go test command")
    args = command[2:]
    if not env.get("GOTOOLCHAIN", "").strip():
        raise SystemExit(f"line {index + 2}: missing explicit GOTOOLCHAIN metadata")

    def values(flag):
        result = []
        cursor = 0
        while cursor < len(args):
            value = args[cursor]
            if value == flag:
                if cursor + 1 >= len(args):
                    raise SystemExit(f"line {index + 2}: {flag} requires a value")
                result.append(args[cursor + 1])
                cursor += 2
            elif value.startswith(flag + "="):
                result.append(value[len(flag) + 1:])
                cursor += 1
            else:
                cursor += 1
        return result

    modules = values("-C")
    packages = [value for value in args if value == "." or value.startswith("./")]
    if modules != ["experiments/g01-scaleset"] or len(packages) != 1:
        raise SystemExit(f"line {index + 2}: package/module identity is ambiguous")
    actual_package = f"{modules[0]}:{packages[0]}"
    tags = values("-tags")
    if len(tags) > 1:
        raise SystemExit(f"line {index + 2}: duplicate -tags flag")
    tag_identity = "default" if not tags else ",".join(sorted(tags[0].split(",")))
    race_flags = [value for value in args if value == "-race" or value.startswith("-race=")]
    if len(race_flags) > 1:
        raise SystemExit(f"line {index + 2}: duplicate -race flag")
    race_identity = "race" if race_flags and race_flags[0] != "-race=false" else "norace"
    actual_build = f"{tag_identity}+{race_identity}+{env.get('GOTOOLCHAIN', '')}"
    if actual_package != fields[4] or actual_build != fields[5]:
        raise SystemExit(
            f"{fields[3]}: expected {fields[4]}/{fields[5]}, "
            f"observed {actual_package}/{actual_build}"
        )
    seen.append(fields[3])
if len(seen) != 28 or len(set(seen)) != len(seen):
    raise SystemExit(f"expected 28 unique wrapper metadata records, observed {len(seen)}")
print(
    f"package/build metadata audit: passed; {len(seen)} wrapper prescriptions "
    "matched one package identity and explicit GOTOOLCHAIN/tag/race build "
    "configuration; duplicate package candidates fail closed"
)
PY
```

The package/build metadata audit exited 0 with 28 unique records. Every
package identity matched its `-C` directory and sole package argument, every
record carried explicit `GOTOOLCHAIN` metadata, and every build identity
matched its tag set, race mode and toolchain; a duplicate
`./livecanary`/`./liveworker` package list is rejected before the list probe.
The recorded metadata-audit output was:

```text
package/build metadata audit: passed; 28 wrapper prescriptions matched one package identity and explicit GOTOOLCHAIN/tag/race build configuration; duplicate package candidates fail closed
```

The wrapper's selector edge cases were then exercised with a trimmed copy of
the documented Python body that stops before the original test subprocess. Its
only child processes are the effective `go env GOFLAGS` and corresponding `go
test -list` probes; the second list probe uses Go's own regexp implementation
for `-skip` matching:

```sh
set -euo pipefail
python3 - <<'PY'
import hashlib
import io
import sys
import tempfile
from contextlib import redirect_stdout
from pathlib import Path

packet = Path("docs/evidence/g01-recovery-packet.md").read_text(encoding="utf-8")
start = packet.index("\nimport hashlib\n", packet.index("go_test_checked()")) + 1
end = packet.index("\nPY\n}", start)
wrapper = packet[start:end]
wrapper = wrapper[:wrapper.index("run_result = subprocess.run")]
one_name = "TestNoMessageDoesNotCountAsCompletedBarrier"
one_digest = hashlib.sha256((one_name + "\n").encode()).hexdigest()
supported_name = "TestSupportedListenerBarriersAndReservation"
supported_digest = hashlib.sha256((supported_name + "\n").encode()).hexdigest()
common = [
    "GOTOOLCHAIN=go1.26.8", "go", "test", "-C", "experiments/g01-scaleset",
    "-race", "-count=1", "-timeout=45s",
]
cases = [
    (
        "equals-run",
        "1", one_digest, "experiments/g01-scaleset:./livecanary",
        "default+race+go1.26.8",
        common + ["./livecanary", "-run=" + "^" + one_name + "$"],
        True,
    ),
    (
        "separated-filter",
        "22", "f3efc29451112b27d2cb590763612790db48f05b54718b1e023196ead6625222",
        "experiments/g01-scaleset:./livecanary", "g01_pair_fixture+race+go1.26.8",
        common + ["-tags=g01_pair_fixture", "./livecanary", "-run", "^TestPaired",
                  "-skip", "^TestPairedTerminal"],
        True,
    ),
    (
        "no-match",
        "1", one_digest, "experiments/g01-scaleset:./livecanary",
        "default+race+go1.26.8",
        common + ["./livecanary", "-run=^TestNoSuchSelectorName$"],
        False,
    ),
    (
        "skip-all-equals",
        "1", one_digest, "experiments/g01-scaleset:./livecanary",
        "default+race+go1.26.8",
        common + ["./livecanary", "-run=^" + one_name + "$",
                  "-skip=^" + one_name + "$"],
        False,
    ),
    (
        "posix-class-skip",
        "1", supported_digest, "experiments/g01-scaleset:./livecanary",
        "default+race+go1.26.8",
        common + ["./livecanary", "-run",
                  "^(TestSupportedListenerBarriersAndReservation|TestNoMessageDoesNotCountAsCompletedBarrier)$",
                  "-skip", "[[:upper:]]o"],
        True,
    ),
    (
        "package-mismatch",
        "1", one_digest, "experiments/g01-scaleset:./livecanary",
        "default+race+go1.26.8",
        common + ["./liveworker", "-run=^" + one_name + "$"],
        False,
    ),
    (
        "build-mismatch",
        "1", one_digest, "experiments/g01-scaleset:./livecanary",
        "default+race+go1.26.8",
        common + ["-tags=g01_live", "./livecanary", "-run=^" + one_name + "$"],
        False,
    ),
    (
        "duplicate-package-target",
        "1", one_digest, "experiments/g01-scaleset:./livecanary",
        "default+race+go1.26.8",
        common + ["./livecanary", "./liveworker", "-run=^" + one_name + "$"],
        False,
    ),
]
with tempfile.TemporaryDirectory() as goenv_dir:
    goenv = Path(goenv_dir) / "env"
    goenv.write_text("GOFLAGS=-run=^PersistedConfigProbe$\n", encoding="utf-8")
    cases.append(
        (
            "persisted-goflags",
            "1", one_digest, "experiments/g01-scaleset:./livecanary",
            "default+race+go1.26.8",
            [f"GOENV={goenv}", *common, "./livecanary",
             "-run=^" + one_name + "$"],
            False,
        )
    )
    for label, count, digest, package, build, command, should_pass in cases:
        sys.argv = ["wrapper-probe", count, digest, label, package, build, *command]
        output = io.StringIO()
        try:
            with redirect_stdout(output):
                exec(compile(wrapper, "<wrapper>", "exec"), {"__name__": "__main__"})
        except SystemExit as error:
            if should_pass:
                raise SystemExit(f"{label}: unexpected rejection: {error}")
            print(f"{label}: rejected before test body: {error}")
        else:
            if not should_pass:
                raise SystemExit(f"{label}: unexpectedly accepted")
            if "list validation passed" not in output.getvalue():
                raise SystemExit(f"{label}: missing list-validation result")
            print(f"{label}: accepted list-only selector")
PY
```

The focused list-only probes passed: equals-form `-run=` selected 1/1;
separated `-run` plus `-skip` preserved the filtered 22/22 set (raw list 48);
the POSIX-class `[[:upper:]]` skip probe preserved the exact 1/1 set; the
no-match and equals-form skip-all cases were rejected with 0 observed executed
names before any test body; a livecanary/liveworker package mismatch, tag/build
mismatch and temporary GOENV-persisted non-empty `GOFLAGS` were rejected before
listing; and duplicate package targeting was rejected before listing. No test
body, live operation or secret-bearing input was run.

The tagged worker command is a complete `./liveworker` `^TestPaired` partition
using the same fixture tag, toolchain, race detector, count and timeout as the
controller-side partitions. The controller-side terminal groups in this
fragment are exhaustive and disjoint: the five heavy names, the complementary
terminal remainder, and the named persistence tests. Its first two
controller-side collection/listener commands are likewise exhaustive and disjoint as specified
in [the terminal path record](g01-paired-terminal.md).

For every rerun, retain the exact source/evidence SHA, command, exit status,
elapsed time where useful, classification and limitations. Do not convert a
skipped, unavailable or unauthorized live phase into “passed.”

## Documentation-only validation

This packet correction requires markdown/link-target, JSON syntax, ledger/table,
fragment, selector, diff, and secret/private-path checks only. No artificial Go
red or green test is created for documentation changes. The stable working
directory and source boundary were checked from immutable validation snapshot
5979b7d722f3bf8e24404912f9b1f3e888d0828d; that historical snapshot is not the
current PR #78 head. The prior packet-correction head at the start of this
correction was exactly
6b1535ee7b6f08582ff162eca30f1e4294dbf32b and has the explicit exact-head
record below. The earlier PR #78 head at the previous validation point was
82eeef99f9bb5ec85c8cb3bea7a9a5947e8df26a and is retained only as historical
provenance; the pushed correction head will necessarily be a new SHA and
requires its own PR metadata, exact-head review and CI. This packet correction
updates only packet documentation, including selector-wrapper metadata and
validation wording; it leaves the experiments source unchanged. The comparison parent
ee8df8b7e00204c74a892b27f8b4c0ab278751ba is only the unchanged
experiments/g01-scaleset source-comparison parent. The selector declaration audit is
anchored to immutable source commit
`95cd9210620c54e098ecbe0df1217af1659f0c74` and tree
`d8b79cd1ddc6993792a44a8e8ae88985ce7466c0`. The audited default-build
declarations are in `./liveworker` (`docker_observation_test.go`,
`docker_test.go`, `worker_test.go`, and `journal_test.go`) and `./livecanary`
(`driver_test.go`, `observer*.go`, `statistics_fence_test.go`,
`owned_proof_test.go`, `journal*.go`, and `preparation_test.go`). The requested
direct contracts are plain `package livecanary`/`package liveworker` default-build
tests with no file build constraint; the only tag-sensitive test is
`TestUnsupportedAccountLookupRefusesBeforeJournal`, duplicated in both
packages and guarded by `//go:build !cgo || osusergo || android`, so the added
livecanary invocation forces `-tags=osusergo`; `TestZeroStatisticsAndOptionalAbsencePermitEmptyCleanup`
remains a default-build `./livecanary` test in `statistics_fence_test.go`.

The `./livecanary` test binary has a `TestMain` in
`preparation_fixture_test.go`, so Go starts that function before processing a
`-list` request. The source audit below verifies that the normal list path
matches neither of the two explicit `--prepare-approved-*` branches and falls
through to `os.Exit(m.Run())`; those branches are the only paths that read
approval/state inputs or prepare a journal. Accordingly, the livecanary
`go test -list` checks are treated as TestMain initialization checks as well as
selector checks: they use no live endpoint or credentials and run no test body.

The following commands are the exact documentation/static checks used for this
correction. Their results are recorded immediately after each check; no command
below runs a test body or performs a live App, runner, Docker, Lima, Keychain,
launchd or workflow operation.

### Markdown links, JSON, and ledger shape

```sh
set -euo pipefail
python3 - <<'PY'
import re
import subprocess
import unicodedata
from pathlib import Path
from urllib.parse import unquote

files = subprocess.check_output(["git", "ls-files", "*.md"], text=True).splitlines()
link = re.compile(r"(?<!!)" + re.escape("[") + r"[^]]*" + re.escape("]") + re.escape("(") + r"([^)]+)" + re.escape(")"))
heading = re.compile(r"^#{1,6}[ \t]+(.+?)[ \t]*#*[ \t]*$")

def slug(value):
    value = re.sub(r"[\`*_~]", "", unquote(value).strip().lower())
    value = unicodedata.normalize("NFKD", value).encode("ascii", "ignore").decode()
    return re.sub(r"[^\w\s-]", "", value).replace(" ", "-").strip("-")

def anchors(path):
    return {slug(m.group(1)) for m in map(heading.match, path.read_text(encoding="utf-8").splitlines()) if m}

errors = []
checked = 0
for name in files:
    source = Path(name)
    for match in link.finditer(source.read_text(encoding="utf-8")):
        target = match.group(1).strip().strip("<>")
        if target.startswith(("http://", "https://", "mailto:")):
            continue
        if target.startswith("#"):
            path, fragment = source, target[1:]
        else:
            target, separator, fragment = target.partition("#")
            path, fragment = (source.parent / target).resolve(), fragment if separator else None
        checked += 1
        if not path.is_file():
            errors.append(f"{name}: missing target {target}")
        elif fragment and slug(fragment) not in anchors(path):
            errors.append(f"{name}: missing anchor {path}#{fragment}")
if errors:
    raise SystemExit("\n".join(errors))
print(f"local markdown link/anchor check: passed; {checked} local targets checked; external URLs syntax-skipped")
PY
jq empty docs/backlog.json
ledger_rows=$(awk '
  /^\| Boundary \| Reuse unchanged evidence when \| Exact focused rerun \| Class\/result to record \|$/ { in_table=1; next }
  in_table && /^\|---/ { next }
  in_table && /^\|/ { rows++; next }
  in_table && !/^\|/ { exit }
  END { print rows + 0 }
' docs/evidence/g01-recovery-packet.md)
test "$ledger_rows" -eq 10
awk '
  /^\| Boundary \| Reuse unchanged evidence when \| Exact focused rerun \| Class\/result to record \|$/ { in_table=1; next }
  in_table && /^\|---/ { next }
  in_table && /^\|/ {
    if (split($0, fields, "\\|") != 6) { exit 1 }
    next
  }
  in_table && !/^\|/ { exit }
' docs/evidence/g01-recovery-packet.md
printf 'JSON and changed-boundary ledger checks: passed; backlog JSON valid, 10 data rows, and 4 columns in every ledger row\n'
```

The link/anchor checker reported 119 local targets with all targets present and
skipped external URLs after syntax recognition. `jq` exited 0, and the ledger
check exited 0 with 10 data rows and four columns in every row.

The packet's controller-side paired-terminal command fragment is compared with
the independently maintained fragment in `g01-paired-terminal.md`; the worker
partition and worker vet command are intentionally packet-only additions.

```sh
set -euo pipefail
diff -u \
  <(awk '
    /^terminal_heavy_tests=/{capture=1}
    capture {
      line=$0
      sub(/^[[:space:]]+/, "", line)
      if (line ~ /^(terminal_(heavy|remainder|storage)_tests=|GOTOOLCHAIN=.*go (test|vet) -C )/) print line
      if (line ~ /^GOTOOLCHAIN=.*go vet -C experiments\/g01-scaleset -tags=g01_pair_fixture \.\/livecanary$/) exit
    }
  ' docs/evidence/g01-paired-terminal.md) \
  <(awk '
    /^terminal_heavy_tests=/{capture=1}
    capture {
      line=$0
      sub(/^[[:space:]]+/, "", line)
      if (line ~ /^(terminal_(heavy|remainder|storage)_tests=|GOTOOLCHAIN=.*go (test|vet) -C )/) print line
      if (line ~ /^GOTOOLCHAIN=.*go vet -C experiments\/g01-scaleset -tags=g01_pair_fixture \.\/livecanary$/) exit
    }
  ' docs/evidence/g01-recovery-packet.md | grep -v 'liveworker')
printf 'paired-terminal normalized command fragment comparison: passed; packet controller commands match g01-paired-terminal.md\n'
```

The paired-terminal normalized fragment comparison exited 0 with no diff and
printed the pass message above; wrapper metadata is intentionally excluded, but
the underlying package, tag, selector, skip, toolchain, race, count and timeout
arguments remain compared exactly.

The terminal remainder skip expression is also compared as one exact
assignment/value, independently of the broader fragment normalization. The
paired-terminal record is stored at the repository path
`docs/evidence/g01-paired-terminal.md`; missing, duplicated or changed
assignments fail closed:

```sh
set -euo pipefail
python3 - <<'PY'
from pathlib import Path

packet_path = Path("docs/evidence/g01-recovery-packet.md")
paired_path = Path("docs/evidence/g01-paired-terminal.md")

def assignment(path):
    values = [
        line.strip()
        for line in path.read_text(encoding="utf-8").splitlines()
        if line.strip().startswith("terminal_remainder_skip=")
    ]
    if len(values) != 1:
        raise SystemExit(f"{path}: expected one terminal_remainder_skip assignment")
    return values[0]

packet_value = assignment(packet_path)
paired_value = assignment(paired_path)
if packet_value != paired_value:
    raise SystemExit("terminal_remainder_skip assignment/value mismatch")
print("terminal_remainder_skip assignment/value comparison: passed; packet and docs/evidence/g01-paired-terminal.md match exactly")
PY
```

The exact `terminal_remainder_skip` assignment/value comparison exited 0; the
packet and paired-terminal record matched, and the one-assignment guard would
reject a missing, duplicate or changed value before any selector ran.

### Prior packet-correction exact-head record

At the start of this packet-only correction, the worktree was the exact prior
packet-correction commit `6b1535ee7b6f08582ff162eca30f1e4294dbf32b`. The
following immutable identity check was run before editing and its output is
retained verbatim. It validates that actual commit, tree, packet blob and
parent; it is the post-correction record for that prior head, not a claim that
the later packet fix has the same SHA, and it must not be combined with the
historical `5979...` snapshot block or the earlier `82ee...` head block as one
passing checkout.

```sh
set -euo pipefail
packet_correction_head='6b1535ee7b6f08582ff162eca30f1e4294dbf32b'
packet_correction_tree='87a0724932277dd3cc79ca50fcf0b2c1fe9b9e06'
packet_correction_blob='0907f18b9f664f7d021a88ad13a50423fbef21d4'
packet_correction_parent='82eeef99f9bb5ec85c8cb3bea7a9a5947e8df26a'
test "$(git rev-parse HEAD)" = "$packet_correction_head"
test "$(git rev-parse --verify "${packet_correction_head}^{commit}")" = "$packet_correction_head"
test "$(git rev-parse "${packet_correction_head}^{tree}")" = "$packet_correction_tree"
test "$(git rev-parse "${packet_correction_head}:docs/evidence/g01-recovery-packet.md")" = "$packet_correction_blob"
test "$(git rev-parse "${packet_correction_head}^")" = "$packet_correction_parent"
printf 'existing packet-correction exact-head audit: passed; HEAD=%s; commit=%s; tree=%s; packet blob=%s; parent=%s\n' "$packet_correction_head" "$packet_correction_head" "$packet_correction_tree" "$packet_correction_blob" "$packet_correction_parent"
```

Recorded output from the pre-edit exact-head validation (the command exited 0):

```text
existing packet-correction exact-head audit: passed; HEAD=6b1535ee7b6f08582ff162eca30f1e4294dbf32b; commit=6b1535ee7b6f08582ff162eca30f1e4294dbf32b; tree=87a0724932277dd3cc79ca50fcf0b2c1fe9b9e06; packet blob=0907f18b9f664f7d021a88ad13a50423fbef21d4; parent=82eeef99f9bb5ec85c8cb3bea7a9a5947e8df26a
```

### Stable checkout and selector audit

```sh
set -euo pipefail
test "$(git rev-parse HEAD)" = "5979b7d722f3bf8e24404912f9b1f3e888d0828d"
test "$(git rev-parse --verify HEAD^{commit})" = "5979b7d722f3bf8e24404912f9b1f3e888d0828d"
test "$(git show -s --format=%H HEAD)" = "5979b7d722f3bf8e24404912f9b1f3e888d0828d"
current_head="$(git rev-parse HEAD)"
test "$(git rev-parse --show-toplevel)" = "$(pwd -P)"
test -d experiments/g01-scaleset
test "$(git rev-parse --verify ee8df8b7e00204c74a892b27f8b4c0ab278751ba)" = "ee8df8b7e00204c74a892b27f8b4c0ab278751ba"
git diff --quiet ee8df8b7e00204c74a892b27f8b4c0ab278751ba -- experiments/g01-scaleset
source_status="$(git status --porcelain=v1 --untracked-files=all -- experiments/g01-scaleset)"
test -z "$source_status"
test "$(git rev-parse 1396e201d905be204c3ac697be43723820581314:docs/evidence/g01-red.md)" = "c36e0af0c8e9b301f4889a02454c83ece8d5942f"
printf 'stable checkout audit: passed; current HEAD is %s, repo root is current directory, experiments/g01-scaleset exists, source comparison parent is unchanged, scoped tracked/untracked status is empty, and g01-red.md resolves to its pinned blob\n' "$current_head"
```

The stable checkout commands were actually run from immutable validation
snapshot 5979b7d722f3bf8e24404912f9b1f3e888d0828d, and the audit exited 0:
each literal 5979b7d722f3bf8e24404912f9b1f3e888d0828d assertion and the scoped
source checks passed. The same block intentionally fails closed when checked
from the pre-correction PR #78 head 82eeef99f9bb5ec85c8cb3bea7a9a5947e8df26a,
because its immutable snapshot assertion rejects that different head. The
pre-correction head assertion and the read-only remote-ref assertion both
passed at correction start; after push they are historical checks that must
fail closed. The prior follow-up documentation commit changed only validation
wording; this packet correction adds selector parsing and package/build metadata
checks while preserving that source comparison. It confirmed
the unchanged
experiments/g01-scaleset source relative to comparison parent
ee8df8b7e00204c74a892b27f8b4c0ab278751ba and the g01-red.md commit/blob pin. The
scoped `git status --porcelain=v1 --untracked-files=all --
experiments/g01-scaleset` output was empty (zero lines), so no tracked or
untracked source file could contaminate declaration or list results. It did not
inspect or execute any live system.

The pre-correction PR #78 head check is deliberately separate from the
immutable validation-snapshot block above; the two literal heads must never be
asserted in one passing checkout. It was run before editing and is retained as
historical provenance; after this packet-only fix is pushed, both literals must
fail closed and the new pushed head must be verified independently:

```sh
set -euo pipefail
test "$(git rev-parse HEAD)" = "82eeef99f9bb5ec85c8cb3bea7a9a5947e8df26a"
test "$(git ls-remote origin refs/heads/orca/g01-evidence-packet | awk '{print $1}')" = "82eeef99f9bb5ec85c8cb3bea7a9a5947e8df26a"
printf 'pre-correction PR #78 head/remote-ref audit: passed; both returned 82eeef99f9bb5ec85c8cb3bea7a9a5947e8df26a\n'
```

The pre-correction head/remote-ref audit exited 0 at correction start: both
the worktree and remote branch returned
`82eeef99f9bb5ec85c8cb3bea7a9a5947e8df26a`; it is a historical check and is
expected to fail after the correction push.

### Recorded post-correction head audits

The exact `08ce02f7716c991d088eebf1f7311628e7991f9e` post-correction audit is
recorded below as historical read-only Git worktree/remote-ref output. Its exact
time context is the `08ce` commit timestamp
`2026-09-14T01:58:23+09:00`; the audit belongs after that push and before the
subsequent packet-only corrections. It is not a current-checkout assertion and
does not claim live App, runner, Docker, Lima, Keychain, launchd or workflow
success.

Recorded output for the historical exact-`08ce` head/remote audit:

```text
post-correction current/remote head audit: passed; both returned 08ce02f7716c991d088eebf1f7311628e7991f9e
```

The new packet-correction head that followed the `08ce` review was
`201f5eed4d561a1255fbf5a2e930d676c23024c1`, committed at
`2026-09-14T02:33:08+09:00`. Its literal current-final parity output was
recorded after push and is kept separate from the historical `08ce` output:

```text
post-correction current/remote head audit: passed; both returned 201f5eed4d561a1255fbf5a2e930d676c23024c1
```

These two output records are historical parity evidence only; the immutable
`5979...`, `82ee...` and `6b153...` records above remain separate exact-head
records and are not combined into one checkout or result.

The final head check is dynamic and is run only after the packet commit is
pushed, so it remains internally runnable without adding a self-invalidating
literal SHA to a later packet commit:

```sh
set -euo pipefail
test -z "$(git status --porcelain=v1 --untracked-files=all)"
final_head="$(git rev-parse HEAD)"
remote_head="$(git ls-remote origin refs/heads/orca/g01-evidence-packet | awk '{print $1}')"
test -n "$final_head"
test "$final_head" = "$remote_head"
printf 'post-correction current/remote head audit: passed; both returned %s\n' "$final_head"
```

The post-correction current/remote head audit is a required final handoff
check; its output is recorded with the exact pushed SHA after this packet-only
commit.

The four focused offline selector checks below are list-only source checks. Each
has a literal expected test-name set and an explicit count; the helper exits
nonzero on a command failure, unexpected output, count mismatch or set
mismatch. It runs from the repository root with literal subprocess arguments,
so a missing or renamed build-tagged alternative cannot produce a false green.
The `-tags=osusergo` case is intentionally included in that fail-closed set.

```sh
set -euo pipefail
python3 - <<'PY'
import os
import re
import subprocess
from pathlib import Path

invocation_root = Path.cwd().resolve()
repo_root = Path(
    subprocess.check_output(
        ["git", "rev-parse", "--show-toplevel"],
        cwd=invocation_root,
        text=True,
    ).strip()
).resolve()
if invocation_root != repo_root:
    raise SystemExit("run this selector audit from the repository root")

env = {**os.environ, "GOTOOLCHAIN": "go1.26.8"}
effective_goflags = subprocess.run(
    ["go", "env", "GOFLAGS"], cwd=repo_root, env=env, text=True,
    capture_output=True, check=False,
)
if effective_goflags.returncode != 0 or effective_goflags.stderr.strip():
    raise SystemExit("selector audit: effective GOFLAGS query failed")
effective_lines = effective_goflags.stdout.splitlines()
if len(effective_lines) > 1 or (effective_lines and effective_lines[0].strip()):
    raise SystemExit("selector audit: effective GOFLAGS must be empty")
env["GOFLAGS"] = ""
cases = [
    {
        "label": "liveworker runtime",
        "args": [
            "go", "test", "-C", "experiments/g01-scaleset", "-race",
            "-count=1", "-timeout=45s", "./liveworker", "-list",
            r"^(TestSocketModesAndControllerOwnership|TestSocketPostConnectRecheckClosesBeforeHTTP|TestNoCreateBeforeDurableIntent|TestUnknownCreateNeverRetriesAfterRestart|TestCreationWarningsPreserveKnownIDWithoutAuthorizingStart|TestWorkerPreparationReturnsCanonicalSnapshotAndRejectsPriorEffect|TestUnixInspectRequiresStateFlagsBeforeMutation|TestDockerInspectExact(KnownStatesAndSerializableFacts|StatePresenceAndLegacyRequirements|RejectsMalformedOrAmbiguousBodiesBeforeMutation|NotFoundReportsOnlyTheExactGET|RejectsOtherResponsesAndInvalidTargets|RequiresSupported404Body|CancellationNeverReportsPresenceOrAbsence|RejectsReplacedSocket|EOFCancellationKeepsUnknownOutcome)|TestDockerInspectLegacyCleanupKeepsSignedAndAbsentExitPolicy|TestDockerInspectMapsPreserveCaseSensitiveKeysAndProfile|TestDockerCompletedMutationResponseSurvivesEOFCancellation|TestDockerInspectUnknownOrAbsentStatusCannotAuthorizeMutation|TestPrivateJournalLocksAndRetainsReservationAcrossRestart|TestJournalRejectsChangedApprovalTornTailAndUnsafeFiles|TestAuthorityLeaseRefusesConcurrentRunsAndFencesClose|TestAuthorityRejectsReplacedJournalOrDirectory|TestChangedDaemonCannotCreate)$",
        ],
        "expected_count": 25,
        "expected": [
            "TestUnixInspectRequiresStateFlagsBeforeMutation",
            "TestDockerInspectExactKnownStatesAndSerializableFacts",
            "TestDockerInspectExactStatePresenceAndLegacyRequirements",
            "TestDockerInspectLegacyCleanupKeepsSignedAndAbsentExitPolicy",
            "TestDockerInspectExactRejectsMalformedOrAmbiguousBodiesBeforeMutation",
            "TestDockerInspectUnknownOrAbsentStatusCannotAuthorizeMutation",
            "TestDockerInspectMapsPreserveCaseSensitiveKeysAndProfile",
            "TestDockerInspectExactNotFoundReportsOnlyTheExactGET",
            "TestDockerInspectExactRejectsOtherResponsesAndInvalidTargets",
            "TestDockerInspectExactRequiresSupported404Body",
            "TestDockerInspectExactCancellationNeverReportsPresenceOrAbsence",
            "TestDockerInspectExactRejectsReplacedSocket",
            "TestDockerCompletedMutationResponseSurvivesEOFCancellation",
            "TestDockerInspectExactEOFCancellationKeepsUnknownOutcome",
            "TestSocketModesAndControllerOwnership",
            "TestSocketPostConnectRecheckClosesBeforeHTTP",
            "TestAuthorityLeaseRefusesConcurrentRunsAndFencesClose",
            "TestAuthorityRejectsReplacedJournalOrDirectory",
            "TestPrivateJournalLocksAndRetainsReservationAcrossRestart",
            "TestWorkerPreparationReturnsCanonicalSnapshotAndRejectsPriorEffect",
            "TestJournalRejectsChangedApprovalTornTailAndUnsafeFiles",
            "TestCreationWarningsPreserveKnownIDWithoutAuthorizingStart",
            "TestNoCreateBeforeDurableIntent",
            "TestUnknownCreateNeverRetriesAfterRestart",
            "TestChangedDaemonCannotCreate",
        ],
    },
    {
        "label": "livecanary preparation",
        "args": [
            "go", "test", "-C", "experiments/g01-scaleset", "-race",
            "-count=1", "-timeout=45s", "./livecanary", "-list",
            r"^(TestCanonicalPreparationRecordsNoPhaseOrRemoteIntent|TestCanonicalPreparationRefusesInvalidJournalAndPhase|TestCanonicalPreparationRecoveryAndDriverShareLocalGate|TestZeroStatisticsAndOptionalAbsencePermitEmptyCleanup)$",
        ],
        "expected_count": 4,
        "expected": [
            "TestCanonicalPreparationRecordsNoPhaseOrRemoteIntent",
            "TestCanonicalPreparationRefusesInvalidJournalAndPhase",
            "TestCanonicalPreparationRecoveryAndDriverShareLocalGate",
            "TestZeroStatisticsAndOptionalAbsencePermitEmptyCleanup",
        ],
    },
    {
        "label": "livecanary unsupported-account build tag",
        "args": [
            "go", "test", "-C", "experiments/g01-scaleset", "-race",
            "-tags=osusergo", "-count=1", "-timeout=45s", "./livecanary",
            "-list", r"^TestUnsupportedAccountLookupRefusesBeforeJournal$",
        ],
        "expected_count": 1,
        "expected": ["TestUnsupportedAccountLookupRefusesBeforeJournal"],
    },
    {
        "label": "livecanary reconciliation",
        "args": [
            "go", "test", "-C", "experiments/g01-scaleset", "-race",
            "-count=1", "-timeout=45s", "./livecanary", "-list",
            r"^(TestAmbiguousCreateNeverRetriesAfterRestart|TestObserve.*|TestStatistics.*|TestInvalidOwnedProof.*|TestJournal.*|TestAuthority.*)$",
        ],
        "expected_count": 26,
        "expected": [
            "TestJournalFailureStopsBeforeCreate",
            "TestAmbiguousCreateNeverRetriesAfterRestart",
            "TestAuthorityMismatchStopsBeforeCreate",
            "TestAuthorityLeaseRefusesConcurrentRunsAndFencesClose",
            "TestAuthorityRejectsReplacedJournalOrDirectory",
            "TestJournalLocksBindsApprovalAndRetainsIncompleteIntent",
            "TestJournalRejectsTornTailSymlinksAndSharedModes",
            "TestObserveRESTRunnerRejectsIncompleteOrAmbiguousFields",
            "TestObserveRESTJobRejectsSourceAndAttemptAmbiguity",
            "TestObserveInvalidAuthorityAndInputsNeverReachNetwork",
            "TestObserveStatusAndResponseBudget",
            "TestObserveSampleKeepsOneDeadlineAcrossSourceListDetail",
            "TestObserveCancellationAndApprovalExpiry",
            "TestObserveSDKBootstrapFailureCannotReportRunnerAbsent",
            "TestObserveCancellationReachesSDKBootstrapAndRESTDetail",
            "TestObserveJobStatusProgressionWithinOneSample",
            "TestObserveRejectsIndependentBaseFork",
            "TestObserveJobAttemptRequiresDetailCorroboration",
            "TestObserveSDKRunnerIdentityAndProvenance",
            "TestObserveRESTExactFacts",
            "TestInvalidOwnedProofCannotBeClearedBeforeFirstCleanup",
            "TestAuthoritySplitAndPolicyRejection",
            "TestStatisticsAtEverySourceSurviveLaterZeroAndFreshDriver",
            "TestStatisticsFenceSurvivesFileJournalReopen",
            "TestStatisticsResultWriteFailureRetainsFenceAcrossRestart",
            "TestObservedRunnerSurvivesLaterAbsenceAndFirstCleanup",
        ],
    },
]
test_name = re.compile(r"Test[A-Za-z0-9_]+$")
go_status = re.compile(r"ok\s+\S+\s+[0-9.]+s(?:\s+\(cached\))?$")
for case in cases:
    expected = case["expected"]
    if len(expected) != case["expected_count"] or len(set(expected)) != case["expected_count"]:
        raise SystemExit(f"{case['label']}: invalid expected set/count in audit")
    if case["args"].count("-list") != 1 or "-run" in case["args"]:
        raise SystemExit(f"{case['label']}: selector audit must remain list-only")
    result = subprocess.run(
        case["args"], cwd=repo_root, env=env, text=True,
        capture_output=True, check=False,
    )
    if result.returncode != 0:
        raise SystemExit(f"{case['label']}: go test -list exited {result.returncode}")
    output = [line for line in result.stdout.splitlines() if line]
    if any(not test_name.fullmatch(line) and not go_status.fullmatch(line) for line in output):
        raise SystemExit(f"{case['label']}: unexpected non-test output")
    actual = [line for line in output if test_name.fullmatch(line)]
    if len(actual) != case["expected_count"] or sorted(actual) != sorted(expected):
        raise SystemExit(
            f"{case['label']}: expected {case['expected_count']} names, observed {len(actual)}"
        )
    print(
        f"{case['label']}: passed; expected/observed {case['expected_count']} names; "
        "exact set matched; list-only"
    )
PY
```

The fail-closed selector audit was actually run from immutable validation
snapshot 5979b7d722f3bf8e24404912f9b1f3e888d0828d against the unchanged source
under comparison parent ee8df8b7e00204c74a892b27f8b4c0ab278751ba: exact sets matched at
25/25 `liveworker` runtime names, 4/4 preparation names, 1/1 `osusergo`
build-tag name and 26/26 reconciliation names. Every invocation used
`go test -list` after the effective `go env GOFLAGS` check and explicit
`GOFLAGS=` pin; no test body ran.

The declaration consistency check also avoids self-referential line numbers and
uses immutable source/tree assertions plus quiet presence/absence checks:

```sh
set -euo pipefail
test "$(git rev-parse 95cd9210620c54e098ecbe0df1217af1659f0c74)" = "95cd9210620c54e098ecbe0df1217af1659f0c74"
test "$(git rev-parse '95cd9210620c54e098ecbe0df1217af1659f0c74^{tree}')" = "d8b79cd1ddc6993792a44a8e8ae88985ce7466c0"
rg -q 'func TestAmbiguousCreateNeverRetriesAfterRestart' experiments/g01-scaleset/livecanary/driver_test.go
rg -q 'func (TestSocketModesAndControllerOwnership|TestSocketPostConnectRecheckClosesBeforeHTTP)' experiments/g01-scaleset/liveworker/docker_test.go
rg -q 'func (TestNoCreateBeforeDurableIntent|TestUnknownCreateNeverRetriesAfterRestart)' experiments/g01-scaleset/liveworker/worker_test.go
rg -q 'func (TestWorkerPreparationReturnsCanonicalSnapshotAndRejectsPriorEffect|TestCreationWarningsPreserveKnownIDWithoutAuthorizingStart)' experiments/g01-scaleset/liveworker/journal_test.go
rg -q 'func (TestDockerInspectExactRejectsMalformedOrAmbiguousBodiesBeforeMutation|TestDockerInspectExactNotFoundReportsOnlyTheExactGET|TestDockerInspectExactRejectsOtherResponsesAndInvalidTargets|TestDockerInspectExactRequiresSupported404Body|TestDockerInspectExactCancellationNeverReportsPresenceOrAbsence|TestDockerInspectExactEOFCancellationKeepsUnknownOutcome)' experiments/g01-scaleset/liveworker/docker_observation_test.go
rg -q 'func (TestDockerInspectExactKnownStatesAndSerializableFacts|TestDockerInspectExactStatePresenceAndLegacyRequirements|TestDockerInspectLegacyCleanupKeepsSignedAndAbsentExitPolicy|TestDockerInspectMapsPreserveCaseSensitiveKeysAndProfile|TestDockerInspectExactRejectsReplacedSocket|TestDockerCompletedMutationResponseSurvivesEOFCancellation)' experiments/g01-scaleset/liveworker/docker_observation_test.go
rg -q 'func TestChangedDaemonCannotCreate' experiments/g01-scaleset/liveworker/worker_test.go
rg -q 'func (TestCanonicalPreparationRecordsNoPhaseOrRemoteIntent|TestCanonicalPreparationRefusesInvalidJournalAndPhase|TestCanonicalPreparationRecoveryAndDriverShareLocalGate)' experiments/g01-scaleset/livecanary/preparation_test.go
rg -q '^package livecanary$' experiments/g01-scaleset/livecanary/driver_test.go
rg -q '^package liveworker$' experiments/g01-scaleset/liveworker/docker_observation_test.go experiments/g01-scaleset/liveworker/worker_test.go
! rg -q '^//go:build|^// \+build' experiments/g01-scaleset/livecanary/driver_test.go experiments/g01-scaleset/liveworker/docker_observation_test.go experiments/g01-scaleset/liveworker/worker_test.go
rg -q '^func Test(Observe|Statistics|InvalidOwnedProof|Journal|Authority)' experiments/g01-scaleset/livecanary --glob '*_test.go'
rg -q '^//go:build !cgo \|\| osusergo \|\| android$|func TestUnsupportedAccountLookupRefusesBeforeJournal' experiments/g01-scaleset/liveworker/admission_lookup_unsupported_test.go experiments/g01-scaleset/livecanary/admission_lookup_unsupported_test.go
rg -q '^func TestMain\(m \*testing\.M\)' experiments/g01-scaleset/livecanary/preparation_fixture_test.go
python3 - <<'PY'
from pathlib import Path

source = Path("experiments/g01-scaleset/livecanary/preparation_fixture_test.go").read_text(encoding="utf-8")
start = source.index("func TestMain(m *testing.M) {")
main = source[start:]
first_branch = 'if len(os.Args) > 1 && os.Args[1] == "--prepare-approved-paired-journal"'
second_branch = 'if len(os.Args) > 1 && os.Args[1] == "--prepare-approved-journal"'
if not main.startswith("func TestMain(m *testing.M) {\n"):
    raise SystemExit("TestMain declaration is not in the expected form")
first = main.index(first_branch)
second = main.index(second_branch)
normal = main.index("os.Exit(m.Run())")
if not first < second < normal:
    raise SystemExit("TestMain preparation branches do not precede normal m.Run")
prefix = main[:first]
prefix_lines = prefix.splitlines()
if not prefix_lines or prefix_lines[0] != "func TestMain(m *testing.M) {" or any(
    line.strip() for line in prefix_lines[1:]
):
    raise SystemExit("TestMain has work before its explicit preparation branches")
for forbidden in ("http.", "net.", "exec.", "Driver.Run"):
    if forbidden in prefix:
        raise SystemExit(f"TestMain list prefix contains {forbidden}")
print("livecanary TestMain list-path audit: passed; normal -list path reaches m.Run without preparation branch or live-resource call")
PY
printf 'selector declaration audit: passed; immutable source/tree, requested declarations, package names, and build constraints matched; no test bodies executed\n'
```

The declaration audit also passed the `TestMain` list-path check: the normal
`-list` path reaches `m.Run` before any preparation branch and has no
live-resource call. The existing livecanary selector audits therefore exercise
that initialization path only; every invocation remains `go test -list` and no
test body or live resource was run.

### Diff and staged secret/private-path scan

After staging only this packet file, the final local checks were:

```sh
set -euo pipefail
test "$(git diff --cached --name-only)" = "docs/evidence/g01-recovery-packet.md"
test "$(git diff --cached --name-only | wc -l | tr -d ' ')" -eq 1
git diff --check
git diff --cached --check
private_user_root='/'"Users/"
private_home_root='/'"home/"
private_var_root='/'"private/var/"
secret_private_pattern='^\+.*(-----BEGIN[[:space:]]+[A-Z0-9 ]*PRIVATE KEY|gh[pousr]_[A-Za-z0-9_]+|github_pat_[A-Za-z0-9_]+|AKIA[0-9A-Z]{16}|xox[baprs]-[A-Za-z0-9-]+|'"${private_user_root}"'|'"${private_home_root}"'|'"${private_var_root}"')'
staged_diff="$(git diff --cached --unified=0 -- docs/evidence/g01-recovery-packet.md)"
if rg -q -- "$secret_private_pattern" <<< "$staged_diff"; then
  printf 'staged secret/private-path scan: FAILED\n'
  exit 1
else
  staged_scan_status=$?
  test "$staged_scan_status" -eq 1
fi
early_match_probe="$(python3 - <<'PY'
synthetic_match = "+" + "gh" + "p_" + "early_probe"
print("\n".join([synthetic_match] + [f"+ordinary-line-{index:04d}" for index in range(2048)]))
PY
)"
if rg -q -- "$secret_private_pattern" <<< "$early_match_probe"; then
  printf 'staged secret/private-path early-match regression probe: passed; first added line was detected and would be rejected\n'
else
  printf 'staged secret/private-path early-match regression probe: FAILED; first added line was not detected\n'
  exit 1
fi
printf 'diff and staged secret/private-path checks: passed; working/staged diff checks exited 0, one staged packet path, no added-line matches, and early-match rejection probe passed\n'
```

The complete staged diff is captured in a shell variable before scanning, so an
early `rg -q` match cannot send the diff producer SIGPIPE 141 under `pipefail`.
The staged diff check exited 0, the staged path set was exactly this packet
file, and the added-line secret/private-path scan found no matches. The focused
regression fixture put a synthetic matching added line first and 2,048 ordinary
added lines after it; the scanner detected that early match and exercised the
fail-closed rejection path. The recorded output was:

```text
staged secret/private-path early-match regression probe: passed; first added line was detected and would be rejected
diff and staged secret/private-path checks: passed; working/staged diff checks exited 0, one staged packet path, no added-line matches, and early-match rejection probe passed
```

No check is claimed against an unrecorded SHA. The staged correction diff is
distinct from the cumulative PR #78 diff; the cumulative history still
includes the independent driver correction from
`7ce053380003c9260f46bf93ea118893b95f01e7` and team-review routing corrections
from `f59a30532bfdc62876065dcf8b4997520606e6a6`, which remain outside this
staged change.

## Historical Codex finding ledger

The following stale/outdated root findings were omitted from the earlier
correction paragraphs. Each row preserves the original finding URL and the
immutable source commit where Codex observed it; disposition cites the packet
change that reproduced and fixed the gap, not staleness alone. The residual
selector-matching finding is resolved by the reusable wrapper above, which now
guards every runnable long selector before executing its original command.

| Finding and immutable source | Reproduction and disposition |
|---|---|
| [3998925786](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3998925786), source [0e08ce8285846e26a94fb6fcb33bbeede662bf14](https://github.com/1XP-AI/gh-runnerd/commit/0e08ce8285846e26a94fb6fcb33bbeede662bf14) | Reproduced: the root-only JIT rerun omitted worker transport. The packet now includes worker-runtime and tagged worker-command selectors; the wrapper validates their package/tag name sets before execution. |
| [3998925788](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3998925788), source [0e08ce8285846e26a94fb6fcb33bbeede662bf14](https://github.com/1XP-AI/gh-runnerd/commit/0e08ce8285846e26a94fb6fcb33bbeede662bf14) | Reproduced: the non-terminal paired command excluded every terminal test. The packet now prescribes collection, all-except, worker, heavy, remainder and storage partitions, each guarded by list validation. |
| [3998925790](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3998925790), source [0e08ce8285846e26a94fb6fcb33bbeede662bf14](https://github.com/1XP-AI/gh-runnerd/commit/0e08ce8285846e26a94fb6fcb33bbeede662bf14) | Reproduced: inline selector pipes could split the ledger table. Commands remain outside the ten-row ledger, and the recorded shape check requires ten data rows with four columns. |
| [3998925793](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3998925793), source [0e08ce8285846e26a94fb6fcb33bbeede662bf14](https://github.com/1XP-AI/gh-runnerd/commit/0e08ce8285846e26a94fb6fcb33bbeede662bf14) | Reproduced: the earlier driver blob contradicted ACK-before-acquisition. The evidence index now points to the corrected driver revision 0e08ce8285846e26a94fb6fcb33bbeede662bf14. |
| [3998925795](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3998925795), source [0e08ce8285846e26a94fb6fcb33bbeede662bf14](https://github.com/1XP-AI/gh-runnerd/commit/0e08ce8285846e26a94fb6fcb33bbeede662bf14) | Reproduced: callback-loss reruns omitted the missing-callback and error/quarantine cases. The ACK root selector now names all three recovery tests and the wrapper checks the resulting set/count. |
| [3998973873](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3998973873), source [9d07f3ccbbf4ff2a80aaa5ff43f376cb2b3da999](https://github.com/1XP-AI/gh-runnerd/commit/9d07f3ccbbf4ff2a80aaa5ff43f376cb2b3da999) | Reproduced: root-only ACK reruns omitted driver ACK/acquisition checks. The livecanary ACK selector now includes the driver and refusal contracts, with pre-run set/count validation. |
| [3998973876](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3998973876), source [9d07f3ccbbf4ff2a80aaa5ff43f376cb2b3da999](https://github.com/1XP-AI/gh-runnerd/commit/9d07f3ccbbf4ff2a80aaa5ff43f376cb2b3da999) | Reproduced: worker-only JIT reruns omitted controller JIT-loss probes. The livecanary JIT selector now names both controller cases and is guarded before execution. |
| [3998973877](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3998973877), source [40daf0f074b9cc9559135adac1cafaad218275be](https://github.com/1XP-AI/gh-runnerd/commit/40daf0f074b9cc9559135adac1cafaad218275be) | Reproduced: the reconciliation prefix omitted retained-uncertainty predicates. The quarantine selector now explicitly includes all named predicates, with count/set validation. |
| [3999010977](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999010977), source [9d07f3ccbbf4ff2a80aaa5ff43f376cb2b3da999](https://github.com/1XP-AI/gh-runnerd/commit/9d07f3ccbbf4ff2a80aaa5ff43f376cb2b3da999) | Reproduced: valid singleton cases did not reject foreign or multi-job messages. The ACK/livecanary selector now includes both refusal tests and validates their presence before running. |
| [3999010981](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999010981), source [9d07f3ccbbf4ff2a80aaa5ff43f376cb2b3da999](https://github.com/1XP-AI/gh-runnerd/commit/9d07f3ccbbf4ff2a80aaa5ff43f376cb2b3da999) | Reproduced: the image/profile row selected only one worker test. The worker-runtime selector now includes profile, update-policy and Docker-preflight contracts, guarded by the wrapper. |
| [3999010985](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999010985), source [9d07f3ccbbf4ff2a80aaa5ff43f376cb2b3da999](https://github.com/1XP-AI/gh-runnerd/commit/9d07f3ccbbf4ff2a80aaa5ff43f376cb2b3da999) | Reproduced: retained-uncertainty cases were absent from both reconciliation commands. The quarantine selector now includes observed-job, unexpected-work, later-absence and persistence-failure cases. |
| [3999010989](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999010989), source [9d07f3ccbbf4ff2a80aaa5ff43f376cb2b3da999](https://github.com/1XP-AI/gh-runnerd/commit/9d07f3ccbbf4ff2a80aaa5ff43f376cb2b3da999) | Reproduced: the baseline-only command omitted global admission checks. The packet now has explicit controller and worker admission selectors, including cross-directory and concurrent-start cases. |
| [3999010990](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999010990), source [9d07f3ccbbf4ff2a80aaa5ff43f376cb2b3da999](https://github.com/1XP-AI/gh-runnerd/commit/9d07f3ccbbf4ff2a80aaa5ff43f376cb2b3da999) | Reproduced: SDK session-refresh behavior was omitted from the ACK selector. The root selector now includes TestSDKHTTPFailuresAndSessionRefresh and validates the exact set. |
| [3999060310](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999060310), source [60fdefe124d7c83ad54eac36950cb574f7e0c9aa](https://github.com/1XP-AI/gh-runnerd/commit/60fdefe124d7c83ad54eac36950cb574f7e0c9aa) | Reproduced: demand and stale-statistics tests were absent from the SDK rerun. The root selector now names both cases and the wrapper checks count and set before running. |
| [3999208511](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999208511), source [cdaf879fed807da517ab88ea5db3dba6e99a948e](https://github.com/1XP-AI/gh-runnerd/commit/cdaf879fed807da517ab88ea5db3dba6e99a948e) | Reproduced: the tagged controller command was missing. The packet now prescribes the ten tagged cmd/g01-live input/refusal checks with fail-closed validation. |
| [3999208514](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999208514), source [f59a30532bfdc62876065dcf8b4997520606e6a6](https://github.com/1XP-AI/gh-runnerd/commit/f59a30532bfdc62876065dcf8b4997520606e6a6) | Reproduced: tagged worker JIT input checks were absent. The packet now includes the g01_worker command and expected set/count guard. |
| [3999208519](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999208519), source [9dea03887cc530f742e705055064579121a84206](https://github.com/1XP-AI/gh-runnerd/commit/9dea03887cc530f742e705055064579121a84206) | Reproduced: worker journal/authority tests were omitted. The liveworker reconciliation selector now names those durability contracts and validates them before execution. |
| [3999433329](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999433329), source [9dea03887cc530f742e705055064579121a84206](https://github.com/1XP-AI/gh-runnerd/commit/9dea03887cc530f742e705055064579121a84206) | Reproduced: worker admission contracts were not explicit. The worker admission selector now covers the integrity suite and the tagged unsupported-account case, with wrapper validation. |
| [3999433330](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999433330), source [080b9c29c0c056dffd03f54567b593a426a1909d](https://github.com/1XP-AI/gh-runnerd/commit/080b9c29c0c056dffd03f54567b593a426a1909d) | Reproduced: controller update-policy contracts were absent. The livecanary update-policy selector now includes disable-update, unconfirmed-setting and drift cases. |
| [3999433331](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999433331), source [9dea03887cc530f742e705055064579121a84206](https://github.com/1XP-AI/gh-runnerd/commit/9dea03887cc530f742e705055064579121a84206) | Reproduced: uncertainty and cleanup contracts were omitted from the worker selector. The worker-runtime command now includes uncertain-start, active/unknown retention and terminal-removal cases. |
| [3999433334](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999433334), source [9dea03887cc530f742e705055064579121a84206](https://github.com/1XP-AI/gh-runnerd/commit/9dea03887cc530f742e705055064579121a84206) | Reproduced: scheduled reconciliation was stated too much like implemented behavior. The packet now labels scheduling, fencing and rehydration as future production requirements and preserves the live gap. |
| [3999433335](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999433335), source [9dea03887cc530f742e705055064579121a84206](https://github.com/1XP-AI/gh-runnerd/commit/9dea03887cc530f742e705055064579121a84206) | Reproduced: broad rollback could revert independent corrections. The rollback row now scopes removal/restoration to this packet and preserves driver and routing corrections. |

### Supplemental stale/outdated root findings

These eight root findings were received after the historical 22-row ledger was
recorded. Each row retains the discussion URL and immutable source commit;
disposition cites the packet selector or source text that reproduces the issue
and its correction, rather than relying on staleness alone.

| Finding and immutable source | Reproduction and disposition |
|---|---|
| [3999099702](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999099702), source [40daf0f074b9cc9559135adac1cafaad218275be](https://github.com/1XP-AI/gh-runnerd/commit/40daf0f074b9cc9559135adac1cafaad218275be) | Reproduced: the admission selector omitted `TestAuditPR25DistinctStateDirectoriesMustShareCap`. The packet's admission-livecanary selector now names that cross-directory cap regression, with wrapper list/set/count validation before execution. |
| [3999099708](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999099708), source [40daf0f074b9cc9559135adac1cafaad218275be](https://github.com/1XP-AI/gh-runnerd/commit/40daf0f074b9cc9559135adac1cafaad218275be) | Reproduced: the reconciliation selectors omitted `TestDemandStatisticsAllowControlledProbeButNeverCleanup`. The packet now includes that demand-statistics quarantine contract in the reconciliation and quarantine selectors, with exact list/set/count validation. |
| [3999140786](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999140786), source [65071cfb1e8ad0f47ab7e09dd119f1c98beaaefc](https://github.com/1XP-AI/gh-runnerd/commit/65071cfb1e8ad0f47ab7e09dd119f1c98beaaefc) | Reproduced: reconciliation selectors omitted the four `TestInventory*` contracts. The packet now names `TestInventoryStrictPages`, `TestInventoryMalformedStopsLegacyEffects`, `TestInventoryTransportRefusalIsBoundedAndSanitized` and `TestInventoryImpossibleTotalStopsBeforeNextPage` in the exact reconciliation selector and audit. |
| [3999140787](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999140787), source [65071cfb1e8ad0f47ab7e09dd119f1c98beaaefc](https://github.com/1XP-AI/gh-runnerd/commit/65071cfb1e8ad0f47ab7e09dd119f1c98beaaefc) | Reproduced: the ACK selector omitted `TestNoMessageDoesNotCountAsCompletedBarrier`. The packet now includes this empty-poll barrier contract in the exact ACK/livecanary selector and list-only audit. |
| [3999140792](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999140792), source [65071cfb1e8ad0f47ab7e09dd119f1c98beaaefc](https://github.com/1XP-AI/gh-runnerd/commit/65071cfb1e8ad0f47ab7e09dd119f1c98beaaefc) | Reproduced: the secret/error selector omitted decoded-body truncation and gzip-budget checks. The packet now includes `TestResponseReaderConsumesOnlyBudgetPlusOneAndRejectsTruncation` and `TestResponseBudgetAppliesAfterGzipDecompression` in the livecanary selector. |
| [3999140797](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999140797), source [65071cfb1e8ad0f47ab7e09dd119f1c98beaaefc](https://github.com/1XP-AI/gh-runnerd/commit/65071cfb1e8ad0f47ab7e09dd119f1c98beaaefc) | Reproduced: journal/authority selectors omitted strict duplicate-field, failed-sync, unrecorded-authority and renewed-approval contracts. The packet now includes the named livecanary and liveworker journal/authority cases in the reconciliation partitions and audits their exact sets. |
| [3999140800](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999140800), source [65071cfb1e8ad0f47ab7e09dd119f1c98beaaefc](https://github.com/1XP-AI/gh-runnerd/commit/65071cfb1e8ad0f47ab7e09dd119f1c98beaaefc) | Reproduced: the tagged controller command did not exercise the livecanary credential-attestation and plaintext/off-host transport refusals. The packet now includes `TestCredentialAttestationMismatchAndExpiredTokenRejected` and `TestTransportRejectsPlaintextOffHostAndProxyBeforeNetwork` in the live-transport selector. |
| [3999140806](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999140806), source [9b94ff9f3114e5546abbe741b96486c3bc13626c](https://github.com/1XP-AI/gh-runnerd/commit/9b94ff9f3114e5546abbe741b96486c3bc13626c) | Reproduced against the immutable source: `docs/reviews/team-review.md` already marks the Astra assignments historical-only and routes current work through `docs/EXECUTION.md`; the correction was made by `f59a30532bfdc62876065dcf8b4997520606e6a6`. This stale finding is outside packet-only ownership, so no other file is changed here. |

The two new exact-head findings
on `95cd9210620c54e098ecbe0df1217af1659f0c74`—[3999634756](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999634756)
and [3999634759](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999634759)—are
covered by the controller-side `TestAmbiguousCreateNeverRetriesAfterRestart`
entry in the livecanary reconciliation selector and the seven direct liveworker
runtime contracts in the worker selector, with matching list-only/declaration/tag
checks above. The five new exact-head findings
on `4a4e159ce29414fa8e5bdf7a0d0e5d9b8d038c67`—[3999599127](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999599127),
[3999599130](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999599130),
[3999599132](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999599132),
[3999599134](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999599134),
and [3999599136](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999599136)—are covered by the explicit liveworker Docker-inspection and worker-preparation selectors, the suffixed livecanary reconciliation families, and the matching list-only/declaration checks above.
The prior five exact-head findings
on `080b9c29c0c056dffd03f54567b593a426a1909d`—[3999566558](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999566558),
[3999566563](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999566563),
[3999566567](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999566567),
[3999566570](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999566570),
and [3999566571](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999566571)—are covered by the worker/runtime and canonical-preparation selectors, the tagged livecanary invocation, and the explicit Runner pin/source-audit gate above.
The prior four exact-head
findings [3999532146](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999532146),
[3999532149](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999532149),
[3999532151](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999532151),
and [3999532158](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999532158)
are covered by these selector, partition, and validation-scope corrections.
The prior six exact-head findings [3999486217](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999486217),
[3999486223](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999486223),
[3999486229](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999486229),
[3999486231](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999486231),
[3999486236](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999486236),
and [3999486239](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999486239)
remain covered by the existing selector and partition corrections. The
three exact-head findings on `c4b73c75db9556f96491fb8a10a8257408a7d11b`—
[3999807788](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999807788),
[3999807790](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999807790),
and [3999807791](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999807791)—are
addressed by the literal selector expected-set/count audit, its list-only
root-safe invocation, and the scoped tracked/untracked source audit above.
The historical [secret/error finding 3999010986](https://github.com/1XP-AI/gh-runnerd/pull/78#discussion_r3999010986)
remains addressed by the dedicated Secret and error handling row and its root
and livecanary commands above; no issue, Project, or Goal state is changed.
