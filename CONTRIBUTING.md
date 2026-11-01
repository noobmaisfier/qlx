# Contributing to QLX

Thank you for considering a contribution. QLX is a type-safe SQL query
builder for Go and PostgreSQL 16–19 (pgx v5). Its promise is that a query which
compiles and passes the analyzer is accepted by the server, so most of the work
in this repository is about keeping that promise provable. This file explains
how the project is organised, what a good change looks like, and what the
checks expect.

Before a larger change, open an issue that describes the problem and the
approach. Small fixes, tests and documentation can go straight to a pull
request.

## Principles that every change must respect

- **No SQL text in the public API.** Builders take typed values, never strings
  that become SQL. `TestNoSQLTextInAPI` guards this; identifiers are always
  quoted by the renderer.
- **The root module has no dependencies.** Anything that needs a third-party
  package lives in its own module (`pgxexec`, `otelqlx`, `analyzer`,
  `bench`). `scripts/check_root.sh` fails if the root `go.mod` gains a
  `require` or a `go.sum` appears.
- **Builders are immutable; a statement is rendered once.** Every method
  returns a new value. Rendering produces a `Stmt` whose shape is cached; the
  allocation budgets in `budget_test.go` and `bench/` must still hold.
- **Mistakes are caught as early as possible.** Prefer, in this order: the Go
  compiler (types, sealed constraints), the analyzer (shape and scope visible
  in the source), render time (`ErrShape`, `ErrScope`, version gates), and
  finally the server (`PREPARE`). A rule that can be a type error should not be
  a runtime error.
- **Versions are explicit.** An API that exists only from, or only until, a
  PostgreSQL major carries it in code and in its doc comment (see below).
- **Never guess.** When the generator or the analyzer cannot decide something,
  it reports or stays silent. It does not invent a mapping, and a false
  positive in the analyzer is treated as a bug.

## Getting set up

You need:

- **Go 1.27** or later (the root module uses generic methods).
- **PostgreSQL server binaries** (`initdb`, `postgres`, `pg_ctl`) for the tests
  that talk to a server. Any of 16, 17, 18 or 19 works; CI runs all four.
- `psql` for the extraction commands.
- Python 3 only for regenerating the user guides (`docs/GUIDE.md`, `docs/GUIDE.en.md`).

```bash
git clone <your fork>
cd qlx
go build ./...
go test ./...                      # tests that need a server skip themselves
scripts/with_pg.sh go test ./...   # the same, against a throwaway cluster
```

`scripts/with_pg.sh` starts a cluster on a unix socket, loads
`fixture/schema.sql`, exports the `QLX_PG_*` variables, runs your command and
tears the cluster down. Set `PGBIN` if your binaries are not under
`/usr/lib/postgresql/*/bin`. To test another major, point `PGBIN` at it.

### Running the CI workflows locally

`scripts/ci-local.sh` replays `.github/workflows/ci.yml` and
`.github/workflows/postgis.yml` with Docker, without `act`: same images, same
databases, same environment variables, same commands.

```bash
scripts/ci-local.sh                # test ledgers extensions bench
scripts/ci-local.sh test --pg 18 --pgx latest --race
scripts/ci-local.sh full           # + fuzz (10M executions) + postgis
scripts/ci-local.sh --help
```

CI runs on every pull request, and once on `main` when a pull request is
merged. There is no schedule: the long fuzz run and the Go `tip` toolchain are
manual workflows (`.github/workflows/fuzz.yml`,
`.github/workflows/toolchain-tip.yml`), and PostGIS runs manually or when its
own files change on `main`.

| Job | What it does |
| --- | --- |
| `test` | the matrix: PostgreSQL 16/17/18/19 × pgx latest/min, `go vet`, the four modules, `gofmt` |
| `ledgers` | grammar ledger against PG 18 and master, command ledger, generated code is current |
| `extensions` | extracts and diffs pgvector, pg_trgm, hstore, ltree, citext, runs their wrappers |
| `bench` | budgets and benchmarks, written to `~/.cache/qlx-ci/bench.txt` |
| `fuzz` | `FuzzQuoteIdent` and `FuzzRenderNeverPanics` (the manual workflow) |
| `postgis` | extracts PostGIS from PGDG on 16–19, merges, diffs the committed ledger |
| `regress` | reports `skip`: `qlx regress` is a manual tool, see below |

`qlx regress` is not part of CI. It needs a `qlx.yaml`, a `schema.lock` and a
baseline `ledgers/regress.json`, and with the fixture schema only 4 of the
~50,000 statements of PostgreSQL's regression suite translate: the ledger is
9 MB and gates almost nothing. To use it by hand, generate a config for
`fixture/schema.sql` and compare against a baseline you record yourself.

Ports are assigned by Docker, so a PostgreSQL already listening on 5432 does
not matter. The pgx floor pin and the `sql2go` `go get` rewrite `go.mod`/`go.sum`
and restore them through a trap; containers are removed unless `--keep`.
Compared with GitHub, the toolchain is your local `go` (not `tip`) and there is
no cache between runs.

## Repository map

| Path | What it is |
| --- | --- |
| `*.go` (root) | the typed AST, builders, renderer, version gates |
| `qt/` | value types: `Null[T]`, `Opt[T]`, ranges, extension types |
| `pgcat/`, `pg/` | **generated** wrappers for catalog functions and operators |
| `pgcat/ext/*` | **generated** wrappers for extension packs |
| `pgcat.json`, `ledgers/` | the catalog and extension ledgers the wrappers come from |
| `cmd/qlx` | the generator and checks (`gen`, `check`, `verify`, `doctor`, `watch`, `prepare`, `regress`, `sql2go`, plus `init`, `vet`, `rows`) |
| `cmd/pgcat` | the ledger tool (`extract`, `merge`, `versions`, `diff`, `gen`, `docs`, `grammar`) |
| `internal/` | generator, ledger and pack code |
| `pgxexec/` | executor on pgx v5 (own module) |
| `analyzer/` | `go/analysis` analyzer, codes V001 and up (own module) |
| `otelqlx/`, `bench/` | OpenTelemetry adapter; benchmarks and budgets (own modules) |
| `fixture/` | the small hand-written schema package most tests use |
| `docs/` | guide, cookbook, limits, decisions, hand-off notes |

`docs/decisions.md` records why things are the way they are, and
`docs/limits.md` records what is deliberately not checked. Read both before
proposing to change a rule.

## Running the checks

Run what your change touches, then the whole suite before you open the pull
request.

```bash
gofmt -l .                              # must print nothing
go vet ./...
go test ./...
go test -race ./...
scripts/with_pg.sh go test ./...        # against a real server

(cd pgxexec     && go test ./...)
(cd otelqlx     && go test ./...)
(cd analyzer    && go test ./...)
(cd bench       && go test -run TestBudgets)
```

Changes that can behave differently across majors should be run on more than
one server (16 and 19 at least). Say in the pull request which majors you ran.

Useful test families:

- **Golden tests** (`golden_test.go`, `testdata/*.golden`): rendered SQL.
  Review the diff by hand, then update with `go test -run Golden -update`.
- **Compile-fail corpus** (`compilefail_test.go`): every rule the compiler is
  supposed to enforce has an example that must not build, with the message it
  must give. A new type-level rule needs a new entry.
- **Server tests**: rendered SQL is prepared on a real server. A new wrapper or
  builder feature should be covered by a case that runs `PREPARE`.
- **Fuzz** (`fuzz_render_test.go`, `fuzz_pg_test.go`): the renderer never
  panics, and well-typed random queries are accepted by the server.

## Generated files

Do not edit these by hand: `pgcat/**`, `pg/pg.go`, `docs/pgcat/**`,
`docs/GUIDE.md`, `docs/GUIDE.en.md`. A test
(`TestGeneratedPackagesAreUpToDate`) fails when they are stale or when a file
is not produced by any ledger.

```bash
go run ./cmd/pgcat gen  -ledger pgcat.json -o pgcat
go run ./cmd/pgcat docs -ledger pgcat.json -o docs/pgcat -pkg pg
for f in ledgers/*.json; do   # extension ledgers, not commands/grammar_map/regress
  go run ./cmd/pgcat gen -ledger "$f" -o pgcat
done
```

`scripts/release.sh` does all of this and also checks that every ledger lists
the targeted majors. If a generator change alters the output, commit the
regenerated files in the same pull request and say so in the description.

If you change a generator naming rule or type mapping, add a test next to the
existing ones in `internal/pgcat` (see `merge_test.go`).

## Ledgers: how server facts enter the repository

The catalog (`pgcat.json`) and the extension ledgers (`ledgers/*.json`) are
**extracted from real servers, never written by hand**.

```bash
DATABASE_URL=postgres://… go run ./cmd/pgcat extract -o /tmp/pg16.json          # on each major
go run ./cmd/pgcat merge -o pgcat.json /tmp/pg16.json /tmp/pg17.json /tmp/pg18.json /tmp/pg19.json
DATABASE_URL=postgres://… go run ./cmd/pgcat extract -ext pg_trgm -o /tmp/t16.json   # an extension
go run ./cmd/pgcat diff old.json new.json           # exit 1 when they differ
go run ./cmd/pgcat diff -report old.json new.json   # print only, exit 0
go run ./cmd/pgcat versions -ledger ledgers/x.json -like pgcat.json
```

A ledger must list every targeted major (16–19), otherwise the wrappers it
produces have no bound on the majors it never saw. `versions -like` and
`scripts/retag.sh` enforce it. A function that appears or disappears between
majors carries `since`/`until` and the generated wrapper carries the matching
gate and doc sentence.

A pack for a new extension needs its types mapped in `internal/packs` (the one
hand-written part), a ledger extracted from servers where the extension is
installed, and a round-trip test in `pgxexec` that skips itself when the
extension is missing. Until a real ledger exists, the pack is listed as
extraction-only in `internal/packs/status.go`.

## Adding or changing the public API

1. Put it in the right file and keep the type-level rules: a column family,
   `Pred`, `Expr[T]`, `AggExpr[T]`, and so on, decide where an expression may
   appear.
2. **Version gates.** If the API needs, or stops working on, a PostgreSQL major,
   gate it in code (`b.need(PGxx, "…")` at render time) **and** end its doc
   comment with a sentence the analyzer reads:

   ```go
   // Requires PostgreSQL 19.
   // Removed in PostgreSQL 19.
   ```

   `TestVersionGatedAPIIsDocumented` fails when a gated API lacks it, and the
   analyzer (V009) uses it to reject the call when `-targets` includes an older
   major.
3. Add golden and server tests, and a compile-fail entry when the point is that
   something must not compile.
4. Run `TestNoSQLTextInAPI`, the budget tests and the generated-package test.
5. Document user-visible behaviour in `docs/guide/GUIDE.tmpl.md` (see below),
   and note anything deliberately unsupported in `docs/limits.md`.

## Analyzer rules

The analyzer (`analyzer/qlxvet`) reports what the type system cannot see but
the source does. Each finding has a stable code (`V001`…); the list is in the
package documentation of `qlxvet.go` and in the user guide.

Follow the rules the existing checks use:

- A finding about something **present** (a column assigned twice, `Limit` with
  `FetchFirst`) holds for any chain, however it is built.
- A finding about something **absent** (an `UPDATE` with no `Set`) holds only
  when the chain is handed straight to something (argument, return value,
  composite literal). A chain stored in a variable may be completed later.
- Anything the analyzer cannot resolve (spread arguments, helper functions,
  reassigned variables) keeps the check **silent**. Do not guess; a false
  positive is a bug.

To add a rule:

1. Pick the next free code. Put the check in `shape.go` (statement shape),
   `gaps.go` (other source-level rules) or `mapping.go` (result types), and wire
   it in `qlxvet.go`. Add the code to the package comment.
2. Add cases to a package under `analyzer/qlxvet/testdata/src/` (create one
   and list it in `qlxvet_test.go` if needed). Every rule needs lines that
   **fire** with a `// want` comment, and lines that **stay quiet** (the valid
   form, the assigned-to-a-variable form, the unresolvable form). Facts need
   their own expectation, for example `// want ScanTargets:"row ID,Email"`.
3. Run `cd analyzer && go test ./...` with the real `golang.org/x/tools`.
4. Add the code to the table in `docs/guide/GUIDE.tmpl.md` and, if it has
   limits, to `docs/limits.md`.

A suppression is `//qlx:ignore <reason>`; a reason is mandatory (V010) and
`-audit` lists every suppression (V011).

## Documentation

- Repository documents are written in English. The user guide exists in two
  languages, **both generated**: `docs/GUIDE.en.md` (English) and `docs/GUIDE.md`
  (French). Edit `docs/guide/GUIDE.en.tmpl.md` and `docs/guide/GUIDE.tmpl.md` for
  prose, and `guide_examples_test.go` for code: the examples are compiled and
  rendered by `TestGuideExamples`, so the guide cannot show code that does not
  build.

  ```bash
  QLX_GUIDE_OUT=/tmp/guide_sql.txt go test -run TestGuideExamples .
  python3 docs/guide/gen.py /tmp/guide_sql.txt     # writes both guides
  ```

- A doc comment on an exported identifier starts with its name and says what
  it does, not how.
- Change both templates when you change the guide. If you write only one
  language, say so in the pull request and a maintainer will translate the
  other; do not leave the two out of step silently.

## Pull requests

- Keep a pull request to one subject. A refactor and a feature are two pull
  requests.
- Explain **why** in the description, which majors you tested on, and any
  generated files that changed.
- Include tests that fail without your change.
- `gofmt`, `go vet`, the root tests, and the tests of each module you touched
  must pass; CI runs the matrix over PostgreSQL 16–19.
- Commit messages: a short imperative summary line, then a body when the reason
  is not obvious. Keep generated changes in their own commit when practical.
- Do not add a dependency to the root module. If you think one is needed, open
  an issue first.

## Reporting bugs

Open an issue with: the QLX version, Go and PostgreSQL versions, the smallest
Go program that shows the problem, the SQL it renders (`stmt.SQL()`), and what
you expected. For a query the server rejects although it compiled and passed the
analyzer, that is a soundness bug in this project's central claim: please say so
in the title, because each one becomes a permanent regression case.

For a security problem (for example a way to make the renderer emit unquoted
input), do not open a public issue; contact the maintainers privately.
