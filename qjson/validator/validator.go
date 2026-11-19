// Package qvalidator adapts go-playground/validator to QLX request DTOs.
//
// It registers the rules the qt types need and reports failures under the JSON
// names of the fields (the json tag), so the response can be sent to a client
// without translating Go names.
//
//	v := qvalidator.New()
//	if err := v.Validate(body); err != nil {
//		var fields qjson.Errors
//		if errors.As(err, &fields) {
//			// []qjson.FieldError{{Field:"email", Code:"required", ...}}
//		}
//	}
//
// The rules:
//
//	provided  the field is an Opt[T] that the body set (including an explicit null)
//	notnull   the field is a Null[T] that is not NULL, or a non-nil pointer/slice/map
//	nonzero   the field is not its zero value (qt.ID, qt.Date, Opt[T], ...)
//
// Use them instead of `required`: required tests the struct, not its content, so
// a zero qt.Null, qt.Opt or qt.ID always passes it.
//
// An Opt[T] is unwrapped before the other rules run: an unset Opt becomes null,
// so omitempty skips it, and a set Opt becomes its .V, so content rules finally
// apply to the value. `validate:"provided,email"` therefore checks both that the
// key was present and that the value is an email. Without provided or omitempty,
// an unset Opt fails the first rule, like a nil pointer.
package qvalidator

import (
	"errors"
	"reflect"
	"strings"
	"sync"

	"github.com/go-playground/validator/v10"
	"github.com/noobmaisfier/qlx/qjson"
)

// Rule names registered by Register and New.
const (
	TagProvided = "provided"
	TagNotNull  = "notnull"
	TagNonzero  = "nonzero"
)

// Validator wraps a *validator.Validate configured for QLX DTOs.
type Validator struct {
	v   *validator.Validate
	msg func(validator.FieldError) string

	mu         sync.RWMutex
	prepared   map[reflect.Type]bool
	registered map[reflect.Type]bool
}

// New returns a Validator with the JSON tag names and the QLX rules registered.
// The first time a DTO type is validated, New also teaches the validator how to
// unwrap every Opt[T] it contains; no per-instantiation registration is needed.
func New() *Validator {
	v := validator.New()
	if err := Register(v); err != nil {
		// Register only fails on a duplicate tag, which a fresh instance has not.
		panic("qvalidator: " + err.Error())
	}
	return &Validator{
		v:          v,
		msg:        DefaultMessage,
		prepared:   make(map[reflect.Type]bool),
		registered: make(map[reflect.Type]bool),
	}
}

// Register adds the JSON tag name function and the QLX rules to an existing
// validator. Calling it twice replaces the same rules; the only error is a nil
// validator. Register does not unwrap Opt[T]; follow it with RegisterOpts, or
// use New, which does both.
func Register(v *validator.Validate) error {
	if v == nil {
		return errors.New("qvalidator: nil validator")
	}
	v.RegisterTagNameFunc(qjson.ValidatorName)
	rules := map[string]validator.Func{
		TagProvided: provided,
		TagNotNull:  notnull,
		TagNonzero:  nonzero,
	}
	for name, fn := range rules {
		if err := v.RegisterValidation(name, fn); err != nil {
			return err
		}
	}
	return nil
}

// Validate runs the rules and returns qjson.Errors (one entry per rejected
// field, named by its JSON key) or nil.
//
// Validate is safe for concurrent use. The first call for a DTO type registers
// the unwrappers its Opt fields need, then the type is marked prepared.
func (v *Validator) Validate(s any) error {
	if v == nil || v.v == nil {
		return errors.New("qvalidator: nil Validator")
	}
	t := reflect.TypeOf(s)

	v.mu.RLock()
	ready := v.prepared[t]
	v.mu.RUnlock()
	if !ready {
		v.mu.Lock()
		if !v.prepared[t] {
			v.registerOptsLocked(t)
			v.prepared[t] = true
		}
		v.mu.Unlock()
	}
	err := v.structErr(s)

	if err == nil {
		return nil
	}
	var ve validator.ValidationErrors
	if !errors.As(err, &ve) {
		return err
	}
	out := make(qjson.Errors, 0, len(ve))
	for _, fe := range ve {
		msg := ""
		if v.msg != nil {
			msg = v.msg(fe)
		}
		out = append(out, qjson.FieldError{
			Field: fe.Field(),
			Code:  fe.Tag(),
			Msg:   msg,
		})
	}
	return out
}

// structErr runs the validator once the DTO type is prepared. The read lock
// keeps concurrent first-time registrations from writing the validator's
// custom type map while Struct reads it.
func (v *Validator) structErr(s any) error {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.v.Struct(s)
}

// RegisterOpts teaches a validator built outside New how to unwrap every
// Opt[T] instantiation reachable from sample's type. It is idempotent; call it
// once per DTO shape before v.Struct when you use Register on your own
// validator. New does this automatically, so it is only needed with Raw or a
// hand-built validator.
func RegisterOpts(v *validator.Validate, sample any) error {
	if v == nil {
		return errors.New("qvalidator: nil validator")
	}
	for _, t := range optTypes(reflect.TypeOf(sample)) {
		v.RegisterCustomTypeFunc(unwrapOpt, reflect.Zero(t).Interface())
	}
	return nil
}

// SetMessageFunc replaces the message of every FieldError. It returns v so it
// can be chained.
func (v *Validator) SetMessageFunc(fn func(validator.FieldError) string) *Validator {
	v.msg = fn
	return v
}

// Raw returns the underlying validator, for rules this package does not wrap.
func (v *Validator) Raw() *validator.Validate { return v.v }

// DefaultMessage is the message of a FieldError when SetMessageFunc was not
// called. It names the rule and its parameter rather than repeating the field.
func DefaultMessage(fe validator.FieldError) string {
	switch fe.Tag() {
	case TagProvided:
		return "must be provided"
	case TagNotNull:
		return "must not be null"
	case TagNonzero:
		return "must be set"
	case "required":
		return "is required"
	case "email":
		return "must be a valid email address"
	case "url", "uri":
		return "must be a valid URL"
	case "uuid", "uuid3", "uuid4", "uuid5", "uuid6", "uuid7":
		return "must be a valid UUID"
	case "min":
		return "must be at least " + fe.Param()
	case "max":
		return "must be at most " + fe.Param()
	case "len":
		return "must have length " + fe.Param()
	case "gte":
		return "must be greater than or equal to " + fe.Param()
	case "lte":
		return "must be less than or equal to " + fe.Param()
	case "gt":
		return "must be greater than " + fe.Param()
	case "lt":
		return "must be less than " + fe.Param()
	case "oneof":
		return "must be one of: " + strings.ReplaceAll(fe.Param(), " ", ", ")
	case "numeric", "number":
		return "must be a number"
	case "alphanum":
		return "must contain only letters and digits"
	case "boolean":
		return "must be a boolean"
	case "e164":
		return "must be a valid E.164 phone number"
	}
	if p := fe.Param(); p != "" {
		return "failed the " + fe.Tag() + " rule (" + p + ")"
	}
	return "failed the " + fe.Tag() + " rule"
}

// registerOptsLocked registers the unwrappers for t. The caller holds mu for
// writing.
func (v *Validator) registerOptsLocked(t reflect.Type) {
	for _, ot := range optTypes(t) {
		if v.registered[ot] {
			continue
		}
		v.v.RegisterCustomTypeFunc(unwrapOpt, reflect.Zero(ot).Interface())
		v.registered[ot] = true
	}
}

// isSetRule is the method Opt[T] implements; it is what lets the engine
// recognise an Opt without the package importing qt.
var isSetRule = reflect.TypeOf((*interface{ IsSet() bool })(nil)).Elem()

// isOptType reports whether t is an Opt[T]: a struct with an exported V, a
// bool Set and the IsSet method.
func isOptType(t reflect.Type) bool {
	if t == nil || t.Kind() != reflect.Struct || !t.Implements(isSetRule) {
		return false
	}
	vf, ok := t.FieldByName("V")
	if !ok || !vf.IsExported() {
		return false
	}
	sf, ok := t.FieldByName("Set")
	return ok && sf.Type.Kind() == reflect.Bool
}

// optTypes returns every Opt instantiation reachable from t, without walking
// the same type twice.
func optTypes(t reflect.Type) []reflect.Type {
	var out []reflect.Type
	var walk func(reflect.Type)
	seen := make(map[reflect.Type]bool)
	walk = func(t reflect.Type) {
		if t == nil || seen[t] {
			return
		}
		seen[t] = true
		switch t.Kind() {
		case reflect.Ptr, reflect.Slice, reflect.Array:
			walk(t.Elem())
		case reflect.Map:
			walk(t.Key())
			walk(t.Elem())
		case reflect.Struct:
			if isOptType(t) {
				out = append(out, t)
			}
			for i := 0; i < t.NumField(); i++ {
				f := t.Field(i)
				if f.IsExported() {
					walk(f.Type)
				}
			}
		}
	}
	walk(t)
	return out
}

// unwrapOpt is the CustomTypeFunc validator calls for an Opt[T]. An unset Opt
// becomes nil (so omitempty skips it and any other rule fails); a set Opt
// becomes its .V, so email, min and friends run on the value the client sent.
func unwrapOpt(field reflect.Value) any {
	for field.Kind() == reflect.Ptr || field.Kind() == reflect.Interface {
		if field.IsNil() {
			return nil
		}
		field = field.Elem()
	}
	if field.Kind() != reflect.Struct || !field.CanInterface() {
		return nil
	}
	s, ok := field.Interface().(interface{ IsSet() bool })
	if !ok || !s.IsSet() {
		return nil
	}
	vf := field.FieldByName("V")
	if !vf.IsValid() || !vf.CanInterface() {
		return nil
	}
	return vf.Interface()
}

func provided(fl validator.FieldLevel) bool {
	// The engine calls this rule on the unwrapped value, so look the original
	// struct field back up to read Set.
	if f, ok := structField(fl); ok && f.CanInterface() {
		if s, ok := f.Interface().(interface{ IsSet() bool }); ok {
			return s.IsSet()
		}
	}
	return true
}

// structField returns the field as declared in its parent struct, before the
// custom type func unwrapped it. It returns false when the field has no parent
// struct, as with Var.
func structField(fl validator.FieldLevel) (reflect.Value, bool) {
	name := fl.StructFieldName()
	parent := fl.Parent()
	if name == "" || !parent.IsValid() {
		return reflect.Value{}, false
	}
	for parent.Kind() == reflect.Ptr || parent.Kind() == reflect.Interface {
		if parent.IsNil() {
			return reflect.Value{}, false
		}
		parent = parent.Elem()
	}
	if parent.Kind() != reflect.Struct {
		return reflect.Value{}, false
	}
	f := parent.FieldByName(name)
	if !f.IsValid() {
		return reflect.Value{}, false
	}
	return f, true
}

func notnull(fl validator.FieldLevel) bool {
	f := fl.Field()
	switch f.Kind() {
	case reflect.Ptr, reflect.Interface, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan:
		return !f.IsNil()
	}
	if f.CanInterface() {
		if s, ok := f.Interface().(interface{ IsNull() bool }); ok {
			return !s.IsNull()
		}
	}
	return true
}

func nonzero(fl validator.FieldLevel) bool {
	f := fl.Field()
	if !f.CanInterface() {
		return true
	}
	if s, ok := f.Interface().(interface{ IsZero() bool }); ok {
		return !s.IsZero()
	}
	return !f.IsZero()
}
