# G01 runner release-source readiness checkpoint

Date: 2026-10-13. This checkpoint combines read-only public GitHub release,
source, published-advisory, and pinned SDK disclosure-policy queries through the
REST API. The advisory results are a point-in-time screen of the stated filters.
No archive bytes were downloaded or hashed, no runner was extracted or executed,
no tenant-specific availability was checked, and no report submission, GitHub
write, or live operation was performed.

## Selection and release metadata

The integration selection remains the released `github.com/actions/scaleset`
`v0.4.0` high-level listener with independent reconciliation. The official
latest-release endpoint still resolves to `v0.4.0`, published
`2026-05-05T15:32:02Z`; its tag points directly to
`6ce025902cd964747a078c2aabe7340ebc667eca`. Upstream `main` resolves to
`e6daac702355cdb5b880b4fbdcf6d85dcd9e48e5` (`2026-09-16T21:46:19Z`). The
historical audited comparison at
`cb0405b2d874500e75ae34eff8d582ab75956b45` remains unchanged: this checkpoint
does not replace that comparison or claim protocol tests against current
`main`. ([release](https://github.com/actions/scaleset/releases/tag/v0.4.0),
[tag commit](https://github.com/actions/scaleset/commit/6ce025902cd964747a078c2aabe7340ebc667eca),
[current main commit](https://github.com/actions/scaleset/commit/e6daac702355cdb5b880b4fbdcf6d85dcd9e48e5))

The existing runner canary selection remains `v2.337.0`, as recorded in the
[contract](g01-contract.md#runner-and-jit-transport) and
[canary plan](g01-live-canary.md#required-environment-and-authorization-record).
The latest official runner release is `v2.338.0`, published
`2026-10-06T13:55:11Z`, with tag commit
`197ca027b90de199464dee3bb9ff198a1db60967`. It is a reviewed-source candidate
only; it is not tenant-qualified, selected, or executed. Its release notes say
the progressive release may not yet be available to a particular enterprise,
organization, or repository. The named canary tenant is not established here,
so “latest” does not establish what that tenant can install. ([release and
official rollout note](https://github.com/actions/runner/releases/tag/v2.338.0),
[tag commit](https://github.com/actions/runner/commit/197ca027b90de199464dee3bb9ff198a1db60967))

| Official `v2.338.0` asset | Published size | Published SHA-256 |
|---|---:|---|
| `actions-runner-linux-arm64-2.338.0.tar.gz` | 140012091 bytes | `628b4a7258487b80c1d3c221095a7ded349f5ae0175fd3dfab708d07428f041b` |
| `actions-runner-osx-arm64-2.338.0.tar.gz` | 128562863 bytes | `df4cebda25c86a886ed204e49fee63f5c2e7cec5f447b5c98440a826bbdf9df2` |

These are release-API checksum and size metadata, not independently verified
archive bytes.

## Source comparison relevant to the existing contract

The immutable source comparison from runner `v2.337.0`
(`397b032cbf865e9c3ddfab89d533ec19325e1273`) to `v2.338.0` reports
`diverged`, 39 commits ahead, 1 behind, and 43 changed files. Do not describe
the candidate as a descendant or fast-forward of the old source; the findings
below are limited to the inspected files. ([compare](https://github.com/actions/runner/compare/397b032cbf865e9c3ddfab89d533ec19325e1273...197ca027b90de199464dee3bb9ff198a1db60967))

| Surface | Read-only source finding | Canary preparation implication |
|---|---|---|
| JIT input and file materialization | `CommandSettings.cs` is blob `7c29e2b6d2a2b44d40e5ed6496af439c62aa543b` (18,579 bytes) at both commits; `Runner.cs` is blob `31c87130257f534a1a30f0cbcc37bc61e6406405` (64,169 bytes) at both. The inspected parsing and materialization source is unchanged. ([v2.337.0 parser](https://github.com/actions/runner/blob/397b032cbf865e9c3ddfab89d533ec19325e1273/src/Runner.Listener/CommandSettings.cs), [v2.338.0 parser](https://github.com/actions/runner/blob/197ca027b90de199464dee3bb9ff198a1db60967/src/Runner.Listener/CommandSettings.cs), [v2.337.0 materializer](https://github.com/actions/runner/blob/397b032cbf865e9c3ddfab89d533ec19325e1273/src/Runner.Listener/Runner.cs), [v2.338.0 materializer](https://github.com/actions/runner/blob/197ca027b90de199464dee3bb9ff198a1db60967/src/Runner.Listener/Runner.cs)) | The historical argv/environment and on-disk JIT conclusions retain this specific source basis only; they do not qualify the new runtime. |
| Internal Node and service wrapper | `NodeUtil.cs` now defaults internal operations to `node24`, and `runsvc.sh` selects `node24`. `Constants.cs` changes only the Node 20 removal date from September 16 to September 23; this is not a new JIT masking change. ([NodeUtil](https://github.com/actions/runner/blob/197ca027b90de199464dee3bb9ff198a1db60967/src/Runner.Common/Util/NodeUtil.cs), [runsvc.sh](https://github.com/actions/runner/blob/197ca027b90de199464dee3bb9ff198a1db60967/src/Misc/layoutbin/runsvc.sh), [Constants.cs](https://github.com/actions/runner/blob/197ca027b90de199464dee3bb9ff198a1db60967/src/Runner.Common/Constants.cs)) | If `v2.338.0` is later selected, include Node 24 compatibility and the service-wrapper runtime in the reviewed no-op canary, and record the exact runtime/image digest. |
| Runner broker session deletion | `BrokerMessageListener` passes its owned `_session.SessionId` to `BrokerServer.DeleteSessionAsync(Guid, ...)`. This is a runner broker-session change; it does not establish Scale Set SDK manager-session replacement or drain semantics. ([listener](https://github.com/actions/runner/blob/197ca027b90de199464dee3bb9ff198a1db60967/src/Runner.Listener/BrokerMessageListener.cs), [broker server](https://github.com/actions/runner/blob/197ca027b90de199464dee3bb9ff198a1db60967/src/Runner.Common/BrokerServer.cs)) | Keep runner broker-session deletion observations separate from the SDK manager-session and safe-drain evidence gates. |
| Conditional WebSocket diagnostics | `JobExtension` starts the probe only when the job variable `system.runner.brokerwebsocketprobeurl` is nonempty. `BrokerServer` traces received/replied text and exception details, and `JobExtension` adds connectivity telemetry. This is a conditional diagnostics/logging surface, not evidence of an always-on probe, leaked live secrets, safe raw-log publication, or manager credential inheritance. ([variable](https://github.com/actions/runner/blob/197ca027b90de199464dee3bb9ff198a1db60967/src/Sdk/DTWebApi/WebApi/WellKnownDistributedTaskVariables.cs), [job extension](https://github.com/actions/runner/blob/197ca027b90de199464dee3bb9ff198a1db60967/src/Runner.Worker/JobExtension.cs), [broker server](https://github.com/actions/runner/blob/197ca027b90de199464dee3bb9ff198a1db60967/src/Runner.Common/BrokerServer.cs)) | Before using the candidate, check whether the named service supplies this variable. If a reviewed canary enables it, qualify and sanitize the resulting logs and telemetry before retaining or sharing evidence. |

## Readiness boundary

Before any future candidate change, identify the private organization/repository
and confirm the version offered there; verify the downloaded archive bytes against
the published digest and record the exact runtime/image digest. If the candidate
is `v2.338.0`, include the Node 24, runner broker-session, and conditional
diagnostics checks above in the reviewed canary preparation. None of those steps
has occurred here.

Tenant availability, archive-byte verification, runtime/image qualification,
reviewed live harness authorization, sanitized live results, and the safe-drain
decision remain outstanding. Full G01/G02 and dependent production gates remain
unresolved. No GitHub issue or Project status was changed.

## Public advisory and reporting-policy screen

Read-only queries made on 2026-10-13 against public GitHub REST endpoints. List
queries used `--paginate` with no explicit `per_page` override. The two
repository feeds requested `state=published`; the global list queries also
specified `type=reviewed`. A zero from a global query means no matching
GitHub-reviewed published advisory for that exact filter, not no unreviewed
reports or all public reports. Results are not a vulnerability scan. Commands
below show endpoint and API filters; `--jq` only projected response counts or
fields.

```text
gh api --paginate 'repos/actions/scaleset/security-advisories?state=published'
gh api --paginate 'repos/actions/runner/security-advisories?state=published'
gh api advisories/GHSA-2c6m-6gqh-6qg3
gh api --paginate 'advisories?type=reviewed&ecosystem=actions&affects=actions/runner@2.337.0'
```

The Scale Set repository feed returned `[]` (0). The runner repository feed
returned one record: GHSA-2c6m-6gqh-6qg3 / CVE-2022-39321, severity `medium`,
published and updated `2022-10-24T16:00:18Z`, with vulnerable range `<2.296.1`
and listed patched versions `2.296.2`, `2.293.1`, `2.289.4`, `2.285.2`, and
`2.283.4`. The exact Actions ecosystem query for `actions/runner@2.337.0`
returned `[]` (0 matching GitHub-reviewed published advisories). ([runner repository advisory](https://github.com/actions/runner/security/advisories/GHSA-2c6m-6gqh-6qg3))

The successful global advisory detail GET returns the same GHSA/CVE as severity
`high`, published `2022-10-25T19:54:27Z` and updated `2023-01-30T05:02:47Z`.
It lists these separate affected ranges and first patched versions:
`<2.283.4` → `2.283.4`; `>=2.284.0,<2.285.2` → `2.285.2`;
`>=2.286.0,<2.289.4` → `2.289.4`; `>=2.290.0,<2.293.1` → `2.293.1`; and
`>=2.294.0,<2.296.1` → `2.296.2`. Keep this global severity/date/range record
separate from the repository-feed record above; do not collapse the different
feed values. Literal runner version `2.337.0` is outside both returned range
forms, which does not verify an archive, image, tenant runtime, or general
vulnerability status. ([canonical global advisory](https://github.com/advisories/GHSA-2c6m-6gqh-6qg3))

The coordinator's earlier repository-specific detail GET returned HTTP 404; it
was a failed lookup, not an empty-feed or passing result. A direct confirmation
at `2026-10-13T03:40:16Z` used:

```text
gh api --include repos/actions/runner/security-advisories/GHSA-2c6m-6gqh-6qg3
```

It returned HTTP 200. Both observations are recorded; the successful global
advisory entry above remains the canonical cross-version record.

The exact-version Go global-advisory queries were:

```text
gh api --paginate 'advisories?type=reviewed&ecosystem=go&affects=github.com/actions/scaleset@v0.4.0'
gh api --paginate 'advisories?type=reviewed&ecosystem=go&affects=github.com/golang-jwt/jwt/v4@v4.5.2'
gh api --paginate 'advisories?type=reviewed&ecosystem=go&affects=github.com/google/uuid@v1.6.0'
gh api --paginate 'advisories?type=reviewed&ecosystem=go&affects=github.com/hashicorp/go-cleanhttp@v0.5.2'
gh api --paginate 'advisories?type=reviewed&ecosystem=go&affects=github.com/hashicorp/go-retryablehttp@v0.7.8'
```

Each returned `[]` (0 matching GitHub-reviewed published advisories) for the
exact pinned external module version named in G01's existing dependency files.
The positive query control was:

```text
gh api --paginate 'advisories?type=reviewed&ecosystem=go&affects=github.com/golang-jwt/jwt/v4@v4.5.1'
```

It returned GHSA-mh63-6h87-95cp / CVE-2025-30204, severity `high`, published
`2025-03-21T22:04:00Z`, updated `2025-04-10T13:02:35Z`, affected range
`<4.5.2`, first patched `4.5.2`. This control is for the older `v4.5.1` query,
not a matching advisory for the pinned `v4.5.2` dependency. ([query-control advisory](https://github.com/advisories/GHSA-mh63-6h87-95cp))

The pinned SDK `SECURITY.md` was read with
`gh api 'repos/actions/scaleset/contents/SECURITY.md?ref=6ce025902cd964747a078c2aabe7340ebc667eca'`
at commit `6ce025902cd964747a078c2aabe7340ebc667eca` (blob
`67a9cbf2c6c4161a66b9ae4fcd9b74bbff8d0b4d`, 1,744 bytes). It requires
coordinated disclosure and says not to report vulnerabilities through public
GitHub issues, discussions, or pull requests. This is a reporting policy, not a
runner-version support or lifetime guarantee. A prior `gh` code search targeting
`github/docs` for runner support-window wording returned no matches; that search
does not establish that no support policy exists. Do not infer a 30-day or
indefinite support promise; support and security status for the named canary
tenant remain a pre-execution qualification. ([pinned SDK SECURITY.md](https://github.com/actions/scaleset/blob/6ce025902cd964747a078c2aabe7340ebc667eca/SECURITY.md))

No new actual vulnerability was established by these metadata queries. The five
exact-version Go results do not cover the full module graph, import reachability,
all `go.sum`-only packages, standard library, toolchain, tests, a bundled runtime,
or an SBOM, and are not a `govulncheck` result. This screen does not replace
archive-byte verification, image/runtime auditing, tenant-specific version
qualification, or a fresh public-feed check before a future canary. Existing
G01/G02, live-authorization, and safe-drain gates remain open; no pin changes
follow from this advisory screen.

Rollback scope: remove only this file's advisory-screen addendum; preserve parent
release/source commit `8ae0c1a3dd158360784de64232a4890f3f3b2be6` and its
historical validation. No live cleanup is involved.

## Local documentation validation

On 2026-10-13, relative Markdown links and heading fragments resolved for all
three changed documents; `git diff --check` emitted no diagnostics; and the
credential/token/private-key/personal-path scan found no matches. No tests were
run because this change is documentation-only.

## Advisory-screen addendum validation

On 2026-10-13, the added local links and heading fragments resolved;
`git diff --check` emitted no diagnostics; and the credential/token/private-key/
personal-path scan found no matches. No tests, builds, compilers, or vulnerability
scanners were run for this documentation-only addendum.

## Local source-mode Go vulnerability and reachability analysis

The preceding advisory-screen validation is historical for that earlier
metadata-only addendum; this source analysis is a separate later addition.

Date: 2026-10-13. This is read-only source analysis of the committed G01 module
and its local G02 replacement. It does not establish a clean security gate:
the source scan returned findings, the Go toolchain pin remains unchanged, and
separate remediation and re-verification are required before dependent G01,
G02, live-canary, and safe-drain gates can close.

### Source and tool identity

The scan used parent commit
`7fa9bce30e88bb214d2cc7aec47c76778e9d281d`, G01 tree
`9636ee550304dc8f155a44faa78d93b80939a138`, and local G02 replacement tree
`8015fedeccd7604cedef3e5e1814d190d7443757`. The G01
[module file](../../experiments/g01-scaleset/go.mod) declares Go `1.26.8`
and replaces
`github.com/1XP-AI/gh-runnerd/experiments/g02-auth` with
`../g02-auth`, whose [local module](../../experiments/g02-auth/go.mod) is in
the same committed tree; both module trees were clean and contained no untracked Go,
`go.mod`, or `go.sum` inputs.

No `govulncheck` executable was present in the standard tool locations. The
pinned tool was installed only into an ephemeral directory with
`go install golang.org/x/vuln/cmd/govulncheck@v1.7.0`, then checked with
`go version -m` before scanning target source. The binary reports module
`golang.org/x/vuln v1.7.0` with checksum
`h1:4MQBuhmXbz2uepNJrf3v+aaZLGDqw1JluwYboegA1qg=`; a public
`go mod download -json golang.org/x/vuln@v1.7.0` returned the same checksum.
The cached pinned documentation blob matches
`8de6904a51b632fbb4c546ce7b50678798394312` at the immutable
[govulncheck v1.7.0 source](https://github.com/golang/vuln/blob/617f44b718537dccdea1915395650e0529e3b72e/cmd/govulncheck/doc.go).
The source documentation says `-test` includes test files, `-tags` selects
build variants, and source analysis uses the Go command on `PATH`; it also
documents that JSON, SARIF, and VEX formats can exit 0 despite findings.

Each scanner invocation used
`govulncheck -mode=source -format=text -db=https://vuln.go.dev -test [-tags=<profile>] ./...`
from the G01 module. The child environment was built from an allowlist with
Go `1.26.8` (`GOTOOLCHAIN=go1.26.8`), Darwin/ARM64
(`GOARM64=v8.0`), `GOENV=off`, `GOWORK=off`,
`GOFLAGS=-mod=readonly -p=2`, `GOEXPERIMENT=none`, `GOFIPS140=off`,
`GOMAXPROCS=2`, and `GOAUTH=off`, public-only
`GOPROXY=https://proxy.golang.org`, `GOSUMDB=sum.golang.org`, and empty
private-module/no-proxy scopes. `GOROOT` was omitted and derived by Go from
the selected toolchain; `GOCACHEPROG` was unset. No HOME, Git/GitHub
credentials, SSH settings, proxy credentials, or ambient Go overlays/tool
overrides were passed. For `CGO_ENABLED=1`, `CC`/`CXX` were set to Apple
Clang 21.0.0. Profiles ran sequentially with a 15-minute per-profile deadline
and two-way Go parallelism. No G01/G02 application or test binary was launched,
no fixture was executed, and no `go test` command ran. `-test` caused test
source to be included in source analysis; Go package loading may compile or
load package export data, and the `govulncheck` tool itself was installed with
`go install`.

The scanner queried the live, unpinned `https://vuln.go.dev` database. A
read-only request to the [database index](https://vuln.go.dev/index/db.json)
after the profile batch returned body
`{"modified":"2026-10-08T22:31:09Z"}`, ETag
`8875c8c67555e9c875a431c63837b83d`, and Last-Modified
`2026-10-12T22:21:33Z` (response date `2026-10-13T03:42:05Z`). This is
observed database metadata, not an immutable database snapshot bound to each
scan. The advisory links below point to the public Go vulnerability database;
no advisory publication dates are inferred.

### Profiles and exit results

The six requested source profiles ran once in the same batch. A separate
`CGO=1,osusergo` profile was added after checking the build constraints:
G01 imports `os/user`, and this combination selects the unsupported account
lookup source while retaining cgo for other packages. The Android-only
constraint is outside the selected Darwin/ARM64 target.

| Profile | CGO | Build tags | Elapsed | Result |
|---|---:|---|---:|---|
| A | 1 | default | 3.2 s | exit 3; 10 symbol-result IDs |
| B | 1 | `g01_live,g01_worker` | 2.2 s | exit 3; same 10 symbol-result IDs |
| C | 1 | `g01_pair_fixture` | 2.3 s | exit 3; same 10 symbol-result IDs |
| D | 1 | `g01_live,g01_worker,g01_pair_fixture` | 2.2 s | exit 3; same 10 symbol-result IDs |
| E | 1 | `g01_pair_fixture,g01_pair_real_cadence` | 2.3 s | exit 3; same 10 symbol-result IDs |
| F | 0 | default | 2.2 s | exit 3; same 10 symbol-result IDs |
| G (additional) | 1 | `osusergo` | 2.7 s | exit 3; same 10 symbol-result IDs |

Text format returned nonzero exit 3 with findings for every profile; none
timed out. Complete raw logs were discarded after normalized result extraction,
so this record does not claim a separate review of every diagnostic line.
An A-profile `-show traces` replay (2.3 s, exit 3) collected call context.
One A-profile and the additional G profile were also run with `-show verbose`
(2.6 s and 2.7 s, exit 3) to distinguish symbol, package, and module results.
Those verbose outputs each reported 10 symbol-level findings, one
package-import finding, and two required-module findings. The non-symbol IDs
below are classified from those A and G verbose outputs; they are not
profile-by-profile verbose results for B–F.

### Findings and source-call scope

The text summaries classified these ten IDs as symbol-level findings from the
Go standard library in every requested profile and in profile G:
[GO-2026-6603](https://vuln.go.dev/ID/GO-2026-6603.json),
[GO-2026-6604](https://vuln.go.dev/ID/GO-2026-6604.json),
[GO-2026-6605](https://vuln.go.dev/ID/GO-2026-6605.json),
[GO-2026-6607](https://vuln.go.dev/ID/GO-2026-6607.json),
[GO-2026-6608](https://vuln.go.dev/ID/GO-2026-6608.json),
[GO-2026-6610](https://vuln.go.dev/ID/GO-2026-6610.json),
[GO-2026-6611](https://vuln.go.dev/ID/GO-2026-6611.json),
[GO-2026-6612](https://vuln.go.dev/ID/GO-2026-6612.json),
[GO-2026-6613](https://vuln.go.dev/ID/GO-2026-6613.json), and
[GO-2026-6617](https://vuln.go.dev/ID/GO-2026-6617.json). The scanner reported
the relevant standard-library packages at Go `1.26.8`, with Go `1.26.9`
as the fix version. The Go `1.26.9` tag is
[commit `2ae494ef9fb90e0da0073c31800ba38870249994`](https://github.com/golang/go/commit/2ae494ef9fb90e0da0073c31800ba38870249994);
no toolchain or manifest pin was changed here.

The trace examples included these source call paths:

| Finding IDs | Scanner-reported source path |
|---|---|
| GO-2026-6603, GO-2026-6611, GO-2026-6612, GO-2026-6617 | `net/http.CanonicalHeaderKey` / trailer handling, with traces through G01 `drainPollHook.RoundTrip` and `SDKAPI` request paths |
| GO-2026-6613 | `net/http.CanonicalHeaderKey` reached through `drainObservedBody.Close` and the drain response-body path |
| GO-2026-6605, GO-2026-6610 | `net/http.Client.CloseIdleConnections` reached through `drainPollHook.RoundTrip` and `SDKAPI.Preflight` / `SDKAPI.get` |
| GO-2026-6608 | `mime/multipart@go1.26.8`; trace includes `drainObservedBody.Read` and `net/textproto.Reader.ReadMIMEHeader` |
| GO-2026-6607 | `crypto/tls.Conn.HandshakeContext`; the example trace includes test-source function `TestDockerInspectExactRejectsReplacedSocket` |
| GO-2026-6604 | `os@go1.26.8` through `OpenJournal` / `openJournalAtAdmission`; the public Go database marks both affected imports Windows-only |

GO-2026-6604 appears in the scanner's symbol results, but its Windows-only
metadata makes it inapplicable to these Darwin/ARM64 profiles; this is not a
Darwin exploitability finding. The other nine symbol results are analyzer
call-reachability reports, not proof that attacker-controlled inputs satisfy
the advisory's trigger conditions. Traces include test-source paths because
`-test` was enabled; tests and fixtures were not run.

The verbose A and G outputs also reported:

| Result level | Finding | Found / fixed | Classification |
|---|---|---|---|
| Package | [GO-2026-6609](https://vuln.go.dev/ID/GO-2026-6609.json) | `net/http@go1.26.8` → `go1.26.9` | Package is imported; scanner did not find a call from analyzed source |
| Required module | [GO-2026-6599](https://vuln.go.dev/ID/GO-2026-6599.json) | `stdlib@go1.26.8` → `go1.26.9` | Required module only; no package or call result |
| Required module | [GO-2026-6600](https://vuln.go.dev/ID/GO-2026-6600.json) | `stdlib@go1.26.8` → `go1.26.9` | Required module only; no package or call result |

The public Go database also lists `golang.org/x/net`, fixed at `v0.60.0`,
for GO-2026-6603, GO-2026-6610, GO-2026-6611, GO-2026-6612, and
GO-2026-6617. No `x/net` call, package, or required-module finding appeared
in the verbose A or G output; this does not replace a full module-graph or
SBOM inventory. The scanner's source-level symbol analysis is approximate:
interface and function-pointer paths can be conservative, while reflection
and unsafe can hide paths, as documented by the
[pinned tool source](https://github.com/golang/vuln/blob/617f44b718537dccdea1915395650e0529e3b72e/cmd/govulncheck/doc.go).

### Gate and scope

This adds a Go standard-library security gate for the current Go `1.26.8`
source profile. The listed fixes are in Go `1.26.9`, but shared toolchain
remediation and fresh source re-verification need separate ownership and
authorization; no Go pin, dependency, source, workflow, canary, or runner
selection changed. These results do not qualify the tenant, archived runner
bytes, image or bundled runtime, live credentials, safe drain, or production
execution, and do not close full G01/G02.

No Go tests, application/test-binary launches, fixture execution, live runner
operations, GitHub writes, or GitHub advisory submissions occurred. The
`govulncheck` tool itself was installed in a temporary directory; source-mode
analysis loaded the selected source and test files and may compile/load package
export data without launching the application or tests. Rollback for this
checkpoint is limited to reverting this local evidence addendum; retain the
earlier advisory and release-source evidence unchanged.

## Source-analysis evidence validation

On 2026-10-13, all four relative Markdown links and their heading fragments resolved;
`git diff --check` emitted no diagnostics; and the added diff scan for
credential/token/private-key and personal-machine-path patterns found no
matches. No Go tests or application suite were run.

## Go 1.26.9 remediation and source re-verification

This addendum follows the Go 1.26.8 source-mode findings recorded above. The
authorized local remediation selects Go 1.26.9 across the active shared
toolchain pins and updates G01/G02 executable BuildInfo admission to require
the exact version. SDK `v0.4.0` and runner `v2.337.0`
remain unchanged; earlier release, advisory, and Go 1.26.8 scan records are
historical evidence and remain intact.

### Toolchain and scan provenance

Go `1.26.9` was resolved as the native Darwin/ARM64
`golang.org/toolchain@v0.0.1-go1.26.9.darwin-arm64` module through
the public Go module proxy, with checksum verification through sum.golang.org:
module `h1:u1IV5D8HPeURJjWAqzcC8QRXDJqvXlh7FpsA4lGIg1c=`, go.mod
`h1:8wlg68NqwW7eMnI1aABk/C2pDYXj8mrMY4TyRfiLeS0=`. The official
Go `1.26.9` source tag resolves to commit
[`2ae494ef9fb90e0da0073c31800ba38870249994`](https://github.com/golang/go/commit/2ae494ef9fb90e0da0073c31800ba38870249994).
The trusted `govulncheck@v1.7.0` module is
`h1:4MQBuhmXbz2uepNJrf3v+aaZLGDqw1JluwYboegA1qg=`; its pinned
source documentation is
[`cmd/govulncheck/doc.go`](https://github.com/golang/vuln/blob/617f44b718537dccdea1915395650e0529e3b72e/cmd/govulncheck/doc.go).
Before the batch, `go version` reported
`go1.26.9 darwin/arm64` and `govulncheck -version`
reported scanner `v1.7.0`, database URL
`https://vuln.go.dev`, and database-modified metadata
`2026-10-08T22:31:09Z`. The native cgo compiler was Apple Clang
`21.0.0`.

The seven-profile source tree was staged at tree
`e37ee1834af7e6ba2543eed373396eb9da657b3a`; the G01 module
subtree was `da98e542ea2a8066a3af664a4b5d3f7bdf655ebb` and the G02
module subtree was `4526e7951921658b5b0fc13979e488667cc0d98b`.
These identify the scanned local source before this evidence text was added;
no untracked Go files were present.

Each profile ran sequentially from `experiments/g01-scaleset`
with `govulncheck -mode=source -format=text
-db=https://vuln.go.dev -test [-tags=<profile>] ./...`, with a 900-second
per-profile deadline. The child
environment was built from an allowlist: exact Go `1.26.9` first
on `PATH`; `GOTOOLCHAIN=go1.26.9`,
`GOENV=off`, `GOWORK=off`,
`GOFLAGS=-mod=readonly -p=2`, `GOAUTH=off`,
`GOEXPERIMENT=none`, `GOFIPS140=off`,
`GOMAXPROCS=2`; Darwin/ARM64 with `GOARM64=v8.0`;
and `GOPROXY=https://proxy.golang.org` only,
`GOSUMDB=sum.golang.org`, and empty private module/no-proxy
scopes. The existing Go module/build caches were explicitly selected;
`GOCACHEPROG` was unset, `GOROOT` was omitted for
default toolchain derivation, and no `HOME`, Git/GitHub, SSH,
proxy credentials, overlays, or ambient Go settings were passed. For cgo
profiles, `CC`/`CXX` selected the recorded Apple
Clang compiler. The scan date was 2026-10-13 UTC.

| Profile | CGO | Build tags | Elapsed | Text result |
|---|---:|---|---:|---|
| A | 1 | default | 1.6 s | exit 0; no finding IDs |
| B | 1 | `g01_live,g01_worker` | 1.3 s | exit 0; no finding IDs |
| C | 1 | `g01_pair_fixture` | 1.3 s | exit 0; no finding IDs |
| D | 1 | `g01_live,g01_worker,g01_pair_fixture` | 1.3 s | exit 0; no finding IDs |
| E | 1 | `g01_pair_fixture,g01_pair_real_cadence` | 1.3 s | exit 0; no finding IDs |
| F | 0 | default | 1.5 s | exit 0; no finding IDs |
| G | 1 | `osusergo` | 1.3 s | exit 0; no finding IDs |

All seven invocations completed without scanner, package-loading, or database
errors; text output contained no vulnerability IDs. The scanner queried the
mutable public database without an immutable snapshot identifier. A separate
GET to the database index after the batch returned body field
`modified=2026-10-08T22:31:09Z`, ETag
`8875c8c67555e9c875a431c63837b83d`, Last-Modified
`2026-10-13T04:17:36Z`, and HTTP Date
`2026-10-13T06:33:35Z`; its
`Cache-Control` was `public, max-age=3600`. These
mutable index/response times are not per-invocation scan timestamps or a
database snapshot bound to every profile.

The Go 1.26.8 findings and exit-3 results above remain historical. This
Go 1.26.9 run found no known vulnerabilities in the seven stated source
profiles; that is not a guarantee that the toolchain, dependencies, tests, or
all runtime paths are vulnerability-free. `-test` loaded test
source for analysis but did not execute tests or fixtures. The source analyzer
may load or compile package export data; the scanner itself was installed with
`go install`. No G01/G02 application or test binary was launched by
the scan, and the scan batch ran no `go test` command.

### Pin admission and focused verification

Tests were added before implementation. Against the prior Go 1.26.8 state,
`TestToolingCheckToolchainRequiresGo1269` failed because the
checker still accepted 1.26.8 and rejected the required 1.26.9 fixture;
G01 live and worker `TestBuildRevisionAdmissionRequiresGo1269`
also failed their Go 1.26.9 admission cases, and G02
`TestBrokerBuildMustMatchReviewedController` rejected the new
reviewed-build metadata. After the pin and exact BuildInfo gates changed,
the focused tests passed under Go 1.26.9. The tests accept only the exact
`go1.26.9` build value and reject Go 1.26.8, Go 1.27.1, and an
empty version while retaining the existing revision, SDK, tag, cgo, and
platform checks.

Focused verification under the controlled Go 1.26.9 environment:

- `make toolchain`: exit 0; selected `go1.26.9`.
- Root tooling subset
  `go test -count=1 -run '^(TestToolingCheckToolchainRequiresGo1269|TestToolingPinsNewerSystemGo|TestToolingEstablishedModulesAreRequired|TestToolingOfflineModuleSelection|TestToolingDefaultG01PartitionsRun|TestToolingDefaultG02PartitionsRun|TestFastCheck.*)$' ./scripts`:
  exit 0, 304.652 s; the subsequent exact toolchain-table rerun also passed
  (0.809 s).
- G01 admission tests
  `go test -count=1 -tags=g01_live -run '^TestBuildRevisionAdmissionRequiresGo1269$' ./cmd/g01-live`
  and the corresponding `g01_worker`/`./cmd/g01-worker`
  command: both exit 0 (0.351 s and 0.298 s).
- G02 focused build-admission tests
  `go test -count=1 -run '^(TestBrokerBuildMustMatchReviewedController|TestBrokerBuildRejectsFixtureCapability|TestBrokerBuildUnsupportedNativeAdmissionRefuses|TestPairedFixtureBuildRequiresExactGo1269)$' .`:
  exit 0 (0.334 s).
- Final `go vet` checks for `./scripts`,
  G01 `./cmd/g01-live` with `g01_live`, G01
  `./cmd/g01-worker` with `g01_worker`, and G02
  `.`: all exit 0.
- `go mod tidy -diff` for the root, G01, G02, and R1 modules:
  exit 0 with no module changes. No `go.sum` changed.
  `gofmt -d` on changed Go files and `git diff --check`
  emitted no differences or diagnostics.

The focused Go tests exercise synthetic tooling and admission fixtures only;
they do not run the full root, G01, G02, or R1 suites and do not execute
production runner, live-canary, or real-cadence operations. No packet specimen,
live service, runner, credential, Docker, or host operation was used. Existing
G01/G02 source, authorization, safe-drain, tenant/archive/image, and live
qualification gates remain open; no issue or Project state changed. SDK and
runner pins remain `v0.4.0` and `v2.337.0`. No Go
dependency or checksum pin changed. Rollback for this local remediation is the
single coherent local documentation/code pin commit; no external state changed.
