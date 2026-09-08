# G01 JIT response-loss causality follow-up

Date: 2026-09-08. **Offline synthetic evidence only; no live or production bug
is claimed.** Worktree/branch: `orca/g01-jit-causal-evidence`, based directly on
`origin/main` at `df0c010`.

## Scope and finding

Independent Luna max review found that the historical
`TestSDKJITResponseLossDiscoversIdentityWithoutReissuing` fixture always
returned `owned-1` from `/agents`. Therefore, the old green assertion could pass
without a JIT POST or after a POST that had no fixture-side committed effect.
This correction is limited to the root offline contract test and evidence
records; it does not report a GitHub service defect and does not change an SDK,
transport, retry policy, production adapter or live operation.

## Causal fixture contract

The synthetic handler records requested creation separately from committed
creation under a mutex. A JIT POST always records one request; the configured
negative path drops the response without committing, while the positive path
commits the requested stable name before dropping the response. `/agents`
returns an empty list until that committed name exists, then returns only the
committed synthetic reference. No retry, duplicate-name guarantee or GitHub
idempotency behavior is inferred.

The two response-loss controls each require the missing JIT response (`jit == nil`
with an error), keep admission paused, and retain the creating worker as
quarantined. The pre-create control issues no JIT POST and checks the same
quarantine and admission invariants; the two response-loss controls issue
exactly one.

## TDD record

The behavior-neutral fixture extraction and compiled negative controls were
committed first as `5694fd6` (`test(g01): add causal JIT loss controls`). Against
the old unconditional `/agents` inventory, this exact command was run from
`experiments/g01-scaleset`:

```text
$ GOTOOLCHAIN=go1.26.8 go test -count=1 -run '^(TestSDKJITLookupBeforeCreationDoesNotDiscoverIdentity|TestSDKJITResponseLossWithoutCommitDoesNotDiscoverIdentity)$' .
--- FAIL: TestSDKJITLookupBeforeCreationDoesNotDiscoverIdentity (0.00s)
    protocol_test.go:367: pre-create lookup fabricated a runner identity or changed recovery quarantine
--- FAIL: TestSDKJITResponseLossWithoutCommitDoesNotDiscoverIdentity (0.00s)
    protocol_test.go:379: non-committed response loss fabricated a runner identity or changed recovery quarantine
FAIL
FAIL github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset 0.492s
FAIL
```

Those assertions are retained in the red checkpoint: pre-create requires zero
requests and no recovered reference; non-committed response loss requires one
request and no recovered reference; both require a quarantined worker and paused
admission. The minimal stateful fixture and positive/negative boundary tests
were then committed as `7079b25` (`test(g01): bind JIT lookup to fixture commit`).
The current naming and evidence-wording corrections preserve that prior red
checkpoint; no additional red run is claimed for identifier or prose changes.

## Validation record

Commands were run sequentially from the module unless noted:

```text
$ GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=45s -run '^(TestSDKJITLookupBeforeCreationDoesNotDiscoverIdentity|TestSDKJITResponseLossWithoutCommitDoesNotDiscoverIdentity|TestSDKJITResponseLossDiscoversIdentityWithoutReissuing)$' .
ok  github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset  1.332s

$ GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=45s .
ok  github.com/1XP-AI/gh-runnerd/experiments/g01-scaleset  1.283s

$ GOTOOLCHAIN=go1.26.8 go vet .
exit 0; no diagnostics

$ ./compare-sdk.sh .
github.com/actions/scaleset v0.4.0 — PASS (race enabled, 1.456s)
github.com/actions/scaleset v0.4.1-0.20260721134647-cb0405b2d874 — PASS (race enabled, 1.442s)

$ make experiments  # repository offline gate wrapper
offline experiment checks passed: 2 module(s)
```

The full offline wrapper also passed its existing G01 tagged command/livecanary
fixture and G02 module checks; it performed no real GitHub, App, runner, Docker,
Lima, Keychain, launchd or workflow operation. `make check` was inspected: its
`experiments` target is the offline wrapper above, while its separate build,
dependency/license and pinned vulnerability targets were not widened or
replayed for this focused correction.

## Limitations and rollback

This proves only that the test fixture's lookup is causally linked to its own
synthetic commit state. It does not recover a lost JIT secret, establish server
durability, prove runner ownership/current activity, provide idempotency, or
authorize deletion, drain, worker launch or live evidence. Existing recovery
behavior intentionally keeps the worker quarantined and admission paused.

Rollback must revert the entire merged PR, or reverse the complete branch diff
against the stable baseline `df0c010`, covering red `5694fd6`, green `7079b25`,
and all related naming and documentation changes. Never perform a green-only
rollback of `7079b25`: it restores the known failing assertions from `5694fd6`
while leaving the rest of the correction inconsistent. No live resource or
persistent production state was created.
