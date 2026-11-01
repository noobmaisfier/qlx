# Cookbook

Recipes by SQL feature. Every snippet uses the fixture schema (`users`, `orders`) and the dot-imported generated package; `q` is the `qlx` package, `pgxexec` runs statements. The matching SQL is in the comment. The full list of syntax forms is in [sql-to-go.md](sql-to-go.md).

## Write queries naturally: the static path is automatic

```go
users, err := client.All[UserRow](ctx,
    q.Select(Users.ID, Users.Email).From(Users).Where(Users.Tier.Eq(tier)))
```

Nothing to declare. The first call with a given *shape* renders it (validation included); later calls find the SQL by hashing what rendering would write and only collect the values (`qlx.ShapeCache`, `client.Shapes.Stats()`). A dynamic filter that adds or drops predicates produces a few shapes, each rendered once. Two things keep the shape count bounded: pass lists as `Expr.Any(slice)` (one shape for any length) and do not build selectable columns or joins from user input per call. A growing `Evictions` counter is the signal.

## Explicit statements for the hottest paths

```go
var userByID = q.Register("user_by_id", q.Tagged("user_by_id",
    q.Select(Users.ID, Users.Email).From(Users).Where(Users.ID.EqX(idParam))))
st, _ := client.Render(userByID)                    // once, at startup
row, err := client.One[UserRow](ctx, st, idParam.Bind(42)) // no AST walk at all
```

`Register` names the statement for snapshots and `qlx prepare`; `Tagged` puts `/* qlx: user_by_id */` in front so logs and `pg_stat_statements` show the name. `NewStatic0..4` give typed handles.

## Filtering and dynamic conditions

```go
sel := q.Select(Users.ID).From(Users)
if tier != "" {
    sel = sel.Where(Users.Tier.Eq(tier))      // WHERE tier = $1
}
sel = sel.Where(Users.Email.NotLike("%@spam.test")) // AND email NOT LIKE $2
```

Builders are immutable: always assign the result (the analyzer flags a discarded one, V001). `Eq`, `Gt`, `Between`, `NotBetween`, `BetweenSymmetric`, `In`, `Like`, `ILike`, `SimilarTo`, `LikeEscape`, `IsNull`, `IsDistinctFrom` exist; `EqIf(qt.Opt)` skips the predicate for an absent optional filter.

## Joins

```go
u, o := Users.As("u"), Orders.As("o")
q.Select(u.Email, o.Total).From(u).
    Join(o).On(o.UserID.EqX(u.ID))            // JOIN orders o ON o.user_id = u.id
q.Select(u.Email).From(u).Join(o).OnFK(o.FKOrdersUserIDFkey(u))  // the foreign key names the columns
```

`OnFK` uses the generated foreign-key method, so renaming a column breaks the build instead of the query.

## Aggregates, GROUP BY, HAVING

```go
q.Select(Users.Tier, q.CountAll().As("n")).From(Users).
    GroupBy(Users.Tier).Having(q.CountAll().Gt(10)) // GROUP BY tier HAVING count(*) > $1
```

Selecting `Users.Email` next to `CountAll()` without `GroupBy` is rejected at render time and by the analyzer (V007). `ROLLUP`, `CUBE` and `GROUPING SETS` are `q.Rollup`, `q.Cube`, `q.GroupingSets`.

## Window functions

```go
rank := q.RowNumber().Over(q.Window().PartitionBy(Orders.UserID).OrderBy(Orders.Total.Desc()))
q.Select(Orders.ID, rank.As("rn")).From(Orders)
```

On PostgreSQL 19 `Lag(...).IgnoreNulls()` renders `IGNORE NULLS`; on 18 it is a render error that names the version.

## CTEs, recursion, SEARCH and CYCLE

```go
big := q.NewCTE("big", q.Select(Orders.UserID).From(Orders).Where(Orders.Total.Gt(1000)))
sel := q.Select(Users.Email).From(Users).With(big)   // WITH big AS (SELECT ...) SELECT ...
tree := q.Recursive("tree", anchor, func(self q.CTE) q.Query { return step(self) }).
    SearchDepthFirst("ord", byColumns...)             // WITH RECURSIVE ... SEARCH DEPTH FIRST
```

`Materialized()` / `NotMaterialized()` and `Cycle(mark, path, cols...)` complete the CTE options.

## Subqueries

`q.Exists(sub)`, `q.InSelect(expr, sub)`, `q.NotInSelect(expr, sub)`, `q.AnySelect(expr, q.OpEq, sub)`, `q.AllSelect`, `q.ArrayFromQuery(sub, col)`, `q.Scalar(sub, col)` and `q.Lateral(sub, "alias")` cover `EXISTS`, `IN`, `NOT IN`, `= ANY (SELECT ...)`, `ARRAY(SELECT ...)`, scalar subqueries and `LATERAL`.

## INSERT, upsert, RETURNING

```go
q.InsertInto(Orders).
    Row(Orders.UserID.To(7), Orders.Total.To(100), Orders.Status.To("new")).
    OnConflict(Orders.ID).DoUpdate(Orders.Total.ToX(Orders.Total.Excluded())).
    Returning(Orders.ID)
```

Leaving out a required column (`NOT NULL` without default) fails at render time and in the analyzer (V008). `RETURNING OLD/NEW` is `Column.Old()` / `Column.New()` on PostgreSQL 18; `ON CONFLICT DO SELECT` is `DoSelect()` on 19.

## UPDATE and DELETE

```go
q.Update(Orders).Set(Orders.Status.To("paid")).Where(Orders.ID.Eq(9))
q.DeleteFrom(Orders).Where(Orders.UserID.Eq(7))
q.Update(Orders).Set(Orders.Status.To("void")).All()   // every row, said out loud
```

An UPDATE or DELETE with neither `Where` nor `All()` does not render.

## MERGE

`q.Merge(Target).Using(src).On(pred)` then `.WhenMatched(cond...).ThenUpdate(sets...)` and the not-matched forms; `q.MergeAction()` reports the action in RETURNING. The merge tests show each form.

## JSON and SQL/JSON

`q.JSONObject`, `q.JSONArray`, `q.JSONExists`, `q.JSONValue[T]`, `q.JSONQuery`, `q.JSONTable` build the standard constructors and `JSON_TABLE`; typed JSONB columns are `qt.JSONB[T]` (set `jsonTypes` in qlx.yaml to map a column to your struct).

## Scanning results

```go
rows, err := client.All[UserRow](ctx, st)           // []UserRow
one, err := client.One[UserRow](ctx, st)            // exactly one row, else ErrNoRows / ErrTooManyRows
opt, err := client.Maybe[UserRow](ctx, st)          // qt.Opt[UserRow]
for row, err := range client.Each[UserRow](ctx, st) { ... } // streaming
```

Row types are generated by `qlx rows`; single-column results scan into scalars.

## Transactions and retries

```go
err := pgxexec.InTx(ctx, client.Pool, pgx.TxOptions{IsoLevel: pgx.Serializable}, pgxexec.Retry{Attempts: 5},
    func(ctx context.Context, tx pgx.Tx) error { ... })
```

The function may run more than once on a serialization failure or deadlock: keep it free of side effects outside the transaction.

## Pipelines, batches, COPY, LISTEN

`pgxexec.Pipeline` sends several statements in one round trip and returns typed futures (`PipeAll`, `PipeOne`, `PipeExec`); `BatchAll` / `BatchOne` read `pgx.Batch` results; `pgxexec.CopyIn` uses the binary protocol; `ListenChan` yields notifications and reconnects with backoff.

## Row-level security tenants

`q.SetConfig("app.tenant", id, true)` is `set_config` with bound values, so the tenant id is never spliced into SQL.

## Plan advice (PostgreSQL 19)

```go
adv := q.Advice(q.JoinOrder(q.Rel(q.On(u)), q.Rel(q.On(o))), q.HashJoin(q.On(o)))
client.Exec(ctx, mustRender(q.LoadPlanAdvice()))
client.Exec(ctx, mustRender(adv.Set(true)))
```

Targets are tables of the query, not hand-typed names; `q.Explain(sel, q.PlanAdvice)` prints the advice the planner would generate.

## Functions and operators from the catalog

Every built-in function and operator is a typed wrapper: `text.Lower`, `num.AbsInt8`, `array.ArrayAppend` under `pgcat/<area>`, or the flat `pg` facade (`pg.Lower`). Nullable-argument overloads end in `N`. Extension packs live under `pgcat/ext/<pack>` (hstore, ltree, citext, pg_trgm).

What the wrappers give you, by shape:

```go
// Typed aggregate with FILTER / DISTINCT / ORDER BY / OVER (AggExpr)
q.Select(num.StddevInt8(Orders.Total).Filter(Orders.Status.Eq("paid"))).From(Orders)

// Variadic and void functions
q.Select(text.Concat(Users.Email, q.Val("-x")))           // Expr[qt.Null[string]]
q.Select(sys.PgAdvisoryLockInt8(q.Val(int64(42))))        // Expr[qt.Void]: run it, nothing to read

// Set-returning function as a row source: you name the column and its type
x := q.NewField[int32]("x")
s := num.GenerateSeriesInt4Int4(q.Val(int32(1)), q.Val(int32(3))).As("s", x)
q.Select(q.RefNum(s, x)).From(s)

// Function with OUT columns: typed columns come with the wrapper (all nullable)
cols := sys.PgGetKeywordsCols
e := sys.PgGetKeywords().As("e", cols.Fields()...)
q.Select(q.Ref(e, cols.Word), q.Ref(e, cols.Catcode)).From(e)

// Text-form types: jsonpath, "char", snapshots, regclass ...
jsonb.JsonbPathExists(doc, q.Val(qt.JSONPath("$.a[*] ? (@ > 1)")), vars, q.Val(true))
fts.SetweightTsvectorChar(vec, q.Val(qt.Char('A')))
```

Results that the catalog marks STRICT can still be NULL (a missing JSON key, an unbounded range bound): those wrappers return `qt.Null[T]`. An aggregate over no rows is NULL, except `count`.

## Utility commands

`q.Listen`, `q.Notify`, `q.SetConfig`, `q.SetRole`, `q.Reset`, `q.Show`, `q.DiscardPlans`, `q.SetConstraints`, `q.Savepoint`, `q.Vacuum`, `q.Truncate`, `q.LockTable`, `q.RefreshMaterializedView`, `q.Reindex`, `q.Cluster`, `q.Grant`, `q.Revoke` are `Command`s: render and execute them like any statement.

## Dates, times and typed ids

`date`, `time` and `timetz` columns are `qt.Date`, `qt.TimeOfDay` and `qt.TimeTZ`. They are plain comparable values:

```go
d := qt.DateOf(2026, time.October, 28)
d.AddDays(30).Before(qt.Today(time.UTC))           // calendar arithmetic, no zone
d.Time()                                           // midnight UTC, when a time.Time is needed
qt.DateFromTime(t)                                 // drop the clock of a time.Time
qt.TimeOfDayOf(13, 45, 0, 0).On(d, loc)            // combine into a time.Time
Posts.PostedOn.Gte(d)                              // a TimeCol, like any timestamp column
```

`infinity` and `-infinity` are `qt.DateInfinity` and `qt.DateNegInfinity`. To keep `time.Time` for dates, set `date: stdlib` in `qlx.yaml`.

### Opaque ids

With `ids: string` every single-column integer primary key, and every foreign key that points at one, gets its table's id type:

```yaml
ids: string
```

```go
qt.MustSetIDKey(secret)                  // once at startup: 16+ random bytes, kept stable

p, _ := c.One[blog.Post](ctx, query)   // p.ID is blog.PostID, p.AuthorID is blog.AuthorID
b, _ := json.Marshal(p)                  // {"id":"7mQp3VxK9cE","author_id":"Ck4RtYw2nHa",...}

id, err := qt.ParseID[blog.PostsIDSpace](r.PathValue("id"))   // typo or forged string: qt.ErrBadID
Posts.ID.Eq(id)                          // the database still compares integers
Posts.AuthorID.Eq(id)                    // does not compile: an author id is not a post id
```

The conversion is automatic wherever Go meets text (`encoding/json`, `encoding.TextUnmarshaler` in routers and flag parsing, `fmt`, `slog`), and the database never sees a string. `id.Int64()` is the integer, `qt.IDOf[Space](n)` wraps one. A primary key that is also a foreign key (a one-to-one extension table) shares the space of the table it references.

The text form hides numbering and rejects mistakes; it is obfuscation, not encryption. Keep authorisation checks. Changing the key changes every text form already handed out (never the stored integers), so treat it like any long-lived secret. Domain: 0 up to 2^48 - 1.
