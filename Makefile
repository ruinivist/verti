.PHONY: build test-repo

build:
	go build -o verti .

test-repo:
	./scripts/test-repo.sh
