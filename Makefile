SHELL := /usr/bin/env bash

BENCH_REGEX = BenchmarkRunDispatchListNoop|BenchmarkRunSnapshotFixture|BenchmarkPromptRestoreConfirmation

.PHONY: build test test-commands test-acceptance bench-smoke bench

build:
	mkdir -p bin
	go build -o bin/verti .

test:
	go test ./...

test-commands:
	go test ./internal/commands

test-acceptance:
	go test ./internal/commands -run TestMVPAcceptanceMatrixAC1ToAC9
	go test ./internal/snapshots -run TestMVPAcceptanceCriterion10NoPartialSnapshotVisibleOnPublishFailure

bench-smoke:
	go test ./... -run '^$$' -bench '$(BENCH_REGEX)' -benchmem -benchtime=1x

bench:
	go test ./... -run '^$$' -bench '$(BENCH_REGEX)' -benchmem -count=10
