#!/usr/bin/env bash

set -euo pipefail

mode="${1:-check}"
go_cmd="${GO:-go}"
case "${mode}" in
	check|write) ;;
	*)
		printf 'usage: %s [check|write]\n' "$0" >&2
		exit 2
		;;
esac

goroot="$("${go_cmd}" env GOROOT)"
gofmt_bin="${goroot}/bin/gofmt"
files_file="$(mktemp "${TMPDIR:-/tmp}/gh-runnerd-gofmt.XXXXXX")"
trap 'rm -f "${files_file}"' EXIT

rg_status=0
rg --files -g '*.go' -0 > "${files_file}" || rg_status=$?
if ((rg_status != 0 && rg_status != 1)); then
	printf 'gofmt file discovery failed (rg exit %s)\n' "${rg_status}" >&2
	exit "${rg_status}"
fi

file_count=0
format_status=0
while IFS= read -r -d '' file; do
	file_count=$((file_count + 1))
	if [[ "${mode}" == "write" ]]; then
		"${gofmt_bin}" -w "${file}"
		continue
	fi

	# Keep gofmt's parse failure visible and fatal; do not infer a clean result
	# from empty stdout when the formatter itself returned an error.
	formatted="$("${gofmt_bin}" -l "${file}")"
	if [[ -n "${formatted}" ]]; then
		printf '%s\n' "${formatted}"
		format_status=1
	fi
done < "${files_file}"

if ((file_count == 0)); then
	printf 'No Go source files found; formatting %s has nothing to do.\n' "${mode}"
fi

if ((format_status != 0)); then
	printf '%s\n' 'Go sources are not formatted; run make fmt.' >&2
	exit "${format_status}"
fi
