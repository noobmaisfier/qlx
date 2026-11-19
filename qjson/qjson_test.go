package qjson_test

import (
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/noobmaisfier/qlx/qjson"
	"github.com/noobmaisfier/qlx/qt"
)

type body struct {
	Email string `json:"email"`
	Age   int    `json:"age"`
}

func TestDecodeStrictByDefault(t *testing.T) {
	var v body
	err := qjson.DecodeInto(&v, strings.NewReader(`{"email":"a@b.c","age":3,"nick":"x"}`))
	var de *qjson.Error
	if !errors.As(err, &de) {
		t.Fatalf("want *qjson.Error, got %v", err)
	}
	if de.Code != qjson.CodeUnknownField || de.Field != "nick" {
		t.Fatalf("got %s / %q, want unknown_field / nick", de.Code, de.Field)
	}
	if !strings.Contains(de.Error(), "unknown_field (nick)") {
		t.Fatalf("message does not name the JSON key: %s", de.Error())
	}
}

func TestDecodeAllowUnknownFields(t *testing.T) {
	var v body
	if err := qjson.DecodeInto(&v, strings.NewReader(`{"email":"a@b.c","age":3,"nick":"x"}`),
		qjson.AllowUnknownFields()); err != nil {
		t.Fatalf("unknown field must be accepted: %v", err)
	}
	if v.Email != "a@b.c" || v.Age != 3 {
		t.Fatalf("decoded %+v", v)
	}
}

func TestDecodeOneValue(t *testing.T) {
	var v body
	err := qjson.DecodeInto(&v, strings.NewReader(`{"email":"a@b.c"}{"email":"x"}`))
	var de *qjson.Error
	if !errors.As(err, &de) || de.Code != qjson.CodeTrailingData {
		t.Fatalf("want trailing_data, got %v", err)
	}
}

func TestDecodeEmptyAndNull(t *testing.T) {
	for _, in := range []string{"", "  \n\t "} {
		var v body
		err := qjson.DecodeInto(&v, strings.NewReader(in))
		var de *qjson.Error
		if !errors.As(err, &de) || de.Code != qjson.CodeEmptyBody {
			t.Fatalf("%q: want empty_body, got %v", in, err)
		}
	}
	var v body
	err := qjson.DecodeInto(&v, strings.NewReader("null"))
	var de *qjson.Error
	if !errors.As(err, &de) || de.Code != qjson.CodeInvalidJSON {
		t.Fatalf("null: want invalid_json, got %v", err)
	}
}

func TestDecodeMaxBytes(t *testing.T) {
	in := `{"email":"` + strings.Repeat("a", 64) + `"}`
	var v body
	if err := qjson.DecodeInto(&v, strings.NewReader(in), qjson.MaxBytes(int64(len(in)))); err != nil {
		t.Fatalf("exactly at the limit must pass: %v", err)
	}
	err := qjson.DecodeInto(&v, strings.NewReader(in), qjson.MaxBytes(int64(len(in)-1)))
	var de *qjson.Error
	if !errors.As(err, &de) || de.Code != qjson.CodeTooLarge {
		t.Fatalf("want too_large, got %v", err)
	}
	if err := qjson.DecodeInto(&v, strings.NewReader(in), qjson.MaxBytes(0)); err != nil {
		t.Fatalf("MaxBytes(0) must disable the limit: %v", err)
	}
}

func TestDecodeWrongTypeNamesTheField(t *testing.T) {
	var v body
	err := qjson.DecodeInto(&v, strings.NewReader(`{"age":"three"}`))
	var de *qjson.Error
	if !errors.As(err, &de) || de.Code != qjson.CodeInvalidJSON || de.Field != "age" {
		t.Fatalf("want invalid_json / age, got %v", err)
	}
}

func TestDecodeMalformed(t *testing.T) {
	var v body
	err := qjson.DecodeInto(&v, strings.NewReader(`{"age":`))
	var de *qjson.Error
	if !errors.As(err, &de) || de.Code != qjson.CodeInvalidJSON {
		t.Fatalf("want invalid_json, got %v", err)
	}
	if !strings.Contains(err.Error(), "middle") {
		t.Fatalf("truncated body must be explained: %v", err)
	}
}

type sentinelUnmarshaler struct{}

var errRefused = errors.New("refused by the type")

func (sentinelUnmarshaler) UnmarshalJSON([]byte) error { return errRefused }

func TestDecodeKeepsTheUnderlyingError(t *testing.T) {
	var v struct {
		V sentinelUnmarshaler `json:"v"`
	}
	err := qjson.DecodeInto(&v, strings.NewReader(`{"v":1}`))
	if !errors.Is(err, errRefused) {
		t.Fatalf("errors.Is must see through *qjson.Error: %v", err)
	}
	var de *qjson.Error
	if !errors.As(err, &de) || de.Field != "v" {
		t.Fatalf("a failure inside UnmarshalJSON must still name the field, got %v", err)
	}
}

func TestDecodeNamesFieldInsideUnmarshalJSON(t *testing.T) {
	type patch struct {
		Email qt.Opt[string] `json:"email"`
		When  qt.Date        `json:"when"`
	}

	var p patch
	err := qjson.DecodeInto(&p, strings.NewReader(`{"email":12}`))
	var de *qjson.Error
	if !errors.As(err, &de) || de.Code != qjson.CodeInvalidJSON || de.Field != "email" {
		t.Fatalf("wrong type inside Opt: want invalid_json / email, got %v", err)
	}

	err = qjson.DecodeInto(&p, strings.NewReader(`{"when":"not-a-date"}`))
	if !errors.As(err, &de) || de.Code != qjson.CodeInvalidJSON || de.Field != "when" {
		t.Fatalf("refused value inside Date: want invalid_json / when, got %v", err)
	}

	// A valid body is untouched, and the values land in the fields.
	if err := qjson.DecodeInto(&p, strings.NewReader(`{"email":"a@b.c","when":"2024-02-29"}`)); err != nil {
		t.Fatalf("valid body: %v", err)
	}
	if !p.Email.IsSet() || p.Email.V != "a@b.c" || p.When.IsZero() {
		t.Fatalf("decoded %+v", p)
	}
}

func TestDecodeRequestContentType(t *testing.T) {
	newReq := func(ct string) *http.Request {
		r, _ := http.NewRequest(http.MethodPost, "/", strings.NewReader(`{"email":"a@b.c"}`))
		if ct != "" {
			r.Header.Set("Content-Type", ct)
		}
		return r
	}

	if got, err := qjson.DecodeRequest[body](newReq("application/json; charset=utf-8")); err != nil || got.Email != "a@b.c" {
		t.Fatalf("application/json: %+v %v", got, err)
	}
	if got, err := qjson.DecodeRequest[body](newReq("application/vnd.qlx+json")); err != nil || got.Email != "a@b.c" {
		t.Fatalf("+json: %+v %v", got, err)
	}
	for _, ct := range []string{"", "text/plain"} {
		_, err := qjson.DecodeRequest[body](newReq(ct))
		var de *qjson.Error
		if !errors.As(err, &de) || de.Code != qjson.CodeUnsupportedMediaType {
			t.Fatalf("%q: want unsupported_media_type, got %v", ct, err)
		}
	}
	if _, err := qjson.DecodeRequest[body](newReq("text/plain"), qjson.AnyContentType()); err != nil {
		t.Fatalf("AnyContentType: %v", err)
	}
}

type named struct {
	ID      int    `json:"id"`
	Email   string `json:"email,omitempty"`
	NoTag   string
	Ignored string `json:"-"`
}

func TestJSONNameAndValidatorName(t *testing.T) {
	typ := reflect.TypeOf(named{})
	cases := []struct {
		field    string
		json     string
		validate string
	}{
		{"ID", "id", "id"},
		{"Email", "email", "email"},
		{"NoTag", "NoTag", "NoTag"},
		{"Ignored", "", "Ignored"},
	}
	for _, c := range cases {
		f, _ := typ.FieldByName(c.field)
		if got := qjson.JSONName(f); got != c.json {
			t.Errorf("JSONName(%s) = %q, want %q", c.field, got, c.json)
		}
		if got := qjson.ValidatorName(f); got != c.validate {
			t.Errorf("ValidatorName(%s) = %q, want %q", c.field, got, c.validate)
		}
	}
}

type selfValidating struct{ Err error }

func (s selfValidating) Validate() error { return s.Err }

func TestValidate(t *testing.T) {
	if err := qjson.Validate(selfValidating{}); err != nil {
		t.Fatalf("nil error: %v", err)
	}
	want := errors.New("nope")
	if err := qjson.Validate(selfValidating{Err: want}); !errors.Is(err, want) {
		t.Fatalf("Validate must return the DTO error: %v", err)
	}
	if err := qjson.Validate(42); err != nil {
		t.Fatalf("a value without Validate must pass: %v", err)
	}
}
