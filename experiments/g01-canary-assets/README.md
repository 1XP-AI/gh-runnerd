# Inactive G01 workload assets

This directory supplies an auditable workload for the private live experiment.
It does not install or dispatch a workflow, create a runner group, launch a
worker, or authorize any live operation. G01 remains open until actual approved
server/runtime evidence is collected.

Before use, replace the three template markers with the exact dedicated group,
scale-set name and experiment nonce from the approved controller manifest.
They must be fixed non-secret identifiers consisting only of letters, digits
and hyphens. Place the rendered file at `.github/workflows/canary.yml` in the
approved **private** repository, review its immutable commit, and bind that
commit and the actual run ID in the controller manifest. The group must allow
only that private repository. Do not change labels through dispatch inputs.

The workflow has only manual dispatch, one job, no repository checkout or
third-party action, no secrets, no service containers, and no token grants.
Each dispatch emits 60 harmless heartbeat lines and a completion marker. There
is no workflow cancellation or rerun policy. The ten-minute job timeout is an
upper bound on one disposable job, not permission to terminate existing work.
Run phases serially with at most the approved two queued jobs and one worker.

The bootstrap check requires a non-root Linux ARM64 worker with the official
runner's `/home/runner` layout. It reports only presence-check success: no JIT
input variable in the job, no named controller credential variables, and an
existing runner credential file. These checks do **not** prove that every
possible secret channel is absent, erase the original JIT environment, or
establish hostile-workload isolation. Controller credentials must never be
passed to the worker. Its private filesystem and container metadata still
require protected access and owned cleanup.

An executor and its cleanup/secret-transport path require independent review
before this workload is run. In particular, the official runner image contains
a Docker client and passwordless sudo: do not mount a host Docker socket,
controller home, credentials or other host paths. A later executor must pin an
ARM64 image digest, enforce the approved CPU/memory/process bounds, suppress raw
runner diagnostics, and retain unknown/busy resources. The controller-only
canary driver does not implement that executor.

Sources: [runner image source at v2.337.0](https://github.com/actions/runner/blob/v2.337.0/images/Dockerfile)
and [official runner image](https://github.com/actions/runner/pkgs/container/actions-runner).

## Registry metadata observed on 2026-09-07

Read-only GHCR manifest requests for `ghcr.io/actions/actions-runner:2.337.0`
returned index digest
`sha256:e5496277be5d09bc968b3d64911b74e219ac4a3f2edce956a3ecf9271bea1ef4`
and Linux ARM64 manifest digest
`sha256:f5a0d9a3d857315f2aed7075a02a29f46927ad198221c3b1c66585ae9fe36c0d`.
A future reviewed executor can bind that immutable ARM64 reference. No image
layers were downloaded or executed by this metadata check. The tag's source
Dockerfile and registry metadata are not a claim that runtime contents or the
runner binary digest have been measured; verify the pinned runtime before live
admission and record that evidence separately.

The template's two embedded Bash scripts passed syntax checks. Static inspection
confirmed it has only manual dispatch, no external action/checkout, fixed runner
identifiers and no workflow-input interpolation into shell commands. It has not
been installed or executed on GitHub.
