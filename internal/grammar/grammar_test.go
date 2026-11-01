package grammar

import (
	"strings"
	"testing"
)

const sample = `
%{ int x; %}
%token A B
%%
stmt: SelectStmt | InsertStmt | /*EMPTY*/ { $$ = NULL; } ;
SelectStmt: select_no_parens %prec UMINUS
	| select_with_parens
	;
select_with_parens: '(' select_no_parens ')' { $$ = $2; /* } */ }
	| '(' select_with_parens ')'
	;
%%
tail
`

func TestParse(t *testing.T) {
	g, err := Parse(sample)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(g.Rules["stmt"]); got != 3 {
		t.Fatalf("stmt alternatives = %d", got)
	}
	if g.Rules["SelectStmt"][0].Alt != "select_no_parens" {
		t.Fatalf("%%prec not dropped: %q", g.Rules["SelectStmt"][0].Alt)
	}
	if g.Rules["select_with_parens"][0].Alt != "'(' select_no_parens ')'" {
		t.Fatalf("action not dropped: %q", g.Rules["select_with_parens"][0].Alt)
	}
}

func TestCheck(t *testing.T) {
	g, _ := Parse(sample)
	l, err := Build(g, "sample", []string{"SelectStmt"})
	if err != nil {
		t.Fatal(err)
	}
	m := &Map{Statements: map[string]Entry{}, Productions: map[string]Entry{}}
	if n := Fill(l, m); n != 4 {
		t.Fatalf("Fill added %d", n)
	}
	if ps := Check(l, m); len(ps) != 4 {
		t.Fatalf("unmapped entries should fail: %v", ps)
	}
	m.Statements["SelectStmt"] = Entry{Status: "supported", Symbol: "qlx.Select"}
	m.Statements["InsertStmt"] = Entry{Status: "unsupported"}
	ps := Check(l, m)
	var all []string
	for _, p := range ps {
		all = append(all, p.String())
	}
	if !strings.Contains(strings.Join(all, "\n"), "needs a reason") {
		t.Fatalf("missing reason not reported: %v", ps)
	}
	m.Statements["Gone"] = Entry{Status: "supported", Symbol: "x"}
	found := false
	for _, p := range Check(l, m) {
		if strings.Contains(p.Msg, "stale") {
			found = true
		}
	}
	if !found {
		t.Fatal("stale entry not reported")
	}
}
