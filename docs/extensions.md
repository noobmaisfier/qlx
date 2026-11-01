# Extension packs: adding, refreshing, verifying

An extension pack has three parts:

1. **Types** (`internal/packs/packs.go`): the only hand-written part. Maps
   extension types to Go types (`qt.Vector`, `qt.Geometry`, `string` for citext).
2. **Ledger** (`ledgers/<pack>.json`): functions, operators and casts, extracted
   from real servers of **every** targeted major (16–19) and merged, so a
   function that appears or disappears between majors carries `since`/`until`.
3. **Generated wrappers** (`pgcat/ext/<name>`): produced by `pgcat gen`; never
   edited.

Nothing in a ledger is written by hand. A pack with types but no ledger is
*extraction-only* (`internal/packs/status.go`).

## Refreshing or adding a pack

You need one server per major with the extension available (the official
`postgres` images carry the contrib extensions; use `pgvector/pgvector` or
`postgis/postgis` images for those, or build the extension from source where no
image exists for a major).

```bash
QLX_PG_URLS="16=postgres://… 17=… 18=… 19=…" \
  scripts/extract_ext.sh pg_trgm hstore ltree citext
```

For each pack the script extracts on each major (`pgcat extract -ext` runs `CREATE EXTENSION IF NOT EXISTS` itself), merges
into `ledgers/<pack>.json`, refuses a ledger whose majors differ from
`pgcat.json`, prints the difference with the previous ledger, and regenerates
`pgcat/ext/<name>`.

Review the printed difference: a real server-side change (a function added or
removed) is expected; an unexplained one is a reason to stop.

## Verifying

```bash
go test ./...                                   # generated code is current, ledgers list every major
QLX_RELEASE=1 go test ./cmd/pgcat           # the majors check becomes an error
```

On a server with the extension installed, on each major:

```bash
QLX_PG_HOST=… QLX_PG_PORT=… QLX_PG_VERSION=18 PGPASSWORD=… \
  go test -run TestServerAcceptsEveryWrapper ./pgcat/ext/<name>
(cd pgxexec && QLX_TEST_DSN=… go test -run <Pack>Extension .)
```

Every wrapper must prepare on the majors inside its range and must be skipped
outside it. A round-trip test in `pgxexec` (see `vector_test.go`) skips itself
when the extension is missing.

## Finishing a pack that was extraction-only

1. Extract and generate as above.
2. Remove the pack from `extractionOnly` in `internal/packs/status.go`
   (`TestExtractionOnlyHasNoLedger` fails while both exist).
3. Add the round-trip test and update the extension section of the guide.
4. Record the counts (functions and operators wrapped, SHA-256 of the ledger)
   in `docs/HANDOFF.md`.
