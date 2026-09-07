# Credential transport debug audit

This correction tracks [audit #30](https://github.com/1XP-AI/gh-runnerd/issues/30)
and extends the [PR 29 HTTP/2 debug finding](https://github.com/1XP-AI/gh-runnerd/pull/29#discussion_r3949682038)
to the already merged G01 controller and G02 enrollment clients. The broker is
not included in this change and remains separately gated.

## Observed failure

At base `166a59bf6a9440ce1498a123efafbadd8eb5105d`, both clients allowed HTTP/2
through a clone of Go's default transport, or through the default transport
itself. Discarding application loggers and returning fixed errors did not stop
Go's `GODEBUG=http2debug=1` or `http2debug=2` traces from printing credential
headers and response data to standard error.

Behavioral red commit `7906138` adds fresh test-process TLS fixtures. Both debug
levels disclosed synthetic Authorization and response canaries in each client;
the tests reported booleans rather than printing the captured data. Debug level
0 completed without that disclosure but negotiated HTTP/2. G02's additional
ownership test failed because its default constructor did not own a transport.
The targeted race runs failed as expected (G01 0.444s, G02 1.159s).

The G01 red commit also extracts the existing transport construction into a
private factory without changing behavior. The fixture uses that factory and
the actual response-budget wrapper, changing only certificate trust and dialing
to reach its own TLS listener. The G02 fixture exercises `NewGitHubAPI(now, nil)`
in an isolated test process whose default pointer is fenced to its local TLS
listener. Neither fixture contacts GitHub or uses real credentials.

## Correction

G01's inner credential transport and G02's privately cloned default transport
now explicitly support HTTP/1 only. Their private TLS configuration advertises
only `http/1.1` through ALPN. Setting `http.Transport.Protocols` alone was not
sufficient: the first working-tree implementation failed the G02 TLS fixture
because `Transport.Clone` initialized and retained an `h2` ALPN advertisement.
The server negotiated HTTP/2 while the client wrote HTTP/1.1. The final fixture
retains the production ALPN configuration so it detects this incompatibility.

Certificate verification and existing destination, proxy, redirect, timeout,
and body-budget behavior are preserved. The change does not overwrite the
shared default transport or an explicitly supplied G02 synthetic transport.
G01's response-budget wrapper still enforces its existing body and PATCH guards.
The private factory's owned configuration is used by the real SDK constructor.

## Verification and limits

The corrected focused race tests passed: G01 1.564s and G02 2.266s. Each client
completed the credential-bearing local request over HTTP/1.1 at debug levels
0, 1, and 2 without either canary in captured logs. Ownership tests verify the
private protocol selection and preservation of injected fixture settings.
The full `make check` also passed: root build/format/vet/unit/race and dependency
gates, both mandatory offline modules (including G01 tagged CLI tests/vet),
and the pinned vulnerability check. G01 livecanary race tests took 1.529s and
G02 race tests took 6.111s. Fuzzing explicitly skipped because no targets exist.

These tests are synthetic and run in the default offline suites. They execute
only Go test subprocesses and local TLS servers. They do not execute an enrolled
controller, runner or worker, mint an installation token, convert a real App
Manifest, contact Docker, or claim live GitHub protocol compatibility. Live
G01/G02 gates and exact-head external review remain required. The narrow change
prevents the observed Go HTTP/2 trace path; it is not a claim that arbitrary
instrumentation or trusted same-user code cannot read process credentials.
