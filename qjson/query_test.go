package qjson_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/noobmaisfier/qlx/qjson"
	"github.com/noobmaisfier/qlx/qt"
)

func init() {
	if err := qt.SetIDKey([]byte("0123456789abcdef-test-key")); err != nil {
		panic(err)
	}
}

type qUsers struct{}

func (qUsers) IDSpace() string { return "public.users" }

type qBase struct {
	Limit int `query:"limit"`
}

type qFilter struct {
	qBase                         // flattened
	Tier    *string               `query:"tier"`
	Search  qt.Opt[string]        `form:"q"`
	Active  qt.Opt[bool]          `json:"active,omitempty"`
	Owner   qt.Opt[qt.ID[qUsers]] `query:"owner"`
	IDs     []qt.ID[qUsers]       `query:"id"`
	Since   qt.Opt[qt.Date]       `query:"since"`
	Timeout time.Duration         `query:"timeout"`
	Score   float64               // lower-cased field name: "score"
	Skip    string                `query:"-"`
	Small   int8                  `query:"small"`
	N       uint                  `query:"n"`
	hidden  string
}

func get(t *testing.T, rawQuery string, opts ...qjson.Option) (qFilter, error) {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/users?"+rawQuery, nil)
	return qjson.DecodeQuery[qFilter](r, opts...)
}

func asErr(t *testing.T, err error) *qjson.Error {
	t.Helper()
	var de *qjson.Error
	if !errors.As(err, &de) {
		t.Fatalf("want *qjson.Error, got %v", err)
	}
	return de
}

func TestDecodeQueryHappy(t *testing.T) {
	id := qt.IDOf[qUsers](42)
	idText, _ := id.MarshalText()
	f, err := get(t, "limit=25&tier=gold&q=ann&active=true&owner="+string(idText)+
		"&id="+string(idText)+"&id="+string(idText)+"&since=2026-01-02&timeout=90s&score=1.5&small=-7&n=9")
	if err != nil {
		t.Fatal(err)
	}
	if f.Limit != 25 || f.Tier == nil || *f.Tier != "gold" {
		t.Fatalf("limit/tier: %+v", f)
	}
	if !f.Search.Set || f.Search.V != "ann" || !f.Active.Set || !f.Active.V {
		t.Fatalf("opts: %+v %+v", f.Search, f.Active)
	}
	if !f.Owner.Set || f.Owner.V != id || len(f.IDs) != 2 || f.IDs[1] != id {
		t.Fatalf("ids: %+v %+v", f.Owner, f.IDs)
	}
	if !f.Since.Set || f.Timeout != 90*time.Second || f.Score != 1.5 || f.Small != -7 || f.N != 9 {
		t.Fatalf("rest: %+v", f)
	}
}

func TestDecodeQueryAbsentStaysZero(t *testing.T) {
	f, err := get(t, "")
	if err != nil {
		t.Fatal(err)
	}
	if f.Tier != nil || f.Search.Set || f.IDs != nil || f.Limit != 0 {
		t.Fatalf("not zero: %+v", f)
	}
}

func TestDecodeQueryEmptyIsAbsentByDefault(t *testing.T) {
	f, err := get(t, "tier=&q=&limit=&id=")
	if err != nil {
		t.Fatal(err)
	}
	if f.Tier != nil || f.Search.Set || f.IDs != nil {
		t.Fatalf("empty should be absent: %+v", f)
	}
	f, err = get(t, "tier=&q=", qjson.KeepEmptyParams())
	if err != nil {
		t.Fatal(err)
	}
	if f.Tier == nil || *f.Tier != "" || !f.Search.Set || f.Search.V != "" {
		t.Fatalf("empty should be kept: %+v %+v", f.Tier, f.Search)
	}
	if _, err = get(t, "limit=", qjson.KeepEmptyParams()); asErr(t, err).Code != qjson.CodeInvalidParam {
		t.Fatalf("empty int kept should be invalid: %v", err)
	}
}

func TestDecodeQueryErrors(t *testing.T) {
	for _, tc := range []struct {
		q, code, field string
	}{
		{"limit=abc", qjson.CodeInvalidParam, "limit"},
		{"limit=1&limit=2", qjson.CodeInvalidParam, "limit"},
		{"small=300", qjson.CodeInvalidParam, "small"},
		{"n=-1", qjson.CodeInvalidParam, "n"},
		{"active=maybe", qjson.CodeInvalidParam, "active"},
		{"owner=notanid", qjson.CodeInvalidParam, "owner"},
		{"id=zzz", qjson.CodeInvalidParam, "id"},
		{"since=yesterday", qjson.CodeInvalidParam, "since"},
		{"timeout=soon", qjson.CodeInvalidParam, "timeout"},
		{"score=NaNx", qjson.CodeInvalidParam, "score"},
		{"nope=1", qjson.CodeUnknownParam, "nope"},
		{"skip=1", qjson.CodeUnknownParam, "skip"},
		{"hidden=1", qjson.CodeUnknownParam, "hidden"},
		{"limit=1&b=1&a=1", qjson.CodeUnknownParam, "a"}, // first unknown, sorted
		{"%zz=1", qjson.CodeInvalidQuery, ""},
	} {
		_, err := get2(t, tc.q)
		de := asErr(t, err)
		if de.Code != tc.code || de.Field != tc.field {
			t.Errorf("%q: got %s/%q, want %s/%q (%v)", tc.q, de.Code, de.Field, tc.code, tc.field, err)
		}
	}
}

// get2 builds the request by hand: httptest.NewRequest rejects a malformed
// query in the target.
func get2(t *testing.T, rawQuery string) (qFilter, error) {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/users", nil)
	r.URL.RawQuery = rawQuery
	return qjson.DecodeQuery[qFilter](r)
}

func TestDecodeQueryAllowUnknown(t *testing.T) {
	f, err := get(t, "limit=3&utm_source=x", qjson.AllowUnknownFields())
	if err != nil || f.Limit != 3 {
		t.Fatalf("%+v %v", f, err)
	}
}

func TestDecodeQueryIDErrorKept(t *testing.T) {
	_, err := get(t, "owner=notanid")
	if !errors.Is(err, qt.ErrBadID) {
		t.Fatalf("errors.Is(ErrBadID) = false for %v", err)
	}
}

func TestDecodeQueryBadStructs(t *testing.T) {
	type nested struct{ A struct{ X int } }
	type slice struct{ B []byte }
	type dup struct {
		A int `query:"x"`
		B int `form:"x"`
	}
	type mapf struct{ M map[string]string }
	for name, f := range map[string]func() error{
		"struct field": func() error { _, e := qjson.DecodeQuery[nested](req()); return e },
		"byte slice":   func() error { _, e := qjson.DecodeQuery[slice](req()); return e },
		"duplicate":    func() error { _, e := qjson.DecodeQuery[dup](req()); return e },
		"map":          func() error { _, e := qjson.DecodeQuery[mapf](req()); return e },
		"not a struct": func() error { _, e := qjson.DecodeQuery[int](req()); return e },
	} {
		err := f()
		if err == nil {
			t.Errorf("%s: no error", name)
			continue
		}
		var de *qjson.Error
		if errors.As(err, &de) {
			t.Errorf("%s: a bad struct is a plain error, got *Error %v", name, err)
		}
	}
	if err := qjson.DecodeQueryInto(nil, url.Values{}); err == nil {
		t.Error("nil dst accepted")
	}
}

func req() *http.Request { return httptest.NewRequest(http.MethodGet, "/x", nil) }

func TestQueryName(t *testing.T) {
	typ := reflect.TypeOf(qFilter{})
	want := map[string]string{
		"Tier": "tier", "Search": "q", "Active": "active", "Score": "score",
		"Skip": "", "hidden": "",
	}
	for field, name := range want {
		f, _ := typ.FieldByName(field)
		if got := qjson.QueryName(f); got != name {
			t.Errorf("%s: got %q want %q", field, got, name)
		}
	}
}

func TestDecodeQueryOptNullRefused(t *testing.T) {
	type p struct {
		//qlx:ignore query-only struct testing opt-null refusal
		X qt.Opt[qt.Null[string]] `query:"x" json:",omitzero"`
	}
	vals := url.Values{"x": {"a"}}
	var v p
	err := qjson.DecodeQueryInto(&v, vals)
	if asErr(t, err).Code != qjson.CodeInvalidParam {
		t.Fatalf("got %v", err)
	}
}

func TestDecodeQueryConcurrentPlanCache(t *testing.T) {
	done := make(chan struct{})
	for i := 0; i < 8; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 200; j++ {
				if _, err := get(t, "limit=1&q=a"); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	for i := 0; i < 8; i++ {
		<-done
	}
}
