# Authentication and security design

## Enrollment

Each operator owns a GitHub App and its private key. Two organizations use different installation IDs under one App; the App must permit installation on other accounts ("Any account"). This does not publish organization repositories or require Marketplace distribution. Because organization permissions are requested, an organization owner must authorize installation.

For organizational runner management, request `organization_self_hosted_runners:write` and baseline metadata read permission. Repository Administration is unnecessary for this scope. Add Actions read only if a separately justified job/log/policy feature requires it. Do not ship a universal App private key, reusable registration token, PAT or OAuth client secret in public binaries.

Planned `init` supports App import first and optional Manifest-assisted creation once G02 passes. The browser performs user approval. Use loopback-only setup endpoints, one-time state, exact Host/path validation and Origin checks where applicable (normal browser redirect GETs may omit Origin), bounded lifetimes and code replay rejection. Do not equate Manifest `redirect_url` with an OAuth callback; prove loopback behavior. Abort/retry must not leak PEM files or silently create duplicate Apps. No persistent public webhook receiver is required for Scale Set polling.

## Runtime credentials

The controller uses App JWTs and organization-specific installation credentials to obtain runner registration/Actions admin credentials and JIT runner configuration. The official SDK handles portions of this flow; keep it behind a credential broker. Validate expiration, clock skew, refresh serialization, 401 refresh, permission denial and installation removal. Prevent org A credentials from being cached or sent under org B's identity.

App credentials remain in the controller, never in worker environment, argv, mounts, logs or state DB. JIT configuration is itself sensitive and short-lived. Use the safest supported runner bootstrap transport established in G01, with explicit documented residual exposure where unavoidable. Do not log raw SDK errors: reviewed `errors.go` includes HTTP response bodies and URLs. Normalize and redact errors at the boundary; test canary secrets in every output sink.

Credential storage must match the actual launchd context. Login-agent and system-daemon Keychain behavior differs; Apple notes that launchd daemons cannot use the data-protection keychain. Start with honest login-scoped operation and fail closed while credentials are locked. Login-free boot is a separate verified feature, not a marketing alias for login startup. A chmod-restricted file is not a stronger trust boundary than its owning account.

## Execution trust

Native macOS jobs share their execution account/OS. The supported profile separates the controller identity from job identities and defaults to separate job identities per organization. A narrow allowlisted helper, if required, is verified in G02/G11; the networked controller does not run as root. Different directories or Keychain storage do not stop malicious same-user code from attacking the manager. Same-controller-UID execution is not a protected default. v0.1 native pools require explicitly acknowledged trusted repositories and a shared trust domain. The organizations must trust each other before sharing a native account. Do not offer this provider for untrusted forks.

Use GitHub runner-group repository access policies as the primary admission boundary and verify selected repositories at setup. Current Scale Set messages do not establish trustworthy fork-head identity; fail closed where additional verification is required. Public repository contribution tests execute on hosted runners, never automatically on privileged organization pools. Labels only route work; they do not establish authorization.

Linux workers get a dedicated Docker daemon and worker-owned networks/storage so DB services work without exposing the manager engine socket. Prefer a disposable runner-plus-DinD execution unit and validated mount layout. Privileged DinD is still not a hostile-tenant security boundary against the runtime VM; describe that limit and isolate trust domains accordingly. Validate image digest/architecture, minimize inherited environment and do not share writable credential directories.

## Local control and cleanup

Private Unix socket and state directories, checked peer UID, single daemon lock, explicit runtime endpoint, validated pool IDs and path ownership. Refuse unsafe symlinks/path traversal. Persist creation intent and a unique ownership nonce before creation; record immutable external resource IDs after response or verified discovery. Only delete resources carrying this manager's verifiable ownership identity. Never use global Docker prune, wildcard kill, recursive cleanup outside a verified owned root or silent force termination during scale-down.

Removing an organization disables admission, drains workers and revokes local access. Ambiguous, stale or permission-denied state is quarantined for reconciliation; it is not assumed empty. Host reboot recovery does not transparently replay user workflows with potentially irreversible side effects.

See [TDD evidence](TEST-STRATEGY.md) and [source ledger](SOURCES.md). Real GitHub credentials are never stored in fixtures or published reports.

A trusted job with its worker Docker daemon has broad control over that daemon, including inner privileged containers unless an explicit tested authorization layer restricts them. The protected claim is that the outer runner is launched unprivileged and only its reviewed worker-specific DinD service receives manager-runtime privilege. This does not prevent inner privilege or prove runtime-VM escape resistance.
