// Package qjson decodes JSON request bodies strictly and reports errors under
// the names of the JSON contract (the json tag), not the Go field names.
//
// The decoder is the strict one an API wants and a handler keeps forgetting:
// unknown keys are refused by default, the body is bounded, exactly one JSON
// value is accepted, and a malformed document is a typed *Error with a stable
// Code and, when possible, the offending JSON field.
//
//	qtype, err := qjson.Decode[PatchUser](r)
//	if err != nil {
//		var de *qjson.Error
//		if errors.As(err, &de) && de.Code == qjson.CodeUnknownField {
//			// 400: the client sent a key this endpoint does not accept
//		}
//	}
//
// The package depends on the standard library only. Validation is a separate
// concern: see qjson/validator for the go-playground/validator adapter, or
// implement Validatable and call Validate.
package qjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
)

// Error codes reported by Decode and DecodeRequest. They are stable and meant
// to be matched, not printed.
const (
	// CodeInvalidJSON is malformed JSON, a JSON value of the wrong type, or a
	// value refused by a type's UnmarshalJSON (for example qt.ErrBadID).
	CodeInvalidJSON = "invalid_json"
	// CodeUnknownField is a key that no field of the destination accepts.
	CodeUnknownField = "unknown_field"
	// CodeEmptyBody is a request with no body, or a body that is only spaces.
	CodeEmptyBody = "empty_body"
	// CodeTooLarge is a body above the MaxBytes limit.
	CodeTooLarge = "too_large"
	// CodeTrailingData is a second JSON value after the first one.
	CodeTrailingData = "trailing_data"
	// CodeUnsupportedMediaType is a Content-Type that is not JSON.
	CodeUnsupportedMediaType = "unsupported_media_type"
	// CodeReadError is a failure while reading the body.
	CodeReadError = "read_error"
)

// DefaultMaxBytes is the default body limit of Decode and DecodeRequest: 1 MiB.
const DefaultMaxBytes int64 = 1 << 20

// Error is a decode failure. Field is the JSON name of the field involved, or
// "" when the failure is not about one field. Err is the underlying error, so
// errors.Is keeps working (for example with qt.ErrBadID).
type Error struct {
	Code  string `json:"code"`
	Field string `json:"field,omitempty"`
	Msg   string `json:"msg,omitempty"`
	Err   error  `json:"-"`
}

func (e *Error) Error() string {
	switch {
	case e.Field != "" && e.Msg != "":
		return "qjson: " + e.Code + " (" + e.Field + "): " + e.Msg
	case e.Msg != "":
		return "qjson: " + e.Code + ": " + e.Msg
	default:
		return "qjson: " + e.Code
	}
}

// Unwrap returns the underlying error, if any.
func (e *Error) Unwrap() error { return e.Err }

// FieldError is one field refused by a validation rule, named by its JSON key.
type FieldError struct {
	Field string `json:"field"`
	Code  string `json:"code"`
	Msg   string `json:"msg,omitempty"`
}

// Errors is a non-empty list of field errors. It is the error returned by the
// validator adapter in qjson/validator.
type Errors []FieldError

func (e Errors) Error() string {
	if len(e) == 0 {
		return "qjson: no field errors"
	}
	var b strings.Builder
	b.WriteString("qjson: invalid request")
	for _, fe := range e {
		b.WriteString("; ")
		b.WriteString(fe.Field)
		if fe.Msg != "" {
			b.WriteString(" ")
			b.WriteString(fe.Msg)
		} else if fe.Code != "" {
			b.WriteString(": ")
			b.WriteString(fe.Code)
		}
	}
	return b.String()
}

// Validatable is implemented by a request DTO that checks itself.
type Validatable interface {
	Validate() error
}

// Validate calls v.Validate when v implements Validatable and returns nil
// otherwise. It does not run go-playground/validator rules; use qjson/validator
// for those.
func Validate(v any) error {
	if x, ok := v.(Validatable); ok {
		return x.Validate()
	}
	return nil
}

// JSONName returns the name a struct field has in the JSON contract: the name
// in its json tag, or the Go name when the tag has no name. It returns "" for
// a field tagged json:"-", which is not part of the contract.
func JSONName(f reflect.StructField) string {
	name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
	if name == "-" {
		return ""
	}
	if name == "" {
		return f.Name
	}
	return name
}

// ValidatorName returns JSONName, falling back to the Go name for json:"-"
// fields. It is the function to pass to validator.Validate.RegisterTagNameFunc
// so that validation errors carry the names clients see.
func ValidatorName(f reflect.StructField) string {
	if name := JSONName(f); name != "" {
		return name
	}
	return f.Name
}

type config struct {
	maxBytes       int64
	strict         bool
	anyContentType bool
}

func defaultConfig() config {
	return config{maxBytes: DefaultMaxBytes, strict: true}
}

// Option configures Decode and DecodeRequest.
type Option func(*config)

// MaxBytes bounds the body. A value <= 0 disables the limit. The default is
// DefaultMaxBytes.
func MaxBytes(n int64) Option {
	return func(c *config) { c.maxBytes = n }
}

// AllowUnknownFields accepts keys that no field of the destination declares.
// The default refuses them: a typo or a field a client must not set is a 400,
// not a silently ignored key.
func AllowUnknownFields() Option {
	return func(c *config) { c.strict = false }
}

// AnyContentType makes DecodeRequest skip the Content-Type check. The default
// accepts application/json and any application/*+json media type.
func AnyContentType() Option {
	return func(c *config) { c.anyContentType = true }
}

// Decode reads exactly one JSON value from r into a new T and returns it.
func Decode[T any](r io.Reader, opts ...Option) (T, error) {
	var v T
	err := DecodeInto(&v, r, opts...)
	return v, err
}

// DecodeInto reads exactly one JSON value from r into dst, which must be a
// non-nil pointer.
func DecodeInto(dst any, r io.Reader, opts ...Option) error {
	cfg := defaultConfig()
	for _, o := range opts {
		if o != nil {
			o(&cfg)
		}
	}
	if r == nil {
		return &Error{Code: CodeEmptyBody, Msg: "no body"}
	}

	var (
		data []byte
		err  error
	)
	if cfg.maxBytes > 0 {
		data, err = io.ReadAll(io.LimitReader(r, cfg.maxBytes+1))
		if err == nil && int64(len(data)) > cfg.maxBytes {
			return &Error{Code: CodeTooLarge, Msg: fmt.Sprintf("body exceeds %d bytes", cfg.maxBytes)}
		}
	} else {
		data, err = io.ReadAll(r)
	}
	if err != nil {
		return &Error{Code: CodeReadError, Msg: "cannot read the body", Err: err}
	}

	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return &Error{Code: CodeEmptyBody, Msg: "the body is empty"}
	}
	if bytes.Equal(trimmed, []byte("null")) {
		return &Error{Code: CodeInvalidJSON, Msg: "the body is null, not an object"}
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	if cfg.strict {
		dec.DisallowUnknownFields()
	}
	if err := dec.Decode(dst); err != nil {
		return decodeError(err, data, dst)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return &Error{Code: CodeTrailingData, Msg: "the body must contain exactly one JSON value"}
	}
	if cfg.strict {
		// DisallowUnknownFields stops at a type with its own UnmarshalJSON
		// (qt.JSONB, qt.Opt, qt.Range): check the keys inside them too.
		return checkNested(data, dst)
	}
	return nil
}

func decodeError(err error, data []byte, dst any) error {
	var te *json.UnmarshalTypeError
	if errors.As(err, &te) {
		field := te.Field
		if field == "" {
			field = errorField(data, dst)
		}
		msg := "expected " + te.Type.String() + ", got JSON " + te.Value
		return &Error{Code: CodeInvalidJSON, Field: field, Msg: msg, Err: err}
	}
	var se *json.SyntaxError
	if errors.As(err, &se) {
		return &Error{
			Code: CodeInvalidJSON,
			Msg:  fmt.Sprintf("malformed JSON at byte %d: %v", se.Offset, se),
			Err:  err,
		}
	}
	if field, ok := unknownField(err); ok {
		return &Error{Code: CodeUnknownField, Field: field, Msg: "unknown field", Err: err}
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return &Error{Code: CodeInvalidJSON, Msg: "the body ends in the middle of a JSON value", Err: err}
	}
	var ie *json.InvalidUnmarshalError
	if errors.As(err, &ie) {
		return &Error{Code: CodeInvalidJSON, Msg: "destination is not a non-nil pointer", Err: err}
	}
	return &Error{Code: CodeInvalidJSON, Field: errorField(data, dst), Msg: err.Error(), Err: err}
}

// errorField finds the top-level JSON key whose value the decoder refused. It
// is the fallback for errors raised inside a field's UnmarshalJSON, where
// encoding/json cannot attach the enclosing key: each key is decoded on its own
// into a fresh destination, and the first one that fails names the field. It
// runs only after a decode has already failed, so the extra work is off the
// success path.
func errorField(data []byte, dst any) string {
	rv := reflect.ValueOf(dst)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return ""
	}
	t := rv.Type().Elem()
	if t.Kind() != reflect.Struct && t.Kind() != reflect.Map {
		return ""
	}
	if t.Kind() == reflect.Struct && reflect.PointerTo(t).Implements(unmarshalerType) {
		// A custom UnmarshalJSON on the destination sees a different object
		// here, so a failure would not identify a field.
		return ""
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('{') {
		return ""
	}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return ""
		}
		key, ok := tok.(string)
		if !ok {
			return ""
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return ""
		}
		name, err := json.Marshal(key)
		if err != nil {
			return ""
		}
		one := append([]byte{'{'}, name...)
		one = append(one, ':')
		one = append(one, raw...)
		one = append(one, '}')
		if err := json.Unmarshal(one, reflect.New(t).Interface()); err != nil {
			return key
		}
	}
	return ""
}

var unmarshalerType = reflect.TypeOf((*json.Unmarshaler)(nil)).Elem()

const unknownFieldPrefix = "json: unknown field "

func unknownField(err error) (string, bool) {
	s := err.Error()
	if !strings.HasPrefix(s, unknownFieldPrefix) {
		return "", false
	}
	rest := strings.TrimPrefix(s, unknownFieldPrefix)
	name, uerr := strconv.Unquote(rest)
	if uerr != nil {
		return strings.Trim(rest, `"`), true
	}
	return name, true
}
