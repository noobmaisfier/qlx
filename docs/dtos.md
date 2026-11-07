# DTOs with QLX

A DTO is the Go struct of an HTTP request or response. QLX does not need one: a generated row struct can be returned as it is. This page says when that is a good idea, which `qt` types are safe to put in a DTO, and how to write each shape (create, patch, list, relation, error) with an HTTP framework such as Echo.

What is tested and what is not:

- Every snippet marked **tested** is compiled and run by `qt/dto_guide_test.go`, `dto_statements_test.go` (it renders the statements on the fixture schema) or `pgxexec/dto_errors_test.go`. The JSON forms in the tables come from `qt/json_test.go`.
- The Echo, `validator` and `httptest` snippets are marked **not compiled**. They follow the documented behaviour of those libraries; run them in your project before relying on them.
- Column names in the examples are those of the fixture schema (`users`: `id`, `email`, `nickname`, `tier`, `balance`, `created_at`; `orders`: `id`, `user_id`, `total`). Use what `qlx gen` wrote for your own schema.

---

## 1. The short answer

Use `qt` types in your DTOs when `qt` is your vocabulary anyway. The types listed as safe in section 3 have a JSON form that is stable and meant for it. Convert at the edge only for the few that have no good public form (last rows of that table).

Keep three shapes apart even when they look alike today:

| Shape | Direction | Typical content |
| --- | --- | --- |
| create request | in | the required fields of the insert, validated |
| patch request | in | a subset of the columns, each one optional, `null` allowed |
| response | out | what a client may see, never the whole row by reflex |

The reason is not style. A row has every column, including the ones a client must never set (`role`, `tier`, `balance`, `deleted_at`) or see (`password_hash`). A DTO is the allow-list.

---

## 2. The four layers

```
HTTP body ──bind──► request DTO ──validate──► QLX statement ──► pgxexec ──► row struct ──► response DTO ──► JSON
                    (allow-list in)           (typed, checked)              (qlx rows)     (allow-list out)
```

- **Request DTO**: only the fields a client may send. Validation tags live here.
- **QLX statement**: built from the validated values, with the generated insert type (`NewUserInsert(email)`) or patch type (`UserPatch`, section 5).
- **Row struct**: generated; read with `client.One[User]` (or `pgxexec.One[User]`) and friends. Nullable columns are `qt.Null[T]`.
- **Response DTO**: a copy of the fields a client may see.

---

## 3. Which `qt` types are safe in a DTO

JSON forms, as produced by the code in this repository:

| Type | JSON out | JSON in | Use in a DTO |
| --- | --- | --- | --- |
| `qt.Null[T]` | `null` or the value | `null` and an absent key both give NULL; a value gives the value | yes |
| `qt.Opt[T]` | the value; unset is `null` (use `omitzero`) | key present: set. `null`: set only if `T` can hold NULL (section 5) | yes, for patches |
| `qt.ID[S]` | an opaque 11-character string | the same string; a forged one is an error | yes (section 4) |
| `qt.Date` | `"2024-02-29"`, or `"infinity"` | the same | yes |
| `qt.Timestamptz` | `"2024-02-29T01:02:03Z"`, or `"infinity"` | the same | yes; tell clients about `"infinity"` |
| `qt.Numeric` | `"1.50"`, a string | a string | yes: no float rounding |
| `qt.TimeOfDay` | `"01:00:00"` | the same | yes |
| `qt.Interval` | ISO 8601 duration, `"P1Y2M3DT1H"` | the same, and PostgreSQL's own style | yes |
| `qt.Range[T]`, `qt.Multirange[T]` | `{"lower":1,"upper":5,"lower_inc":true,"upper_inc":false}`, `{"empty":true}`, array of those | the same | yes; a bound flagged inclusive on an unbounded side is rejected |
| `qt.JSONB[T]` | the document itself | the document | yes |
| `netip.Addr`, inside `qt.Null` too | `"10.0.0.1"` | the same | yes |
| `qt.Vector` | `[1,2]` | the same | yes |
| `qt.Char` | a number (`97`) | a number | no: convert to `string` |
| `[16]byte` (uuid) | an array of 16 numbers | the same | no: convert to a string, or use a typed id |

Rules that follow:

- A response DTO can hold a `qt.Null[string]` directly: it already writes `null` or the value. Convert to `*string` (`Null.Ptr()`) only if the consumers of the DTO want pointers.
- Never put a generated enum or a text-form type (`qt.Geometry`, `qt.LTree`) in a public contract without deciding that its text form is the contract.
- `qt.Date` and `qt.Timestamptz` write `"infinity"` and `"-infinity"`. A client that parses every timestamp with a date library fails on those rows. If you never store infinities, clients never see them; if you do, document it. Note that mapping generated table columns to `time.Time` (and opting out of `qt.Timestamptz`) requires setting `timestamptz: stdlib` in `qlx.yaml` (by default, columns are `qt.Timestamptz`).

Example, a DTO that mixes several (**tested**, `qt/json_test.go` covers each form):

```go
type SubscriptionResp struct {
	ID        SubscriptionID    `json:"id"`         // "7mQp3VxK9cE"
	Price     qt.Numeric        `json:"price"`      // "19.90"
	Period    qt.Interval       `json:"period"`     // "P1M"
	ValidDays qt.Range[int32]   `json:"valid_days"` // {"lower":1,"upper":31,...}
	StartsOn  qt.Date           `json:"starts_on"`  // "2024-02-29"
	EndsAt    qt.Null[qt.Timestamptz] `json:"ends_at"` // null or "2025-02-28T00:00:00Z"
}
```

---

## 4. Ids

With `ids: string` in `qlx.yaml`, every integer primary key and every foreign key to one is a `qt.ID[S]`: an integer in the database and an opaque 11-character string everywhere else, JSON and URLs included. That is made for DTOs:

```go
type UserResp struct {
	ID    UserID `json:"id"`      // "7mQp3VxK9cE"
	Email string `json:"email"`
}
```

Things to do:

- Call `qt.SetIDKey` once at startup, with at least 16 random bytes that never change. Changing the key changes every string you handed out. Without it, encoding returns `qt.ErrIDKey`.
- Parse path parameters with `qt.ParseID[db.UsersIDSpace](c.Param("id"))`. A mistyped or forged string is `qt.ErrBadID`: answer 404, not 500.
- An id of one table cannot be used where another table's is expected; the compiler says so. A DTO field typed `UserID` therefore cannot be filled with an order id by mistake.

### A plain `string` in the DTO instead

You can keep `qt.ID` out of the DTO and write `ID string `json:"id"``. That is a reasonable design when the DTO package should not import the generated package, or when an OpenAPI generator has trouble with a struct that implements `TextMarshaler`. Do it in one place, the mapper:

```go
func toUserResp(r db.User) UserResp { return UserResp{ID: r.ID.String(), Email: r.Email} }
```

What changes:

- **Never read the id column into the string field.** pgx converts an integer to its decimal text, so `ID string` filled straight from a row holds `"196"`, the raw number, not the hashid (**tested**, `TestMappingCases/integer id read into a string field`). Read the typed id and convert with `String()`.
- `String()` panics when `qt.SetIDKey` was never called (a programming error); `json.Marshal` of a `qt.ID` returns `qt.ErrIDKey` instead. Set the key at startup either way.
- The compiler no longer stops you from putting an order id in a `UserResp`; the type was doing that.
- A request body field typed `string` is not validated at `Bind`. Call `qt.ParseID[db.UsersIDSpace](body.ID)` yourself and answer 400 or 404 on `qt.ErrBadID`; a `qt.ID` field fails in `Bind` already.

If you already write a mapper per resource (which this guide recommends for public APIs), `string` at the edge costs one line. Without a mapper, keep `qt.ID`.

What it is not: the text is a keyed permutation, not encryption. It hides the numbering (no "how many users", no walking `id+1`). **It is not authorization**: check that the caller may access the row.

---

## 5. Patch requests: three states

A PATCH body has three cases per field:

| Body | Meaning | In the DTO |
| --- | --- | --- |
| key absent | leave the column alone | `Opt{}` (unset) |
| `"nickname": null` | set the column to NULL | `Opt[Null[string]]` with `Set` true and a NULL value |
| `"nickname": "ada"` | set the value | `Opt[Null[string]]` with `Set` true and the value |

`qt.Opt` decodes exactly that when its element type can hold NULL (a `qt.Null[T]`, which a nullable column is). For a column that is NOT NULL, `Opt[T]` leaves the field unset on `null`, as `encoding/json` does for a plain value.

**Tested** (`dto_statements_test.go` renders the three cases):

```go
type PatchUser struct {
	Email    qt.Opt[string]          `json:"email,omitzero"`    // NOT NULL column
	Nickname qt.Opt[qt.Null[string]] `json:"nickname,omitzero"` // nullable column
}

func (p PatchUser) Assignments() []q.Assignment {
	return []q.Assignment{
		Users.Email.ToIf(p.Email),
		Users.Nickname.ToIf(p.Nickname),
	}
}

st := q.Update(Users).Set(p.Assignments()...).Where(Users.ID.Eq(id))
```

`ToIf(opt)` assigns the option's value and **leaves the assignment out when the option is unset**: no `SET` entry in an `UPDATE`, no column in an `INSERT`. It replaces the `if p.Email.Set { ... }` block you would otherwise write for every field, and works the same in `Row`, `Set`, `DoUpdate` and MERGE `ThenUpdate`.

| Body | SQL | Arguments |
| --- | --- | --- |
| `{"nickname":null}` | `UPDATE "public"."users" SET "nickname" = $1 WHERE ...` | `[NULL, id]` |
| `{"email":"x@y.z","nickname":"ada"}` | `SET "email" = $1, "nickname" = $2 ...` | `[x@y.z, ada, id]` |
| `{}` | not rendered: every `ToIf` is left out, so the `UPDATE` has no `SET` and fails with `ErrShape` | |

`omitzero` (Go 1.24 and later) with the `IsZero` method of `Opt` drops unset fields when you write the DTO back out. `omitempty` does not work on a struct: it would never omit.

Decide what an empty body means for your API. Usually 400 (nothing to do) or 204. The render error for the empty `UPDATE` is not a user-facing message: check the body first (for example a `Changed()` method on the DTO that tests `Set` on each field).

Each combination of present columns is a different statement shape, and the shape cache keeps each one. With n optional columns there can be 2^n shapes: sixteen for a four-field PATCH, which is nothing; a table with thirty optional columns patched freely would not be. `qlx.DefaultShapes.OnThrash` tells you when the cache starts to evict.

### The generated patch

The generator already writes `UserPatch`, made of `qt.Opt` fields, and `Update(...).SetPatch(p)` applies it. Since `Opt` has JSON methods, `UserPatch` decodes a request body directly. **Do not bind a request straight into the generated patch** unless every column of the table may be changed by that caller: a client could send `"role":"admin"`. This is mass assignment, and the type system cannot know it is wrong.

The safe form is a DTO with only the allowed fields, converted:

```go
func (p PatchUser) ToPatch() UserPatch {
	return UserPatch{Email: p.Email, Nickname: p.Nickname}
}

st := q.Update(Users).SetPatch(p.ToPatch()).Where(Users.ID.Eq(id))
```

(The field names of the generated patch depend on your schema.) The conversion is one line per allowed field, and adding a column to the table never exposes it by itself.

### Read-only fields

A field a client must not change (`id`, `created_at`, `balance`) is simply not in the DTO. Do not accept it and ignore it: a client that sends `"balance": 0` and gets 200 believes it worked. If you want to be strict, decode with `DisallowUnknownFields` so that an unknown key is a 400:

```go
dec := json.NewDecoder(c.Request().Body)
dec.DisallowUnknownFields()
if err := dec.Decode(&body); err != nil { /* 400 */ }
```

---

## 6. Create requests

The generated insert type takes the required columns as constructor arguments, so a missing one is a compile error. What the DTO needs to decide is how each optional field behaves when the client does not send it:

| Column | Absent from the body should mean | DTO field |
| --- | --- | --- |
| nullable, no default | NULL | `qt.Null[T]` |
| has a `DEFAULT` | keep the default | `qt.Opt[T]`, written with `ToIf` |
| NOT NULL, no default | an error | plain `T` with `validate:"required"` |

`qt.Null[T]` cannot tell an absent key from `"x": null`: both give NULL (**tested**, `TestDTOCreateBodyNullVersusPointer`). For a nullable column without a default that is exactly right. For a column with a default it would write NULL over the default (or fail on NOT NULL), so use a `qt.Opt[T]` and `ToIf`: an absent key leaves the column out of the `INSERT`, and PostgreSQL applies the default.

**Tested** (`TestDTOCreateStatement`):

```go
type CreateUser struct {
	Email string          `json:"email" validate:"required,email"`
	Nick  qt.Null[string] `json:"nickname"` // nullable, no default
	Tier  qt.Opt[string]  `json:"tier"`     // has a DEFAULT: absent keeps it
}

func (r CreateUser) Insert() q.InsertQuery {
	return q.InsertInto(Users).Row(
		Users.Email.To(r.Email),
		Users.Nickname.To(r.Nick),
		Users.Tier.ToIf(r.Tier), // absent: the column is left out and takes its DEFAULT
	).Returning(Users.ID, Users.Email, Users.Nickname)
}
```

With the generated insert type the same code is shorter (`NewUserInsert(r.Email).Nickname(r.Nick)`), and the compiler checks that the required columns are there. Use `ToIf` when you write the `Row` yourself.

### `ToIf`, `ToOrDefault` and `ToDefault`

| Method | Option unset | Use it for |
| --- | --- | --- |
| `ToIf(opt)` | the assignment is left out (column omitted in an `INSERT`, untouched in an `UPDATE`) | `INSERT` and `UPDATE` bodies with optional fields |
| `ToOrDefault(opt)` | writes the keyword `DEFAULT` | `INSERT`, in particular **multi-row** inserts: every row keeps the same columns |
| `ToDefault()` | n/a: always writes `DEFAULT` | resetting a column on purpose |

Three rules, enforced by tests and by the analyzer:

- Do not `ToIf` a column that is NOT NULL without a default (`email` above): it can disappear and the statement fails (`ErrShape`; V008 reports it). Use `To`.
- Rows of one multi-row `INSERT` must assign the same columns. Rows whose options differ do not render with `ToIf` (V013 reports it); use `ToOrDefault`.
- `ToOrDefault` in a `SET` list resets an absent field to its default instead of leaving it alone. V027 reports it. In updates, use `ToIf`.

`ToOrDefault` does not reduce the number of shapes (a value renders `$n`, `DEFAULT` renders the keyword in the same position); it makes multi-row inserts valid.

Validation is about the request, the compiler is about the statement: both are needed. The compiler cannot know that an email must contain an `@`.

---

## 7. Responses

**Tested** (`TestDTOResponse`):

```go
type UserResp struct {
	ID        UserID          `json:"id"`
	Email     string          `json:"email"`
	Nickname  qt.Null[string] `json:"nickname"`             // null or the value
	DeletedAt *qt.Timestamptz `json:"deleted_at,omitempty"` // key absent when nil
}

func toUserResp(r User) UserResp {
	return UserResp{ID: r.ID, Email: r.Email, Nickname: r.Nickname, DeletedAt: r.DeletedAt.Ptr()}
}
```

Choose per field whether NULL is `null` (keep `qt.Null[T]`) or an absent key (`*T` with `omitempty`). Pick one convention for the API and keep it.

Returning the row struct itself is acceptable for an internal service. For a public API write the DTO: the row grows with every migration.

A mapper for a slice is one loop; keep it a plain function, not a generic helper, so each resource can differ:

```go
func toUserResps(rows []User) []UserResp {
	out := make([]UserResp, len(rows))
	for i, r := range rows {
		out[i] = toUserResp(r)
	}
	return out // never nil: JSON writes [] and not null
}
```

---

## 8. Lists, filters and pages

A client number must never reach `Limit` unchecked: it is a way to ask for the whole table. Filters come in as pointers (absent means no filter), and the statement is built from what was sent.

**Tested** (`TestDTOListStatement`: no filter gives no `WHERE`, the limit is capped, a negative offset is dropped, the search text is a parameter):

```go
type ListUsers struct {
	Tier   *string `query:"tier"`
	Search *string `query:"q"`
	Limit  int     `query:"limit"`
	Offset int     `query:"offset"`
}

const maxLimit = 100

func (f ListUsers) page() (limit, offset int64) {
	limit = int64(f.Limit)
	if limit <= 0 {
		limit = 20
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	if f.Offset > 0 {
		offset = int64(f.Offset)
	}
	return
}

func (f ListUsers) Statement() q.SelectQuery {
	var preds []q.Pred
	if f.Tier != nil {
		preds = append(preds, Users.Tier.Eq(*f.Tier))
	}
	if f.Search != nil {
		preds = append(preds, Users.Email.ILike("%"+*f.Search+"%"))
	}
	limit, offset := f.page()
	sel := q.Select(Users.ID, Users.Email, Users.Nickname).From(Users)
	if len(preds) > 0 {
		sel = sel.Where(preds...)
	}
	return sel.OrderBy(Users.CreatedAt.Desc(), Users.ID.Desc()).Limit(limit).Offset(offset)
}
```

Two details:

- Sort by a column **and a unique tie-breaker** (`ID`), otherwise rows with the same `created_at` can move between pages.
- The search text is a bound parameter, so it cannot inject SQL. A client typing `%` or `_` still acts as a wildcard in `ILIKE`; escape them if that matters.

`qt.Opt` has JSON methods only, so a framework that binds query strings (Echo does, with the `query` tag) will not fill it: use pointers as above. Columns also have `EqIf(qt.Opt[T])`, and `NeIf`, `InIf`, `GtIf`, `GteIf`, `LtIf`, `LteIf`, `BetweenIf`, `IsNullIf` and the text searches (`ContainsFoldIf`, `ILikeIf`, ...), which drop the predicate when the option is unset (an empty string counts as unset for text searches), if you already hold an `Opt`.

### The response envelope

A list answers with an object, not a bare array, so that you can add fields later without breaking clients:

```go
type Page[T any] struct {
	Items  []T   `json:"items"`
	Limit  int64 `json:"limit"`
	Offset int64 `json:"offset"`
	Total  *int64 `json:"total,omitempty"` // only when asked for: counting is a second query
}
```

Offset paging is simple and is fine for admin screens. For a large table, or a feed that changes while it is read, page by key instead: `WHERE (created_at, id) < ($1, $2) ORDER BY created_at DESC, id DESC LIMIT n`, and return the last row's pair as the cursor. Make the cursor opaque to clients (encode it); a `qt.ID` is already opaque for the id half.

---

## 9. Relations and nested DTOs

A user with their orders: one query with a join, grouped in Go. **Tested** (`TestDTORelationStatement` renders the statement):

```go
st := q.Select(Users.ID, Users.Email, Orders.ID.As("order_id"), Orders.Total).
	From(Users).
	LeftJoin(Orders).On(Orders.UserID.EqX(Users.ID)).
	Where(Users.ID.Eq(id)).
	OrderBy(Orders.ID.Asc())
```

The orders side of a `LEFT JOIN` can be NULL (a user with no orders): wrap it with `q.Nullable(...)` and read into `qt.Null[...]`; the analyzer (V018) reports a joined column read into a plain field. Then:

```go
type OrderResp struct {
	ID    OrderID `json:"id"`
	Total int64   `json:"total"`
}

type UserWithOrdersResp struct {
	UserResp
	Orders []OrderResp `json:"orders"`
}

func group(rows []UserOrderRow) *UserWithOrdersResp {
	var out *UserWithOrdersResp
	for _, r := range rows {
		if out == nil {
			out = &UserWithOrdersResp{UserResp: UserResp{ID: r.UserID, Email: r.Email}, Orders: []OrderResp{}}
		}
		if r.OrderID.Valid {
			out.Orders = append(out.Orders, OrderResp{ID: r.OrderID.V, Total: r.Total.V})
		}
	}
	return out // nil: the user does not exist, answer 404
}
```

Notes:

- The embedded `UserResp` is flattened by `encoding/json`: the JSON is `{"id":...,"email":...,"orders":[...]}`.
- Initialise `Orders` to an empty slice so the JSON is `[]`, not `null`.
- For one-to-many with many parents (a page of users, each with orders), do **not** query per user. Run the page query, collect the parent ids, run one `WHERE user_id = ANY($1)` query for the children, and group by parent id in a map. Two queries, not N+1.

---

## 10. One error shape

Every failing endpoint answers with the same JSON, and a database error never reaches the client as text. **Tested** (`pgxexec/dto_errors_test.go`):

```go
type apiError struct {
	Status int    `json:"status"`
	Code   string `json:"code"`
	Detail string `json:"detail,omitempty"`
}

func toAPIError(err error) apiError {
	switch {
	case err == nil:
		return apiError{}
	case errors.Is(err, qt.ErrBadID), errors.Is(err, pgx.ErrNoRows):
		return apiError{http.StatusNotFound, "not_found", ""}
	case isConstraint(err, "23505"):
		return apiError{http.StatusConflict, "conflict", ""}
	case pgxexec.IsForeignKey(err, ""):
		return apiError{http.StatusUnprocessableEntity, "invalid_reference", ""}
	case pgxexec.IsNotNull(err, ""), pgxexec.IsCheck(err, ""):
		return apiError{http.StatusUnprocessableEntity, "invalid_value", ""}
	}
	return apiError{http.StatusInternalServerError, "internal", ""}
}

func isConstraint(err error, code string) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == code
}
```

`pgxexec.IsUnique(err, constraint)` needs the constraint name, so use it where you want a specific message (`"users_email_key"` becomes "this email is already used") and the code check above as the catch-all. Log the original error (it holds the constraint and the detail) and send only the `apiError`.

A serialization failure or deadlock (`40001`, `40P01`) is retried by `InTx`; if it still reaches the handler it is a 500 or a 503 with a retry hint, never a 409.

---

## 11. Echo

**Not compiled.**

```go
func (h *Handler) Patch(c echo.Context) error {
	id, err := qt.ParseID[db.UsersIDSpace](c.Param("id"))
	if err != nil {
		return h.fail(c, err) // 404 for a forged or mistyped id
	}
	var body PatchUser
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, apiError{http.StatusBadRequest, "bad_body", "invalid JSON"})
	}
	if err := c.Validate(&body); err != nil {
		return c.JSON(http.StatusUnprocessableEntity, apiError{http.StatusUnprocessableEntity, "invalid", err.Error()})
	}
	as := body.Assignments()
	if len(as) == 0 {
		return c.NoContent(http.StatusNoContent)
	}
	st := q.Update(Users).SetPatch(body.ToPatch()).Where(Users.ID.Eq(id)).
		Returning(Users.ID, Users.Email, Users.Nickname)
	u, err := h.db.One[User](c.Request().Context(), st) // h.db is the pgxexec client
	if err != nil {
		return h.fail(c, err)
	}
	return c.JSON(http.StatusOK, toUserResp(u))
}

func (h *Handler) fail(c echo.Context, err error) error {
	c.Logger().Error(err) // the real error stays in the logs
	e := toAPIError(err)
	return c.JSON(e.Status, e)
}
```

Points specific to the framework:

- `Bind` reads JSON with `encoding/json`, so the forms of section 3 apply. It does not reject unknown keys by default (section 5 shows the strict decode).
- `Bind` also reads path and query parameters into a struct with `param` and `query` tags. Whether a `qt.ID` is filled from a path parameter depends on the Echo version supporting `encoding.TextUnmarshaler`; calling `qt.ParseID` yourself, as above, does not depend on it.
- **`validate:"required"` tests the struct, not the value.** A `qt.Opt[T]`, `qt.Null[T]` or `qt.ID[S]` is a struct, so `required` is always satisfied by a zero value. For "this field must be provided" write a check on `IsSet()` or `.Valid`.
- `validator` cannot register one rule for every instantiation of a generic type, but it can call an interface. `Opt[T]` has `IsSet() bool`; one custom rule covers every `Opt`:

```go
v.RegisterValidation("provided", func(fl validator.FieldLevel) bool {
	s, ok := fl.Field().Interface().(interface{ IsSet() bool })
	return !ok || s.IsSet()
})
```

- `c.Validate` returns the validator's error text, which names Go field names and tags: map it to field errors for your clients rather than forwarding it.

---

## 12. A whole resource

The files of one resource, from the table to the route. Everything that is not HTTP is **tested** above; the handlers are **not compiled**.

```
db/                      generated: tables, rows, inserts, patches
internal/store/users.go  functions that take values and return rows
internal/api/users_dto.go
internal/api/users_handler.go
```

`store/users.go` knows nothing about HTTP:

```go
func (s *Store) Create(ctx context.Context, ins q.InsertQuery) (User, error) {
	return s.db.One[User](ctx, ins)
}
func (s *Store) Get(ctx context.Context, id UserID) (User, error) {
	return s.db.One[User](ctx, q.Select(userCols...).From(Users).Where(Users.ID.Eq(id)))
}
func (s *Store) List(ctx context.Context, sel q.SelectQuery) ([]User, error) {
	return s.db.All[User](ctx, sel)
}
func (s *Store) Delete(ctx context.Context, id UserID) error {
	_, err := s.db.Exec(ctx, q.DeleteFrom(Users).Where(Users.ID.Eq(id)))
	return err
}
```

`api/users_dto.go` holds `CreateUser`, `PatchUser`, `ListUsers`, `UserResp`, `Page`, `apiError` and the mappers (sections 5 to 10). `api/users_handler.go` binds, validates, calls the store and maps the result:

| Route | Request DTO | Store call | Success | Failures |
| --- | --- | --- | --- | --- |
| `POST /users` | `CreateUser` | `Create(insert)` | 201 + `UserResp` | 409 (email used), 422 |
| `GET /users/:id` | path id | `Get` | 200 + `UserResp` | 404 |
| `GET /users` | `ListUsers` | `List` | 200 + `Page[UserResp]` | 400 (bad query) |
| `PATCH /users/:id` | `PatchUser` | `Update` | 200 + `UserResp`, or 204 for `{}` | 404, 409, 422 |
| `DELETE /users/:id` | path id | `Delete` | 204 | 404, 409 (still referenced) |

Because the store takes values and statements, never `echo.Context`, the same functions work from a CLI, a job or a test.

### Soft delete

If rows are never removed, the DTO still must not expose the flag as writable. A pattern (**not compiled**: the fixture has no `deleted_at` column):

```go
// delete
q.Update(Users).
    Set(Users.DeletedAt.To(q.NowTZ())).
    Where(Users.ID.Eq(id), Users.DeletedAt.IsNull())
// every read
q.Select(userCols...).
    From(Users).
    Where(Users.DeletedAt.IsNull(), ...)
```

Two traps: every read must carry the `IsNull` filter (put it in one function that builds the base `Select`), and a unique index must be partial (`WHERE deleted_at IS NULL`) or a deleted email blocks a new account. `PATCH` must not be able to set `deleted_at`: it is not in `PatchUser`.

---

## 13. Testing handlers

**Not compiled.** With `httptest`, a handler test checks the contract, not the SQL:

```go
func TestPatchUserNull(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPatch, "/users/"+id.String(), strings.NewReader(`{"nickname":null}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id.String())

	if err := h.Patch(c); err != nil { t.Fatal(err) }
	if rec.Code != http.StatusOK { t.Fatalf("status %d: %s", rec.Code, rec.Body) }

	var got UserResp
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Nickname.Valid { t.Errorf("nickname should be NULL, got %+v", got.Nickname) }
}
```

Cases worth a test per resource: absent / `null` / value for every nullable patch field, `{}`, an unknown key, a forged id, a duplicate unique value, a `limit` above the cap, and a body that sends a read-only field. Test the JSON round trip of a response with `json.Unmarshal` into the same DTO: it catches a field that serializes but does not parse back.

---

## 14. Pitfalls

| Pitfall | What happens | Fix |
| --- | --- | --- |
| `omitempty` on a `qt` struct field | the key is never omitted | `omitzero` |
| `required` on `qt.Null`, `qt.Opt`, `qt.ID` | always passes | check `.Valid` or `IsSet()` |
| binding a request into the generated patch or insert | mass assignment of every column | a DTO with an allow-list, converted |
| `qt.Null` for a column with a `DEFAULT` | an absent key writes NULL over the default | `qt.Opt[T]` with `ToIf` |
| `ToIf` on a required column, or `ToOrDefault` in a `SET` | the column disappears, or is reset to its default | `To` for required columns, `ToIf` in updates |
| `qt.Opt` in a query-string struct | not filled (JSON methods only) | pointers, then `if` or `EqIf` |
| accepting and ignoring a read-only field | the client believes it worked | leave it out of the DTO, strict decode |
| `"infinity"` in a timestamp field | strict date parsers in clients fail | do not store infinities, or document them |
| `qt.Char`, `[16]byte` in a DTO | numbers and arrays of numbers | `string` |
| returning the row struct from a public endpoint | every migration changes your contract | response DTO |
| trusting an opaque id | it is obfuscation | check authorization on the row |
| unbounded `limit` from the client | whole-table reads | cap it |
| paging without a tie-breaker | rows jump between pages | sort by a unique column last |
| a nil slice in a response | `null` instead of `[]` | initialise to an empty slice |
| one query per parent for children | N+1 | one `ANY($1)` query, group in Go |
| forwarding the database error text | leaks constraint and table names | `toAPIError`, log the original |

---

## 15. Checklist for a new resource

1. List the columns a client may set and see. Everything else stays out of the DTOs.
2. Write `Create`, `Patch`, `Resp`, and the `List` query struct. Decide, per optional field, what an absent key means (section 6).
3. Add `toResp` and the `Assignments` or `ToPatch` conversion. No DTO is passed to the store.
4. Cap `limit`, sort with a tie-breaker, and return a `Page`.
5. Map errors with the one `toAPIError`; log the original.
6. Check authorization on every `:id` route, not only that the id parses.
7. Write the handler tests of section 13.
8. Run the analyzer, `go vet -vettool=$(which qlxvet) ./...` (V018 checks that the row type matches the selected columns), and, in tests, `pgxexec.VerifyRow` for each row type against the real server: it also reports a nullable column read into a plain field.
