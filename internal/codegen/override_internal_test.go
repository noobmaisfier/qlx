package codegen

import (
	"testing"

	"github.com/noobmaisfier/qlx/internal/catalog"
)

// An override to a slice or map type is not wrapped in qt.Null, like the
// built-in arrays: nil stands for NULL. Other overrides still are.
func TestOverrideSliceTypesAreNotWrappedInNull(t *testing.T) {
	m := &mapper{over: Overrides{
		"tags":   {Type: "[]string"},
		"attrs":  {Type: "map[string]string"},
		"amount": {Type: "example.com/app/money.Amount", Family: "num"},
	}}
	for name, wantNullOK := range map[string]bool{"tags": false, "attrs": false, "amount": true} {
		g, ok := m.override(catalog.Type{Schema: "public", Name: name})
		if !ok || g.nullOK != wantNullOK {
			t.Errorf("%s: ok=%v nullOK=%v, want nullOK=%v", name, ok, g.nullOK, wantNullOK)
		}
	}
}
