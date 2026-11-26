package qpage

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
	"github.com/noobmaisfier/qlx/qt"
)

type row struct {
	ID      int64     `db:"id"`
	Email   string    `db:"email"`
	Created time.Time `db:"created_at"`
}

var testSecret = []byte("0123456789abcdef-test-secret")

func setup(t *testing.T) {
	t.Helper()
	old := secret.Load()
	if err := SetSecret(testSecret); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { secret.Store(old) })
}

// both sorts go the same way, so every After is one row comparison whose
// arguments are the key values: that lets a fake table apply the cursor.
func sorts() *SortSet[row] {
	return Sorts[row](
		Sort("created", Desc(Users.CreatedAt)),
		Sort("email", Desc(Users.Email)),
		Tiebreak(Desc(Users.ID)),
	)
}

func table(n int) []row {
	day := time.Date(2026, 1, 1, 12, 0, 0, 123456789, time.UTC)
	var out []row
	for i := 0; i < n; i++ {
		// two rows share each timestamp, so the id breaks the tie
		out = append(out, row{ID: int64(n - i), Email: string(rune('z'-i/2)) + "@x.io", Created: day.Add(-time.Duration(i/2) * time.Hour)})
	}
	return out
}

func selectRows() q.SelectQuery { return q.Select(Users.ID, Users.Email, Users.CreatedAt).From(Users) }

// fake plays the database: it renders the real statement, reads the key
// values from its arguments (one row comparison: key1, key2, limit), and
// filters the rows that come strictly after them in the sort order.
func fake(t *testing.T, all []row, w *Window, sqls *[]string) []row {
	t.Helper()
	return fakeSel(t, all, w, sqls, selectRows())
}

func fakeSel(t *testing.T, all []row, w *Window, sqls *[]string, base q.SelectQuery) []row {
	t.Helper()
	sel, err := w.Apply(base)
	if err != nil {
		t.Fatal(err)
	}
	st, err := q.Render(q.Dialect{Version: q.PG18}, sel)
	if err != nil {
		t.Fatal(err)
	}
	*sqls = append(*sqls, st.SQL)
	args, _ := st.Args()
	var out []row
	for _, r := range all {
		if len(args) == 3 {
			switch k := args[0].(type) {
			case time.Time:
				id := args[1].(int64)
				if !(r.Created.Before(k) || (r.Created.Equal(k) && r.ID < id)) {
					continue
				}
			case string:
				id := args[1].(int64)
				if !(r.Email < k || (r.Email == k && r.ID < id)) {
					continue
				}
			}
		}
		out = append(out, r)
		if int64(len(out)) == w.Limit {
			break
		}
	}
	return out
}

func walk(t *testing.T, s *SortSet[row], sort string, size int, all []row) ([]int64, []string) {
	t.Helper()
	var ids []int64
	var sqls []string
	cursor := ""
	for pages := 0; pages < 50; pages++ {
		w, err := s.Window(Request{Sort: sort, Cursor: cursor, Size: size})
		if err != nil {
			t.Fatal(err)
		}
		page, err := s.Page(w, fake(t, all, &w, &sqls))
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range page.Items {
			ids = append(ids, r.ID)
		}
		if page.Next == "" {
			return ids, sqls
		}
		if len(page.Items) != size {
			t.Fatalf("a page before the last has %d items, want %d", len(page.Items), size)
		}
		cursor = page.Next
	}
	t.Fatal("never ended")
	return nil, nil
}

func TestWalkEverySort(t *testing.T) {
	setup(t)
	s := sorts()
	all := table(11)
	for _, sort := range []string{"", "created"} {
		ids, sqls := walk(t, s, sort, 4, all)
		if len(ids) != 11 {
			t.Fatalf("sort %q: %d rows", sort, len(ids))
		}
		for i, id := range ids {
			if id != all[i].ID {
				t.Fatalf("sort %q: row %d is %d, want %d (skipped or repeated)", sort, i, id, all[i].ID)
			}
		}
		if strings.Contains(sqls[0], "WHERE") {
			t.Errorf("first page has a predicate: %s", sqls[0])
		}
		if !strings.Contains(sqls[1], `("users"."created_at", "users"."id") < ($1, $2) ORDER BY "users"."created_at" DESC, "users"."id" DESC LIMIT $3`) {
			t.Errorf("later page: %s", sqls[1])
		}
	}
	ids, sqls := walk(t, s, "email", 3, all)
	if len(ids) != 11 || !strings.Contains(sqls[1], `("users"."email", "users"."id") < ($1, $2)`) {
		t.Errorf("email: %d rows, %s", len(ids), sqls[1])
	}
}

func TestExactMultipleEmptyAndSizes(t *testing.T) {
	setup(t)
	s := sorts()
	var sqls []string
	all := table(8)
	w, _ := s.Window(Request{Size: 4})
	first, _ := s.Page(w, fake(t, all, &w, &sqls))
	w, err := s.Window(Request{Cursor: first.Next, Size: 4})
	if err != nil {
		t.Fatal(err)
	}
	second, _ := s.Page(w, fake(t, all, &w, &sqls))
	if len(second.Items) != 4 || second.Next != "" {
		t.Fatalf("a full last page has no next: %+v", second)
	}
	w, _ = s.Window(Request{})
	empty, err := s.Page(w, nil)
	if err != nil || empty.Items == nil || len(empty.Items) != 0 || empty.Next != "" {
		t.Fatalf("empty page: %#v %v", empty, err)
	}
	for size, want := range map[int]int{0: 20, -5: 20, 7: 7, 100: 100, 5000: 100} {
		if w, _ := s.Window(Request{Size: size}); w.Size() != want || w.Limit != int64(want)+1 {
			t.Errorf("size %d: got %d (limit %d), want %d", size, w.Size(), w.Limit, want)
		}
	}
	//qlx:ignore test without tiebreak
	small := Sorts[row](Sort("a", Asc(Users.ID)), Sizes(50, 10))
	if w, _ := small.Window(Request{}); w.Size() != 10 {
		t.Errorf("a default above the maximum is capped: %d", w.Size())
	}
}

func TestSortNames(t *testing.T) {
	setup(t)
	s := sorts()
	if got := strings.Join(s.Names(), ","); got != "created,email" {
		t.Errorf("names %s", got)
	}
	_, err := s.Window(Request{Sort: "password"})
	if !errors.Is(err, ErrSort) || !errors.Is(err, q.ErrSort) || !strings.Contains(err.Error(), "created, email") {
		t.Errorf("unknown sort: %v", err)
	}
}

func TestCursorIsRefused(t *testing.T) {
	setup(t)
	s := sorts()
	var sqls []string
	w, _ := s.Window(Request{Size: 2})
	page, _ := s.Page(w, fake(t, table(5), &w, &sqls))
	good := page.Next
	if good == "" || strings.ContainsAny(good, "+/= ") {
		t.Fatalf("cursor %q", good)
	}
	flip := func(i int) string {
		b := []byte(good)
		if b[i] == 'A' {
			b[i] = 'B'
		} else {
			b[i] = 'A'
		}
		return string(b)
	}
	for name, c := range map[string]Request{
		"garbage":        {Cursor: "not base64 !!"},
		"truncated":      {Cursor: good[:10]},
		"body altered":   {Cursor: flip(12)},
		"mac altered":    {Cursor: flip(len(good) - 2)},
		"other sort":     {Sort: "email", Cursor: good},
		"default vs not": {Sort: "", Cursor: mustSeal(t, "email")},
	} {
		if _, err := s.Window(c); !errors.Is(err, ErrCursor) {
			t.Errorf("%s: got %v, want ErrCursor", name, err)
		}
	}
	if _, err := s.Window(Request{Cursor: good}); err != nil {
		t.Errorf("the untouched cursor must pass: %v", err)
	}
	// a cursor with the wrong number of keys, correctly signed
	bad, _ := sealCursor("created", nil)
	if _, err := s.Window(Request{Cursor: bad}); !errors.Is(err, ErrCursor) {
		t.Errorf("wrong key count: %v", err)
	}
	// a key that does not decode into the column's type
	bad, _ = sealCursor("created", []json.RawMessage{json.RawMessage(`"not a time"`), json.RawMessage(`1`)})
	if _, err := s.Window(Request{Cursor: bad}); !errors.Is(err, ErrCursor) {
		t.Errorf("undecodable key: %v", err)
	}
}

func mustSeal(t *testing.T, name string) string {
	t.Helper()
	c, err := sealCursor(name, []json.RawMessage{json.RawMessage(`"2026-01-01T00:00:00Z"`), json.RawMessage(`1`)})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestSecretSetupAndRotation(t *testing.T) {
	old := secret.Load()
	t.Cleanup(func() { secret.Store(old) })
	s := sorts()
	var sqls []string
	mk := func() string {
		w, _ := s.Window(Request{Size: 2})
		p, err := s.Page(w, fake(t, table(5), &w, &sqls))
		if err != nil {
			t.Fatal(err)
		}
		return p.Next
	}

	secret.Store(nil)
	if _, err := s.Window(Request{Size: 2}); !errors.Is(err, ErrCursorKey) {
		t.Errorf("no secret, first page: %v", err)
	}
	if err := CheckSecret(); !errors.Is(err, ErrCursorKey) {
		t.Errorf("CheckSecret without a secret: %v", err)
	}
	_ = SetSecret(testSecret)
	if err := CheckSecret(); err != nil {
		t.Errorf("CheckSecret with a secret: %v", err)
	}
	w, _ := s.Window(Request{Size: 2})
	secret.Store(nil) // the secret goes away between the two steps
	if _, err := s.Page(w, table(5)[:3]); !errors.Is(err, ErrCursorKey) {
		t.Errorf("no secret, making a cursor: %v", err)
	}
	if _, err := s.Window(Request{Cursor: "AAAAAAAAAAAAAAAAAAAAAAAAAAAA"}); !errors.Is(err, ErrCursor) && !errors.Is(err, ErrCursorKey) {
		t.Errorf("no secret, checking: %v", err)
	}
	for name, f := range map[string]func() error{
		"short":          func() error { return SetSecret([]byte("short")) },
		"short previous": func() error { return SetSecret(testSecret, []byte("short")) },
	} {
		if f() == nil {
			t.Errorf("%s secret accepted", name)
		}
	}

	oldSecret := []byte("old-secret-0123456789")
	_ = SetSecret(oldSecret)
	issuedBefore := mk()
	_ = SetSecret([]byte("brand-new-secret-0123"), oldSecret)
	if _, err := s.Window(Request{Cursor: issuedBefore}); err != nil {
		t.Errorf("a cursor signed with a previous secret must pass: %v", err)
	}
	issuedAfter := mk()
	_ = SetSecret(oldSecret)
	if _, err := s.Window(Request{Cursor: issuedAfter}); !errors.Is(err, ErrCursor) {
		t.Errorf("the old secret must not accept a cursor of the new one: %v", err)
	}
	_ = SetSecret([]byte("unrelated-secret-01234"))
	if _, err := s.Window(Request{Cursor: issuedBefore}); !errors.Is(err, ErrCursor) {
		t.Errorf("an unrelated secret must refuse: %v", err)
	}
}

func TestKeyKeepsNanoseconds(t *testing.T) {
	setup(t)
	s := sorts()
	rows := table(3)
	w, _ := s.Window(Request{Size: 2})
	p, _ := s.Page(w, rows)
	w, err := s.Window(Request{Cursor: p.Next, Size: 2})
	if err != nil {
		t.Fatal(err)
	}
	sel, err := w.Apply(selectRows())
	if err != nil {
		t.Fatal(err)
	}
	st, err := q.Render(q.Dialect{Version: q.PG18}, sel)
	if err != nil {
		t.Fatal(err)
	}
	args, err := st.Args()
	if err != nil {
		t.Fatal(err)
	}
	if got := args[0].(time.Time); !got.Equal(rows[1].Created) {
		t.Errorf("key came back as %v, want %v", got, rows[1].Created)
	}
}

func TestMixedDirectionsRender(t *testing.T) {
	setup(t)
	s := Sorts[row](Sort("mixed", Desc(Users.CreatedAt)), Tiebreak(Asc(Users.ID)))
	rows := table(3)
	w, _ := s.Window(Request{Size: 2})
	p, _ := s.Page(w, rows)
	w, _ = s.Window(Request{Cursor: p.Next, Size: 2})
	sel, err := w.Apply(selectRows())
	if err != nil {
		t.Fatal(err)
	}
	st, err := q.Render(q.Dialect{Version: q.PG18}, sel)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(st.SQL, `"users"."created_at" < $1 OR ("users"."created_at" = $2 AND "users"."id" > $3)`) {
		t.Errorf("mixed directions: %s", st.SQL)
	}
}

func TestApplyChecksTheSelectList(t *testing.T) {
	setup(t)
	s := sorts()
	w, _ := s.Window(Request{})
	if _, err := w.Apply(q.Select(Users.ID, Users.Email).From(Users)); err == nil || !strings.Contains(err.Error(), "created_at") {
		t.Errorf("a key missing from the select list must be reported: %v", err)
	}
	if _, err := w.Apply(q.Select(Users.ID, Users.Email, Users.CreatedAt.As("made")).From(Users)); err == nil {
		t.Error("an aliased key is not returned under its own name")
	}
	var zero Window
	if _, err := zero.Apply(selectRows()); err == nil {
		t.Error("a zero Window must be refused")
	}
	//qlx:ignore test without tiebreak
	other := Sorts[row](Sort("x", Asc(Users.ID)))
	if _, err := other.Page(w, nil); err == nil {
		t.Error("a Window of another SortSet must be refused")
	}
	for name, bad := range map[string]q.SelectQuery{
		"OrderBy":    selectRows().OrderBy(Users.ID.Asc()),
		"Limit":      selectRows().Limit(10),
		"Offset":     selectRows().Offset(5),
		"FetchFirst": selectRows().FetchFirst(10),
	} {
		if _, err := w.Apply(bad); err == nil {
			t.Errorf("Apply must refuse a base SELECT with %s", name)
		}
	}
}

type embedded struct {
	ID int64 `db:"id"`
}
type withEmbedded struct {
	embedded
	Mail    string `db:"Email"` // the tag is matched without regard to case
	Balance int64
	skipped int64
}
type nullableRow struct {
	Nick qt.Null[string] `db:"nickname"`
	ID   int64           `db:"id"`
}
type wrongType struct {
	ID int `db:"id"`
}

func TestDeclarationsThatCannotWork(t *testing.T) {
	setup(t)
	cases := map[string]func() error{
		"no sort": func() error { _, e := newSorts[row](nil); return e },
		"duplicate name": func() error {
			_, e := newSorts[row]([]Option{Sort("a", Asc(Users.ID)), Sort("a", Asc(Users.Email))})
			return e
		},
		"bad name":     func() error { _, e := newSorts[row]([]Option{Sort("a,b", Asc(Users.ID))}); return e },
		"no key":       func() error { _, e := newSorts[row]([]Option{Sort("a")}); return e },
		"expression":   func() error { _, e := newSorts[row]([]Option{Sort("a", Asc(Users.Balance.Add(1)))}); return e },
		"key twice":    func() error { _, e := newSorts[row]([]Option{Sort("a", Asc(Users.ID), Desc(Users.ID))}); return e },
		"not a struct": func() error { _, e := newSorts[int]([]Option{Sort("a", Asc(Users.ID))}); return e },
		"no field":     func() error { _, e := newSorts[row]([]Option{Sort("a", Asc(Users.Balance))}); return e },
		"wrong type":   func() error { _, e := newSorts[wrongType]([]Option{Sort("a", Asc(Users.ID))}); return e },
		"nullable":     func() error { _, e := newSorts[nullableRow]([]Option{Sort("a", Asc(Users.Nickname))}); return e },
	}
	for name, f := range cases {
		if err := f(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	defer func() {
		if recover() == nil {
			t.Error("Sorts must panic on a declaration that cannot work")
		}
	}()
	//qlx:ignore test empty declaration
	Sorts[row]()
}

func TestRowFieldsLikePgxexec(t *testing.T) {
	setup(t)
	// an embedded struct is flattened, a tag matches without regard to case,
	// an untagged field is matched by its name, an unexported one is not
	s, err := newSorts[withEmbedded]([]Option{Sort("a", Asc(Users.Email), Desc(Users.Balance)), Tiebreak(Asc(Users.ID))})
	if err != nil {
		t.Fatal(err)
	}
	rows := []withEmbedded{{embedded{3}, "c", 30, 0}, {embedded{2}, "b", 20, 0}}
	w, _ := s.Window(Request{Size: 1})
	p, err := s.Page(w, rows)
	if err != nil || p.Next == "" || len(p.Items) != 1 {
		t.Fatalf("%+v %v", p, err)
	}
	if _, err := s.Window(Request{Cursor: p.Next}); err != nil {
		t.Errorf("the cursor of an embedded row must read back: %v", err)
	}
}

func TestTiebreakAlreadyAKeyIsKept(t *testing.T) {
	setup(t)
	s, err := newSorts[row]([]Option{Sort("by_id", Desc(Users.ID)), Tiebreak(Asc(Users.ID))})
	if err != nil {
		t.Fatal(err)
	}
	w, _ := s.Window(Request{})
	sel, err := w.Apply(selectRows())
	if err != nil {
		t.Fatal(err)
	}
	st, err := q.Render(q.Dialect{Version: q.PG18}, sel)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(st.SQL, `ORDER BY "users"."id" DESC LIMIT`) {
		t.Errorf("the sort's own key must win over the tiebreak: %s", st.SQL)
	}
}

func TestMaxIntPageSizeDoesNotOverflowLimit(t *testing.T) {
	setup(t)
	//qlx:ignore test without tiebreak
	s := Sorts[row](Sort("id", Asc(Users.ID)), Sizes(math.MaxInt, math.MaxInt))
	w, err := s.Window(Request{Size: math.MaxInt})
	if err != nil {
		t.Fatal(err)
	}
	if w.Limit <= 0 {
		t.Fatalf("Limit overflowed to %d", w.Limit)
	}
}

func TestOversizedCursorRejected(t *testing.T) {
	setup(t)
	//qlx:ignore test without tiebreak
	s := Sorts[row](Sort("id", Asc(Users.ID)))
	_, err := s.Window(Request{Cursor: strings.Repeat("A", maxCursorLen+1)})
	if !errors.Is(err, ErrCursor) {
		t.Fatalf("expected ErrCursor for oversized cursor, got %v", err)
	}
}
