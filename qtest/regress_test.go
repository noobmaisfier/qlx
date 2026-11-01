package qtest

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/noobmaisfier/qlx"
	"github.com/noobmaisfier/qlx/fixture"
)

func TestRunRegress(t *testing.T) {
	d := qlx.Dialect{Version: qlx.PG18}
	norm := func(s string) (string, error) { return strings.Join(strings.Fields(strings.ToLower(s)), " "), nil }
	good := RegressCase{ID: "good", Original: `select "id" from "public"."users"`, Build: func() qlx.Statement {
		return qlx.Select(fixture.Users.ID).From(fixture.Users)
	}}
	if err := runOne(good, RegressOptions{Dialect: d, Normalize: func(s string) (string, error) {
		return "x", nil
	}}); err != nil {
		t.Fatal(err)
	}
	bad := RegressCase{ID: "bad", Original: "select 1", Build: good.Build}
	if err := runOne(bad, RegressOptions{Dialect: d, Normalize: norm}); err == nil {
		t.Fatal("different SQL must fail")
	}
	// parser does not know the syntax: the server decides
	unknown := func(string) (string, error) { return "", errors.New("syntax error") }
	called := false
	prep := func(context.Context, string, string) error { called = true; return nil }
	if err := runOne(good, RegressOptions{Dialect: d, Normalize: unknown, Prepare: prep}); err != nil || !called {
		t.Fatalf("fallback not used: %v %v", err, called)
	}
	if err := runOne(good, RegressOptions{Dialect: d, Normalize: unknown}); err == nil {
		t.Fatal("no oracle must fail")
	}
}
