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
marker. A branch name, mutable tag, filename suffix, or working-tree build is
not an immutable input. `G02_HARNESS_SHA` was previously only a filename
suffix; it is not a source or artifact check and is not used below.

| Input | Required private record |
|---|---|
| G02 source for both binaries | `G02_SOURCE_SHA=<REVIEWED_FULL_SOURCE_SHA_40_HEX>`; exactly 40 lowercase hexadecimal characters, detached in a standalone clone with a real `.git` directory |
| G02 enrollment artifact | `G02_ENROLL_SHA256=<APPROVED_ENROLL_ARTIFACT_SHA256_64_HEX>` plus an independently recorded private `codesign` identity record |
| G02 synthetic probe artifact | `G02_PROBE_SHA256=<APPROVED_PROBE_ARTIFACT_SHA256_64_HEX>` plus an independently recorded private `codesign` identity record |
| G01 controller/harness | `G01_HARNESS_SHA=<REVIEWED_G01_HARNESS_SHA_40_HEX>` |
| G01 workflow | `G01_WORKFLOW_SHA=<REVIEWED_WORKFLOW_SHA_40_HEX>` |
| Mac release artifact | `MAC_RELEASE_SHA=<REVIEWED_RELEASE_SHA_40_HEX>` plus signing identity record |
| Resource identity | `OWNER_NONCE=<NEW_APPROVED_NONSECRET_NONCE>` plus private App/org/repository IDs |

The live-effect commands below are proposals for implemented, bounded
experiments only. They require the approvals above and a freshly reviewed exact
artifact; no live-effect command was run by this docs continuation.

### Mandatory G02 source and artifact gate

Neither `g02-enroll` nor `g02-keychain-probe` verifies its own source revision,
working-tree state, digest, or signing identity. The shell preflight below is
the documented gate and must complete successfully before **any** G02 binary
execution. It is not a runtime verifier and does not add product-runtime
scope. An owner first creates an owned temporary **standalone clone** (not an
Orca linked worktree), preserves its real `.git` directory, detaches it at the
approved full SHA, and sets `G02_PRIVATE_PARENT` to a separate private
directory outside that clone. The expected artifact SHA-256 values and
normalized signing-fact files come from an independent private approval
record; do not calculate and trust them in the same invocation.

Create the checkout as an owned temporary clone and preserve its Git metadata;
do not use `git worktree` or change an Orca-managed checkout:

```sh
git clone --no-local "$G02_REPOSITORY_URL" "$G02_SOURCE_DIR"
git -C "$G02_SOURCE_DIR" checkout --detach "$G02_SOURCE_SHA"
```

Run this as Bash with the private values already set; `set -Eeuo pipefail`, the
explicit `-buildvcs=true`, `GOENV=off`, and removal of `GOFLAGS` are required.
Any failed command or mismatch aborts the gate. Each signing-fact file contains
the sorted, non-path lines emitted by `codesign -d --verbose=4` for that exact
artifact (`Identifier=`, `Authority=` and/or `Signature=`, and
`TeamIdentifier=`).

```bash
set -Eeuo pipefail

: "${G02_SOURCE_SHA:?set the approved full 40-hex source SHA}"
: "${G02_PRIVATE_PARENT:?set the owned private artifact directory}"
: "${G02_SOURCE_DIR:?set the owned standalone clone directory}"
: "${G02_ENROLL_SHA256:?set the independently recorded enrollment digest}"
: "${G02_PROBE_SHA256:?set the independently recorded probe digest}"
: "${G02_ENROLL_SIGNING_RECORD:?set the private enrollment signing-facts file}"
: "${G02_PROBE_SIGNING_RECORD:?set the private probe signing-facts file}"

die() { printf 'G02 provenance refusal: %s\n' "$1" >&2; exit 1; }
if ! [[ "$G02_SOURCE_SHA" =~ ^[0-9a-f]{40}$ ]]; then
  die 'source SHA is not exactly 40 lowercase hexadecimal characters'
fi
if ! [[ "$G02_ENROLL_SHA256" =~ ^[0-9a-f]{64}$ && "$G02_PROBE_SHA256" =~ ^[0-9a-f]{64}$ ]]; then
  die 'artifact SHA-256 is not exactly 64 lowercase hexadecimal characters'
fi
if ! [[ "$G02_SOURCE_DIR" = /* && "$G02_PRIVATE_PARENT" = /* ]]; then
  die 'source and artifact paths must be absolute'
fi
if ! [[ -d "$G02_SOURCE_DIR/.git" && ! -L "$G02_SOURCE_DIR/.git" ]]; then
  die 'source must be a standalone clone with a real .git directory'
fi
if ! [[ -d "$G02_PRIVATE_PARENT" && ! -L "$G02_PRIVATE_PARENT" ]]; then
  die 'artifact parent must be an existing private directory'
fi
source_root="$(cd "$G02_SOURCE_DIR" && pwd -P)"
artifact_root="$(cd "$G02_PRIVATE_PARENT" && pwd -P)"
if [[ "$source_root" == "$artifact_root" || "$source_root" == "$artifact_root/"* || "$artifact_root" == "$source_root/"* ]]; then
  die 'source clone and artifact directory must not overlap'
fi
if git -C "$G02_SOURCE_DIR" symbolic-ref --quiet HEAD >/dev/null 2>&1; then
  die 'source must be detached at the approved commit'
fi
if [[ "$(git -C "$G02_SOURCE_DIR" rev-parse --verify HEAD^{commit})" != "$G02_SOURCE_SHA" ]]; then
  die 'source HEAD does not equal the approved full SHA'
fi
if [[ -n "$(git -C "$G02_SOURCE_DIR" status --porcelain=v1 --untracked-files=all --ignored)" ]]; then
  die 'source has tracked, untracked, or ignored changes'
fi
for record in "$G02_ENROLL_SIGNING_RECORD" "$G02_PROBE_SIGNING_RECORD"; do
  [[ -s "$record" && ! -L "$record" ]] || die 'missing private signing-facts record'
done

G02_ENROLL_BINARY="$G02_PRIVATE_PARENT/g02-enroll"
G02_PROBE_BINARY="$G02_PRIVATE_PARENT/g02-keychain-probe"
[[ ! -e "$G02_ENROLL_BINARY" && ! -L "$G02_ENROLL_BINARY" && ! -e "$G02_PROBE_BINARY" && ! -L "$G02_PROBE_BINARY" ]] \
  || die 'artifact path already exists; use a fresh private path'

cd "$G02_SOURCE_DIR/experiments/g02-auth"
env -u GOFLAGS GOENV=off GOTOOLCHAIN=go1.26.8 GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 \
  go build -buildvcs=true -trimpath -o "$G02_ENROLL_BINARY" ./cmd/g02-enroll
env -u GOFLAGS GOENV=off GOTOOLCHAIN=go1.26.8 GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 \
  go build -buildvcs=true -trimpath -tags=g02runtime -o "$G02_PROBE_BINARY" ./cmd/g02-keychain-probe
chmod 0500 "$G02_ENROLL_BINARY" "$G02_PROBE_BINARY"

check_buildinfo() {
  local binary="$1" info="$G02_PRIVATE_PARENT/$(basename "$1").buildinfo"
  env -u GOFLAGS GOENV=off GOTOOLCHAIN=go1.26.8 go version -m "$binary" >"$info"
  awk -v want="$G02_SOURCE_SHA" '
    $1 == "build" && $2 ~ /^vcs\.revision=/ { revisions++; revision = substr($2, index($2, "=") + 1) }
    $1 == "build" && $2 ~ /^vcs\.modified=/ { modifieds++; modified = substr($2, index($2, "=") + 1) }
    END { exit !(revisions == 1 && modifieds == 1 && revision == want && modified == "false") }
  ' "$info" || die "missing, wrong, or dirty VCS metadata in $binary"
}

signing_facts() {
  awk -F= '$1 == "Identifier" || $1 == "Authority" || $1 == "Signature" || $1 == "TeamIdentifier" { print }' "$1" | LC_ALL=C sort
}

check_artifact() {
  local binary="$1" expected_digest="$2" expected_signing="$3"
  [[ -f "$binary" && ! -L "$binary" ]] || die "artifact is not a regular file: $binary"
  local actual_digest
  actual_digest="$(shasum -a 256 "$binary" | awk 'NF == 2 { count++; digest = $1 } END { if (count != 1) exit 1; print digest }')" \
    || die "could not hash $binary"
  [[ "$actual_digest" == "$expected_digest" ]] || die "artifact digest mismatch: $binary"
  codesign --verify --strict --verbose=2 "$binary" >/dev/null 2>&1 \
    || die "codesign verification failed: $binary"
  local dump="$G02_PRIVATE_PARENT/.$(basename "$binary").codesign"
  local actual_signing="$G02_PRIVATE_PARENT/.$(basename "$binary").signing-facts"
  codesign -d --verbose=4 "$binary" >"$dump" 2>&1 \
    || die "could not inspect signing identity: $binary"
  signing_facts "$dump" >"$actual_signing"
  cmp -s "$actual_signing" "$expected_signing" \
    || die "signing identity mismatch: $binary"
}

check_buildinfo "$G02_ENROLL_BINARY"
check_buildinfo "$G02_PROBE_BINARY"
check_artifact "$G02_ENROLL_BINARY" "$G02_ENROLL_SHA256" "$G02_ENROLL_SIGNING_RECORD"
check_artifact "$G02_PROBE_BINARY" "$G02_PROBE_SHA256" "$G02_PROBE_SIGNING_RECORD"
printf '%s\n' 'G02 provenance gate passed; binary execution remains separately authorized.'
```

The two binaries are now referred to by their private absolute paths, never by
a SHA-bearing filename. Inspect and retain the private `go version -m`, digest,
and signing records. Do not execute either path if any record is absent,
different, or stale; rebuild and obtain a fresh independent approval. The
preflight intentionally refuses the current linked-worktree layout because
its `.git` file can produce missing VCS metadata even when `-buildvcs=true` is
requested.

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
# G02_PROBE_BINARY is built and passes the mandatory provenance gate above.
"$G02_PROBE_BINARY" --synthetic-current-login
```

The G02 live driver is implemented as verify-only and has not been run against
GitHub. After exact source/binary/resource review, an owner may propose the
following one-shot forms; the values remain private placeholders and the
credential input is never placed in the command line:

```sh
# G02_ENROLL_BINARY is built and passes the mandatory provenance gate above.

# Only after the Manifest-specific approval; submit the remote form once.
"$G02_ENROLL_BINARY" manifest --live-github \
  --owner "$APP_OWNER_ALIAS" --app-name "$DISPOSABLE_APP_ALIAS" \
  --org "$ORG_A_ALIAS:$ORG_A_ID" --org "$ORG_B_ALIAS:$ORG_B_ID" \
  --journal-dir "$G02_PRIVATE_PARENT/g02-attempt-$OWNER_NONCE"

# Only for the same-App manual fallback, after owner supplies protected input.
"$G02_ENROLL_BINARY" manual --live-github \
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
test and no live test was created. No G02 probe or enrollment binary was
executed. The provenance correction was reproduced offline with nonsecret
disposable snapshots and the commands above: the former linked-worktree build
(`go build -trimpath`, both packages) produced no `vcs.revision` or
`vcs.modified` lines; it was not executed. In an owned standalone clone,
detached at the approved full SHA, both explicit `-buildvcs=true` builds passed
the clean source, `vcs.revision`, `vcs.modified=false`, SHA-256, and
`codesign --verify` checks. The failure matrix was also run through the same
preflight: wrong revision (HEAD mismatch), tracked edit plus untracked file
(clean-check refusal), missing `.git`/VCS metadata (standalone-clone or
build-info refusal), and one-byte artifact tampering (digest refusal) all
exited nonzero before execution; the clean snapshot exited zero. These checks
used no PEM, App, GitHub, Keychain, launchd, or probe/enrollment side effects.
An inherited `GOFLAGS=-buildvcs=false` and unrelated `GOTOOLCHAIN`/`GOENV`
override were also present during a clean run; the explicit environment
sanitization and pinned toolchain still passed.

Remaining file validation is `git diff --check`, local-link resolution, and a
secret-pattern scan. The offline/live results cited above remain those recorded
in the linked evidence; they are not upgraded or re-audited here.

Factual sources used without adding private identifiers:

- [G02 enrollment evidence](g02-enrollment-evidence.md) and [G02 live driver](g02-live-driver.md)
- [G02 remaining live procedure](g02-live-procedure.md), [G01 live driver](g01-live-driver.md), and [G01 canary plan](g01-live-canary.md)
- [ADR 0003: manual import and login-scoped identity](../decisions/0003-enrollment-and-service-identity.md)
- [Security design](../SECURITY-DESIGN.md), [execution policy](../EXECUTION.md), and [G01 workflow template](../../experiments/g01-canary-assets/canary.yml.template)
- [GitHub Manifest registration](https://docs.github.com/en/apps/sharing-github-apps/registering-a-github-app-from-a-manifest), [installation requirements](https://docs.github.com/en/apps/using-github-apps/installing-a-github-app-from-a-third-party), and [organization runner credentials](https://docs.github.com/en/rest/actions/self-hosted-runners#create-a-registration-token-for-an-organization)
- [Apple TN3137](https://developer.apple.com/documentation/Technotes/tn3137-on-mac-keychains) and [Apple launchd guidance](https://developer.apple.com/library/archive/documentation/MacOSX/Conceptual/BPSystemStartup/Chapters/CreatingLaunchdJobs.html)
