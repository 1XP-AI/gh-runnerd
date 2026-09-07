# Dependency and license inventory

Checked 2026-09-07 for the G03 bootstrap. The runtime module intentionally has no third-party Go dependencies yet, so no `go.sum` is needed. Future implementation issues must add every runtime module to the table and keep `make deps` and `make licenses` passing.

## Runtime and toolchain

| Module or component | Version | License | Use and source |
| --- | --- | --- | --- |
| `github.com/1XP-AI/gh-runnerd` | local module | MIT | Project code; [repository license](../LICENSE). |
| Go standard library | `go1.26.8` | BSD-3-Clause | Language/runtime baseline; [official release history](https://go.dev/doc/devel/release). |

`go list -m all` is the source of truth for the runtime module graph. `scripts/check-licenses.sh` verifies that every listed module has a row in this inventory and a top-level `LICENSE`, `COPYING`, `NOTICE` or equivalent file. The current graph contains only the local module.

## Public CI actions

Action refs are immutable commit pins. The version labels are recorded for human review; the workflow uses the full SHA.

| Action | Immutable ref | License | Source |
| --- | --- | --- | --- |
| `actions/checkout` | `11bd71901bbe5b1630ceea73d27597364c9af683` (`v4.2.2`) | MIT | [action repository](https://github.com/actions/checkout). |
| `actions/setup-go` | `d35c59abb061a4a6fb18e82ac0862c26744d6ab5` (`v5.5.0`) | MIT | [action repository](https://github.com/actions/setup-go). |

## CI-only vulnerability tool

The workflow invokes `golang.org/x/vuln/cmd/govulncheck@v1.7.0` with an exact version. It is downloaded into Go's module cache at check time and is not a runtime dependency or shipped binary. The Go vulnerability tooling repository uses the Go project BSD-3-Clause license; its transitive tool dependencies remain governed by their upstream licenses and are not part of gh-runnerd's runtime graph.
