package qjson

import (
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/noobmaisfier/qlx/internal/textval"
)

// Error codes reported by DecodeQuery and DecodeQueryInto. Like the body
// codes they are stable and meant to be matched. Field is the name of the
// query parameter.
const (
	// CodeInvalidParam is a parameter whose value cannot be read into its
	// field: not an integer, not a boolean, an id that is not this table's,
	// or a single-valued parameter given twice.
	CodeInvalidParam = "invalid_param"
	// CodeUnknownParam is a parameter that no field declares. AllowUnknownFields
	// accepts it.
	CodeUnknownParam = "unknown_param"
	// CodeInvalidQuery is a query string that is not valid URL encoding.
	CodeInvalidQuery = "invalid_query"
)

// KeepEmptyParams makes DecodeQuery read an empty value ("?q=") as a value:
// "" for a string, a parse error for a number. The default treats an empty
// value as an absent parameter, which is what a search box that submits all
// its fields wants.
func KeepEmptyParams() Option {
	return func(c *config) { c.keepEmpty = true }
}

// DecodeQuery reads the URL query of r into a new T, which must be a struct.
// It is the query-string counterpart of DecodeRequest and follows the same
// rules: unknown parameters are refused (AllowUnknownFields lifts that), every
// failure is a *Error with a stable Code and the parameter name in Field, and
// nothing else happens: no normalization and no validation, so call
// Validate (or the validator adapter) afterwards, as for a body.
//
//	type ListUsers struct {
//		Tier   *string          `query:"tier"`
//		Search qt.Opt[string]   `query:"q"`
//		IDs    []qt.ID[UserSpace] `query:"id"`
//		Limit  int              `query:"limit"`
//	}
//	f, err := qjson.DecodeQuery[ListUsers](r)
//
// The parameter of a field is, in order: the name of its query tag, of its
// form tag, of its json tag, then the lower-cased field name. A tag without a
// name (`json:",omitempty"`) is skipped, and `query:"-"` (or form, or json
// when no earlier tag names the field) leaves the field out. Embedded structs
// are flattened; unexported fields are ignored.
//
// A field can be a string, a bool, an integer, a float, a time.Duration ("90s"),
// any type whose pointer implements encoding.TextUnmarshaler (qt.ID, qt.Date,
// time.Time, ...), a pointer to one of those, a qt.Opt of one of those, or a
// slice of one of those, filled from a repeated parameter (?id=1&id=2). A
// parameter that is not a slice and is given twice is CodeInvalidParam.
// An absent parameter leaves the field at its zero value, so a pointer stays
// nil and a qt.Opt stays unset; an empty value counts as absent unless
// KeepEmptyParams is given. Any other field type makes DecodeQuery return a
// plain error (not a *Error): it is a bug in the struct, not in the request.
func DecodeQuery[T any](r *http.Request, opts ...Option) (T, error) {
	var v T
	if r == nil || r.URL == nil {
		return v, &Error{Code: CodeInvalidQuery, Msg: "the request has no URL"}
	}
	vals, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return v, &Error{Code: CodeInvalidQuery, Msg: "the query string is not valid", Err: err}
	}
	err = DecodeQueryInto(&v, vals, opts...)
	return v, err
}

// DecodeQueryInto is DecodeQuery for values you already hold (a form, a
// url.Values built by a test). dst must be a non-nil pointer to a struct.
func DecodeQueryInto(dst any, vals url.Values, opts ...Option) error {
	cfg := defaultConfig()
	for _, o := range opts {
		if o != nil {
			o(&cfg)
		}
	}
	rv := reflect.ValueOf(dst)
	if rv.Kind() != reflect.Pointer || rv.IsNil() || rv.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("qjson: DecodeQuery needs a pointer to a struct, got %T", dst)
	}
	root := rv.Elem()
	plan, err := queryPlanOf(root.Type())
	if err != nil {
		return err
	}

	if cfg.strict {
		var unknown []string
		for k := range vals {
			if _, ok := plan.byName[k]; !ok {
				unknown = append(unknown, k)
			}
		}
		if len(unknown) > 0 {
			sort.Strings(unknown)
			return &Error{Code: CodeUnknownParam, Field: unknown[0], Msg: "unknown parameter"}
		}
	}

	for i := range plan.fields {
		f := &plan.fields[i]
		raw, ok := vals[f.name]
		if !ok {
			continue
		}
		if !cfg.keepEmpty {
			raw = nonEmpty(raw)
		}
		if len(raw) == 0 {
			continue
		}
		fv := root.FieldByIndex(f.index)
		if f.slice {
			out := reflect.MakeSlice(f.typ, 0, len(raw))
			for _, s := range raw {
				el := reflect.New(f.typ.Elem()).Elem()
				if err := textval.Set(el, s); err != nil {
					return paramError(f.name, err)
				}
				out = reflect.Append(out, el)
			}
			fv.Set(out)
			continue
		}
		if len(raw) > 1 {
			return &Error{Code: CodeInvalidParam, Field: f.name, Msg: "given more than once"}
		}
		if err := textval.Set(fv, raw[0]); err != nil {
			return paramError(f.name, err)
		}
	}
	return nil
}

// QueryName returns the query parameter a struct field is read from, or "" for
// a field DecodeQuery leaves out. Pass a function built on it to a validator's
// RegisterTagNameFunc so that errors carry the names clients see, as
// ValidatorName does for JSON bodies.
func QueryName(f reflect.StructField) string {
	name, _ := queryName(f)
	return name
}

func paramError(name string, err error) error {
	msg := err.Error()
	return &Error{Code: CodeInvalidParam, Field: name, Msg: msg, Err: err}
}

func nonEmpty(in []string) []string {
	for i, s := range in {
		if s != "" {
			continue
		}
		out := append([]string(nil), in[:i]...)
		for _, t := range in[i+1:] {
			if t != "" {
				out = append(out, t)
			}
		}
		return out
	}
	return in
}

// queryName returns the parameter name of f and whether the field is read at
// all. Unexported fields are not.
func queryName(f reflect.StructField) (string, bool) {
	if !f.IsExported() {
		return "", false
	}
	for _, tag := range [...]string{"query", "form", "json"} {
		v, ok := f.Tag.Lookup(tag)
		if !ok || v == "" {
			continue
		}
		name, _, _ := strings.Cut(v, ",")
		switch name {
		case "":
			continue
		case "-":
			return "", false
		}
		return name, true
	}
	return strings.ToLower(f.Name), true
}

type queryField struct {
	name  string
	index []int
	typ   reflect.Type
	slice bool
}

type queryPlan struct {
	fields []queryField
	byName map[string]int
}

var planCache sync.Map // reflect.Type -> *planEntry

type planEntry struct {
	plan *queryPlan
	err  error
}

func queryPlanOf(t reflect.Type) (*queryPlan, error) {
	if e, ok := planCache.Load(t); ok {
		pe := e.(*planEntry)
		return pe.plan, pe.err
	}
	p := &queryPlan{byName: map[string]int{}}
	err := p.collect(t, nil)
	pe := &planEntry{plan: p, err: err}
	if err != nil {
		pe.plan = nil
	}
	planCache.Store(t, pe)
	return pe.plan, pe.err
}

func (p *queryPlan) collect(t reflect.Type, prefix []int) error {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		idx := append(append([]int(nil), prefix...), i)
		if f.Anonymous && f.Type.Kind() == reflect.Struct &&
			!reflect.PointerTo(f.Type).Implements(textType) && f.Tag == "" {
			if err := p.collect(f.Type, idx); err != nil {
				return err
			}
			continue
		}
		name, ok := queryName(f)
		if !ok {
			continue
		}
		qf := queryField{name: name, index: idx, typ: f.Type}
		elem := f.Type
		if f.Type.Kind() == reflect.Slice {
			qf.slice = true
			elem = f.Type.Elem()
			if elem.Kind() == reflect.Uint8 {
				return fmt.Errorf("qjson: DecodeQuery: field %s.%s is a byte slice, which a query parameter cannot fill", t, f.Name)
			}
		}
		if !textval.Supported(elem) {
			return fmt.Errorf("qjson: DecodeQuery: field %s.%s has type %s, which a query parameter cannot fill", t, f.Name, f.Type)
		}
		if _, dup := p.byName[name]; dup {
			return fmt.Errorf("qjson: DecodeQuery: parameter %q is read by two fields of %s", name, t)
		}
		p.byName[name] = len(p.fields)
		p.fields = append(p.fields, qf)
	}
	return nil
}
