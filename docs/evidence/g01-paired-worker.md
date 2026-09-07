# G01 paired worker scope — issue #46

This bounded worker-side feature follows merged issue #44. It does not implement
controller pairing, acquisition/JIT, a command phase, broker slot or live execution.
Synthetic controller checks prove worker behavior only; actual two-journal
integration remains required before live wiring.

## Compiled feature red

Starting from main `f676e8cc83625b7090b0d62d70fdeefacca42e28`, added the approved
seven-method API and concrete receipt types with a fail-closed implementation.
Both actual private FileJournal tests compile and fail behaviorally:

```text
GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=45s ./liveworker -run '^TestPairedActualFile'
```

The lifecycle test cannot enter the lease-held callback or obtain its durable
binding/create/start/observation/deletion receipts. The bound-state test cannot
produce the original binding needed to test copied-receiver revocation and
reopen refusal. The command exits 1; this is not a missing-symbol compilation
failure. Fixtures use actual temporary worker journals/claims and a synthetic
runtime/controller checker, without any real account admission root or runtime.

Implementation, green verification and reviews are pending in this red commit.
