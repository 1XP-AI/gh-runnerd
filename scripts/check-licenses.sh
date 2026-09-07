#!/usr/bin/env bash

set -euo pipefail

go_cmd="${GO:-go}"
inventory="${LICENSE_INVENTORY:-docs/DEPENDENCIES.md}"

if [[ ! -f LICENSE ]]; then
	printf 'license check failed: repository LICENSE file is missing\n' >&2
	exit 1
fi
if [[ ! -f "${inventory}" ]]; then
	printf 'license check failed: inventory %s is missing\n' "${inventory}" >&2
	exit 1
fi

main_module="$("${go_cmd}" list -m -f '{{.Path}}')"
module_paths="$("${go_cmd}" list -m -f '{{.Path}}' all)"
module_count=0

while IFS= read -r module_path; do
	[[ -n "${module_path}" ]] || continue
	module_count=$((module_count + 1))
	if ! grep -F -q -- "| \`${module_path}\` |" "${inventory}"; then
		printf 'license check failed: %s is absent from %s\n' "${module_path}" "${inventory}" >&2
		exit 1
	fi

	if [[ "${module_path}" == "${main_module}" ]]; then
		module_dir="."
	else
		module_dir="$("${go_cmd}" list -m -f '{{.Dir}}' "${module_path}")"
	fi
	license_file=""
	for candidate in LICENSE LICENSE.txt LICENSE.md COPYING COPYING.txt NOTICE NOTICE.txt COPYRIGHT UNLICENSE; do
		if [[ -f "${module_dir}/${candidate}" ]]; then
			license_file="${candidate}"
			break
		fi
	done
	if [[ -z "${license_file}" ]]; then
		printf 'license check failed: no top-level license file found for %s\n' "${module_path}" >&2
		exit 1
	fi
	printf 'license: %s (%s)\n' "${module_path}" "${license_file}"
done <<< "${module_paths}"

printf 'license inventory verified: %s module(s)\n' "${module_count}"
