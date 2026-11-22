package qpage

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	q "github.com/noobmaisfier/qlx"
)

// Key is one column of a sort, with its direction.
type Key struct {
	col    string
	typ    reflect.Type
	def    q.KeyDef
	decode func(json.RawMessage) (q.Key, error)
}

// Asc and Desc declare a key column. The column must be NOT NULL; its Go type
// is the type of the matching field of the row.
func Asc[T any](col q.Operand[T]) Key { return newKey(q.KeyAsc(col)) }

// Desc declares a key ordered descending.
func Desc[T any](col q.Operand[T]) Key { return newKey(q.KeyDesc(col)) }

func newKey[T any](kc q.KeyCol[T]) Key {
	return Key{
		col: kc.Column(), typ: reflect.TypeFor[T](), def: kc,
		decode: func(raw json.RawMessage) (q.Key, error) {
			var v T
			if err := json.Unmarshal(raw, &v); err != nil {
				return q.Key{}, err
			}
			return kc.After(v), nil
		},
	}
}

// Option declares a sort, a tiebreak or the sizes.
type Option interface{ apply(*decl) }

type decl struct {
	sorts    []namedSort
	tiebreak []Key
	def, max int
}

type namedSort struct {
	name string
	keys []Key
}

type optFunc func(*decl)

func (f optFunc) apply(d *decl) { f(d) }

// Sort declares a sort a client may ask for by name. The first one declared is
// the default.
func Sort(name string, keys ...Key) Option {
	return optFunc(func(d *decl) { d.sorts = append(d.sorts, namedSort{name, keys}) })
}

// Tiebreak adds keys at the end of every sort (a sort that already has the
// column keeps its own), which makes the order total. Use the primary key.
func Tiebreak(keys ...Key) Option {
	return optFunc(func(d *decl) { d.tiebreak = append(d.tiebreak, keys...) })
}

// Sizes sets the default and the maximum page size (20 and 100 otherwise).
func Sizes(def, max int) Option {
	return optFunc(func(d *decl) { d.def, d.max = def, max })
}

type sortDef struct {
	name   string
	keys   []Key
	fields [][]int        // by name: index path of each key's field in the row
	cols   []reflect.Type // by position: the type of each scanned column of the row; nil when by name
}

// targeter is what a row scanned by position implements: generated rows, qlx.Tuple1..8
// and the structs marked //qlx:row.
type targeter interface{ ScanTargets() []any }

// SortSet is a set of sorts over one row type. It is safe for concurrent use.
type SortSet[R any] struct {
	sorts    []*sortDef
	byName   map[string]*sortDef
	def, max int
}

// Sorts declares the sorts over the row type R. A declaration that cannot work
// panics, as regexp.MustCompile does: no sort, a duplicate name, a key that is
// an expression, a key with no field in R or of another type, a nullable key.
func Sorts[R any](opts ...Option) *SortSet[R] {
	s, err := newSorts[R](opts)
	if err != nil {
		panic(err)
	}
	return s
}

func newSorts[R any](opts []Option) (*SortSet[R], error) {
	var d decl
	for _, o := range opts {
		o.apply(&d)
	}
	if len(d.sorts) == 0 {
		return nil, errors.New("qpage: declare at least one Sort")
	}
	rt := reflect.TypeFor[R]()
	if rt.Kind() != reflect.Struct {
		return nil, fmt.Errorf("qpage: the row type %s is not a struct", rt)
	}
	// A row that implements ScanTargets is scanned by position, so its keys are
	// found by position; any other struct is scanned by name (db tag or field
	// name). qpage follows the rule pgxexec follows for the same type.
	var cols []reflect.Type
	if tg, ok := any(new(R)).(targeter); ok {
		for i, ptr := range tg.ScanTargets() {
			pt := reflect.TypeOf(ptr)
			if pt == nil || pt.Kind() != reflect.Pointer {
				return nil, fmt.Errorf("qpage: %s.ScanTargets()[%d] is not a pointer", rt, i)
			}
			cols = append(cols, pt.Elem())
		}
	}
	fields := rowFields(rt)
	if d.def <= 0 {
		d.def = DefaultSize
	}
	if d.max <= 0 {
		d.max = DefaultMax
	}
	if d.def > d.max {
		d.def = d.max
	}
	s := &SortSet[R]{byName: map[string]*sortDef{}, def: d.def, max: d.max}
	for _, ns := range d.sorts {
		if ns.name == "" || strings.ContainsAny(ns.name, ", ") {
			return nil, fmt.Errorf("qpage: sort name %q is empty or contains a comma or a space", ns.name)
		}
		if s.byName[ns.name] != nil {
			return nil, fmt.Errorf("qpage: sort %q declared twice", ns.name)
		}
		if len(ns.keys) == 0 {
			return nil, fmt.Errorf("qpage: sort %q has no key", ns.name)
		}
		sd := &sortDef{name: ns.name, cols: cols}
		seen := map[string]bool{}
		for _, k := range append(append([]Key(nil), ns.keys...), d.tiebreak...) {
			if k.col == "" {
				return nil, fmt.Errorf("qpage: sort %q: a key must be a plain column, not an expression", ns.name)
			}
			if seen[k.col] {
				if containsKey(ns.keys, k.col) {
					return nil, fmt.Errorf("qpage: sort %q: column %s is a key twice", ns.name, k.col)
				}
				continue // the tiebreak column is already a key of this sort
			}
			seen[k.col] = true
			if isNullType(k.typ) {
				return nil, fmt.Errorf("qpage: sort %q: %s is nullable (%s); a key must be NOT NULL", ns.name, k.col, k.typ)
			}
			if cols != nil {
				sd.keys = append(sd.keys, k) // the position is known once the SELECT is
				continue
			}
			idx, ok := fields[strings.ToLower(k.col)]
			if !ok {
				return nil, fmt.Errorf("qpage: sort %q: %s has no field for column %s (tag a field with `db:\"%s\"`)", ns.name, rt, k.col, k.col)
			}
			if ft := rt.FieldByIndex(idx).Type; ft != k.typ {
				return nil, fmt.Errorf("qpage: sort %q: column %s is %s but the field of %s is %s", ns.name, k.col, k.typ, rt, ft)
			}
			sd.keys = append(sd.keys, k)
			sd.fields = append(sd.fields, idx)
		}
		s.sorts = append(s.sorts, sd)
		s.byName[sd.name] = sd
	}
	return s, nil
}

func containsKey(keys []Key, col string) bool {
	n := 0
	for _, k := range keys {
		if k.col == col {
			n++
		}
	}
	return n > 1
}

func isNullType(t reflect.Type) bool {
	return t.PkgPath() == "github.com/noobmaisfier/qlx/qt" && strings.HasPrefix(t.Name(), "Null[")
}

// rowFields maps a lower-cased column name to the index path of its field, the
// way pgxexec scans a row: the `db` tag or the field name; an embedded struct
// without a tag is flattened; unexported fields and "-" are skipped.
func rowFields(rt reflect.Type) map[string][]int {
	out := map[string][]int{}
	var walk func(t reflect.Type, path []int)
	walk = func(t reflect.Type, path []int) {
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			at := append(append([]int(nil), path...), i)
			if f.Anonymous && f.Type.Kind() == reflect.Struct && f.Tag.Get("db") == "" {
				walk(f.Type, at)
				continue
			}
			if !f.IsExported() {
				continue
			}
			name := f.Name
			if tag, ok := f.Tag.Lookup("db"); ok {
				if tag == "-" {
					continue
				}
				name = tag
			}
			out[strings.ToLower(name)] = at
		}
	}
	walk(rt, nil)
	return out
}

// Names lists the sorts a client may ask for, the default first.
func (s *SortSet[R]) Names() []string {
	out := make([]string, len(s.sorts))
	for i, sd := range s.sorts {
		out[i] = sd.name
	}
	return out
}

// Window decodes the request into the pieces of the query. An unknown sort is
// an error wrapping ErrSort; a cursor that does not belong is ErrCursor; a
// missing SetSecret is ErrCursorKey, whatever the page.
func (s *SortSet[R]) Window(req Request) (Window, error) {
	if err := CheckSecret(); err != nil {
		return Window{}, err // on the first page too: a cursor is made only when a next page exists
	}
	sd := s.sorts[0]
	if req.Sort != "" {
		if sd = s.byName[req.Sort]; sd == nil {
			return Window{}, fmt.Errorf("%w: unknown sort %q (allowed: %s)", ErrSort, req.Sort, strings.Join(s.Names(), ", "))
		}
	}
	size := req.Size
	switch {
	case size <= 0:
		size = s.def
	case size > s.max:
		size = s.max
	}
	defs := make([]q.KeyDef, len(sd.keys))
	for i, k := range sd.keys {
		defs[i] = k.def
	}
	w := Window{Order: q.KeyOrder(defs...), Limit: int64(size) + 1, size: size, sort: sd}
	if req.Cursor == "" {
		return w, nil
	}
	raws, err := openCursor(req.Cursor, sd.name)
	if err != nil {
		return Window{}, err
	}
	if len(raws) != len(sd.keys) {
		return Window{}, ErrCursor
	}
	keys := make([]q.Key, len(raws))
	for i, raw := range raws {
		if keys[i], err = sd.keys[i].decode(raw); err != nil {
			return Window{}, ErrCursor
		}
	}
	w.After = q.After(keys...)
	return w, nil
}

// Page cuts the rows the query returned (at most Window.Limit of them) into
// the items of the page and the next cursor.
func (s *SortSet[R]) Page(w Window, rows []R) (Page[R], error) {
	if w.sort == nil || s.byName[w.sort.name] != w.sort {
		return Page[R]{}, errors.New("qpage: Window not made by this SortSet")
	}
	if len(rows) <= w.size {
		if rows == nil {
			rows = []R{}
		}
		return Page[R]{Items: rows}, nil
	}
	items := rows[:w.size:w.size]
	vals := make([]json.RawMessage, len(w.sort.keys))
	var last reflect.Value
	var targets []any
	if w.sort.cols != nil {
		if w.pos == nil {
			return Page[R]{}, errors.New("qpage: Window.Apply was not called on this Window")
		}
		targets = any(&items[len(items)-1]).(targeter).ScanTargets()
		if len(targets) != len(w.sort.cols) {
			return Page[R]{}, fmt.Errorf("qpage: ScanTargets returned %d pointers, then %d", len(w.sort.cols), len(targets))
		}
	} else {
		last = reflect.ValueOf(&items[len(items)-1]).Elem()
	}
	for i := range w.sort.keys {
		var v any
		if targets != nil {
			v = reflect.ValueOf(targets[w.pos[i]]).Elem().Interface()
		} else {
			v = last.FieldByIndex(w.sort.fields[i]).Interface()
		}
		b, err := json.Marshal(v)
		if err != nil {
			return Page[R]{}, fmt.Errorf("qpage: key %s is not encodable: %w", w.sort.keys[i].col, err)
		}
		vals[i] = b
	}
	next, err := sealCursor(w.sort.name, vals)
	if err != nil {
		return Page[R]{}, err
	}
	return Page[R]{Items: items, Next: next}, nil
}

// The cursor is base64url(version | JSON{n: sort name, k: key values} | MAC):
// signed, so a client cannot forge key values, and bound to its sort, so it
// cannot be replayed against another.
const version = 1

type cursorBody struct {
	N string            `json:"n"`
	K []json.RawMessage `json:"k"`
}

func sealCursor(sortName string, vals []json.RawMessage) (string, error) {
	body, err := json.Marshal(cursorBody{N: sortName, K: vals})
	if err != nil {
		return "", err
	}
	out := append([]byte{version}, body...)
	tag, err := sign(out)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(append(out, tag...)), nil
}

func openCursor(s, sortName string) ([]json.RawMessage, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil || len(raw) < 1+macLen || raw[0] != version {
		return nil, ErrCursor
	}
	signed, tag := raw[:len(raw)-macLen], raw[len(raw)-macLen:]
	ok, err := verify(signed, tag)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrCursor
	}
	var b cursorBody
	if json.Unmarshal(signed[1:], &b) != nil || b.N != sortName {
		return nil, ErrCursor
	}
	return b.K, nil
}
