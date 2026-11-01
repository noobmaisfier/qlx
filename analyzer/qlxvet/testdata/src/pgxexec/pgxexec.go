// Package pgxexec is a stand-in with the same generic entry points as the real
// one, so the analyzer's V018 can be tested without pgx.
package pgxexec

import q "github.com/noobmaisfier/qlx"

func All[R any](ctx any, db any, st q.Statement) ([]R, error) { return nil, nil }
func One[R any](ctx any, db any, st q.Statement) (R, error)   { var r R; return r, nil }
func Maybe[R any](ctx any, db any, st q.Statement) (R, error) { var r R; return r, nil }

type Client struct{}

func NewStatic1[R any, A any](d any, s q.Statement, a A) R { var r R; return r }
