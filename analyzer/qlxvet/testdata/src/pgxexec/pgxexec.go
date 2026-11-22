// Package pgxexec is a stand-in with the same generic entry points as the real
// one, so the analyzer's V018 can be tested without pgx.
package pgxexec

import q "github.com/noobmaisfier/qlx"

func All[R any](ctx any, db any, st q.Statement) ([]R, error) { return nil, nil }
func One[R any](ctx any, db any, st q.Statement) (R, error)   { var r R; return r, nil }
func Maybe[R any](ctx any, db any, st q.Statement) (R, error) { var r R; return r, nil }

type Client struct{}

func NewStatic1[R any, A any](d any, s q.Statement, a A) R { var r R; return r }

// Page stands for the Go 1.27 generic methods Client.Page[R] and Tx.Page[R].
func (c *Client) Page(ctx any, base q.SelectQuery, sorts any, req any) (any, error) { return nil, nil }

type Tx struct{}

func (t *Tx) Page(ctx any, base q.SelectQuery, sorts any, req any) (any, error) { return nil, nil }
