#!/usr/bin/env bash

set -euo pipefail

go_cmd="${GO:-go}"
expected_toolchain="go1.26.8"
expected_go_directive="1.26.8"

if [[ ! -f go.mod ]]; then
	printf 'toolchain check failed: go.mod is missing\n' >&2
	exit 1
fi

go_directive="$(awk '$1 == "go" { print $2; exit }' go.mod)"
toolchain_directive="$(awk '$1 == "toolchain" { print $2; exit }' go.mod)"
if [[ "${go_directive}" != "${expected_go_directive}" ]]; then
	printf 'toolchain check failed: go.mod requires go %s, want %s\n' \
		"${go_directive:-<missing>}" "${expected_go_directive}" >&2
	exit 1
fi
if [[ -n "${toolchain_directive}" ]]; then
	printf 'toolchain check failed: go.mod has redundant toolchain directive %s; keep the exact go directive tidy\n' \
		"${toolchain_directive}" >&2
	exit 1
fi

actual_toolchain="$("${go_cmd}" version | awk '{ print $3 }')"
if [[ "${actual_toolchain}" != "${expected_toolchain}" ]]; then
	printf 'toolchain check failed: selected %s, want %s\n' \
		"${actual_toolchain:-<unknown>}" "${expected_toolchain}" >&2
	exit 1
fi

printf 'toolchain: %s\n' "${actual_toolchain}"
go_toolchain_mode="$("${go_cmd}" env GOTOOLCHAIN)"
printf 'GOTOOLCHAIN: %s\n' "${go_toolchain_mode}"
