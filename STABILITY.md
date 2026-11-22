# Stability

From 1.0, the root module and its sub-modules follow semantic versioning.

## What the promise covers

- The exported API of `qlx`, `qt`, `qjson`, `pgxexec`, `otelqlx`, `qjson/validator` and `qext/*`, checked by the API audit test.
- The **shape of generated code**: names and types users write (`surface.golden`). A change to it is a minor release at least, announced in the CHANGELOG; generated code carries `qlx.ABI1` so a runtime that cannot compile it refuses early.
- The `qlx.yaml` keys, the `schema.lock` format version, and the `qlxvet` rule codes (V001–V033): a code is never reused.
- The documented JSON contract of `qt` types (`Null`, `Opt`, `JSONB`, `Range`, `Multirange`).

## Cursor format of `qpage`

The cursor string is a contract with clients that hold it between requests. Within 1.x a cursor issued by one release is accepted by the next, so a rolling deploy never invalidates the pages in flight; the layout is versioned (first byte) and a new one is added alongside, not substituted. The sentinels `qpage.ErrSort`, `qpage.ErrCursor` and `qpage.ErrCursorKey` are part of the API.

## Optional interfaces read by qjson's strict decoder

`qjson` finds the types hidden inside a wrapper by duck typing, so a custom type can take part without importing `qjson`:

- `JSONInner() reflect.Type`: the type the wrapper holds (`Opt`, `Null`, `JSONB`, `Multirange` implement it). The strict decoder checks unknown keys inside it.
- `JSONKeys() []string`: the JSON keys of a type with a custom encoding (`Range`).

Both are part of the stability promise: the method names and meaning will not change in 1.x. A type with its own `UnmarshalJSON` that implements neither is treated as opaque.

## What it does not cover

- Behaviour that the server decides: a newer PostgreSQL than the generated code is allowed (`ConnectOptions.OnAhead` reports it), and generated catalog wrappers follow the pinned ledger.
- Internal packages (`internal/*`), `bench`, and anything marked experimental in its doc comment.
- The text of error messages and warnings; match on error types, codes or `Field`.
- Bug fixes that make an invalid value an error (for example out-of-range time-zone offsets or intervals) are not breaking changes.
