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

## Intended delivery

The original product envelope is still a native macOS ARM64 Go CLI plus supervised daemon, user-owned GitHub App authentication, Linux workers through an explicit Docker Engine connection, trusted-only native macOS processes, multiple organizations and a shared host budget. **There is no runnable product yet** (`cmd/gh-runnerd` is an empty entry point). Maintainer-accepted sequencing in [#66](https://github.com/1XP-AI/gh-runnerd/issues/66) splits that envelope:

- **R1 Internal MVP:** this Mac, one organization, one private test repository, the existing Linux-container backend, concurrency one, manual App, foreground command. Full G01 ACK/acquisition/JIT recovery remains required. Full G02 #2 remains a required pre-release evidence gate classified R3; R1 is not independently deliverable until that gate passes.
- **R2 Everyday operations:** install/start/stop/status, restart recovery, bounded scaling.
- **R3 General distribution:** multi-organization support, additional native backend, automated onboarding, signing/update/diagnostics. Full G02 original acceptance remains here and also gates R1 #68 production.
- **Future:** optional macOS VM / multi-host research ([#21](https://github.com/1XP-AI/gh-runnerd/issues/21)).

Linux still needs a Linux kernel/runtime on macOS. R1 connects to an existing engine; it does not make Linux containers native macOS processes, add a second backend, or provision a Kubernetes cluster. See [approved delivery releases](docs/PLAN.md#approved-delivery-releases).

## Track implementation

[GitHub Project](https://github.com/orgs/1XP-AI/projects/2) provides [Goals](https://github.com/orgs/1XP-AI/projects/2/views/1), [Ready](https://github.com/orgs/1XP-AI/projects/2/views/2) and [Board](https://github.com/orgs/1XP-AI/projects/2/views/3) views. The original 21 goal issues remain; the live board has 36 items after additive Release metadata and R1 children. Start from currently Ready work on the live Project; do not redispatch from historical `docs/backlog.json` Ready values. The board does not dispatch agents automatically.

## Read the plan

- [Product and architecture plan](docs/PLAN.md)
- [Language and deployment decisions](docs/decisions/0001-language-and-boundaries.md)
- [Authentication and trust boundaries](docs/SECURITY-DESIGN.md)
- [TDD and release evidence](docs/TEST-STRATEGY.md)
- [Issue goals and agent execution](docs/EXECUTION.md)
- [Public CI and local checks](docs/CI.md)
- [Dependency and license inventory](docs/DEPENDENCIES.md)
- [Published issue goals](docs/ISSUES.md)
- [Ordered backlog](docs/BACKLOG.md)
- [Sources and unresolved experiments](docs/SOURCES.md)

R1 is one Apple Silicon Mac and one organizational installation. Two-organization qualification remains the G16/R3 target. Organization examples in configuration are illustrative. No existing runner is migrated or removed by this repository.

## License

MIT. Runtime dependencies and downloaded OS images keep their own licenses. No macOS image, restricted virtualization tool, credential, or third-party private key is distributed here.
