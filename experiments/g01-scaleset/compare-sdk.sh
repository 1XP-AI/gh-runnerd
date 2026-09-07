#!/bin/sh
# Same offline suite against immutable SDK selections. Requires downloaded Go
# modules/toolchain; dependency resolution may access public Go proxy/sumdb.
set -eu
experiment_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
comparison_dir=$(mktemp -d "${TMPDIR:-/tmp}/g01-compare.XXXXXXXX")
trap 'rm -rf -- "$comparison_dir"' EXIT HUP INT TERM
export GOTOOLCHAIN=go1.26.8
for sdk_version in v0.4.0 v0.4.1-0.20260721134647-cb0405b2d874; do
    mkdir "$comparison_dir/$sdk_version"
    cp "$experiment_dir/go.mod" "$experiment_dir/go.sum" "$experiment_dir/"*.go "$comparison_dir/$sdk_version/"
    (
        cd "$comparison_dir/$sdk_version"
        go mod edit -require="github.com/actions/scaleset@$sdk_version"
        go mod tidy
        go list -m github.com/actions/scaleset
        go test -race -count=1 -timeout=45s ./...
        go vet ./...
    )
done
