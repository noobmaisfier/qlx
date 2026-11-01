package regress

import (
	"errors"
	"strings"
	"testing"
)

const src = `-- header
\set x 1
CREATE TABLE t (a int);
INSERT INTO t VALUES (1), (2); SELECT 'a;b', "c;d" FROM t
  WHERE a > 1 /* ; */ ;
DO $$ BEGIN PERFORM 1; END $$;
SELECT $tag$ ; $tag$;
`

func TestSplit(t *testing.T) {
	ss := Split("f", src)
	if len(ss) != 5 {
		for _, s := range ss {
			t.Logf("%d: %q", s.Index, s.SQL)
		}
		t.Fatalf("got %d statements", len(ss))
	}
	if !strings.Contains(ss[2].SQL, "'a;b'") || !strings.Contains(ss[2].SQL, `"c;d"`) {
		t.Fatalf("quotes split: %q", ss[2].SQL)
	}
	if ss[1].Line != 4 {
		t.Fatalf("line = %d", ss[1].Line)
	}
}

func TestBuildAndRegressions(t *testing.T) {
	tr := func(sql string) (string, []string, error) {
		if strings.Contains(sql, "'a;b'") {
			return "", nil, errors.New("unsupported\nmore")
		}
		return "stmt := q.Select()", nil, nil
	}
	l := Build("x", map[string]string{"f": src}, tr)
	c := l.Counts()
	if c[SkippedDDL] != 2 || c[Untranslated] != 1 || c[Translated] != 2 {
		t.Fatalf("counts %v", c)
	}
	old := Build("x", map[string]string{"f": src}, func(string) (string, []string, error) { return "stmt := 1", nil, nil })
	if r := Regressions(old, l); len(r) != 1 {
		t.Fatalf("regressions %v", r)
	}
	if !strings.Contains(EmitCases("p", "", l, nil), "RegressCases") {
		t.Fatal("no cases emitted")
	}
}
