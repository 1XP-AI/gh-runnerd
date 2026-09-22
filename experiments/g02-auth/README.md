# G02 enrollment evidence harness

This separate Go module exercises [issue G02](https://github.com/1XP-AI/gh-runnerd/issues/2). It is a gate experiment, not the implemented `gh-runnerd init` command or a production credential store.

From this directory:

```sh
go test -race ./...
go vet ./...
go run ./cmd/g02-synthetic
```

The command generates a new RSA key, verifies two synthetic organization/installation bindings through a fake API, commits them once to an in-memory sink, and prints only a sanitized summary. It has no live mode. Tests make a local random-port HTTP request and use fake GitHub responses. No existing credentials are read, no App/installation/runner is created, and the synthetic demo saves no key. Driver input tests create only their own temporary synthetic PEM fixture. Go 1.26.8 is pinned; there are no third-party module dependencies.

The library contains:

- A ten-minute, 256-bit state attempt with exact IPv4 loopback Host/path checks, strict query parsing, single conversion consumption, bounded calls, no secret-bearing responses, and explicit manual recovery after failure.
- A fixed-origin `api.github.com` App-JWT adapter for App identity, organization installation lookup, and one Manifest conversion request. It rejects redirects, normalizes errors, bounds response bodies, and does not retry conversion. It pins supported REST API version `2022-11-28`.
- An in-memory manual-import boundary that validates the RSA key and all organization/App/installation identities before invoking an atomic storage sink. It refuses unknown or suspended installations, missing runner write permission, and extra permissions outside the minimal profile (runner write and optional baseline metadata read).
- `EncodeManifest` for the local registration form: IPv4 loopback `redirect_url` only, webhook delivery disabled, and no OAuth `callback_urls`/`setup_url`. The approved 2026-09-17 disposable run accepted that shape; production enrollment guarantees remain unproven.

A public struct containing a PEM must not become an application logging boundary merely because its formatting/JSON methods redact it. The key still exists in process memory; Go does not guarantee erasure of every copy. Same-UID malicious code is outside the protected trust claim.

## Verify-only enrollment driver

`go run ./cmd/g02-enroll --help` describes the executable Manifest/manual-input boundary. Its live modes require `--live-github` and explicit approval of the exact App and two organization installations. The approved 2026-09-17 disposable run completed Manifest enrollment and same-App manual fallback for both organizations; credentials were not persisted and all disposable remote resources were removed. Production credential persistence remains unimplemented. Tests run the loopback browser flow, GitHub adapter, private stdin boundary and crash recovery using only generated synthetic keys and local responses.

The journal directory must be owned by the current UID, mode `0700`, and not itself a symlink. The driver holds an exclusive file lock and retains only a non-secret App/organization inventory in `0600` files. Any existing attempt blocks new Manifest registration; matching manual import can recover the same App. Credentials are held only in memory until verification or exit. Normal output reports `credentials_not_persisted`; this is not a secure-memory-erasure guarantee.

The driver supports Darwin/Linux POSIX file ownership and locking for this experiment. It is not a Keychain importer, a product setup command or a token broker. See [the exact live proposal and offline evidence](../../docs/evidence/g02-live-driver.md).

## Optional synthetic macOS probe

The source at `cmd/g02-keychain-probe/main.go` is excluded from ordinary builds unless `-tags=g02runtime` is supplied. It creates a fresh private **file-based** Keychain, a generated 32-byte canary with an ACL for the same executable, and unique transient `gui/<uid>` launchd jobs. It disables interaction only for probe processes. It locks only the newly created Keychain, checks explicit access denial, deletes the Keychain, removes the exact owned jobs and temporary directory, and compares default/search-list metadata before and after. If service absence remains uncertain, it reports incomplete cleanup, attempts to lock only its synthetic Keychain, and retains the private recovery inventory instead of deleting it. It does not use a system daemon or a release signing identity.

Run only the reviewed immutable build in the authorized current-login experiment:

```sh
go build -tags=g02runtime -o "$G02_PROBE_BINARY" ./cmd/g02-keychain-probe
"$G02_PROBE_BINARY" --synthetic-current-login
```

`G02_PROBE_BINARY` must be a new absolute path in a private temporary directory. The probe's only other mode is an internal child read of its exact canary in the owned temporary directory. No UI, account creation, global Keychain lock/unlock, default/search-list setter, existing service mutation, screen lock, logout, or reboot occurs.

See the [evidence record](../../docs/evidence/g02-enrollment-evidence.md), [remaining live procedure](../../docs/evidence/g02-live-procedure.md), and [provisional decision](../../docs/decisions/0003-enrollment-and-service-identity.md). Skipped/unperformed live tests are not passing evidence.

## Broker-only workflow provenance gate

Controller and paired-terminal entrypoints require a one-shot signed
`BrokerProvenanceReceipt` before reading credential input. Its Ed25519 payload
binds the controller-approval SHA-256, workflow run ID, workflow ref and
commit, selected outer phase, owner nonce, receipt nonce, and named source.
The admission ledger retains the receipt digest, receipt nonce, and source so a
completed receipt cannot be replayed for another claim; direct regular-file,
FIFO, or tagged-entry paths without a receipt stop before credential input or
GitHub API effects.

The current GitHub REST response does not attest which workflow input selected
the controller phase or whether an ambiguous `head_branch` names a branch or a
tag, and the broker does not infer those facts from ordinary workflow metadata.
`BrokerProvenanceAdapter` is therefore an explicit signed fixture/adapter
contract: tests may inject a separately held key, while production constructs
no adapter and keeps controller/paired execution quarantined. The existing
workflow-run check is fail-closed for attested refs by accepting only
`refs/heads/...` until an authoritative ref-type field is available; tags and
pull refs are rejected. This check runs before token minting; no live workflow
or runner operation is part of this evidence.

Both provenance adapter calls receive the bounded approval/deadline context;
an adapter that waits for cancellation cannot outlive the parent, approval
expiry or the ten-minute provenance budget. Verification is therefore not a
context-free signature hook.
