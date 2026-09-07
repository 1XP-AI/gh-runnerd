# G01 broker external-review corrections

This continuation addresses the six valid findings in
[PR29](https://github.com/1XP-AI/gh-runnerd/pull/29), tracked with
[issue30](https://github.com/1XP-AI/gh-runnerd/issues/30) and G01. The earlier audit
at `4acc93fd` retained all seven original findings, including the dot-component
false positive. The current review workflow reads status and all finding details,
including stale sources; a prior internal approval is not merge clearance.

Paused input work was preserved verbatim in local checkpoint commit `d821749`
before integrating main `2fd5844`. That checkpoint is incomplete historical work,
not the selected input implementation. The broker adopts main's independently
reviewed shared private-input helper, including its inherited-pipe poller and
short deadline probes. Existing ownership and byte checks still wrap that helper;
regular-file reads have a byte bound, not a filesystem-stall deadline guarantee.

Red commit `ccead3c` preserves four reproduced failures after main integration:

- [Folded JSON keys](https://github.com/1XP-AI/gh-runnerd/pull/29#discussion_r3949548267)
  still changed interpreted authority. Duplicate matching now uses the same
  Unicode simple-fold equivalence as Go struct-field decoding, including long-s;
  depth/byte budgets and single-alias acceptance remain intact.
- [Organization casing](https://github.com/1XP-AI/gh-runnerd/pull/29#discussion_r3949682046)
  consumed one mint before refusal. The installation login must now exactly match
  the approved canonical organization before issuance.
- [Private broker HTTP debug](https://github.com/1XP-AI/gh-runnerd/pull/29#discussion_r3949682038)
  exposed synthetic Authorization and response canaries at levels1/2. The broker's
  own cloned transport now pins both HTTP protocols and TLS ALPN to HTTP/1.1;
  main's separate G02 default transport fix did not cover this injected transport.
- [Inherited input](https://github.com/1XP-AI/gh-runnerd/pull/29#discussion_r3949682027)
  exceeded the child guard for deadline and cancellation on a real inherited
  current-UID private FIFO. The retained checkpoint test exercises immediate and
  delayed EOF as positive controls, plus both broker/manual cancellation routes.

The fresh red command was
`GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=45s -run 'TestBrokerFolded|TestBrokerCanonical|TestBrokerRepositoryDot|TestBrokerHTTPDebug|TestPrivateInheritedInput$' .`
in the G02 module (expected failure, 5.952s). The same focused suite passed after
the corrections (3.222s). An additional HTTP test preserves an inherited `h2`
ALPN list through the production constructor, changes only loopback trust/dial
configuration afterward, and passes (1.550s). All exchanges use local fixtures;
no real credential or GitHub endpoint is contacted.

The existing first-character-alphanumeric repository rule rejects `.` and `..`
before any API call. Its zero-call behavioral control passes; no source change
is made for the [rebutted finding](https://github.com/1XP-AI/gh-runnerd/pull/29#discussion_r3949548246).
The wrapper may still list that original finding; listing is not a claim that its
thread remains unresolved after the integrator's evidence-based reply.

Red commit `5c80d40` adds the remaining two reproductions:

- [Cross-directory duplicate mint](https://github.com/1XP-AI/gh-runnerd/pull/29#discussion_r3949548235): the same discovery with a changed expiry and new attempt directory minted twice. The finite native-account ledger now consumes a permanent slot before any API call. A distinct broker lock avoids holding the controller admission lock over child execution.
- [Late snapshot reservation](https://github.com/1XP-AI/gh-runnerd/pull/29#discussion_r3949548251): both an existing regular snapshot and a symlink consumed one mint. The exact digest-verified bytes are now exclusively reserved, written and synced before issuance; retained inode/hash checks gate later authenticated effects and handoff.

The red commands in the G02 module were `go test -race -count=1 -run 'TestBrokerDifferentAttempt|TestBrokerBuildUnsupported' .` (failed, 0.621s) and `go test -race -count=1 -run TestBrokerSnapshotCollision .` with a clean metadata-only controller fixture (failed, 0.845s). The controller was built from a clean standalone main `2fd5844` clone with Go1.26.8, CGO enabled, native Darwin/ARM64 and SDKv0.4.0, then only read/hashed. It was never executed. Fixture-based tests skip explicitly when those optional private paths are absent; default synthetic snapshot tests still exercise pre-mint collisions and post-mint replacement refusal.

The reviewed main controller now requires native account lookup. An adjacent actual-binary reproduction confirmed that the older broker accepted both a clean no-CGO binary and an `osusergo` binary even though that controller deterministically refuses its admission lookup. The verifier now also requires native host GOOS/GOARCH, CGO_ENABLED=1 and absence of the exact osusergo tag, preserving Go/path/clean SHA/SDK checks. An independently built native positive control remains accepted. Real unsupported binaries were tested through the private front door with synthetic API fixtures and a guaranteed snapshot collision; they never execute, even on a regression. Replaying that added fixture against production source at red `5c80d40` failed with one mint for all four unsupported/collision combinations (0.985s); this is an added scratch-test replay, not a claim that the added fixture was in the original red commit. The native-positive plus unsupported front-door matrix then passed on the correction (2.182s).

The finite contract is described in [the updated operator guide](g01-broker.md): one discovery and one of each named controller phase, including only one inspection and cleanup. The first controller authority must include all intended mutating phases. Expiry/phase renewal is later recovery only, with stable resource, binary/harness, workflow/hosts/nonce and state identity. Earlier incomplete slots survive later recovery completion. Further reconciliation, repeated inspection and production refresh remain unresolved gates. No automatic root creation, state adoption, reset or migration is supplied.

Supplemental negative/positive fixtures cover malformed and contradictory valid-JSON receipts, authority rollback/rebinding, missing/symlinked root, directory sync failure/re-sync, root/parent replacement, cross-process lock contention, killing only an owned synthetic child after its issuance intent, and restart refusal with unchanged permanent record bytes. Matching pre-existing controller claim metadata is accepted; conflicting, symlinked, empty or partial inventory refuses before mint. A separate G01 package fixture invoked main2fd's actual local `openJournalAtAdmission` with injected private directories, wrote its typed approval/claim, closed it, and the broker read-only compatibility check passed against those files. No G01 Driver or API method ran. This establishes the actual typed JSON/ownership digest and inode contract without a cross-module replace or production import.

The four non-admission corrections at `20cdb53c8e4a16a439b1726cbfb560595b1c3bbd` received independent Astra review and fresh external-probe reruns (1.918s; inherited FIFO/debug suite 3.006s). The final combined ledger/build-profile delta still requires independent exact-head review, hosted CI and external Codex review. Passing these synthetic tests is not live authorization.

No live use, resource approval, production recovery, credential persistence or
G01 completion is established by these synthetic checks.

Current supplemental checks passed on Darwin ARM64/Go1.26.8: finite phase, directory/expiry, build metadata and authority cases (2.622s); malformed receipts, renewal and existing claim cases (3.892s); separate-process crash and sync/replacement cases (1.950s); source-wide ledger cases (4.830s); actual G01 typed claim bridge and native snapshot fixture (1.915s). A full default module race suite passed before the final documentation/replay-history tightening (library17.768s, broker CLI1.501s, enrollment CLI2.486s). Final integrated checks are recorded separately when complete; these numbers do not imply final-head review.
