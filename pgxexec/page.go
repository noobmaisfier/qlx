package pgxexec

import (
	"context"

	"github.com/noobmaisfier/qlx"
	"github.com/noobmaisfier/qlx/qpage"
)

// pageR is the whole of Client.Page and Tx.Page: it asks the sorts for the
// window of the request, adds it to base, runs the statement, and cuts the rows
// into a page. It is a plain generic function so it can be tested without Go
// 1.27 generic methods.
func pageR[R any](ctx context.Context, db Querier, res func(qlx.Statement) resolver,
	base qlx.SelectQuery, s *qpage.SortSet[R], req qpage.Request, binds []qlx.Binding) (qpage.Page[R], error) {
	w, err := s.Window(req)
	if err != nil {
		return qpage.Page[R]{}, err
	}
	sel, err := w.Apply(base)
	if err != nil {
		return qpage.Page[R]{}, err
	}
	rows, err := allR[R](ctx, db, res(sel), binds)
	if err != nil {
		return qpage.Page[R]{}, err
	}
	return s.Page(w, rows)
}
