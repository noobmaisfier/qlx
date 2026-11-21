package qt

import (
	"encoding/json"
	"fmt"
	"strings"
)

// The String methods below are for logs and error messages: fmt's %v, %s and
// Println print the value, not the struct that holds it. They are display
// only; use MarshalJSON, the pgx codec or Value to move data.

// String prints NULL, or the value as fmt prints it.
func (n Null[T]) String() string {
	if !n.Valid {
		return "NULL"
	}
	return fmt.Sprint(n.V)
}

// String prints "unset", or the value as fmt prints it (an unset Opt[Null[T]]
// is "unset", a set one that holds NULL is "NULL").
func (o Opt[T]) String() string {
	if !o.Set {
		return "unset"
	}
	return fmt.Sprint(o.V)
}

// String prints the document as compact JSON.
func (j JSONB[T]) String() string {
	b, err := json.Marshal(j.V)
	if err != nil {
		return fmt.Sprint(j.V)
	}
	return string(b)
}

// String prints the range the way PostgreSQL writes it: [1,5), (,9], empty,
// or NULL. A bound is printed as fmt prints it, without PostgreSQL's quoting.
func (r Range[T]) String() string {
	switch {
	case r.Null:
		return "NULL"
	case r.Empty:
		return "empty"
	}
	var b strings.Builder
	if r.LowerInc {
		b.WriteByte('[')
	} else {
		b.WriteByte('(')
	}
	if r.Lower != nil {
		fmt.Fprint(&b, *r.Lower)
	}
	b.WriteByte(',')
	if r.Upper != nil {
		fmt.Fprint(&b, *r.Upper)
	}
	if r.UpperInc {
		b.WriteByte(']')
	} else {
		b.WriteByte(')')
	}
	return b.String()
}

// String prints the multirange as PostgreSQL writes it: {[1,3),[5,9)}, {} for
// the empty one, NULL for a nil slice.
func (m Multirange[T]) String() string {
	if m == nil {
		return "NULL"
	}
	parts := make([]string, len(m))
	for i, r := range m {
		parts[i] = r.String()
	}
	return "{" + strings.Join(parts, ",") + "}"
}
