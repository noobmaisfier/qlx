package catalog

import (
	"reflect"
	"testing"
)

func TestTextArrayNamesVariablesInsteadOfBuildingLiterals(t *testing.T) {
	expr, vars, err := textArray([]string{"public", "App$1"})
	if err != nil {
		t.Fatal(err)
	}
	want := "ARRAY[:'schema_0',:'schema_1']::text[]"
	if expr != want {
		t.Errorf("expression = %q, want %q", expr, want)
	}
	wantVars := map[string]string{"schema_0": "public", "schema_1": "App$1"}
	if !reflect.DeepEqual(vars, wantVars) {
		t.Errorf("variables = %v, want %v", vars, wantVars)
	}
}

func TestTextArrayRejectsNamesThatCouldBreakOut(t *testing.T) {
	for _, s := range []string{"public'; DROP TABLE x; --", "", "a b", `a"b`, "a.b"} {
		if expr, _, err := textArray([]string{s}); err == nil {
			t.Errorf("textArray(%q) = %q, want an error", s, expr)
		}
	}
}

func TestTextArrayHandlesNoSchemas(t *testing.T) {
	expr, vars, err := textArray(nil)
	if err != nil {
		t.Fatal(err)
	}
	if expr != "ARRAY[]::text[]" || vars != nil {
		t.Errorf("textArray(nil) = %q, %v; want ARRAY[]::text[] and no variables", expr, vars)
	}
}
