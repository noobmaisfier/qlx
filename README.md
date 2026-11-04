# QLX

**A type-safe, SQL-shaped query builder for PostgreSQL 16–19 in Go, on pgx v5.**

You write queries as Go code that reads like SQL. The compiler, an analyzer
and a real `PREPARE` reject the mistakes SQL strings let through, and every
statement shape is rendered once and reused.

```go
rows, err := client.All[UserOrders](ctx,
    qlx.Select(Users.ID, Users.Email, qlx.Count(Orders.ID).As("orders")).
        From(Users).
        LeftJoin(Orders).On(Orders.UserID.EqX(Users.ID)).
        Where(Users.Tier.Eq("pro"), Users.Nickname.IsNotNull()).
        GroupBy(Users.ID).
        Having(qlx.Count(Orders.ID).Gt(5)).
        OrderBy(Users.CreatedAt.Desc().NullsLast()).
        Limit(50))
```

```sql
SELECT "users"."id", "users"."email", count("orders"."id") AS "orders"
FROM "public"."users" LEFT JOIN "public"."orders" ON "orders"."user_id" = "users"."id"
WHERE "users"."tier" = $1 AND "users"."nickname" IS NOT NULL
GROUP BY "users"."id" HAVING count("orders"."id") > $2
ORDER BY "users"."created_at" DESC NULLS LAST LIMIT $3
```

There is no string-based construction anywhere in the API: no raw fragments, no
`Where("age > ?")`. What is not modelled cannot be written until it is, and a
grammar ledger lists exactly what is missing.

> **Status: pre-release.** The core, the generator, the executor and the
> analyzer are complete against their plan and verified on PostgreSQL 16, 17,
> 18 and 19. See [Verification](#verification) for what has been run and what
> has not.

---

## Why QLX

Most query builders check that you wrote *valid Go*. QLX tries to check that
you wrote *valid SQL for your schema*, as early as each kind of mistake can be
seen:

| Tier | Catches | Example |
| --- | --- | --- |
| **Go compiler** | wrong operand types, nullability, `LIKE` on a number, `SUM` over text, an aggregate in `WHERE`, `OLD`/`NEW` outside `RETURNING`, mixing ids of two tables, an insert missing a required column | `Users.Balance.Like("%x")` does not compile |
| **Analyzer** (`go vet`, gopls) | what the types cannot express but the source shows: a column of a table that is not in `FROM`, a bare column next to a `GROUP BY`, `UPDATE` without `WHERE`, a row type that disagrees with the selected columns, a PostgreSQL 19 feature when your targets include 18 | V001–V026 |
| **Render time** | scope, grouping, shape and version rules on statements built dynamically | `ErrScope`, `ErrShape`, `ErrUnsupportedByServer` |
| **The server** | everything only PostgreSQL knows | `PREPARE` in tests (`qlx prepare`), `VerifyRow`, `VerifySchema` |

The central claim, measured by differential fuzzing, is that **a query which
compiles and passes the analyzer is accepted by PostgreSQL**. Every
counter-example becomes a permanent regression case.

## How it works

QLX is *schema first*. You keep writing migrations in SQL; the generator
turns the resulting schema into typed Go.

```
migrations/*.sql ──► qlx gen ──► db/ (typed tables, row structs, inserts, patches)
                                └─► schema.lock (committed; checked in CI, no database needed)

PostgreSQL's own catalog ──► pgcat ──► typed wrappers for every built-in function and operator
```

- **Typed tables.** `Users.ID` is a `NumCol[int64]`, `Users.Nickname` a
  `TextCol[qt.Null[string]]`. A column knows its operations: text columns have
  `Like`, numeric ones have `Between`, only nullable ones have `IsNull`.
- **Required columns are constructor arguments.** `NewUserInsert(email)` takes
  exactly the columns that have no default; forgetting one is a compile error.
- **Patches are `qt.Opt` fields.** Only the fields you set are written.
- **Functions and operators come from the server.** 3,488 functions and 805
  operators are extracted from real PostgreSQL 16–19 servers with their
  version ranges; each wrapper is generated, typed and checked with `PREPARE`.
- **One statement shape is rendered once.** The first call renders and caches;
  later calls hash the shape and only collect the values.

## Quick start

Requirements: Go 1.27 or later, PostgreSQL 16–19 (18 and 19 recommended for
production), and PostgreSQL server binaries on the machine that runs
`qlx gen` (it applies your migrations on a throwaway server, or reads a live
database through `database_url`).

```bash
go get github.com/noobmaisfier/qlx
go install github.com/noobmaisfier/qlx/cmd/qlx@latest

qlx init     # writes qlx.yaml and migrations/001_init.sql (never overwrites)
# ... write your schema in migrations/*.sql ...
qlx gen      # writes the typed package and schema.lock
qlx check    # CI: fails if generated code or schema.lock is stale or hand-edited
```

```go
client, err := pgxexec.Connect(ctx, os.Getenv("DATABASE_URL"), pgxexec.ConnectOptions{
    Spec:      &db.Spec, // checks the server version against your targets
    MinServer: 16,
})
if err != nil { return err }
defer client.Close()

// Read
type UserRow struct {
    ID    int64  `db:"id"`
    Email string `db:"email"`
}
rows, err := client.All[UserRow](ctx, qlx.Select(Users.ID, Users.Email).From(Users).Where(Users.Tier.Eq("pro")))
one,  err := client.One[UserRow](ctx, qlx.Select(Users.ID, Users.Email).From(Users).Where(Users.ID.Eq(7)))
n,    err := client.One[int64](ctx, qlx.Select(qlx.CountAll()).From(Users))

// Write
tag, err := client.Exec(ctx, qlx.InsertInto(Users).Row(Users.Email.To("ada@example.com")).
    OnConflict(Users.Email).DoUpdate(Users.Tier.To("pro")))

// Write and read back, with PostgreSQL 18's OLD and NEW
got, err := client.One[qlx.Tuple2[int64, int64]](ctx, qlx.Update(Users).
    Set(Users.Balance.ToX(Users.Balance.Add(10))).Where(Users.ID.Eq(7)).
    Returning(Users.Balance.Old().As("before"), Users.Balance.New().As("after")))
```

`client.All[R]`, `One`, `Maybe` and `Each` use Go 1.27 generic methods. The
package functions `pgxexec.All[R](ctx, db, stmt)` do the same on a `pgx.Tx`.
`R` can be a scalar, a `qlx.Tuple1…Tuple8`, a struct read by position
(`ScanTargets`, no reflection) or a struct read by name through a cached plan.

The complete walkthrough is **[docs/GUIDE.en.md](docs/GUIDE.en.md)** (English;
[French version](docs/GUIDE.md)). Every code sample in it is compiled and
rendered by the test suite, so none can be wrong.

## What you can write

- **Queries:** `SELECT` with joins (including `LATERAL`), `WHERE`, `GROUP BY`,
  `HAVING`, `DISTINCT ON`, window functions and named windows, CTEs including
  `RECURSIVE` with `SEARCH`/`CYCLE`, subqueries, set operations, row locking,
  `VALUES`, table functions, `TABLESAMPLE`.
- **Writes:** `INSERT` (rows, `SELECT`, `DEFAULT VALUES`, `OVERRIDING`), upserts
  (`DoNothing`, `DoUpdate`, `DoSelect`), `UPDATE ... FROM`, `DELETE ... USING`,
  `MERGE`, `RETURNING` with `OLD`/`NEW`.
- **SQL/JSON:** `JSON_OBJECT`, `JSON_ARRAY`, `JSON_QUERY`, `JSON_VALUE`,
  `JSON_EXISTS`, `JSON_TABLE`, `IS JSON`, typed path variables.
- **Operations:** `COPY`, `VACUUM`/`ANALYZE`/`REPACK`, `EXPLAIN` with every
  option, `GRANT`/`REVOKE`, `LISTEN`/`NOTIFY`, `SET`, `LOCK`, `pg_plan_advice`,
  `WAIT FOR LSN`.
- **Execution:** transactions with retries and savepoints, pipelines and
  batches, binary `COPY`, row-level-security tenancy, OpenTelemetry tracing,
  `VerifyRow` and `VerifySchema` to check mappings and schema against a server.
- **Dynamic queries without strings:** optional filters built from `qt.Opt`,
  conditions assembled in loops, `Any(slice)` for lists.

## The analyzer

The analyzer is a `go/analysis` pass, so it runs under `go vet`, gopls and
golangci-lint.

```bash
go install github.com/noobmaisfier/qlx/analyzer/cmd/qlxvet@latest
go vet -vettool=$(which qlxvet) ./...
```

It reports 26 stable codes, V001 to V026 (V011 is the audit report of
suppressions). A few of them:

| Code | Reports |
| --- | --- |
| V002 | `Update`/`Delete` handed on with neither `Where` nor `All` |
| V006 | a column of a table that is not in `FROM`/`JOIN` |
| V009 | a function, method or `EXPLAIN` option newer than your `-targets` |
| V014 | a bare column next to a `GroupBy` that does not cover it |
| V018 | a result type that does not match the selected columns (count, names, Go types, nullability, order) |
| V019 | a window function in `WHERE`/`HAVING`/`ON` |
| V022 | a JSON path variable that is not passed |
| V026 | a column of another table used as an assignment or conflict target |

The full list is in [the guide](docs/GUIDE.en.md#121-who-catches-what). The
analyzer **stays silent rather than guess**: a query built in a loop, across
functions or in a reassigned variable is left to the render step and to
`PREPARE`. A finding you accept is suppressed with `//qlx:ignore <reason>`
(the reason is mandatory), and `-audit` lists every suppression.

`qlx vet` (standard library only, no `x/tools`) is the zero-dependency
subset, V001 to V005.

## PostgreSQL coverage

QLX targets **PostgreSQL 16, 17, 18 and 19** and tells you when you step
outside your targets. A construct that needs a newer major than your oldest
target is rejected by the analyzer (V009) and, if it slips through, by the
renderer on a server that lacks it.

| Source | Functions wrapped | Operators wrapped |
| --- | ---: | ---: |
| PostgreSQL catalog | 1,862 of 3,488 | 793 of 805 |
| pgvector 0.8.7 | 67 of 118 | 40 of 40 |
| PostGIS 3.7 | 560 of 809 | 37 of 52 |
| pg_trgm | 16 of 31 | 10 of 10 |
| hstore | 31 of 60 | 20 of 20 |
| ltree | 42 of 80 | 49 of 49 |
| citext | 30 of 47 | 26 of 26 |

What is not wrapped is server plumbing (`internal` and `cstring` arguments,
operator implementations, casts, which `Cast` covers), a few types with no Go
mapping yet (`pg_ddl_command`, `fdw_handler`, `index_am_handler`) and window functions of
extensions. The PostGIS text-form types (`box2d`, `box3d`, `gidx`, `spheroid`, ...) are mapped
as named strings; the committed PostGIS ledger predates that mapping until it is regenerated
(see docs/HANDOFF.md, "Regenerating the PostGIS ledger"). Set-returning, variadic, default-argument and `OUT`-parameter
functions are generated with their own shapes, never guessed.

Extension packs are enabled in `qlx.yaml` (`extensions: [pgvector, postgis,
pg_trgm, hstore, ltree, citext]`). Values of `vector`, `halfvec`, `hstore`,
`ltree` and similar travel as typed Go values (`qt.Vector`, `qt.Hstore`,
`qt.LTree`); PostGIS `geometry` and `geography` are extended well-known binary
in hex, for the geometry library of your choice.

## Performance

A statement shape is rendered once per process. After that:

- a **static handle** (`NewStatic…`, render once and bind per call) costs one
  allocation per call for its arguments, about 60–70 ns;
- the **automatic path** (an inline builder with values) hashes the shape and
  collects values; the dynamic path allocates less than squirrel (69 against
  104 in the budget test) while keeping every check;
- AST nodes are at most 80 bytes, and the allocation budgets are enforced by
  tests (`budget_test.go`, `bench/`), so a regression fails the build.

`bench/` compares against squirrel, goqu, bun, jet and hand-written pgx.
Benchmark on your own workload before drawing conclusions.

## Verification

What is run, and where:

| Check | Result |
| --- | --- |
| Full test suite against real servers | 32 packages pass on PostgreSQL 16.15, 17.11, 18.4 and 19rc1 (and with `-race` on 18) |
| Every generated wrapper prepared on a server | 3,721 / 3,761 / 3,845 / 3,892 cases on 16 / 17 / 18 / 19, 0 errors; each extension pack prepared on all four majors |
| Differential fuzz (well-typed random queries vs `PREPARE`) | 100,000 queries per major, 0 rejected |
| Analyzer with the real `analysistest` | green on six test packages, including Go 1.27 generic methods on the real `pgxexec.Client` |
| Executor (`pgxexec`) | 18 tests on all four majors |
| Compile-fail corpus | every rule the compiler enforces has an example that must not build |
| Generated code | regenerated from the ledgers in CI and compared byte for byte |

Not yet observed: the first run of the GitHub Actions workflows (matrix over
16–19 and the extension jobs). PostgreSQL 19 is verified on a release
candidate; re-pinning on RC2 and GA is planned in [docs/watchlist.md](docs/watchlist.md).

## Repository layout

| Path | What it is |
| --- | --- |
| `qlx` (root) | typed AST and builders, renderer, shape cache, version gates; no dependencies |
| `qt/` | `Null[T]`, `Opt[T]`, ranges, `JSONB[T]`, extension value types |
| `pgcat/`, `pg/` | generated wrappers for catalog functions and operators, and a flat facade |
| `pgcat/ext/*` | generated wrappers for extension packs |
| `pgxexec/` | executor on pgx v5 (own module) |
| `analyzer/` | the `go/analysis` analyzer (own module) |
| `otelqlx/` | OpenTelemetry adapter (own module) |
| `bench/` | comparisons and allocation budgets (own module) |
| `cmd/qlx`, `cmd/pgcat` | the generator and checks; the ledger tool |
| `ledgers/`, `pgcat.json` | catalog and extension ledgers extracted from real servers |
| `docs/` | guide, cookbook, limits, decisions, migration guides |

The root module has no dependencies; anything that needs one lives in its own
module, so your `go.mod` only gains what you use.

## Documentation

- [User guide](docs/GUIDE.en.md) (also in [French](docs/GUIDE.md)): start here.
- [Cookbook](docs/cookbook.md): recipes (keyset pagination, work queues, soft delete, tenancy).
- [SQL to Go cheat sheet](docs/sql-to-go.md): the SQL you know, written in QLX.
- [Migration guides](docs/migration.md): from squirrel, goqu, bun, jet and raw pgx.
- [Limits](docs/limits.md): what each tier can and cannot catch.
- [DTOs](docs/dtos.md): request and response structs, patches, ids, Echo.
- [Extension packs](docs/extensions.md): adding or refreshing a pack.
- [Decisions](docs/decisions.md): why it is the way it is.
- [Watch list](docs/watchlist.md): PostgreSQL 19 and the re-pinning plan.
- [Generated function reference](docs/pgcat/README.md).

## Known limits

- There is **no raw SQL escape hatch**, by design.
- `In` with a nullable element type is not supported.
- `COPY (query)` and `COPY ... WHERE` cannot take parameters (the server refuses them).
- There are no DDL builders: migrations stay SQL.
- PostgreSQL 19 support is verified on a release candidate.
- PostGIS wrappers cover the committed ledger; the functions that use `box2d`, `box3d`,
  `gidx`, `spheroid`, `geometry_dump` and `valid_detail` appear after the ledger is
  regenerated with the extended pack (their Go types exist already).

Details, and the reasoning, in [docs/limits.md](docs/limits.md).

## Contributing

Contributions are welcome: bug reports (especially queries that compiled and
passed the analyzer but that PostgreSQL rejected), tests, documentation,
analyzer rules and generator improvements. Read [CONTRIBUTING.md](CONTRIBUTING.md)
and the [code of conduct](CODE_OF_CONDUCT.md) first.

```bash
go build ./... && go test ./...            # tests that need a server skip themselves
scripts/with_pg.sh go test ./...           # the same against a throwaway PostgreSQL cluster
```

## License

To be decided before the first release (see `LICENSE`).
