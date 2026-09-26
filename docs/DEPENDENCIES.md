# Dependency and license inventory

Checked 2026-09-26 for the G03 bootstrap. The runtime module intentionally has no third-party Go dependencies yet, so no `go.sum` is needed. Future implementation issues must add every runtime module to the table and keep `make deps` and `make licenses` passing.

## Runtime modules

| Module | Version | Replacement | License | Use and source |
| --- | --- | --- | --- | --- |
| `github.com/1XP-AI/gh-runnerd` | `local` | `none` | MIT | Project code; [repository license](../LICENSE). |

`go list -m all` is the source of truth for this exact runtime module table.
`scripts/check-licenses.sh` compares module path, selected version and replacement
identity, rejecting missing, stale and duplicate rows. Use `local` for the main
module, the exact selected version for dependencies, and `none` without a
replacement. A replacement is its Go-reported path, followed by `@version` when
versioned (for example `example.org/replacement@v1.2.3` or `./local-replacement`).
The selected source must also have a top-level `LICENSE`, `COPYING`, `NOTICE` or
equivalent file. This checks the recorded identity and license-file presence;
human review must still determine the license and its obligations. The current
runtime graph contains only the local module; nested experiments remain separate.

## Toolchain

Go standard library: `go1.26.8`, BSD-3-Clause, the language/runtime baseline;
[official release history](https://go.dev/doc/devel/release).

The selector matcher helper adapted from the Go standard library is documented
in the [Go testing matcher adaptation notice](third-party/go-testing-matcher.md).

## Public CI actions

Action refs are immutable commit pins. The version labels are recorded for human review; the workflow uses the full SHA.

| Action | Immutable ref | License | Source |
| --- | --- | --- | --- |
| `actions/checkout` | `3d3c42e5aac5ba805825da76410c181273ba90b1` (`v7.0.1`) | MIT | [manifest at pin](https://github.com/actions/checkout/blob/3d3c42e5aac5ba805825da76410c181273ba90b1/action.yml), [release](https://github.com/actions/checkout/releases/tag/v7.0.1), [license at pin](https://github.com/actions/checkout/blob/3d3c42e5aac5ba805825da76410c181273ba90b1/LICENSE). |
| `actions/setup-go` | `b7ad1dad31e06c5925ef5d2fc7ad053ef454303e` (`v7.0.0`) | MIT | [manifest at pin](https://github.com/actions/setup-go/blob/b7ad1dad31e06c5925ef5d2fc7ad053ef454303e/action.yml), [release](https://github.com/actions/setup-go/releases/tag/v7.0.0), [license at pin](https://github.com/actions/setup-go/blob/b7ad1dad31e06c5925ef5d2fc7ad053ef454303e/LICENSE). |

Both immutable manifests declare `runs.using: node24`; the upstream action
documentation lists Actions Runner `v2.327.1` or later as the Node 24 minimum.
The workflows stay on GitHub-hosted `ubuntu-24.04`. Setup Go v7's ESM migration
does not change its inputs or behavior. The workflow keeps `go-version-file:
go.mod`, the Go 1.26.8 declaration and `GOTOOLCHAIN=go1.26.8`; `cache: false`
keeps module caching disabled. Checkout keeps the exact triggering SHA for
Public CI, the PR head SHA with `fetch-depth: 0`, and
`persist-credentials: false`. The v7 unsafe fork checkout opt-in remains unset.

## CI-only vulnerability tool

The workflow invokes `golang.org/x/vuln/cmd/govulncheck@v1.7.0` with an exact version. It is downloaded into Go's module cache at check time and is not a runtime dependency or shipped binary. The Go vulnerability tooling repository uses the Go project BSD-3-Clause license; its transitive tool dependencies remain governed by their upstream licenses and are not part of gh-runnerd's runtime graph.
