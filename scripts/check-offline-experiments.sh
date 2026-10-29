#!/usr/bin/env bash

set -euo pipefail

go_cmd="${GO:-go}"
exact_toolchain="go1.27.3"
default_heavy_test_regex='^TestBaselineStatisticsPresenceAndEligibility$'
default_handoff_test_regex='^TestControllerHandoff'
default_heavy_and_handoff_skip_regex='^Test(BaselineStatisticsPresenceAndEligibility|ControllerHandoff.*)$'
paired_collection_regex='^TestPaired'
replacement_regex='^TestSessionReplacement'
acquire_loss_command_regex='^TestExperimentalAcquireLossCommand$'
drain_command_regex='^TestExperimentalDrainCommand$'
jit_boundary_regex='^TestJITBoundary'
jit_boundary_storage_regex='^TestJITBoundaryStorageAndCancellationEveryAppend$'
jit_boundary_expiry_regex='^TestJITBoundaryExpiryAtEveryAppend$'
jit_boundary_admission_regex='^TestJITBoundary(PreparationRequiresOriginalFreshCreate|AtomicPrefixBeforeCredentialInput|ConcurrentConsumesOnce|HistoryCannotResumeOrRenew|StrictJournalSchemaAndRawReferences|ProofCannotCrossOpen|DirectRunAndCredentialShapes|CloneOwnsNestedPayload)$'
jit_boundary_physical_regex='^TestJITBoundaryPhysicalMutationsStop$'
jit_boundary_heavy_regex='^TestJITBoundary(StorageAndCancellationEveryAppend|ExpiryAtEveryAppend|PreparationRequiresOriginalFreshCreate|AtomicPrefixBeforeCredentialInput|ConcurrentConsumesOnce|HistoryCannotResumeOrRenew|StrictJournalSchemaAndRawReferences|ProofCannotCrossOpen|DirectRunAndCredentialShapes|CloneOwnsNestedPayload|PhysicalMutationsStop)$'
non_paired_skip_regex='^Test(Paired|SessionReplacement|JITBoundary|ExperimentalAcquireLossCommand$|ExperimentalDrainCommand$)'
storage_regex='^TestPairedTerminal(Actual(Controller|Worker)SyncFailures|PostIntent(JournalIdentity|AuthorityBoundaries)|ClosedReplayActualFile|WorkerReceiptSurvivesControllerWriteFailure|FixtureStorageFailure)$'
storage_journal_regex='^TestPairedTerminalPostIntentJournalIdentity$'
terminal_heavy_regex='^TestPairedTerminal(FinalResultCapacity|PendingChildCapacity|EligibilityUsesFreshExactFacts|CapturedAcknowledgementCancellation|MissingAcknowledgementsAndPostchecks)$'
terminal_remainder_skip_regex='^TestPairedTerminal(FinalResultCapacity|PendingChildCapacity|EligibilityUsesFreshExactFacts|CapturedAcknowledgementCancellation|MissingAcknowledgementsAndPostchecks|Actual(Controller|Worker)SyncFailures|PostIntent(JournalIdentity|AuthorityBoundaries)|ClosedReplayActualFile|WorkerReceiptSurvivesControllerWriteFailure|FixtureStorageFailure)$'
real_pair_cadence_regex='^TestPairedBrokerRealCadenceChildExceedsThirtySeconds$'
paired_prep_regex='^TestPairedBrokerPrepareReviewedG01LiveBinary$'
paired_prep_or_cadence_regex='^TestPairedBroker(PrepareReviewedG01LiveBinary|RealCadenceChildExceedsThirtySeconds)$'
paired_broker_regex='^TestPaired'

# These are the established offline gate modules. Keep this list explicit so
# a new or unreviewed experiment cannot enter public CI by directory naming.
offline_modules=(
	experiments/g01-scaleset
	experiments/g02-auth
	experiments/r1-credentials
)
selected_modules=("${offline_modules[@]}")

if [[ -n "${OFFLINE_EXPERIMENT_MODULE:-}" ]]; then
	case "${OFFLINE_EXPERIMENT_MODULE}" in
		experiments/g01-scaleset|experiments/g02-auth|experiments/r1-credentials)
			selected_modules=("${OFFLINE_EXPERIMENT_MODULE}")
			;;
		*)
			printf 'offline experiment check failed: unsupported module %s\n' \
				"${OFFLINE_EXPERIMENT_MODULE}" >&2
			exit 1
			;;
	esac
fi

# Profiles are finite reviewed G01 partitions, never arbitrary test selectors.
profile="${OFFLINE_EXPERIMENT_PROFILE:-all}"
case "${profile}" in
	all) ;;
	core|jit)
		if [[ "${OFFLINE_EXPERIMENT_MODULE:-}" != "experiments/g01-scaleset" ]]; then
			printf 'offline experiment check failed: profile %s requires explicit G01 module\n' "${profile}" >&2
			exit 1
		fi
		;;
	*)
		printf 'offline experiment check failed: unsupported profile %s\n' "${profile}" >&2
		exit 1
		;;
esac

# Check the complete inventory before running any suite. A deleted/moved module
# must not silently remove a gate that already exists on main.
for module_dir in "${offline_modules[@]}"; do
	if [[ ! -f "${module_dir}/go.mod" ]]; then
		printf 'offline experiment check failed: required module %s is missing\n' "${module_dir}" >&2
		exit 1
	fi
done

checked=0
for module_dir in "${selected_modules[@]}"; do
	checked=$((checked + 1))
	printf 'offline experiment: %s (toolchain=%s)\n' "${module_dir}" "${exact_toolchain}"
	(
		cd "${module_dir}"
		if [[ "${module_dir}" == "experiments/g01-scaleset" ]]; then
			if [[ "${profile}" != jit ]]; then
				# Keep the baseline-statistics and expanded controller-handoff families
				# in separate bounded processes while retaining ./... discovery.
				GOTOOLCHAIN="${exact_toolchain}" "${go_cmd}" test -race -count=1 -timeout=45s -run "${default_heavy_test_regex}" ./...
				GOTOOLCHAIN="${exact_toolchain}" "${go_cmd}" test -race -count=1 -timeout=45s -run "${default_handoff_test_regex}" ./...
				# Keep the remainder unfiltered so examples and fuzz seeds execute;
				# skip exactly the disjoint union of the two named partitions.
				GOTOOLCHAIN="${exact_toolchain}" "${go_cmd}" test -race -count=1 -timeout=45s -skip "${default_heavy_and_handoff_skip_regex}" ./...
			fi
		elif [[ "${module_dir}" == "experiments/g02-auth" ]]; then
			# Bounded fixture clone/build is a separate 45-second process so the
			# cadence partition keeps the production seven-times-five-second wait
			# without hiding preparation in an unbounded helper. Then isolate the
			# exact cadence name, the remaining TestPaired family, and an
			# unfiltered complement so Example Output and fuzz seeds still run.
			# Package discovery stays on ./... in every command.
			g02_prep_dir=$(mktemp -d)
			# Remove only this owned mktemp path. EXIT covers success and set -e
			# failure without changing status. INT/TERM/HUP are not EXIT on
			# Bash 5, so trap them explicitly, disarm EXIT, and keep 130/143/129.
			trap 'rm -rf -- "${g02_prep_dir}"' EXIT
			trap 'trap - EXIT; rm -rf -- "${g02_prep_dir}"; exit 130' INT
			trap 'trap - EXIT; rm -rf -- "${g02_prep_dir}"; exit 143' TERM
			trap 'trap - EXIT; rm -rf -- "${g02_prep_dir}"; exit 129' HUP
			chmod 0700 "${g02_prep_dir}"
			export G01_PAIR_BRIDGE_PREP_DIR="${g02_prep_dir}"
			GOTOOLCHAIN="${exact_toolchain}" "${go_cmd}" test -race -count=1 -timeout=45s -run "${paired_prep_regex}" ./...
			GOTOOLCHAIN="${exact_toolchain}" "${go_cmd}" test -race -count=1 -timeout=45s -run "${real_pair_cadence_regex}" ./...
			GOTOOLCHAIN="${exact_toolchain}" "${go_cmd}" test -race -count=1 -timeout=45s -run "${paired_broker_regex}" -skip "${paired_prep_or_cadence_regex}" ./...
			GOTOOLCHAIN="${exact_toolchain}" "${go_cmd}" test -race -count=1 -timeout=45s -skip "${paired_broker_regex}" ./...
			unset G01_PAIR_BRIDGE_PREP_DIR
		else
			# R1 credentials is a standalone fixture-backed module. Keep its
			# package discovery and race run explicit so nested-module coverage is
			# part of the same public offline gate without enabling live effects.
			GOTOOLCHAIN="${exact_toolchain}" "${go_cmd}" test -race -count=1 -timeout=45s ./...
		fi
		if [[ "${profile}" != jit ]]; then
			GOTOOLCHAIN="${exact_toolchain}" "${go_cmd}" vet ./...
		fi
		if [[ "${module_dir}" == "experiments/g01-scaleset" ]]; then
			if [[ "${profile}" != jit ]]; then
				# These reviewed command tests use synthetic fixtures and refusal/plan
				# paths only. Do not discover arbitrary opt-in tags or platform probes.
				GOTOOLCHAIN="${exact_toolchain}" "${go_cmd}" test -race -count=1 -timeout=45s -tags=g01_live,g01_worker ./cmd/g01-live ./cmd/g01-worker
				GOTOOLCHAIN="${exact_toolchain}" "${go_cmd}" vet -tags=g01_live,g01_worker ./cmd/g01-live ./cmd/g01-worker
			fi
			# Keep the non-terminal collection/listener tests exhaustive and disjoint:
			# paired collection, replacement, JIT boundary, then the non-paired remainder. Terminal coverage
			# is four complementary 120-second partitions: the five heavy names,
			# the remaining TestPairedTerminal tests, storage journal identity,
			# and the other storage names. Keep the storage union unchanged.
			# Each job prepares its own current-source synthetic TEST commands,
			# serially, for its existing fixture partitions. No artifact transfer.
			g01_phase_dir=$(mktemp -d)
			trap 'rm -rf -- "${g01_phase_dir}"' EXIT
			trap 'trap - EXIT; rm -rf -- "${g01_phase_dir}"; exit 130' INT
			trap 'trap - EXIT; rm -rf -- "${g01_phase_dir}"; exit 143' TERM
			trap 'trap - EXIT; rm -rf -- "${g01_phase_dir}"; exit 129' HUP
			chmod 0700 "${g01_phase_dir}"
			export G01_PHASE_HELPER_DIR="${g01_phase_dir}"
			export G01_PHASE_GO="${go_cmd}"
			export G01_PHASE_CONTROLLER_TEST_BINARY="${g01_phase_dir}/controller.test"
			export G01_PHASE_BROKER_TEST_BINARY="${g01_phase_dir}/broker.test"
			GOTOOLCHAIN="${exact_toolchain}" "${go_cmd}" test -race -count=1 -timeout=45s -tags=g01_pair_fixture -run '^TestExperimentalPrepareCommandHelpers$' ./livecanary
			if [[ "${profile}" != jit ]]; then
				GOTOOLCHAIN="${exact_toolchain}" "${go_cmd}" test -race -count=1 -timeout=120s -tags=g01_pair_fixture -run "${paired_collection_regex}" -skip '^TestPairedTerminal' ./livecanary
				# Keep the new storage/expiry-heavy family out of the already bounded
				# non-paired remainder; the finite family selectors remain disjoint.
				GOTOOLCHAIN="${exact_toolchain}" "${go_cmd}" test -race -count=1 -timeout=120s -tags=g01_pair_fixture -run "${replacement_regex}" ./livecanary
				GOTOOLCHAIN="${exact_toolchain}" "${go_cmd}" test -race -count=1 -timeout=120s -tags=g01_pair_fixture -run "${acquire_loss_command_regex}" ./livecanary
				GOTOOLCHAIN="${exact_toolchain}" "${go_cmd}" test -race -count=1 -timeout=120s -tags=g01_pair_fixture -run "${drain_command_regex}" ./livecanary
			fi
			if [[ "${profile}" != core ]]; then
				GOTOOLCHAIN="${exact_toolchain}" "${go_cmd}" test -race -count=1 -timeout=120s -tags=g01_pair_fixture -run "${jit_boundary_storage_regex}" ./livecanary
				GOTOOLCHAIN="${exact_toolchain}" "${go_cmd}" test -race -count=1 -timeout=120s -tags=g01_pair_fixture -run "${jit_boundary_expiry_regex}" ./livecanary
				GOTOOLCHAIN="${exact_toolchain}" "${go_cmd}" test -race -count=1 -timeout=120s -tags=g01_pair_fixture -run "${jit_boundary_admission_regex}" ./livecanary
				GOTOOLCHAIN="${exact_toolchain}" "${go_cmd}" test -race -count=1 -timeout=120s -tags=g01_pair_fixture -run "${jit_boundary_physical_regex}" ./livecanary
				GOTOOLCHAIN="${exact_toolchain}" "${go_cmd}" test -race -count=1 -timeout=120s -tags=g01_pair_fixture -run "${jit_boundary_regex}" -skip "${jit_boundary_heavy_regex}" ./livecanary
			fi
			if [[ "${profile}" != jit ]]; then
				GOTOOLCHAIN="${exact_toolchain}" "${go_cmd}" test -race -count=1 -timeout=120s -tags=g01_pair_fixture -skip "${non_paired_skip_regex}" ./livecanary
				GOTOOLCHAIN="${exact_toolchain}" "${go_cmd}" test -race -count=1 -timeout=120s -tags=g01_pair_fixture -run "${terminal_heavy_regex}" ./livecanary
				GOTOOLCHAIN="${exact_toolchain}" "${go_cmd}" test -race -count=1 -timeout=120s -tags=g01_pair_fixture -run '^TestPairedTerminal' -skip "${terminal_remainder_skip_regex}" ./livecanary
				GOTOOLCHAIN="${exact_toolchain}" "${go_cmd}" test -race -count=1 -timeout=120s -tags=g01_pair_fixture -run "${storage_journal_regex}" ./livecanary
				GOTOOLCHAIN="${exact_toolchain}" "${go_cmd}" test -race -count=1 -timeout=120s -tags=g01_pair_fixture -run "${storage_regex}" -skip "${storage_journal_regex}" ./livecanary
				GOTOOLCHAIN="${exact_toolchain}" "${go_cmd}" vet -tags=g01_pair_fixture ./livecanary
			fi
		fi
	)
done

printf 'offline experiment checks passed: %s module(s)\n' "${checked}"
