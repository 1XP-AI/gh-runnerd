SHELL := /usr/bin/env bash

GO ?= go
GOFLAGS ?=
# A go directive is a minimum under auto, including on a newer installation.
override GOTOOLCHAIN := go1.26.8
export GOTOOLCHAIN
export GOFLAGS
export FAST_MODULE FAST_PACKAGE FAST_TEST
FUZZTIME ?= 1s
GOVULNCHECK_VERSION ?= v1.7.0

.PHONY: help build check fast toolchain fmt fmt-check vet test test-race fuzz-smoke deps licenses vuln experiments

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

test:
	"$(GO)" test -count=1 ./...

test-race:
	"$(GO)" test -race -count=1 ./...

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
