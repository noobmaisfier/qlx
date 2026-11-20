package qjson

import (
	"bytes"
	"encoding"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

// Strict decoding of values wrapped by a type with its own UnmarshalJSON.
//
// encoding/json hands such a method the raw bytes, and the method decodes them
// with a decoder of its own: DisallowUnknownFields does not reach it. A body
// like {"settings":{"theme_typo":1}} would then pass for a qt.JSONB[Settings]
// field. After a successful decode, checkNested walks the JSON and the
// destination type together and refuses any key the destination does not
// declare, however deep, under the path of the offending key ("settings.theme_typo").
//
// A wrapper tells the walker what it holds through two optional methods, so
// this package needs no import of qt:
//
//	JSONInner() reflect.Type  // a transparent wrapper: Opt, Null, JSONB
//	JSONKeys() []string       // an object with a closed set of keys: Range
//
// Any other type with its own UnmarshalJSON or UnmarshalText is opaque and is
// not looked into, like json.RawMessage or an `any` field.

type shaper interface{ JSONInner() reflect.Type }
type keyed interface{ JSONKeys() []string }

var (
	shaperType = reflect.TypeOf((*shaper)(nil)).Elem()
	keyedType  = reflect.TypeOf((*keyed)(nil)).Elem()
	textType   = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
)

var needsWalkCache sync.Map // reflect.Type -> bool

// needsWalk reports whether the type tree under t holds a wrapper, so a plain
// DTO never pays for the second pass.
func needsWalk(t reflect.Type) bool {
	if v, ok := needsWalkCache.Load(t); ok {
		return v.(bool)
	}
	r := computeNeedsWalk(t, map[reflect.Type]bool{})
	needsWalkCache.Store(t, r)
	return r
}

func computeNeedsWalk(t reflect.Type, seen map[reflect.Type]bool) bool {
	if seen[t] {
		return false
	}
	seen[t] = true
	if isWrapper(t) {
		return true
	}
	if opaque(t) {
		return false
	}
	switch t.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
		return computeNeedsWalk(t.Elem(), seen)
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() && !f.Anonymous {
				continue
			}
			if JSONName(f) == "" && f.Tag.Get("json") == "-" {
				continue
			}
			if computeNeedsWalk(f.Type, seen) {
				return true
			}
		}
	}
	return false
}

func isWrapper(t reflect.Type) bool {
	return t.Implements(shaperType) || t.Implements(keyedType)
}

// opaque reports whether t decodes itself and is not a known wrapper.
func opaque(t reflect.Type) bool {
	if isWrapper(t) {
		return false
	}
	pt := reflect.PointerTo(t)
	return pt.Implements(unmarshalerType) || pt.Implements(textType) ||
		t.Implements(unmarshalerType) || t.Implements(textType)
}

// checkNested returns an unknown_field *Error for the first key, at any depth,
// that the destination type does not declare. dst is the pointer given to
// DecodeInto; data has already been decoded into it without error.
func checkNested(data []byte, dst any) error {
	rv := reflect.ValueOf(dst)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return nil
	}
	t := rv.Type().Elem()
	if !needsWalk(t) {
		return nil
	}
	if path, bad := walk(bytes.TrimSpace(data), t, ""); bad {
		return &Error{Code: CodeUnknownField, Field: path, Msg: "unknown field"}
	}
	return nil
}

// walk returns the path of the first unknown key under raw, if any.
func walk(raw []byte, t reflect.Type, path string) (string, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", false
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Implements(shaperType) {
		inner := reflect.Zero(t).Interface().(shaper).JSONInner()
		if inner == nil {
			return "", false
		}
		return walk(raw, inner, path)
	}
	if t.Implements(keyedType) {
		keys := reflect.Zero(t).Interface().(keyed).JSONKeys()
		ms, ok := members(raw)
		if !ok {
			return "", false
		}
		for _, m := range ms {
			if !hasKeyFold(keys, m.key) {
				return join(path, m.key), true
			}
		}
		return "", false
	}
	if opaque(t) {
		return "", false
	}
	switch t.Kind() {
	case reflect.Struct:
		fields := structFields(t)
		ms, ok := members(raw)
		if !ok {
			return "", false
		}
		for _, m := range ms {
			ft, found := lookupField(fields, m.key)
			if !found {
				return join(path, m.key), true
			}
			if p, bad := walk(m.val, ft, join(path, m.key)); bad {
				return p, true
			}
		}
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return "", false
		}
		ms, ok := members(raw)
		if !ok {
			return "", false
		}
		for _, m := range ms {
			if p, bad := walk(m.val, t.Elem(), join(path, m.key)); bad {
				return p, true
			}
		}
	case reflect.Slice, reflect.Array:
		var items []json.RawMessage
		if json.Unmarshal(raw, &items) != nil {
			return "", false
		}
		for i, it := range items {
			if p, bad := walk(it, t.Elem(), path+"["+strconv.Itoa(i)+"]"); bad {
				return p, true
			}
		}
	}
	return "", false
}

type member struct {
	key string
	val json.RawMessage
}

// members lists the keys of a JSON object in document order.
func members(raw []byte) ([]member, bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, false
	}
	var out []member
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, false
		}
		k, ok := tok.(string)
		if !ok {
			return nil, false
		}
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, false
		}
		out = append(out, member{k, v})
	}
	return out, true
}

type fieldInfo struct {
	name string
	typ  reflect.Type
}

var fieldsCache sync.Map // reflect.Type -> []fieldInfo

// structFields lists the JSON names a struct accepts, flattening embedded
// structs that have no json name, as encoding/json does.
func structFields(t reflect.Type) []fieldInfo {
	if v, ok := fieldsCache.Load(t); ok {
		return v.([]fieldInfo)
	}
	var out []fieldInfo
	var add func(t reflect.Type, seen map[reflect.Type]bool)
	add = func(t reflect.Type, seen map[reflect.Type]bool) {
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			tag := f.Tag.Get("json")
			if tag == "-" {
				continue
			}
			name, _, _ := strings.Cut(tag, ",")
			if f.Anonymous && name == "" {
				ft := f.Type
				if ft.Kind() == reflect.Pointer {
					ft = ft.Elem()
				}
				if ft.Kind() == reflect.Struct && !opaque(ft) && !isWrapper(ft) {
					if !seen[ft] {
						next := make(map[reflect.Type]bool, len(seen)+1)
						for k := range seen {
							next[k] = true
						}
						next[ft] = true
						add(ft, next)
					}
					continue
				}
			}
			if !f.IsExported() {
				continue
			}
			if name == "" {
				name = f.Name
			}
			out = append(out, fieldInfo{name, f.Type})
		}
	}
	add(t, map[reflect.Type]bool{t: true})
	fieldsCache.Store(t, out)
	return out
}

// lookupField matches a key like encoding/json: exact first, then
// case-insensitively.
func lookupField(fs []fieldInfo, key string) (reflect.Type, bool) {
	for _, f := range fs {
		if f.name == key {
			return f.typ, true
		}
	}
	for _, f := range fs {
		if strings.EqualFold(f.name, key) {
			return f.typ, true
		}
	}
	return nil, false
}

func hasKeyFold(keys []string, k string) bool {
	for _, x := range keys {
		if strings.EqualFold(x, k) {
			return true
		}
	}
	return false
}

func join(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}
