package codegen

import "testing"

func TestOddNamesUnit(t *testing.T) {
	for in, want := range map[string]string{"_": "X", "日本語": "X日本語", "1st": "X1st", "user_id": "UserID", "ñ": "Ñ", "{}": "X"} {
		if got := camel(in); got != want {
			t.Errorf("camel(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"plain": "`db:\"plain\"`", "a`b": "\"db:\\\"a`b\\\"\""} {
		if got := dbTag(in); got != want {
			t.Errorf("dbTag(%q) = %s, want %s", in, got, want)
		}
	}
	taken := map[string][]byte{"a_b.go": nil, "enums.go": nil}
	for in, want := range map[string]string{"a_b": "a_b_2", "_": "t_", "": "t", "_x": "t_x", "enums": "enums_2", "fresh": "fresh"} {
		if got := uniqueFile(in, taken); got != want {
			t.Errorf("uniqueFile(%q) = %q, want %q", in, got, want)
		}
	}
}
