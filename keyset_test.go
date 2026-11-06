package qlx_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
)

func TestKeysetRender(t *testing.T) {
	day := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	page := func(keys ...q.Key) q.Statement {
		return q.Select(Users.ID).From(Users).Where(q.After(keys...)).OrderBy(q.KeyOrder(keys...)...).Limit(50)
	}
	cases := []struct {
		name, sql string
		st        q.Statement
		args      int
	}{
		{"first page, no key", `SELECT "users"."id" FROM "public"."users" LIMIT $1`, page(), 1},
		{"one key", `WHERE "users"."id" > $1 ORDER BY "users"."id" LIMIT $2`, page(q.KeyAsc(Users.ID, 7)), 2},
		{"one key desc", `WHERE "users"."id" < $1 ORDER BY "users"."id" DESC`, page(q.KeyDesc(Users.ID, 7)), 2},
		{"same direction: row comparison", `WHERE ("users"."created_at", "users"."id") > ($1, $2) ORDER BY "users"."created_at", "users"."id"`,
			page(q.KeyAsc(Users.CreatedAt, day), q.KeyAsc(Users.ID, 7)), 3},
		{"same direction desc", `WHERE ("users"."created_at", "users"."id") < ($1, $2) ORDER BY "users"."created_at" DESC, "users"."id" DESC`,
			page(q.KeyDesc(Users.CreatedAt, day), q.KeyDesc(Users.ID, 7)), 3},
		{"mixed directions: expansion", `WHERE "users"."created_at" < $1 OR ("users"."created_at" = $2 AND "users"."id" > $3) ORDER BY "users"."created_at" DESC, "users"."id"`,
			page(q.KeyDesc(Users.CreatedAt, day), q.KeyAsc(Users.ID, 7)), 4},
	}
	for _, c := range cases {
		st, args := renderIf(t, c.st)
		if !contains(st.SQL, c.sql) {
			t.Errorf("%s:\n got %s\nwant …%s…", c.name, st.SQL, c.sql)
		}
		if got := len(args); got != c.args {
			t.Errorf("%s: %d args, want %d", c.name, got, c.args)
		}
	}
}

func TestSortsParse(t *testing.T) {
	s := q.NewSorts(map[string]q.Orderable{"email": Users.Email, "created": Users.CreatedAt}).
		Default(Users.CreatedAt.Desc()).Tiebreak(Users.ID.Asc())
	sql := func(spec string) (string, error) {
		ord, err := s.Parse(spec)
		if err != nil {
			return "", err
		}
		st, _ := renderIf(t, q.Select(Users.ID).From(Users).OrderBy(ord...))
		return st.SQL, nil
	}
	ok := map[string]string{
		"":                   `ORDER BY "users"."created_at" DESC, "users"."id"`,
		"email":              `ORDER BY "users"."email", "users"."id"`,
		"-created,email":     `ORDER BY "users"."created_at" DESC, "users"."email", "users"."id"`,
		" +email , -created": `ORDER BY "users"."email", "users"."created_at" DESC, "users"."id"`,
	}
	for spec, want := range ok {
		got, err := sql(spec)
		if err != nil || !contains(got, want) {
			t.Errorf("%q: got %q, %v; want …%s", spec, got, err, want)
		}
	}
	for _, bad := range []string{"password", "email;DROP TABLE users", "-", ",", "email,email", "email,-email", "email,created,email,created", "email,"} {
		if _, err := s.Parse(bad); !errors.Is(err, q.ErrSort) {
			t.Errorf("%q: want ErrSort, got %v", bad, err)
		}
	}
	if !slices.Equal(s.Names(), []string{"created", "email"}) {
		t.Errorf("names %v", s.Names())
	}
	for _, name := range []string{"", "-x", "a,b", "+x", " x"} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("NewSorts accepted %q", name)
				}
			}()
			q.NewSorts(map[string]q.Orderable{name: Users.ID})
		}()
	}
}

func TestInsertChunks(t *testing.T) {
	base := q.InsertInto(Users)
	row := func(n int) q.RowAssigner { return rowOf(n) }
	var rows []q.RowAssigner
	for i := range 7 {
		rows = append(rows, row(i))
	}
	cs, err := base.Chunks(rows, 6) // 2 columns per row: 3 rows per chunk
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 3 {
		t.Fatalf("%d chunks, want 3", len(cs))
	}
	counts := []int{3, 3, 1}
	for i, c := range cs {
		_, args := renderIf(t, c)
		if got := len(args); got != counts[i]*2 {
			t.Errorf("chunk %d: %d args, want %d", i, got, counts[i]*2)
		}
	}
	if cs, err := base.Chunks(nil, 0); err != nil || cs != nil {
		t.Errorf("no rows: %v %v", cs, err)
	}
	if _, err := base.Chunks(rows, 1); !errors.Is(err, q.ErrShape) {
		t.Errorf("row bigger than the budget: %v", err)
	}
	if _, err := base.Row(Users.Email.To("x")).Chunks(rows, 0); !errors.Is(err, q.ErrShape) {
		t.Errorf("base with rows: %v", err)
	}
	cs, _ = base.Chunks(rows, 0)
	if len(cs) != 1 {
		t.Errorf("default budget: %d chunks", len(cs))
	}
	// 100000 rows of 2 columns: 32767 rows per statement, so 4 statements
	big := make([]q.RowAssigner, 100000)
	for i := range big {
		big[i] = rowOf(i)
	}
	cs, err = q.InsertInto(Users).Chunks(big, 0)
	if err != nil || len(cs) != 4 {
		t.Errorf("big: %d chunks, %v", len(cs), err)
	}
}

// rowOf is a two-column insert row.
type rowOf int

func (r rowOf) Assignments() []q.Assignment {
	return []q.Assignment{Users.Email.To("bulk-" + string(rune('a'+int(r)%26))), Users.Balance.To(int64(r))}
}
