// Package qt holds the small value types shared by generated code and the
// query builder: Null[T] for nullable columns and Opt[T] for optional
// patch fields and dynamic filters.
package qt

import (
	"database/sql"
	"database/sql/driver"
	"encoding"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/netip"
	"strings"
)

// Null is a nullable value. The zero value is NULL.
type Null[T any] struct {
	V     T
	Valid bool
}

// Some returns a non-NULL Null.
func Some[T any](v T) Null[T] { return Null[T]{V: v, Valid: true} }

// None returns NULL.
func None[T any]() Null[T] { return Null[T]{} }

// QLXArg lets the renderer bind Null[T] as nil or the plain value, so any
// driver sees a standard argument.
func (n Null[T]) QLXArg() any {
	if !n.Valid {
		return nil
	}
	return n.V
}

// Ptr returns nil for NULL, otherwise a pointer to a copy of the value.
func (n Null[T]) Ptr() *T {
	if !n.Valid {
		return nil
	}
	v := n.V
	return &v
}

// Scan implements sql.Scanner so Null[T] works with database/sql and with pgx,
// which both accept it. Conversions are the standard library's (sql.Null[T]),
// with one extension: when that cannot convert a text source (pgx hands
// inet, cidr and uuid columns over as strings) and *T implements
// encoding.TextUnmarshaler (netip.Addr, netip.Prefix), the text is parsed
// with it; a [16]byte target accepts the canonical uuid text form.
func (n *Null[T]) Scan(src any) error {
	var s sql.Null[T]
	err := s.Scan(src)
	if err == nil {
		n.V, n.Valid = s.V, s.Valid
		return nil
	}
	var text string
	switch x := src.(type) {
	case string:
		text = x
	case []byte:
		text = string(x)
	default:
		return err
	}
	var v T
	switch p := any(&v).(type) {
	case *[16]byte:
		u, perr := parseUUIDText(text)
		if perr != nil {
			return perr
		}
		*p = u
	case *netip.Addr:
		// inet text form carries a mask: a host address comes as "10.0.0.1/32".
		if i := strings.IndexByte(text, '/'); i >= 0 {
			pf, perr := netip.ParsePrefix(text)
			if perr != nil || pf.Bits() != pf.Addr().BitLen() {
				return fmt.Errorf("qt: inet %q has a network mask, scan into netip.Prefix", text)
			}
			*p = pf.Addr()
			break
		}
		a, perr := netip.ParseAddr(text)
		if perr != nil {
			return perr
		}
		*p = a
	case encoding.TextUnmarshaler:
		if perr := p.UnmarshalText([]byte(text)); perr != nil {
			return perr
		}
	default:
		return err
	}
	n.V, n.Valid = v, true
	return nil
}

func parseUUIDText(s string) ([16]byte, error) {
	var u [16]byte
	h := make([]byte, 0, 32)
	for i := 0; i < len(s); i++ {
		if s[i] != '-' {
			h = append(h, s[i])
		}
	}
	if len(h) != 32 {
		return u, fmt.Errorf("qt: invalid uuid %q", s)
	}
	if _, err := hex.Decode(u[:], h); err != nil {
		return u, fmt.Errorf("qt: invalid uuid %q", s)
	}
	return u, nil
}

// Value implements driver.Valuer.
func (n Null[T]) Value() (driver.Value, error) {
	return sql.Null[T]{V: n.V, Valid: n.Valid}.Value()
}

// MarshalJSON encodes NULL as null and a value as the value itself.
func (n Null[T]) MarshalJSON() ([]byte, error) {
	if !n.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(n.V)
}

// UnmarshalJSON decodes null as NULL and anything else as a value.
func (n *Null[T]) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*n = Null[T]{}
		return nil
	}
	if err := json.Unmarshal(b, &n.V); err != nil {
		return err
	}
	n.Valid = true
	return nil
}

// holdsNull marks Null as a type that can represent a JSON null, so an
// Opt[Null[T]] sets itself when the key is present with a null.
func (Null[T]) holdsNull() {}

// Opt is an optional value: unset means "leave out". It is distinct from
// Null so that "do not touch this column" and "set it to NULL" cannot be
// confused.
type Opt[T any] struct {
	V   T
	Set bool
}

// Of returns a set Opt.
func Of[T any](v T) Opt[T] { return Opt[T]{V: v, Set: true} }

// Unset returns an absent Opt.
func Unset[T any]() Opt[T] { return Opt[T]{} }

// FromPtr turns a pointer into a Null: nil is NULL, otherwise a copy of the
// pointed-to value. It is the bridge from APIs that model optionality with
// pointers (JSON payloads, protobuf) to QLX's explicit Null[T].
func FromPtr[T any](p *T) Null[T] {
	if p == nil {
		return Null[T]{}
	}
	return Null[T]{V: *p, Valid: true}
}
