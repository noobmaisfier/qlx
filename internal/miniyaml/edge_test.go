package miniyaml

import (
	"reflect"
	"testing"
)

func TestEdgeCases(t *testing.T) {
	for src, want := range map[string]any{
		"\xEF\xBB\xBFa: 1\n":          map[string]any{"a": int64(1)},
		"a: don't # c\nb: 2\n":        map[string]any{"a": "don't", "b": int64(2)},
		"a: 'it''s' # c\n":            map[string]any{"a": "it's"},
		"a: [ 'x', don't ] # c\n":     map[string]any{"a": []any{"x", "don't"}},
		"a: \"x\\\\\" # c\n":          map[string]any{"a": "x\\"},
		"a: \"say \\\"hi\\\"\" # c\n": map[string]any{"a": `say "hi"`},
		"a: \"x # y\"\n":              map[string]any{"a": "x # y"},
		"a: x#y\n":                    map[string]any{"a": "x#y"},
	} {
		got, err := Parse([]byte(src))
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("%q = %#v, %v; want %#v", src, got, err, want)
		}
	}
}
