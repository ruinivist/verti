.PHONY: build test test-repo

BIN := build/verti

build:
	mkdir -p build
	go build -o $(BIN) ./cmd/verti

test:
	go test ./...

test-repo: build
	./scripts/test-repo.sh
