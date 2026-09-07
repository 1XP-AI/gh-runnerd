#!/usr/bin/env bash

set -euo pipefail

go_cmd="${GO:-go}"
exact_toolchain="go1.26.8"

# These are the two reviewed offline gate modules. Keep this list explicit so
# a new or unreviewed experiment cannot enter public CI by directory naming.
offline_modules=(
	experiments/g01-scaleset
	experiments/g02-auth
)

checked=0
for module_dir in "${offline_modules[@]}"; do
	if [[ ! -f "${module_dir}/go.mod" ]]; then
		printf 'SKIPPED: %s is not present in this checkout.\n' "${module_dir}"
		continue
	fi

	checked=$((checked + 1))
	printf 'offline experiment: %s (toolchain=%s)\n' "${module_dir}" "${exact_toolchain}"
	(
		cd "${module_dir}"
		GOTOOLCHAIN="${exact_toolchain}" "${go_cmd}" test -race -count=1 -timeout=45s ./...
		GOTOOLCHAIN="${exact_toolchain}" "${go_cmd}" vet ./...
	)
done

if ((checked == 0)); then
	printf '%s\n' 'No reviewed offline experiment modules are present; no experiment test or vet claim is made.'
else
	printf 'offline experiment checks passed: %s module(s)\n' "${checked}"
fi
