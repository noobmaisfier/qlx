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
