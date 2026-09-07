# G01 controller credential broker experiment

Status: implemented and tested with **synthetic HTTP and subprocess fixtures only**. No live token mint, registration/admin credential request, tenant request, controller phase, App/installation mutation, workflow dispatch, runner or worker operation has run. The broker is a bounded G01 prerequisite, not G08 credential storage or completion of G01/G02. All future live use needs independent code review and separate exact resource/permission approval.

## Causal order and authority

The broker has two explicit modes in its private approval: `discover-actions-host` and `controller`. Both authenticate the existing App using the audited G02 RSA/JWT and App/installation identity utilities. App ID, exact App slug, owner login/ID/type, organization login/ID, installation/App/account/target identities, explicit known suspension status and minimal installation permissions must match before any token request. The small shared API extension exposes whether `suspended_at` was supplied; the broker refuses an unknown suspension state.

Runner-group and private repository inspection require an installation/user authority; App JWT alone cannot perform those checks. The agreed executable sequence is therefore:

1. Verify App, owner and organization installation with the App JWT.
2. Flush a non-secret token issuance intent.
3. Request **one** installation token with `repository_ids: [exact_canary_id]` and exactly organization self-hosted runners write plus metadata read.
4. Validate the actual token response: token syntax, expiry between one and 65 minutes, exact permissions, selected repository selection and the one matching private non-fork repository. App/installation facts come from authenticated identity reads; expiry/permissions come from this issuance response, not operator-supplied token metadata.
5. Use that token to independently verify exactly one installation-accessible repository, its current owner/name/ID/private/non-fork identity, the exact nondefault/noninherited named runner group, public repositories explicitly disallowed, selected-only visibility and that one repository selected.
6. Only after those checks, perform the separately approved temporary authentication discovery or hand credentials to the exact reviewed controller child.

Repository restriction does **not** cryptographically restrict the organization-level self-hosted-runners permission to one runner group. The broker and reviewed controller enforce the selected canary policy, under the trusted-controller assumption. This boundary assumes a trusted controller UID and host administrators; file modes and a stdin pipe do not isolate either. No second organization-read credential is required. The broker never adds Actions permission to the G02 App or mints the separate workflow authority.

A failed/ambiguous token request, failed policy check, expired credential, uncertain handoff or error leaves the intent and stops. There is no automatic retry, token refresh, token replacement or automatic revocation. An issued token may remain valid until GitHub expires it even if no handoff occurs. A future operator approval must include that issuance, not just the eventual read/phase. The maximum remaining operation lifetime is ten minutes, approval expiry, or one minute before the actual installation-token expiry, whichever occurs first.

## Discovery is an authentication effect

`discover-actions-host` follows only the credential exchange observed in pinned official Scale Set SDK `v0.4.0` source:

- `POST /orgs/{approved-org}/actions/runners/registration-token` using the scoped installation token.
- `POST https://api.github.com/actions/runner-registration` using `RemoteAuth` with that temporary registration credential and the fixed body `{"url":"https://github.com/{approved-org}","runner_event":"register"}`.

The authenticated response supplies a tenant URL and admin credential. The broker validates the HTTPS URL, absent userinfo/query/fragment, allowed port and DNS labels under `.actions.githubusercontent.com`. It returns **only the hostname**, then drops registration/admin credential references. It never contacts the returned tenant and never sends an Actions admin token to a child. This discovers the exact hostname for a later allowlist approval before Scale Set mutation. It is read-only with respect to runner/Scale Set resources, but it **does mint temporary authentication credentials**. The exchange and response shape are source evidence, not a successful live test.

## Controller handoff

`controller` requires a separate digest-pinned G01 controller approval, the exact binary SHA-256 and matching reviewed `harness_sha`. The broker verifies the binary's Go `1.26.8`, command import path, clean embedded VCS revision and un-replaced `github.com/actions/scaleset v0.4.0` dependency. It requires a current-UID private source directory and an owned singly-linked regular binary of mode `0500`, capped at 128 MiB; symlinks are refused. It keeps the opened descriptor and checks the named inode and content hash again immediately before execution. This catches accidental replacement; it does not isolate a hostile process with the same controlling UID.

The controller approval is bounded, strict JSON with no duplicate/unknown keys. Its raw SHA-256 must match the broker approval; its App/installation/repo/group identity, exact phase, harness SHA, workflow facts, expiry and exact Actions hosts must agree. The broker writes only a non-secret immutable-content snapshot into its private attempt directory for the child to read. The child's persistent state directory must already be owned and mode `0700`; the broker verifies the captured directory identity again before execution. The existing G01 controller independently validates approval/build metadata and its own journal.

Only the fixed invocation is available:

```text
<verified-absolute-binary> --execute-approved-canary --approval <private-snapshot> --state-dir <private-controller-state> --phase <one-approved-phase>
```

There is no shell, PATH executable lookup, `gh`, arbitrary argument list, Docker or worker child. The broker starts one controller with only `LANG=C` and `LC_ALL=C` in its environment. An internal stdin pipe carries the existing reviewed G01 credential JSON schema. PEM never crosses that pipe. Token values never enter normal stdout, argv, environment, files or broker logs. Child stdout/stderr are discarded under a shared 8 KiB budget; overflow cancels that exact child. Deadline cancellation kills only that owned controller process. The reviewed controller launches no children; general process-tree/worker management is not implemented here. Exit is reported using a fixed category, never the child's text, panic value or SDK error.

If the controller approval includes an ACK/acquisition workflow-verification phase, it must explicitly authorize the separate verification authority. The private input must supply a distinct token. The broker uses it only for the approved workflow-run GET, verifies the expected run/head/path/repository/event/first-attempt facts and then passes it only to the controller, whose own pre-ACK check remains authoritative. No worker receives it. GitHub does not offer generic token-permission introspection: the operator's separately approved authority must actually be suitably read-only; a successful run GET is not proof of its complete permission set.

## Private inputs and durable inventory

The command requires `--execute-approved-broker`; `--plan` performs no credential reads or network operations. Approval files are current-UID, regular, singly linked, mode `0600`, in current-UID `0700` directories. Inputs are limited to 16 KiB for each approval, 64 KiB for the stdin JSON, 32 KiB for PEM and 1 KiB for the optional verification token. Stdin must be an owned protected regular descriptor or private pipe; terminals and group/other-accessible inputs are refused. Cancellation closes a blocked input within its thirty-second limit. A redirected descriptor cannot establish the already-resolved original filename; no original-path symlink guarantee is claimed for stdin.

The stdin JSON schema contains only `pem` and optional `verification_token`. Supply it through a trusted private pipe or an existing owner-only input file; the broker does not generate or save a credentials file. Do not place values in arguments, environment variables, shell tracing, clipboard, chat, screenshots or source control. Go/RSA/HTTP strings and memory copies are not guaranteed to be securely erased; clearing owned buffers and ending the process is not a stronger claim.

Each invocation uses a fresh exact broker attempt directory under an existing private `0700` parent. The broker captures and syncs that parent, creates and locks an exclusive `broker.jsonl`, syncs the child directory and flushes every intent/result before the next effect. Only approval digest, fixed phase names, numeric identities, observed expiry and the validated hostname can appear in this non-secret inventory. Any existing attempt file blocks reuse, including after success. Incomplete, damaged or ambiguous attempts are retained and never truncated/repaired/deleted automatically. A later phase requires its own reviewed broker approval/attempt while reusing the separately bound controller journal. Never change the directory to obtain an automatic retry after uncertainty.

All requests use direct TLS only to exact `api.github.com`; inherited proxies, redirects, other origins/ports and unexpected compressed encodings are refused. Every response, including errors and reused G02 identity calls, has a 1 MiB body budget plus one detection byte; known oversize lengths are rejected without reading. Successful JSON rejects duplicate keys and nesting beyond 32 levels. Application-level HTTP retries are absent. Error bodies are never returned or printed. Token/API responses that omit required policy facts fail closed.

## Future approval and commands

No live broker approval is granted by this document. The nominated existing-App proposal is `gh-runnerd-gate-deab34`, owner `1XP-AI` / `258160258`; the canary proposal is private `1XP-AI/gh-runnerd-canary` with runner group `gh-runnerd-canary-deab34`. Actual App, installation, repository and group IDs must be independently confirmed after their separate approval/creation. Zero values below are intentionally invalid:

```json
{
  "mode": "discover-actions-host",
  "app_id": 0,
  "app_name": "gh-runnerd-gate-deab34",
  "app_owner": "1XP-AI",
  "app_owner_id": 258160258,
  "installation_id": 0,
  "organization": "1XP-AI",
  "organization_id": 258160258,
  "repository": "gh-runnerd-canary",
  "repository_id": 0,
  "runner_group_id": 0,
  "runner_group_name": "gh-runnerd-canary-deab34",
  "expires_at": "2000-01-01T00:00:00Z",
  "allow_verification_authority": false
}
```

Build only the reviewed broker source; these path variables are non-secret operator-selected absolute paths outside checkouts/shared directories:

```sh
cd experiments/g02-auth
GOTOOLCHAIN=go1.26.8 go build -trimpath -o "$G01_BROKER_BINARY" ./cmd/g01-broker
"$G01_BROKER_BINARY" --plan
```

After separate explicit approval of one token mint and the two discovery authentication requests:

```sh
"$G01_BROKER_BINARY" --execute-approved-broker \
  --approval "$G01_BROKER_APPROVAL" --state-dir "$G01_BROKER_ATTEMPT" \
  < "$G01_PRIVATE_BROKER_INPUT"
```

The output may contain the validated hostname only alongside a fixed status. Include that exact hostname in a separately reviewed controller approval; discovery never automatically edits or approves that file.

For one controller phase, use `mode: "controller"`, add the one `phase`, the lowercase `controller_binary_sha256`, `controller_approval_sha256`, and reviewed 40-hex `controller_harness_sha`, and explicitly authorize workflow verification only when required by the controller approval. Follow the existing [controller build/approval procedure](g01-live-driver.md); its clean standalone clone stamping requirement remains. After building its reviewed binary outside the checkout, restrict that owned binary to `0500` before recording its digest. The broker will refuse a broadly accessible executable.

```sh
"$G01_BROKER_BINARY" --execute-approved-broker \
  --approval "$G01_BROKER_APPROVAL" --state-dir "$G01_BROKER_ATTEMPT" \
  --controller-binary "$G01_CONTROLLER_BINARY" \
  --controller-approval "$G01_CONTROLLER_APPROVAL" \
  --controller-state-dir "$G01_CONTROLLER_STATE" < "$G01_PRIVATE_BROKER_INPUT"
```

Root coordination must select the actual paths, IDs, exact reviewed source/binary digests, phase, expiry and separate workflow authority before final user approval. The command does not establish target-host/controller identity, install a service, modify the G02 App or create the canary repository/group. Controller actions are exactly those in its separate approved phase; no broker permission authorizes a worker or workflow dispatch.

## Actual TDD evidence and limits

All keys were freshly generated synthetic RSA fixtures; all HTTP responses were injected locally. Subprocess tests copied only the Go test executable to an owned temporary fixture, then exercised the fixed argument/pipe/environment/output boundary. That fixture deliberately bypasses the production build-stamp constructor; separate metadata tests reject wrong program, dirty or wrong VCS revision and wrong SDK. No real controller was invoked.

Observed red before implementation:

- `go test -run '^TestBroker' ./...`: four core cases failed with absent broker behavior; required positive mint/handoff counts also failed, so failures could not pass merely by refusing every operation.
- `go test -run 'TestBrokerPipe|TestBrokerChild|TestBrokerBuild' ./...`: successful private-pipe execution and valid reviewed metadata acceptance failed before their implementation.
- Private front-door/transport tests failed for absent successful discovery, duplicate App identity JSON being accepted before mint, and a known oversize shared identity response being read instead of refused immediately.
- `go test ./cmd/g01-broker`: safe plan behavior failed before command implementation.
- Focused schema tests then exposed missing fork, public-access policy and suspension fields being treated as false; all now refuse. A journal-parent test exposed a broadly accessible parent being accepted; it now refuses before any API call.

Green verification includes: exact operation order, durable intent before mint, one restricted request, actual issuance metadata, failed-group refusal before auth/handoff, restart refusal after ambiguous mint, hostname extraction without tenant contact, private-file/input guards, duplicate/oversized response rejection, fixed child argv/environment, secret-bearing child/error suppression, child output/time limits, replaced executable refusal, controller approval/source/phase/authority binding, and bounded canceled input. Ordinary G02 tests still pass.

Validation passed on Darwin ARM64 with Go `1.26.8`: `go test -race -count=1 -timeout=45s ./...` (library `8.369s`, broker CLI `2.241s`, existing enrollment CLI `1.917s`), `go vet ./...`, and the no-input/no-network `go run ./cmd/g01-broker --plan`. Combined repository `make check` passed (G01 core and livecanary, G02 library `8.594s`, broker CLI `1.450s`, existing enrollment CLI `1.734s`, formatting/vet/dependency/license checks and pinned root vulnerability scan with no vulnerabilities found). The root bootstrap explicitly skipped fuzz because it has no fuzz targets. Independent review and hosted CI remain pending at this commit. No test proves live GitHub acceptance of the selected token response fields or registration/admin exchange, physical power-loss durability, exact target/controller identity, a production credential store, worker isolation, workflow execution or completion of any G01/G02 live gate. Review and future explicit authorization remain required.

Primary contracts: GitHub [installation-token issuance](https://docs.github.com/en/rest/apps/apps#create-an-installation-access-token-for-an-app) supports repository/permission narrowing and returns expiry/permissions; [runner-group GET](https://docs.github.com/en/rest/actions/self-hosted-runner-groups#get-a-self-hosted-runner-group-for-an-organization) requires a suitable organization authority; the pinned [official SDK authentication source](https://github.com/actions/scaleset/blob/v0.4.0/client.go) supplies the discovery exchange. These sources establish the proposed protocol, not a live result.
