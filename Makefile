.PHONY: build test-repo

BIN := build/verti

build:
	mkdir -p build
	go build -o $(BIN) .

test-repo: build
	./scripts/test-repo.sh
