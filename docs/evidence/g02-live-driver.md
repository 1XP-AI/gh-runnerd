# G02 verify-only enrollment driver

Status: implemented and tested offline on Darwin ARM64 with Go 1.26.8; **live GitHub enrollment has not been executed**. This advances [issue #2](https://github.com/1XP-AI/gh-runnerd/issues/2) without closing its live, target-identity or persistent-credential gates. No App, installation, runner, Keychain item or launchd service was created by this continuation.

## Implemented boundary

`experiments/g02-auth/cmd/g02-enroll` offers `manifest` and `manual`. Both require explicit `--live-github`; tests inject a synthetic adapter. The executable has no alternate API-origin setting, automatically opened browser, request access log, key output or persistent credential sink. It does not mint installation/runner tokens or run workers.

The Manifest path binds one ephemeral `127.0.0.1` listener for at most ten minutes. Requests have two-second header, five-second read and twenty-second write deadlines, an 8 KiB configured header bound and a 4 KiB local-form body bound. The callback inherits the strict 2 KiB query bound, exact Host/path, 256-bit state, one-time conversion and fifteen-second conversion deadline. Local mutation forms additionally require the exact local Origin, constant-time CSRF validation and unique expected fields. Callback Origin may be absent or exactly `https://github.com`; actual browser behavior remains a live test. Pages use no-store, no-referrer, no scripts, no third-party resources and restrictive CSP. Shutdown waits at most three seconds for active requests, then cancels/closes them. These bounds are not a hostile same-UID process or total-memory isolation claim.

Before exposing GitHub's remote registration form, the driver flushes `registration_started` to the private journal. A callback records `conversion_started` before the single conversion request. It verifies the returned key against `GET /app`, including the exact proposed App slug, organization owner login, numeric ID and `Organization` type. Failed or interrupted conversion retains the proposed App name and available App ID; it never retries creation. GitHub allows the user to edit the proposed name: changing it causes verification refusal and requires reconciliation of that existing App.

After the owner installs the same App in both organizations, the local form accepts the two independently confirmed installation IDs. `ManualImport` rechecks App identity and both App-authenticated organization installation responses, including App/account/target/installation IDs, types, suspension and exact minimal permissions. Only a complete matching pair enters the in-memory sink. Success returns `credentials_not_persisted: true`, the verified count and non-secret App ID; the listener exits and drops its credential references. No token handoff occurs.

Manual mode accepts an existing App ID, both complete organization bindings and PEM from stdin. The executable refuses a terminal, foreign-owned input, group/other-accessible input or multiply-linked regular input. It accepts an owned protected regular descriptor or private pipe. This is a descriptor boundary: shell redirection may already have resolved a symlink, so it does not claim to validate an unavailable original filename. Input is limited to 32 KiB and thirty seconds, cancellation closes the input, and PEM is never copied to driver files. Both modes use the existing RSA/JWT validation. Clearing owned byte slices and dropping references does **not** guarantee erasure of RSA, HTTP decoder, Go runtime, swap or other memory copies. Same-UID hostile code remains outside this trust claim.

## Private non-secret inventory and recovery

The journal directory must be a current-UID directory with mode `0700`; a symlink at that directory is refused. `os.OpenRoot` confines subsequent operations to the captured directory. `active.lock` uses an exclusive non-blocking kernel lock; lock and journal files must be owned, regular, mode `0600`, and singly linked. Record writes use an exclusive temporary file, file sync, rename and directory sync. Nothing changes the current user's default directories, permissions outside this owned state, Keychain or services.

`attempt.json` stores only schema version, owner, App name, phase, App ID if known, and the two organization bindings. It contains no state, conversion code, key, JWT, remote error or callback URL. Its existence blocks every subsequent Manifest start, including after successful verification. Manual recovery requires the same owner/name/organization identities, plus a matching App ID when already recorded. The kernel releases the lock after process exit; the inventory remains. A corrupt record, interrupted `record.next`, unsafe ownership/mode, conflicting process or uncertain journal write fails closed. Do not delete the record or start with a different directory to bypass an ambiguous App creation.

Recovery is deliberately operator-driven: inspect the recorded owner/App name in GitHub; identify the one existing App and both installations; use its existing or replacement key through manual mode. If a `record.next` file exists, reconcile its non-secret inventory against GitHub and the original record before a separately reviewed correction. The driver does not auto-delete or auto-create remote resources. Browser back/reload can resubmit a remote form outside the local handler's control; submit once and reconcile any ambiguity. Keep the journal until exact remote cleanup is verified.

## Concrete proposal for later approval

These are the root-owned proposal's nominated resources, not evidence of their creation:

| Item | Proposed value |
|---|---|
| Owner | `1XP-AI`, organization ID `258160258` |
| App name | `gh-runnerd-gate-deab34` |
| Installations | `1XP-AI` / `258160258`; `1XP-Inc` / `149097057` |
| Visibility | Public App, enabling installation in both organizations |
| Permissions | Organization self-hosted runners write; baseline metadata read only |
| Webhook/events | Disabled; no events; `https://example.invalid/gh-runnerd-g02-unused` |
| Homepage | `https://github.com/1XP-AI/gh-runnerd` |
| Redirect | Actual ephemeral `http://127.0.0.1:<port>/manifest/callback` |
| Additional authority | None: no user OAuth, repository Actions/contents/admin, tokens, runner registration or workflow dispatch |

An owner for both organizations must review this exact permission set. Parent coordination has read-only evidence that the current GitHub account is an administrator of both. Actual host identity is still unconfirmed; this driver can test only the current session. Before live execution, record the exact reviewed source SHA, build binary, new private journal parent and existing owner App/installation inventory. Set `G02_PRIVATE_PARENT` to that explicitly selected existing private directory; do not place private inputs inside the checkout or public output directory. Use a fresh binary path and retain the journal across restarts.

From `experiments/g02-auth`, build after independent review:

```sh
GOTOOLCHAIN=go1.26.8 go build -o "$G02_PRIVATE_PARENT/g02-enroll-deab34" ./cmd/g02-enroll
```

The proposed creation command, to execute **only after final resource/permission approval**, is:

```sh
"$G02_PRIVATE_PARENT/g02-enroll-deab34" manifest --live-github \
  --owner 1XP-AI --app-name gh-runnerd-gate-deab34 \
  --org 1XP-AI:258160258 --org 1XP-Inc:149097057 \
  --journal-dir "$G02_PRIVATE_PARENT/g02-attempt-deab34"
```

Open only the printed bare local URL. Submit the local preparation form, then the remote GitHub form once, preserving the approved App name. Confirm GitHub accepted the disabled webhook shape and minimal permissions. After the callback, install the same App in the two nominated organizations through GitHub's owner UI; record each installation ID privately and enter them into the local verification form. Do not capture callback URLs, form state, network bodies or credential screens in logs/screenshots. The command exits after verification; its key is unavailable for later G01 use.

For the separate manual-fallback test, have the owner obtain a key for this **same** disposable App into an existing protected private input, with confirmed non-secret IDs in the variables below. The harness does not create that file or recover its discarded Manifest key:

```sh
"$G02_PRIVATE_PARENT/g02-enroll-deab34" manual --live-github \
  --owner 1XP-AI --app-name gh-runnerd-gate-deab34 --app-id "$G02_APP_ID" \
  --org "1XP-AI:258160258:$G02_INSTALLATION_AI" \
  --org "1XP-Inc:149097057:$G02_INSTALLATION_INC" \
  --journal-dir "$G02_PRIVATE_PARENT/g02-attempt-deab34" < "$G02_PRIVATE_PEM"
```

Do not provide a key through arguments, environment values, clipboard or chat. G01 token brokering requires its separately reviewed input boundary and later approval; this tool grants no such handoff. If GitHub rejects `.invalid` with a disabled webhook, or the browser rejects the redirect, stop and reconcile the exact App inventory. Do not silently resubmit an altered Manifest. Cleanup remains with the owner: uninstall the two exact IDs, delete the one exact disposable App, verify their absence and then remove only the captured probe key, journal and binary. Coordinate timing with the root's separately approved follow-on experiments; preserve inventory until they are complete.

## Actual offline evidence

All fixture keys were freshly generated synthetic RSA keys. HTTP integration tests used a real ephemeral local listener plus injected API responses; the command test exercised the real JWT adapter over an injected transport. No test contacted GitHub.

1. Initial authored end-to-end tests failed to compile with the missing driver contracts. After inert contracts were added, all six initial cases failed at the absent behavior (`driver not implemented`), including successful Manifest/manual flows and their security/recovery fixtures.
2. Before the executable/App-owner adapter implementation, the manual executable test failed its protected-input success case, `DescribeApp` failed to retain owner identity, and an out-of-order callback regression test demonstrated a consumed state followed by a rejected legitimate conversion. These were observed red results, then corrected.
3. The final local module race suite passed, including wrong Host/Origin/CSRF, duplicate fields/registration, oversized form/query rejection, out-of-order callback, conversion ambiguity, forged second installation, forged App owner/name, same-App manual recovery, unsafe journal input, concurrent ownership, durable-write failure, canceled input, listener shutdown and a subprocess exiting without closing its journal. The subprocess proved fresh creation stays blocked while the released kernel lock permits same-App manual recovery.
4. `go test -race -count=1 -timeout=45s ./...` passed: library `4.988s`, executable `2.020s`; `go vet ./...` passed. `go run ./cmd/g02-enroll --help` printed usage only. The combined repository `make check` also passed (both offline modules, formatting, vet, race tests, dependency/license checks and pinned vulnerability scan with no vulnerabilities found). The root bootstrap reported its expected explicit fuzz skip because it has no fuzz targets. Independent review and hosted CI are still pending at this commit.

GitHub's [Manifest contract](https://docs.github.com/en/apps/sharing-github-apps/registering-a-github-app-from-a-manifest) documents organization registration, state, redirect and code exchange. The authenticated [App endpoint](https://docs.github.com/en/rest/apps/apps?apiVersion=2022-11-28#get-the-authenticated-app) supplies App/owner identity using the App JWT. These contracts justify the adapter shape; they do not prove the proposed disabled `.invalid` webhook, random loopback redirect, two-org installation or browser Origin behavior was accepted live.

Remaining gates: independent review and hosted Linux validation of this continuation; actual GitHub Manifest/manual fallback in both organizations; production persistent credential storage; confirmed target/controller identity; release-signing and distinct-job-UID denial; lock/logout/reboot startup matrix. Prior limited synthetic Keychain evidence is unchanged. G02, G08 and G15 must not treat this verify-only result as installed-product support.
