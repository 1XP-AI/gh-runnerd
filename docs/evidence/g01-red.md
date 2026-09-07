# G01 red evidence

Recorded 2026-09-07, before independent reconciliation was implemented. Actual
host: macOS ARM64, `go version go1.25.8 darwin/arm64`. No live credentials or
GitHub runner resources were used. Tests link the real `actions/scaleset v0.4.0`
client/listener to a loopback-only HTTP fixture.

From `experiments/g01-scaleset`:

```text
$ go mod tidy
$ gofmt -w *.go
$ go test -count=1 -run 'TestRecovery' ./...
--- FAIL: TestRecoveryAfterACKCallbackCrash (0.00s)
    recovery_test.go:86: desired capacity stranded after ACK/callback crash: got 0, want 4
--- FAIL: TestRecoveryMissingLifecycleCallback (0.00s)
    recovery_test.go:108: missing lifecycle callback left worker "ready"; want quarantined
FAIL
FAIL github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset 0.376s
FAIL
```

Exit status: **1**, assertions failed; compilation/setup succeeded. The first
test observes the real listener's successful DELETE before the callback crash
barrier, confirms the fake queue no longer redelivers the message, then exposes
the callback-only baseline's stranded capacity. The second exposes an unchanged
ready record when lifecycle notification is missing. These tests demand recovery
and quarantine, not a change to the SDK's ACK ordering. Queue retention is fixture
behavior following upstream documentation, not measured GitHub behavior.
