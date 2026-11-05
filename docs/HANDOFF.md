# Handoff: what to run, what is unverified

## Release checklist (from the 27 October review)

The review found that the code is sound and the defects were in the release chain. Before sending a tree:

1. `scripts/release.sh` : merges the ledgers of PostgreSQL 16-19, regenerates `pgcat/`, `pg/` and `docs/pgcat`, runs `go mod tidy` in every module (this writes the `go.sum` files), and fails if anything is left uncommitted-different. A ledger of one version only produces wrappers that fail on the others (functions removed in 17 and later are unbounded).
2. `scripts/with_pg.sh go test ./...` on 16, 17, 18 and 19.
3. `cd analyzer && go test ./...` (testdata is its own module: `analyzer/qlxvet/testdata/go.mod`; `app2` covers qualified imports).
4. `cd bench && go test -run TestBudgets` (allocation budgets only; see the bench package comment for what is and is not time-bounded).
5. `scripts/sql2go.sh` for the optional parser (needs cgo and a C compiler).

Two tools check code: `qlx vet` (V001-V005, standard library only) and the analyzer via `go vet -vettool` (V001-V030, needs x/tools).


Written on a machine with Go 1.24 and PostgreSQL 16 only; section 0 records the
first verification round on Go 1.27.2 and PostgreSQL 16-19. Please run in this
order and send back the first failing output of each step.

## Second round: plan deviations fixed, gaps filled

Written on Go 1.24 + PostgreSQL 16 with the Go 1.27-only files removed from the
build (generic methods). Everything else was run there; please run, in order,
and send back the first failing output of each step:

```
go vet ./... && scripts/with_pg.sh go test ./...         # root module; 16, 17, 18, 19
cd pgxexec && QLX_TEST_DSN=... go test ./...           # incl. client_generic.go (Go 1.27 generic methods)
cd analyzer && go mod tidy && go test ./...               # new: go/analysis analyzer, needs x/tools
cd bench && go mod tidy && go test -run TestBudgets -v    # new: budgets vs squirrel/goqu/bun/jet
go test -tags sql2go ./internal/sql2go/ ./cmd/qlx/
go run ./cmd/pgcat grammar -y <REL_18_STABLE gram.y>,<master gram.y> -map ledgers/grammar_map.json
```

What changed since the first round (see docs/decisions.md for why):

- **Layout**: `pgfn` became `pgcat/<area>` packages plus the `pg` facade; extension packs under `pgcat/ext/*`; config is `qlx.yaml`; `go 1.27` everywhere; uuid is the standard library type; JSON columns are `qt.JSONB[T]`.
- **Runtime**: `Connect`/`Client`, `Maybe`, `Each`, plan-based scanning, static handles, pipelines, tracing and stats, reconnecting `ListenChan`, `ConnectOptions.Spec` version check.
- **Syntax gaps found by the grammar ledger and closed**: NOT LIKE, SIMILAR TO, ESCAPE, NOT BETWEEN, BETWEEN SYMMETRIC, COLLATE, AT TIME ZONE / AT LOCAL, CURRENT_USER family, ARRAY(subquery), ANY/ALL (subquery), NOT IN (subquery), IS NOT TRUE/FALSE/UNKNOWN; commands RESET, SHOW, SET ROLE, DISCARD variants, SET CONSTRAINTS, SET TRANSACTION, REFRESH MATERIALIZED VIEW, REINDEX, CLUSTER; `pg_plan_advice` builder.
- **Known unsupported, tracked with reasons in ledgers/grammar_map.json**: XML special syntax, time-of-day CURRENT_TIME/LOCALTIME, JSON()/JSON_SERIALIZE/JSON_SCALAR constructors.
- **Things I could not do here**: commit pgvector and PostGIS ledgers (needs real extension binaries; `pgcat extract -ext pgvector`), run the analyzer, bench and generic-method files, run on PostgreSQL 17-19. Grammar triage of the 18 and master `gram.y` is mine; please review the `supported` symbols for the productions you care about.
- **Automatic static path (plan section 9, Fingerprint)**: `qlx.ShapeCache` / `qlx.Resolve`; `Client.All/One/Maybe/Each/Exec` take any `qlx.Statement`, a builder with in-line values or a rendered `Stmt`. Verified: differential tests against full render (3000 random queries, miss and hit), bounded under concurrency with `-race`, end to end on PG16 (`pgxexec/shape_internal_test.go`). Not met: the plan's 300 ns / 0-allocation lookup budget (see D9l).
- **Date/time types and typed ids (D9m, D9n)**: `qt.Date`, `qt.TimeOfDay`, `qt.TimeTZ` (`qt/datetime.go`), `qt.ID[Space]` with the home-made codec (`qt/id.go`); generator options `date` and `ids` (`internal/codegen/ids.go`). Verified locally: unit tests (round trip, bijection over 300k values, tamper, cross-table rejection, 0 allocations), generator tests with and without the options, generated code vets, end to end on PG16 including `infinity` and `daterange` (`pgxexec/datetime_internal_test.go`). Not verified: Go 1.27.
- **One behavior change**: `Stmt.Args` no longer allocates a map (one allocation per call).

## 0. First verification round (Go 1.27.2; PostgreSQL 16, 17, 18, 19rc1)

Done on your side: build, vet, `go test`, `-race`, the full suite through
`scripts/with_pg.sh` on all four servers, 100 000 differential-fuzz queries per
server (0 rejected), generator end to end, `qlx vet`, `otelqlx`, pgx
integration tests. It found six defects; all are fixed in this revision:

| Defect | Fix |
| --- | --- |
| `JSON_VALUE ... DEFAULT $n` refused by the server | `DefaultOnEmpty/OnError` render a literal (`literalNode`); NaN/Inf and exotic types are refused |
| `JSON_QUERY ... WITH WRAPPER OMIT QUOTES` refused by the server | combination refused at render time (`ErrShape`); `WITHOUT WRAPPER` + quotes still renders |
| Removed built-ins (`pg_read_file_old`, `interval_avg`, ...) failed on 17+ | `until` in the ledger, computed by `Merge`; wrappers use `FuncCallRange` / `BinaryOpRange` / `PrefixOpRange`; the generated test skips entries outside the server's range and attributes errors to the right wrapper |
| `sql2go` INSERT/UPDATE/DELETE: `unsupported: target relation` | `embedded()` reads the flat relation struct; `kindOf` no longer treats flat structs as node wrappers; `EXISTS (SELECT 1 ...)` handled; 15 real-parser tests added (tag `sql2go`) |
| `BatchOne[Tuple1]` failed (scan by column name) | `BatchOneScan` / `BatchAllScan` scan by position through `ScanTargets` |
| `EXPLAIN (IO)` without ANALYZE | `IO`, `WAL`, `SERIALIZE` need ANALYZE, `GENERIC_PLAN` excludes it: refused at render time |
| (test bug) multirange `[3,9)` + `[9,)` | PostgreSQL merges adjacent ranges; the test now uses non-adjacent ones |

Also changed: `qtest.Render/Snapshot/Dump` keep `$n` placeholders instead of
failing on unbound `NewParam` queries, so parameterized registered queries can
be snapshotted, dumped and prepared.

**Please re-run the same matrix.** After this revision these must be green:

```
scripts/with_pg.sh go test ./...                  # on 16, 17, 18, 19
cd pgxexec && QLX_TEST_DSN=... go test ./...   # 6/6
go test -tags sql2go ./internal/sql2go/ ./cmd/qlx/
```

For the `pgcat` packages on 17/18/19, regenerate from a merged ledger of every major you
target (see section 4), otherwise functions removed in 17 are only bounded at
the next merged version.

## 1. Toolchain

All five modules declare `go 1.27` and pass on Go 1.27.2. The scratch modules the tests create use `internal/gover.Directive()`, so there is one place to change.

## 2. Core, no database needed

```
go vet ./...
go test ./...
```

Expected: everything passes except tests that skip for lack of PostgreSQL
binaries. The compile-fail corpus (`compilefail_test.go`) matches substrings of
compiler messages ("SmallInt", "Operand", "Selectable", ...). A newer compiler
may word an error differently; if one fails, send the message and I will adjust
the expected substring (the rule itself is still enforced if it fails to build).

## 3. With PostgreSQL 18, then 19

```
scripts/with_pg.sh go test ./...                       # PREPARE every statement
QLX_FUZZ_N=20000 scripts/with_pg.sh go test -run Differential .
```

`with_pg.sh` needs server binaries (set `PGBIN`). On 18 and 19 the tests tagged
`serverMin: 18/19` flip from "must be rejected" to "must be accepted"; a failure
there means either the version gate or the syntax is wrong. Items marked
UNVERIFIED in the code are PG19 syntax I could not check: REPACK, CHECKPOINT
options, WAIT FOR LSN, COPY FORMAT json / ON_ERROR set_null, IGNORE NULLS,
EXPLAIN IO, GRANTED BY, RANDOM(min,max) for timestamps. On PG17 or later the
SQL/JSON tests (`serverMin: 17`) should now be accepted.

## 4. Built-in catalog

```
go run ./cmd/pgcat extract -o /tmp/pg16.json           # DATABASE_URL=... on each server
go run ./cmd/pgcat extract -o /tmp/pg17.json
go run ./cmd/pgcat extract -o /tmp/pg18.json
go run ./cmd/pgcat extract -o /tmp/pg19.json
go run ./cmd/pgcat merge -o pgcat.json /tmp/pg16.json /tmp/pg17.json /tmp/pg18.json /tmp/pg19.json
go run ./cmd/pgcat gen -ledger pgcat.json -o pgcat
scripts/with_pg.sh go test ./pgcat/... ./cmd/pgcat
```

The merge fills the `since` and `until` markers, so functions new in 18/19
and functions removed in 17/18/19 become version-gated automatically. Merge
every major you target: with 16, 18 and 19 only, a function that left in 17 is
reported as removed in 18. `git diff pgfn` is the list of what 18 and 19 add.

## 5. pgx executor

```
cd pgxexec && go mod tidy && go vet ./...
createdb qlx_test && psql qlx_test -f ../fixture/schema.sql
QLX_TEST_DSN=postgres:///qlx_test go test ./...
```

Most likely compile fixes: pgx signatures (`pgx.CollectRows`, `BatchResults`,
`CopyFromSource`), `pgconn.StatementDescription`, `rows.Scan` with `qt.Null`.

## 6. Generator on a real schema

```
go run ./cmd/qlx gen -config qlx.json       # point it at your migrations
go build ./...
go run ./cmd/qlx check
```

## 7. sql2go

```
go get github.com/pganalyze/pg_query_go/v6      # needs cgo and a C compiler
go test -tags sql2go ./internal/sql2go/ ./cmd/qlx/
go run -tags sql2go ./cmd/qlx sql2go -config qlx.json -e "SELECT id FROM users WHERE id = 1"
```

The emitter is tested on parse trees built by hand in libpg_query's JSON shape
(PostgreSQL 16/17). If the real parser's JSON differs (field names, enum
spellings such as `SETOP_NONE`, how a zero `ival` is written), the fix is in
`internal/sql2go/{emit,expr,stmt}.go`; send me the failing SQL and its tree
(`pg_query.ParseToJSON`). Once it is green, delete the build tag: remove the
`//go:build` lines from `parse.go` and delete `parse_stub.go`.

## 8. qlx prepare / init

```
# in your app's tests: qtest.Dump(t, qlx.Dialect{Version: qlx.PG18}, "queries.json")
qlx prepare -config qlx.json -queries queries.json   # DATABASE_URL, database_url, or migrations
qlx init -dir /tmp/newproj && qlx gen -config /tmp/newproj/qlx.json
```

Both are covered by tests that run when PostgreSQL server binaries exist.

## 9. qlx vet

```
go run ./cmd/qlx vet ./...      # in your application module; exit 1 on findings
```

It needs the code to compile (it reads the compiler's export data). It checks
non-test files only. If you want it as `go vet -vettool`, that is a thin
`unitchecker` wrapper around `internal/vet/checks.go` once x/tools is available.

## 10. OpenTelemetry adapter

```
cd otelqlx && go mod tidy && go vet ./... && go test ./...
```

Likely fixes: OpenTelemetry versions in `go.mod` (I pinned v1.32.0 from memory,
`go mod tidy` will move them), `metricdata` access in the test, and attribute
names if you track a newer semantic-conventions release.

## Still unverified, or only verified in the first round (honest list)

- This revision's fixes: they pass here (Go 1.24, PG16) but have not met your
  matrix. The SQL/JSON `DEFAULT` literal and the removal gates matter most on
  PG17/18/19.
- `internal/sql2go/parse.go` and the real-parser tests (tag `sql2go`): the
  SELECT path was verified; the DML fix and the 15 conversions have not run.
- `pgxexec`: 4/6 integration tests passed in the first round; `BatchAllScan`,
  `BatchOneScan` and the corrected multirange test have not run. Custom
  (user-defined) range types must also be registered with pgtype (`LoadType`).
- `until` precision: needs a ledger per targeted major (see section 4).
- Go 1.27 specifics: `generic_methods.go` and `pgxexec/client_generic.go` use generic methods and are verified on Go 1.27.2 by the reviewer (see the verification report of 27 October); this repository's own environment is Go 1.24.
- Everything in the plan is built; the open items are listed in the README ("Verification state").

## Known design gaps

- The GROUP BY rule is checked only where a table's primary key is known and
  GROUP BY holds plain columns; elsewhere `qtest.VerifyPrepare` is the backstop.
- `pgfn` wrappers take operands, not literals: write `pgfn.AbsInt4(qlx.Val(int32(5)))`.
  A nullable column and a plain literal cannot mix in one call (use the `N`
  variant and `qlx.Val(qt.Some(x))`).


## Ajouts de la dernière passe (non vérifiés sur pgx)

- `Expr.Any(slice)` / `Expr.NotAll(slice)` : `= ANY($n)` / `<> ALL($n)`, un seul paramètre tableau ; slice vide = FALSE / TRUE.
- `qt.FromPtr(*T) Null[T]` (le `Maybe` du plan est l'exécuteur `pgxexec.Maybe`).
- `ForeignKey`, `FKPair`, `JoinBuilder.OnFK` ; le générateur émet `Orders.FK<Contrainte>(Users)` pour chaque FK dont la table cible est générée (fonctionne avec les alias).
- `pgxexec.Pipeline` + `PipeAll/PipeOne/PipeExec` + `Future[T]` (test `TestPipelineTypedFutures`, à lancer avec `QLX_TEST_DSN`).
- `qlx rows [-dir .]` : génère `qlx_rows.go` avec `ScanTargets()` pour les structs marquées `//qlx:row` (ordre des champs = ordre des colonnes, `qlx:"-"` ignore un champ ; champs embarqués et génériques refusés). Le repli `unsafe` par offsets n'est volontairement pas fait : le code généré couvre le besoin sans réflexion ni `unsafe`.
- `pgcat docs -ledger pgcat.json -o docs/pgcat` : pages Markdown de référence (index + fonctions par lettre + opérateurs, avec plages de versions). `docs/pgcat/` est généré depuis le ledger PG16 seul ; à régénérer après la fusion 16–19 pour que les colonnes « Versions » soient réelles.
- `pgcat diff old.json new.json` : fonctions/opérateurs/casts ajoutés, retirés, changés entre deux ledgers extraits de versions différentes (point de départ de la revue d'une release).
- `NotNull` (avec `error_on_null` sur PG19) existait déjà ; un test de rendu par version a été ajouté. Le mapping `uuid` reste `[16]byte` (pas de type uuid dans la stdlib à ma connaissance) ; l'override de type de `qlx.json` permet d'y brancher `google/uuid`.

## Catalog wrappers: what is generated (2026-10)

`pgcat gen` now also emits, from pgcat.json: variadic functions
(`rest ...qlx.Value`, or `qlx.ArgsT` when the variadic type is an
operand), void functions (`qt.Void` result), range/multirange polymorphism
(`qt.Range[E]`, `qt.Multirange[E]`), result nullability for strict-but-nullable
functions, and typed aggregates (`qlx.AggCall`, returning `AggExpr[R]`, so
Filter/Distinct/OrderBy/Over work). Also generated: scalar set-returning functions as `qlx.TableFunc`
(`FuncRange`), and functions with OUT/TABLE columns as a `TableFunc` plus a
`<Name>Cols` value with typed `Field`s (columns come from `Fn.Outs`).
Not generated: ordered-set aggregates (hand-written PercentileCont/Disc,
Mode), `count(*)`, window functions (all 15 hand-written), `internal`/`cstring`
results, and a few statistics types (`pg_ddl_command`, BRIN summaries). `"char"` is `qt.Char` (a byte; pgx reads and writes it as one). Integration test:
`pgxexec/catalog_test.go`.

### OUT columns in the ledger

`Fn.Outs` is filled by `pgcat extract` (new) and by `pgcat enrich -ledger
pgcat.json` on an existing ledger. pgcat.json was enriched on PG16 only: 25
functions that appeared in PG17-19 are skipped ("OUT columns not in the
ledger"). Run `DATABASE_URL=... pgcat enrich -ledger pgcat.json` against a
PG19 server (then 18, 17) and `pgcat gen` to cover them.

### Record shapes (verification round 7)

A function whose OUT record changes between majors (13 of them: pg_stat_get_wal,
pg_stat_get_io, pg_get_replication_slots, ...) is one ledger entry per shape:
`Fn.shapeKey()` (signature plus OUT columns) is the merge and diff identity, and
each shape's wrapper carries its own `FuncRange`. Variants are named by version
(`PgStatGetWalUpTo17`, `PgStatGetWalV18`, `PgStatGetWalFrom19`). pgcat.json in
this tree was enriched on PG16 only: re-extract on 16 to 19 and merge
(`scripts/release.sh`) to get every shape; `pgcat versions -min 2` guards the
single-major case without python3. `scripts/sql2go.sh` restores go.mod/go.sum
after its `go get`, so the root module keeps no dependency.

### Verification round 8 (30 October 2026)

Found on PG17-19 / pgx: versioned names remove the bare name of a function with
several shapes (examples use `PgGetKeywords`, one shape on 16-19); pgx cannot
scan tsvector, tsquery, bit strings and geometric types into a named string, so
`qt` gives them `Scan`/`Value`; `go mod tidy` in the root module re-adds
pg_query_go (release.sh now tidies only the modules with dependencies).
Tests: `TestGenerateNamesShapeVariantsByVersion`, `TestTextFormTypesRoundTrip`.
The 16-19 ledger (3488 functions) comes from the round-7 re-extraction.

### Performance pass (30 October 2026)

Profile of the shape-cache hit (`BenchmarkShapeCacheHit`, 2-vCPU box): about
half is building the statement (30 allocations: every builder step boxes a node
into an interface, inherent to immutable builders), half is `Resolve`. Changes
to `Resolve`: scope keys allocated once and the pooled builder keeps its scope
stack; `WHERE`/`HAVING` render without building an `And` node (`renderAnd`,
`renderLogic`); identifiers are hashed eight bytes at a time. Result:
4.2 us / 38 allocs -> 3.2-3.6 us / 32 allocs (grouped query), 1.38 -> 1.25 us,
13 -> 11 allocs (simple). The static path is unchanged (about 60-70 ns, 1
alloc). Not done: pointer-typed nodes would remove most of the 30 build
allocations but touch every builder.


## Round 9: the analyzer covers the render-time shape errors

- New file `analyzer/qlxvet/shape.go`: V012 (missing part), V013 (contradictory or duplicated parts), V014 (GroupBy coverage, uses the new `PK` in `TableFact`), V015 (EXPLAIN option rules), V016 (`Of` scope), V017 (CTE SEARCH/CYCLE on a non-recursive CTE). The rules for present/absent findings are in the file header and in docs/limits.md.
- V009 now reads methods (`Old`, `New`, `IgnoreNulls`, `DoSelect`, `Only`, `GrantedBy`, `Sort`, `Reverse`, `Casefold`...) and generic calls (`Uuidv7[T]()`), through `Origin()`, plus EXPLAIN options and `Checkpoint(opts)`/`WithHeader(n)`. Hand-written gated APIs got a `Requires PostgreSQL N.` sentence in their doc comment; `versiondoc_test.go` fails when a new gated API lacks it.
- Two fixes to existing behaviour: V006 no longer reports the tables an `EXISTS`/`LATERAL` subquery takes from its outer query (valid correlated subqueries were flagged), and one finding is reported once even when two walks reach it.
- New testdata package `analyzer/qlxvet/testdata/src/shape` (listed in `qlxvet_test.go`). It also holds the negative cases that must stay silent.
- Verified here only against a stand-in for `golang.org/x/tools/go/analysis` (a driver that type-checks the testdata and compares the diagnostics with the `// want` comments): all three testdata packages matched, and the 36 guide examples (valid code) produce no finding. The real `analysistest` run is yours: `cd analyzer && go mod tidy && go test ./...`.

### Round 9b: result-type mapping (V018, `pgxexec.VerifyRow`)

- `analyzer/qlxvet/mapping.go`: V018 compares the result type of `pgxexec.One/All/Maybe/Each`, the `*Scan` forms, `PipeAll/PipeOne` and `NewStatic0..4` (any package whose path ends in `/pgxexec`) with the items of a `Select` written out, wrapped in `Tagged`/`Register`, or held by a package-level variable. Exported helpers whose whole body is `return <Select chain>` are exported as `SelectFact` (the selected items as strings and flags, `colDesc`), so a helper in another package is followed; `ScanTargets` bodies of the form `return []any{&r.A, &r.B}` are exported as `RowFact` (so generated rows and `TupleN` work across packages). Type rules are intentionally narrow (see docs/limits.md); unknown types never produce a finding.
- `pgxexec.VerifyRow[R]` (verifyrow.go): PREPARE on a real server, then name and count rules as the executor and a pgx scan plan per column. It detects an impossible scan through the plan's unexported type name `*pgtype.scanPlanFail`; `TestVerifyRow` fails if pgx renames it. Verified on PG16.
- Testdata: `src/mapping` plus a stand-in `src/pgxexec`. Same verification caveat as round 9: run `cd analyzer && go mod tidy && go test ./...` yourself.
- Not done: the analyzer does not follow a statement held in a local variable or returned by a function; `Exec`-style calls have no result type; `Client` generic methods rely on `types.Info.Instances` recording the method's type arguments (Go 1.27), which I could not run here.

### Round 9c: nullability of outer joins, mapping chapter in the guide

- New `q.Nullable(col)` (funcs.go): retypes a value as `Expr[qt.Null[T]]` with no SQL change, for the optional side of LEFT/RIGHT/FULL JOIN, where a NOT NULL column is NULL when no row matches.
- V018 also reports a plain column from the optional side of an outer join read into a field that cannot hold NULL (`outerTables` in mapping.go). Aliased tables and subqueries are never reported.
- The type rules of V018 were corrected against pgx on PG16 (`VerifyRow` probes): int and float plan into each other, numbers plan into string, bool/time/text do not cross, `timestamptz` does not plan into string. The table in the guide (§10.10) is that list.
- Guide: §10 is now "Mappings et types" with §10.6 names, §10.7 type choice and overrides, §10.8 nullability (including the join trap), §10.9 values to parameters, §10.10 columns to result types, §10.11 how to verify. New compiled examples `param-typing` and `left-join-nullable`; `nullable_test.go`.

## Round 11 (31 October 2026): verification of the analyzer, VerifyRow and the guide

Tree 11 adds rounds 9, 9b and 9c: the analyzer's shape rules (V012-V017) and
result-type mapping (V018), `pgxexec.VerifyRow`, `q.Nullable`, outer-join
nullability, and a generated guide (`docs/guide/gen.py` + `GUIDE.tmpl.md` +
`guide_examples_test.go`). Everything below was run here, not assumed.

Found in the received tree and fixed here:

1. **`analyzer` did not pass `go test`.** `facts.go` now exports `RowFact` for
   every `ScanTargets` method, but `testdata/src/mapping/mapping.go` had no
   expectation for those facts. `analysistest` failed with
   `unexpected fact: row ID,Email` / `row UserID,ID`. The two declarations now
   carry `// want ScanTargets:"row ID,Email"` and
   `// want ScanTargets:"row UserID,ID"`; `cd analyzer && go test ./...` is green
   with the real `analysistest`, not a stand-in.
2. **`pgcat.json` regressed to the PG16-only ledger (3471 functions, SHA-256
   `01150895…de5f82`).** The canonical four-major re-extraction (3488 functions,
   805 operators, SHA-256 `9e840ec7…62bb03`) was restored and `pgcat gen` +
   `pgcat docs` were rerun. `scripts/release.sh` with `QLX_PG_URLS` over
   16/17/18/19 reproduced the same ledger byte for byte.
3. **CI still tested only 18 and 19 and exported `QLX_PG_URL`, read by no
   test**, so the database tests were silently skipped. The matrix now covers
   16-19 with official images, creates `qlx` / `qlx_test`, loads
   `fixture/schema.sql`, and exports `QLX_PG_HOST/DB/USER/VERSION` plus
   `QLX_TEST_DSN`. The contract was replayed locally on PG16 over TCP.
4. **README still said 18/19 and 3471 functions**; it now states the support
   range, the actual ledger counts and the verification state.

Results on the final tree:

- root suite: 31 packages ok on 16.15, 17.11, 18.4 and 19rc1; `-race` green in
  the root module;
- `TestServerAcceptsEveryWrapper`: 3721 / 3761 / 3845 / 3892 wrappers checked
  (PG 16/17/18/19), 0 failures;
- 100 000 differential-fuzz queries per server, seed 1: 0 rejected;
- `pgxexec` 18/18 on the four servers, plus `-race` on 18;
- `analyzer` (real `analysistest`), `bench` (`static 2, handwritten 2, dynamic 69,
  squirrel 104` allocs), `otelqlx`, `scripts/sql2go.sh`: green;
- `gofmt -l` empty; `go vet ./...` clean in all five modules;
- `docs/GUIDE.md` regenerated from `GUIDE.tmpl.md` and the examples is identical
  to the received `GUIDE (3).md`;
- `scripts/release.sh` with all four URLs: same ledger, `root module has no
  dependencies`, `go.sum` in `pgxexec`, `analyzer`, `bench` and `otelqlx`,
  no root-owned files.

Still open: pgvector/PostGIS ledgers (servers with the extensions), and the
first real GitHub Actions run of the 16-19 matrix.

### Round 11b: pgvector generated here (31 October 2026)

pgvector cannot be extracted in an environment that has no server with the
extension installed, so the artifacts were produced here and committed:

- **Ledger**: `ledgers/pgvector.json`, merged from PostgreSQL 16.15, 17.11,
  18.4 and 19rc1, all with pgvector **0.8.7**. The four extractions are
  identical except for the `versions` field; the merged ledger lists
  `[16 17 18 19]`, 118 functions, 40 operators and 23 casts.
  SHA-256 `fde8add84bee2a9792b7ff0ef56428af01c4299c987d666ca9135053ec7b29b7`.
- **Generated package**: `pgcat/ext/vector`, 67 of 118 functions and 40 of 40
  operators wrapped. The 51 unwrapped functions are server plumbing (29
  operator implementations, 11 `internal`/`cstring`, 3 cast functions) and add
  no user-facing API.
- **Tests**: `TestServerAcceptsEveryWrapper` prepares 117 wrappers on each of
  the four majors, 0 failures. `pgxexec/vector_test.go` runs `l2_distance`,
  the `<->` operator, a `vector` round trip through `text`, and a direct scan
  into `qt.Vector`; it skips when the server has no extension, so the normal
  four-major matrix is unaffected. It passes on pgvector 0.8.7 for 16-19.
- **CI**: the `extensions` job now uses official images
  (`pgvector/pgvector:pg18`, `postgres:18` for the contrib packs,
  `postgis/postgis:18-3.6`), extracts the pack, fails on pgvector ledger drift,
  prepares the generated wrappers and runs the pgvector round trip.
- **Caveat**: a literal vector is not pinned by `q.Val` alone; cast it first
  (`q.Cast(q.Val(v), q.NewPgType[qt.Vector]("public", "vector"))`) or use a
  bound `NewParam[qt.Vector]`. The guide says so.

PostGIS is still extraction-only: its type mapping (`qt.Geometry`,
`qt.Geography`) exists, but no ledger or generated package is committed.
The contrib packs (`pg_trgm`, `hstore`, `ltree`, `citext`) still had
single-major ledgers at that point; they were re-extracted over 16-19 in Round
12b below.


## Round 12 (1 November 2026): analyzer gaps closed (V019-V026)

Received `qlx (12).zip`, which adds eight analyzer rules on top of a base
from 26-27 October. Only the analyzer sources and the V019-V026 documentation
were adopted; the rest of the reference tree was kept, because the zip still
carries the PG16-only catalog ledger (3471 functions), no pgvector artifacts,
the old 18/19 CI and no `go.sum`.

Adopted:

- `analyzer/qlxvet/gaps.go` (532 lines): V019 window function in
  WHERE/HAVING/ON/GROUP BY, V020 named window not in `Windows`, V021 `Func`
  without `As` and JSON_TABLE/XMLTABLE without alias or columns, V022 JSON path
  variable not passed and QUOTES with WRAPPER, V023 GRANT/REVOKE shape,
  V024 REFRESH CONCURRENTLY + WITH NO DATA and SET CONSTRAINTS without a name,
  V025 `SearchOrder`/`CycleMark` without SEARCH/CYCLE (following a variable
  assigned once), V026 conflict target or assignment naming a column of
  another table. Empty `JSONPair{}` is V012; COPY option values newer than
  `-targets` are V009 (`OnErrorIgnore` 17, `LogVerbose` 17, `LogSilent` 18,
  `OnErrorSetNull` 19, `CopyJSON` 19). No VACUUM/ANALYZE option needs a gate.
- `qlxvet.go`, `qlxvet_test.go`, `testdata/src/gaps/gaps.go`,
  `docs/limits.md`, the V019-V026 table in the guide template, and the README /
  HANDOFF counters (V001-V030).
- `analyzer/go.mod`: explicit `example.com/qlx v0.0.0` plus the
  `x/mod`/`x/sync` indirects, so `go test` works from the committed `go.sum`
  without network access.

Kept from tree 11 and regenerated, never copied from the zip: `pgcat.json`
(3488 functions, 805 operators, SHA-256 `9e840ec7...62bb03`),
`ledgers/pgvector.json` (SHA-256 `fde8add8...7b29b7`), `pgcat/**`, `pg/pg.go`,
`docs/pgcat/**`, the 16-19 CI with its `extensions` job, and every `go.sum`.

Results on this tree:

- `cd analyzer && go test -count=1 ./...`: **green with the real
  `analysistest`** over `./src/app`, `./src/app2`, `./src/shape`,
  `./src/mapping` and `./src/gaps`; this is the run the author's local stub
  could not provide.
- Root suite: 33 packages ok on PG 16.15 / 17.11 / 18.4 / 19rc1, plus
  `-race` on PG 18.
- Wrappers: 3721 / 3761 / 3845 / 3892 prepared, 0 errors; pgvector 117 per
  major, 0 errors.
- `pgxexec` 18/18 on the four majors; the pgvector round trip is green on all
  four pgvector servers.
- `bench`, `otelqlx`, `sql2go`: green. `gofmt -l` empty; `go vet` clean in
  all five modules.
- `docs/GUIDE.md` regenerated: 1561 lines, exactly `GUIDE (3).md` plus the eight
  V019-V026 rows.
- `pgcat gen` / `pgcat docs` produced no diff, confirming the canonical
  ledgers were untouched.

Still open at that point: PostGIS ledger/package and the first GitHub Actions
run; the contrib packs were re-extracted in Round 12b.


### Round 12b: pgcat gates and contrib packs re-extracted over 16-19

Two changes, both verified here:

1. **`pgcat diff` is a gate.** It now returns an error (exit 1) unless the
   output is exactly `pgcat.NoDifferences`; `-report` prints the differences and
   exits 0 for release review. `TestDiffExitStatus` covers same, different and
   `-report`. The pgvector CI step can rely on the exit status alone.
2. **Extension ledgers must cover every catalog major.**
   `pgcat versions -ledger X -like pgcat.json` fails unless X lists exactly the
   same majors; `scripts/release.sh` runs it for every extension ledger.
   `TestVersionsLike` covers both outcomes, and
   `TestExtensionLedgersListEveryMajor` warns (fails with `QLX_RELEASE=1`).
   `TestGenerateExtensionRespectsVersionRange` locks the generated
   `FuncCallSince`/`FuncCallRange` bounds, the doc sentences and the server-test
   range entries with a synthetic four-major `pg_trgm` ledger.

The gate exposed the expected state, so the four contrib ledgers were
re-extracted from the real 16.15 / 17.11 / 18.4 / 19rc1 servers and merged:

| Pack | Versions | Functions | Operators | Wrapped | SHA-256 |
| --- | --- | ---: | ---: | ---: | --- |
| `pg_trgm` | [16 17 18 19] | 31 | 10 | 16 + 10 | `8c01d92a...2afaaa` |
| `hstore` | [16 17 18 19] | 60 | 20 | 31 + 20 | `93fad361...e81d6b` |
| `ltree` | [16 17 18 19] | 80 | 49 | 42 + 49 | `2f66804a...b6babc` |
| `citext` | [16 17 18 19] | 47 | 26 | 30 + 26 | `1e8bdeda...f95ed9` |

`ltree` gained `hash_ltree` and `hash_ltree_extended` in 17, so those two
wrappers now carry `FuncCallSince(17, ...)`; `hstore`'s `each(hstore)` regained
its two OUT columns (`key`, `value`). `pg_trgm` and `citext` have no API drift.

Results: `QLX_RELEASE=1 go test ./cmd/pgcat ./internal/pgcat` green, so no
extension ledger is single-major; `go test -v ./pgcat/ext/...` prepares
276 / 280 / 280 / 280 cases on PG 16/17/18/19 with 0 errors (vector skips
without the extension); the full root suite stays at 33 packages ok on each
major; `gofmt` and `go vet` are clean. `docs/GUIDE.md` was regenerated after the
extension paragraph update.

Still open: PostGIS ledger/package, and the first GitHub Actions run.


### Round 12c: PostGIS extraction workflow

- `.github/workflows/postgis.yml` (new): extracts PostGIS on 16, 17 and 18,
  merges the three into an artifact, and compares with `ledgers/postgis.json`
  only if one is ever committed. It never writes into the repository.
- `internal/packs/status.go` (new): `ExtractionOnly("postgis")` is true;
  `TestExtractionOnlyHasNoLedger` fails if a PostGIS ledger or generated
  package appears without removing the entry. `TestPostGISTypesMapped` keeps
  `qt.Geometry`/`qt.Geography` mapped.
- Replayed locally with `postgis/postgis:{16,17,18}-3.6`: PostGIS 3.6.4 exposes
  exactly the same API on the three majors (776 functions, 52 operators, 26
  casts), so the merged candidate is `[16 17 18]` and `pgcat diff` against each
  extraction is empty. `pgcat gen` wraps 535 functions and 37 operators; the
  generated package builds and prepares 973 wrappers on PG18 with 0 errors.
- Not committed: PostGIS has no PostgreSQL 19 image
  (`postgis/postgis:19-3.6` is absent), so a `[16 17 18]` ledger would fail
  `pgcat versions -like pgcat.json` and `QLX_RELEASE=1`. The pack stays
  extraction-only until PostGIS 19 exists or PostGIS is built against PG 19.
- The root suite is now 33 packages (the new `internal/packs` tests).


### Round 12d: PostGIS 3.7.0 delivered for 16-19

Round 12c stopped at extraction because no `postgis/postgis:19-*` image exists.
PGDG does publish `postgresql-{16,17,18,19}-postgis-3` 3.7.0, so I used official
PostgreSQL images and installed the package in each:

```bash
for v in 16 17 18 19; do
  docker run -d --name tp$v -e POSTGRES_PASSWORD=q postgres:$v   # 19: postgres:19rc1
  docker exec -u root tp$v sh -c "apt-get update && apt-get install -y postgresql-$v-postgis-3"
  docker exec tp$v psql -U postgres -c 'CREATE EXTENSION postgis'
done
```

The four PostGIS 3.7.0 extractions are identical: **809 functions, 52
operators, 26 casts**. `ledgers/postgis.json` is their merge over
`[16 17 18 19]` (SHA-256 `bd21242a9f6ef0c5474d6969ffbd7cd60e5a6b27f56253fef3e1114db8c10daf`).
`pgcat diff` against each extraction is `No differences.` and
`pgcat versions -like pgcat.json` passes.

`pgcat/ext/postgis` wrapped **560 of 809 functions** and **37 of 52 operators** before the type and window mapping (see "Regenerating the PostGIS wrappers" for the current figures)
(the rest are operator implementations, casts, `internal`/`cstring`, or use
unmapped types: box2d/box2df/box3d, gidx, spheroid, valid_detail,
geometry_dump). The generated `prepare_test` prepares **1017 wrappers on each of
the four majors, 0 errors**.

Changes:

- `ledgers/postgis.json` and `pgcat/ext/postgis/**` are committed;
- `internal/packs/status.go` keeps the extraction-only mechanism but the map is
  empty; `TestExtractionOnlyHasNoLedger` now asserts both pgvector and postgis
  are generated, and PostGIS is `Verified: true`;
- `.github/workflows/postgis.yml` installs PostGIS 3.7.0 from PGDG on
  PostgreSQL 16-19, prepares the generated wrappers on every major, merges the
  four extractions and diffs against the committed ledger;
- `.github/workflows/ci.yml` drops PostGIS from the `extensions` matrix: the
  16-19 workflow owns it (no `postgis/postgis:*-3.7` image exists);
- guide template regenerated: 1561 lines, 72 746 bytes.

The root suite is now 34 packages. Still open: the first GitHub Actions run and
`go.sum` regeneration on the author's machine.


## Regenerating the PostGIS wrappers

The pack maps `box2d`, `box2df`, `box3d`, `gidx`, `spheroid`, `geometry_dump` and `valid_detail` (`internal/packs/packs.go`, types in `qt/ext.go`), and the generator now emits an extension's window functions as `qlx.WinFn` (`qlx.WinCall`; the nine of PostGIS: `ST_ClusterDBSCAN`, `ST_ClusterKMeans`, `ST_ClusterIntersectingWin`, `ST_ClusterRelateWin`, `ST_ClusterWithinWin`, `ST_CoverageClean`, `ST_CoverageInvalidEdges`, `ST_CoverageSimplify`, `ST_MinimumSpanningTree`). The committed `pgcat/ext/postgis` predates both, so `TestGeneratedPackagesAreUpToDate` fails until it is regenerated.

Regeneration is a pure function of the committed ledger and needs **no PostGIS server**:

1. `go run ./cmd/pgcat gen -ledger ledgers/postgis.json -o pgcat`
2. `go run ./cmd/pgcat docs -ledger pgcat.json -o docs/pgcat` (no change expected: the facade pages list `pg_catalog` only).
3. `python3 docs/guide/gen.py` for the guide, if its template changed.
4. Commit `pgcat/ext/postgis`. Measured on the committed ledger: 624 of 809 functions (was 560) and 52 of 52 operators (was 37 of 52); `prepare_test.go` lists 1140 wrappers (was 1017).

Only `prepare_test` needs a server: run it with PostGIS 3.7 on PostgreSQL 16 to 19. Window wrappers are prepared as `fn(...) OVER ()`. Do not change the PostGIS version without updating the pack notes. (A new ledger, as opposed to regenerating from this one, does need a server: `pgcat extract -ext postgis` on each major, then `pgcat merge`.)

## Adding a contrib pack (pgcrypto, uuid-ossp, unaccent, fuzzystrmatch, btree_gist)

The five packs are defined in `internal/packs/packs.go` and listed in `extractionOnly` (`status.go`): the mapping exists, the ledger does not, and a ledger comes only from a real server. For each pack NAME, on PostgreSQL 16, 17, 18 and 19 with the contrib module installed:

1. `psql -c 'CREATE EXTENSION "NAME"'` in a scratch database (the name is quoted: `uuid-ossp` has a hyphen).
2. `pgcat extract -ext NAME` and merge the four extractions into `ledgers/NAME.json`, as was done for `pg_trgm`.
3. `pgcat gen` and `pgcat docs`. The package directory is `pgcat/ext/` plus the name without `_` and `-` (`uuidossp`, `btreegist`), see `pgcat.ExtPackage`.
4. Remove NAME from `extractionOnly`; `TestExtractionOnlyHasNoLedger` fails while both exist.
5. Run the generated `prepare_test` on the four majors: every wrapper must prepare.

Expect `pgcrypto` and `fuzzystrmatch` to be mostly text/bytea functions, `btree_gist` mostly operators and `<->` distance functions, `uuid-ossp` four to six functions plus the `uuid_ns_*` constants (these take no arguments and return `uuid`).
