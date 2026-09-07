# G01 offline Scale Set contract spike

The original contract package exercises the official SDK against a loopback-only
HTTP fixture. It is not a daemon, provider, live canary or persistent journal.
The contract tests register no real scale set or runner. The fake client
uses synthetic authentication and rejects connections to any address except its
own test server. Public dependency/toolchain downloads are separate from tests.

```sh
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=45s ./...
GOTOOLCHAIN=go1.26.8 go vet ./...
./compare-sdk.sh
```

`compare-sdk.sh` copies the module to a disposable directory and runs the same
suite against release `v0.4.0` and the immutable audited pseudo-version. It does
not change this module's pin. With dependencies cached, the tests are offline.
`GOTOOLCHAIN=go1.26.8` makes the verification toolchain exact; the module's Go
directive alone is a minimum and the toolchain directive alone is a suggestion.

The red commit contains a compiling callback-only recovery baseline. The green
implementation recovers aggregate desired demand and reference IDs through
supported SDK reads, while keeping unknown workers quarantined and admission
paused. It deliberately cannot prove reservation release, process ownership,
current activity, crash-safe persistence or server freshness. It fixes neither
SDK delivery guarantees nor the live gate by itself.

See [red evidence](../../docs/evidence/g01-red.md),
[contract evidence](../../docs/evidence/g01-contract.md),
[live-canary plan](../../docs/evidence/g01-live-canary.md) and
[ADR 0002](../../docs/decisions/0002-scaleset-integration.md).

A separate controller-only driver is available under the explicit `g01_live`
build tag. It is never run by these tests and has not run live. Its private
approval, authority split, bounded phases and remaining worker/live gaps are in
[the driver guide](../../docs/evidence/g01-live-driver.md). The original fixture
remains loopback-only and cannot be repointed to GitHub.
