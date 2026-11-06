package qlx

import (
	"fmt"
	"slices"
	"strings"
)

// Orderable is anything with Asc and Desc: every column and expression.
type Orderable interface {
	Asc() OrderTerm
	Desc() OrderTerm
}

// Sorts turns a user-supplied sort string ("-created,email") into ORDER BY
// terms, accepting only the names of an allowlist. A name never reaches the
// SQL: it selects a column the program declared, so a client cannot order by
// something it should not see, and the set of statement shapes stays bounded
// by the allowlist.
//
//	var userSorts = qlx.NewSorts(map[string]qlx.Orderable{
//		"email":   Users.Email,
//		"created": Users.CreatedAt,
//	}).Default(Users.CreatedAt.Desc()).Tiebreak(Users.ID.Asc())
//
//	order, err := userSorts.Parse(r.URL.Query().Get("sort")) // "-created,email"
//	if err != nil { /* 400 */ }
//	sel = sel.OrderBy(order...)
//
// A name prefixed with "-" sorts descending; "+" is accepted as ascending.
type Sorts struct {
	allowed map[string]Orderable
	def     []OrderTerm
	tie     []OrderTerm
	max     int
}

// DefaultMaxSortKeys is the number of keys Parse accepts unless Max changes it.
const DefaultMaxSortKeys = 3

// NewSorts builds an allowlist. It panics on an empty name or one that starts
// with "-" or "+" or contains a comma, since Parse could never match it: that
// is a programming error, found when the package variable is initialised.
func NewSorts(allowed map[string]Orderable) Sorts {
	m := make(map[string]Orderable, len(allowed))
	for name, o := range allowed {
		if name == "" || strings.ContainsAny(name, ",") || name[0] == '-' || name[0] == '+' || strings.TrimSpace(name) != name {
			panic(fmt.Sprintf("qlx: invalid sort name %q", name))
		}
		if o == nil {
			panic(fmt.Sprintf("qlx: sort name %q has no column", name))
		}
		m[name] = o
	}
	return Sorts{allowed: m, max: DefaultMaxSortKeys}
}

// Default is the order used when the sort string is empty.
func (s Sorts) Default(ts ...OrderTerm) Sorts { s.def = slices.Clone(ts); return s }

// Tiebreak terms are always appended last, so that the order is total and
// pages do not overlap (usually the primary key).
func (s Sorts) Tiebreak(ts ...OrderTerm) Sorts { s.tie = slices.Clone(ts); return s }

// Max sets the number of keys Parse accepts.
func (s Sorts) Max(n int) Sorts { s.max = n; return s }

// Names lists the allowed names, sorted.
func (s Sorts) Names() []string {
	names := make([]string, 0, len(s.allowed))
	for n := range s.allowed {
		names = append(names, n)
	}
	slices.Sort(names)
	return names
}

// Parse returns the ORDER BY terms for spec, then the tiebreak terms. An
// empty spec gives the default order. An unknown name, an empty item, a
// repeated name or too many keys is an error wrapping ErrSort.
func (s Sorts) Parse(spec string) ([]OrderTerm, error) {
	spec = strings.TrimSpace(spec)
	var out []OrderTerm
	if spec == "" {
		out = slices.Clone(s.def)
	} else {
		items := strings.Split(spec, ",")
		if len(items) > s.max {
			return nil, fmt.Errorf("%w: %d keys, at most %d", ErrSort, len(items), s.max)
		}
		seen := make(map[string]bool, len(items))
		for _, it := range items {
			name, desc := strings.TrimSpace(it), false
			switch {
			case strings.HasPrefix(name, "-"):
				name, desc = name[1:], true
			case strings.HasPrefix(name, "+"):
				name = name[1:]
			}
			if name == "" {
				return nil, fmt.Errorf("%w: empty key", ErrSort)
			}
			o, ok := s.allowed[name]
			if !ok {
				return nil, fmt.Errorf("%w: unknown key %q (allowed: %s)", ErrSort, name, strings.Join(s.Names(), ", "))
			}
			if seen[name] {
				return nil, fmt.Errorf("%w: key %q repeated", ErrSort, name)
			}
			seen[name] = true
			if desc {
				out = append(out, o.Desc())
			} else {
				out = append(out, o.Asc())
			}
		}
	}
	return append(out, s.tie...), nil
}
