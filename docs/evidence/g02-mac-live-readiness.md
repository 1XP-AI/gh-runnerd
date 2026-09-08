# G02 macOS live-validation readiness packet

Status on 2026-09-08: **candidate host only; no live operation was run by this
continuation**. This is a concise operational companion to the [G02 evidence
record](g02-enrollment-evidence.md), [verify-only driver](g02-live-driver.md),
and [remaining live procedure](g02-live-procedure.md); it does not replace
their boundaries or mark issue [#2](https://github.com/1XP-AI/gh-runnerd/issues/2)
Done.

Issue/Project snapshot read for this packet: issue #2 is open and In progress,
Stage M0 - Evidence gates, Agent Luna max, with Goal “Resolve GitHub App Manifest
loopback enrollment and the macOS credential/service identity contract with real
platform evidence and a safe manual-import fallback.” Native `blockedBy` is
empty. This continuation creates no Goal and changes no issue or Project field.

## Decision

This Apple Silicon Mac is a plausible eventual installation host, but it is not
yet a G02-ready installable-product target. Continue only with serial,
individually authorized evidence on freshly reviewed immutable artifacts. The
current evidence is limited to offline harnesses and one narrow synthetic
current-login probe; it does not establish Manifest acceptance, two-organization
installation, a production credential store, a persistent service, or reboot
recovery.

The operator has authorized preparation of one **private empty canary
repository** and manual installation of a disposable test App in a nominated
organization. The operator-supplied preparation check is organization
`self-hosted runners: write` with no repository permission; no key was generated
by this agent. This preparation is for G01 only. It does not prove G02 Manifest
or two-organization acceptance, and no preparation action was performed here.

## Read-only host inventory

Facts below are a read-only inventory for the eventual host, not a capacity or
security approval.

| Fact | Observation | What it proves / does not prove |
|---|---|---|
| Architecture | ARM64 (`arm64`) | Matches the planned native Mac architecture; not release/runtime compatibility |
| macOS | 26.6.2 | Identifies the platform under test; not launchd, Keychain, signing, or reboot evidence |
| Logical CPUs | 14 | Host inventory only; no worker/concurrency budget |
| Memory | 48 GiB | Host inventory only; no workload headroom or native hard limit |
| Docker | Executable is available | Not a daemon, endpoint, runtime, image, architecture, or headroom check |
| Lima | `limactl` is not on `PATH` | No VM-provider availability claim; do not install or select a VM implicitly |

Do not use Docker availability as evidence for G01/G02 or run `docker info` as
part of this packet. A future Linux-worker experiment must choose and approve a
specific endpoint/runtime, image digest, CPU/memory budget, and owned cleanup.
The host-wide reserve and native process budget remain unmeasured.

## What exists versus what is still unimplemented

| Surface | Current, bounded evidence | Product conclusion |
|---|---|---|
| `g02-enroll` | Offline-tested Manifest/manual verification boundary; exact Host/path/state/query checks; one-time conversion; two binding checks; credentials held in memory and reported as not persisted | A verify-only experiment, not `gh-runnerd init`, a broker, or a persistent import |
| Synthetic macOS probe | Fresh file-based Keychain/canary and transient GUI launchd children in one current login; owned cleanup and explicit locked denial were observed | No system daemon, production Keychain backend, release identity, logout, screen-lock, or reboot result |
| G01 controller driver | Offline-tested bounded phases and private journal/authority rules; `--plan` is available; no live phase or worker was run | G01 live delivery, JIT, worker, drain, and cleanup gates remain open |
| G01 broker experiment | Controller-only experimental boundary with temporary credentials in process memory | Not an installed product broker and not a source of credentials for this packet |
| Installable product | No production daemon, `init`, persistent Keychain/import sink, launchd package, native worker adapter, or update/recovery path is established by these experiments | G02 cannot be represented as an installed-product acceptance result |

The offline manual-import sink is in memory. A credential broker with reviewed
issuance authority and a persistent, atomic, launchd-context-appropriate import
path remain implementation gaps. Do not lower the permission profile, copy a
PEM into repository state, pass credentials through argv/environment/clipboard,
or claim that best-effort memory clearing erases every copy.

## Minimal distinct approvals

Each row is a separate gate. Approval of one row does not imply any other row.

| Gate | Minimum approval and scope | Explicit non-authorization |
|---|---|---|
| 1. Canary resources | Maintainer names one private empty repository, one rendered no-secret workflow, one dedicated runner-group policy, and one nonce; owner confirms the repository is isolated from existing manual runners | No public/fork workflow, unrelated repository, or existing runner mutation |
| 2. Test App/org | Owners approve the exact disposable App and each nominated organization installation; verify only organization runner-write plus baseline metadata as reviewed, with no repository/Admin/Actions grant | One manual installation does not prove Manifest, two-org identity, or production-App acceptance |
| 3. Credential authority | A trusted controller-side broker is independently reviewed and supplies only scoped temporary credentials through the approved private input boundary; owner supplies any manual-import key privately | No agent-generated key, credential access, persistent import, worker handoff, or raw token/error logging |
| 4. G01 protocol | Maintainer separately authorizes the exact reviewed controller/workflow SHAs, one scale-set/session owner, bounded JIT/workflow/barrier phases, and owned cleanup | No automatic workflow replay, broad runner action, or use of this G02 packet as G01 live approval |
| 5. Native Mac identity | Maintainer authorizes the exact trusted controller identity, distinct job identity/allowlist, selected Keychain backend, launchd domain, and any narrow helper | No same-UID hostile-code isolation, root controller, arbitrary helper, or existing Keychain/service mutation |
| 6. Lifecycle window | Operator individually authorizes each screen-lock, logout/login, reboot, signed-update, and rollback stage after busy work drains | No implicit reboot, FileVault change, busy cancellation, broad process kill, or unattended login-free support claim |
| 7. Review and release | Independent Luna max reviews the exact source/binary/approval; before merge, GitHub Codex reviews the exact current PR head and findings are resolved | Passing offline CI, an old review, a stale binary, or a Project status is not live authorization |

The live App/organization and G01 gates must use aliases in public evidence.
Keep numeric IDs, installation IDs, owner names, repository names, callback
values, keys, tokens, personal paths, raw SDK errors, and private logs in the
owner-controlled record only.

## Immutable inputs and safe command proposals

The following markers are intentionally invalid until replaced in a private
approval by exact, independently reviewed values. Never run a command with a
marker. A branch name, mutable tag, or working-tree build is not an immutable
input.

| Input | Required private record |
|---|---|
| G02 source/harness | `G02_HARNESS_SHA=<REVIEWED_HARNESS_SHA_40_HEX>` |
| G02 synthetic probe source | `G02_PROBE_SHA=<REVIEWED_PROBE_SHA_40_HEX>` |
| G01 controller/harness | `G01_HARNESS_SHA=<REVIEWED_G01_HARNESS_SHA_40_HEX>` |
| G01 workflow | `G01_WORKFLOW_SHA=<REVIEWED_WORKFLOW_SHA_40_HEX>` |
| Mac release artifact | `MAC_RELEASE_SHA=<REVIEWED_RELEASE_SHA_40_HEX>` plus signing identity record |
| Resource identity | `OWNER_NONCE=<NEW_APPROVED_NONSECRET_NONCE>` plus private App/org/repository IDs |

These commands are proposals for implemented, bounded experiments only. They
require the approvals above and a freshly reviewed exact artifact; they were
not run by this docs continuation.

Read-only host inventory:

```sh
uname -m
sw_vers -productVersion
sysctl -n hw.logicalcpu
sysctl -n hw.memsize
command -v docker
docker --version
command -v limactl || true
```

Offline G02 checks, with no GitHub access:

```sh
cd experiments/g02-auth
GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=45s ./...
GOTOOLCHAIN=go1.26.8 go vet ./...
GOTOOLCHAIN=go1.26.8 go run ./cmd/g02-synthetic
```

The synthetic macOS probe is the existing, independently reviewed
non-persistent experiment. It must use a new private temporary binary path and
only the current-login mode:

```sh
cd experiments/g02-auth
GOTOOLCHAIN=go1.26.8 go build -tags=g02runtime -trimpath -o "$G02_PROBE_BINARY" ./cmd/g02-keychain-probe
"$G02_PROBE_BINARY" --synthetic-current-login
```

The G02 live driver is implemented as verify-only and has not been run against
GitHub. After exact source/binary/resource review, an owner may propose the
following one-shot forms; the values remain private placeholders and the
credential input is never placed in the command line:

```sh
cd experiments/g02-auth
GOTOOLCHAIN=go1.26.8 go build -trimpath -o "$G02_PRIVATE_PARENT/g02-enroll-$G02_HARNESS_SHA" ./cmd/g02-enroll

# Only after the Manifest-specific approval; submit the remote form once.
"$G02_PRIVATE_PARENT/g02-enroll-$G02_HARNESS_SHA" manifest --live-github \
  --owner "$APP_OWNER_ALIAS" --app-name "$DISPOSABLE_APP_ALIAS" \
  --org "$ORG_A_ALIAS:$ORG_A_ID" --org "$ORG_B_ALIAS:$ORG_B_ID" \
  --journal-dir "$G02_PRIVATE_PARENT/g02-attempt-$OWNER_NONCE"

# Only for the same-App manual fallback, after owner supplies protected input.
"$G02_PRIVATE_PARENT/g02-enroll-$G02_HARNESS_SHA" manual --live-github \
  --owner "$APP_OWNER_ALIAS" --app-name "$DISPOSABLE_APP_ALIAS" --app-id "$APP_ID" \
  --org "$ORG_A_ALIAS:$ORG_A_ID:$INSTALL_A_ID" \
  --org "$ORG_B_ALIAS:$ORG_B_ID:$INSTALL_B_ID" \
  --journal-dir "$G02_PRIVATE_PARENT/g02-attempt-$OWNER_NONCE" < "$G02_PRIVATE_PEM"
```

The forms above are not an installable product flow: they do not persist a
credential or hand one to G01. If Manifest registration, callback, conversion,
or storage becomes ambiguous, stop and reconcile the one existing App through
the retained journal; never resubmit with a changed name or silently create a
second App. The manual fallback is the same-App recovery path, not permission
relaxation.

The G01 controller has a safe plan-only command. Live execution remains
separately gated by the reviewed controller broker and the [G01 driver
procedure](g01-live-driver.md); this packet does not provide a credential
invocation:

```sh
cd experiments/g01-scaleset
CGO_ENABLED=1 GOTOOLCHAIN=go1.26.8 go build -buildvcs=true -trimpath -tags=g01_live -o "$G01_PRIVATE_BINARY" ./cmd/g01-live
"$G01_PRIVATE_BINARY" --plan
```

The inactive [G01 workflow template](../../experiments/g01-canary-assets/canary.yml.template)
must be rendered into the approved private repository, reviewed at its
immutable workflow commit, and bound to the private controller approval. It
does not authorize dispatch, a worker, JIT, or cleanup by itself.

## Deferred final Mac acceptance

These rows are intentionally unperformed and remain release-blocking for the
protected native/installable-product profile.

| Acceptance row | Evidence still required | Current disposition / owner |
|---|---|---|
| Installable daemon | Production `init`/daemon, private IPC, durable journal, startup/shutdown, and no-secret diagnostics on a signed artifact | Unimplemented; G06/G15 own daemon and lifecycle contracts |
| Credential persistence | Atomic import of verified App + all org bindings into the selected backend, owner/permission checks, lock refusal, recovery after interrupted write | Unimplemented product sink; G07/G08 own auth/import integration |
| launchd identity | Actual controller UID/domain, distinct job identity, executable signing requirement, no-interaction access, and narrow-helper decision | Synthetic GUI child only; G11/G15 own native identity/service evidence |
| Keychain choice | Real selected backend under the actual launchd context; system daemon must separately prove file-Keychain/noninteractive behavior | Deferred; synthetic file-Keychain result does not choose production backend |
| Screen lock | Separate observation from Keychain lock; refusal must stop admission without prompt or secret disclosure | Not run; requires individual lifecycle approval |
| Logout/login | Agent termination/restart, denied or absent credential behavior, no work admission under another UID | Not run; login-scoped continuity is not promised |
| Reboot/cold boot | Pre-login behavior and post-authorized-login recovery without FileVault changes or transparent replay | Deferred; login-free boot unsupported until separately demonstrated |
| Signed update | Repeat build/sign identity, update while idle/busy as approved, preserve or clearly quarantine credential/service state, recover without trust broadening | Unimplemented/deferred; G15/G18 own update evidence |

The final acceptance is not satisfied by a source build, ad-hoc signature,
current terminal success, a transient launchd job, or a Docker executable. No
system daemon, persistent Keychain import, release-signing identity, distinct
controller/job account, reboot, or update acceptance is claimed here.

## Execution order and fail-stop rules

1. Freeze the private approval with aliases, exact reviewed source/binary/workflow
   SHAs, permissions, resource budget, owner nonce, phase, expiry, and rollback
   contact. Inventory existing Apps, installations, runners, services, and
   Keychain metadata read-only; publish only counts/aliases.
2. Run only one approved phase at a time. Verify ownership before every effect;
   keep existing manual runners untouched. A phase timeout, failed private
   journal sync, lost response, unknown service state, or mismatched identity is
   **unresolved**, not success.
3. On unresolved state, stop new admission and credential use, retain the exact
   private journal/recovery inventory, and do not retry, delete the journal,
   switch Docker context, run global prune, broadly kill processes, or reboot.
   Reconciliation is an owner decision using the original immutable identity;
   later inspection cannot erase uncertainty.
4. For the authorized G01 canary, drain busy work normally and retain any
   active/unknown runner, session, scale set, App, installation, process, or
   container until ownership and absence are proven. Never cancel a busy job or
   replay it automatically.
5. Roll back only individually verified resources owned by this experiment:
   stop admission, wait for owned busy work, remove exact disposable App/
   installation/repository/workflow/service/Keychain/runner resources as their
   owner approves, verify absence, then remove private state. Preserve the
   original manual runners and unrelated Apps/services. If any cleanup fact is
   uncertain, leave the private journal and recovery inventory intact and
   escalate to the owner.

The existing G02 procedure's unknown-service rule remains authoritative: retain
the private recovery inventory and report cleanup incomplete rather than
deleting evidence. The G01 driver likewise retains its journal after JIT or
acquisition ambiguity; no cleanup override is introduced by this packet.

## Packet validation and source refs

This is documentation-only continuation work; no artificial red application
test and no live test was created. Validation for this file is limited to
`git diff --check`, local-link resolution, and a secret-pattern scan. The
offline/live results cited above remain those recorded in the linked evidence;
they are not upgraded or re-audited here.

Factual sources used without adding private identifiers:

- [G02 enrollment evidence](g02-enrollment-evidence.md) and [G02 live driver](g02-live-driver.md)
- [G02 remaining live procedure](g02-live-procedure.md), [G01 live driver](g01-live-driver.md), and [G01 canary plan](g01-live-canary.md)
- [ADR 0003: manual import and login-scoped identity](../decisions/0003-enrollment-and-service-identity.md)
- [Security design](../SECURITY-DESIGN.md), [execution policy](../EXECUTION.md), and [G01 workflow template](../../experiments/g01-canary-assets/canary.yml.template)
- [GitHub Manifest registration](https://docs.github.com/en/apps/sharing-github-apps/registering-a-github-app-from-a-manifest), [installation requirements](https://docs.github.com/en/apps/using-github-apps/installing-a-github-app-from-a-third-party), and [organization runner credentials](https://docs.github.com/en/rest/actions/self-hosted-runners#create-a-registration-token-for-an-organization)
- [Apple TN3137](https://developer.apple.com/documentation/Technotes/tn3137-on-mac-keychains) and [Apple launchd guidance](https://developer.apple.com/library/archive/documentation/MacOSX/Conceptual/BPSystemStartup/Chapters/CreatingLaunchdJobs.html)
