# G02 remaining live procedure

This is a concrete experiment proposal, not a report that these checks passed. Use reviewed immutable code, a dedicated trusted environment, explicit authorization for these exact resources and an organization owner available for both installations. This G02 harness intentionally has **no live enrollment CLI or persistent real-key importer**. A reviewed local driver must wire the already-tested `Attempt`, `GitHubAPI.Convert`, and `ManualImport` before the browser experiment. That driver and the persistent credential boundary are remaining work, not imaginary existing commands.

## A. Disposable GitHub enrollment proposal

| Field | Proposed value / required access |
|---|---|
| App owner | Operator-nominated disposable test personal account or test organization; owner name must be recorded privately before creation |
| App name | `gh-runnerd-g02-20260907-<fresh-six-hex-suffix>`; inventory this exact name before/after |
| Homepage | `https://github.com/1XP-AI/gh-runnerd` |
| Visibility | Any account (`public: true`) so the same App can be installed in both nominated test organizations |
| Permissions | Organization self-hosted runners read/write, baseline metadata read; no repository Administration, Actions, contents or user permissions |
| Events / webhook | No events; `hook_attributes.active: false` |
| Initial webhook URL shape | Include `https://example.invalid/gh-runnerd-g02-unused` to honor the documented required URL while targeting no real receiver; record whether GitHub accepts this exact disabled shape |
| Redirect | Actual address of one already-bound `tcp4`, `127.0.0.1:0` listener, with `/manifest/callback`; no proxy, localhost alias or OAuth callback |
| Authorization extras | No `callback_urls`, `setup_url`, user OAuth-on-install, client secret flow or public receiver |
| Network operations | Browser creation POST once, one code conversion POST, App-auth GETs for App and each org installation; do not request runner registration/JIT credentials |
| Installations | Two operator-nominated test orgs; owner authorizes the same App in each; chosen private test repositories where applicable |
| Cleanup | Owner uninstalls the two exact installation IDs, deletes the one exact disposable App, verifies absence, removes only its probe credentials/state |

A disabled webhook URL using `.invalid` is a deliberate safe test input, **not** a claim that GitHub accepts it. If GitHub refuses that shape before registration, record the refusal and inspect the nominated owner's App inventory. Do not silently resubmit a changed Manifest. A follow-up no-URL shape (`hook_attributes: {"active": false}`) is a separate explicit experiment after confirming no App was created. If neither shape works, manual registration with the Webhook Active box cleared is the fallback; do not add a publicly reachable webhook server to make the test pass.

Exact initial Manifest, substituting only the recorded name and actual listener port:

```json
{
  "name": "gh-runnerd-g02-20260907-<fresh-six-hex-suffix>",
  "url": "https://github.com/1XP-AI/gh-runnerd",
  "redirect_url": "http://127.0.0.1:<actual-port>/manifest/callback",
  "public": true,
  "hook_attributes": {
    "active": false,
    "url": "https://example.invalid/gh-runnerd-g02-unused"
  },
  "default_permissions": {
    "organization_self_hosted_runners": "write",
    "metadata": "read"
  },
  "default_events": [],
  "request_oauth_on_install": false
}
```

1. Privately inventory the nominated owner's App names/IDs and existing installation IDs. Record only sanitized aliases and counts in the public evidence. Confirm owner access for both test orgs and repository/runner-group restrictions. Leave existing manual runners unchanged.
2. Start the reviewed local driver with one random-port loopback listener, 10-minute state deadline, 15-second conversion bound, request/header limits, disabled request access logs, and a shutdown deadline. Do not start a reusable or externally bound enrollment server. The browser form sends the JSON under `manifest` and state to GitHub's personal or organization App registration endpoint documented for that owner.
3. Click creation once. Observe the GitHub validation and exact disabled-webhook configuration. Do not capture a raw callback URL, state, code, PEM, webhook/client secret, network dump or API body in screenshots/logs. Public evidence records only accepted/refused, scheme/host/path template, ephemeral port boolean, and normalized result.
4. On redirect, validate the request before a single `POST /app-manifests/{code}/conversions`. Retain only the App ID and private key in protected process memory long enough to verify `GET /app`; discard unneeded response fields. A future real storage sink must be independently verified before persistence. A 4xx, 5xx, timeout, closed browser/process or storage ambiguity never triggers another creation POST. Inspect the exact existing App, then use import/rekey or delete that disposable App.
5. Have each organization owner install the same App. Privately confirm each org's stable numeric ID and installation ID independently of the callback. For each intended org, use the imported App JWT for `GET /orgs/{org}/installation`. Check matching App/account/target/installation identities, `Organization` types, no suspension and the exact minimal granted permissions. A swapped org/installation ID must fail before any commit. Do not infer authorization from a browser `installation_id` query.
6. Exercise manual fallback on this same disposable App: generate a replacement private key through its settings if necessary; pass it through the reviewed protected input boundary, never argv/environment/clipboard/chat. Verify the same App and both orgs before committing. Test failure on the second org leaves neither partial binding nor credential. Remove the superseded disposable key only after verification. No extra App is created.
7. The owner removes the two inventoried installations and the one inventoried disposable App. Shut down the exact listener and delete only the probe-owned credentials/state. Verify no extra App or installation was created and pre-existing runners remain untouched. Publish sanitized counts, permissions, normalized statuses and any remaining failures.

## B. Current-session synthetic Keychain/launchd probe

The build-tagged command is the only executable platform experiment supplied by this PR. It uses no GitHub credentials and no installed persistent agent:

```sh
cd experiments/g02-auth
go build -tags=g02runtime -o "$G02_PROBE_BINARY" ./cmd/g02-keychain-probe
"$G02_PROBE_BINARY" --synthetic-current-login
```

Prepare `G02_PROBE_BINARY` as a new absolute path in a private temporary build directory. Confirm the exact source commit and binary code-signing identity with `codesign`; an ad-hoc source build is a separate matrix row from a Developer ID release. The probe creates a private Keychain/canary, restricts reads to that explicit Keychain, runs fresh child reads through unique transient `gui/<current-uid>` jobs, locks only its own Keychain, expects `errSecInteractionNotAllowed`, then deletes its exact Keychain/jobs/temp state. Its output contains status codes and booleans only. Failure to preserve Keychain preferences or clean up is failure, not a passing access result.

The root task must review this concrete probe's ownership, no-UI handling, explicit paths and cleanup before its first execution. Re-running the same reviewed probe is within the same non-disruptive authorization. This experiment does not require or establish screen lock, logout, reboot, a system daemon or target-controller identity.

## C. Target identity and startup matrix still required

Required access: confirmed intended host; confirmed dedicated non-root controller and distinct job accounts already provisioned by an authorized administrator; maintainer-controlled reviewed source and release-signed binaries; signing identity/provisioning access if data-protection Keychain is chosen; and an operator present for explicitly scheduled lock/logout/reboot checks. Creating these accounts, installing a persistent service/helper, unlocking existing Keychains or changing FileVault is outside the current probe authorization.

1. Inventory the intended UID, launchd domain, selected Keychain implementation and executable signing requirement privately. Record only role aliases and `same/different UID` booleans publicly. Use a fresh synthetic item with the same item access-control policy and service identity the product will use; an arbitrary terminal or `security` CLI read does not stand in for this binary.
2. As the real controller launchd process, read while unlocked with interaction disallowed; verify a signing operation against the corresponding synthetic public key without printing the key. Test denied access from the distinct job identity; never broaden the ACL to make it pass.
3. With the operator present and maintenance agreed, test **screen lock separately from Keychain lock**. The actual selected Keychain's behavior determines whether admission should pause. A file-Keychain item may remain accessible when the screen locks; record actual behavior. Locked/unavailable credential access must return a normalized refusal with no prompt or worker admission.
4. Test controller logout/login and repeat reads after restart. The login-scoped design makes no continuity promise across logout. Confirm the product does not accept work during absent/locked credentials and does not silently start under another account.
5. In an explicitly scheduled maintenance window after busy jobs drain, reboot without changing FileVault. Observe pre-login behavior and after authorized login. Cold-boot login-free operation remains unsupported until separately demonstrated; failed jobs are not automatically replayed.
6. Repeat source build, stable signed release, and updated binary identity. Verify that an update neither silently broadens trust nor loses access without a clear recoverable status. A system-daemon row is a separate file-Keychain experiment and needs its own reviewed service installation and authorization.
7. Remove only test items, owned service entries and test directories; restore the original reviewed service configuration if this maintenance explicitly changed it. Verify original runner/service availability and publish a sanitized matrix with every failed/skipped case visible.

G02 remains open until required live evidence, independent review and repository publication/merge criteria are satisfied. G08/G15 and protected native work must not treat this procedure as passing results.
