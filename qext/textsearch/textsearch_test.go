package textsearch_test

import (
	"strings"
	"testing"

	"github.com/noobmaisfier/qlx"
	"github.com/noobmaisfier/qlx/qext/textsearch"
	"github.com/noobmaisfier/qlx/qt"
)

func render(t *testing.T, s qlx.Statement) string {
	t.Helper()
	st, err := qlx.Render(qlx.Dialect{Version: qlx.PG18}, s)
	if err != nil {
		t.Fatal(err)
	}
	return st.SQL
}

func TestRenderedShapes(t *testing.T) {
	title, body := qlx.NewParam[string](), qlx.NewParam[qt.Null[string]]()
	q := qlx.NewParam[qt.TSQuery]()
	doc := qlx.NewParam[qt.TSVector]()
	text := qlx.NewParam[string]()
	for name, tc := range map[string]struct {
		sql  string
		want []string
	}{
		"web with config":     {render(t, qlx.Select(textsearch.Web("english", text))), []string{`"websearch_to_tsquery"(`, "::regconfig"}},
		"web default config":  {render(t, qlx.Select(textsearch.Web("", text))), []string{`"websearch_to_tsquery"(`}},
		"document two parts":  {render(t, qlx.Select(textsearch.Document("english", textsearch.Weighted(textsearch.A, title), textsearch.Weighted(textsearch.B, body)))), []string{`"setweight"(`, `"to_tsvector"(`, "coalesce(", "||"}},
		"match":               {render(t, qlx.Select(qlx.Test(textsearch.Match(doc, q)))), []string{"@@"}},
		"rank":                {render(t, qlx.Select(textsearch.Rank(doc, q))), []string{`"ts_rank"(`}},
		"headline with opts":  {render(t, qlx.Select(textsearch.Headline("english", text, q, textsearch.Highlight{StartSel: "<mark>", StopSel: "</mark>", MaxWords: 20}))), []string{`"ts_headline"(`}},
		"headline no options": {render(t, qlx.Select(textsearch.Headline("", text, q, textsearch.Highlight{}))), []string{`"ts_headline"(`}},
	} {
		for _, w := range tc.want {
			if !strings.Contains(tc.sql, w) {
				t.Errorf("%s: %q lacks %q", name, tc.sql, w)
			}
		}
		if strings.Contains(tc.sql, "<mark>") {
			t.Errorf("%s: option text was written into the SQL: %s", name, tc.sql)
		}
	}
}

func TestSearchIfIsAbsentWhenBlank(t *testing.T) {
	doc := qlx.NewParam[qt.TSVector]()
	for _, v := range []qt.Opt[string]{{}, qt.Of(""), qt.Of("   \t")} {
		if p := textsearch.SearchIf(doc, "english", v); p != (qlx.Pred{}) {
			t.Errorf("%+v should be absent", v)
		}
	}
	if textsearch.SearchIf(doc, "english", qt.Of("go")) == (qlx.Pred{}) {
		t.Error("a real search was dropped")
	}
}
