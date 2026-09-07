# ADR 0003: Manual import first; login-scoped controller pending identity evidence

- Date: 2026-09-07
- Status: Provisional; G02's required live evidence is still open
- Scope: G02 experiments and the enrollment/service identity decision, not production implementation

## Decision

Implement App import as the fallback contract. The operator selects an existing App, supplies its private key through a protected input boundary, and confirms each organization's immutable identity and installation ID. Authenticate as that App, retrieve `GET /app` and `GET /orgs/{org}/installation`, and verify App ID, installation ID, account ID/login/type, target ID/type, suspension state and the minimal permission profile before persisting anything. Perform a single atomic credential/binding commit only after every organization passes. Callback-supplied installation IDs never authorize a binding. The experiment proves this ordering with generated RSA keys and fake authoritative responses; protected file input and a persistent credential store remain future implementations.

Use `organization_self_hosted_runners:write` with baseline metadata read and no repository Administration or Actions permissions. Organization owners must install this App because it requests organization permissions. The same App can be installed into both organizations when registration allows **Any account**. This does not require Marketplace listing or publishing private repositories. G02's import experiment intentionally refuses extra grants; separately justified features must revise this profile explicitly.

Keep Manifest assistance unavailable as a supported feature until GitHub accepts the tested disabled-webhook shape and actual `http://127.0.0.1:<random-port>/manifest/callback` redirect. A documented `redirect_url` field is not proof of this HTTP loopback behavior. OAuth redirect exceptions and App authorization `callback_urls` are different contracts. Each attempt gets random, one-time state with a short local deadline. Consume a valid callback before attempting code exchange. Network/conversion/storage ambiguity ends the attempt: inspect the existing App and recover through import, never silently register another App. Restart invalidates all local state; it does not roll back remote App creation.

Start with a non-root, login-scoped controller under its dedicated identity. Native jobs require separate job identities, normally one per organization, plus explicitly trusted repositories and verified runner-group admission. A same-controller-UID job is not a protected default. The pilot may need the controller user's active login session. Logout can stop its agent and interrupt native work; cold boot before login is unsupported. Do not disable FileVault or promise unattended reboot recovery.

Do not select a production Keychain backend solely from terminal success. Verify the real launchd domain, UID, code-signing requirement and locked behavior first. The synthetic probe targets a private file-based Keychain using `SecItem` with an explicit keychain/search list for each operation. This does not commit production to deprecated file-keychain lifecycle APIs. Data-protection Keychain is a candidate only for a correctly signed/provisioned user-context package. A future system daemon must use file-based Keychain and separately prove noninteractive access; it cannot reuse the data-protection Keychain assumption.

No privileged helper is implemented or approved by this ADR. If distinct job identities require one, G11 must review a narrow local helper contract: fixed executable/identity allowlist, validated owned worker directories and process groups, authenticated local peer, minimal environment, no arbitrary shell/command/path forwarding, and no management credential access. The networked controller remains non-root. Until this is demonstrated, the protected native profile stays gated.

## Evidence and limitations

The [G02 record](../evidence/g02-enrollment-evidence.md) distinguishes actual offline results from unperformed platform tests. The [live procedure](../evidence/g02-live-procedure.md) defines the remaining evidence and rollback. Green harness tests do not establish real App enrollment, real organization permissions, signed-release Keychain access, production atomic persistence, logout/reboot behavior or hostile-code isolation.

Official contracts reviewed:

- [GitHub Manifest registration](https://docs.github.com/en/apps/sharing-github-apps/registering-a-github-app-from-a-manifest): `redirect_url`, one-hour conversion deadline, state, and disabled-hook fields; `hook_attributes.url` is still documented as required.
- [GitHub installation requirements](https://docs.github.com/en/apps/using-github-apps/installing-a-github-app-from-a-third-party): owner approval for organization permissions and multi-account installation.
- [Organization runner API](https://docs.github.com/en/rest/actions/self-hosted-runners#create-a-registration-token-for-an-organization): organization self-hosted-runners write for registration credentials.
- [App installation lookup](https://docs.github.com/en/rest/apps/apps#get-an-organization-installation-for-the-authenticated-app): App JWT and authoritative organization installation fields.
- [Apple TN3137](https://developer.apple.com/documentation/Technotes/tn3137-on-mac-keychains): user-context data-protection Keychain versus file-based Keychain for daemons, with differing access-control models.
- [Apple launchd guide](https://developer.apple.com/library/archive/documentation/MacOSX/Conceptual/BPSystemStartup/Chapters/CreatingLaunchdJobs.html): per-user startup and termination at logout. Current target behavior still requires measurement.

## Narrow cgo/unsafe experiment exception

ADR 0001 prohibits first-party `unsafe` without a separately reviewed exception. For the build-tagged, synthetic-only `experiments/g02-auth/cmd/g02-keychain-probe/main.go`, this ADR proposes the smallest platform bridge needed to measure Apple's public Security framework contract. It is excluded from ordinary builds and the product. Independent Astra xhigh security review approved this exception before executing the controlled probe; it does not authorize importing this bridge into production. The limited current-login source-build result is recorded in the G02 evidence matrix.

The Go `unsafe.Pointer` uses are limited to synchronous calls that copy a fixed 32-byte synthetic canary into/out of C framework data, and freeing `C.CString` allocations. No Go pointer is retained by C, no arbitrary memory arithmetic is used, and Core Foundation results are type/length checked before copying. Created Core Foundation references and C strings are released by their owning scope. The bridge uses explicit owned Keychain references/paths, no UI, no default/search-list setters and no existing-item query. It does not access a real App key. Deprecated file-Keychain lifecycle APIs are used because this experiment specifically measures that implementation. A production backend, signing/distribution policy and any broader FFI surface require a new independent review.
