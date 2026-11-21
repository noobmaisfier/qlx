package catalog

import (
	"os"
	"strings"
	"testing"
)

// The stored query tree of
//
//	SELECT a.id, a.email, i.qty, i.id AS item_id, 1 AS one, COALESCE(i.qty,0) AS q0,
//	       count(*) OVER () AS c, a.balance::bal_nn AS bnn
//	FROM accounts a LEFT JOIN items i ON i.account_id = a.id
//
// as PostgreSQL 16 prints it (testdata/view_left_join.pg16.tree).
func TestInferFromAPostgres16Tree(t *testing.T) {
	raw, err := os.ReadFile("testdata/view_left_join.pg16.tree")
	if err != nil {
		t.Fatal(err)
	}
	tree := strings.TrimSpace(string(raw))
	const accounts, items, notNullDomain = 294913, 303106, 303105
	facts := viewFacts{
		Views:   []viewDef{{OID: 1, Schema: "vw", Name: "j", Tree: tree}},
		Attrs:   [][2]int64{{accounts, 1}, {accounts, 2}, {accounts, 3}, {items, 1}, {items, 3}},
		Domains: []uint32{notNullDomain},
	}
	got := inferViewNotNull(facts)[1]
	want := map[int]string{1: "id", 2: "email", 5: "one", 6: "q0", 7: "c", 8: "bnn"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("column %d: got %q, want %q (all: %v)", k, got[k], v, got)
		}
	}
	// qty (3) and item_id (4) come from the nullable side of the LEFT JOIN.
	if _, ok := got[3]; ok {
		t.Error("qty is on the nullable side of a LEFT JOIN")
	}
	if _, ok := got[4]; ok {
		t.Error("item_id is on the nullable side of a LEFT JOIN")
	}

	// Without the domain, and with accounts.balance not NOT NULL, the cast proves
	// nothing.
	facts.Domains = nil
	facts.Attrs = [][2]int64{{accounts, 1}}
	got = inferViewNotNull(facts)[1]
	if _, ok := got[8]; ok {
		t.Errorf("bnn must not be proven without the domain or a NOT NULL balance: %v", got)
	}
}

func TestInferBailsOutOnWhatItDoesNotUnderstand(t *testing.T) {
	for name, tree := range map[string]string{
		"garbage":     "not a tree",
		"unbalanced":  "({QUERY :commandType 1",
		"two queries": "({QUERY :commandType 1} {QUERY :commandType 1})",
		"not select":  "({QUERY :commandType 2 :rtable <> :targetList <>})",
		"union":       "({QUERY :commandType 1 :setOperations {SETOPERATIONSTMT :op 1} :rtable <> :targetList <>})",
		"rollup":      "({QUERY :commandType 1 :groupingSets ({GROUPINGSET :kind 1}) :rtable <> :targetList <>})",
		"srf":         "({QUERY :commandType 1 :hasTargetSRFs true :rtable <> :targetList <>})",
	} {
		got := inferViewNotNull(viewFacts{Views: []viewDef{{OID: 1, Name: name, Tree: tree}}})
		if len(got) != 0 {
			t.Errorf("%s: proved %v from a tree it cannot trust", name, got)
		}
	}
}

func TestInferVarsAreNotTrustedWhenAnythingIsOff(t *testing.T) {
	// One base relation (oid 10) with a NOT NULL column 1, referenced by a Var.
	q := func(varFields string) string {
		return "({QUERY :commandType 1 :rtable ({RANGETBLENTRY :rtekind 0 :relid 10}) " +
			":targetList ({TARGETENTRY :expr {VAR :varno 1 :varattno 1 " + varFields + "} :resno 1 :resname x :resjunk false})})"
	}
	facts := func(tree string) viewFacts {
		return viewFacts{Views: []viewDef{{OID: 1, Tree: tree}}, Attrs: [][2]int64{{10, 1}}}
	}
	if got := inferViewNotNull(facts(q(":varnullingrels (b) :varlevelsup 0")))[1]; got[1] != "x" {
		t.Fatalf("the baseline must be proven: %v", got)
	}
	for name, fields := range map[string]string{
		"nulled by an outer join": ":varnullingrels (b 3) :varlevelsup 0",
		"no nullingrels field":    ":varlevelsup 0",
		"outer query":             ":varnullingrels (b) :varlevelsup 1",
	} {
		if got := inferViewNotNull(facts(q(fields)))[1]; len(got) != 0 {
			t.Errorf("%s: proved %v", name, got)
		}
	}
}

func TestParserHandlesEscapesAndDatums(t *testing.T) {
	v, err := parseNodeTree(`({A :resname my\ col :constvalue 4 [ 1 0 0 0 ] :x <> :l (b 1 2)})`)
	if err != nil {
		t.Fatal(err)
	}
	n := v.([]any)[0].(*treeNode)
	if atom(n.f["resname"]) != "my col" || atom(n.f["constvalue"]) != "4" || n.f["x"] != nil || len(n.f["l"].([]any)) != 3 {
		t.Errorf("parsed wrongly: %+v", n.f)
	}
}
