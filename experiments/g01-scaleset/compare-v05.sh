#!/bin/sh
# Candidate-only offline HTTP probes. Public Go proxy/sumdb downloads may occur;
# SDK requests are restricted to each test's exact loopback fixture address.
# This is NOT a live packet runner or a shared toolchain/SDK pin migration.
set -eu
experiment_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
candidate_dir=$(mktemp -d "${TMPDIR:-/tmp}/g01-v05.XXXXXXXX")
trap 'rm -rf -- "$candidate_dir"' EXIT HUP INT TERM
cp "$experiment_dir/testdata/sdk-v05/go.mod" "$experiment_dir/testdata/sdk-v05/go.sum" \
    "$experiment_dir/client_test.go" "$experiment_dir/listener_test.go" \
    "$experiment_dir/server_test.go" "$experiment_dir/recovery.go" "$candidate_dir/"
export GOTOOLCHAIN=go1.27.3
export GOEXPERIMENT=
export GOWORK=off
cd "$candidate_dir"
go version
go mod tidy -diff
go list -m github.com/actions/scaleset
go test -mod=readonly -race -count=1 -timeout=60s ./...
go vet -mod=readonly ./...
