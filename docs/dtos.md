# DTOs with QLX

A DTO is the Go struct of an HTTP request or response. QLX does not need one: a generated row struct can be returned as it is. This page says when that is a good idea, which `qt` types are safe to put in a DTO, and how to write the create, patch and response shapes with an HTTP framework such as Echo.

What is tested and what is not:

- Every Go snippet that uses only `qt` is compiled and run by `qt/dto_guide_test.go`, and the JSON forms in the tables come from `qt/json_test.go` and from running `json.Marshal`.
- The Echo and `go-playground/validator` snippets are marked **(not compiled here)**. They follow the documented behaviour of those libraries; run them in your project before relying on them.

---

## 1. The short answer

Use `qt` types in your DTOs when `qt` is your vocabulary anyway. The types listed as safe in section 3 have a JSON form that is stable and meant for it. Convert at the edge only for the few that have no good public form (section 3, last rows).

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
| `qt.Null[T]` | `null` or the value | `null` gives NULL, a value gives the value | yes |
| `qt.Opt[T]` | the value; unset is `null` (use `omitzero`) | key present: set. `null`: set only if `T` can hold NULL (section 5) | yes, for patches |
| `qt.ID[S]` | an opaque 11-character string | the same string; a forged one is an error | yes (section 4) |
| `qt.Date` | `"2024-02-29"`, or `"infinity"` | the same | yes |
| `qt.Timestamptz` | `"2024-02-29T01:02:03Z"`, or `"infinity"` | the same | yes; tell clients about `"infinity"` |
| `qt.Numeric` | `"1.50"`, a string | a string | yes: no float rounding |
| `qt.TimeOfDay` | `"01:00:00"` | the same | yes |
| `qt.Interval` | ISO 8601 duration, `"P1Y2M3DT1H"` | the same, and PostgreSQL's own style | yes |
| `qt.Range[T]`, `qt.Multirange[T]` | `{"lower":1,"upper":5,"lower_inc":true,"upper_inc":false}`, `{"empty":true}`, array of those | the same | yes; a range with a bound flagged inclusive on an unbounded side is rejected |
| `qt.JSONB[T]` | the document itself | the document | yes |
| `netip.Addr`, inside `qt.Null` too | `"10.0.0.1"` | the same | yes |
| `qt.Vector` | `[1,2]` | the same | yes |
| `qt.Char` | a number (`97`) | a number | no: convert to `string` |
| `[16]byte` (uuid) | an array of 16 numbers | the same | no: convert to a string, or use a typed id |

Two rules that follow:

- A response DTO can hold a `qt.Null[string]` directly: it already writes `null` or the value. Convert to `*string` (`Nullable.Ptr()`) only if the consumers of the DTO want pointers.
- Never put a generated enum or a text-form type (`qt.Geometry`, `qt.LTree`) in a public contract without deciding that its text form is the contract.

Behaviour on infinities: `qt.Date` and `qt.Timestamptz` write `"infinity"` and `"-infinity"`. A client that parses every timestamp with a date library will fail on those rows. If you do not store infinities, your clients never see them; if you do, document it.

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
- Parse path parameters with `qt.ParseID[users.IDSpace](c.Param("id"))`. A mistyped or forged string is `qt.ErrBadID`: answer 400 or 404, not 500.
- An id of one table cannot be used where another table's is expected; the compiler says so. A DTO field typed `UserID` therefore cannot be filled with an order id by mistake.

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

```go
type PatchUser struct {
	Email    qt.Opt[string]          `json:"email,omitzero"`    // NOT NULL column
	Nickname qt.Opt[qt.Null[string]] `json:"nickname,omitzero"` // nullable column
}
```

`omitzero` (Go 1.24 and later) with the `IsZero` method of `Opt` drops unset fields when you write the DTO back out. `omitempty` does not work on a struct: it would never omit.

Build the statement from what was sent:

```go
func (p PatchUser) Assignments() []q.Assignment {
	var as []q.Assignment
	if p.Email.Set {
		as = append(as, Users.Email.To(p.Email.V))
	}
	if p.Nickname.Set {
		as = append(as, Users.Nickname.To(p.Nickname.V))
	}
	return as
}

st := q.Update(Users).Set(p.Assignments()...).Where(Users.ID.Eq(id))
```

An empty body gives no assignments, and an `UPDATE` without `SET` is an error at render time (and V012 in the analyzer), not a silent no-op: decide whether your endpoint answers 400 or 204 for `{}`.

### The generated patch

The generator already writes `UserPatch`, made of `qt.Opt` fields, and `Update(...).SetPatch(p)` applies it. Since `Opt` now has JSON methods, `UserPatch` decodes a request body directly. **Do not bind a request straight into the generated patch** unless every column of the table may be changed by that caller: a client could send `"role":"admin"`. This is mass assignment, and the type system cannot know it is wrong.

The safe form is a DTO with only the allowed fields, converted:

```go
func (p PatchUser) ToPatch() UserPatch {
	return UserPatch{Email: p.Email, Nickname: p.Nickname}
}

st := q.Update(Users).SetPatch(patch).Where(Users.ID.Eq(id))
```

(The field names of the generated patch depend on your schema: use what `qlx gen` wrote.) The conversion is one line per allowed field, and adding a column to the table never exposes it by itself.

---

## 6. Create requests

The generated insert type takes the required columns as constructor arguments, so a missing one is a compile error:

```go
type CreateUser struct {
	Email string  `json:"email" validate:"required,email"`
	Nick  *string `json:"nickname"`
}

func (r CreateUser) Insert() UserInsert {
	ins := NewUserInsert(r.Email)
	if r.Nick != nil {
		ins = ins.Nickname(qt.Some(*r.Nick))
	}
	return ins
}
```

Validation is about the request, the compiler is about the statement: both are needed. The compiler cannot know that an email must contain an `@`.

---

## 7. Responses

```go
type UserResp struct {
	ID        UserID          `json:"id"`
	Email     string          `json:"email"`
	Nickname  qt.Null[string] `json:"nickname"`            // null or the value
	DeletedAt *qt.Timestamptz `json:"deleted_at,omitempty"` // key absent when nil
}

func toUserResp(r User) UserResp {
	return UserResp{ID: r.ID, Email: r.Email, Nickname: r.Nickname, DeletedAt: r.DeletedAt.Ptr()}
}
```

Choose per field whether NULL is `null` (keep `qt.Null[T]`) or an absent key (`*T` with `omitempty`). Pick one convention for the API and keep it.

Returning the row struct itself is acceptable for an internal service. For a public API write the DTO: the row grows with every migration.

---

## 8. Query parameters and filters

`qt.Opt` has JSON methods only, so a framework that binds query strings (Echo does, with the `query` tag) will not fill it. Use pointers in the query struct and build the predicates:

```go
type ListUsers struct {
	Tier  *string `query:"tier"`
	Limit int     `query:"limit"`
}

var preds []q.Pred
if f.Tier != nil {
	preds = append(preds, Users.Tier.Eq(*f.Tier))
}
```

Columns also have `EqIf(qt.Opt[T])`, which drops the predicate when the option is unset: `Users.Tier.EqIf(opt)`.

Cap `limit` yourself. A DTO that passes a client number to `Limit` unchecked is a way to ask for the whole table.

---

## 9. Echo

**(not compiled here)**

```go
func (h *Handler) Patch(c echo.Context) error {
	id, err := qt.ParseID[users.IDSpace](c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound) // forged or mistyped id
	}
	var body PatchUser
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if err := c.Validate(&body); err != nil {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, err.Error())
	}
	st := q.Update(Users).SetPatch(body.ToPatch()).Where(Users.ID.Eq(id)).
		Returning(Users.ID, Users.Email, Users.Nickname, Users.DeletedAt)
	u, err := h.db.One[User](c.Request().Context(), st) // h.db is the pgxexec client
	switch {
	case pgxexec.IsUnique(err, "users_email_key"): // the constraint name; the generator also writes constants for them
		return echo.NewHTTPError(http.StatusConflict)
	case errors.Is(err, pgx.ErrNoRows):
		return echo.NewHTTPError(http.StatusNotFound)
	case err != nil:
		return err
	}
	return c.JSON(http.StatusOK, toUserResp(u))
}
```

Points specific to the framework:

- `Bind` reads JSON with `encoding/json`, so the forms of section 3 apply. It does not reject unknown keys by default; if you want a strict body, decode yourself with `DisallowUnknownFields`.
- `Bind` also reads path and query parameters into a struct with `param` and `query` tags. Whether a `qt.ID` is filled from a path parameter depends on the Echo version supporting `encoding.TextUnmarshaler`; calling `qt.ParseID` yourself, as above, does not depend on it.
- **`validate:"required"` tests the struct, not the value.** A `qt.Opt[T]`, `qt.Null[T]` or `qt.ID[S]` is a struct, so `required` is always satisfied by a zero value. For "this field must be provided" in a patch or create body, write a check on `IsSet()` or `.Valid`.
- `validator` cannot register one rule for every instantiation of a generic type, but it can call an interface. `Opt[T]` has `IsSet() bool`; a single custom rule that asserts `interface{ IsSet() bool }` on the field covers every `Opt`:

```go
v.RegisterValidation("provided", func(fl validator.FieldLevel) bool {
	s, ok := fl.Field().Interface().(interface{ IsSet() bool })
	return !ok || s.IsSet()
})
```

- Map errors on purpose: `qt.ErrBadID` to 404 or 400, `pgxexec.IsUnique(err, constraint)` to 409 (the constraint name is required), `pgxexec.IsForeignKey(err, "")` to 422 (an empty name means any), `pgx.ErrNoRows` to 404. Do not send a raw database error to a client.

---

## 10. Pitfalls

| Pitfall | What happens | Fix |
| --- | --- | --- |
| `omitempty` on a `qt` struct field | the key is never omitted | `omitzero` |
| `required` on `qt.Null`, `qt.Opt`, `qt.ID` | always passes | check `.Valid` or `IsSet()` |
| binding a request into the generated patch or insert | mass assignment of every column | a DTO with an allow-list, converted |
| `qt.Opt` in a query-string struct | not filled (JSON methods only) | pointers, then `EqIf` or `if` |
| `"infinity"` in a timestamp field | strict date parsers in clients fail | do not store infinities, or document them |
| `qt.Char`, `[16]byte` in a DTO | numbers and arrays of numbers | `string` |
| returning the row struct from a public endpoint | every migration changes your contract | response DTO |
| trusting an opaque id | it is obfuscation | check authorization on the row |
| unbounded `limit` from the client | whole-table reads | cap it |

---

## 11. Layout

A layout that keeps the layers apart without ceremony:

```
db/            generated: tables, rows, inserts, patches
internal/api/
    users_dto.go       CreateUser, PatchUser, UserResp, toUserResp
    users_handler.go   bind, validate, call the store, map errors
internal/store/
    users.go           functions that take values and return rows (the QLX statements live here)
```

The store takes plain values or generated types, never `echo.Context`. The handler never builds a statement beyond passing the converted patch. That way the same store works from a CLI, a job or a test.

Where several resources share a shape (paging, timestamps), put it in a small shared DTO and embed it; embedded structs are flattened by `encoding/json`.
