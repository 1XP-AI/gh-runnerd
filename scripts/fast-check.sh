#!/usr/bin/env bash

set -euo pipefail

go_cmd="${GO:-go}"
exact_toolchain="go1.26.8"

# Keep this standalone entry point on the same pinned toolchain as Make and
# hosted CI. It is an iteration aid, not a replacement for the complete gate.
export GOTOOLCHAIN="${exact_toolchain}"
export GOWORK=off

fail() {
	printf 'fast check failed: %s\n' "$1" >&2
	exit 2
}

require_selector() {
	local name="$1"
	local value="$2"

	[[ -n "${value}" ]] || fail "${name} is required; specify FAST_MODULE, FAST_PACKAGE, and FAST_TEST"
	[[ "${value}" != *[[:space:]]* ]] || fail "${name} must not contain whitespace"
	[[ "${value}" != -* ]] || fail "${name} must not begin with '-'"
}

fast_module="${FAST_MODULE:-}"
fast_package="${FAST_PACKAGE:-}"
fast_test="${FAST_TEST:-}"
require_selector FAST_MODULE "${fast_module}"
require_selector FAST_PACKAGE "${fast_package}"
require_selector FAST_TEST "${fast_test}"

case "${fast_module}" in
	.) ;;
	./*) [[ "${fast_module}" != "./" ]] || fail 'FAST_MODULE must name a module directory'
		;;
	*) fail 'FAST_MODULE must be . or a relative path beginning with ./' ;;
esac

case "${fast_package}" in
	.) ;;
	./*) [[ "${fast_package}" != "./" ]] || fail 'FAST_PACKAGE must name a package pattern'
		;;
	*) fail 'FAST_PACKAGE must be . or a relative package pattern beginning with ./' ;;
esac

# Resolve the selected module and every package matched by the Go pattern
# physically before running tests. The `...` package wildcard is valid in any
# path segment; only an actual `..` path segment is refused lexically.
module_path="${fast_module#./}"
package_parts=()
IFS='/' read -r -a package_parts <<< "${fast_package}"
module_parts=()
IFS='/' read -r -a module_parts <<< "${module_path}"
for part in "${module_parts[@]}" "${package_parts[@]}"; do
	[[ "${part}" != ".." ]] || fail 'selectors must not contain a .. path segment'
done

repo_root="$(pwd -P)"
module_root="$(cd "${fast_module}" 2>/dev/null && pwd -P)" || fail "module directory does not exist: ${fast_module}"
[[ -f "${module_root}/go.mod" ]] || fail "selected module has no go.mod: ${fast_module}"
if [[ "${module_root}" != "${repo_root}" && "${module_root}" != "${repo_root}"/* ]]; then
	fail 'FAST_MODULE must resolve inside the current repository'
fi

if ! package_dirs="$(cd "${module_root}" && "${go_cmd}" list -json=false -f '{{.Dir}}' "${fast_package}")"; then
	fail "package pattern could not be resolved: ${fast_package}"
fi
package_found=0
while IFS= read -r package_dir; do
	[[ -n "${package_dir}" ]] || continue
	package_found=1
	package_root="$(cd "${package_dir}" 2>/dev/null && pwd -P)" || fail "package directory does not exist: ${fast_package}"
	if [[ "${package_root}" != "${module_root}" && "${package_root}" != "${module_root}"/* ]]; then
		fail 'FAST_PACKAGE must resolve inside selected module'
	fi
done <<< "${package_dirs}"
((package_found == 1)) || fail "FAST_PACKAGE matched no packages: ${fast_package}"

printf 'fast check: module=%s package=%s test=%s (focused selector only; not make check)\n' \
	"${fast_module}" "${fast_package}" "${fast_test}"
(
	cd "${module_root}"
	set +e
	# Keep useful inherited build flags such as -race and -mod=readonly, while
	# bounding controls that can suppress or repeat the selected test. JSON mode
	# makes per-test run events distinct from arbitrary TestMain output below.
	test_output="$("${go_cmd}" test -json -list= -bench= -fuzz= -skip= -c=false -count=1 -cpu=1 -exec= -run "${fast_test}" "${fast_package}" 2>&1)"
	status=$?
	set -e
	printf '%s\n' "${test_output}"
	if ((status != 0)); then
		printf 'fast check failed: selected test command exited %d\n' "${status}" >&2
		exit "${status}"
	fi
	matched_test=0
	while IFS= read -r line; do
		# `go test -json` emits one JSON object per line. An Action=run event
		# has a Test field; arbitrary test-process output is escaped inside an
		# Output string and cannot provide these unescaped JSON keys.
		case "${line}" in
			*'"Action":"run"'*)
				case "${line}" in
					*'"Test":"'*) matched_test=1 ;;
				esac
				;;
		esac
	done <<< "${test_output}"
	if ((matched_test == 0)); then
		printf 'fast check failed: FAST_TEST matched no compiled test in FAST_PACKAGE\n' >&2
		exit 2
	fi
)

printf 'fast check passed: selected test only; complete public validation remains make check/CI\n'
