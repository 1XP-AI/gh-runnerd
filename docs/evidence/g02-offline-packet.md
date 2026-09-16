# G02 offline enrollment packet

Status on 2026-09-17: **offline callback/adapter/import packet is implemented and tested; live GitHub Manifest enrollment, intended controller identity, and lock/logout/reboot evidence remain open**. This record does not mark issue [#2](https://github.com/1XP-AI/gh-runnerd/issues/2) Done and does not claim live enrollment or reboot/lock evidence passed.

This increment sits on current `main` after the earlier harness in [g02-enrollment-evidence.md](g02-enrollment-evidence.md) and the verify-only driver in [g02-live-driver.md](g02-live-driver.md). It does not replace those records.

## Existing offline inventory (already on main)

The `experiments/g02-auth` module is a gate experiment, not `gh-runnerd init` and not a production credential store. GitHub REST behavior stays behind the local `GitHubAPI` adapter pinned to `https://api.github.com` and REST version `2022-11-28`. No third-party GitHub SDK was added.

Already covered before this increment:

| Contract | Existing tests |
|---|---|
| Wrong, expired, and replayed callback state | `TestCallbackRejectsUntrustedRequests`, `TestCallbackIsConsumedBeforeConversionIncludingFailure` |
| Duplicate `state`/`code` and unknown callback parameters, including `installation_id` | `TestCallbackRejectsUntrustedRequests` |
| Wrong Host, localhost alias, encoded path, method, Origin | `TestCallbackRejectsUntrustedRequests`, `TestLoopbackListenerUsesItsActualRandomPort` |
| Forged App/org/installation IDs, suspension, extra permissions | `TestManualImportRejectsForgedBindingsBeforeStoringAnyCredential`, `TestManualImportRejectsExcessPermissions` |
| Manual import of two org bindings into an in-memory sink | `TestManualImportVerifiesTwoOrganizationsAndStoresOnce` |
| App JWT, redirects, bounded/malformed GitHub responses, Manifest conversion | `github_test.go` |
| Verify-only driver, journal recovery, protected manual input | `driver_test.go`, `cmd/g02-enroll` |

Callback-supplied installation IDs never authorize a binding. After conversion failure or replay, the handler tells the operator to inspect the existing App and use manual import. Restart invalidates local attempt state; it does not roll back a remote App.

## Gap closed by this increment

`ManualImport` previously stored credentials when GitHub omitted `suspended_at`, so suspension was unknown. The broker path already required `SuspensionKnown`. The Manifest HTML form previously inlined JSON without rejecting non-loopback `redirect_url` values or asserting that OAuth `callback_urls`/`setup_url` are absent.

## TDD sequence and actual results

Working directory: `experiments/g02-auth`. Toolchain: `GOTOOLCHAIN=go1.26.8`. No live GitHub, App, installation, runner, Keychain, launchd, Docker, or Lima operation was performed.

1. Baseline of the existing callback/import selectors passed (`ok` in 0.442s).
2. After adding `unknown suspension` and `EncodeManifest` tests, compilation failed with `undefined: EncodeManifest`.
3. An inert `EncodeManifest` stub that marshaled the current JSON without redirect checks produced the behavioral red results below.

```
--- FAIL: TestManualImportRejectsForgedBindingsBeforeStoringAnyCredential/unknown_suspension
    import_test.go: invalid import accepted: error=<nil> stores=1
--- FAIL: TestEncodeManifestRejectsNonLoopbackAndOmitsOAuthFields
    accepted untrusted Manifest redirect "http://localhost:43111/manifest/callback"
    accepted untrusted Manifest redirect "http://127.0.0.1/manifest/callback"
    accepted untrusted Manifest redirect "https://127.0.0.1:43111/manifest/callback"
    accepted untrusted Manifest redirect "http://127.0.0.1:43111/manifest/callback?x=1"
    accepted untrusted Manifest redirect "http://127.0.0.1:43111/other"
    accepted untrusted Manifest redirect "http://0.0.0.0:43111/manifest/callback"
    accepted untrusted Manifest redirect "http://127.0.0.2:43111/manifest/callback"
    accepted untrusted Manifest redirect "http://127.0.0.1:0/manifest/callback"
```

4. Minimal green: `ManualImport` now requires `SuspensionKnown && !Suspended`; `EncodeManifest` accepts only `http://127.0.0.1:<1-65535>/manifest/callback` and omits OAuth/setup fields; the driver renders that payload. Happy-path fixtures include explicit `suspended_at: null` or `SuspensionKnown: true`.
5. The enroll executable's protected-input success case then failed (`status 1 wanted 0`) because its synthetic installation JSON omitted `suspended_at`. That fixture now includes `"suspended_at":null`. The unknown-omission case remains rejected by the package tests.

Focused verification after the batch (not a full `make check` and not hosted CI):

```sh
GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=60s \
  -run '^(TestManualImport|TestEncodeManifest|TestGitHubAdapter|TestCallback|TestManifestDriverSyntheticEndToEndAndRestartRefusal|TestCredentialsRedactFormattingAndJSON)$' .
GOTOOLCHAIN=go1.26.8 go test -count=1 -timeout=90s -skip '^TestPaired' .
GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=45s ./cmd/g02-enroll
GOTOOLCHAIN=go1.26.8 go vet . ./cmd/g02-enroll ./cmd/g02-synthetic
GOTOOLCHAIN=go1.26.8 go run ./cmd/g02-synthetic
git diff --check
```

Actual results: focused race `ok` 1.260s; skip-paired `ok` 14.744s; enroll command race `ok` 1.290s; `go vet` exit 0; synthetic demo printed only:

```json
{"profile":"synthetic","verified_organizations":2,"in_memory_commit":true,"live_github":false}
```

`git diff --check` exit 0. No RSA PEM, callback state/code, token, webhook secret, client secret, raw provider body, or personal path is included here. The tagged macOS Keychain probe was not rebuilt or executed.

## Support matrix

| Scenario | Evidence | Current conclusion |
|---|---|---|
| Wrong/expired/replayed state, duplicate callback parameters, wrong Host | Existing passing tests plus this rerun of the focused selectors | Harness behavior verified offline |
| Forged installation/App/org IDs | Existing passing tests plus this rerun | Local validation contract verified |
| Unknown `suspended_at` / unknown suspension | New red (`stores=1`) then green | Import now matches the broker fail-closed rule |
| Disabled-webhook Manifest JSON, IPv4 loopback `redirect_url`, no OAuth callback/setup fields | New `EncodeManifest` tests | Offline payload contract verified; GitHub acceptance is not proven |
| Manual import of two synthetic org bindings | Existing tests plus `g02-synthetic` | Synthetic fallback works; real persistence remains unverified |
| Disabled-webhook Manifest with HTTP random-port redirect against GitHub | Not run | Unresolved live gate |
| Two actual org installations and granted permissions | Not run | Unresolved live gate |
| Intended controller UID/domain, signed release, screen lock, logout, reboot | Not run | Unresolved live gate |

## Manual-import contract

Manual import does not create an App. The operator supplies an existing App ID, PEM through a protected input, and independently confirmed organization/installation IDs. The adapter authenticates as that App, then `GET /app` and `GET /orgs/{org}/installation` must match App ID, installation ID, account ID/login/type, target ID/type, known non-suspended state, and the minimal permission profile (`organization_self_hosted_runners=write` and optional `metadata=read`) before the in-memory commit. A mismatch, unknown suspension, extra permission, or storage error stores nothing. Interrupted Manifest conversion uses this same import path; it must not start another registration.

## Skipped live gaps

- GitHub accepting `hook_attributes.active=false` with `https://example.invalid/gh-runnerd-g02-unused`.
- Browser redirect to the ephemeral `http://127.0.0.1:<port>/manifest/callback`.
- Owner installation into both nominated organizations.
- Production Keychain persistence, launchd identity, screen lock, logout, and reboot.

## Rollback

Offline rollback is a normal revert of this branch. No live App, installation, runner, Keychain item, or launchd service was created. Do not delete existing Apps or runners as a recovery step.

## User-run checklist (later explicit authorization only)

Do not run these steps from this packet. They require a reviewed immutable commit, owner approval of the exact disposable App/installations, and maintainer authorization for the concrete host. Follow [g02-live-procedure.md](g02-live-procedure.md) and [g02-live-driver.md](g02-live-driver.md) when that authorization exists.

1. Inventory existing owner Apps/installations privately; leave current manual runners unchanged.
2. Build the reviewed `g02-enroll` binary into an owned private directory; do not place keys in the checkout.
3. Run `manifest --live-github` once with the approved owner/name/org IDs and journal directory. Submit the GitHub form once. Do not capture callback URLs, codes, PEMs, or network bodies.
4. If conversion is interrupted or GitHub rejects the disabled-webhook shape, inspect the existing App inventory and stop. Use manual import for that same App; do not register another App.
5. After owner installation in both orgs, enter the independently confirmed installation IDs. Expect `credentials_not_persisted`.
6. Optionally repeat with `manual --live-github` and a protected PEM input for the same App.
7. Owner uninstalls the two exact installations, deletes the one disposable App, verifies absence, then removes only the probe binary/journal/key.
8. Separately authorized macOS identity work remains the tagged synthetic probe plus the lock/logout/reboot matrix in the live procedure. This packet does not authorize those operations.
