package qt

import (
	"database/sql/driver"
	"fmt"
	"strconv"
	"strings"
)

// Types of the extension packs. They travel in PostgreSQL's text form, which
// every extension type has, so no binary codec is needed and nothing depends
// on the extension's type OID (it differs per database).

// Vector is a pgvector vector.
type Vector []float32

// Value implements driver.Valuer: the text form "[1,2,3]".
func (v Vector) Value() (driver.Value, error) {
	if v == nil {
		return nil, nil
	}
	return formatFloats(v), nil
}

// Scan implements sql.Scanner.
func (v *Vector) Scan(src any) error {
	s, ok, err := textOf(src)
	if err != nil || !ok {
		*v = nil
		return err
	}
	f, err := parseFloats(s)
	*v = f
	return err
}

// HalfVector is a pgvector halfvec; values are held as float32.
type HalfVector []float32

// Value implements driver.Valuer.
func (v HalfVector) Value() (driver.Value, error) { return Vector(v).Value() }

// Scan implements sql.Scanner.
func (v *HalfVector) Scan(src any) error { return (*Vector)(v).Scan(src) }

// SparseVector is a pgvector sparsevec in text form, "{1:0.5,3:2}/5".
type SparseVector string

// Geometry is a PostGIS geometry as extended well-known binary in hex, the
// text form PostgreSQL uses for it.
type Geometry string

// Geography is a PostGIS geography as extended well-known binary in hex.
type Geography string

// LTree is an ltree label path, "Top.Science.Astronomy".
type LTree string

// LQuery is an ltree lquery pattern.
type LQuery string

// LTxtQuery is an ltree ltxtquery full-text pattern.
type LTxtQuery string

// Hstore is an hstore: string keys to nullable string values.
type Hstore map[string]Null[string]

// Value implements driver.Valuer: the hstore text form.
func (h Hstore) Value() (driver.Value, error) {
	if h == nil {
		return nil, nil
	}
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sortStrings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(hstoreQuote(k))
		b.WriteString("=>")
		if v := h[k]; v.Valid {
			b.WriteString(hstoreQuote(v.V))
		} else {
			b.WriteString("NULL")
		}
	}
	return b.String(), nil
}

func hstoreQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		if s[i] == '"' || s[i] == '\\' {
			b.WriteByte('\\')
		}
		b.WriteByte(s[i])
	}
	b.WriteByte('"')
	return b.String()
}

// Scan implements sql.Scanner for the hstore text form.
func (h *Hstore) Scan(src any) error {
	s, ok, err := textOf(src)
	if err != nil || !ok {
		*h = nil
		return err
	}
	m := Hstore{}
	i := 0
	skip := func() {
		for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n') {
			i++
		}
	}
	quoted := func() (string, error) {
		if i >= len(s) || s[i] != '"' {
			return "", fmt.Errorf("qt: hstore: expected '\"' at %d in %q", i, s)
		}
		i++
		var b strings.Builder
		for i < len(s) {
			switch c := s[i]; c {
			case '\\':
				i++
				if i >= len(s) {
					return "", fmt.Errorf("qt: hstore: dangling escape in %q", s)
				}
				b.WriteByte(s[i])
			case '"':
				i++
				return b.String(), nil
			default:
				b.WriteByte(c)
			}
			i++
		}
		return "", fmt.Errorf("qt: hstore: unterminated string in %q", s)
	}
	for skip(); i < len(s); skip() {
		k, err := quoted()
		if err != nil {
			return err
		}
		skip()
		if !strings.HasPrefix(s[i:], "=>") {
			return fmt.Errorf("qt: hstore: expected '=>' at %d in %q", i, s)
		}
		i += 2
		skip()
		if strings.HasPrefix(s[i:], "NULL") {
			i += 4
			m[k] = Null[string]{}
		} else {
			v, err := quoted()
			if err != nil {
				return err
			}
			m[k] = Some(v)
		}
		skip()
		if i < len(s) {
			if s[i] != ',' {
				return fmt.Errorf("qt: hstore: expected ',' at %d in %q", i, s)
			}
			i++
		}
	}
	*h = m
	return nil
}

func textOf(src any) (string, bool, error) {
	switch v := src.(type) {
	case nil:
		return "", false, nil
	case string:
		return v, true, nil
	case []byte:
		return string(v), true, nil
	}
	return "", false, fmt.Errorf("qt: cannot scan %T from an extension type", src)
}

func formatFloats(v []float32) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(f), 'g', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}

func parseFloats(s string) ([]float32, error) {
	s = strings.TrimSpace(s)
	if len(s) < 2 || s[0] != '[' || s[len(s)-1] != ']' {
		return nil, fmt.Errorf("qt: vector: expected [..], got %q", s)
	}
	inner := strings.TrimSpace(s[1 : len(s)-1])
	if inner == "" {
		return []float32{}, nil
	}
	parts := strings.Split(inner, ",")
	out := make([]float32, len(parts))
	for i, p := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(p), 32)
		if err != nil {
			return nil, fmt.Errorf("qt: vector element %d: %w", i, err)
		}
		out[i] = float32(f)
	}
	return out, nil
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
