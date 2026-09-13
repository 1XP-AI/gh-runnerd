# R1 manual credential identity fixture

This standalone Go module is the offline, fixture-backed credential slice for
issue #67. It validates one manually supplied in-memory RSA App key, the
configured App/organization installation/private repository identities and the
exact R1 permission profile before an optional metadata-only commit callback.

The adapter boundary receives only an App ID and public-key fingerprint. A
validated binding contains configuration metadata and permissions, never a PEM,
private key, JWT, installation token or worker bootstrap value. Source errors,
adapter errors and commit errors are normalized so their details cannot reach
callers.

Run the candidate checks from this directory:

```sh
GOTOOLCHAIN=go1.26.8 go test ./...
GOTOOLCHAIN=go1.26.8 go vet ./...
```

The module performs no filesystem, environment, Keychain, process, GitHub,
runner, Docker or Lima operation. It does not persist credentials or implement
the foreground command, worker handoff, live App/installation verification or
the broader G02 Manifest, multi-organization and launchd evidence gate.
