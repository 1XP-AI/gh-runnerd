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
Any failed command or mismatch aborts the gate.

Source and artifact policies are distinct. The source clone must be a real
directory owned by the current UID, with no group/other write bit; `0700` is
not required because a Git checkout is an identity boundary, not a private
artifact store. Every source ancestor must be a real directory owned by root
or the current UID, with no group/other write permission unless its sticky bit
prevents a cross-UID rename. A foreign-owned source directory or ancestor is
refused even when `0755`, `0711`, or `0700` has no group/other write bit,
because its owner can replace the checkout after the SHA/clean checks and
before `go build` or offline `go test`/`go run`. The gate canonicalizes the
source path, retains that physical path, and rechecks owner, ancestor chain,
detached HEAD, approved SHA, and cleanliness immediately before any build or
offline execution.

The artifact parent must be an existing directory owned by the current UID
with mode `0700`; every artifact ancestor uses the same root-or-current-UID
replaceability allowlist as the source chain. A foreign-owned artifact
ancestor is refused even when `0755`, `0711`, or `0700` has no group/other
write bit, because its owner can rename descendants through owner-write
access.

Darwin system tools used by the gate are pinned to absolute `/usr/bin` and
`/bin` paths and refused when foreign-owned or group/other-writable. `go` is
resolved once from `PATH` to an absolute path and then subjected to the same
executable owner/mode check; a relative or foreign-owned `go` is refused.
Mode masks use `8#` constants so macOS `/bin/bash` 3.2 does not treat
leading-zero literals as decimal.

Each signing-fact file is an independently recorded, singly-linked regular file
owned by the current UID with mode `0600`; it contains the sorted, non-path
lines emitted by `codesign -d --verbose=4` for that exact artifact
(`Identifier=`, `Authority=` and/or `Signature=`, and `TeamIdentifier=`).
The gate does not create or overwrite signing-fact, build-info, or codesign
output files: it reads the expected record first and compares all observed
facts and digests in memory.

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
G02_ID=/usr/bin/id
G02_STAT=/usr/bin/stat
G02_DIRNAME=/usr/bin/dirname
G02_CURRENT_UID="$("$G02_ID" -u)" || die 'could not determine the current UID'
if ! [[ "$G02_SOURCE_SHA" =~ ^[0-9a-f]{40}$ ]]; then
  die 'source SHA is not exactly 40 lowercase hexadecimal characters'
fi
if ! [[ "$G02_ENROLL_SHA256" =~ ^[0-9a-f]{64}$ && "$G02_PROBE_SHA256" =~ ^[0-9a-f]{64}$ ]]; then
  die 'artifact SHA-256 is not exactly 64 lowercase hexadecimal characters'
fi
if ! [[ "$G02_SOURCE_DIR" = /* && "$G02_PRIVATE_PARENT" = /* && \
        "$G02_ENROLL_SIGNING_RECORD" = /* && "$G02_PROBE_SIGNING_RECORD" = /* ]]; then
  die 'source, artifact, and signing-record paths must be absolute'
fi

check_safe_path_chain() {
  local path="$1" kind="$2" line owner mode
  while :; do
    [[ -d "$path" && ! -L "$path" ]] || die "unsafe ${kind} component: $path"
    line="$("$G02_STAT" -f '%u %A' "$path")" || die "could not inspect ${kind} component: $path"
    read -r owner mode <<<"$line"
    if [[ "$owner" != 0 && "$owner" != "$G02_CURRENT_UID" ]]; then
      die "${kind} chain permits cross-UID rename: $path"
    fi
    if (( (8#$mode & 8#22) != 0 )); then
      if (( (8#$mode & 8#1000) == 0 )); then
        die "${kind} chain permits cross-UID rename: $path"
      fi
    fi
    [[ "$path" == / ]] && break
    path="$("$G02_DIRNAME" "$path")"
  done
}

check_safe_parent_chain() {
  check_safe_path_chain "$1" "artifact parent"
}

check_safe_source_chain() {
  check_safe_path_chain "$1" "source"
}

require_trusted_exec() {
  local path="$1" label="$2" line owner mode
  [[ "$path" = /* && -f "$path" ]] || die "$label is missing or not absolute: $path"
  line="$("$G02_STAT" -f '%u %A' "$path")" || die "could not inspect $label: $path"
  read -r owner mode <<<"$line"
  if [[ "$owner" != 0 && "$owner" != "$G02_CURRENT_UID" ]]; then
    die "$label is foreign-owned: $path"
  fi
  if (( (8#$mode & 8#22) != 0 )); then
    die "$label is group/other-writable: $path"
  fi
}

require_owner_nonce() {
  : "${OWNER_NONCE:?set the approved nonsecret nonce}"
  if ! [[ "$OWNER_NONCE" =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$ ]]; then
    die 'OWNER_NONCE must be one path component of 1-63 characters matching [A-Za-z0-9][A-Za-z0-9._-]*'
  fi
  G02_JOURNAL_DIR="$G02_PRIVATE_PARENT/g02-attempt-$OWNER_NONCE"
  [[ "$("$G02_DIRNAME" "$G02_JOURNAL_DIR")" == "$G02_PRIVATE_PARENT" ]] \
    || die 'journal path is not a direct child of the private parent'
}

G02_GIT=/usr/bin/git
G02_ENV=/usr/bin/env
G02_AWK=/usr/bin/awk
G02_SHASUM=/usr/bin/shasum
G02_CODESIGN=/usr/bin/codesign
G02_CHMOD=/bin/chmod
require_trusted_exec "$G02_ID" id
require_trusted_exec "$G02_STAT" stat
require_trusted_exec "$G02_DIRNAME" dirname
require_trusted_exec "$G02_GIT" git
require_trusted_exec "$G02_ENV" env
require_trusted_exec "$G02_AWK" awk
require_trusted_exec "$G02_SHASUM" shasum
require_trusted_exec "$G02_CODESIGN" codesign
require_trusted_exec "$G02_CHMOD" chmod
G02_GO="$(type -P go)" || die 'go is not on PATH'
[[ "$G02_GO" = /* ]] || die 'go is not an absolute executable'
require_trusted_exec "$G02_GO" go

if ! [[ -d "$G02_SOURCE_DIR" && ! -L "$G02_SOURCE_DIR" && \
        -d "$G02_SOURCE_DIR/.git" && ! -L "$G02_SOURCE_DIR/.git" ]]; then
  die 'source must be a standalone clone with a real .git directory'
fi
if ! [[ -d "$G02_PRIVATE_PARENT" && ! -L "$G02_PRIVATE_PARENT" ]]; then
  die 'artifact parent must be an existing private directory'
fi
source_root="$(cd "$G02_SOURCE_DIR" && pwd -P)"
artifact_root="$(cd "$G02_PRIVATE_PARENT" && pwd -P)"
parent_stat="$("$G02_STAT" -f '%u %A' "$artifact_root")" || die 'could not inspect artifact parent'
read -r parent_uid parent_mode <<<"$parent_stat"
[[ "$parent_uid" == "$G02_CURRENT_UID" && "$parent_mode" == 700 ]] \
  || die 'artifact parent must be owned by the current UID with mode 0700'
check_safe_parent_chain "$artifact_root"

source_stat="$("$G02_STAT" -f '%u %A' "$source_root")" || die 'could not inspect source'
read -r source_uid source_mode <<<"$source_stat"
[[ "$source_uid" == "$G02_CURRENT_UID" ]] || die 'source must be owned by the current UID'
if (( (8#$source_mode & 8#22) != 0 )); then
  die 'source directory is group/other-writable'
fi
check_safe_source_chain "$source_root"
if [[ "$source_root" == "$artifact_root" || "$source_root" == "$artifact_root/"* || "$artifact_root" == "$source_root/"* ]]; then
  die 'source clone and artifact directory must not overlap'
fi
# Use the checked physical paths for every later git, build, and invocation;
# do not carry a user-supplied symlink alias forward.
G02_PRIVATE_PARENT="$artifact_root"
G02_SOURCE_DIR="$source_root"

recheck_source() {
  local live_uid line source_uid source_mode
  live_uid="$("$G02_ID" -u)" || die 'could not recheck the current UID'
  [[ "$live_uid" == "$G02_CURRENT_UID" ]] || die 'current UID changed during the gate'
  [[ -d "$G02_SOURCE_DIR" && ! -L "$G02_SOURCE_DIR" && \
     -d "$G02_SOURCE_DIR/.git" && ! -L "$G02_SOURCE_DIR/.git" ]] \
    || die 'source must remain a standalone clone with a real .git directory'
  line="$("$G02_STAT" -f '%u %A' "$G02_SOURCE_DIR")" || die 'could not recheck source'
  read -r source_uid source_mode <<<"$line"
  [[ "$source_uid" == "$G02_CURRENT_UID" ]] || die 'source must be owned by the current UID'
  if (( (8#$source_mode & 8#22) != 0 )); then
    die 'source directory is group/other-writable'
  fi
  check_safe_source_chain "$G02_SOURCE_DIR"
  if "$G02_GIT" -C "$G02_SOURCE_DIR" symbolic-ref --quiet HEAD >/dev/null 2>&1; then
    die 'source must be detached at the approved commit'
  fi
  if [[ "$("$G02_GIT" -C "$G02_SOURCE_DIR" rev-parse --verify HEAD^{commit})" != "$G02_SOURCE_SHA" ]]; then
    die 'source HEAD does not equal the approved full SHA'
  fi
  if [[ -n "$("$G02_GIT" -C "$G02_SOURCE_DIR" status --porcelain=v1 --untracked-files=all --ignored)" ]]; then
    die 'source has tracked, untracked, or ignored changes'
  fi
}

require_g01_plan_source() {
  : "${G01_HARNESS_SHA:?set the approved full 40-hex G01 harness SHA}"
  : "${G01_PRIVATE_BINARY:?set the private G01 plan binary path}"
  if ! [[ "$G01_HARNESS_SHA" =~ ^[0-9a-f]{40}$ ]]; then
    die 'G01 harness SHA is not exactly 40 lowercase hexadecimal characters'
  fi
  [[ "$G01_PRIVATE_BINARY" = /* ]] || die 'G01 plan binary path must be absolute'
  [[ "$("$G02_DIRNAME" "$G01_PRIVATE_BINARY")" == "$G02_PRIVATE_PARENT" ]] \
    || die 'G01 plan binary must be a direct child of the private artifact parent'
  [[ ! -e "$G01_PRIVATE_BINARY" && ! -L "$G01_PRIVATE_BINARY" ]] \
    || die 'G01 plan binary path already exists; use a fresh private path'
  recheck_source
  if [[ "$("$G02_GIT" -C "$G02_SOURCE_DIR" rev-parse --verify HEAD^{commit})" != "$G01_HARNESS_SHA" ]]; then
    die 'G01 plan source is not the approved G01 harness SHA'
  fi
}

recheck_source

signing_facts_from_text() {
  "$G02_AWK" -F= '$1 == "Identifier" || $1 == "Authority" || $1 == "Signature" || $1 == "TeamIdentifier" { print }' \
    | LC_ALL=C sort
}

read_signing_record() {
  local record="$1" output_name="$2" line record_uid record_mode record_links raw normalized
  [[ -f "$record" && ! -L "$record" ]] || die "signing-facts record is not a regular file: $record"
  line="$("$G02_STAT" -f '%u %A %l' "$record")" || die "could not inspect signing-facts record: $record"
  read -r record_uid record_mode record_links <<<"$line"
  [[ "$record_uid" == "$G02_CURRENT_UID" && "$record_mode" == 600 && "$record_links" == 1 ]] \
    || die "signing-facts record is not private and singly linked: $record"
  raw="$(<"$record")"
  [[ -n "$raw" ]] || die "signing-facts record is empty: $record"
  normalized="$(printf '%s\n' "$raw" | signing_facts_from_text)"
  [[ "$raw" == "$normalized" ]] || die "signing-facts record is not normalized: $record"
  printf -v "$output_name" '%s' "$raw"
}

read_signing_record "$G02_ENROLL_SIGNING_RECORD" G02_ENROLL_SIGNING_FACTS
read_signing_record "$G02_PROBE_SIGNING_RECORD" G02_PROBE_SIGNING_FACTS

G02_ENROLL_BINARY="$G02_PRIVATE_PARENT/g02-enroll"
G02_PROBE_BINARY="$G02_PRIVATE_PARENT/g02-keychain-probe"
[[ ! -e "$G02_ENROLL_BINARY" && ! -L "$G02_ENROLL_BINARY" && ! -e "$G02_PROBE_BINARY" && ! -L "$G02_PROBE_BINARY" ]] \
  || die 'artifact path already exists; use a fresh private path'

recheck_source
(
recheck_source
cd "$G02_SOURCE_DIR/experiments/g02-auth"
"$G02_ENV" -u GOFLAGS GOENV=off GOTOOLCHAIN=go1.26.8 GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 \
  "$G02_GO" build -buildvcs=true -trimpath -o "$G02_ENROLL_BINARY" ./cmd/g02-enroll
"$G02_ENV" -u GOFLAGS GOENV=off GOTOOLCHAIN=go1.26.8 GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 \
  "$G02_GO" build -buildvcs=true -trimpath -tags=g02runtime -o "$G02_PROBE_BINARY" ./cmd/g02-keychain-probe
"$G02_CHMOD" 0500 "$G02_ENROLL_BINARY" "$G02_PROBE_BINARY"
)

check_buildinfo() {
  local binary="$1" want_sha="${2:-$G02_SOURCE_SHA}" info
  info="$("$G02_ENV" -u GOFLAGS GOENV=off GOTOOLCHAIN=go1.26.8 "$G02_GO" version -m "$binary")" \
    || die "could not inspect build metadata: $binary"
  "$G02_AWK" -v want="$want_sha" '
    $1 == "build" && $2 ~ /^vcs\.revision=/ { revisions++; revision = substr($2, index($2, "=") + 1) }
    $1 == "build" && $2 ~ /^vcs\.modified=/ { modifieds++; modified = substr($2, index($2, "=") + 1) }
    END { exit !(revisions == 1 && modifieds == 1 && revision == want && modified == "false") }
  ' <<<"$info" || die "missing, wrong, or dirty VCS metadata in $binary"
}

check_artifact() {
  local binary="$1" expected_digest="$2" expected_signing="$3" line binary_uid binary_mode binary_links live_uid
  live_uid="$("$G02_ID" -u)" || die 'could not recheck the current UID'
  [[ "$live_uid" == "$G02_CURRENT_UID" ]] || die 'current UID changed during the gate'
  check_safe_parent_chain "$artifact_root"
  parent_stat="$("$G02_STAT" -f '%u %A' "$artifact_root")" || die 'could not recheck artifact parent'
  read -r parent_uid parent_mode <<<"$parent_stat"
  [[ "$parent_uid" == "$G02_CURRENT_UID" && "$parent_mode" == 700 ]] \
    || die 'artifact parent changed from current-UID mode 0700'
  [[ -f "$binary" && ! -L "$binary" ]] || die "artifact is not a regular file: $binary"
  line="$("$G02_STAT" -f '%u %A %l' "$binary")" || die "could not inspect artifact: $binary"
  read -r binary_uid binary_mode binary_links <<<"$line"
  [[ "$binary_uid" == "$G02_CURRENT_UID" && "$binary_mode" == 500 && "$binary_links" == 1 ]] \
    || die "artifact is not current-UID-owned, mode 0500, and singly linked: $binary"
  local actual_digest
  actual_digest="$("$G02_SHASUM" -a 256 < "$binary" | "$G02_AWK" 'NF >= 1 { count++; digest = $1 } END { if (count != 1) exit 1; print digest }')" \
    || die "could not hash $binary"
  [[ "$actual_digest" == "$expected_digest" ]] || die "artifact digest mismatch: $binary"
  "$G02_CODESIGN" --verify --strict --verbose=2 "$binary" >/dev/null 2>&1 \
    || die "codesign verification failed: $binary"
  local dump actual_signing
  dump="$("$G02_CODESIGN" -d --verbose=4 "$binary" 2>&1)" \
    || die "could not inspect signing identity: $binary"
  actual_signing="$(printf '%s\n' "$dump" | signing_facts_from_text)"
  [[ "$actual_signing" == "$expected_signing" ]] \
    || die "signing identity mismatch: $binary"
}

run_verified() {
  local binary="$1" expected_digest="$2" expected_signing="$3"
  shift 3
  check_artifact "$binary" "$expected_digest" "$expected_signing"
  "$binary" "$@"
}

check_buildinfo "$G02_ENROLL_BINARY"
check_buildinfo "$G02_PROBE_BINARY"
check_artifact "$G02_ENROLL_BINARY" "$G02_ENROLL_SHA256" "$G02_ENROLL_SIGNING_FACTS"
check_artifact "$G02_PROBE_BINARY" "$G02_PROBE_SHA256" "$G02_PROBE_SIGNING_FACTS"
printf '%s\n' 'G02 provenance gate passed; binary execution remains separately authorized.'
```

The two binaries are now referred to by their private absolute paths, never by
a SHA-bearing filename. `G02_SOURCE_DIR` is the checked physical clone path.
The gate writes no build-info, codesign, or observed signing-fact output files;
inspect or retain those values only in the private approval record. Do not
execute either path if any record is absent, different, stale, aliased,
hard-linked, or symlinked; rebuild and obtain a fresh independent approval.
Keep this Bash process alive for every separately authorized invocation and
call `recheck_source` immediately before any later build or offline
`go test`/`go vet`/`go run`, and `run_verified` immediately before a binary
start; `run_verified` rechecks the current UID, private parent, safe parent
chain, artifact owner/mode/link count, digest, and signing identity.
This is a trusted-UID boundary and provides no hostile same-UID guarantee:
same-UID code can still mutate the source tree, `PATH` entries, or artifact
paths after a check. The preflight intentionally refuses the current
linked-worktree layout because its `.git` file can produce missing VCS
metadata even when `-buildvcs=true` is requested.

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

Offline G02 checks, with no GitHub access. Keep the Bash process that passed
the gate above; recheck the canonical source immediately, and run the module
commands in a subshell so the caller directory is unchanged:

```sh
(
recheck_source
cd "$G02_SOURCE_DIR/experiments/g02-auth"
GOTOOLCHAIN=go1.26.8 "$G02_GO" test -race -count=1 -timeout=45s ./...
GOTOOLCHAIN=go1.26.8 "$G02_GO" vet ./...
GOTOOLCHAIN=go1.26.8 "$G02_GO" run ./cmd/g02-synthetic
)
```

The synthetic macOS probe is the existing, independently reviewed
non-persistent experiment. It must use a new private temporary binary path and
only the current-login mode:

```sh
# Keep the Bash process that passed the gate above; recheck immediately before
# each authorized invocation. This is still the non-persistent current-login
# probe, not a production Keychain/service test.
run_verified "$G02_PROBE_BINARY" "$G02_PROBE_SHA256" "$G02_PROBE_SIGNING_FACTS" \
  --synthetic-current-login
```

The G02 live driver is implemented as verify-only and has not been run against
GitHub. After exact source/binary/resource review, an owner may propose the
following one-shot forms; the values remain private placeholders and the
credential input is never placed in the command line:

```sh
# Keep the Bash process that passed the gate above; run only one separately
# authorized form, with the immediate `run_verified` recheck.
require_owner_nonce

# Only after the Manifest-specific approval; submit the remote form once.
run_verified "$G02_ENROLL_BINARY" "$G02_ENROLL_SHA256" "$G02_ENROLL_SIGNING_FACTS" \
  manifest --live-github \
  --owner "$APP_OWNER_ALIAS" --app-name "$DISPOSABLE_APP_ALIAS" \
  --org "$ORG_A_ALIAS:$ORG_A_ID" --org "$ORG_B_ALIAS:$ORG_B_ID" \
  --journal-dir "$G02_JOURNAL_DIR"

# Only for the same-App manual fallback, after owner supplies protected input.
run_verified "$G02_ENROLL_BINARY" "$G02_ENROLL_SHA256" "$G02_ENROLL_SIGNING_FACTS" \
  manual --live-github \
  --owner "$APP_OWNER_ALIAS" --app-name "$DISPOSABLE_APP_ALIAS" --app-id "$APP_ID" \
  --org "$ORG_A_ALIAS:$ORG_A_ID:$INSTALL_A_ID" \
  --org "$ORG_B_ALIAS:$ORG_B_ID:$INSTALL_B_ID" \
  --journal-dir "$G02_JOURNAL_DIR" < "$G02_PRIVATE_PEM"
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
invocation. Keep the Bash process that passed the gate above. The G01 plan
builds only when the already-validated detached source equals
`G01_HARNESS_SHA`; a different G01 revision needs a separately reviewed
detached clone that has passed the same source-chain gate.

```sh
(
require_g01_plan_source
cd "$G02_SOURCE_DIR/experiments/g01-scaleset"
"$G02_ENV" -u GOFLAGS GOENV=off GOTOOLCHAIN=go1.26.8 CGO_ENABLED=1 \
  "$G02_GO" build -buildvcs=true -trimpath -tags=g01_live -o "$G01_PRIVATE_BINARY" ./cmd/g01-live
check_buildinfo "$G01_PRIVATE_BINARY" "$G01_HARNESS_SHA"
"$G01_PRIVATE_BINARY" --plan
)
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

This is documentation-only continuation work; no G02 probe or enrollment
binary, live test, PEM, App, GitHub, Keychain, launchd, service, Docker, or
repository-runtime operation was performed. No artificial application red test
was created. The provenance correction was reproduced offline with nonsecret
disposable snapshots and the commands above: the former linked-worktree build
(`go build -trimpath`, both packages) produced no `vcs.revision` or
`vcs.modified` lines; it was not executed. In an owned standalone clone,
detached at the approved full SHA, both explicit `-buildvcs=true` builds passed
the clean source, `vcs.revision`, `vcs.modified=false`, SHA-256, and
`codesign --verify` checks. The same preflight was then exercised with the
artifact parent and signing records under a path containing spaces. A
group/world-writable artifact parent and a non-sticky group/world-writable
ancestor were refused; a symlinked or multiply-linked signing record was
refused before any generated output; a signing mismatch and one-byte artifact
tamper were each refused; stdin hashing accepted the spaced artifact path; and
the clean singly-linked, current-UID-owned snapshot passed. The immediate
`run_verified` recheck was exercised after changing the parent mode and refused
before any executable start.
For the earlier red-before-fix comparison, the frozen wrapper accepted a `0777`
artifact parent (`rc=0`, leaving eight generated files), rejected a spaced
artifact path at `could not hash`, and accepted an expected signing record
pointing at its generated observed-facts path (`rc=0` after overwriting and
self-comparing that record). These controls were offline only and establish
why the original provenance, hash, signing, and alias checks are required.

### PR59 exact-head Codex finding follow-up

On the integrated PR59 head `e25c2f1cd9b07ed131f29156ff41e943005f11a8`, the
installed review wrapper was read with both commands below; `all` included the
four earlier findings, and `detail-all` included their full stale/outdated
records:

```sh
bash "$CODEX_REVIEW" all 59 --repo 1XP-AI/gh-runnerd
bash "$CODEX_REVIEW" detail-all 59 --repo 1XP-AI/gh-runnerd
```

The wrapper reported two actionable findings on that exact head and four
earlier stale/outdated findings:

| Finding | Exact-head disposition and evidence |
|---|---|
| [P1 `r3955817035`](https://github.com/1XP-AI/gh-runnerd/pull/59#discussion_r3955817035) | Fixed by requiring a writable sticky ancestor's owner to be root (`0`) or `G02_CURRENT_UID`; a foreign-UID sticky fixture now refuses before any artifact check can return. The artifact parent remains current-UID-owned and mode `0700`. |
| [P2 `r3955817042`](https://github.com/1XP-AI/gh-runnerd/pull/59#discussion_r3955817042) | Fixed by running both builds and `chmod` in a subshell. A real temporary-directory fixture preserved the caller's `PWD` on both success and failure, while the failing build status propagated. `run_verified` remains defined and called in the caller shell. |
| [P1 `r3954505806`](https://github.com/1XP-AI/gh-runnerd/pull/59#discussion_r3954505806) | Stale/outdated; the earlier filename-only pin was removed. The current packet verifies the detached full source SHA, clean status, embedded `vcs.revision`, `vcs.modified=false`, binary digest, and signing identity before execution. |
| [P1 `r3954677724`](https://github.com/1XP-AI/gh-runnerd/pull/59#discussion_r3954677724) | Stale; current-UID ownership and mode `0700` are required for the artifact parent and rechecked before each artifact verification. |
| [P2 `r3954677733`](https://github.com/1XP-AI/gh-runnerd/pull/59#discussion_r3954677733) | Stale/outdated; hashing reads the binary through stdin, so spaces in the private artifact path are not parsed as filename fields. |
| [P2 `r3954677731`](https://github.com/1XP-AI/gh-runnerd/pull/59#discussion_r3954677731) | Stale/outdated; independently approved signing records are loaded and normalized before builds, expected facts stay in memory, generated codesign/output files are not used, and symlink/hard-link aliases are refused. |

The red/green boundary commands used only `mktemp -d` fixtures and a shell
`stat()` stub returning synthetic owner/mode tuples; they did not use `chown`,
create accounts, or change system state. The extracted function command was:

```sh
eval "$(sed -n '/^check_safe_parent_chain() {/,/^}/p' \
  docs/evidence/g02-mac-live-readiness.md)"
```

Before the edit, the fixture matrix was red: `foreign-sticky` (synthetic
foreign UID/mode `1777`) incorrectly returned `rc=0` where refusal was
required; `root-sticky` returned `rc=0`; `current-sticky` returned `rc=0`; and
`unsafe-writable` (synthetic foreign UID/mode `0777`) returned `rc=1`. After
the edit, the same extracted-function harness was green: `foreign-sticky`
returned `rc=1`, `root-sticky` `rc=0`, `current-sticky` `rc=0`, and
`unsafe-writable` `rc=1`.

Before the edit, a real temporary `experiments/g02-auth` directory and direct
`cd "$G02_SOURCE_DIR/experiments/g02-auth"` left the caller in the module
directory, so the cwd assertion exited `1` (red). After the edit, the
subshell-shaped harness returned `success rc=0` and `failure rc=17`; both
left the caller cwd unchanged (green), proving failure propagation without
poisoning the later relative `docs`/`experiments` commands. A syntax-only
`bash -n` check of the extracted gate and a guard audit for source SHA, clean
checkout, current-UID `0700` parent, stdin hash, signing-record checks,
`! -L` alias checks, and `run_verified` all passed.

The existing failure matrix also remained nonzero before execution: wrong
revision (HEAD mismatch), tracked edit plus untracked file (clean-check
refusal), missing `.git`/VCS metadata (standalone-clone or build-info refusal),
and one-byte artifact tampering (digest refusal). These checks used no PEM,
App, GitHub, Keychain, launchd, or probe/enrollment side effects. An inherited
`GOFLAGS=-buildvcs=false` and unrelated `GOTOOLCHAIN`/`GOENV` override were also
present during a clean run; the explicit environment sanitization and pinned
toolchain still passed.

Current file validation passed: `git diff --check`, `make fmt-check`, the
local-link target check, the secret-pattern scan, and the syntax-only gate check
above. The pre-edit PR59 head `060766efdb2a84547ed32c3660ddea7ec5263fc1`
had a successful hosted Go check in run
`34206178477` / job `101996075580`; no CI files were edited here. That result
belongs to the old head and does not clear a new commit. Issue #64's separate CI
hardening remains active. The coordinator must freeze the new head and obtain
fresh CI, independent security review, and exact-head Codex review before merge.
The offline/live results cited above remain those recorded in the linked
evidence; they are not upgraded or re-audited here.

The fresh same-PR P1 [foreign-owned ancestor finding](https://github.com/1XP-AI/gh-runnerd/pull/59#discussion_r3956164334)
was reproduced and corrected as a TDD boundary check using only a physical
nested `mktemp -d` directory tree and a shell `stat` stub that returned
synthetic owner/mode tuples; no `chown`, account creation, live runner, or other
system mutation was used. The extracted function was evaluated with:

```sh
eval "$(sed -n '/^check_safe_parent_chain() {/,/^}/p' \
  docs/evidence/g02-mac-live-readiness.md)"
```

The frozen pre-sticky-owner version at `e25c2f1` was red for every foreign-owner
mode: `0755`, `0711`, `0700`, and `1777` each returned `rc=0` where refusal was
required. The pre-edit `060766e` function preserved the earlier `1777` refusal
(`rc=1`) but still returned `rc=0` for foreign-owner `0755`, `0711`, and `0700`;
these are the same-PR ancestor-owner red witnesses. After the owner allowlist
edit, all four foreign-owner modes returned `rc=1`.

The boundary matrix after the edit was also green for synthetic ancestor
tuples: root-owned `0755` and current-UID-owned `0711` were accepted as safe
(`rc=0`); root/current-UID `0777`/`0775` were refused as non-sticky unsafe
(`rc=1`); root/current-UID `1777` were accepted under the existing sticky
exception (`rc=0`); a malformed foreign owner tuple was refused (`rc=1`); and
a symlink alias to the otherwise valid artifact directory was refused by the
existing `! -L` check (`rc=1`).
For the whole-chain proof, the immediate artifact parent was synthetic
current-UID `0700` while its grandparent was synthetic foreign-owner `0755`:
the pre-edit function returned `rc=0`, and the corrected function returned
`rc=1`, showing that every canonical ancestor is checked rather than only the
immediate parent. The existing cwd subshell, detached immutable source SHA and
clean-check, stdin hash, signing-record, artifact alias, and immediate
`run_verified` recheck guards remained present and passed the guard audit.
This remains a trusted-UID boundary: the owner allowlist addresses foreign-UID
pathname replacement only; same-UID code can still mutate paths after a check,
so same-UID workdirs are not hostile-code isolation.

### PR59 source-path and related shell-gate correction

Exact-head Codex P1
[r3956402913](https://github.com/1XP-AI/gh-runnerd/pull/59#discussion_r3956402913)
was reproduced on frozen `974999b5b0fce5e69867d166e57db21b7cac30ea` with a
synthetic `stat`/`git` stub: detached approved SHA plus a clean status accepted
a foreign-owned source (`rc=0`) because owner/ancestor validation applied only
to `artifact_root`, and `G02_SOURCE_DIR` was not rewritten to the physical
path. A prospective source-chain check on the same foreign `0755` tuple
returned `rc=1`. Staleness of the other twelve wrapper records is not
resolution; each was re-read via `all`/`detail-all` and either re-fixed here
or recorded with evidence.

The wrapper-reported set of 13 records and this pass:

| Finding | Disposition |
|---|---|
| [P1 `r3956402913`](https://github.com/1XP-AI/gh-runnerd/pull/59#discussion_r3956402913) | Fixed: canonical source directory and every ancestor are validated before git identity, builds, and offline `go test`/`go vet`/`go run`; `G02_SOURCE_DIR` is retained as the physical path and `recheck_source` rechecks owner, chain, detached HEAD, approved SHA, and cleanliness immediately before those uses. |
| [P1 `r3954505806`](https://github.com/1XP-AI/gh-runnerd/pull/59#discussion_r3954505806) | Still addressed: detached full SHA, clean status, `vcs.revision`, `vcs.modified=false`, digest, and signing identity remain before execution. |
| [P1 `r3954677724`](https://github.com/1XP-AI/gh-runnerd/pull/59#discussion_r3954677724) | Still addressed: artifact parent remains current-UID mode `0700` and is rechecked in `check_artifact`. |
| [P1 `r3955817035`](https://github.com/1XP-AI/gh-runnerd/pull/59#discussion_r3955817035) | Still addressed: sticky ancestors require root or current UID. |
| [P1 `r3956164334`](https://github.com/1XP-AI/gh-runnerd/pull/59#discussion_r3956164334) | Still addressed: every artifact ancestor uses the root/current-UID replaceability allowlist. |
| [P2 `r3954677733`](https://github.com/1XP-AI/gh-runnerd/pull/59#discussion_r3954677733) | Still addressed: hashing reads the binary through stdin. |
| [P2 `r3954677731`](https://github.com/1XP-AI/gh-runnerd/pull/59#discussion_r3954677731) | Still addressed: expected signing facts stay in memory; symlink/hard-link aliases are refused. |
| [P2 `r3955817042`](https://github.com/1XP-AI/gh-runnerd/pull/59#discussion_r3955817042) | Still addressed: G02 builds remain in a subshell. |
| [P1 `r3956164372`](https://github.com/1XP-AI/gh-runnerd/pull/59#discussion_r3956164372) | Fixed: Darwin system tools are pinned to absolute `/usr/bin` and `/bin` paths and owner-checked; `go` is resolved once to an absolute path and owner-checked. A full `PATH` ancestor walk was reproduced and rejected: it false-refuses macOS system prefixes, so it is not used. |
| [P2 `r3956164354`](https://github.com/1XP-AI/gh-runnerd/pull/59#discussion_r3956164354) | Fixed: offline checks run in a subshell. |
| [P1 `r3956164344`](https://github.com/1XP-AI/gh-runnerd/pull/59#discussion_r3956164344) | Fixed: offline checks call `recheck_source` and `cd` the validated `$G02_SOURCE_DIR/experiments/g02-auth`. |
| [P2 `r3956164383`](https://github.com/1XP-AI/gh-runnerd/pull/59#discussion_r3956164383) | Fixed: `require_owner_nonce` allows one `[A-Za-z0-9][A-Za-z0-9._-]{0,62}` component and requires the journal path to be a direct child of the private parent. |
| [P1 `r3956164362`](https://github.com/1XP-AI/gh-runnerd/pull/59#discussion_r3956164362) | Fixed: `require_g01_plan_source` rechecks the validated source, requires `HEAD == G01_HARNESS_SHA`, builds under the private parent, and runs `check_buildinfo` against `G01_HARNESS_SHA` before `--plan`. |

Source versus artifact policy is distinct. The source clone must be
current-UID owned with no group/other write; mode `0700` is not required. The
artifact parent remains current-UID mode `0700`. Both trees share the same
root-or-current-UID ancestor replaceability allowlist, including the sticky
exception. Mode masks use `8#22` and `8#1000` because macOS `/bin/bash` 3.2
does not treat leading-zero literals as octal; a `0755` directory must not be
classified as group/other-writable.

Red before the source-chain edit, using the frozen prefix and synthetic
foreign UID `424242` mode `0755` with stubbed detached/clean git: current
source-validation `rc=0`. After the edit, the extracted `check_safe_source_chain`
matrix was:

```text
source-foreign-0755,0711,0700,1777 rc=1
source-root/current 0755,0711,0700 rc=0
source-root/current 0775,0777 rc=1
source-root/current 1777 rc=0
source-malformed-owner rc=1
source-symlink-alias rc=1
source-real-current rc=0
source-path-with-spaces rc=0
artifact-foreign-0755 rc=1
artifact-current-0700 rc=0
artifact-parent-0700-retained
```

Related helpers: `OWNER_NONCE` values `../escaped`, `foo/bar`,
`../../../outside`, empty, and `has space` each returned `rc=1`; `canary1`
returned `rc=0` as a direct child of the private parent. `require_trusted_exec`
accepted `/usr/bin/stat` and `/usr/bin/git` (`rc=0`) and refused a missing
path, a relative `./go`, and a synthetic foreign-owned executable (`rc=1`).
Offline/G01-shaped subshells returned success `rc=0` and injected failure
`rc=17` without changing caller `PWD`. `/bin/bash -n` of the extracted gate
passed. Same-UID workdirs, `PATH` entries, and source trees remain
non-isolation.

No live App, key, Keychain, signing-state, account, `chown`, launchd, runner,
Docker, Lima, service, or workflow operation was performed. Commands in this
packet were not executed as live authorization. Remaining gaps: hosted CI and
exact-head Codex review of the new SHA, independent Luna review, and the
original G02 live evidence. Rollback is to restore this file from
`974999b5b0fce5e69867d166e57db21b7cac30ea`.

Factual sources used without adding private identifiers:

- [G02 enrollment evidence](g02-enrollment-evidence.md) and [G02 live driver](g02-live-driver.md)
- [G02 remaining live procedure](g02-live-procedure.md), [G01 live driver](g01-live-driver.md), and [G01 canary plan](g01-live-canary.md)
- [ADR 0003: manual import and login-scoped identity](../decisions/0003-enrollment-and-service-identity.md)
- [Security design](../SECURITY-DESIGN.md), [execution policy](../EXECUTION.md), and [G01 workflow template](../../experiments/g01-canary-assets/canary.yml.template)
- [GitHub Manifest registration](https://docs.github.com/en/apps/sharing-github-apps/registering-a-github-app-from-a-manifest), [installation requirements](https://docs.github.com/en/apps/using-github-apps/installing-a-github-app-from-a-third-party), and [organization runner credentials](https://docs.github.com/en/rest/actions/self-hosted-runners#create-a-registration-token-for-an-organization)
- [Apple TN3137](https://developer.apple.com/documentation/Technotes/tn3137-on-mac-keychains) and [Apple launchd guidance](https://developer.apple.com/library/archive/documentation/MacOSX/Conceptual/BPSystemStartup/Chapters/CreatingLaunchdJobs.html)
