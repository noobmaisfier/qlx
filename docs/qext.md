# Extension helpers (`qext/*`)

The generated wrappers in `pgcat/ext/*` cover every function and operator of an
extension, with names that mirror the catalog (`OpLtEqGtVectorVector`). The
`qext` packages are a small hand-written layer on top for the queries most
applications actually write. Each helper returns an ordinary `qlx.Expr` or
`qlx.Pred`, so it composes with everything else, and values always travel as
bound parameters.

| Package | Extension | Helpers |
| --- | --- | --- |
| `qext/textsearch` | built in (full-text search) | `Web`, `Plain`, `Phrase`, `Syntax`, `Document`, `Weighted`, `Match`, `Search`, `SearchIf`, `Rank`, `RankCover`, `Headline` |
| `qext/pgvector` | pgvector | `L2`, `Cosine`, `NegativeInnerProduct`, `L1` (vector and halfvec), `SetEfSearch`, `SetProbes` |
| `qext/trgm` | pg_trgm | `Similar`, `WordSimilar`, `SimilarIf`, `Distance`, `Similarity`, threshold settings |
| `qext/postgis` | PostGIS | `Point`, `WithinMeters`, `DistanceMeters`, `GeoJSON` (geography, WGS84) |
| `qext/ltree` | ltree | `IsDescendant`, `IsAncestor`, `Matches`, `Depth` |
| `qext/hstore` | hstore | `Get`, `HasKey`, `Contains`, `Merge` |

## Full-text search

```go
doc := textsearch.Document("english",
    textsearch.Weighted(textsearch.A, a.Title),
    textsearch.Weighted(textsearch.B, a.Body))

qlx.Select(a.ID, textsearch.Rank(doc, textsearch.Web("english", qlx.Val(text)))).
    From(a).
    Where(textsearch.SearchIf(doc, "english", filter.Text)). // absent when unset or blank
    OrderBy(...)
```

`Web` takes what a user types (quotes, `or`, `-word`) and never raises a syntax
error. Weighted parts accept nullable columns. `Highlight` options are bound as
data, so quotes in them cannot break out; a small `MaxWords` alone is accepted
(`MinWords` is lowered with it).

**Index.** A GIN index on an expression is used only when the query repeats
the same expression. Prefer a stored generated `tsvector` column with a GIN
index, and match against that column.

## pgvector

Distances use the operators (`<->`, `<=>`, `<#>`, `<+>`), because an index is
used only through operators, never through `l2_distance` and friends.
`NegativeInnerProduct` returns the operator's value (negated); order ascending
like the others. `SetEfSearch` and `SetProbes` are transaction-local settings.

## pg_trgm

Also operator-based, so a GIN/GIST trigram index applies. `WordSimilar` suits a
short query against long text. The threshold helpers are transaction-local.

## PostGIS

Helpers work on `geography`: distances are meters on the spheroid and
`WithinMeters` can use a GIST index. `Point` takes longitude first, as PostGIS
does. It emits the public `st_dwithin`, not the internal `_st_dwithin`, which
cannot use an index. For geometry work, use `pgcat/ext/postgis`.

## Notes

- Requires the extension in the database (`CREATE EXTENSION`); the helpers do
  not create it.
- Integration tests live in `pgxexec/qext_test.go` and skip when an extension
  is unavailable.
