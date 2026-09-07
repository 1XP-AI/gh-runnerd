# G01 paired execution and collection — issue52

This private library connects the controller/worker journals and collects bounded
identity evidence. No command, broker phase, cleanup or live run is authorized.
Implementation and its final evidence are still pending at this red checkpoint.

## Compiled feature red

The private runPairedBaseline and observeRoster methods initially return unresolved.
Tests compile and fail at their feature assertions after actual private C/W
FileJournal initialization and concrete private TLS/Unix fixture construction.
The C fixture contains the original inventory/create history; it does not run
remote policy Preflight or claim a live original creation. The worker helper
creates/fsyncs its own canonical generated state/admission directories and permits
reopening only those roots. No real account root is read or opened.

The paired control requires exactly one acquire/JIT/create/start, eight rounds,
known outstanding session and zero cleanup. The two roster controls require
complete structured observations for empty and multi-page responses whose legacy
Inventory controls already succeed through actual TLS.

```text
GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=120s -tags=g01_pair_fixture ./livecanary -run '^(TestPairedBaselineActualJournalsExecuteAndCollect|TestRosterActualTLSCompleteObservation)$'
```

Run from the G01 module: exit1,0.469s. All three assertions fail behaviorally;
there is no missing-symbol or fixture-setup failure in this retained red. This
shows a new feature is unimplemented, not a regression in an existing path.
No actual SDK acquisition, worker process, runtime or external resource occurred.
