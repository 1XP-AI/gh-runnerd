# R1 manual credential identity fixture

This standalone Go module is the offline, fixture-backed credential slice for
issue #67. It validates one manually supplied in-memory RSA App key, the
configured App/organization installation/private repository identities and the
exact R1 permission profile before an optional metadata-only commit callback.

The adapter boundary receives only an App ID and public-key fingerprint. Every
fixture identity call corroborates the same fingerprint and App identity. A
validated binding contains configuration metadata and permissions, never a PEM,
private key, JWT, token, installation token or worker bootstrap value. Source
errors, adapter errors and commit errors are normalized so their details cannot
reach callers. Only the package-created manual source capability is accepted;
the exported source shape and `SourceManual` value alone do not authorize a
caller-provided implementation.

Run the candidate checks from this directory:

```sh
GOTOOLCHAIN=go1.26.8 go test ./...
GOTOOLCHAIN=go1.26.8 go test -race -count=1 ./...
GOTOOLCHAIN=go1.26.8 go vet ./...
```

The stable repository gate also runs the nested module with package discovery:

```sh
bash scripts/check-offline-experiments.sh
```

## Evidence recorded for issue #67

On 2026-09-13, the following commands completed successfully from the module
directory or repository root as shown:

```text
GOTOOLCHAIN=go1.26.8 go test -count=1 ./...       PASS
GOTOOLCHAIN=go1.26.8 go test -race -count=1 ./... PASS
GOTOOLCHAIN=go1.26.8 go vet ./...                PASS
bash scripts/check-offline-experiments.sh        PASS (3 offline modules)
GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=15m ./scripts -run 'TestToolingEstablishedModulesAreRequired|TestToolingDefaultG01PartitionsRun|TestToolingDefaultG02PartitionsRun'  PASS
gofmt -d experiments/r1-credentials/credentials.go experiments/r1-credentials/credentials_test.go  PASS (no output)
git diff --check                                PASS
```

The red-first evidence is preserved in the preceding test commit: the exact
pre-fix command `GOTOOLCHAIN=go1.26.8 go test ./...` failed on wrong-valid-key,
unmarked-source, late-expiry, cancellation-boundary and typed-nil regressions;
the implementation is in the later commit. Fixtures generate ephemeral RSA
keys with `crypto/rand`, use synthetic identity/permission responses, and keep
all key bytes in memory. No credential, adapter response, private path or test
log is an artifact of this module.

## Ownership, ACL limits and live gap

The caller owns the manually supplied key bytes and is responsible for keeping
them in memory and out of logs and persistence. This boundary checks the
configured App ID, installation organization/suspension/permission metadata,
private repository owner and repository ID; the adapter owns the real API
authentication and must corroborate those identities with the same key
fingerprint. It does not grant, inspect or change GitHub ACLs, and native
same-user workdirs or Keychain access would not isolate hostile code.

The module performs no filesystem, environment, Keychain, process, GitHub,
runner, Docker or Lima operation. It does not implement the foreground command,
worker handoff, JIT or live App/installation/repository verification, and does
not satisfy the broader G02 Manifest, multi-organization or launchd evidence
gate. A reviewed maintainer dispatch is still required for any real Mac or
self-hosted runner test.

The metadata commit callback receives a context and must be context-aware and
transactional; this boundary checks cancellation and expiry before and after
the callback but cannot roll back an external side effect. On commit failure or
cancellation, the caller should discard any partial local state, revalidate the
credential and identities, and retry only through an independently reviewed
transactional adapter. No automatic workflow replay or live rollback is
performed here.
