package qlx

import (
	"fmt"
	"slices"
)

// MaxBindParams is the number of parameters one statement can carry: the
// PostgreSQL wire protocol counts them in 16 bits.
const MaxBindParams = 65535

// Chunks splits a bulk insert into statements that each stay under the
// parameter limit. base holds the target, ON CONFLICT and RETURNING clauses
// and no rows; every chunk is base plus its share of rows.
//
// A row costs one parameter per assignment. Assignments built with ToX over an
// expression that itself carries several arguments cost more: pass a lower
// maxParams for those. maxParams <= 0 means MaxBindParams.
//
// Every statement but the last has the same number of rows; the last may be
// shorter, which is one more shape in the cache.
func (q InsertQuery) Chunks(rows []RowAssigner, maxParams int) ([]InsertQuery, error) {
	if len(q.rows) > 0 || q.source != nil || q.defaults {
		return nil, fmt.Errorf("%w: Chunks needs an insert without rows, source or DEFAULT VALUES", ErrShape)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	if maxParams <= 0 {
		maxParams = MaxBindParams
	}
	live0 := live(rows[0].Assignments())
	if len(live0) == 0 {
		return nil, fmt.Errorf("%w: the first row assigns no column", ErrShape)
	}
	per := maxParams / len(live0)
	if per < 1 {
		return nil, fmt.Errorf("%w: one row needs %d parameters, more than the %d allowed", ErrShape, len(live0), maxParams)
	}
	var out []InsertQuery
	for start := 0; start < len(rows); start += per {
		end := min(start+per, len(rows))
		c := q
		c.rows = nil
		for i, r := range rows[start:end] {
			as := live(r.Assignments())
			if len(as) != len(live0) {
				return nil, fmt.Errorf("%w: row %d assigns %d columns, the first assigns %d", ErrShape, start+i, len(as), len(live0))
			}
			c.rows = append(c.rows, slices.Clone(as))
		}
		out = append(out, c)
	}
	return out, nil
}
