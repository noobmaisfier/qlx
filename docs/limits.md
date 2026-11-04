# What the compiler cannot check

QLX moves as much as it can into the Go compiler, then into render time, then into the analyzer and the test database. This page says which tier catches what, and what nothing catches.

| Mistake | Caught by |
|---|---|
| Wrong column or table name | compiler (generated symbols) |
| Comparing a number with a string, summing text, `LIKE` on a number | compiler (typed operands) |
| NULL into a NOT NULL column; nullable value where a plain one is needed | compiler (`qt.Null[T]` vs `T`) |
| SQL text where a column, table or predicate is expected | compiler (no API accepts it) |
| Aggregate in `WHERE`; `OLD` outside `RETURNING` | compiler (distinct types) |
| UPDATE/DELETE without `Where`/`All`; bare column next to an aggregate without `GroupBy` | render time, and the analyzer earlier (V002, V007) |
| Column of a table that is not in the query's FROM/JOIN | render time (`ErrScope`), and the analyzer (V006) |
| INSERT that skips a required column | render time, and the analyzer (V008) |
| A function or clause newer than the target server | render time (`ErrUnsupportedByServer`), and the analyzer against `-targets` (V009) |
| Discarded builder, ignored render error, duplicate or late `Register` | analyzer (V001, V003–V005) |
| A statement missing a required part (UPDATE without `Set`, INSERT without a row, MERGE without `Using`/`On`/`When`, `DoUpdate` without assignments, `CASE` without `When`, empty `Truncate`) | render time (`ErrShape`), and the analyzer (V012) |
| Contradictory or duplicated parts (a column assigned twice, `Limit` with `FetchFirst`, `WithTies` without `FetchFirst`/`OrderBy`, rows with different columns, `INSERT ... SELECT` or set operands of different width) | render time (`ErrShape`), and the analyzer (V013) |
| Column selected next to a `GroupBy` that never mentions its table | render time, and the analyzer (V014) |
| `EXPLAIN` options that need `Analyze` (`Wal`, `IO`, `SerializeText/Binary`) or exclude it (`GenericPlan`) | render time, and the analyzer (V015) |
| `FOR UPDATE OF` a table that is not in the query | render time (`ErrScope`), and the analyzer (V016) |
| `SearchDepthFirst`/`SearchBreadthFirst`/`Cycle` on a CTE that is not recursive | render time, and the analyzer (V017) |
| A result type that does not match the selected columns (count, field names, Go types, nullability, order) | first execution (`pgxexec` plan error or pgx scan error), the analyzer for statements written out in the source (V018), and `pgxexec.VerifyRow` against a real server |
| Generated code out of date with the schema | `qlx check`, `qlx verify` |
| The live schema drifted from the generated code | `VerifySchema` at startup |
| The SQL is rejected by the server for the live schema | `qlx prepare` / `qtest.VerifyPrepare` (PREPARE on a real server) |

## What the analyzer reads, and what it leaves alone

`go vet -vettool=qlxvet ./...` (the `analyzer` module) reports V001 to V026. It works on the source, so it follows two rules:

- A finding about something that is **present** (a column assigned twice, `Limit` and `FetchFirst` together, `Explain(stmt, IO)` without `Analyze`) holds for any chain and is always reported.
- A finding about something **absent** (an UPDATE with no `Set`, an INSERT with no row) is reported only when the chain is handed straight to something: an argument, a composite literal, or a return from a function whose result is `qlx.Statement`. A builder stored in a variable, or returned as a builder type (a helper that returns `SelectQuery` or `TableFunc`), may be completed later by the caller, so it is left alone, exactly like V002.

When the analyzer cannot resolve an argument (a spread `args...`, a helper function that builds assignments, an alias whose name is not a constant or comes from a parameter or a reassigned variable, a subquery used as a table), the check stays silent rather than guess. `//qlx:ignore <reason>` suppresses one finding; `-audit` lists every suppression.

Version gates (V009) read the sentence `Requires PostgreSQL N.` (or `Removed in PostgreSQL N.`) from the doc comment of the function or method, which the generator writes for every catalog wrapper and which hand-written gated APIs carry too (`TestVersionGatedAPIIsDocumented` keeps it that way). `EXPLAIN` options, `Checkpoint(opts...)` and `WithHeader(n > 1)` are checked by call shape.

V018 reads `client.One[R](ctx, stmt)` and the other `pgxexec` entry points (`All`, `Maybe`, `Each`, the `*Scan` forms, `PipeAll`/`PipeOne`, `NewStatic0..4`) when the statement is a `Select` written out, wrapped in `Tagged`/`Register`, held by a package-level variable, or returned by a helper of the same package whose whole body is `return <Select chain>` (`activeUsers().Where(...)`; what the call site adds after the helper does not change the selected columns). A helper with several statements or returns, and a helper from another package, are not followed. A result type with a `ScanTargets` method is compared by position (count, Go types, and column names against field names to catch a permutation); any other struct by name, as `pgxexec` does; any other type must be one column. What it knows about Go types is deliberately narrow: bool, integer, float, time (`time.Time`, `qt.Timestamptz`), string, byte-slice and slice classes, the types that carry their own wire format (`qt.Date`, `qt.Interval`, `qt.TimeTZ`, `qt.Range`/`qt.Multirange`, `netip.Addr`/`netip.Prefix`, `[16]byte` as uuid) and `qt.Null[T]`; text-like fields are never judged against those, because a uuid or range column may be mapped to a string. A NULL-able column read into a plain `int64`/`string`/`bool`/`time.Time` field is reported, and so is a column from the optional side of a LEFT/RIGHT/FULL JOIN (declared NOT NULL, but NULL when no row matches; wrap it with `q.Nullable`) (pgx fails on the row that holds NULL); text types accept anything pgx can render as text.

`pgxexec.VerifyRow[R](ctx, conn, stmt)` is the run-time counterpart for what the source does not decide: it prepares the statement on a real server and checks that every result column has a pgx scan plan for its destination, with the same name and count rules as the executor. Put it in a test next to each statement. It cannot see NULLs.

V019 to V026 close the shape rules that `Render` only reports at run time: window functions in WHERE/HAVING/ON/GROUP BY, named windows missing from `Windows`, table functions and JSON/XML tables without their naming, JSON path variables that are not passed, GRANT/REVOKE parts, REFRESH and SET CONSTRAINTS shape, `SearchOrder`/`CycleMark` without SEARCH/CYCLE (also through a variable assigned once), and columns of another table used as conflict target or assignment. They stay silent on statements held in reassigned variables, paths held in variables, and subqueries as write targets. An aliased table is followed when its name is a constant: `Users.As("u")`, written inline or held by a variable assigned once, is one table, so `Update(Users.As("v")).Where(Users.ID.Eq(1))` is reported (the original symbol is not in scope once the table is aliased). When an alias is in scope and a column comes from a parameter, a struct field or a variable assigned more than once, the analyzer cannot tell whether it is the same alias under another name and stays silent.

`qlx vet` (standard library only) remains the zero-dependency subset: V001 to V005.

Left to render time and to `PREPARE` because the source does not decide them: values computed at run time (a `TABLESAMPLE` percentage, an LSN, an identifier built from input), queries assembled at run time (a helper with several statements or returns, a loop, a reassigned variable), parameters left unbound, and everything that depends on the live schema.

## What nothing can check

- **Row counts and constraints at run time**: a unique violation, a foreign-key violation, a serialization failure. `pgxexec` classifies these errors (`IsUnique`, `IsForeignKey`, `IsNotNull`, `IsCheck`, `IsSerialization`, retry in `InTx`), but they happen at run time.
- **Whether a NULL really is absent**: `.NotNull()` asserts an expression is not NULL; on PostgreSQL 19 it renders `error_on_null`, on 18 it checks at scan time. The assertion is yours.
- **Semantics**: the query may type-check, render and run, and still ask the wrong question. Types do not read intent.
- **Performance**: a typed query can still be a sequential scan. Use `Explain` and `pg_plan_advice`.
- **Many shapes (cache pressure)**: a shape is a query's structure with values replaced by their placeholders, so constant values reuse one entry. Varying the structure at run time (a select list, join set, `ORDER BY` or set of predicates chosen from input) is supported and correct, but each distinct structure is a new entry: the first call renders and validates it, later calls reuse it. The automatic cache is bounded (default 32,768 shapes, 1,024 per shard) and counts evictions; the ceiling is not preallocated, and a stored shape costs its SQL text plus about 200 bytes (roughly 8 to 15 MB if all 32,768 slots fill). A program with many legitimate shapes, such as user-built reports or dynamic filters, can raise the ceiling with `ConnectOptions.Shapes: qlx.NewShapeCache(n)` for one client or `qlx.DefaultShapes` globally, or render its hottest shapes once with `client.Render`. When `Evictions` keeps growing, one of those two is the lever. The analyzer cannot tell a data-dependent shape from a legitimate one, so the check is also at run time: `qlx.DefaultShapes.OnThrash(func(qlx.ShapeStats){ ... })` is called once, the first time a shape has to be evicted, and `Shapes.Stats().Evictions` keeps counting. Lists are already one shape: `col.Any(slice)` and `In(...)` both render `= ANY($1)`, so their length does not multiply shapes.
- **Simple protocol (`pgxexec.Simple`)**: pgx sends no parameter type OIDs, so it cannot encode `[16]byte`, and a `"char"` with the high bit set cannot be read. Ranges, multiranges and arrays of `qt` types have a text form and do work (see "What Go types cannot represent"). Use the default `CacheStatements` or `CacheDescribe` when a statement carries those types.
- **Serverless and transaction-mode poolers**: `CacheStatements` (the default) prepares named statements per connection, which a pooler in transaction mode and a scale-to-zero server can invalidate; use `pgxexec.CacheDescribe`. The shape cache, the pgx statement caches and the `qt.ID` key live in the process: nothing is shared between instances, and the ID key must be the same everywhere.
- **DDL**: QLX builds none (decision D2). Migrations stay in SQL.
- **XML special syntax, time-of-day types, a few JSON constructors**: see "Not supported" in [sql-to-go.md](sql-to-go.md). Each is tracked in the grammar ledger with a reason.
- **Extensions not in a pack**: the generator reads any installed extension (`pgcat extract -ext`), but only the listed packs are generated and tested in CI.
- **Unwrapped APIs in extension packs**: extension packs only wrap functions and operators whose types are mapped in Go. The PostGIS pack maps `box2d`, `box2df`, `box3d`, `gidx`, `spheroid`, `geometry_dump` and `valid_detail` as text-form named strings, so the functions that use them are generated from the committed ledger. Window functions of an extension (9 in PostGIS) are generated as `qlx.WinFn` and completed with `Over`; those of `pg_catalog` are hand-written in the core package. Functions and operators depending on them are omitted from `pgcat/ext/<pack>`.
- **A newer server than the generated code**: allowed (the ledger is pinned, the server is not); `ConnectOptions.OnAhead` reports it.

## What Go types cannot represent

A Go type is a smaller set than a PostgreSQL type. Where the two differ, QLX picks one of three behaviours: the type is chosen so nothing is lost, an option lets you choose, or the read fails loudly. Nothing is truncated silently.

| PostgreSQL value | Default Go type | What happens | How to handle it |
|---|---|---|---|
| `timestamptz` `infinity`, `-infinity` | `time.Time` | the read fails (a `time.Time` has no infinity) | config `timestamptz: qt` generates `qt.Timestamptz`, which keeps both infinities and compares with `Equal` |
| `date` `infinity` | `qt.Date` | represented | `DateInfinity`, `DateNegInfinity` |
| array with a NULL element (`int4[]`, `inet[]`) | `[]int32`, `[]netip.Addr` | the read fails on that row | config `array_nulls: true` generates `[]qt.Null[T]` |
| array of two or more dimensions | `[]T` | refused with an error naming the shape (it used to be flattened silently) | read into `pgtype.Array[T]` or `[][]T` |
| array whose lower bound is not 1 | `[]T` | refused | same |
| `uuid` | `[16]byte` | represented; has no text form | in the simple protocol use `string` for the column (see below) |
| `"char"` with the high bit set | `qt.Char` | pgx cannot read the text form of such a byte | use the binary protocol, or cast to `int` in the query |
| `time` `24:00:00` | `qt.TimeOfDay` | represented as 24h | `time.Time` cannot hold it |
| `float8` into a `float32` field | n/a | the value is converted by pgx and may overflow to infinity | declare the field `float64` unless the column is `float4` |
| `numeric` `NaN`, huge scale | `qt.Numeric` (text) | represented | convert at the edge with a decimal library |
| `interval` | `qt.Interval` (months, days, micros) | represented; not a `time.Duration`, which cannot say "one month" | convert deliberately |

Nullable columns are checked at three levels: the generator types them as `qt.Null[T]`; the analyzer (V018) reports a nullable column read into a plain field; `pgxexec.VerifyRow` asks the server (table OID and attribute number to `pg_attribute.attnotnull`) and reports the same, for plain columns of a table. Computed columns and the NULLs an outer join adds are invisible to the server's description; V018 covers the join case when the tables are named in the source.

### Simple protocol and PgBouncer

In `pgxexec.Simple` mode pgx has no type OIDs and sends every argument as text. QLX supplies the text forms that pgx lacks: `qt.Range` and `qt.Multirange` implement `driver.Valuer` with the range literal, and slices of `qt` types (`[]qt.Null[T]`, `[]qt.Date`, `[]qt.Numeric`, `[]qt.RawJSON`, `[]qt.Timestamptz`) are sent as an array literal. The round-trip matrix (`TestTypeRoundTripMatrix`) runs every case in the binary, describe and text modes, so the claim is tested. The two exceptions are a `[16]byte` argument and a `"char"` above 127, listed above.

## Typed ids are obfuscation

`ids: string` (see the cookbook) derives the text of an id from a secret with a permutation and a 16-bit check. It stops enumeration by counting, hides how many rows exist, and rejects mistyped or forged strings. It does not make an id a capability: anyone holding a valid string may use it, and a leaked key reveals every id's integer. Domain 0 <= n < 2^48. Rotating the key invalidates every string handed out. Values are not compatible with Hashids or any other library.
