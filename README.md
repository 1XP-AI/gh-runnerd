# gh-runnerd

A planned, fully self-hosted CLI and daemon for managing GitHub Actions runners on a Mac. One local service manages Linux and macOS runner pools across organizations, with fixed capacity, demand-based scaling, safe draining, and recovery.

**Status: design and implementation backlog. There is no runnable product or stability claim yet.** Commands below describe the intended interface, not installed functionality. This project is independent of GitHub.

```console
gh-runnerd init
gh-runnerd org add example-org
gh-runnerd pool add tests --org example-org --os linux --max 3
gh-runnerd autoscale tests --min 0 --max 3
gh-runnerd status
gh-runnerd logs tests --follow
gh-runnerd stop tests --drain
```

## Intended first release

- A native macOS ARM64 Go executable, with a CLI and separately supervised daemon.
- User-owned GitHub App authentication; no management SaaS, shared vendor private key, public inbound endpoint, or Kubernetes requirement.
- Linux runners through an explicit Docker Engine connection. Existing Docker Desktop or Lima engines can be used without changing the user's global Docker context.
- Native macOS process runners for explicitly trusted repositories only. A fresh working directory is **not** OS or credential isolation. macOS VM isolation is a later, separately gated provider.
- Multiple organizations, per-pool limits, and a shared host-wide capacity budget.
- One-job runners, durable state reconciliation, safe scale-down, structured diagnostics, and tests that exercise crashes and concurrent work.

Linux still needs a Linux kernel/runtime on macOS. The first release connects to an existing engine; it does not make Linux containers native macOS processes or provision a Kubernetes cluster.

## Track implementation

[GitHub Project](https://github.com/orgs/1XP-AI/projects/2) provides [Goals](https://github.com/orgs/1XP-AI/projects/2/views/1), [Ready](https://github.com/orgs/1XP-AI/projects/2/views/2) and [Board](https://github.com/orgs/1XP-AI/projects/2/views/3) views. All 21 issues carry a goal, model/effort, TDD cases, acceptance criteria and dependencies. Start with G01/G02/G03; the board does not dispatch agents automatically.

## Read the plan

- [Product and architecture plan](docs/PLAN.md)
- [Language and deployment decisions](docs/decisions/0001-language-and-boundaries.md)
- [Authentication and trust boundaries](docs/SECURITY-DESIGN.md)
- [TDD and release evidence](docs/TEST-STRATEGY.md)
- [Issue goals and agent execution](docs/EXECUTION.md)
- [Published issue goals](docs/ISSUES.md)
- [Ordered backlog](docs/BACKLOG.md)
- [Sources and unresolved experiments](docs/SOURCES.md)

The implementation target begins with one Apple Silicon Mac and two organizational installations. Organization examples in configuration are illustrative. No existing runner is migrated or removed by this repository.

## License

MIT. Runtime dependencies and downloaded OS images keep their own licenses. No macOS image, restricted virtualization tool, credential, or third-party private key is distributed here.
