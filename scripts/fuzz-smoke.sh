#!/usr/bin/env bash

set -euo pipefail

go_cmd="${GO:-go}"
fuzz_time="${FUZZTIME:-1s}"
summary_file="${GITHUB_STEP_SUMMARY:-}"
declare -a targets=()

write_summary() {
	local detail="$1"
	if [[ -n "${summary_file}" ]]; then
		{
			printf '%s\n' '### Fuzz smoke'
			printf -- '- %s\n' "${detail}"
		} >> "${summary_file}"
	fi
}

# Query compiled tests, so build-excluded sources and similarly named helpers
# cannot be reported as executed fuzz targets. Both package and test discovery
# run in checked substitutions; a loading/compilation failure is fatal.
package_paths="$("${go_cmd}" list -f '{{.ImportPath}}' ./...)"
while IFS= read -r package_path; do
	[[ -n "${package_path}" ]] || continue
	compiled_tests="$("${go_cmd}" test -list '^Fuzz' "${package_path}")"
	while IFS= read -r fuzz_name; do
		if [[ "${fuzz_name}" =~ ^Fuzz[A-Za-z0-9_]+$ ]]; then
			targets+=("${package_path}"$'\t'"${fuzz_name}")
		fi
	done <<< "${compiled_tests}"
done <<< "${package_paths}"

if ((${#targets[@]} == 0)); then
	detail='SKIPPED: no Go fuzz targets are present; this bootstrap does not claim fuzz coverage.'
	printf '%s\n' "${detail}"
	write_summary "${detail}"
	exit 0
fi

for target in "${targets[@]}"; do
	IFS=$'\t' read -r package_path fuzz_name <<< "${target}"
	printf 'fuzz smoke: %s %s (fuzztime=%s)\n' "${package_path}" "${fuzz_name}" "${fuzz_time}"
	"${go_cmd}" test -run '^$' -fuzz "^${fuzz_name}$" \
		-fuzztime="${fuzz_time}" "${package_path}"
done

detail="RAN: ${#targets[@]} Go fuzz target(s) with fuzztime=${fuzz_time}."
printf '%s\n' "${detail}"
write_summary "${detail}"
