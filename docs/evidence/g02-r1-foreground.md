# R1 foreground credential configuration evidence

Status on 2026-09-16: **offline foreground document, worker-isolation,
fail-closed live-adapter and non-forgeable launch-capability slice complete;
the authorized GitHub App/API path, worker/JIT handoff and G02
Manifest/launchd gates remain open**. This record does not mark issue #67 or
G02 #2 Done. It does not use the 2026-09-16 private canary runner dispatch as
evidence for this path.

## Environment and scope

Working directory: repository root, then `experiments/r1-credentials` as noted.
Toolchain: `GOTOOLCHAIN=go1.26.8`. No third-party module was added. No live
GitHub, App, Keychain, launchd, runner, Docker, Lima or workflow operation was
performed. No private key was read from disk, environment or Keychain.

This slice extends the merged offline identity fixture from PR #77. It adds:

- strict parse of a non-secret one-organization foreground document
- refusal of missing, wrong, expired and partial credential setup
- declared foreground-only controller identity and lifecycle
- a worker launch plan that never inherits process environment and never
  receives management credentials or a JIT envelope
- `NewLiveGitHubAPI`, which fails closed instead of minting JWTs or calling GitHub

`cmd/gh-runnerd` remains an empty entry point. Production persistence, Keychain
import and unattended service identity are refused.

## TDD sequence and actual results

Failing tests were added against intentionally accepting stubs in
`foreground.go`, `worker.go` and `live.go`. The red command, run from
`experiments/r1-credentials` before replacing the stubs, was:

```sh
GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=45s -run 'TestParseForegroundDocumentRejectsMissingWrongAndPartialDeclarations|TestPrepareForegroundRejectsMissingWrongExpiredAndPartialSetup|TestPrepareForegroundRejectsUnrelatedValidKeyBeforeRemoteEffectsComplete|TestPlanWorkerLaunchKeepsManagementCredentialsOutOfWorkerSurfaces|TestPlanWorkerLaunchRejectsManagementCredentialAndJITMaterial|TestNewLiveGitHubAPIIsFailClosedWithoutAuthorization|TestPrepareForegroundCannotUseUnauthorizedLiveAdapter|TestPrepareForegroundBindsManualIdentityWithoutRetainingCredentials' ./...
```

Actual red result: FAIL (exit 1). Observable missing behavior, not a setup error:

- `ParseForegroundDocument` accepted empty, partial, extra-organization,
  credential-field, Keychain/file/environment persistence, launchd lifecycle,
  file source and public-repository documents (`err=<nil>`).
- `PrepareForeground` skipped `Validate`, returned no permissions, and accepted
  missing source, missing installation, Keychain persistence, launchd
  lifecycle, expired source, wrong installation and an unrelated valid key.
- `PlanWorkerLaunch` inherited the process environment, including a
  `GITHUB_APP_PRIVATE_KEY` canary, and accepted PEM env/argv/files plus JIT
  envelopes. The failure output included host process environment; that dump is
  not copied here.
- `NewLiveGitHubAPI` returned a non-nil adapter (`err=<nil>`).

The stubs were then replaced with fail-closed parsing, `Validate` reuse,
metadata-only worker planning and a live constructor that returns
`ErrLiveUnauthorized` without an API value.

Green commands from `experiments/r1-credentials` after the implementation:

```text
GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=45s ./...
  PASS  4.233s
GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=45s ./...
  PASS  6.809s
GOTOOLCHAIN=go1.26.8 go vet ./...
  PASS  (no output)
```

Smallest relevant broader check from the repository root:

```text
OFFLINE_EXPERIMENT_MODULE=experiments/r1-credentials bash scripts/check-offline-experiments.sh
  PASS  offline experiment checks passed: 1 module(s)  6.214s
git diff --check
  PASS
gofmt -d experiments/r1-credentials/foreground.go experiments/r1-credentials/foreground_test.go experiments/r1-credentials/worker.go experiments/r1-credentials/worker_test.go experiments/r1-credentials/live.go experiments/r1-credentials/live_test.go
  PASS  (no output)
```

The full repository suite, Public CI, Codex review and live profiles were not
run. No GitHub SDK dependency was added.

## Support matrix and live gaps

| Scenario | Evidence | Current conclusion |
|---|---|---|
| Missing/wrong/expired credential, wrong installation, partial setup | Passing module tests with generated in-memory RSA keys and fixture API | Offline refusal verified |
| Extra organization, embedded credential fields, public repository | Passing parse tests with `DisallowUnknownFields` | Offline refusal verified |
| File/environment/Keychain persistence | Passing parse/prepare tests | Implicit storage refused |
| Launchd/daemon lifecycle | Passing parse/prepare tests | Unattended service refused |
| Management credentials in worker env/argv/files | Passing worker tests; process env canary not inherited | Offline isolation verified |
| Forged or mutated `ValidatedBinding` | Passing external and package tests; `PlanWorkerLaunch` returns `ErrConfig` | Offline capability boundary verified |
| Direct `WorkerLaunchPlan` env/argv/files/JIT construction | Fields are unexported; package-private proof required | Offline capability boundary verified |
| Per-worker JIT bootstrap | `PlanWorkerLaunch` returns `ErrJIT` for any envelope | G01 live gap remains |
| Authorized GitHub App/API path (`GET /app`, `GET /orgs/{org}/installation`, installation-token repository corroboration) | `NewLiveGitHubAPI` returns `ErrLiveUnauthorized` and a nil API | Fail closed; no JWT, socket or SDK |
| `cmd/gh-runnerd` foreground command | Unchanged empty `main` | Not implemented |
| Manifest, multi-organization, launchd Keychain identity | Out of this slice | G02 #2 remains open |
| 2026-09-16 canary runner dispatch | Explicitly not used | Does not prove this path |

## P1 capability-boundary correction

Independent Luna max review of candidate `4a5fbc500acf103548f65ba71b819fd60dcd5afd`
found one blocking P1: exported `WorkerLaunchPlan` fields could be constructed
with env/argv/files/JIT, and `PlanWorkerLaunch` accepted a caller-forged
`ValidatedBinding` because it only normalized metadata.

The red command, run from `experiments/r1-credentials` after adding regression
tests and before the provenance/unexported-field change, was:

```sh
GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=45s -run 'TestPlanWorkerLaunchRejectsExternalForgedValidatedBinding|TestPlanWorkerLaunchRejectsExternallyMutatedValidatedBinding|TestValidatedBindingAndWorkerLaunchPlanCapabilitiesAreNotExternallyForgeable|TestPlanWorkerLaunchRejectsManagementCredentialAndJITMaterial' ./...
```

Actual red result: FAIL (exit 1). Observable missing behavior, not a setup error:

- a caller-constructed `ValidatedBinding` with valid-looking identity metadata was accepted (`err=<nil>`)
- mutating `AppID` on a package-validated binding was accepted (`err=<nil>`)
- a forged binding plus JIT envelope reached the JIT check (`err=worker JIT bootstrap is not available`) instead of being rejected as unvalidated
- `ValidatedBinding` had no package-private provenance state

The implementation adds package-private identity provenance on
`ValidatedBinding`, unexports `WorkerLaunchPlan` env/argv/files/JIT, and makes
`PlanWorkerLaunch` return only a proof-marked plan.

Green commands from `experiments/r1-credentials` after the implementation:

```text
GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=45s -run 'TestPlanWorkerLaunchRejectsExternalForgedValidatedBinding|TestPlanWorkerLaunchRejectsExternallyMutatedValidatedBinding|TestValidatedBindingAndWorkerLaunchPlanCapabilitiesAreNotExternallyForgeable|TestPlanWorkerLaunchRejectsManagementCredentialAndJITMaterial|TestPlanWorkerLaunchAcceptsExternallyHeldValidatedBinding|TestPlanWorkerLaunchRejectsDirectlyConstructedLaunchMaterial|TestPlanWorkerLaunchKeepsManagementCredentialsOutOfWorkerSurfaces' ./...
  PASS  0.579s
GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=45s ./...
  PASS  4.086s
GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=45s ./...
  PASS  7.048s
GOTOOLCHAIN=go1.26.8 go vet ./...
  PASS
```

From repository root:

```text
OFFLINE_EXPERIMENT_MODULE=experiments/r1-credentials bash scripts/check-offline-experiments.sh
  PASS  offline experiment checks passed: 1 module(s)  6.537s
git diff --check
  PASS
gofmt -d experiments/r1-credentials/credentials.go experiments/r1-credentials/credentials_test.go experiments/r1-credentials/credentials_external_test.go experiments/r1-credentials/worker.go experiments/r1-credentials/worker_test.go experiments/r1-credentials/foreground_test.go
  PASS  (no output)
```

The full repository suite, Public CI, Codex review, push, PR and live profiles
were not run.

## P2 triage (non-blocking)

These Luna max P2 findings were read and are not part of this fix:

- Request redaction: `WorkerLaunchRequest` still has no `String`/`GoString`/`MarshalJSON` redaction. Extra env/argv/files/JIT are refused before a plan is returned, and `WorkerLaunchPlan` already redacts. Routine P2; closed without a fix.
- Duplicate JSON keys: `ParseForegroundDocument` uses `encoding/json` last-key-wins. Unknown credential/org fields are already rejected; duplicate known keys are not a launch-capability forge. Routine P2; closed without a fix.
- Immutable red evidence: this correction records the exact red command and failure modes above rather than a separate red-only commit, matching the one focused local fix requested for this dispatch. Process P2; closed without a follow-up issue.

## Rollback

Revert the local commit on `orca/r1-credential-grok` that adds the foreground
slice and this capability-boundary correction. Do not use global Docker prune,
process kill, Keychain/launchd mutation, runner deletion or workflow replay.
There are no live resources created by this change.

Issue #67 stays open. Completing this slice does not complete G02.
