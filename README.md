# verti

## Commands

```bash
# run all tests
make test

# run command package tests
make test-commands

# run acceptance matrix tests (AC1-AC10)
make test-acceptance

# run all benchmarks (quick single-iteration smoke)
make bench-smoke

# run benchmark suite with repeated samples
make bench

# run a local binary command
go run . list
go run . snapshot
go run . restore <sha>
go run . restore --orphan <id>
```

## Direct Go Commands

```bash
go test ./...

go test ./... -run '^$' \
  -bench 'BenchmarkRunDispatchListNoop|BenchmarkRunSnapshotFixture|BenchmarkPromptRestoreConfirmation' \
  -benchmem -count=10
```
