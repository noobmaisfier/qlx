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

`go vet -vettool=qlxvet ./...` (the `analyzer` module) reports V001 to V017. It works on the source, so it follows two rules:

- A finding about something that is **present** (a column assigned twice, `Limit` and `FetchFirst` together, `Explain(stmt, IO)` without `Analyze`) holds for any chain and is always reported.
- A finding about something **absent** (an UPDATE with no `Set`, an INSERT with no row) is reported only when the chain is handed straight to something: an argument, a return value or a composite literal. A builder stored in a variable may be completed later, so it is left alone, exactly like V002.

When the analyzer cannot resolve an argument (a spread `args...`, a helper function that builds assignments, an aliased table, a subquery used as a table), the check stays silent rather than guess. `//qlx:ignore <reason>` suppresses one finding; `-audit` lists every suppression.

Version gates (V009) read the sentence `Requires PostgreSQL N.` (or `Removed in PostgreSQL N.`) from the doc comment of the function or method, which the generator writes for every catalog wrapper and which hand-written gated APIs carry too (`TestVersionGatedAPIIsDocumented` keeps it that way). `EXPLAIN` options, `Checkpoint(opts...)` and `WithHeader(n > 1)` are checked by call shape.

V018 reads `client.One[R](ctx, stmt)` and the other `pgxexec` entry points (`All`, `Maybe`, `Each`, the `*Scan` forms, `PipeAll`/`PipeOne`, `NewStatic0..4`) when the statement is a `Select` written out, wrapped in `Tagged`/`Register`, or held by a package-level variable. A result type with a `ScanTargets` method is compared by position (count, Go types, and column names against field names to catch a permutation); any other struct by name, as `pgxexec` does; any other type must be one column. What it knows about Go types is deliberately narrow: bool, integer, float, time, string, byte-slice and slice classes, and `qt.Null[T]`. A NULL-able column read into a plain `int64`/`string`/`bool`/`time.Time` field is reported, and so is a column from the optional side of a LEFT/RIGHT/FULL JOIN (declared NOT NULL, but NULL when no row matches; wrap it with `q.Nullable`) (pgx fails on the row that holds NULL); text types accept anything pgx can render as text.

`pgxexec.VerifyRow[R](ctx, conn, stmt)` is the run-time counterpart for what the source does not decide: it prepares the statement on a real server and checks that every result column has a pgx scan plan for its destination, with the same name and count rules as the executor. Put it in a test next to each statement. It cannot see NULLs.

V019 to V026 close the shape rules that `Render` only reports at run time: window functions in WHERE/HAVING/ON/GROUP BY, named windows missing from `Windows`, table functions and JSON/XML tables without their naming, JSON path variables that are not passed, GRANT/REVOKE parts, REFRESH and SET CONSTRAINTS shape, `SearchOrder`/`CycleMark` without SEARCH/CYCLE (also through a variable assigned once), and columns of another table used as conflict target or assignment. They stay silent on statements held in reassigned variables, paths held in variables, aliased tables and subqueries as write targets.

`qlx vet` (standard library only) remains the zero-dependency subset: V001 to V005.

Left to render time and to `PREPARE` because the source does not decide them: values computed at run time (a `TABLESAMPLE` percentage, an LSN, an identifier built from input), queries assembled across functions or in loops, parameters left unbound, and everything that depends on the live schema.

## What nothing can check

- **Row counts and constraints at run time**: a unique violation, a foreign-key violation, a serialization failure. `pgxexec` classifies these errors (`IsUnique`, `IsForeignKey`, `IsNotNull`, `IsCheck`, `IsSerialization`, retry in `InTx`), but they happen at run time.
- **Whether a NULL really is absent**: `.NotNull()` asserts an expression is not NULL; on PostgreSQL 19 it renders `error_on_null`, on 18 it checks at scan time. The assertion is yours.
- **Semantics**: the query may type-check, render and run, and still ask the wrong question. Types do not read intent.
- **Performance**: a typed query can still be a sequential scan. Use `Explain` and `pg_plan_advice`.
- **Shape explosion**: a shape is a query's structure with values replaced by their placeholders, so constant values reuse one entry. The automatic cache is bounded (default 16,384 shapes, 512 per shard) and counts evictions; the ceiling is not preallocated, and a stored shape costs its SQL text plus about 200 bytes. A program that varies the structure per call (a select list, join set, `ORDER BY` or set of predicates taken from input) builds a new shape each time: it still works and still renders correct SQL, but every call becomes a miss that pays a full render, and a full shard evicts an arbitrary entry, so a hot shape can be evicted repeatedly. Keep the structure fixed. Lists are already one shape: `col.Any(slice)` and `In(...)` both render `= ANY($1)`, so their length does not multiply shapes. Do not build columns or joins from input, and watch `Shapes.Stats().Evictions`.
- **Serverless and transaction-mode poolers**: `CacheStatements` (the default) prepares named statements per connection, which a pooler in transaction mode and a scale-to-zero server can invalidate; use `pgxexec.CacheDescribe`. The shape cache, the pgx statement caches and the `qt.ID` key live in the process: nothing is shared between instances, and the ID key must be the same everywhere.
- **DDL**: QLX builds none (decision D2). Migrations stay in SQL.
- **XML special syntax, time-of-day types, a few JSON constructors**: see "Not supported" in [sql-to-go.md](sql-to-go.md). Each is tracked in the grammar ledger with a reason.
- **Extensions not in a pack**: the generator reads any installed extension (`pgcat extract -ext`), but only the listed packs are generated and tested in CI.
- **Unwrapped APIs in extension packs**: extension packs only wrap functions and operators whose types are mapped in Go. Types left unmapped (e.g. in PostGIS: `box2d`, `box2df`, `box3d`, `gidx`, `spheroid`, `geometry_dump`, `valid_detail`) as well as window functions (9 in PostGIS) are intentional omissions of the pack, not bugs. Functions and operators depending on them are omitted from `pgcat/ext/<pack>`.
- **A newer server than the generated code**: allowed (the ledger is pinned, the server is not); `ConnectOptions.OnAhead` reports it.

## Typed ids are obfuscation

`ids: string` (see the cookbook) derives the text of an id from a secret with a permutation and a 16-bit check. It stops enumeration by counting, hides how many rows exist, and rejects mistyped or forged strings. It does not make an id a capability: anyone holding a valid string may use it, and a leaked key reveals every id's integer. Domain 0 <= n < 2^48. Rotating the key invalidates every string handed out. Values are not compatible with Hashids or any other library.
