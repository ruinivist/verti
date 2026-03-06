# verti

## Commands

```bash
# build the binary
make build

# run all tests
make test

# run command package tests
make test-commands

# run acceptance-focused tests
make test-acceptance

# run all benchmarks (quick single-iteration smoke)
make bench-smoke

# run benchmark suite with repeated samples
make bench

# run a local binary command
go run . list
go run . sync
go run . sync --debounced
```

## Direct Go Commands

```bash
go test ./...

go test ./... -run '^$' \
  -bench 'BenchmarkRunDispatchListNoop' \
  -benchmem -count=10
```
