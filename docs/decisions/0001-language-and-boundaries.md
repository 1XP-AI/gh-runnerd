# ADR 0001: Go daemon with replaceable execution adapters

Status: accepted planning decision; SDK and platform integrations remain evidence-gated.

## Decision

Use Go for the CLI, background service, scheduler and state reconciliation. The workload is mostly API I/O, process supervision and durable bookkeeping; no evidence identifies computation as a bottleneck. Start with one language and one daemon. Select a supported stable Go release compatible with the pinned SDK during bootstrap; do not follow `@latest` at runtime.

| Candidate | Benefit | Cost in this product |
|---|---|---|
| Go | Official Scale Set client; standard race detector/fuzzing; simple native deployment | Races remain possible; GC and concurrency behavior require measured limits and tests |
| Rust | Strong compile-time ownership and data-race guarantees; predictable memory management | No equivalent official client was established; rewriting the preview protocol or maintaining a Go bridge adds integration and recovery risk |
| Swift | Strong macOS API and future UI integration | SDK integration would require a bridge or protocol implementation; unnecessary complexity for the first headless service |

This is a product-level safety and maintenance tradeoff, not a claim that Go has stronger memory/concurrency guarantees than Rust. No comparative performance benchmark has been run. Revisit only after profiles or a concrete platform requirement show a material bottleneck. A small audited platform bridge may be introduced if real Keychain/launchd tests require one; do not promise a fully static binary before those dependencies are selected.

## Boundaries

- SDK behind an internal adapter, pinned with contract tests. Reviewed source commit: `cb0405b2d874500e75ae34eff8d582ab75956b45`; latest release observed: `v0.4.0`. These differ; G01 selects a release/commit deliberately.
- Existing Docker Engine connection is explicit and scoped. Docker Desktop and Lima are runtime choices, not daemon dependencies. No global context mutation and no surprise VM installation.
- macOS native execution is limited to operator-approved trusted repositories; VM isolation will be a separate adapter and release gate.
- Avoid direct embedding of Tart until its FSL terms and distribution implications have been resolved. Do not describe FSL as permissive MIT/Apache open source. An external optional integration still needs explicit license documentation.
- Separate interface definitions from providers so later VM or remote-host implementations do not change CLI behavior.

## Optimization and safety requirements

Prohibit first-party unsafe code unless a separately reviewed ADR justifies it. Use bounded goroutine pools, context cancellation, timeouts, retry backoff with jitter, a single scheduler owner/transactional reservations, typed states and structured error categories. No shell construction from untrusted strings, unsafe deserialization or undocumented low-level optimizations. Prefer the standard library; new dependencies need maintenance/license review.

Validate with `go test -race`, property/state-machine tests, fuzzing of config/IPC inputs, known-vulnerability scanning and resource measurements. Set quantitative latency/memory budgets from the first measured prototype and check regressions on the same hardware. A process manager's RSS improvements never justify weakening crash consistency or trust boundaries.

Sources: [official client](https://github.com/actions/scaleset), [Go security/fuzzing](https://go.dev/doc/security/), [Rust concurrency guarantees](https://doc.rust-lang.org/book/ch16-00-concurrency.html), [Tart license](https://github.com/openai/tart/blob/main/LICENSE).
