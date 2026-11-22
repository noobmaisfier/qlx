package qpage

import (
	"strings"
	"testing"
	"time"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
)

// posRow has no tags and is scanned by position, like a //qlx:row struct:
// field i receives column i of the SELECT.
type posRow struct {
	Created time.Time
	ID      int64
	Mail    string
}

func (r *posRow) ScanTargets() []any { return []any{&r.Created, &r.ID, &r.Mail} }

func posSorts() *SortSet[posRow] {
	return Sorts[posRow](
		Sort("created", Desc(Users.CreatedAt)),
		Sort("email", Desc(Users.Email)),
		Tiebreak(Desc(Users.ID)),
	)
}

// the SELECT is in the order of the fields: created_at, id, email
func posSelect() q.SelectQuery { return q.Select(Users.CreatedAt, Users.ID, Users.Email).From(Users) }

func posRows(rows []row) []posRow {
	out := make([]posRow, len(rows))
	for i, r := range rows {
		out[i] = posRow{r.Created, r.ID, r.Email}
	}
	return out
}

func TestWalkByPosition(t *testing.T) {
	setup(t)
	s := posSorts()
	all := table(11)
	for _, sort := range []string{"created", "email"} {
		var ids []int64
		cursor := ""
		for pages := 0; ; pages++ {
			if pages > 20 {
				t.Fatal("never ended")
			}
			w, err := s.Window(Request{Sort: sort, Cursor: cursor, Size: 4})
			if err != nil {
				t.Fatal(err)
			}
			var sqls []string
			rows := fakeSel(t, all, &w, &sqls, posSelect())
			page, err := s.Page(w, posRows(rows))
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range page.Items {
				ids = append(ids, r.ID)
			}
			if page.Next == "" {
				break
			}
			cursor = page.Next
		}
		if len(ids) != 11 {
			t.Fatalf("%s: %d rows", sort, len(ids))
		}
		for i, id := range ids {
			if id != all[i].ID && sort == "created" {
				t.Fatalf("created: row %d is %d, want %d", i, id, all[i].ID)
			}
		}
	}
}

// the same cursor is produced whichever way the row is scanned
func TestPositionAndNameMakeTheSameCursor(t *testing.T) {
	setup(t)
	all := table(6)
	byName := sorts()
	byPos := posSorts()
	var sqls []string
	w1, _ := byName.Window(Request{Sort: "created", Size: 2})
	p1, _ := byName.Page(w1, fake(t, all, &w1, &sqls))
	w2, _ := byPos.Window(Request{Sort: "created", Size: 2})
	if _, err := w2.Apply(posSelect()); err != nil {
		t.Fatal(err)
	}
	p2, err := byPos.Page(w2, posRows(all[:3]))
	if err != nil {
		t.Fatal(err)
	}
	if p1.Next != p2.Next {
		t.Errorf("by name %q, by position %q", p1.Next, p2.Next)
	}
}

func TestPositionChecks(t *testing.T) {
	setup(t)
	s := posSorts()
	w, _ := s.Window(Request{})
	for name, c := range map[string]struct {
		sel  q.SelectQuery
		want string
	}{
		"reordered select list": {q.Select(Users.ID, Users.CreatedAt, Users.Email).From(Users), "column 2 of the SELECT, where the row type holds a int64, not a time.Time"},
		"key not selected":      {q.Select(Users.ID, Users.Email).From(Users), "does not return"},
		"more columns than fields": {q.Select(Users.ID, Users.Email, Users.Balance, Users.Tier, Users.Nickname, Users.CreatedAt).From(Users),
			"column 6 of the SELECT, but the row type scans only 3"},
		"key twice": {q.Select(Users.CreatedAt, Users.ID, Users.ID).From(Users), "twice"},
	} {
		w := w
		if _, err := w.Apply(c.sel); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want it to mention %q", name, err, c.want)
		}
	}
	// Page without Apply cannot know where the keys are
	w, _ = s.Window(Request{Size: 1})
	if _, err := s.Page(w, posRows(table(3))); err == nil || !strings.Contains(err.Error(), "Apply") {
		t.Errorf("Page before Apply: %v", err)
	}
	// an unexpected ScanTargets length is reported, not indexed out of range
	w, _ = s.Window(Request{Size: 1})
	if _, err := w.Apply(posSelect()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Page(w, posRows(table(3))); err != nil {
		t.Errorf("a regular page after Apply: %v", err)
	}
}

type badTargets struct{ N int64 }

func (r *badTargets) ScanTargets() []any { return []any{r.N} } // not a pointer

func TestPositionDeclarations(t *testing.T) {
	setup(t)
	// no tag and no matching name is fine by position
	if _, err := newSorts[posRow]([]Option{Sort("a", Asc(Users.CreatedAt))}); err != nil {
		t.Errorf("a row scanned by position needs no tag: %v", err)
	}
	if _, err := newSorts[badTargets]([]Option{Sort("a", Asc(Users.ID))}); err == nil {
		t.Error("ScanTargets that does not return pointers must be refused")
	}
	// the other declaration errors still apply
	if _, err := newSorts[posRow]([]Option{Sort("a", Asc(Users.Balance.Add(1)))}); err == nil {
		t.Error("an expression key must be refused")
	}
	if _, err := newSorts[posRow]([]Option{Sort("a", Asc(Users.Nickname))}); err == nil {
		t.Error("a nullable key must be refused")
	}
}
