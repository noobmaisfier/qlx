// Package qpage is cursor pagination for a service layer.
//
// Declare the sortable fields once; qpage builds the keyset query pieces, signs
// the cursor, and cuts the rows into a page and the next cursor. It knows
// nothing about HTTP or JSON envelopes: a request is a sort name, a cursor
// string and a size, and the answer is the items and the next cursor string.
//
//	var userSorts = qpage.Sorts[UserRow](
//	    qpage.Sort("created", qpage.Desc(Users.CreatedAt)), // the first sort is the default
//	    qpage.Sort("email", qpage.Asc(Users.Email)),
//	    qpage.Tiebreak(qpage.Desc(Users.ID)),               // ends every sort
//	)
//
//	// at startup, once:
//	qpage.SetSecret(secret)
//
//	// with pgxexec, one call:
//	page, err := client.Page[UserRow](ctx, baseSelect, userSorts, qpage.Request{Sort: s, Cursor: c, Size: n})
//
//	// or run the query yourself:
//	w, err := userSorts.Window(req)
//	sel, err := w.Apply(baseSelect)
//	rows := ...                                  // run sel
//	page, err := userSorts.Page(w, rows)
//
// The value of each key is read from the last row of a page the way pgxexec
// scans that row type: for a struct scanned by name, the field named by its
// `db` tag (else its lowercased name); for a row scanned by position (it
// implements ScanTargets: generated rows, structs marked //qlx:row), the field
// at the position of the key column in the SELECT. The key columns must be in
// the select list under their own names. They must be NOT NULL and, with the
// tiebreak, unique: a NULL never compares, and two rows with the same key
// would be skipped or repeated at a page boundary.
package qpage

import (
	"errors"
	"fmt"

	q "github.com/noobmaisfier/qlx"
)

// Errors. A bad request is ErrSort or ErrCursor (answer 400); ErrCursorKey is
// a missing SetSecret (answer 500).
var (
	// ErrSort is the sentinel of qlx.NewSorts: one errors.Is covers a sort
	// refused by either.
	ErrSort = q.ErrSort
	// ErrCursor means the cursor is not one this program issued: malformed,
	// altered, signed with an unknown secret, or made for another sort. The
	// reason is deliberately not part of the error.
	ErrCursor = errors.New("qpage: invalid cursor")
)

// Default and maximum page size, changed with Sizes.
const (
	DefaultSize = 20
	DefaultMax  = 100
)

// Request is what a caller asks for: all three may be empty.
type Request struct {
	Sort   string // a name declared with Sort; empty is the default sort
	Cursor string // the Next of the previous page; empty is the first page
	Size   int    // not positive is the default size, above the maximum is capped
}

// Page is one page of rows and the cursor of the next one, empty on the last
// page. Items is never nil.
type Page[R any] struct {
	Items []R
	Next  string
}

// Window is what the query of one page needs, made by SortSet.Window.
type Window struct {
	// After is the predicate for the rows after the cursor; absent on the
	// first page.
	After q.Pred
	// Order is the ORDER BY of the sort and its tiebreak.
	Order []q.OrderTerm
	// Limit is one more than the page size: the extra row tells whether a next
	// page exists.
	Limit int64

	size int
	sort *sortDef
	pos  []int // by position: the column of the SELECT that holds each key, set by Apply
}

// Size is the page size after defaulting and capping.
func (w Window) Size() int { return w.size }

// Apply adds the cursor predicate, the order and the limit to a SELECT, and
// checks that the SELECT returns the key columns under their own names. It
// must be called before SortSet.Page, on the Window that Page receives.
//
// The SELECT must not have its own ORDER BY or LIMIT. For a row type scanned by
// position (it implements ScanTargets), the column of each key must hold a
// value of the key's type at the position where the row type expects it.
func (w *Window) Apply(sel q.SelectQuery) (q.SelectQuery, error) {
	if w.sort == nil {
		return sel, errors.New("qpage: Window not made by SortSet.Window")
	}
	index := map[string]int{}
	for i, n := range sel.OutputNames() {
		if _, dup := index[n]; dup {
			index[n] = -1 // returned twice: which one is the key?
			continue
		}
		index[n] = i
	}
	pos := make([]int, len(w.sort.keys))
	for i, k := range w.sort.keys {
		p, ok := index[k.col]
		switch {
		case !ok:
			return sel, errors.New("qpage: sort " + w.sort.name + " is keyed on " + k.col + ", which the SELECT does not return under that name")
		case p < 0:
			return sel, errors.New("qpage: sort " + w.sort.name + ": the SELECT returns " + k.col + " twice")
		}
		if cols := w.sort.cols; cols != nil {
			if p >= len(cols) {
				return sel, fmt.Errorf("qpage: sort %s: %s is column %d of the SELECT, but the row type scans only %d columns", w.sort.name, k.col, p+1, len(cols))
			}
			if cols[p] != k.typ {
				return sel, fmt.Errorf("qpage: sort %s: %s is column %d of the SELECT, where the row type holds a %s, not a %s (is the select list in the order of the fields?)", w.sort.name, k.col, p+1, cols[p], k.typ)
			}
		}
		pos[i] = p
	}
	if w.sort.cols != nil {
		w.pos = pos
	}
	return sel.Where(w.After).OrderBy(w.Order...).Limit(w.Limit), nil
}
