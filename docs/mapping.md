# Mapping rows to Go types

This page is about the type `R` in `client.One[R]`, `All[R]`, `Maybe[R]` and `Each[R]`: how a result row becomes a Go value, which mistakes are caught before the statement runs, which ones are not, and how to write structs of your own for joins, aggregates and nested data.

Everything marked **Tested** is exercised by `TestMappingCases` in `pgxexec/mapping_test.go` against a real PostgreSQL server. The analyzer examples come from `analyzer/qlxvet/testdata/src/mapping`. Other claims are checked by the tests named in the text.

## 1. The short version

| You have | Use for `R` | Checked by |
| --- | --- | --- |
| a whole table row | the generated struct (`db.User`) | the generator; nothing to write |
| a few columns, a few call sites | a struct of your own marked `//qlx:row`, then `qlx rows` | analyzer V018 by position and name, `VerifyRow` |
| one column | a scalar: `int64`, `string`, `qt.Null[T]`... | analyzer V018, `VerifyRow` |
| two or three columns, used once | `q.Tuple2`, `q.Tuple3` | analyzer V018, `VerifyRow` |
| a response with a shape of its own (joins, aggregates) | a struct with `db` tags, read by name | analyzer V018 by name, `VerifyRow` |
| a parent with its children | a JSON column read into `qt.JSONB[[]Child]`, or two queries | see section 7 |

Two rules cover most of the pain:

1. **A result type is not tied to its query by the compiler.** `client.One[R](ctx, stmt)` compiles for any `R`. The analyzer, `pgxexec.VerifyRow` in a test, and the first execution are what check it. Section 9 lists them in order.
2. **A column that can be NULL must be read into a type that can hold NULL**: `qt.Null[T]`, a pointer, a slice, or an `sql.Scanner`. This includes the optional side of an outer join, which the schema declares `NOT NULL`.

## 2. The three modes

`R` is read in one of three modes, tried in this order:

| Mode | When | Rule |
| --- | --- | --- |
| Position | `*R` has a `ScanTargets()` method: generated structs, `//qlx:row` structs, `q.Tuple1..8` | column *i* goes into entry *i* of the list; same count |
| Name | `R` is a struct without `ScanTargets` | the column name, lowercased, is compared to the `db` tag, otherwise to the **lowercased** field name |
| Scalar | `R` is not a struct, or is a struct that is one value (`time.Time`, `qt.Null[T]`, `netip.Addr`) | the result has exactly one column |

Position needs no reflection and no cache. Name builds a plan from the type and the list of column names, once, and keeps it (`planCache` in `pgxexec/run.go`): the plan stores each field's byte offset, so reading a row is a loop of pointer writes. The first execution of a given (type, columns) pair pays for the reflection; the rest do not.

## 3. Reading by name

```go
type UserRow struct {
	ID    int64           `db:"id"`
	Email string          `db:"email"`
	Nick  qt.Null[string] `db:"nickname"`
}
rows, err := pgxexec.All[UserRow](ctx, db, stmt)
```

**Tested** (`by name`). The rules that surprise people:

- **The Go name is not converted to snake_case.** `UserID` is `userid`, not `user_id`. Tag it: `UserID int64 `db:"user_id"``. Generated structs carry the tags.
- **A column without a field is an error**, and so is **a field without a column**. **Tested** (`column without field`, `field without column`):

  ```
  pgxexec: noNick has no field for column nickname (tag a field with `db:"nickname"`)
  pgxexec: field extra.posts has no column in the result (tag it `db:"-"` to skip it)
  ```

  Nothing is dropped silently and no field is left at zero without you knowing.
- **Two columns with the same name are an error.** A join that selects `users.id` and `orders.id` returns two columns called `id`. **Tested** (`two columns with the same name`):

  ```
  pgxexec: dup: two result columns are called id, and both would be read into one field (give one an alias with .As("..."))
  ```

  Give one an alias: `Orders.ID.As("order_id")`. (An earlier version of the library read the second `id` over the first without a word.) The analyzer reports it in the source too (V018: `two columns of the query are called "id"`).
- **Unexported fields are ignored; `db:"-"` ignores an exported field.** An embedded anonymous struct without a `db` tag has its fields flattened. **Tested** (`embedded struct is flattened`). With a `db` tag it is a field like any other.
- **A computed column needs an alias.** Without one it is called `count`, `sum`, `coalesce` or `?column?`. Write `.As("total")`.
- **A struct with a single column is also tried as a value.** If its plan fails, a single-column result is scanned into the struct itself, so a typo on a one-field struct gives a pgx error ("cannot scan int8 ... into *T") instead of the clearer "has no field for column". **Tested** (`go field name is not snake_case`). With two or more columns you always get the clear message.

## 4. Reading by position

Position is what generated structs, `//qlx:row` structs and tuples use. It is the fastest mode, and the one where an order mistake does not announce itself.

### 4.1 `//qlx:row`

```go
//qlx:row
type UserSummary struct {
	ID    int64
	Email string
	Posts int64
	Note  string `qlx:"-"` // not a selected column
}
```

`qlx rows` writes `ScanTargets()` for it, in field order. Embedded fields and generic types are rejected, because their order would be a guess. The columns of the `Select` must be in the same order as the fields.

### 4.2 The swap that nothing reports

If two columns have the same Go type and you select them in the other order, the row reads without an error and the values are swapped. **Tested** (`position: same types swapped, no error`): a `ScanTargets` of `(balance, id)` over `SELECT id, balance` returns `A = id, B = balance`.

The analyzer catches it when the statement is written out in the source, by comparing names (V018: `column id (position 1) matches field TwoInts.ID (position 2)`). `VerifyRow` does not: for position it checks the count and that each column can be read into its target, and two `int8` targets can read any `int8`. If a statement is built in a helper the analyzer cannot follow, keep the order obvious or read it by name.

### 4.3 Tuples

```go
t, err := pgxexec.One[q.Tuple2[int64, string]](ctx, db, stmt) // t.V1, t.V2
```

A tuple is a position-mode struct that someone already wrote for you, named `V1..V8`. It is fine for one query, two or three columns, no name worth giving: `INSERT ... RETURNING id, created_at`, `SELECT count(*), max(x)`. It is not a safer type than a `//qlx:row` struct: the compiler does not tie either to the query, and both are checked the same way (V018, `VerifyRow`). Past three or four columns, or as soon as the result crosses a function boundary, `t.V3` tells the reader nothing: write the struct.

Do not remove them. They are the only reflection-free `R` that needs no generated code, and the analyzer and `VerifyRow` treat them like any position type.

**Tested** (`tuple and scalar`): a tuple over the wrong number of columns fails on the first run, with a pgx message, `number of field descriptions must equal number of destinations, got 1 and 2`.

## 5. Scalars

```go
n, err := pgxexec.One[int64](ctx, db, q.Select(q.CountAll()).From(Users))
```

A result that is not a struct must have exactly one column. **Tested**:

```
pgxexec: result type int64 is not a struct, so the query must select exactly one column
```

## 6. What a column can be read into

The full table is in the guide (section 10). The rules that matter:

- **NULL.** A nullable column goes into `qt.Null[T]`, a pointer, a slice or an `sql.Scanner`. A plain `int64` fails on the row that holds a NULL: `cannot scan NULL into *int64`. The generated row types already use `qt.Null[T]` for nullable columns.
- **Aggregates are typed for you.** `q.Max(Users.Balance)` is an `AggExpr[qt.Null[int64]]`, because an aggregate over zero rows is NULL. `q.CountAll()` is not nullable. Use the same type in your struct.
- **Integers read into a string field give the decimal text.** pgx converts. **Tested** (`integer id read into a string field`): an `id` column read into a `string` field gives `"196"`. This matters for hashids: never read an id column into a `string` field and call the result a hashid. Read it into the typed id (`db.UserID`, a `qt.ID`), and convert at the edge (see `docs/dtos.md`, section 4). The analyzer reports it (V018: `an integer column is read into a string field`), for integer and `qt.ID` columns; `VerifyRow` does not, since at run time a text-like field may legitimately hold a uuid or a range. A column of one table's `qt.ID` read into another table's `qt.ID` is reported as well.
- **The pgx read plan can exist and the value can still fail**: an `int8` too large for an `int16`, a non-integer `float8` into an `int64`. No static check sees values.

## 7. Joins

### 7.1 Inner join

```go
type OrderLine struct {
	Email   string `db:"email"`
	OrderID int64  `db:"order_id"`
	Total   int64  `db:"total"`
}
sel := q.Select(Users.Email, Orders.ID.As("order_id"), Orders.Total).
	From(Users).Join(Orders).On(Orders.UserID.EqX(Users.ID))
```

Select a column of each table that you need, give an alias to any name that appears twice (`id`, `created_at`, `status`), and map by name. The names in the struct are the aliases, not the table columns.

### 7.2 Left, right and full joins: the optional side

The optional side of an outer join is declared `NOT NULL` in the schema and is NULL when no row matches. The generated type of `Orders.Total` is `int64`. Wrap it so the Go type says so:

```go
type UserTotal struct {
	Email string         `db:"email"`
	Total qt.Null[int64] `db:"total"`
}
sel := q.Select(Users.Email, q.Nullable(Orders.Total).As("total")).
	From(Users).LeftJoin(Orders).On(Orders.UserID.EqX(Users.ID))
```

`q.Nullable` changes the Go type of the expression and not the SQL. **Tested** (`left join`): reading `Orders.Total` into a plain `int64` fails on the user without orders (`can't scan into dest[1]: cannot scan NULL into *int64`), and with `q.Nullable` into `qt.Null[int64]` it reads.

Who catches the mistake:

| Check | Outer join column read into a plain field |
| --- | --- |
| compiler | no |
| analyzer V018 | yes, when the `Select` and the tables are visible in the source: `the column comes from the optional side of an outer join` |
| `VerifyRow` | **no**: the server's description of the statement does not say that a join makes the column NULL (**Tested**: it is silent). A test with a row that has no match catches it |
| first run on such a row | yes, with the pgx error above |

So the test for a query with an outer join needs a dataset that contains a row with no match.

### 7.3 Self-joins and aliases

`Users.As("m")` returns the table under an alias, with every column rebound. Alias the selected columns as well when the same column of two aliases is selected (`manager.Email.As("manager_email")`), for the same reason as in 7.1.

### 7.4 One to many

There is no automatic nesting: the rows of a join are flat. Three ways, in the order we reach for them:

**Two queries.** The parent rows, then the children `WHERE parent_id = ANY($1)`, grouped in Go. Simple, fast, no row multiplication, and each result type is a plain struct. For lists of more than a handful of parents this is usually the right answer.

**One JSON column.** The database builds the children, one column carries them:

```go
type orderItem struct {
	ID    int64 `json:"id"`
	Total int64 `json:"total"`
}
type UserOrders struct {
	Email  string                         `db:"email"`
	Orders qt.Null[qt.JSONB[[]orderItem]] `db:"orders"`
}
item := q.JSONObject(
    q.JSONKV(q.Val("id"), Orders.ID),
    q.JSONKV(q.Val("total"), Orders.Total)).
    Expr()
sel := q.Select(Users.Email,
	q.JSONArrayAgg(item).Filter(Orders.ID.IsNotNull()).Expr().As("orders")).
	From(Users).LeftJoin(Orders).On(Orders.UserID.EqX(Users.ID)).
	GroupBy(Users.ID)
```

**Tested** (`one to many as one JSON column`). Three details:

- **Add the `Filter(... IsNotNull())`.** Without it, a user without orders still gets one element, because the `LEFT JOIN` produced a row of NULLs and `JSON_ARRAYAGG` aggregates it: `[{"id": null, "total": null}]`, which decodes into a phantom `{0, 0}` item. The test asserts the phantom item without the filter and its absence with it.
- With the filter, an aggregate over zero rows is NULL, so the field is `qt.Null[qt.JSONB[[]orderItem]]`; `.Valid` is false for a user without orders. Treat that as an empty list in the mapper.
- The `json` tags of the child type are what `json_object` keys must match. Nothing checks that statically: the keys are strings. A test with a row catches a typo.

**`ARRAY_AGG` of one column** for a list of scalars: `q.ArrayAgg(Tags.Name)` is an `AggExpr[qt.Null[[]string]]`.

## 8. DML, CTEs and set operations

`INSERT`, `UPDATE`, `DELETE` and `MERGE` with `RETURNING`, and statements with `WITH`, `UNION` and friends, are read exactly like a `Select`: by the columns of the final result. The analyzer follows a `Select` written out in the source; for the others it stays silent, and `VerifyRow` still works, since it only needs the rendered SQL.

## 9. Checking a mapping, from earliest to latest

| Check | Looks at | Misses |
| --- | --- | --- |
| compiler | operand types and `T` versus `qt.Null[T]` inside expressions | the type `R`: it is not tied to the statement |
| `qlx gen`, `qlx check` | schema and generated code out of sync | |
| analyzer V018 | `R` against the items of a `Select` visible in the source: count, names, Go classes, NULL-ability, optional side of an outer join, order | statements built in a loop or across packages |
| `pgxexec.VerifyRow[R]` in a test | any statement, on a real server, **without running it**: count, names, pgx read plan of each column, nullable table columns | an actual NULL from a join or an expression, a swap of same-typed columns |
| `pgxexec.VerifySchema` at startup | the real schema against the generated code | |
| a test with real rows | everything above, plus NULLs and bad values | rows you did not put in the dataset |

### 9.1 How the analyzer does it

V018 reads the call (`pgxexec.One/All/Maybe/Each`, the `*Scan` forms, `PipeAll`/`PipeOne`, `NewStatic0..4`), finds the `Select` (written out, wrapped in `Tagged` or `Register`, held by a package-level variable or by a local variable that is assigned once and only extended with `sel = sel.Where(...)`-style calls, or returned by a helper whose whole body is `return <Select chain>`, in this package or, if the helper is exported, in another one: the analyzer exports the selected columns as a fact), and compares the list of items with `R`:

- `R` with `ScanTargets`: by position. Count, Go class of each field against the column, and the names against the field names to catch a permutation. The `ScanTargets` body (`return []any{&r.A, &r.B}`) is exported as a fact, so generated rows and `TupleN` work across packages.
- another struct: by name, with the rules of section 3.
- anything else: one column.

Its knowledge of Go types is deliberately narrow (bool, integer, float, time, string, byte slice and slice classes, and the types that carry their own wire format such as `qt.Date`, `qt.Interval`, `qt.Range`, `netip.Addr`). A text-like field is never judged against a uuid or range column. **When it does not know, it says nothing**: a missing finding is not a proof. Examples of what it says (`analyzer/qlxvet/testdata/src/mapping`):

```
V018 UserRow scans 2 columns (its ScanTargets) but the query selects 1
V018 column 2 (Users.Nickname) into qlx.Tuple2[int64, string].V2: the column can be NULL
V018 column 2 (Orders.Total) into qlx.Tuple2[string, int64].V2: the column comes from the optional side of an outer join
V018 column id (position 1) matches field TwoInts.ID (position 2)
V018 ByName has no field for column "tier"
V018 int64 is not a struct, so the query must select exactly one column, but it selects 2
```

### 9.2 How `VerifyRow` does it

```go
st, _ := q.Render(dialect, userByID)
if err := pgxexec.VerifyRow[UserRow](ctx, conn, st); err != nil { t.Fatal(err) }
```

1. It **prepares** the statement (`Prepare` with the rendered SQL): the server parses and plans it and describes the result, without running it. The description gives, for each column, its name, its type OID, and for a plain column of a table the table OID and attribute number.
2. It builds the targets `R` would use (position: `ScanTargets`; name: the same plan the run-time mapping builds, so the name errors are the real ones; scalar: one target).
3. For each column it asks pgx for a **scan plan** from the column's OID to the target and reports the columns pgx cannot read (`column "x" (type OID n) cannot be scanned into *T`).
4. For a column that is a plain column of a table, it looks the attribute up in `pg_attribute.attnotnull`. A nullable column read into a plain Go type is reported:

   ```
   column "nickname" is nullable (users) but is read into *string, which cannot hold NULL: use qt.Null or a pointer, or pass pgxexec.AllowNullable() if the query excludes NULLs
   ```

   **Tested** (`VerifyRow sees a nullable table column`). `AllowNullable()` is for a query that filters NULLs out itself.

The server does not describe a computed column (an aggregate, `coalesce`, a cast) as nullable or not, and does not say that an outer join added NULLs; `VerifyRow` is silent on both (**Tested**). That is why the last row of the table is a test with real rows.

### 9.3 The test to write

For each registered query, one test with the production result type, on a database that contains at least one row with a NULL in every nullable column and at least one outer-join row with no match:

```go
func TestRowTypes(t *testing.T) {
	ctx := context.Background()
	conn := testConn(t) // a *pgx.Conn on the seeded test database
	render := func(s q.Statement) q.Stmt {
		st, err := q.Render(q.Dialect{Version: q.PG18}, s)
		if err != nil {
			t.Fatal(err)
		}
		return st
	}
	if err := pgxexec.VerifyRow[UserRow](ctx, conn, render(userByID(7))); err != nil {
		t.Errorf("userByID: %v", err)
	}
	if err := pgxexec.VerifyRow[UserOrders](ctx, conn, render(usersWithOrders())); err != nil {
		t.Errorf("usersWithOrders: %v", err)
	}
	// then run each query once on the seeded rows, including the user without orders
}
```

## 10. The errors you will see

| Message | Meaning | Fix |
| --- | --- | --- |
| `T has no field for column x (tag a field with db:"x")` | a column nobody reads | add the field, or stop selecting the column |
| `field T.f has no column in the result (tag it db:"-" to skip it)` | a field nobody fills | remove it, tag `db:"-"`, or select the column |
| `T: two result columns are called x, and both would be read into one field` | a join selected the same name twice | alias one with `.As("...")` |
| `cannot scan NULL into *int64` | a NULL reached a plain field | `qt.Null[T]`, a pointer, or `q.Nullable` on an outer join |
| `result type int64 is not a struct, so the query must select exactly one column` | a scalar over several columns | select one column, or use a struct |
| `number of field descriptions must equal number of destinations, got 1 and 2` | a position type over the wrong count | fix the select or the struct |
| `cannot scan int8 (OID 20) in binary format into *T` | no read plan; also what a typo on a one-field struct gives | check the type, the tag, the name |
| `no rows in result set` / `too many rows in result set` | `One` with zero or several rows | use `Maybe`, or add `LIMIT 1` or a unique `WHERE` |

## 11. What is not covered

Said plainly, so you know where the test with real rows is the only protection:

- Statements built in loops or from parameters, a variable that is reassigned to another query, and a helper (of this package or another) that builds its Select in several statements or from the caller's column list are invisible to the analyzer; `VerifyRow` still sees them if you call it.
- Same-typed columns in a swapped order, in a statement the analyzer cannot see.
- NULLs produced by outer joins or expressions, for `VerifyRow`.
- JSON keys of a JSON-aggregated child against the child's `json` tags.
- Values: overflow, precision loss, non-integer into integer.
