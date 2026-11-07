# otelqlx

OpenTelemetry instrumentation for QLX and `pgxexec`. Every statement becomes a
client span and a duration histogram, with bounded names and no argument values
in the telemetry.

It is a separate module, so the core library and `pgxexec` do not depend on
OpenTelemetry.

## Install

```bash
go get github.com/noobmaisfier/qlx/otelqlx
```

## Use

```go
import (
    q "github.com/noobmaisfier/qlx"
    "github.com/noobmaisfier/qlx/otelqlx"
    "github.com/noobmaisfier/qlx/pgxexec"
)

obs, err := otelqlx.New(
    otelqlx.WithTracerProvider(tracerProvider),
    otelqlx.WithMeterProvider(meterProvider),
)
if err != nil {
    return err
}

tracedDB := pgxexec.Observed(client.DB(), obs.Observe)

st, _ := q.Render(q.Dialect{Version: q.PG18},
    q.Tagged("users.by_id",
        q.Select(db.Users.ID, db.Users.Email).
            From(db.Users).
            Where(db.Users.ID.Eq(7))))
user, err := pgxexec.One[UserRow](ctx, tracedDB, st)
```

`Observe` has the signature of `pgxexec.Observer`. Without explicit providers,
the global OpenTelemetry providers are used:

```go
obs, err := otelqlx.New()
tracedDB := pgxexec.Observed(pool, obs.Observe)
```

`pgxexec.Observed` also works with any `pgx.Tx`, `*pgx.Conn` or
`*pgxpool.Pool`.

## What is reported

Spans follow the stable database semantic conventions:

| Attribute | Value |
| --- | --- |
| `db.system.name` | `postgresql` |
| `db.operation.name` | `SELECT`, `INSERT`, `UPDATE`, ... |
| `db.query.summary` | the `qlx.Tagged` name, when present |
| `db.query.text` | the SQL with `$n` placeholders (on by default) |
| `db.response.status_code` | the PostgreSQL SQLSTATE |
| `error.type` | SQLSTATE, `context.Canceled`, `context.DeadlineExceeded` or `_OTHER` |

The span name is the `qlx.Tagged` name, or the SQL verb when untagged. The same
name is used for the metric label, so cardinality stays bounded:

```text
metric: db.client.operation.duration  (seconds)
```

Arguments are never recorded. Query text contains placeholders only, and
PostgreSQL error messages are not recorded by default: they can contain row
values (`Key (email)=(a@b.c) already exists`). The SQLSTATE is recorded instead.

## Options

| Option | Default | Effect |
| --- | --- | --- |
| `WithTracerProvider(tp)` | global provider | use a specific tracer provider |
| `WithMeterProvider(mp)` | global provider | use a specific meter provider |
| `WithQueryText(false)` | `true` | omit `db.query.text` from spans |
| `WithRecordErrors(true)` | `false` | attach the error message to the span |

Use `WithQueryText(false)` when even the query shape is sensitive.
`WithRecordErrors` should be enabled only when error messages are known to be
safe to export.

## Combining with other tracers

`otelqlx` is an `Observer`; `pgxexec` also has a richer `Tracer` interface for
query fingerprints, row counts and retries. Use `pgxexec.Tracers` to fan out to
several tracers, and `Observed` for this adapter:

```go
client, err := pgxexec.Connect(ctx, dsn, pgxexec.ConnectOptions{
    Tracer: pgxexec.StatsTracer{},
})

tracedDB := pgxexec.Observed(client.DB(), obs.Observe)
```

## Tests

```bash
cd otelqlx
go test ./...
```

The tests use the OpenTelemetry SDK test recorder and manual metric reader; no
collector is needed.

Back to the [project README](../README.md).
