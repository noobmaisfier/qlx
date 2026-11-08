package pgvector_test

import (
	"strings"
	"testing"

	"github.com/noobmaisfier/qlx"
	"github.com/noobmaisfier/qlx/qext/pgvector"
	"github.com/noobmaisfier/qlx/qt"
)

func TestOperatorsAreUsedAndTheQueryIsCast(t *testing.T) {
	v := qlx.NewParam[qt.Vector]()
	h := qlx.NewParam[qt.HalfVector]()
	for name, tc := range map[string]struct {
		e    qlx.Selectable
		want string
	}{
		"l2":     {pgvector.L2(v, qt.Vector{1, 2}), "<->"},
		"cosine": {pgvector.Cosine(v, qt.Vector{1, 2}), "<=>"},
		"ip":     {pgvector.NegativeInnerProduct(h, qt.HalfVector{1, 2}), "<#>"},
		"l1":     {pgvector.L1(h, qt.HalfVector{1, 2}), "<+>"},
	} {
		st, err := qlx.Render(qlx.Dialect{Version: qlx.PG18}, qlx.Select(tc.e))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !strings.Contains(st.SQL, tc.want) || !strings.Contains(st.SQL, "CAST(") || strings.Contains(st.SQL, "_distance") {
			t.Errorf("%s: %s", name, st.SQL)
		}
	}
	for _, c := range []qlx.Command{pgvector.SetEfSearch(100), pgvector.SetProbes(10)} {
		if _, err := qlx.Render(qlx.Dialect{Version: qlx.PG18}, c); err != nil {
			t.Error(err)
		}
	}
}
