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

Implementation, green checks and review remain pending in this red commit.
