# G02 verify-only enrollment driver

Status: implemented and tested offline on Darwin ARM64 with Go 1.26.8; the approved live GitHub enrollment and same-App manual fallback were executed on 2026-09-17. This advances [issue #2](https://github.com/1XP-AI/gh-runnerd/issues/2) without closing its target-identity or persistent-credential gates. The disposable App and its installations were removed after verification; the pre-existing test App, runner, Keychain items and launchd services were not changed.

## Recorded live run (2026-09-17)

- The reviewed source `25f7e7e2ced2291a750bb63f1933f1aeed86b18f` was built and run on the intended arm64 macOS host. The existing self-hosted runner remained available throughout the run.
- The approved disposable App was created once with the disabled webhook, loopback Manifest and minimal organization-runner/metadata permissions. The strict same-port loopback request was accepted. Native browser form forwarding initially failed the exact Host/Origin contract; the generated one-time Manifest page completed the same reviewed flow without weakening validation or creating a duplicate App.
- The Manifest process reached `app_received`. After its bounded lifetime, a replacement key was passed through the protected manual input boundary for the same journal and App. App identity and both nominated organization installations, including account/target identity, suspension and minimal permissions, verified successfully: `verified_organizations=2`, `credentials_not_persisted=true`.
- Both exact disposable installations were removed and the disposable App registration was deleted by its owner. Follow-up organization inventory showed zero disposable installations; the pre-existing `1xp-gh-runnerd-test` installation remained untouched. The replacement PEM and probe-owned temporary access were removed after verification.
- This run does not establish production credential persistence, a distinct controller/job UID or release-signing continuity.

## Recorded current-login matrix (2026-09-17)

The reviewed source `25f7e7e2ced2291a750bb63f1933f1aeed86b18f` was exercised on the intended ARM64 macOS host. The synthetic probe ran from an exact one-shot GUI `launchd` parent with an owned transient label; direct SSH execution was not used as evidence because it has a different audit session. The executed source build passed `codesign --verify`; `codesign -d -v` reported `Identifier=a.out` and `TeamIdentifier=not set`, so this is an ad-hoc/source-build identity rather than Developer ID release evidence. Every row started with no busy runner worker, and no existing runner configuration or service was changed.

| Row | GUI state | Direct read | GUI launchd unlocked | GUI launchd after synthetic Keychain lock | Cleanup |
|---|---|---|---|---|---|
| Current login baseline | Unlocked | `0`, match | `0`, match, same UID | `-25293`, no match, same UID | Complete |
| Screen lock | Locked | `0`, match | `0`, match, same UID | `-25293`, no match, same UID | Complete |
| Unlock/resume | Unlocked | `0`, match | `0`, match, same UID | `-25293`, no match, same UID | Complete |
| Logout/login | Unlocked | `0`, match | `0`, match, same UID | `-25293`, no match, same UID | Complete |
| Reboot/login | Unlocked | `0`, match | `0`, match, same UID | `-25293`, no match, same UID | Complete |

The synthetic Keychain lock bit and unchanged Keychain preferences were verified in every row. Listener count changed from three to two after logout/reboot without a runner configuration change; no worker was present. This proves the reviewed source-build synthetic current-login contract across the authorized maintenance transitions for the selected single-login pilot. It does not prove hostile-code isolation, a persistent product launch agent/system daemon, pre-login cold-boot operation or production credential persistence.

## Implemented boundary

`experiments/g02-auth/cmd/g02-enroll` offers `manifest` and `manual`. Both require explicit `--live-github`; tests inject a synthetic adapter. The executable has no alternate API-origin setting, automatically opened browser, request access log, key output or persistent credential sink. It does not mint installation/runner tokens or run workers.

The Manifest path binds one ephemeral `127.0.0.1` listener for at most ten minutes. Requests have two-second header, five-second read and twenty-second write deadlines, an 8 KiB configured header bound and a 4 KiB local-form body bound. The callback inherits the strict 2 KiB query bound, exact Host/path, 256-bit state, one-time conversion and fifteen-second conversion deadline. Local mutation forms additionally require the exact local Origin, constant-time CSRF validation and unique expected fields. Callback Origin may be absent or exactly `https://github.com`. In the 2026-09-17 run, the generated one-time Manifest page reached `app_received` through the strict same-port callback, while the initial SSH-forwarded native form failed the exact Host/Origin contract. This is run-specific browser evidence, not a universal proof for every browser Origin variant. The driver returns `303 Location: /` on success and `303 Location: /callback-result` on an accepted-host callback rejection, with a fixed failure page. Neither destination retains code/state; direct `Attempt` tests retain their original HTTP rejection-status contract. Browser automation must wait for the clean destination before inspection and must not capture intermediate redirects/history/network data; a redirect does not erase browser-internal history. Pages use no-store, no-referrer, no scripts, no third-party resources and restrictive CSP. Shutdown waits at most three seconds for active requests, then cancels/closes them. These bounds are not a hostile same-UID process or total-memory isolation claim.

Before exposing GitHub's remote registration form, the driver flushes `registration_started` to the private journal. A callback records `conversion_started` before the single conversion request. It verifies the returned key against `GET /app`, including the exact proposed App slug, organization owner login, numeric ID and `Organization` type. Failed or interrupted conversion retains the proposed App name and available App ID; it never retries creation. GitHub allows the user to edit the proposed name: changing it causes verification refusal and requires reconciliation of that existing App.

After the owner installs the same App in both organizations, the local form accepts the two independently confirmed installation IDs. `ManualImport` rechecks App identity and both App-authenticated organization installation responses, including App/account/target/installation IDs, types, suspension and exact minimal permissions. Only a complete matching pair enters the in-memory sink. Success returns `credentials_not_persisted: true`, the verified count and non-secret App ID; the listener exits and drops its credential references. No token handoff occurs.

Manual mode accepts an existing App ID, both complete organization bindings and PEM from stdin. The executable refuses a terminal, foreign-owned input, group/other-accessible input or multiply-linked regular input. It accepts an owned protected regular descriptor or private pipe. This is a descriptor boundary: shell redirection may already have resolved a symlink, so it does not claim to validate an unavailable original filename. Input is limited to 32 KiB and thirty seconds, cancellation closes the input, and PEM is never copied to driver files. Both modes use the existing RSA/JWT validation. Clearing owned byte slices and dropping references does **not** guarantee erasure of RSA, HTTP decoder, Go runtime, swap or other memory copies. Same-UID hostile code remains outside this trust claim.

## Private non-secret inventory and recovery

The journal directory must be a current-UID directory with mode `0700`; a symlink at that directory is refused. `os.OpenRoot` captures the existing parent before creating/opening the child and confines subsequent operations. The existing parent is synced before writing any attempt record or exposing a form, including on reopen after a previous sync failure. A failed sync prevents startup. `active.lock` uses an exclusive non-blocking kernel lock; lock and journal files must be owned, regular, mode `0600`, and singly linked. Record writes use an exclusive temporary file, file sync, rename and directory sync. Nothing changes the current user's default directories, permissions outside this owned state, Keychain or services.

`attempt.json` stores only schema version, owner, App name, phase, App ID if known, and the two organization bindings. It contains no state, conversion code, key, JWT, remote error or callback URL. Its existence blocks every subsequent Manifest start, including after successful verification. Manual recovery requires the same owner/name/organization identities. A fresh `prepared` record leaves the candidate App ID unpinned until the key authenticates the exact proposed App; legacy `prepared` records may correct an unverified ID through the same journal. Every later phase with a recorded ID requires that same ID, including ambiguous Manifest conversion results and interrupted verification. See the [manual identity correction evidence](g02-manual-id-correction.md). The kernel releases the lock after process exit; the inventory remains. A corrupt record, interrupted `record.next`, unsafe ownership/mode, conflicting process or uncertain journal write fails closed. Do not delete the record or start with a different directory to bypass an ambiguous App creation.

Recovery is deliberately operator-driven: inspect the recorded owner/App name in GitHub; identify the one existing App and both installations; use its existing or replacement key through manual mode. If a `record.next` file exists, reconcile its non-secret inventory against GitHub and the original record before a separately reviewed correction. The driver does not auto-delete or auto-create remote resources. Browser back/reload can resubmit a remote form outside the local handler's control; submit once and reconcile any ambiguity. Keep the journal until exact remote cleanup is verified.

## Concrete procedure and approved resource shape

These values were used for the 2026-09-17 disposable evidence run. The remote resources were cleaned up afterward; retain the shape as the repeatable procedure for a separately authorized run:

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

An owner for both organizations must review this exact permission set. Parent coordination has read-only evidence that the current GitHub account is an administrator of both. The live run confirmed the intended host/session; distinct controller/job identity remains unconfirmed. Before any later live execution, record the exact reviewed source SHA, build binary, new private journal parent and existing owner App/installation inventory. Set `G02_PRIVATE_PARENT` to that explicitly selected existing private directory; do not place private inputs inside the checkout or public output directory. Use a fresh binary path and retain the journal across restarts.

From `experiments/g02-auth`, build after independent review:

```sh
GOTOOLCHAIN=go1.26.8 go build -o "$G02_PRIVATE_PARENT/g02-enroll-deab34" ./cmd/g02-enroll
```

The exact creation command used for the live run, and to execute only after final resource/permission approval on any later run, is:

```sh
"$G02_PRIVATE_PARENT/g02-enroll-deab34" manifest --live-github \
  --owner 1XP-AI --app-name gh-runnerd-gate-deab34 \
  --org 1XP-AI:258160258 --org 1XP-Inc:149097057 \
  --journal-dir "$G02_PRIVATE_PARENT/g02-attempt-deab34"
```

Open only the printed bare local URL. Submit the local preparation form, then the remote GitHub form once, preserving the approved App name. Confirm GitHub accepted the disabled webhook shape and minimal permissions. After the callback, install the same App in the two nominated organizations through GitHub's owner UI; record each installation ID privately and enter them into the local verification form. Do not capture callback URLs, form state, network bodies or credential screens in logs/screenshots. The command exits after verification; its key is unavailable for later G01 use.

For a manual-fallback test, have the owner obtain a key for this **same** disposable App into an existing protected private input, with confirmed non-secret IDs in the variables below. The harness does not create that file or recover its discarded Manifest key:

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
4. `go test -race -count=1 -timeout=45s ./...` passed: library `4.988s`, executable `2.020s`; `go vet ./...` passed. `go run ./cmd/g02-enroll --help` printed usage only. The combined repository `make check` also passed (both offline modules, formatting, vet, race tests, dependency/license checks and pinned vulnerability scan with no vulnerabilities found). The root bootstrap reported its expected explicit fuzz skip because it has no fuzz targets. Initial hosted CI passed; independent review found a missing parent-directory sync. A focused regression test first failed on accepting the unsynced directory, then passed after the fix, including retry of the same existing directory after injected sync failure and ordering before intent. Two additional red cases confirmed rejected/out-of-order callbacks remained on their query URLs; both now use the fixed failure redirect. The updated module race suite passed (library `5.656s`, executable `1.673s`) and vet passed. These are API-ordering/failure-injection and process-exit checks, not a reboot or physical power-loss experiment. Final independent re-review and hosted CI remain pending.

GitHub's [Manifest contract](https://docs.github.com/en/apps/sharing-github-apps/registering-a-github-app-from-a-manifest) documents organization registration, state, redirect and code exchange. The authenticated [App endpoint](https://docs.github.com/en/rest/apps/apps?apiVersion=2022-11-28#get-the-authenticated-app) supplies App/owner identity using the App JWT. These contracts justify the adapter shape; they do not prove the proposed disabled `.invalid` webhook, random loopback redirect, two-org installation or browser Origin behavior was accepted live.

Remaining gates: independent review and hosted Linux validation of this live continuation; production persistent credential storage; confirmed target/controller identity; release-signing and distinct-job-UID denial; persistent product startup/pre-login cold-boot behavior. The current-login maintenance matrix above is complete, but no persistent product service was installed. Prior limited synthetic Keychain evidence is unchanged. G02, G08 and G15 must not treat this verify-only result as installed-product support.
