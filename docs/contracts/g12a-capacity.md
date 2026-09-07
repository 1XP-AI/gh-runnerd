# G12a capacity arithmetic contract

Issue: [#40](https://github.com/1XP-AI/gh-runnerd/issues/40), an independent
leaf of G12. This package is pure, offline arithmetic for a future scheduler
caller. It does not parse configuration, read measurements, reserve capacity,
operate workers, or apply a proposal.

## API

`internal/scheduler/capacity` provides:

```go
FixedTarget(count int64) (int64, error)
AutoscaleTarget(assignedJobs, minTotal, maxTotal int64) (int64, error)
AdditionalCapacity(requested int64, budgets []Budget) (int64, error)
```

`Resources` has signed `int64` `CPUMillicores` and `MemoryBytes` fields.
`Budget` has `Limit`, `Committed`, and `PerWorker` `Resources` fields. All
quantities are inputs that must be non-negative; an error is returned for an
invalid value.

`FixedTarget` returns its count. `AutoscaleTarget` clamps assigned jobs to the
inclusive `[minTotal, maxTotal]` interval and rejects a negative input or a
minimum greater than the maximum.

`AdditionalCapacity` requires a non-empty budget list and at least one
positive per-worker cost across the complete list, including when the request
is zero. Every request and budget scalar is validated before any result is
returned. A budget whose committed value exceeds its limit makes the result
zero after validation. A budget may have zero per-worker cost in both
dimensions; it contributes no quotient bound, so a separate budget must
provide a positive per-worker cost. A zero cost in one dimension is likewise
non-limiting.

For each positive per-worker cost, the implementation computes
`floor((limit - committed) / perWorker)` and takes the minimum with the
requested count. It first establishes that committed values are within their
limits, so the subtraction cannot underflow. It never multiplies the request
by a cost, which keeps `int64` maximum boundaries safe. Inputs are values and
slices are not modified.

## Evidence

The red commit `220481b9f9b5fef6a551c0018929c430e3ac0920` ran the behavioral
tests against an explicit unimplemented package and failed with the expected
`capacity arithmetic is not implemented` errors. The green implementation is
validated by table cases for zero and bounded targets, CPU and memory limits,
mixed host/runtime budgets, exact last slots, overcommit, missing and invalid
inputs, all-zero costs, zero-cost dimensions, and `MaxInt64` values.

The tests also use an independent `math/big` feasibility and maximality oracle,
deterministic bounded property cases, and a fuzz target. They check result
bounds, request and limit monotonicity, deterministic repeated calls, and
preservation of the input slice.

## Boundary

This leaf does not provide atomic reservation or stale-calculation protection,
fairness between pools or organizations, pressure sensing, native memory
enforcement, worker lifecycle, snapshot state, or proposal application. Those
remain the responsibility of the parent G12/G05/G13 work after their stated
dependency gates pass.
