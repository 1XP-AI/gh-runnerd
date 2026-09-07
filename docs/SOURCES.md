# Evidence ledger

Checked 2026-09-07. Links describe upstream behavior; local product implementation remains untested. Pin versions in implementation issues instead of following moving branches.

| Source | Finding / consequence |
|---|---|
| [actions/scaleset README](https://github.com/actions/scaleset) | MIT, public preview, non-Kubernetes client, JIT, outbound message sessions and total assigned job statistics |
| [Audited SDK commit](https://github.com/actions/scaleset/commit/cb0405b2d874500e75ae34eff8d582ab75956b45) | Code-review snapshot; not equivalent to latest released v0.4.0 |
| [Listener at reviewed commit](https://github.com/actions/scaleset/blob/cb0405b2d874500e75ae34eff8d582ab75956b45/listener/listener.go) | Current high-level handleMessage ACKs before callbacks; do not infer exactly-once durable behavior from README flow |
| [SDK errors](https://github.com/actions/scaleset/blob/cb0405b2d874500e75ae34eff8d582ab75956b45/errors.go) | Raw error output can contain URLs/body data; normalize before logging |
| [GitHub App runner auth](https://docs.github.com/en/actions/how-tos/manage-runners/use-actions-runner-controller/authenticate-to-the-api) | Org self-hosted runner write permission; repository Administration not required for org scope |
| [GitHub App registration](https://docs.github.com/en/apps/creating-github-apps/registering-a-github-app/registering-a-github-app) | Multiple organization installation needs Any account; webhooks may be disabled |
| [App Manifest](https://docs.github.com/en/apps/sharing-github-apps/registering-a-github-app-from-a-manifest) | Browser approval/code exchange; loopback acceptance is an experiment, not established by OAuth rules |
| [Apple Keychains TN3137](https://developer.apple.com/documentation/technotes/tn3137-on-mac-keychains) | Credential storage must match login-agent/system-daemon execution context |
| [GitHub self-hosted reference](https://docs.github.com/en/actions/reference/runners/self-hosted-runners) | Linux + Docker required for Docker job/service actions; ephemeral registration alone is not an OS reset |
| [GitHub secure use](https://docs.github.com/en/actions/reference/security/secure-use) | Self-hosted execution must account for untrusted workflow code and credentials |
| [Docker Desktop runtime](https://docs.docker.com/desktop/use-desktop/kubernetes/) | Mac Linux execution still uses a VM; Kubernetes is optional |
| [Go security](https://go.dev/doc/security/) | Native fuzzing and vulnerability tooling |
| [Rust concurrency](https://doc.rust-lang.org/book/ch16-00-concurrency.html) | Compile-time ownership/concurrency advantages were considered |
| [Mactions](https://github.com/Kyter-com/Mactions) | Similar product shape, explicitly proof-of-concept; not adopted as production base |
| [Tart license](https://github.com/openai/tart/blob/main/LICENSE) | FSL-1.1-ALv2 terms differ from permissive open source; no default bundling |
| [Apple macOS license](https://www.apple.com/legal/sla/docs/macOSTahoe.pdf) | VM deployment and redistribution conditions must be checked for a future provider |

## Not yet proven

- Manifest loopback creation and disabled-webhook registration on GitHub.com with two actual org installations.
- Correct Keychain behavior under the exact launchd service identity, including locked/reboot state.
- SDK release pin and crash/replay behavior at each ACK/state write boundary.
- Runner/DinD topology compatibility with this team's DB service workflows on ARM64.
- Native provider cleanup and recovery with representative real tests; no strong isolation claim.
- CPU/memory/queue latency baselines and 24-hour mixed-workload soak.
- GitHub App/organization runner policies under the operator's account plan; repository access is a separate check from credentials.
