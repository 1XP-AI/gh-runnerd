# G01 offline Scale Set contract spike

The original contract package exercises the official SDK against a loopback-only
HTTP fixture. It is not a daemon, provider, live canary or persistent journal.
The contract tests register no real scale set or runner. The fake client
uses synthetic authentication and rejects connections to any address except its
own test server. Public dependency/toolchain downloads are separate from tests.

```sh
cd experiments/g01-scaleset
GOTOOLCHAIN=go1.27.3 go test -race -count=1 -timeout=45s ./...
GOTOOLCHAIN=go1.27.3 go vet ./...
./compare-sdk.sh
```

Current selection is SDK `v0.5.0` and Go 1.27.3; the
[adoption ledger](../../docs/evidence/g01-sdk-v05-adoption.md) distinguishes local
validation, required reviews and live gaps. `compare-sdk.sh` instead copies the
frozen `testdata/sdk-v04` baseline plus unchanged recovery helper to disposable
modules and compares `v0.4.0` with the historical audited pseudo-version under
exact Go 1.26.9. It does not run the current v0.5 suite against an old API or
change the selected pin. With dependencies cached, tests are offline.
`GOTOOLCHAIN=go1.27.3` makes current verification exact; the module's Go
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

## SDK v0.5.0 isolated comparison

From the repository root, `sh experiments/g01-scaleset/compare-v05.sh` runs a
separate minimal Go 1.27.3/SDK v0.5.0 module against synthetic loopback fixtures.
Public toolchain/module downloads may occur. It copies the promoted root
client/listener/server tests, unchanged recovery helper and `testdata/sdk-v05`
manifests into a disposable module. Do not run either testdata directory directly.
Those 17 protocol tests now also run in the default G01 suite alongside adapter
tests; the old qualification record below describes their earlier opt-in state.
The large recovery packet remains frozen v0.4 evidence, not a current recipe;
the adoption ledger is the current profile. No command here authorizes live work.
See the [candidate evidence and limitations](../../docs/evidence/g01-sdk-v05-candidate.md).
