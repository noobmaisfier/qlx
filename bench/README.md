# QLX benchmarks

This module measures the cost of building and rendering SQL, without a
database. It compares QLX with the builders people commonly use, and enforces
allocation budgets in CI.

## Run

```bash
cd bench
go test -run '^$' -bench . -benchmem -count 10
```

For summarised results, pipe the output through
[`benchstat`](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat):

```bash
go test -run '^$' -bench . -benchmem -count 10 > /tmp/qlx-bench.txt
benchstat /tmp/qlx-bench.txt
```

## What is measured

The same three statements in every library:

- a filtered `SELECT` with a join, `ORDER BY` and `LIMIT`;
- an `INSERT ... RETURNING`;
- an `UPDATE ... WHERE`.

Every loop builds the statement and obtains its SQL text and argument list.
There is no connection, no network and no pgx I/O, so the numbers show the
library's own CPU and allocation cost.

| Path (SELECT) | Time | Bytes | Allocs |
| --- | ---: | ---: | ---: |
| hand-written SQL + args | 118 ns | 64 B | 2 |
| QLX static mode | 268 ns | 79 B | 2 |
| QLX normal (cache hit) | 5.80 µs | 2.81 KiB | 22 |
| QLX normal (cache miss, full render) | 11.8 µs | 5.04 KiB | 69 |
| squirrel | 28.7 µs | 5.73 KiB | 104 |
| goqu | 17.8 µs | 6.41 KiB | 189 |
| bun | 2.91 µs | 1.56 KiB | 17 |
| jet | 11.8 µs | 3.85 KiB | 84 |

| Path (INSERT / UPDATE) | Time | Bytes | Allocs |
| --- | ---: | ---: | ---: |
| QLX normal INSERT (cache miss) | 5.83 µs | 2.08 KiB | 30 |
| squirrel INSERT | 12.9 µs | 3.11 KiB | 68 |
| goqu INSERT | 9.97 µs | 3.30 KiB | 92 |
| QLX normal UPDATE (cache miss) | 3.83 µs | 1.19 KiB | 26 |
| squirrel UPDATE | 10.4 µs | 2.61 KiB | 53 |

Measured on 5 November 2026 with Go 1.27.3 on a 4-vCPU AMD Ryzen 3 3250U.
Ten runs per benchmark, summarised with `benchstat`.

### Reading the numbers

- **Static mode** is the hot path: render once, bind arguments per call. It
  costs two allocations, like the hand-written string plus argument slice.
- **Normal mode, cache hit** builds the statement, hashes its shape, finds the
  cached SQL and collects the values. It is about twice as fast as a full
  render and 3.9× faster than squirrel.
- **Normal mode, cache miss** is a full build and render on every call, with
  every QLX check. It still allocates a third less than squirrel and is about
  twice as fast. This is the cost the first call with a new shape pays.
- **Bun** is faster at building a raw SELECT, but does not check column names,
  Go types or nullability. The comparison is about construction cost, not
  feature parity.

## Budgets

`TestBudgets` turns the important claims into a failing test:

```bash
go test -run TestBudgets -v
```

- static mode allocates no more than the hand-written SQL plus one;
- normal mode, cache miss, allocates at most 1.5× squirrel;
- normal mode, cache hit, allocates no more than squirrel.

Time is not budgeted in CI because it depends on the machine. The test prints
the allocation counts; a regression fails the build.

The root module also enforces AST node sizes and allocation counts in
`budget_test.go`.

## Adding a benchmark

Add a `Benchmark<Operation>_<Library>` function to `bench_test.go`, follow the
same statement as the existing family, assign the result to `sink`, and call
`b.ReportAllocs()`. Keep the statement comparable: a benchmark that measures a
different query does not belong in the same table.

Back to the [project README](../README.md).
