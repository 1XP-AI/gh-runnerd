# G01 v0.5.0 adoption ledger and current evidence profile

Issue: [G01 #1](https://github.com/1XP-AI/gh-runnerd/issues/1). Writer: Codex,
maintainer-selected Sol medium. Status: **local adoption candidate; independent
base review approved, zero-identity delta and GitHub review pending**.
No live execution, G01 closure or dependent production approval.

## Selection and change boundary

Select SDK `v0.5.0`, source `af394d86c0d5024225238b0209043c70193bcfe6`, and exact
Go `1.27.3`. The prior [isolated qualification](g01-sdk-v05-candidate.md) was
reviewed/merged in [PR #106](https://github.com/1XP-AI/gh-runnerd/pull/106).
Main `193c53d8e5b8a10951eabfa2ab1d3fb2184814ba` subsequently passed
[all seven Public CI jobs](https://github.com/1XP-AI/gh-runnerd/actions/runs/40162663518).
That is old-profile integration evidence, not evidence for this later adoption.

| Changed surface | Contract / validation boundary |
| --- | --- |
| Four module Go directives, Make, selectors, CI | Exact Go 1.27.3 everywhere active; no global Go configuration changed. Legacy comparison remains explicitly Go 1.26.9. |
| G01 SDK/HTTP options | Exact v0.5.0; guarded HTTP client, zero automatic retries, discarded logger. Hashicorp HTTP/JWT-v4 dependencies are removed from the active graph, not from historical records. |
| Controller/worker/broker admission | Exact Go 1.27.3; controller and broker additionally require SDK v0.5.0 without replacement. Existing revision/clean/tag/platform/cgo/identity restrictions remain. Old reviewed binaries are not silently admitted. |
| Default protocol tests | The 17 v0.5 tests are promoted unchanged from testdata to the module root. They now participate in default G01 discovery and its CI leg, rather than remaining opt-in. |
| Baseline, fault and drain listeners | Real new Scale interface plus explicit caller-owned post-ACK policy below. No upstream patch/fork or production provider. |
| G02 | Only shared pin and G01 controller-build compatibility expectations; no enrollment, provenance trust-root, credential or service behavior implemented. |
| Review | Independent Sol medium protocol/security review approved `586091b05b6c76c556c6538eeef63d259ffb40e7` with nonblocking Q1 below. Its correction needs focused delta review; exact-head GitHub Codex review remains pending. PR #106 approval does not cover this interface/pin migration. |

## Handler, ACK and continuation policy

The new SDK performs `Scale -> ACK`; it no longer acquires or calls lifecycle
methods. The experiment deliberately chooses this sequence:

1. Capture and durably journal the complete bounded poll and corroborated
   approved workflow source. Validate message identity, statistics and items
   against those durable facts inside Scale. Rejection stops before ACK.
2. Let the unmodified high-level listener request ACK. The adapter journals its
   exact intent/result under the original bounded approval context.
3. Only after known ACK, explicitly call supported acquisition, record the
   acquisition/continuation receipt chain, and process lifecycle/desired
   observations. These are application policy, not legacy SDK callbacks.

Keeping post-ACK acquisition preserves the existing receipt chain before worker
continuation; moving uncertain creation/acquisition into Scale would not make it
idempotent. The durable request identity narrows loss classification but never
proves GitHub acceptance/execution. A 202 explicitly retains the journaled
snapshot without declaring it fresh; SDK v0.5 itself supplies nil.

The post-ACK wrapper can return a continuation error through the SDK delete-call
boundary even when the ACK was known successful. Classify the actual operation
from its journal, not the SDK error's wording. There is no automatic retry or
claim that such an error means the ACK failed. All original-context cancellation,
authority, fsync, ownership and unknown-reservation gates remain in force.

## Preserved v0.4 evidence, not relabeled recipes

The original protocol/recovery/server tests are moved byte-for-byte into
`experiments/g01-scaleset/testdata/sdk-v04`; their original Go 1.26.9 manifests
are preserved. `compare-sdk.sh` runs these against v0.4.0 and the audited
pseudo-version, with the unchanged recovery helper. It does not run v0.5 tests
against the old API or rewrite historical results.

The large `g01-recovery-packet.md` remains byte-identical to the reviewed v0.4
packet at main `193c53d`:
SHA-256 `e2a0d876e47e9ced7565a5dbde083f72ae1aeb14a7ac01cc9fccf56abf377959`.
Its Go 1.26.9 wrapper and older Go 1.26.8 specimens are a frozen profile, not
current execution instructions. The current reusable profile is this ledger,
the checked-in v0.5 causal HTTP tests and the separately gated driver/worker
sources. Do not relabel old recipes by replacing version strings.

The parse-only profile checker now checks both the exact frozen packet digest
and its historical pin, and independently checks all active module pins plus
SDK v0.5.0. It does not disable a mismatch check or claim recipe execution. The
unchanged packet bytes reuse their earlier frozen static review; the expensive
whole-packet scan was not repeated. This profile/checker delta still needs review.

## Initial supplemental profile

The initial measurements below used the qualification record's scrubbed
environment, including `GOEXPERIMENT=none`. A subsequent real-binary check showed
that Go 1.27 stamps this non-default selection with a `-X:nojsonv2,...` suffix.
Those results remain supplemental evidence, **not stock release-ABI
qualification**. No credential/configuration environment was forwarded; SDK
traffic was synthetic fixtures only. No real runner/JIT/workflow or runtime
image operation occurred.

| Check | Actual result |
| --- | --- |
| Toolchain admission RED | New Go 1.27.3 expectation failed against the prior checker: module directive 1.27.3 rejected with expected 1.26.9. Exit 1, 0.479s. This does not claim the old checker accepted a false Go report. |
| Toolchain focused GREEN | `go test -count=1 -timeout=45s -run '^TestToolingCheckToolchainRequiresGo1273$' ./scripts`: exit 0, 0.905s after pin change; exact/prior/previous/newer versions covered. |
| Exact selection with newer system Go | Combined `TestToolingPinsNewerSystemGo` and `TestToolingCheckToolchainRequiresGo1273`: exit 0, 1.445s; controlled system Go reports 1.28.1 and the checker/Make select 1.27.3. |
| SDK integration RED | After compiling the new interface but before connecting the batch policy, `TestBaselinePinnedSDKAdmission/valid` failed with quarantine, ACK=0/acquire=0/continuation=0/forbidden=0. Exit 1, 0.461s; not a compile failure counted as RED. |
| Focused baseline/drain GREEN | `TestBaselinePinnedSDKAdmission` plus the held-poll drain control: exit 0, 0.594s after adapter connection. |
| Default v0.5 protocol package | `go test -race -count=1 -timeout=60s .` in G01: exit 0, 1.418s; 17 top-level tests. |
| Changed listener boundaries | Race selection of `TestBaselineV05HandlerValidatesDurableBatchBeforeACK`, `TestBaselinePinnedSDKAdmission`, `TestSupportedListenerBarriersAndReservation`, `TestNoMessageDoesNotCountAsCompletedBarrier`, `TestForeignIdentityAndUnreviewedWorkNeverACKOrDelete`, and `TestDrainListener.*`: exit 0, 2.354s. New five-case handler test rejects changed ID/statistics/request identity and canceled authority without ACK/acquisition/continuation. |
| G01 vet | `go vet ./...`: exit 0, no diagnostics. |
| Combined tagged compile | `go test -mod=readonly -run '^$' -tags=g01_live,g01_worker,g01_pair_fixture ./...`: exit 0 for all five discovered packages. Compile-only, not executed-test evidence. |
| Active G01 dependency/security delta | `go mod tidy -diff`, `go mod verify` and `govulncheck@v1.7.0 -test ./...`: exit 0, manifest clean, modules verified, no vulnerabilities found in the default test-inclusive source profile. Tagged/platform profiles remain separate. |
| G02 build-admission delta | Race selection `^(TestBrokerBuild.*\|TestPairedFixtureBuildRequiresExactGo1273)$` in G02: package exit 0, 1.361s. Other discovered packages had no selected tests; not claimed as test passes for them. |
| Controller/worker admission | Tagged `g01_live,g01_worker`, exact `TestBuildRevisionAdmissionRequiresGo1273`: race PASS, controller 1.336s and worker 1.325s; prior/shared and unexpected versions rejected. |
| Preserved legacy comparison | `compare-sdk.sh`: exit 0; v0.4.0 race 1.478s, audited pseudo-version race 1.381s; both vet passes. Three moved fixture files also matched old main bytes via cmp. |
| Packet profile classification | Prior dynamic comparison failed three of four assertions after the pin changed. New frozen-identity/active-profile checks: seven PASS, 0.008s, inert parse/read only. Packet unchanged. |
| Broader tooling observations | A five-test selector timed out at 90s during `TestToolingDefaultG02PartitionsRun`; its isolated observation also timed out at 180s. Neither is passing evidence. A final isolated 10m observation passed in 242.106s under the initial non-default profile. Product/child 45s limits were unchanged. This is tooling-routing evidence, not stock release-ABI qualification. |

## Stock release-profile correction and actual results

The real prepared-binary test at source
`69e33aa97ea47e25aa2dcc5cd167d57e452e79f3` failed in 3.231s with the initial
`GOEXPERIMENT=none` profile. The compiled artifact reported
`go1.27.3-X:nojsonv2,nogreenteagc,norandomizedheapbase64,nosizespecializedmalloc`;
its clean revision, SDK and tags matched, but the exact Go gate correctly
refused it. **No prefix/suffix allowance was added to the admission code.**

Canonical verification/builds use **`GOEXPERIMENT=` (release defaults)**, not
`none`. Other scrubbed settings remain: exact cached Go/compiler, public proxy
without direct fallback plus checksum database, GOENV/GOWORK/GOAUTH/telemetry off,
empty private-module selectors, GOMAXPROCS=2, GOFLAGS=-p=2, CGO_ENABLED=1,
GOOS=darwin, GOARCH=arm64 and GOARM64=v8.0. This is a per-command environment,
not a change to the user's global Go configuration.

| Canonical check at the same source | Actual result |
| --- | --- |
| `TestPairedBrokerPrepareReviewedG01LiveBinary`, race, 45s | PASS 11.568s. Built both fixture tag variants from the clean committed source, stored/reloaded the private receipts, validated exact Go 1.27.3/SDK v0.5.0/source SHA/tags/platform/cgo and unchanged byte digest. `go version -m` confirmed bare `go1.27.3`; this was build/read qualification, not live execution. |
| Default G01 protocol package, race, 60s | PASS 1.380s; all 17 top-level protocol tests. |
| Actual SDK adapter/fault/drain boundaries, race, 45s | PASS 3.892s; selector covers `TestPinnedSDK*`, `TestDriver.*PinnedSDK*`, the new handler gate, baseline admission, supported fault barriers, missing-message/foreign-work rejection and drain listener cases. |
| Tagged vet (`g01_live,g01_worker,g01_pair_fixture`) | Exit 0, no diagnostics. |
| govulncheck v1.7.0 | Exit 0/no vulnerabilities for G01 default test-inclusive source, `g01_live,g01_worker` source, and `g01_pair_fixture` livecanary source profiles. Other module scans are recorded separately when complete. |
| Other module security profiles | Root, G02 and R1 `govulncheck@v1.7.0 -test ./...`: each exit 0, no vulnerabilities found under release defaults. |
| Non-default-profile preflight | New `TestToolingCheckToolchainRequiresReleaseDefaults` first failed (exit 1, 0.515s): the old checker accepted GOEXPERIMENT=none because `go version` still reported base 1.27.3. The checker now rejects nonempty GOEXPERIMENT with a fixed safe message; CI sets it empty and the minimal v0.5 comparison exports release defaults. |
| Exact/profile-selection regression | Release-default, exact-patch and newer-system-Go tests passed together, 1.718s. Parse-only frozen-packet/current-profile checks also passed all seven tests, 0.007s. |
| Stock admission negatives | Controller/worker version tests: race PASS 1.332s/1.313s. G02 broker/paired metadata tests: race PASS 1.444s, then vet exit 0. Non-default `-X:nojsonv2` Go stamps are explicitly rejected without normalizing the actual version string. |
| Cadence-tagged source security | Test-inclusive `g01_live,g01_pair_fixture,g01_pair_real_cadence` cmd/livecanary govulncheck: exit 0, no vulnerabilities; not cadence execution evidence. |
| Go 1.27 tooling execution delta | Six focused checks covering GOFLAGS, JSON output, complete subtest matching, synthetic/spoofed TestMain output and skipped selections: PASS 10.007s under release defaults. The exhaustive stock root matrix remains postmerge CI evidence, not a local claim. |
| R1 shared-toolchain delta | Full R1 default race suite PASS 7.516s, vet exit 0. |
| Frozen SDK comparison under release defaults | `compare-sdk.sh` exit 0: SDK v0.4.0 race 1.370s, audited pseudo-version race 1.381s, both vet passes. The wrapper still explicitly selects Go 1.26.9; it is not the active G01 profile. |
| Updated minimal v0.5 wrapper | `compare-v05.sh` exit 0, race 1.414s, tidy/vet pass. It copies promoted tests correctly and explicitly selects empty GOEXPERIMENT/release defaults. |

The real-cadence bridge selector does **not** prove a 35-second run: it reaches
the existing controller provenance quarantine and skips before that cadence.
Neither package `ok` output nor the compiled cadence variant is a live or
real-cadence success. Verbose verification explicitly reported SKIP at the
broker-only provenance guard (case 0.15s; package 1.430s). Keep this gate unresolved.

Formatting, local document link targets, diff hygiene and new-artifact secret/
personal-path scans also passed. The generated Python bytecode cache was moved
out of the worktree and is not part of the candidate.

## Independent finding Q1: real message identity zero

The independent two-pass review of `586091b` approved the bounded experiment,
with one inherited nonblocking P2 qualification limit: the selected SDK documents
real messages numbered from zero, but baseline, fault and drain paths required
positive IDs. No unsafe effect was found; a legitimate first message could be
refused. This useful offline gap is corrected in the same batch, without a new
follow-up issue or any claim that the review covered the later fix.

Meaningful RED: `TestBaselineMessageZeroIsNotMissingOrNegative` accepted absent,
null and negative wire IDs, and
`TestBaselineZeroMessageTraversesJournalACKAndContinuation` rejected valid zero
before ACK/acquisition/continuation. Focused run exit 1, package 0.308s. The
previous positive-only downstream check concealed DTO defaulting; simply changing
`<= 0` to `< 0` would have admitted missing identities.

The correction requires a present nonnegative wire ID before SDK DTO decoding,
and the same required identity when decoding a durable batch. The transport reads
only successful capacity-bearing queue GETs under its existing decoded-body
budget, rejects duplicate/case-folded keys and read/close errors, and replays
identical bytes. Baseline ACK still binds the durable batch/source references;
zero needs an explicit known-cursor fact. Drain uses observed-message presence
instead of zero as absence. Both cursor guards retain the selected SDK's omission
of `lastMessageId` for zero, while rejecting missing prior batches and unacknowledged
work. Resource/request/runner IDs remain positive; synthetic initial `-1` is not
admitted as a wire message. No upstream SDK patch or response mutation is used.

Canonical release-profile checks after the correction:

- Focused SDK/baseline/drain/identity race selection: PASS 3.138s. A later new
  journal assertion initially failed because it incorrectly expected the generic
  result event's unused `Succeeded` flag; the assertion now checks the actual
  result-kind, session and ID contract. This was a test correction, not a product
  success reclassified from failure.
- Broader race regression, `go test -race -count=1 -timeout=120s -run
  '^Test(Baseline.*|DurableBatch.*|QueueIdentity.*|ZeroACK.*|GuardBaseline.*|Response.*|SharedTransport.*|PinnedSDK.*|Driver.*PinnedSDK.*|SupportedListenerBarriersAndReservation|NoMessageDoesNotCountAsCompletedBarrier|ForeignIdentityAndUnreviewedWorkNeverACKOrDelete|DrainListener.*)$'
  ./livecanary`: PASS **32.296s**. Covers valid zero through durable baseline and
  journaled drain ACK/acquisition, actual SDK fault barriers, omitted zero cursor,
  repeated zero, missing/null/negative/duplicate IDs, body errors/budget, and
  existing authority/ownership/wire safety controls. Synthetic HTTP only.
- `go vet -tags=g01_live,g01_worker,g01_pair_fixture ./...`: exit 0.
- `go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 -test ./livecanary`:
  exit 0, no vulnerabilities found. This is the changed source profile; earlier
  unchanged-module checks above are not relabeled as reruns.
- Exact `586091b` clean prepared-binary fixture verification also passed, 4.587s;
  that is the pre-Q1 source, not a claim that the correction was already built.

## Still required before merge / execution

The changed-surface admission/compile/tagged and vulnerability checks above have
completed. Obtain the focused independent Q1 delta review and exact final-head
GitHub review/PR quick checks before merge. The G02 prepared
binary fixture clones an immutable committed source, so its verification must
use the coherent local adoption commit, not silently build the old committed
source while checking new uncommitted expectations.

Then obtain independent protocol/security review of the exact candidate, push
one stable batch, consume PR quick and exact-head GitHub Codex review, triage all
findings, and merge only the reviewed head. Record the new main matrix separately.
No current passing hosted CI or review is claimed for this unpushed candidate.

G01 remains OPEN and Project In progress. Production provenance/trust-root
provisioning and conditional cleanup remain quarantined; private target/group,
runtime/archive inputs and concrete live authorization remain required. The
runner selection remains 2.337.0 pending its own authorized archive/runtime
qualification. Do not launch the separately planned canary from this change.

Rollback is reverting the adoption as one coherent unit (pins, admission,
adapters, test routing and current-profile docs), retaining the historical
evidence. There is no production state or live resource created by this work.
