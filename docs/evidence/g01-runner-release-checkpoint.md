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
queries used `--paginate` with no explicit `per_page` override; the two
repository feeds requested `state=published` only. Results are advisory-feed
matches, not a vulnerability scan. Commands below show the endpoint and API
filters; `--jq` was used only to project response counts or fields.

```text
gh api --paginate 'repos/actions/scaleset/security-advisories?state=published'
gh api --paginate 'repos/actions/runner/security-advisories?state=published'
gh api advisories/GHSA-2c6m-6gqh-6qg3
gh api --paginate 'advisories?ecosystem=actions&affects=actions/runner@2.337.0'
```

The Scale Set repository feed returned `[]` (0). The runner repository feed
returned one record: GHSA-2c6m-6gqh-6qg3 / CVE-2022-39321, severity `medium`,
published and updated `2022-10-24T16:00:18Z`, with vulnerable range `<2.296.1`
and listed patched versions `2.296.2`, `2.293.1`, `2.289.4`, `2.285.2`, and
`2.283.4`. The exact Actions ecosystem query for `actions/runner@2.337.0`
returned `[]` (0). ([runner repository advisory](https://github.com/actions/runner/security/advisories/GHSA-2c6m-6gqh-6qg3))

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
gh api --paginate 'advisories?ecosystem=go&affects=github.com/actions/scaleset@v0.4.0'
gh api --paginate 'advisories?ecosystem=go&affects=github.com/golang-jwt/jwt/v4@v4.5.2'
gh api --paginate 'advisories?ecosystem=go&affects=github.com/google/uuid@v1.6.0'
gh api --paginate 'advisories?ecosystem=go&affects=github.com/hashicorp/go-cleanhttp@v0.5.2'
gh api --paginate 'advisories?ecosystem=go&affects=github.com/hashicorp/go-retryablehttp@v0.7.8'
```

Each returned `[]` (0 matching published advisories) for the exact pinned
external module version named in G01's existing dependency files. The positive
query control was:

```text
gh api --paginate 'advisories?ecosystem=go&affects=github.com/golang-jwt/jwt/v4@v4.5.1'
```

It returned GHSA-mh63-6h87-95cp / CVE-2025-30204, severity `high`, published
`2025-03-21T22:04:00Z`, updated `2025-04-10T13:02:35Z`, affected range
`<4.5.2`, first patched `4.5.2`. This control is for the older `v4.5.1` query,
not an advisory match for installed `v4.5.2`. ([query-control advisory](https://github.com/advisories/GHSA-mh63-6h87-95cp))

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
