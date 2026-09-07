#!/usr/bin/env bash

set -euo pipefail

go_cmd="${GO:-go}"
version="${GOVULNCHECK_VERSION:-v1.7.0}"

if [[ ! "${version}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
	printf 'govulncheck version must be an exact vX.Y.Z value; got %s\n' "${version}" >&2
	exit 2
fi

printf 'govulncheck: golang.org/x/vuln/cmd/govulncheck@%s\n' "${version}"
"${go_cmd}" run "golang.org/x/vuln/cmd/govulncheck@${version}" ./...
