# G02 enrollment identity gate evidence

Date: 2026-09-22

## Finding and scope

This issue #2 continuation starts from `origin/main` commit
`4e9cf5b829650fb09feb8809df937b5037d1f485`. PR #59 inline finding
[r3957558157](https://github.com/1XP-AI/gh-runnerd/pull/59#discussion_r3957558157)
is the source for the runtime risk. GitHub reports the finding on commit
`6d304935f84ddff2f3a892c63a665e56f8077469`, originally anchored to
`4e41f7bbfb5ba1d1ae5514f7899d51fa8ed395cb`. The merged PR #59 withdrew an
unsafe readiness procedure; that documentation-only correction did not add a
runtime identity gate.

This change is limited to the verify-only `g02-enroll` command boundary. It is
not completion of G02 #2 or its parent Goal.

## Implemented boundary

- Production entry captures `os.Getuid()` and `os.Geteuid()` into an immutable
  value passed to the command path; there is no environment or CLI override.
- Except for exact read-only `--help` / `-h`, both modes require matching,
  nonzero UIDs representable by the `stat` UID field. Root, mismatched, negative
  or reserved-sentinel identities fail with the existing generic status-2
  diagnostic.
- The identity check precedes flag parsing, private input validation or read,
  default GitHub API construction, journal access and Manifest listener startup.
- Tests inject synthetic identities only at an unexported value-based boundary;
  they do not elevate privileges or alter the process identity.

## TDD and verification

The pre-gate red run used valid synthetic arguments and adapters for both modes:

```sh
cd experiments/g02-auth && GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=90s -run 'TestExecutableRejectsInvalidProcessIdentityBeforeEffects|TestExecutableAllowsHelpBeforeIdentityGate|TestManifestExecutableAcceptsMatchingNonRootIdentity|TestManualExecutableSyntheticAdapterAndPrivateStdin' ./cmd/g02-enroll
```

It failed as intended. Invalid-identity Manifest attempts returned status 1,
created journals and emitted the loopback-ready response. Invalid-identity
manual attempts returned status 0, created journals, consumed the protected
synthetic key input and reached the fake API four times. The same tests passed
after the gate was added.

Final focused package checks on the stable source candidate:

```sh
cd experiments/g02-auth && GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=120s ./cmd/g02-enroll
cd experiments/g02-auth && GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=120s ./cmd/g02-enroll
```

Both passed. The package tests exercise full synthetic manual verification and
Manifest callback/verification flows, redaction, help, root real/effective UID,
UID mismatch, unavailable/invalid UID, and absence of input, journal, fake API
and listener effects on rejection. `git diff --check`, `gofmt -d` and the
changed-source credential/private-key/personal-path scan also passed. No full
repository suite was run.

## Remaining evidence gates

No real GitHub App registration, organization API, macOS account, launchd,
Keychain, privilege transition, runner, Docker/Lima or workflow operation was
performed. This change does not establish real Manifest loopback acceptance,
manual-import ACL behavior, production credential persistence, signing
continuity, service/job identity separation, locked-Keychain behavior, logout
or reboot startup, or hostile-code isolation. Those remain explicit G02/ADR 0003
unknowns; native/live use remains gated.
