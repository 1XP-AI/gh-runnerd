#!/usr/bin/env bash

set -euo pipefail

go_cmd="${GO:-go}"
fuzz_time="${FUZZTIME:-1s}"
summary_file="${GITHUB_STEP_SUMMARY:-}"
test_files_file="$(mktemp "${TMPDIR:-/tmp}/gh-runnerd-fuzz-files.XXXXXX")"
trap 'rm -f "${test_files_file}"' EXIT
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

# Keep the target discovery independent of package names and internal layout.
# A target is a conventional func FuzzXxx(*testing.F), as required by go test.
# Run go list as a normal command substitution so a package-loading failure is
# fatal; a process substitution would otherwise hide its exit status.
package_paths="$("${go_cmd}" list -f '{{.ImportPath}}' ./...)"
while IFS= read -r package_path; do
	[[ -n "${package_path}" ]] || continue
	package_dir="$("${go_cmd}" list -f '{{.Dir}}' "${package_path}")"
	file_status=0
	rg --files --max-depth 1 -g '*_test.go' -0 "${package_dir}" > "${test_files_file}" || file_status=$?
	if ((file_status != 0 && file_status != 1)); then
		printf 'fuzz file discovery failed for %s (rg exit %s)\n' "${package_dir}" "${file_status}" >&2
		exit "${file_status}"
	fi
	while IFS= read -r -d '' test_file; do
		[[ -n "${test_file}" ]] || continue
		while IFS= read -r fuzz_name; do
			[[ -n "${fuzz_name}" ]] || continue
			targets+=("${package_path}"$'\t'"${fuzz_name}")
		done < <(
			rg --no-heading --no-filename --only-matching \
				'^func[[:space:]]+Fuzz[A-Za-z0-9_]+' "${test_file}" \
			| sed -E 's/^func[[:space:]]+//'
		)
	done < "${test_files_file}"
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
