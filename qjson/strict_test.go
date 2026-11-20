package qjson_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/noobmaisfier/qlx/qjson"
	"github.com/noobmaisfier/qlx/qt"
)

type settings struct {
	Theme string `json:"theme"`
	Size  int    `json:"size"`
}

type base struct {
	ID string `json:"id"`
}

type nested struct {
	base
	Settings qt.JSONB[settings]          `json:"settings"`
	Patch    qt.Opt[settings]            `json:"patch,omitzero"`
	Maybe    qt.Opt[qt.Null[settings]]   `json:"maybe,omitzero"`
	Span     qt.Range[int32]             `json:"span"`
	List     []qt.JSONB[settings]        `json:"list"`
	ByName   map[string]qt.Opt[settings] `json:"by_name"`
	Raw      qt.RawJSON                  `json:"raw"`
	Any      any                         `json:"any"`
	Ptr      *qt.JSONB[settings]         `json:"ptr"`
	Plain    settings                    `json:"plain"`
}

func TestStrictReachesInsideQtWrappers(t *testing.T) {
	cases := []struct {
		name, body, field string
	}{
		{"jsonb", `{"settings":{"theme":"x","theme_typo":1}}`, "settings.theme_typo"},
		{"opt struct", `{"patch":{"colour":"x"}}`, "patch.colour"},
		{"opt null struct", `{"maybe":{"nope":1}}`, "maybe.nope"},
		{"range", `{"span":{"lower":1,"upper":5,"lower_incl":true}}`, "span.lower_incl"},
		{"slice element", `{"list":[{"theme":"a"},{"bad":1}]}`, "list[1].bad"},
		{"map value", `{"by_name":{"k":{"bad":1}}}`, "by_name.k.bad"},
		{"pointer", `{"ptr":{"bad":1}}`, "ptr.bad"},
		{"embedded", `{"id":"1","oops":1}`, "oops"},
		{"plain still caught", `{"plain":{"bad":1}}`, "plain.bad"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := qjson.Decode[nested](strings.NewReader(c.body))
			var e *qjson.Error
			if !errors.As(err, &e) || e.Code != qjson.CodeUnknownField {
				t.Fatalf("want unknown_field, got %v", err)
			}
			if c.name == "plain still caught" {
				return // reported by encoding/json under its own name
			}
			if e.Field != c.field {
				t.Errorf("field = %q, want %q", e.Field, c.field)
			}
		})
	}
}

func TestStrictAcceptsValidNestedValues(t *testing.T) {
	body := `{"id":"1","settings":{"theme":"dark","size":2},"patch":{"theme":"x"},
	"maybe":null,"span":{"lower":1,"upper":5,"lower_inc":true,"upper_inc":false},
	"span2":null,"list":[{"theme":"a"}],"by_name":{"k":{"size":1}},
	"raw":{"anything":{"goes":1}},"any":{"free":1},"ptr":{"theme":"p"},"plain":{"size":3}}`
	body = strings.Replace(body, `"span2":null,`, "", 1)
	v, err := qjson.Decode[nested](strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if v.Settings.V.Theme != "dark" || !v.Patch.Set || !v.Maybe.Set || !v.Maybe.V.IsNull() {
		t.Errorf("decoded wrongly: %+v", v)
	}
}

func TestStrictCaseInsensitiveLikeEncodingJSON(t *testing.T) {
	if _, err := qjson.Decode[nested](strings.NewReader(`{"settings":{"THEME":"x"}}`)); err != nil {
		t.Errorf("encoding/json matches keys case-insensitively, so must the nested check: %v", err)
	}
}

func TestAllowUnknownFieldsSkipsNestedCheck(t *testing.T) {
	_, err := qjson.Decode[nested](strings.NewReader(`{"settings":{"bad":1}}`), qjson.AllowUnknownFields())
	if err != nil {
		t.Fatal(err)
	}
}

func TestStrictNoWrapperNoWalk(t *testing.T) {
	type plain struct {
		A string `json:"a"`
	}
	if _, err := qjson.Decode[plain](strings.NewReader(`{"a":"x"}`)); err != nil {
		t.Fatal(err)
	}
}

func TestStrictReachesMultirangeAndNestedWrappers(t *testing.T) {
	type body struct {
		M    qt.Multirange[int32]         `json:"m"`
		OptM qt.Opt[qt.Multirange[int32]] `json:"opt_m,omitzero"`
		Many qt.Opt[[]qt.JSONB[settings]] `json:"many,omitzero"`
		Deep qt.JSONB[struct {
			In qt.Opt[settings] `json:"in,omitzero"`
		}] `json:"deep"`
	}
	cases := []struct{ name, body, field string }{
		{"multirange", `{"m":[{"lower":1,"upper":2},{"lower":3,"bad":1}]}`, "m[1].bad"},
		{"opt multirange", `{"opt_m":[{"nope":1}]}`, "opt_m[0].nope"},
		{"opt slice of jsonb", `{"many":[{"theme":"a"},{"bad":1}]}`, "many[1].bad"},
		{"three levels", `{"deep":{"in":{"theme":"x","bad":1}}}`, "deep.in.bad"},
		{"deep top level", `{"deep":{"out":1}}`, "deep.out"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := qjson.Decode[body](strings.NewReader(c.body))
			var e *qjson.Error
			if !errors.As(err, &e) || e.Code != qjson.CodeUnknownField || e.Field != c.field {
				t.Fatalf("want unknown_field at %q, got %v", c.field, err)
			}
		})
	}
	ok := `{"m":[{"lower":1,"upper":2}],"opt_m":[],"many":[{"size":1}],"deep":{"in":{"theme":"x"}}}`
	if _, err := qjson.Decode[body](strings.NewReader(ok)); err != nil {
		t.Fatalf("valid body refused: %v", err)
	}
}

// Embedding is not limited to a fixed depth: a field promoted through ten
// levels must stay a known key.
type deep9 struct {
	Z string `json:"z"`
}
type deep8 struct{ deep9 }
type deep7 struct{ deep8 }
type deep6 struct{ deep7 }
type deep5 struct{ deep6 }
type deep4 struct{ deep5 }
type deep3 struct{ deep4 }
type deep2 struct{ deep3 }
type deep1 struct{ deep2 }
type deep0 struct{ deep1 }

func TestStrictDeepEmbedding(t *testing.T) {
	type body struct {
		deep0
		P qt.Opt[string] `json:"p"`
	}
	if _, err := qjson.Decode[body](strings.NewReader(`{"z":"ok","p":"x"}`)); err != nil {
		t.Fatalf("a deeply promoted field is known: %v", err)
	}
}

// A self-embedding pointer recurses in the type, not in the field list.
type recursive struct {
	*recursive
	X int `json:"x"`
}

func TestStrictRecursiveEmbedding(t *testing.T) {
	if _, err := qjson.Decode[recursive](strings.NewReader(`{"x":1}`)); err != nil {
		t.Fatalf("recursive embedding: %v", err)
	}
}

func TestStrictNullOfStruct(t *testing.T) {
	type body struct {
		N qt.Null[settings] `json:"n"`
	}
	_, err := qjson.Decode[body](strings.NewReader(`{"n":{"bad":1}}`))
	var e *qjson.Error
	if !errors.As(err, &e) || e.Code != qjson.CodeUnknownField || e.Field != "n.bad" {
		t.Fatalf("want unknown_field at n.bad, got %v", err)
	}
	if _, err := qjson.Decode[body](strings.NewReader(`{"n":{"theme":"x"}}`)); err != nil {
		t.Fatalf("valid Null value refused: %v", err)
	}
}
