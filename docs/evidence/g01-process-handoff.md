# G01 private process handoff

This implements the approved bounded offline increment for issue #1 on the
`7dc2cbd989d3ae01e6930790de6f25dd9d538f6e` base. It does not complete G01/G02 or
authorize live execution. Implementation route: Codex gpt-6.1-sol/high, explicit
current user override. Ordinary `RunBroker`, CLI quarantine, production build
admission, dependency pins, default roots and journal schema remain unchanged.

The shared stdlib-only codec adds a separate Ed25519 credential-binding envelope.
Its domain is ASCII `gh-runnerd/g01-issued-credential-binding/v1` plus NUL and
ordered typed unsigned JSON. Exact raw approval and v1 receipt hashes, exact
payload length/hash, workflow identity, issuer/workflow key IDs and sources,
create audience/phase, owner/receipt nonces and issuance/expiry are authenticated.
The existing deterministic v1 receipt signing encoding is unchanged. Strict
binding decoding rejects unknown, missing, null, duplicate and case-folded fields.

Workflow and issuer public roots are independently selected by private receiver
assembly, with distinct keys and roles. Broker assembly compares the entire
workflow root identity before management input. No root comes from messages,
argv or environment. Ordinary runtime has no configured roots or signer. Only
test code and the excluded `g01_pair_fixture` bridge hold synthetic private keys;
the child Go test executable compiles independent public pins. Production binary
admission continues to reject fixture capabilities.

The private broker path snapshots its verified receipt once before management
input. Only the authenticated post-mint/preflight block constructs the owned
handoff, under the durable attempt/admission guards. There is no public
caller-asserted issued-signing API. Once selected, the path has no legacy-launch
fallback. Signer failure, lost claims, start/write/cancel/child failure and output
overflow retain the permanent attempt; none automatically remints or relaunches.

FD3 carries `G01H`, uint32 big-endian version 1, receipt length and envelope
length, exact receipt (1–16384 bytes), exact envelope (1–4096 bytes), then EOF.
Stdin carries exactly 1–16384 serialized credential bytes and EOF. The fixed
launcher owns two separate pipes, closes parent copies of child endpoints after
start, fully closes control before writing credentials, runs its writer beside
child wait, closes on cancellation/early exit, joins its writer/watcher and uses
the existing combined 8 KiB output budget. It kills only its created child.

A single cumulative 30-second ceiling is selected before binding/signing. The
signer context, envelope, owned handoff and launcher carry that same deadline,
shortened by parent, approval, receipt/grant and token-expiry-minus-one-minute
limits. Receiver acquisition, credential input and create remain under this
ceiling; later operations cannot reset it. The receiver rejects missing, regular
or aliased pipe descriptors, validates both roots/signatures and bindings before
stdin, then wraps the existing proof-preserving create path. Durable nonce fsync
precedes bounded payload input; exact length/hash precede strict credentials,
semantic checks, the lazy API factory and Driver effect. Failure burns the create
slot across fresh nonces, reopen and concurrency.

Inherited blocking descriptors are transferred to nonblocking close-on-exec
duplicates registered with Go's poller, with originals immediately closed. This
is necessary on macOS: a blocking kernel read was not interrupted by close in
the initial withheld-EOF probe. The final reader runs in the caller goroutine;
its cancellation watcher closes the concrete pipe and is joined before return.
Transient byte buffers are cleared best effort; decoded strings/adapters retain
copies. Journals/errors receive no payload/envelope hashes, envelope or tokens.

Behavioral TDD: a compiling receiver with digest enforcement omitted accepted
signed payload A plus valid payload B into one factory and one create. The test
required durable consumption and zero factories/creates and failed. Adding the
length/hash check passed; temporarily removing it reproduced that failure, then
restoring it passed. Focused coverage includes independent role/root/source and
identity failures before input; byte/semantic failures after consumption;
replay/reopen/concurrent once; preflight/claim/sign/launch refusal; malformed,
truncated, oversized, trailing, missing/aliased descriptor and withheld-EOF input;
blocked streams, cancellation, early exit and output overflow. A tagged test
invokes actual fake-HTTP observation, broker mint/preflight and durable claims,
actual launcher/carrier, separate Go TEST receiver, fsynced nonce and one fake
create. It starts no real controller/liveworker or external service.

The worker's private implementation report records exact commands/results and
local commit identity. Independent exact-source review, GitHub Codex review and
PR quick checks remain coordinator-owned. No full matrix or live tests were run.
Real signer custody, root distribution/rotation/revocation, deployment identity,
trusted-host races, platform evidence, controlled recovery, cleanup and explicit
live authorization remain external gates. Distinct keys do not isolate hostile
same-UID code or mutually compromised signers.
