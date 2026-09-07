# G02 enrollment and credential evidence

Status on 2026-09-07: **offline harness complete; required live GitHub and target service-identity gates remain open**. This record does not mark issue G02 Done.

## Environment and scope

The harness was developed on Darwin ARM64, macOS 26.6.2 (25G83). Initial red tests used installed Go 1.25.8; final verification uses the isolated module's pinned Go 1.26.8. No third-party dependencies were added. The host's equivalence to the intended deployment Mac/controller account is not inferred from matching hardware specifications.

Only synthetic keys/configuration and local fake services were used for the offline tests. Existing runners, GitHub Apps/installations and credentials were untouched. There is no implemented product startup/auth service. The storage sink in this harness is an atomic interface exercised in memory, not a verified Keychain-backed product store.

## TDD sequence and actual results

Tests were written before implementing the callback and import behavior. The first `go test ./...` failed to compile because `NewAttempt`, `Candidate`, `Binding`, `Credential`, and installation contracts did not yet exist. To obtain observable failure evidence, minimal contract stubs were added without security validation. This command then ran the actual rejection tests:

```sh
go test enrollment.go enrollment_test.go import.go import_test.go
```

Actual red results, recorded before replacing the stubs:

- Wrong/expired state, duplicate state/code, wrong Host, loopback alias, wrong/encoded path, wrong method/origin, unknown parameters and malformed queries were accepted: `status=200 calls=1`.
- Twelve concurrent callback requests invoked conversion twelve times, in both successful and failing-conversion cases; expected one.
- Every forged binding, suspension/permission failure, malformed key and upstream error reached the storage sink: `stores=1`; expected zero.
- Storage errors were returned without normalization.

The implementation replaced these stubs, and the tests passed. Additional meaningful regression tests caught two issues before their fixes: credential JSON serialization was not redacted, and extra repository Administration permission was accepted. Both red results were recorded as explicit failures, then fixed.

The HTTP adapter tests were also authored before its implementation. They verify RS256 signatures using the generated key, issuer and timestamps, pinned API version, exact App/organization paths, zero redirect following, bounded/malformed response handling, code path validation, conversion response validation, and normalized errors.

Final commands from `experiments/g02-auth`:

```sh
go vet ./...
go test -race ./...
go run ./cmd/g02-synthetic
go build -tags=g02runtime -o "$G02_PROBE_BINARY" ./cmd/g02-keychain-probe
```

The last command is build-only; it is not runtime evidence. Actual synthetic command output:

```json
{"profile":"synthetic","verified_organizations":2,"in_memory_commit":true,"live_github":false}
```

Offline tests and race tests passed. A real TCP listener on `127.0.0.1:0` was exercised by an HTTP client, including rejection of a forged Host, and closed by test cleanup. This establishes local callback behavior only; GitHub and a browser did not participate. No RSA PEM, callback state/code, token, webhook secret, client secret, raw provider response or personal path is included in this record.

## Support/evidence matrix

| Scenario | Evidence | Current conclusion |
|---|---|---|
| Callback state/Host/path/query/origin/deadline rejection | Passing tests with fake clock and real local HTTP case | Harness behavior verified |
| Concurrent callback replay and conversion ambiguity | Passing race tests; one converter call | Same in-memory attempt is consumed once, including failure |
| Manual import of two org bindings | Generated keys, fake App-auth API, in-memory atomic sink | Synthetic fallback works; real persistence/import remains unverified |
| App JWT, wrong App/org/installation IDs, suspension, missing/excess permissions | Passing adapter/binding tests | Local validation contract verified |
| Disabled-webhook Manifest with HTTP random port redirect | Documentation only; not run against GitHub | Unresolved G02 gate |
| Two actual org installations and minimal granted permissions | Documentation only; not run | Unresolved G02 gate |
| Private file-Keychain/current source executable/current GUI login | Probe built, not yet executed in this revision | No runtime pass claimed |
| Intended controller UID/domain, signed release and binary update | Not run | Unresolved G02 gate |
| Screen lock, controller logout/login, host reboot/cold boot | Not run | Login-free boot unsupported; target behavior unverified |
| Dedicated controller/job identities and narrow helper | Decision only; no accounts/helper created | Protected native profile remains gated |

The local ten-minute state lifetime is intentionally shorter than GitHub's documented one-hour Manifest exchange limit. Normal top-level redirect GETs may omit Origin; state and exact Host/path are mandatory regardless. Accepting an absent Origin is not authorization. IPv6, localhost aliases, GHES/GHE.com, proxies, persistent setup listeners and cross-process resumable Manifest attempts are not covered.

## Official-source findings

[GitHub's Manifest instructions](https://docs.github.com/en/apps/sharing-github-apps/registering-a-github-app-from-a-manifest) distinguish App registration redirect, App authorization callback and post-installation setup. They provide a disabled-hook field but still describe its URL as required. This does not prove either omission of the webhook URL or HTTP loopback/random-port acceptance. No OAuth loopback guarantee was treated as Manifest evidence.

[Installation requirements](https://docs.github.com/en/apps/using-github-apps/installing-a-github-app-from-a-third-party) require owner approval for this organization-permission App. [Runner registration credentials](https://docs.github.com/en/rest/actions/self-hosted-runners#create-a-registration-token-for-an-organization) use organization self-hosted-runners write. [Organization installation lookup](https://docs.github.com/en/rest/apps/apps#get-an-organization-installation-for-the-authenticated-app) requires an App JWT; the harness validates its identity fields rather than trusting a callback installation ID. [JWT guidance](https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/generating-a-json-web-token-jwt-for-a-github-app) permits App ID as issuer, recommends a one-minute `iat` backdate and limits expiry to ten minutes into the future. The harness uses nine minutes. [API version policy](https://docs.github.com/en/rest/about-the-rest-api/api-versions) lists `2022-11-28` as supported through 2028-03-10; its use here is an explicit contract pin.

[Apple TN3137](https://developer.apple.com/documentation/Technotes/tn3137-on-mac-keychains) confines data-protection Keychain to user context and identifies file-based Keychain as the daemon option. The [launchd guide](https://developer.apple.com/library/archive/documentation/MacOSX/Conceptual/BPSystemStartup/Chapters/CreatingLaunchdJobs.html) describes per-user agent termination at logout. Neither source proves this project's packaging or actual machine behavior.

The synthetic probe uses [private temporary Keychain creation](https://developer.apple.com/documentation/security/seckeychaincreate(_:_:_:_:_:_:)) and [an ACL limited to the calling executable](https://developer.apple.com/documentation/security/secaccesscreate(_:_:_:)). Apple's published [StorageManager source](https://github.com/apple-oss-distributions/Security/blob/main/OSX/libsecurity_keychain/lib/StorageManager.cpp) explicitly avoids adding private keychains to the search list. The probe still checks default/search-list metadata before, during and after; it calls no setters for either.

## Remaining gates and rollback

Follow the [reviewable live procedure](g02-live-procedure.md). Do not mark a gate passed from a skipped test or from the source-built synthetic probe. No dependency that requires a completed G02 contract is released by this partial evidence.

Offline rollback is removal of this isolated experiment. The optional runtime probe removes only its captured fresh directory, explicit temporary Keychain reference and generated launchd labels; a cleanup invariant failure is an experiment failure. Live rollback removes only the inventoried disposable App/installation/key resources after operator verification, never existing Apps, runners or services. Manual recovery after interrupted Manifest conversion inspects the existing registration before any new attempt.
