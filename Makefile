.PHONY: build test test-repo e2e-record e2e-test

BIN := build/verti

build:
	mkdir -p build
	go build -o $(BIN) ./cmd/verti

test:
	go test ./...

test-repo: build
	./scripts/test-repo.sh

e2e-record:
	./scripts/e2e-record.sh

e2e-test:
	go test ./e2e/...
