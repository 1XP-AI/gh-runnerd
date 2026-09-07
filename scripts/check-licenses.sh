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

# The Runtime modules table is the exact selected graph, including replacement
# identity. Other tables describe the toolchain/actions and are not module rows.
inventory_records="$(awk -F '|' '
	function field(value) {
		gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)
		sub(/^`/, "", value); sub(/`$/, "", value)
		return value
	}
	/^## / { runtime = ($0 == "## Runtime modules"); next }
	runtime && /^\|[[:space:]]*`/ {
		if (NF != 7) exit 1
		print field($2) "|" field($3) "|" field($4)
	}
' "${inventory}" | LC_ALL=C sort)"
module_records="$("${go_cmd}" list -m -f '{{.Path}}|{{if .Main}}local{{else}}{{.Version}}{{end}}|{{with .Replace}}{{.Path}}{{if .Version}}@{{.Version}}{{end}}{{else}}none{{end}}|{{.Dir}}' all)"
expected_records="$(printf '%s\n' "${module_records}" | awk -F '|' '
	NF != 4 || $1 == "" || $2 == "" || $3 == "" { exit 1 }
	{ print $1 "|" $2 "|" $3 }
' | LC_ALL=C sort)"
if [[ -z "${expected_records}" || "${inventory_records}" != "${expected_records}" ]]; then
	printf 'license check failed: runtime module/version/replacement inventory differs from the selected graph\n' >&2
	exit 1
fi
module_count=0

while IFS='|' read -r module_path module_version replacement module_dir; do
	[[ -n "${module_path}" ]] || continue
	module_count=$((module_count + 1))
	if [[ -z "${module_dir}" || ! -d "${module_dir}" ]]; then
		printf 'license check failed: selected source directory for %s is unavailable\n' "${module_path}" >&2
		exit 1
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
	printf 'license: %s %s (%s)\n' "${module_path}" "${module_version}" "${license_file}"
done <<< "${module_records}"

printf 'license inventory verified: %s module(s)\n' "${module_count}"
