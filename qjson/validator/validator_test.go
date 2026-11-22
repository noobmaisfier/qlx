package qvalidator_test

import (
	"errors"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/noobmaisfier/qlx/qjson"
	"github.com/noobmaisfier/qlx/qjson/validator"
	"github.com/noobmaisfier/qlx/qt"
)

type userSpace struct{}

func (userSpace) IDSpace() string { return "public.users" }

type createUser struct {
	ID    qt.ID[userSpace] `json:"id" validate:"nonzero"`
	Email string           `json:"email" validate:"required,email"`
	Nick  qt.Null[string]  `json:"nickname" validate:"notnull"`
	Tier  qt.Opt[string]   `json:"tier" validate:"provided"`
	Age   int              `json:"age" validate:"min=18"`
}

func TestValidateNamesFieldsByJSONTag(t *testing.T) {
	v := qvalidator.New()
	var body createUser
	err := v.Validate(body)
	if err == nil {
		t.Fatal("the zero DTO must fail")
	}
	var fe qjson.Errors
	if !errors.As(err, &fe) {
		t.Fatalf("want qjson.Errors, got %T: %v", err, err)
	}
	want := map[string]string{
		"id":       "nonzero",
		"email":    "required",
		"nickname": "notnull",
		"tier":     "provided",
		"age":      "min",
	}
	if len(fe) != len(want) {
		t.Fatalf("got %d field errors: %v", len(fe), fe)
	}
	for _, f := range fe {
		code, ok := want[f.Field]
		if !ok {
			t.Fatalf("unexpected field %q", f.Field)
		}
		if f.Code != code {
			t.Fatalf("%s: got rule %q, want %q", f.Field, f.Code, code)
		}
		if f.Msg == "" {
			t.Fatalf("%s: empty message", f.Field)
		}
	}
}

func TestValidateAcceptsAFilledDTO(t *testing.T) {
	v := qvalidator.New()
	body := createUser{
		ID:    qt.IDOf[userSpace](1),
		Email: "ada@example.com",
		Nick:  qt.Some("ada"),
		Tier:  qt.Opt[string]{Set: true, V: "pro"},
		Age:   21,
	}
	if err := v.Validate(body); err != nil {
		t.Fatalf("filled DTO must pass: %v", err)
	}
}

func TestProvidedAcceptsExplicitNull(t *testing.T) {
	type patch struct {
		Tier qt.Opt[qt.Null[string]] `json:"tier" validate:"provided"`
	}
	v := qvalidator.New()
	body := patch{Tier: qt.Opt[qt.Null[string]]{Set: true, V: qt.None[string]()}}
	if err := v.Validate(body); err != nil {
		t.Fatalf("a present null must count as provided: %v", err)
	}
	if err := v.Validate(patch{}); err == nil {
		t.Fatal("an absent field must fail provided")
	}
}

func TestMessages(t *testing.T) {
	v := qvalidator.New().SetMessageFunc(func(fe validator.FieldError) string {
		return "custom:" + fe.Tag()
	})
	var body createUser
	err := v.Validate(body)
	var fe qjson.Errors
	if !errors.As(err, &fe) || len(fe) == 0 {
		t.Fatalf("want field errors, got %v", err)
	}
	for _, f := range fe {
		if f.Msg != "custom:"+f.Code {
			t.Fatalf("%s: got message %q", f.Field, f.Msg)
		}
	}
}

func TestRegisterOnExisting(t *testing.T) {
	v := validator.New()
	if err := qvalidator.Register(v); err != nil {
		t.Fatal(err)
	}
	if err := qvalidator.Register(v); err != nil {
		t.Fatalf("re-registering must replace the same rules: %v", err)
	}
	if err := qvalidator.Register(nil); err == nil {
		t.Fatal("Register(nil) must report the nil validator")
	}
}

func TestRegisterUsesJSONNames(t *testing.T) {
	v := validator.New()
	if err := qvalidator.Register(v); err != nil {
		t.Fatal(err)
	}
	type patch struct {
		Nick string `json:"nick_name" validate:"required"`
	}
	err := v.Struct(patch{})
	var ve validator.ValidationErrors
	if !errors.As(err, &ve) || len(ve) != 1 {
		t.Fatalf("want one validation error, got %v", err)
	}
	if got := ve[0].Field(); got != "nick_name" {
		t.Fatalf("field name = %q, want the json name", got)
	}
}

func TestOptUnwrapsContentRules(t *testing.T) {
	type patch struct {
		Email qt.Opt[string] `json:"email" validate:"provided,email"`
		Min   qt.Opt[string] `json:"min_name" validate:"provided,min=3"`
	}
	v := qvalidator.New()

	ok := patch{
		Email: qt.Opt[string]{Set: true, V: "ada@example.com"},
		Min:   qt.Opt[string]{Set: true, V: "abc"},
	}
	if err := v.Validate(ok); err != nil {
		t.Fatalf("a valid patch must pass: %v", err)
	}

	err := v.Validate(patch{
		Email: qt.Opt[string]{Set: true, V: "not-an-email"},
		Min:   qt.Opt[string]{Set: true, V: "ab"},
	})
	var fe qjson.Errors
	if !errors.As(err, &fe) {
		t.Fatalf("want qjson.Errors, got %T: %v", err, err)
	}
	want := map[string]string{"email": "email", "min_name": "min"}
	if len(fe) != len(want) {
		t.Fatalf("got %d field errors: %v", len(fe), fe)
	}
	for _, f := range fe {
		if code, ok := want[f.Field]; !ok || f.Code != code {
			t.Fatalf("unexpected error %+v", f)
		}
	}

	err = v.Validate(patch{})
	if !errors.As(err, &fe) || len(fe) != 2 {
		t.Fatalf("both absent fields must fail provided, got %v", err)
	}
	for _, f := range fe {
		if f.Code != qvalidator.TagProvided {
			t.Fatalf("%s: got %q, want provided", f.Field, f.Code)
		}
	}
}

func TestOptOmitEmptySkipsUnset(t *testing.T) {
	type patch struct {
		Email qt.Opt[string] `json:"email" validate:"omitempty,email"`
	}
	v := qvalidator.New()

	if err := v.Validate(patch{}); err != nil {
		t.Fatalf("an absent field must be skipped: %v", err)
	}
	if err := v.Validate(patch{Email: qt.Opt[string]{Set: true, V: ""}}); err != nil {
		t.Fatalf("an empty set value must be skipped by omitempty: %v", err)
	}
	if err := v.Validate(patch{Email: qt.Opt[string]{Set: true, V: "nope"}}); err == nil {
		t.Fatal("a set invalid value must fail")
	}
}

func TestOptOmitNilSkipsUnsetAndNullButRejectsEmptySet(t *testing.T) {
	type patch struct {
		Name qt.Opt[string]          `json:"name" validate:"omitnil,min=1"`
		Nick qt.Opt[qt.Null[string]] `json:"nick" validate:"provided,omitnil,min=2"`
	}
	v := qvalidator.New()

	if err := v.Validate(patch{
		Nick: qt.Of(qt.None[string]()),
	}); err != nil {
		t.Fatalf("unset Opt and explicit null with omitnil must pass: %v", err)
	}
	if err := v.Validate(patch{
		Name: qt.Of("Ada"),
		Nick: qt.Of(qt.Some("ad")),
	}); err != nil {
		t.Fatalf("valid set values must pass: %v", err)
	}
	err := v.Validate(patch{
		Name: qt.Of(""),
		Nick: qt.Of(qt.Some("")),
	})
	var fe qjson.Errors
	if !errors.As(err, &fe) || len(fe) != 2 {
		t.Fatalf("empty set strings must fail min under omitnil, got %v", err)
	}
}

func TestOptNullUsesPresenceAndNullRules(t *testing.T) {
	type patch struct {
		Nick qt.Opt[qt.Null[string]] `json:"nickname" validate:"provided,notnull"`
	}
	v := qvalidator.New()

	if err := v.Validate(patch{Nick: qt.Opt[qt.Null[string]]{Set: true, V: qt.Some("ada")}}); err != nil {
		t.Fatalf("a set value must pass: %v", err)
	}
	if err := v.Validate(patch{Nick: qt.Opt[qt.Null[string]]{Set: true, V: qt.None[string]()}}); err == nil {
		t.Fatal("an explicit null must fail notnull")
	}
	if err := v.Validate(patch{}); err == nil {
		t.Fatal("an absent field must fail provided")
	}
}

func TestOptUnwrapReachesNestedAndDivedFields(t *testing.T) {
	type inner struct {
		Email qt.Opt[string] `json:"email" validate:"provided,email"`
	}
	type outer struct {
		Inner inner            `json:"inner"`
		Tags  qt.Opt[[]string] `json:"tags" validate:"omitempty,dive,min=2"`
		Ptr   qt.Opt[*inner]   `json:"ptr" validate:"omitempty"`
	}
	v := qvalidator.New()

	err := v.Validate(outer{
		Inner: inner{Email: qt.Opt[string]{Set: true, V: "nope"}},
		Tags:  qt.Opt[[]string]{Set: true, V: []string{"ok", "x"}},
	})
	if err == nil {
		t.Fatal("the nested email and the short tag must fail")
	}
	var fe qjson.Errors
	if !errors.As(err, &fe) || len(fe) != 2 {
		t.Fatalf("want two field errors, got %v", err)
	}
}

func TestRegisterOptsOnHandBuiltValidator(t *testing.T) {
	type patch struct {
		Email qt.Opt[string] `json:"email" validate:"omitempty,email"`
	}
	v := validator.New()
	if err := qvalidator.Register(v); err != nil {
		t.Fatal(err)
	}
	if err := qvalidator.RegisterOpts(v, patch{}); err != nil {
		t.Fatal(err)
	}
	if err := v.Struct(patch{Email: qt.Opt[string]{Set: true, V: "nope"}}); err == nil {
		t.Fatal("RegisterOpts must make content rules reach .V")
	}
	if err := qvalidator.RegisterOpts(nil, patch{}); err == nil {
		t.Fatal("RegisterOpts(nil) must report the nil validator")
	}
}

func TestValidateIsSafeForConcurrentUse(t *testing.T) {
	type a struct {
		Email qt.Opt[string] `json:"email" validate:"omitempty,email"`
	}
	type b struct {
		Name qt.Opt[string] `json:"name" validate:"omitempty,min=3"`
	}
	v := qvalidator.New()
	good := a{Email: qt.Opt[string]{Set: true, V: "ada@example.com"}}
	bad := b{Name: qt.Opt[string]{Set: true, V: "x"}}
	errs := make(chan error, 16)
	for i := 0; i < 8; i++ {
		go func() { errs <- v.Validate(good) }()
		go func() { errs <- v.Validate(bad) }()
	}
	var passed, failed int
	for i := 0; i < 16; i++ {
		if err := <-errs; err == nil {
			passed++
		} else {
			failed++
		}
	}
	if passed != 8 || failed != 8 {
		t.Fatalf("got %d passes and %d failures, want 8 and 8", passed, failed)
	}
}

// go-playground/validator cannot apply content rules to a struct. Null[T] is
// unwrapped like Opt[T], so max, email and the others judge the value. Before
// that, max on a Null holding a value panicked and email rejected every address.
func TestNullContentRulesJudgeTheValue(t *testing.T) {
	type body struct {
		Nick qt.Null[string] `json:"nick" validate:"omitempty,max=5"`
		Mail qt.Null[string] `json:"mail" validate:"omitempty,email"`
		Age  qt.Null[int64]  `json:"age" validate:"omitempty,gte=18"`
	}
	v := qvalidator.New()
	ok := body{Nick: qt.Some("ada"), Mail: qt.Some("ada@example.com"), Age: qt.Some(int64(30))}
	if err := v.Validate(ok); err != nil {
		t.Fatalf("values that satisfy the rules must pass: %v", err)
	}
	if err := v.Validate(body{}); err != nil {
		t.Fatalf("NULL with omitempty must pass: %v", err)
	}
	bad := body{Nick: qt.Some("toolongvalue"), Mail: qt.Some("nope"), Age: qt.Some(int64(5))}
	var fields qjson.Errors
	if err := v.Validate(bad); !errors.As(err, &fields) || len(fields) != 3 {
		t.Fatalf("want three field errors, got %v", err)
	}
	for i, want := range []string{"nick", "mail", "age"} {
		if fields[i].Field != want {
			t.Errorf("error %d is about %q, want %q", i, fields[i].Field, want)
		}
	}
}

// Without omitempty a NULL fails the first rule, like a nil pointer; required
// on a Null therefore means "not NULL and not the zero value".
func TestNullWithoutOmitemptyFailsTheFirstRule(t *testing.T) {
	type body struct {
		Nick qt.Null[string] `json:"nick" validate:"required"`
	}
	v := qvalidator.New()
	if err := v.Validate(body{}); err == nil {
		t.Fatal("a NULL must fail required")
	}
	if err := v.Validate(body{Nick: qt.Some("ada")}); err != nil {
		t.Fatalf("a value must pass required: %v", err)
	}
}

func TestOptOfNullContentRules(t *testing.T) {
	type body struct {
		Nick qt.Opt[qt.Null[string]] `json:"nick" validate:"provided,omitempty,max=5"`
	}
	v := qvalidator.New()
	cases := []struct {
		name string
		in   qt.Opt[qt.Null[string]]
		ok   bool
	}{
		{"absent", qt.Opt[qt.Null[string]]{}, false},
		{"explicit null", qt.Of(qt.None[string]()), true},
		{"short value", qt.Of(qt.Some("ada")), true},
		{"long value", qt.Of(qt.Some("toolongvalue")), false},
	}
	for _, c := range cases {
		if err := v.Validate(body{Nick: c.in}); (err == nil) != c.ok {
			t.Errorf("%s: err = %v, want ok=%v", c.name, err, c.ok)
		}
	}
}

// A rule that cannot judge the value left by an explicit null (max after
// provided, with no omitempty between them) is a wrong tag. It is reported as
// an error naming the type, not as a panic in the handler.
func TestWrongTagIsAnErrorNotAPanic(t *testing.T) {
	type body struct {
		Nick qt.Opt[qt.Null[string]] `json:"nick" validate:"provided,max=5"`
	}
	v := qvalidator.New()
	err := v.Validate(body{Nick: qt.Of(qt.None[string]())})
	if err == nil {
		t.Fatal("expected an error")
	}
	var fields qjson.Errors
	if errors.As(err, &fields) {
		t.Fatalf("a wrong tag is not a field error: %v", err)
	}
}

func TestNotnullOnNullInsideSlices(t *testing.T) {
	type body struct {
		Items []qt.Null[string] `json:"items" validate:"dive,notnull,max=3"`
	}
	v := qvalidator.New()
	var fields qjson.Errors
	err := v.Validate(body{Items: []qt.Null[string]{qt.Some("a"), {}, qt.Some("toolong")}})
	if !errors.As(err, &fields) || len(fields) != 2 {
		t.Fatalf("want two errors (items[1] NULL, items[2] long), got %v", err)
	}
}

// provided falls back to the unwrapped value when the field is not an Opt: a
// plain Null carries no presence bit, so NULL is not provided, and neither is a
// nil pointer/slice/map. An Opt[Null] still counts an explicit null as present.
func TestProvidedRequiresAValue(t *testing.T) {
	type body struct {
		N qt.Null[string]         `json:"n" validate:"provided"`
		S []string                `json:"s" validate:"provided"`
		M map[string]string       `json:"m" validate:"provided"`
		O qt.Opt[qt.Null[string]] `json:"o" validate:"provided"`
	}
	v := qvalidator.New()
	var fields qjson.Errors
	if err := v.Validate(body{}); !errors.As(err, &fields) || len(fields) != 4 {
		t.Fatalf("want four field errors, got %v", err)
	}
	ok := body{
		N: qt.Some("ada"),
		S: []string{},
		M: map[string]string{},
		O: qt.Of(qt.None[string]()),
	}
	if err := v.Validate(ok); err != nil {
		t.Fatalf("present values must pass: %v", err)
	}
}

// nonzero on a Null judges the unwrapped value, not the wrapper struct: a
// non-NULL empty string is still zero.
func TestNonzeroOnNullChecksTheValue(t *testing.T) {
	type body struct {
		N qt.Null[string] `json:"n" validate:"nonzero"`
	}
	v := qvalidator.New()
	if err := v.Validate(body{N: qt.Some("")}); err == nil {
		t.Fatal("a non-NULL zero value must fail nonzero")
	}
	if err := v.Validate(body{N: qt.Some("ada")}); err != nil {
		t.Fatalf("a non-zero value must pass: %v", err)
	}
}
