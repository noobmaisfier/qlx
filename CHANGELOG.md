# Changelog

Entries list user-visible changes. A **Breaking** line is a change that can make
code that compiled, or produced a value, before behave differently.

## v1.2.0 - 2026-11-22

### Added

- `qlxvet` V032 (`OrderBy`/`Limit`/`Offset`/`FetchFirst` on the SELECT given to `Client.Page`, `Tx.Page`, `Window.Apply`), V033 (`qpage.Sorts` without `Tiebreak`), and V018 result mapping on `Client.Page`/`Tx.Page`.
- `qpage.CheckSecret()`; `SortSet.Window` now reports `ErrCursorKey` on every request, the first page included, instead of only when a next page makes a cursor.
- `qlx.SelectQuery.HasOrderOrLimit()`, used by `Window.Apply` to reject at run time any base `SELECT` that already carries `ORDER BY`, `LIMIT`, `OFFSET` or `FETCH FIRST` (including queries held in variables or built by helper functions).

## v1.1.0 - 2026-11-22

### Added

- `qpage`: cursor pagination for a service layer. Declare the sortable fields (`qpage.Sorts`, `Sort`, `Asc`, `Desc`, `Tiebreak`), set the secret once at startup (`qpage.SetSecret`, with rotation), and call `client.Page[Row](ctx, base, sorts, req)` (or `tx.Page`): items and next cursor as a string, no HTTP or JSON coupling. Signed cursors bound to their sort, client-chosen sort from an allowlist, bounded size. Sentinels: `qpage.ErrSort` (the same value as `qlx.ErrSort`), `qpage.ErrCursor`, `qpage.ErrCursorKey`. Without pgxexec: `SortSet.Window`, `Window.Apply`, `SortSet.Page`. The key of a page is read from the last row the way pgxexec scans the row type: by `db` tag or field name, or by position for a row that implements `ScanTargets` (generated rows, `//qlx:row`), so a DTO needs no tag.
- `qlx.KeyCol.Column` and `qlx.SelectQuery.OutputNames`, which qpage uses to read a key back from a row and to check the select list.

## v1.0.0 - 2026-11-21

First general-availability release. Changes since v1.0.0-rc4:

### Breaking

- **View columns are plain, nullable types.** PostgreSQL never marks a view column NOT NULL, so every view column is `qt.Null[T]` unless the generator can prove otherwise (see "Added"). Code that read a view column as `T` after relying on an older generator output must be regenerated and adjusted.
- **Views that accept writes now get `<View>Insert` and `<View>Patch` types**, and a view column proved NOT NULL changes from `qt.Null[T]` to `T`. Regenerate; the generated surface changes (`surface.golden`).
- **`qt.Opt[qt.Null[T]]` without `,omitzero` fails to marshal** with `qt.ErrUnsetOptNull` when the field is unset: `null` would be read back as "set to NULL". Add `,omitzero` to the json tag. The analyzer reports it (V031).
- **`qt.ParseTimeTZ` rejects offsets beyond ±15:59:59** (hour ≤ 15, minutes and seconds ≤ 59) instead of overflowing.
- **Interval fields out of range are rejected** instead of being truncated.
- **Strict JSON decoding now looks inside `qt.JSONB[T]`, `qt.Opt[struct]`, `qt.Null[struct]`, `qt.Range` and `qt.Multirange`.** Unknown keys there are errors (`settings.theme_typo`). Use `qt.JSONB[map[string]any]` or `any` for a free-form document.
- **`qt` wrapper types (`Null`, `Opt`, `JSONB`, `Range`, `Multirange`) implement `fmt.Stringer`.** `%v`, `%+v` and `fmt.Println` print the value (`"hi"`, `"NULL"`, `"unset"`, `{"a":1}`, `[1,5)`), not the struct representation. Use `%#v` to view struct fields. Code expecting struct formats in `%v` output changes.

### Added

- `qlx.yaml` `not_null: [view.column]` and automatic inference of NOT NULL view columns from the stored query tree (table NOT NULL columns, views of views, constants, `COALESCE`, `count(*)`, NOT NULL domains; outer-join sides, set operations and grouping sets stay nullable).
- Updatable views: `pg_relation_is_updatable` / `pg_column_is_updatable` drive which write types a view gets.
- User functions: `VARIADIC` as a Go variadic, default arguments as `F` / `FAll`, set-returning functions with `OUT`/`TABLE` columns or `SETOF` scalar as `qlx.TableFunc` with `<Name>Columns` / `<Name>Cols`. `qlx generate` lists what it still skips.
- `pgxexec`: transactions and bulk operations directly on `Client` and `Tx`.
- `qlxvet`: V031, `Opt[Null[T]]` without `omitzero`.
- `qjson/validator`: `required` and friends see through `qt.Null` and `qt.Opt`.
- `qjson` errors carry JSON field names; self-contained AI guides.

### Fixed

- CodeQL #9 (`ParseTimeTZ` integer conversion) and #15 (JSON error field allocation size).
- Deterministic encoding of table facts; hardened statement and shape caches.
- `pgxexec`: Ignore views and materialized views in `VerifyRow` nullability checks (views do not carry `attnotnull` constraints in PostgreSQL catalog).
