package trgm_test

import (
	"strings"
	"testing"

	"github.com/noobmaisfier/qlx"
	"github.com/noobmaisfier/qlx/qext/trgm"
	"github.com/noobmaisfier/qlx/qt"
)

func TestOperatorsAndAbsentFilter(t *testing.T) {
	name := qlx.NewParam[string]()
	st, err := qlx.Render(qlx.Dialect{Version: qlx.PG18}, qlx.Select(qlx.Test(trgm.Similar(name, "jon")), trgm.Distance(name, "jon")))
	if err != nil || !strings.Contains(st.SQL, "%") || !strings.Contains(st.SQL, "<->") {
		t.Fatalf("%v %s", err, st.SQL)
	}
	for _, v := range []qt.Opt[string]{{}, qt.Of("")} {
		if trgm.SimilarIf(name, v) != (qlx.Pred{}) {
			t.Errorf("%+v should be absent", v)
		}
	}
	if trgm.SimilarIf(name, qt.Of("jon")) == (qlx.Pred{}) {
		t.Error("a real filter was dropped")
	}
}
