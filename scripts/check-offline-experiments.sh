#!/usr/bin/env bash

set -euo pipefail

go_cmd="${GO:-go}"
exact_toolchain="go1.26.8"
storage_regex='^TestPairedTerminal(Actual(Controller|Worker)SyncFailures|PostIntent(JournalIdentity|AuthorityBoundaries)|ClosedReplayActualFile|WorkerReceiptSurvivesControllerWriteFailure|FixtureStorageFailure)$'

# These are the two established offline gate modules. Keep this list explicit so
# a new or unreviewed experiment cannot enter public CI by directory naming.
offline_modules=(
	experiments/g01-scaleset
	experiments/g02-auth
)

# Check the complete inventory before running any suite. A deleted/moved module
# must not silently remove a gate that already exists on main.
for module_dir in "${offline_modules[@]}"; do
	if [[ ! -f "${module_dir}/go.mod" ]]; then
		printf 'offline experiment check failed: required module %s is missing\n' "${module_dir}" >&2
		exit 1
	fi
done

checked=0
for module_dir in "${offline_modules[@]}"; do
	checked=$((checked + 1))
	printf 'offline experiment: %s (toolchain=%s)\n' "${module_dir}" "${exact_toolchain}"
	(
		cd "${module_dir}"
		GOTOOLCHAIN="${exact_toolchain}" "${go_cmd}" test -race -count=1 -timeout=45s ./...
		GOTOOLCHAIN="${exact_toolchain}" "${go_cmd}" vet ./...
		if [[ "${module_dir}" == "experiments/g01-scaleset" ]]; then
			# These reviewed command tests use synthetic fixtures and refusal/plan
			# paths only. Do not discover arbitrary opt-in tags or platform probes.
			GOTOOLCHAIN="${exact_toolchain}" "${go_cmd}" test -race -count=1 -timeout=45s -tags=g01_live,g01_worker ./cmd/g01-live ./cmd/g01-worker
			GOTOOLCHAIN="${exact_toolchain}" "${go_cmd}" vet -tags=g01_live,g01_worker ./cmd/g01-live ./cmd/g01-worker
			GOTOOLCHAIN="${exact_toolchain}" "${go_cmd}" test -race -count=1 -timeout=120s -tags=g01_pair_fixture -skip '^TestPairedTerminal' ./livecanary
			GOTOOLCHAIN="${exact_toolchain}" "${go_cmd}" test -race -count=1 -timeout=120s -tags=g01_pair_fixture -run '^TestPairedTerminal' -skip "${storage_regex}" ./livecanary
			GOTOOLCHAIN="${exact_toolchain}" "${go_cmd}" test -race -count=1 -timeout=120s -tags=g01_pair_fixture -run "${storage_regex}" ./livecanary
			GOTOOLCHAIN="${exact_toolchain}" "${go_cmd}" vet -tags=g01_pair_fixture ./livecanary
		fi
	)
done

printf 'offline experiment checks passed: %s module(s)\n' "${checked}"
