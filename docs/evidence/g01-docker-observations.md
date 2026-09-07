# G01 exact Docker observations — issue #44

Bounded child of [G01](https://github.com/1XP-AI/gh-runnerd/issues/1), tracked in
[issue #44](https://github.com/1XP-AI/gh-runnerd/issues/44). This does not close the
live gate or authorize a live experiment.

## Red evidence

On baseline `bb4a8fee0025e7f3c6ac7974cdb416bafde7bf78`, ran from the G01 module:

```text
GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=45s ./liveworker -run '^TestUnixInspectRequiresStateFlagsBeforeMutation$'
```

The actual production Unix HTTP adapter received a complete valid container
profile with exactly one state flag omitted or null. All 24 deficient cases
incorrectly reached one start/delete request: four flags, missing/null, and
start-created/cleanup-created/cleanup-exited. All 12 complete-state controls
succeeded. The test exited 1, demonstrating behavior rather than a missing API.

Fixtures used private synthetic Unix sockets only; no real Docker endpoint,
account admission root, image, GitHub live API or credential was used.

## Reference and policy scope

[Docker Engine API v1.45](https://docs.docker.com/reference/api/engine/version/v1.45/)
defines nullable container state and its status/boolean/integer fields. Requiring
known status and present Running, Paused, Restarting and Dead before a legacy
worker decision is this experiment's policy, not a claim that Docker requires
every field. The existing created/exited cleanup policy does not require exit 0.

## Implementation and green evidence

`Docker.InspectExact` now returns the exact full target ID, GET path, received
HTTP status, outcome and presence-aware state. Its raw `Container` is excluded
from JSON serialization and retained for the existing full profile verifier.
Unknown status is normalized. A supported 404 requires the bounded v1.45 error
object with a non-null string message; the message is discarded, and the result
is only `not-found-reported`. NewDocker does not remember successful daemon
preflight, so this fact proves neither daemon verification nor ownership.

The decoder rejects exact duplicate object keys everywhere and folded aliases
of fields decoded into structs, including Unicode aliases and escaped duplicates.
Config, HostConfig, Labels and other maps retain case-sensitive keys. Bodies
remain capped at the existing response limit; JSON nesting is additionally
limited to 64 levels. Unknown or ill-typed decision fields, contradictory IDs,
unsupported responses, cancellation and socket replacement cannot authorize a
subsequent standalone start/delete.

`Docker.Inspect` delegates to the exact read and requires known status and the
four present booleans before returning legacy zero-valued fields. Signed exit
values are retained; absent/null/nonzero exits do not change legacy cleanup.
Runtime, Container, Driver.Run, journal schemas and non-force deletion parameters
are unchanged. This contains no paired controller API or live command change.

Passed with the pinned toolchain, from the G01 module:

```text
GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=45s ./liveworker -run '^TestUnixInspectRequiresStateFlagsBeforeMutation$'
GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=45s ./liveworker
GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=45s ./liveworker -run '^TestDockerInspectExactRejectsMalformedOrAmbiguousBodiesBeforeMutation$'
GOTOOLCHAIN=go1.26.8 go vet ./liveworker
```

The last focused run includes the additional escaped-duplicate and Unicode State
alias cases added after the full worker run. `git diff --check` also passed.
The independent Astra xhigh reviewer reproduced the red failure at `1ae7d12`.

## Integration review correction

Merged current main `fa3e1919321f597bf3d792d57be5301fd06f54d3` into this branch;
the combined review head was `1097854ca926a106dfb50a982cdf23cc4ec6e87d`.
Integrator review identified that adding a context check in the shared response
reader could discard a complete successful mutation response if cancellation
arrived at its body's EOF. A wrapper around the actual Unix transport makes that
boundary deterministic, without replacing the request/client behavior.

With the new regression tests and the pre-fix implementation, this command
exited 1: create, start and cleanup each lost a fully received result. The exact
observation cancellation control passed.

```text
GOTOOLCHAIN=go1.26.8 go test -race -count=1 -timeout=45s ./liveworker -run '^(TestDockerCompletedMutationResponseSurvivesEOFCancellation|TestDockerInspectExactEOFCancellationKeepsUnknownOutcome)$'
```

Removed the added cancellation check from the shared reader, preserving legacy
mutation-result handling; InspectExact retains its own final cancellation guard.
The complete worker race suite (including these regressions and all added alias
cases), worker vet and `git diff --check` then passed on the combined tree.

Final-head independent review, repository-wide checks, hosted CI and GitHub Codex
review remain pending. No real Docker/GitHub live behavior, paired execution,
account admission or daemon-verified absence was tested.
