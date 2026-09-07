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

Admission and snapshot preparation remain separate corrections in progress.
No live use, resource approval, production recovery, credential persistence or
G01 completion is established by these synthetic checks.
