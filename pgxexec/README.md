# pgxexec

`pgxexec` runs QLX statements on [pgx v5](https://github.com/jackc/pgx). It
turns a rendered statement into typed rows, scalars or tuples, with no
per-row reflection and no SQL strings at the call site.

It is a separate module, so programs that only build SQL do not depend on pgx.

## Install

```bash
go get github.com/noobmaisfier/qlx/pgxexec
```

## Connect

```go
client, err := pgxexec.Connect(ctx, os.Getenv("DATABASE_URL"), pgxexec.ConnectOptions{
    Spec:      &db.Spec, // generated: version targets and table fingerprints
    MinServer: 16,
    AfterConnect: registerMyTypes, // optional: your own codecs
})
if err != nil {
    return err
}
defer client.Close()
```

`Connect` opens a pool, detects the server version, registers range and
multirange codecs, applies the pool mode and checks `Spec.Targets`. Every
statement is then rendered for the version actually running: a construct newer
than the server fails with `qlx.ErrUnsupportedByServer` before a network call.

## Read rows

`R` can be a struct, a generated row type, a `qlx.TupleN` or a scalar.

```go
type UserRow struct {
    ID    int64  `db:"id"`
    Email string `db:"email"`
}

user, err := client.One[UserRow](ctx,
    q.Select(db.Users.ID, db.Users.Email).
        From(db.Users).
        Where(db.Users.ID.Eq(7)))
if pgxexec.IsNoRows(err) {
    return ErrNotFound
}

users, err := client.All[UserRow](ctx,
    q.Select(db.Users.ID, db.Users.Email).
        From(db.Users).
        Where(db.Users.Tier.Eq("pro")).
        OrderBy(db.Users.CreatedAt.Desc()))

count, err := client.One[int64](ctx,
    q.Select(q.CountAll()).From(db.Users))

opt, err := client.Maybe[UserRow](ctx, userByEmail) // qt.Opt[UserRow]
if opt.Set {
    use(opt.V)
}
```

- `One` requires exactly one row and reports `pgx.ErrNoRows` or
  `pgx.ErrTooManyRows` otherwise.
- `Maybe` returns an unset `qt.Opt[R]` for zero rows and an error for more.
- `Each[R]` returns `iter.Seq2[R, error]` and streams rows without a slice.
- Package-level `AllScan`/`OneScan`/`EachScan` are the positional forms (they take a rendered `qlx.Stmt`): the type constraint
  checks at compile time that `R` has `ScanTargets`.

```go
for user, err := range client.Each[UserRow](ctx, listUsers) {
    if err != nil {
        return err
    }
    fmt.Println(user.Email)
}
```

## Write rows

```go
tag, err := client.Exec(ctx,
    q.Update(db.Users).
        Set(db.Users.Balance.ToX(db.Users.Balance.Add(10))).
        Where(db.Users.ID.Eq(7)))

id, err := client.One[int64](ctx,
    q.InsertInto(db.Users).
        Row(db.Users.Email.To("ada@example.com")).
        Returning(db.Users.ID))
```

The same functions exist at package level (`pgxexec.Exec`, `pgxexec.One`, ...)
for any `pgx.Tx`, `*pgx.Conn` or `*pgxpool.Pool`; the `Client` methods are the
common case.

## Transactions

```go
err := pgxexec.InTx(ctx, client.Pool, pgx.TxOptions{}, pgxexec.Retry{
    Attempts: 3,
    Base:     10 * time.Millisecond,
}, func(ctx context.Context, tx pgx.Tx) error {
    upd1, _ := client.Render(q.Update(db.Users).
        Set(db.Users.Balance.ToX(db.Users.Balance.Sub(5))).
        Where(db.Users.ID.Eq(7)))
    if _, err := pgxexec.Exec(ctx, tx, upd1); err != nil {
        return err
    }
    upd2, _ := client.Render(q.Update(db.Users).
        Set(db.Users.Balance.ToX(db.Users.Balance.Add(5))).
        Where(db.Users.ID.Eq(8)))
    _, err := pgxexec.Exec(ctx, tx, upd2)
    return err
})
```

`InTx` retries the whole function on serialization failure (`40001`) and
deadlock (`40P01`); `fn` must be safe to run again. Use
`pgxexec.WithSavepoint` when only part of the transaction should roll back.

## Prepared handles

Static mode renders once and binds per call. Use it on a hot path.

```go
tier := q.NewParam[string]()
byTier := pgxexec.NewStatic1[UserRow, string](client.Dialect,
    q.Select(db.Users.ID, db.Users.Email).
        From(db.Users).
        Where(db.Users.Tier.EqX(tier)),
    tier)

rows, err := byTier.All(ctx, client.DB(), "pro")
```

`NewStatic0..4` cover zero to four parameters; each handle has `One`, `All`,
`Maybe`, `Each`, `Exec` and `Stmt`.

## Batching and pipelining

```go
var p pgxexec.Pipeline
id := q.NewParam[int64]()
user := pgxexec.PipeOne[UserRow](&p, userByID, id.Bind(7))
orders := pgxexec.PipeAll[OrderRow](&p, ordersByUser, id.Bind(7))

if err := p.Run(ctx, client.Pool); err != nil {
    return err
}
u, err := user.Get()
```

The statements go in one round trip inside one implicit transaction; results
come back through typed futures. `pgxexec.Batch` is the lower-level form with
`Queue`, `Send` and `BatchOne`/`BatchAll`.

## Pool modes

| Mode | Use |
| --- | --- |
| `CacheStatements` (default) | prepared statements per connection; best on direct PostgreSQL |
| `CacheDescribe` | unnamed statements with a cached description; safe behind PgBouncer in transaction mode |
| `Simple` | simple protocol; only for poolers that support nothing else |

## Errors

```go
switch {
case pgxexec.IsUnique(err, "users_email_key"):
    // HTTP 409
case pgxexec.IsForeignKey(err, ""):
    // referenced row missing
case pgxexec.IsNotNull(err, "email"):
    // required value missing
case pgxexec.IsSerialization(err):
    // retryable, or let InTx do it
case pgxexec.IsNoRows(err):
    // One found nothing
}
```

Errors keep the SQLSTATE and constraint name. `pgxexec` does not turn them
into HTTP statuses; that mapping belongs to the application.

## Tracing and statistics

`client.DB()` carries the tracer configured in `ConnectOptions`. Use a custom
`Tracer` with `TraceQuery`, or the process-wide `Stats`:

```go
client, err := pgxexec.Connect(ctx, dsn, pgxexec.ConnectOptions{
    Tracer: pgxexec.Tracers{
        pgxexec.StatsTracer{},
        myTracer,
    },
})
```

`Observed(db, obs.Observe)` wraps any `Querier` with the lighter `Observer`
interface. The OpenTelemetry adapter is the separate
[`otelqlx`](../otelqlx/README.md) module.

## Verify against a real server

`VerifyRow` prepares the statement without running it and checks the result
shape and every pgx scan plan:

```go
st, err := client.Render(q.Select(db.Users.ID, db.Users.Email).From(db.Users))
if err != nil {
    return err
}
if err := pgxexec.VerifyRow[UserRow](ctx, conn, st); err != nil {
    return err
}
```

It also reports a nullable table column read into a plain `int64`, `string` or
`bool` when the server can describe the column. Pass `pgxexec.AllowNullable()`
when the query filters NULLs out itself.

`VerifySchema` compares the live database with the fingerprints in generated
code:

```go
err := pgxexec.VerifySchema(ctx, client.Pool, db.Spec, "public.users", "public.orders")
```

## Tests

```bash
cd pgxexec
go test ./...                 # unit tests; server tests skip without a DSN
QLX_TEST_DSN="postgres://..." go test ./...
```

The integration suite is run against PostgreSQL 16–19 by the project's CI.

Limits: `pgxexec` does not build SQL and does not replace the analyzer. Read
[the mapping guide](../docs/mapping.md) for how `R` is chosen, and
[the limits](../docs/limits.md) for what only a real server can check.

Back to the [project README](../README.md).
