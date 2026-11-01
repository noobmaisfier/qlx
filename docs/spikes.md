# Phase 0 spikes

| Spike | Question | Outcome in this repository | How to re-run |
|---|---|---|---|
| S1 | How is "nullable if any operand is nullable" encoded? | Encoding C (derived expressions typed nullable; every function has a plain and an `N` overload) ships. A and B are not adopted. The three-line error budget is enforced by the compile-fail corpus (`compilefail_test.go`) | `go test -run TestCompileFailCorpus .` |
| S2 | Closure or global scope API? | Globals with aliases (`Users.As("u")`); scope errors are `ErrScope` at render time and V006 in the analyzer | `go test ./analyzer/...` |
| S3 | Analyzer cost inside gopls | The analyzer reads facts exported per package (tables, version ranges); it never loads a database. Measure with `go vet -vettool` on a large module | `cd analyzer && go build ./cmd/qlxvet && go vet -vettool=$PWD/qlxvet ./...` |
| S4 | Baseline against raw pgx and other libraries | `bench/`: QLX static and dynamic, hand-written, squirrel, goqu, bun, jet for the same statements. Budgets are in `TestBudgets`; in-repo budgets (node size, allocations) are in `budget_test.go` | `cd bench && go test -bench . -benchmem` |
| S5 | Introspection of PG 18 and 19 | `pgcat extract` reads pg_proc, pg_operator, pg_cast and pg_type; the standard-library `uuid` type is the mapping | `pgcat extract` |
