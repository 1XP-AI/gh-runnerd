SHELL := /usr/bin/env bash

GO ?= go
GOFLAGS ?=
# A go directive is a minimum under auto, including on a newer installation.
override GOTOOLCHAIN := go1.27.3
export GOTOOLCHAIN
export GOFLAGS
FAST_MODULE := $(value FAST_MODULE)
FAST_PACKAGE := $(value FAST_PACKAGE)
FAST_TEST := $(value FAST_TEST)
export FAST_MODULE FAST_PACKAGE FAST_TEST
FUZZTIME ?= 1s
GOVULNCHECK_VERSION ?= v1.7.0

.PHONY: help build check fast toolchain fmt fmt-check vet test test-race test-core test-fixtures test-race-core test-race-fixtures fuzz-smoke deps licenses vuln experiments

help:
	@printf '%s\n' \
		'gh-runnerd development commands:' \
		'  make fmt          format Go sources in place' \
		'  make check        run the complete public validation suite' \
		'  make fast         run one explicit FAST_MODULE/FAST_PACKAGE/FAST_TEST selector (not the full gate)' \
		'  make build        compile all packages' \
		'  make experiments  test reviewed offline gate modules when present' \
		'  make vuln         run pinned govulncheck (network access may be needed)'

build:
	"$(GO)" build -o /dev/null ./cmd/gh-runnerd

check: toolchain fmt-check build vet test test-race fuzz-smoke deps licenses experiments vuln

fast:
	GO="$(GO)" bash scripts/fast-check.sh

toolchain:
	GO="$(GO)" bash scripts/check-toolchain.sh

fmt:
	GO="$(GO)" bash scripts/gofmt.sh write

fmt-check:
	GO="$(GO)" bash scripts/gofmt.sh check

vet:
	"$(GO)" vet ./...

# Reuse recipes, rather than prerequisites: complete local gates keep all three
# invocations serial and fail fast even when Make is invoked with parallel jobs.
define root-test-core
	"$(GO)" test $(1) -count=1 -skip '^Test(ToolingTaggedPairFixturePartitionsRun$$|G02OwnedPrepDir)' ./...
endef

define root-test-fixtures
	"$(GO)" test $(1) -count=1 -run '^TestG02OwnedPrepDir' ./scripts
	"$(GO)" test $(1) -count=1 -run '^TestToolingTaggedPairFixturePartitionsRun$$' ./scripts
endef

test:
	$(call root-test-core,)
	$(call root-test-fixtures,)

test-race:
	$(call root-test-core,-race)
	$(call root-test-fixtures,-race)

test-core:
	$(call root-test-core,)

test-fixtures:
	$(call root-test-fixtures,)

test-race-core:
	$(call root-test-core,-race)

test-race-fixtures:
	$(call root-test-fixtures,-race)

fuzz-smoke:
	GO="$(GO)" FUZZTIME="$(FUZZTIME)" bash scripts/fuzz-smoke.sh

deps:
	"$(GO)" mod tidy -diff
	"$(GO)" mod verify
	"$(GO)" list -mod=readonly -deps ./... >/dev/null

licenses:
	GO="$(GO)" bash scripts/check-licenses.sh

experiments:
	GO="$(GO)" bash scripts/check-offline-experiments.sh

vuln:
	GO="$(GO)" GOVULNCHECK_VERSION="$(GOVULNCHECK_VERSION)" bash scripts/govulncheck.sh
