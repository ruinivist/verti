.PHONY: build test test-repo e2e-visual

BIN := build/verti

build:
	mkdir -p build
	go build -o $(BIN) ./cmd/verti

test:
	go test ./...

test-repo: build
	./scripts/test-repo.sh

e2e-visual:
	./scripts/e2e-visual.sh
