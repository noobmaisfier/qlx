# qlx analyzer

`qlxvet` is the `go/analysis` pass for QLX. It catches SQL mistakes that the Go
type system cannot see, before the program runs and without a database.

It reports 30 stable codes, V001 to V030. A finding names the rule, the
statement and the column or clause involved; a valid query stays silent.

## Install

```bash
go install github.com/noobmaisfier/qlx/analyzer/cmd/qlxvet@latest
go vet -vettool="$(go env GOPATH)/bin/qlxvet" ./...
```

The analyzer is a standard `go/analysis` pass, so gopls and golangci-lint can
run it too. `qlxvet ./...` works standalone, but `go vet` is the usual entry
point in CI.

To check a PostgreSQL version range, pass the versions your project targets:

```bash
go vet -vettool="$(go env GOPATH)/bin/qlxvet" \
  -targets=16,17,18,19 ./...
```

## What it catches

| Code | Finding |
| --- | --- |
| V001 | a builder result is discarded (builders are immutable) |
| V002 | `UPDATE`/`DELETE` handed on with neither `Where` nor `All` |
| V003 | `Register` name is not a constant, or is duplicated |
| V004 | the result or error of `SQL`/`Render` is discarded |
| V005 | `Register` inside a function body |
| V006 | a column of a table outside the query's `FROM`/`JOIN`/`USING` |
| V007 | an aggregate mixed with bare columns and no `GroupBy` |
| V008 | an `INSERT` row that omits a required column |
| V009 | a function, method or `EXPLAIN` option outside your `-targets` |
| V010 | `//qlx:ignore` without a reason |
| V011 | the `-audit` report of every suppression |
| V012 | a statement or clause missing a required part |
| V013 | contradictory, duplicated or mismatched parts |
| V014 | a bare column next to a `GroupBy` that does not cover its table |
| V015 | `EXPLAIN` options that require, or conflict with, `ANALYZE` |
| V016 | `FOR ... OF` a table outside the query |
| V017 | CTE `SEARCH`/`CYCLE` on a non-recursive CTE |
| V018 | a result type that does not match the selected columns |
| V019 | a window function in `WHERE`/`HAVING`/`ON`/`GROUP BY` |
| V020 | a named window used but not defined |
| V021 | a table function without `As`, or JSON/XML table without its columns or alias |
| V022 | a JSON path variable not passed, or invalid `QUOTES`/`WRAPPER` use |
| V023 | malformed or misdirected `GRANT`/`REVOKE` parts |
| V024 | `REFRESH ... CONCURRENTLY` with `NO DATA`, or unnamed `SET CONSTRAINTS` |
| V025 | `SearchOrder`/`CycleMark` without `SEARCH`/`CYCLE` |
| V026 | a conflict target or assignment naming another table's column |
| V027 | retired: `ToOrDefault` in a `SET` list no longer compiles; the code is not reused |
| V028 | a row lock with `DISTINCT`, `GROUP BY`, `HAVING`, an aggregate or a window |
| V029 | `SELECT DISTINCT` ordering, or `DISTINCT ON` not matching the leading `ORDER BY` |
| V030 | `ToNull()` on a column whose Go type cannot hold `NULL` (the server answers 23502) |

V018 also follows exported helpers whose whole body is `return <Select chain>`,
including helpers in another package: the defining package exports the selected
columns as a fact, and the call site reads it while `go vet ./...` analyses the
module. A generic helper, a helper with several statements or returns, or one
whose column list comes from its caller (`Select(cols...)`) is not followed.

## Suppressing a finding

Put the reason on the same line or the line before:

```go
//qlx:ignore the tenant is enforced by the connection's search_path
q.DeleteFrom(Users).All()
```

The reason is mandatory (V010). With `-audit`, every suppression is reported
(V011) with its reason, so CI can review the count and the justifications.

## How it works

The analyzer reads source only. It uses `golang.org/x/tools/go/analysis` facts
to carry generated table metadata (`TableFact`), PostgreSQL version ranges
(`VersionFact`), positional row layouts (`RowFact`) and exported Select columns
(`SelectFact`) across packages; it never connects to a database.

A finding about something **present** (a column assigned twice, `Limit` and
`FetchFirst` together) is always reported. A finding about something **absent**
(`UPDATE` without `Set`) is reported only when the chain is handed straight to
something: an argument, a composite literal, or a `return` whose result is a
`qlx.Statement`. A builder held in a variable or returned as a builder may be
completed later, so the analyzer stays silent rather than guess.

## Limits

The analyzer cannot see a query built at run time in a loop, from a reassigned
variable, from an unresolvable helper or from a parameter. Those are left to the
renderer, `PREPARE` and `pgxexec.VerifyRow`. The full rule-by-rule limits are in
[docs/limits.md](../docs/limits.md).

## Tests

```bash
cd analyzer
go test ./...
```

The testdata packages under `qlxvet/testdata/src/` are run with the real
`analysistest`; a corpus test also runs the built `qlxvet` over the root module,
`pgxexec`, `otelqlx` and `bench` the way a user would.

Back to the [project README](../README.md).
