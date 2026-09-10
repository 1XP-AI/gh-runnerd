# Dependency and license inventory

Checked 2026-09-07 for the G03 bootstrap. The runtime module intentionally has no third-party Go dependencies yet, so no `go.sum` is needed. Future implementation issues must add every runtime module to the table and keep `make deps` and `make licenses` passing.

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
| `actions/checkout` | `11bd71901bbe5b1630ceea73d27597364c9af683` (`v4.2.2`) | MIT | [action repository](https://github.com/actions/checkout). |
| `actions/setup-go` | `d35c59abb061a4a6fb18e82ac0862c26744d6ab5` (`v5.5.0`) | MIT | [action repository](https://github.com/actions/setup-go). |

## CI-only vulnerability tool

The workflow invokes `golang.org/x/vuln/cmd/govulncheck@v1.7.0` with an exact version. It is downloaded into Go's module cache at check time and is not a runtime dependency or shipped binary. The Go vulnerability tooling repository uses the Go project BSD-3-Clause license; its transitive tool dependencies remain governed by their upstream licenses and are not part of gh-runnerd's runtime graph.
