// Package pgxexec is a stand-in for the real one with its Go 1.27 generic
// methods on Client (client.One[R](ctx, st)). It needs a Go 1.27 toolchain.
package pgxexec

import (
	"iter"

	q "github.com/noobmaisfier/qlx"
)

type Client struct{}

func (c *Client) All[R any](ctx any, st q.Statement, binds ...q.Binding) ([]R, error) {
	return nil, nil
}

func (c *Client) One[R any](ctx any, st q.Statement, binds ...q.Binding) (R, error) {
	var r R
	return r, nil
}

func (c *Client) Maybe[R any](ctx any, st q.Statement, binds ...q.Binding) (R, error) {
	var r R
	return r, nil
}

func (c *Client) Each[R any](ctx any, st q.Statement, binds ...q.Binding) iter.Seq2[R, error] {
	return nil
}

func (c *Client) Exec(ctx any, st q.Statement, binds ...q.Binding) error { return nil }
