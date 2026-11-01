# Migration guides

## From raw pgx

| pgx | QLX |
|---|---|
| `pool.Query(ctx, "SELECT id, email FROM users WHERE id = $1", id)` | `st := q.Select(Users.ID, Users.Email).From(Users).Where(Users.ID.EqX(p))`, then `client.One[UserRow](ctx, st, p.Bind(id))` |
| `rows.Scan(&a, &b)` loops | `All[R]` / `One[R]` / `Maybe[R]` / `Each[R]` with generated row types (`qlx rows`) |
| `pgx.CollectRows(rows, pgx.RowToStructByName[T])` | `All[T]`; the scan plan is cached per (type, column list), no per-row reflection |
| `pgx.Batch` | `pgxexec.Pipeline` with typed futures, or `BatchAll`/`BatchOne` |
| `pool.Exec(ctx, "UPDATE ...")` | `client.Exec(ctx, st)` |
| `pgx.BeginTxFunc` + hand-written retry | `pgxexec.InTx` with `Retry` |
| `conn.WaitForNotification` loop | `pgxexec.ListenChan` |

Migrate one query at a time: QLX runs on the same pgx pool (`client.Pool`), so raw and typed code share connections and transactions.

**Convert SQL automatically:** `qlx sql2go -e 'SELECT id FROM users WHERE id = 1'` prints the QLX code for a statement (build with `-tags sql2go`). Literals become bound parameters.

## From sqlc

sqlc generates one function per `.sql` query. QLX generates the *vocabulary* and you write the queries in Go.

1. Keep your migrations; point `qlx.yaml` at them and run `qlx gen`.
2. Convert each query: paste the SQL into `qlx sql2go` and review the output.
3. Replace `db.GetUser(ctx, id)` with a registered static statement and `client.One[UserRow]`.
4. Keep sqlc for queries you do not want to convert: both use the same pgx pool.

What you gain: dynamic filters without string building, the compiler checking every column, and one place (the schema) that both the types and the queries come from. What you give up: the SQL file as the unit of review. `qtest.Snapshot` writes every registered statement's SQL to a golden file so review can still read SQL.

## From squirrel

| squirrel | QLX |
|---|---|
| `sq.Select("id", "email").From("users")` | `q.Select(Users.ID, Users.Email).From(Users)` |
| `.Where(sq.Eq{"tier": "pro"})` | `.Where(Users.Tier.Eq("pro"))` |
| `.Where(sq.Gt{"total": 5})` | `.Where(Orders.Total.Gt(5))` |
| `sq.And{...}`, `sq.Or{...}` | `q.And(...)`, `q.Or(...)`, `Pred.And`, `Pred.Or` |
| `.Join("orders o ON o.user_id = u.id")` | `.Join(o).On(o.UserID.EqX(u.ID))` |
| `.PlaceholderFormat(sq.Dollar)` | always `$n`, always bound |
| `.ToSql()` | `q.SQL(dialect, stmt)` or `client.Render(stmt)` |
| `sq.Expr("lower(email) = ?", x)` | `Users.Email.Lower().Eq(x)` or a `pg` wrapper; there is no raw escape hatch |

The one habit to drop is the string fragment: every `sq.Expr(...)` becomes a typed call. Where no wrapper exists, the grammar ledger says why (see the not-supported table).
