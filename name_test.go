package qlx_test

import (
	"errors"
	"strings"
	"testing"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
)

func TestTaggedRendersALeadingComment(t *testing.T) {
	st, err := q.Render(q.Dialect{Version: q.PG18}, q.Tagged("users.by_email", q.Select(Users.ID.As("id")).From(Users)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(st.SQL, "/* qlx: users.by_email */ SELECT") {
		t.Fatalf("got %q", st.SQL)
	}
	if got := q.QueryName(st.SQL); got != "users.by_email" {
		t.Fatalf("QueryName = %q", got)
	}
	if q.QueryName("SELECT 1") != "" {
		t.Fatal("unnamed statement has no name")
	}
}

func TestTaggedCannotEscapeTheComment(t *testing.T) {
	for _, bad := range []string{"", "a */ DROP TABLE x; /*", "a b", strings.Repeat("a", 65), "x\x00y", "a--b"} {
		_, err := q.Render(q.Dialect{Version: q.PG18}, q.Tagged(bad, q.Select(Users.ID.As("id")).From(Users)))
		if !errors.Is(err, q.ErrInvalidIdent) {
			t.Errorf("%q: want ErrInvalidIdent, got %v", bad, err)
		}
	}
}

func TestFingerprintIsPerShape(t *testing.T) {
	p := q.NewParam[int64]()
	a, _ := q.Render(q.Dialect{Version: q.PG18}, q.Select(Users.ID.As("id")).From(Users).Where(Users.ID.EqX(p)))
	b, _ := q.Render(q.Dialect{Version: q.PG18}, q.Select(Users.ID.As("id")).From(Users).Where(Users.ID.Eq(5)))
	c, _ := q.Render(q.Dialect{Version: q.PG18}, q.Select(Users.ID.As("id")).From(Users).Where(Users.ID.Eq(9)))
	if b.Fingerprint() != c.Fingerprint() {
		t.Error("same shape, different values: fingerprints must match")
	}
	if a.Fingerprint() == "" || len(a.Fingerprint()) > 16 {
		t.Errorf("fingerprint %q", a.Fingerprint())
	}
}
