# QLX — AI Agent Guidelines & Context

This document is the authoritative reference for AI coding assistants (Claude Code, Cursor, Windsurf, Copilot, Antigravity, etc.) generating Go code using **QLX** (`github.com/noobmaisfier/qlx`).

---

## 1. Core Mental Model & Invariants

1. **Schema-First & Typed Tables**:
   Tables and columns are generated Go values (`db.Users`, `db.Orders`). Each column is typed and only exposes methods valid for its PostgreSQL data type. Views and materialized views automatically infer `NOT NULL` for columns proven non-null by their query tree (or via manual `not_null: [view.col]` in `qlx.yaml`).
2. **Zero Raw SQL Strings**:
   There is **NO** `Where("age > ?", 18)` or string concatenation. Every expression, operator, and predicate is a Go function/method.
3. **Values vs Expressions (`X` suffix)**:
   * **Value methods** take Go values and render as `$n` parameters: `.Eq(v)`, `.Ne(v)`, `.Gt(v)`, `.Lt(v)`, `.To(v)`.
   * **Expression methods** (`...X`) take columns or subqueries and render as SQL identifiers/expressions: `.EqX(otherCol)`, `.NeX(...)`, `.GtX(...)`, `.ToX(expr)`.
4. **Safety & Analyzer**:
   * `Update` or `DeleteFrom` without `.Where(...)` will be rejected by the analyzer unless `.All()` is explicitly called.
   * `ToNull()` is for nullable columns only.
5. **Zero Reflection Row Scanning**:
   Use `//qlx:row` on structs and run `qlx rows ./...` to generate `ScanTargets()` for reflection-free row scanning into positional fields.

---

## 2. Essential Imports

```go
import (
    q "github.com/noobmaisfier/qlx"
    "github.com/noobmaisfier/qlx/qt"
    "github.com/noobmaisfier/qlx/pgxexec"
    "github.com/noobmaisfier/qlx/qjson"
    "github.com/noobmaisfier/qlx/qjson/validator"
    "example.com/app/db" // Generated tables (e.g. Users, Orders)
)
```

---

## 3. Types: `qt` Package & Validation

Never use Go pointers (`*string`) to model SQL NULL or dynamic parameters:

| Type | Purpose | JSON Format / Marshaling |
|---|---|---|
| `qt.Null[T]` | Represents **SQL NULL** in database columns and results | `"val"` or `null` (zero wrapper struct in JSON) |
| `qt.Opt[T]` | Represents **Presence / Absence** of a query parameter / filter | `"val"` or omitted with `json:",omitzero"` |
| `qt.Opt[qt.Null[T]]` | PATCH updates: Unset = leave untouched; Some = update value; None = set NULL | Direct three-state HTTP PATCH support |
| `qt.Date` | PostgreSQL `date` | Pure ISO date string: `"2026-03-30"` |
| `qt.TimeOfDay` | PostgreSQL `time without time zone` | Pure time string: `"14:30:00.000000"` |
| `qt.Timestamptz` | PostgreSQL `timestamptz` (microsecond ISO 8601 UTC) | `"2026-03-30T14:30:00Z"` or `"infinity"` |
| `qt.Interval` | PostgreSQL `interval` | ISO 8601 Duration: `"P1Y2M3DT1H"` |
| `qt.ID[Space]` | Scrambled, typed prefixed ID | Scrambled public string: `"usr_8v92kf9"` (DB holds `int64`) |
| `qt.Numeric` | PostgreSQL `numeric` / `decimal` | Exact decimal string: `"123.45"` (no float drift) |
| `qt.Vector` | `pgvector` embedding (`[]float32`) | Numbers array: `[0.1, 0.45, 0.89]` |
| `qt.JSONB[T]` | Strong-typed JSONB column | Marshals/unmarshals `T` directly |
| `qt.Range[T]` | Range types (`int4range`, `daterange`, `tstzrange`) | Object: `{"lower":1,"upper":5,"lower_inc":true,"upper_inc":false}` |
| `db.EnumName` | PostgreSQL ENUM (`CREATE TYPE ... AS ENUM`) | String: `"pro"` (typed constants `db.UserTierPro`, validates with `.Valid()`) |

### PostgreSQL ENUMs
Columns referencing a PostgreSQL ENUM generate custom types in `db/enums.go`:
```go
// Generated:
// type UserTier string
// const ( UserTierFree UserTier = "free", UserTierPro UserTier = "pro" )
// var UserTierValues = []UserTier{...}
// func (v UserTier) Valid() bool

// Usage in queries:
Where(Users.Tier.Eq(db.UserTierPro))
// If nullable:
Users.Tier.Set(qt.Some(db.UserTierPro))
```

### DTO Decoding & Validation (`qjson`)
```go
// 1. Strict Decoding: refuses unknown fields & bounds body size
req, err := qjson.Decode[CreateUserDTO](r)

// 2. Validation with QLX rules:
// - "provided": checks Opt[T] was sent in JSON
// - "notnull": checks Null[T] is not SQL NULL
// - "nonzero": checks qt.Date / qt.ID / qt.Range are not zero struct
v := qvalidator.New()
err := v.Validate(req)
```

---

## 4. Query Patterns

### 4.1 SELECT & Scanning

```go
// 1. One scalar value
count, err := client.One[int64](ctx,
    q.Select(q.CountAll()).From(Users).Where(Users.Tier.Eq(db.UserTierPro)))

// 2. Positional scanning with //qlx:row (Zero reflection)
//qlx:row
type UserSummary struct {
    ID    int64
    Email string
    Total int64
    Note  string `qlx:"-"` // Ignored by scanner
}

users, err := client.All[UserSummary](ctx,
    q.Select(
        Users.ID,
        Users.Email,
        q.Count(Orders.ID).As("total"), // Alias matches struct field
    ).
    From(Users).
    LeftJoin(Orders).On(Orders.UserID.EqX(Users.ID)).
    Where(Users.Tier.Eq(db.UserTierPro)).
    GroupBy(Users.ID, Users.Email).
    OrderBy(Users.CreatedAt.Desc()).
    Limit(20).Offset(40))

// 3. Single row (returns pgx.ErrNoRows if missing)
user, err := client.One[UserSummary](ctx, query)

// 4. Optional single row: returns (qt.Opt[R], error)
optUser, err := client.Maybe[UserSummary](ctx, query)
if optUser.Set {
    fmt.Println(optUser.V.Email)
}

// 5. Streaming rows with Go 1.23 iterator
for user, err := range client.Each[UserSummary](ctx, query) {
    if err != nil { break }
    fmt.Println(user.Email)
}
```

### 4.2 Dynamic Filters (`...If`)

Never build dynamic queries by reassigning `query = query.Where(...)` in multiple `if` branches. Use QLX's `...If` helpers with `qt.Opt[T]`:

```go
filter := struct {
    Tier   qt.Opt[string]
    MinBal qt.Opt[int64]
    MaxBal qt.Opt[int64]
    Search qt.Opt[string]
}{
    Tier:   qt.Of("pro"),
    Search: qt.Unset[string](),
}

users, err := client.All[UserSummary](ctx,
    q.Select(Users.ID, Users.Email).
    From(Users).
    Where(
        Users.Tier.EqIf(filter.Tier),
        Users.Balance.BetweenIf(filter.MinBal, filter.MaxBal),
        Users.Email.ContainsFoldIf(filter.Search), // Empty string is also treated as unset
    ))
```

Available `...If` operators: `EqIf`, `NeIf`, `GtIf`, `GteIf`, `LtIf`, `LteIf`, `BetweenIf`, `InIf`, `IsNullIf`, `ContainsIf`, `ContainsFoldIf`, `HasPrefixIf`, `LikeIf`, `ILikeIf`.

### 4.3 Slices & ANY

Pass Go slices directly using `.Any(...)`:
```go
ids := []int64{1, 2, 3}
q.Select(Users.ID).From(Users).Where(Users.ID.Any(ids)) // Renders: "users"."id" = ANY($1)
```

### 4.4 Joins & Aliases

```go
// Standard Join
q.Select(Users.Email, Orders.Total).
    From(Users).
    Join(Orders).On(Orders.UserID.EqX(Users.ID)) // Use EqX for column-to-column!

// Self-join with Aliases
u1, u2 := Users.As("u1"), Users.As("u2")
q.Select(u1.Email, u2.Email).
    From(u1).
    Join(u2).On(u1.ManagerID.EqX(u2.ID))

// Join with Foreign Key metadata
q.Select(u1.Email, o.Total).
    From(u1).
    Join(o).OnFK(o.FKOrdersUserIDFkey(u1))
```

### 4.5 INSERT, Upsert & Returning

```go
// Single row insert with RETURNING
id, err := client.One[int64](ctx,
    q.InsertInto(Users).
        Row(
            Users.Email.To("alice@example.com"),
            Users.Tier.To("pro"),
            Users.Nickname.ToNull(), // Or To(qt.None[string]())
        ).
        Returning(Users.ID))

// Upsert (ON CONFLICT DO UPDATE)
_, err := client.Exec(ctx,
    q.InsertInto(Users).
        Row(Users.Email.To("bob@example.com"), Users.Balance.To(100)).
        OnConflict(Users.Email).
        DoUpdate(Users.Balance.ToX(Users.Balance.Add(100))))

// Bulk insert (directly on Client)
rows := []qlx.RowAssigner{
    Users.Row(Users.Email.To("a@example.com"), Users.Tier.To("pro")),
    Users.Row(Users.Email.To("b@example.com"), Users.Tier.To("free")),
}
ids, err := client.InsertManyReturning[int64](ctx,
    q.InsertInto(Users).Returning(Users.ID), rows, pgxexec.BulkOptions{})
```

### 4.6 UPDATE & DELETE (Safety first)

```go
// Safe Update
_, err := client.Exec(ctx,
    q.Update(Users).
        Set(
            Users.Balance.ToX(Users.Balance.Sub(10)),
            Users.Tier.To("enterprise"),
        ).
        Where(Users.ID.Eq(42)))

// Partial update with Patch struct
patch := db.UserPatch{
    Tier:    qt.Of("enterprise"),
    Balance: qt.Of(int64(500)),
}
_, err := client.Exec(ctx,
    q.Update(Users).
        SetPatch(patch).
        Where(Users.ID.Eq(42)))

// Safe Delete
_, err := client.Exec(ctx,
    q.DeleteFrom(Orders).
        Where(Orders.Status.Eq("cancelled")))

// Explicit full-table Delete (required to bypass analyzer safety)
_, err := client.Exec(ctx, q.DeleteFrom(Logs).All())
```

### 4.7 Keyset Pagination (Fast cursor pagination)

```go
byDate := q.KeyDesc(Users.CreatedAt)
byID   := q.KeyAsc(Users.ID)

var after q.Pred
if lastRow != nil {
    after = q.After(byDate.After(lastRow.CreatedAt), byID.After(lastRow.ID))
}

users, err := client.All[UserSummary](ctx,
    q.Select(Users.ID, Users.Email, Users.CreatedAt).
    From(Users).
    Where(after). // Cleanly omitted if empty
    OrderBy(q.KeyOrder(byDate, byID)...).
    Limit(20))
```

### 4.8 Safe URL Sorting

```go
var userSorts = q.NewSorts(map[string]q.Orderable{
    "email":   Users.Email,
    "created": Users.CreatedAt,
}).Default(Users.CreatedAt.Desc()).Tiebreak(Users.ID.Asc())

order, err := userSorts.Parse(r.URL.Query().Get("sort")) // e.g. "-created,email"
if err != nil { /* invalid sort parameter */ }

q.Select(Users.ID).From(Users).OrderBy(order...)
```

### 4.9 Transactions with Retries

```go
err := client.InTx(ctx, pgx.TxOptions{}, pgxexec.Retry{
    Attempts: 3,
}, func(ctx context.Context, tx *pgxexec.Tx) error {
    _, err := tx.Exec(ctx,
        q.Update(Users).
            Set(Users.Balance.ToX(Users.Balance.Sub(10))).
            Where(Users.ID.Eq(sourceID)))
    if err != nil {
        return err // Rolls back
    }
    return nil // Commits
})
```

---

## 5. Extensions (`qext/*`)

### Full-Text Search (`qext/textsearch`)
```go
import "github.com/noobmaisfier/qlx/qext/textsearch"

doc := textsearch.Document("english",
    textsearch.Weighted(textsearch.A, Articles.Title),
    textsearch.Weighted(textsearch.B, Articles.Body))
query := textsearch.Web("english", q.Val(userInput))

articles, err := client.All[ArticleRow](ctx,
    q.Select(Articles.ID, textsearch.Rank(doc, query).As("rank")).
    From(Articles).
    Where(textsearch.SearchIf(doc, "english", optInput)).
    OrderBy(textsearch.Rank(doc, query).Desc()))
```

### Vector Search (`qext/pgvector`)
```go
import "github.com/noobmaisfier/qlx/qext/pgvector"

target := qt.Vector{0.05, 0.49, 0.86, /* ... */}

items, err := client.All[ItemMatch](ctx,
    q.Select(Items.ID, pgvector.Cosine(Items.Embedding, target).As("distance")).
    From(Items).
    OrderBy(pgvector.Cosine(Items.Embedding, target).Asc()).
    Limit(10))
```

---

## 6. What NOT to Do (Anti-Patterns & Compiler Traps)

1. **DO NOT pass a column to `.Eq()`**:
   * WRONG: `Orders.UserID.Eq(Users.ID)`
   * RIGHT: `Orders.UserID.EqX(Users.ID)`
2. **DO NOT pass raw strings to `.Where()`**:
   * WRONG: `q.Select().Where("id = 1")`
   * RIGHT: `q.Select().Where(Users.ID.Eq(1))`
3. **DO NOT use `*string` or `*int` for SQL NULLs**:
   * WRONG: `type Row struct { Nick *string }`
   * RIGHT: `type Row struct { Nick qt.Null[string] }`
4. **DO NOT run `Update` or `DeleteFrom` without `Where` or `All`**:
   * WRONG: `q.Update(Users).Set(Users.Tier.To("free"))` (Fails analyzer V002)
   * RIGHT: `q.Update(Users).Set(Users.Tier.To("free")).All()`
5. **DO NOT forget `.As("name")` on computed columns**:
   * WRONG: `q.Select(q.CountAll())` mapped into a struct field `Count`
   * RIGHT: `q.Select(q.CountAll().As("count"))`
6. **DO NOT forget `qlx rows ./...`**:
   * Whenever adding a `//qlx:row` struct, the code generator must run to generate `ScanTargets`.
7. **DO NOT use `required` for `qt.Date`, `qt.ID`, `qt.Range` in validation**:
   * Use `nonzero` instead, as zero-value structs pass Go validator's `required` tag.
