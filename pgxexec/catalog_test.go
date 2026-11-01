package pgxexec_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	q "github.com/noobmaisfier/qlx"
	fx "github.com/noobmaisfier/qlx/fixture"
	"github.com/noobmaisfier/qlx/pgcat/fts"
	"github.com/noobmaisfier/qlx/pgcat/jsonb"
	"github.com/noobmaisfier/qlx/pgcat/misc"
	"github.com/noobmaisfier/qlx/pgcat/num"
	"github.com/noobmaisfier/qlx/pgcat/sys"
	"github.com/noobmaisfier/qlx/pgcat/text"
	"github.com/noobmaisfier/qlx/pgxexec"
	"github.com/noobmaisfier/qlx/qt"
)

// Generated catalog wrappers against a real server: a typed aggregate with
// FILTER, a variadic function and its NULL-aware result.
func TestGeneratedCatalogWrappers(t *testing.T) {
	p := pool(t)
	inRollbackTx(t, p, func(ctx context.Context, tx pgx.Tx) {
		for _, e := range []string{"g1@example.com", "g2@example.com"} {
			if _, err := pgxexec.Exec(ctx, tx, render(t, q.InsertInto(fx.Users).Row(fx.Users.Email.To(e)))); err != nil {
				t.Fatal(err)
			}
		}
		like := fx.Users.Email.Like("g_@example.com")
		all := render(t, q.Select(misc.BoolAnd(q.Val(true))).From(fx.Users).Where(like))
		if v, err := pgxexec.One[qt.Null[bool]](ctx, tx, all); err != nil || !v.Valid || !v.V {
			t.Fatalf("bool_and: %+v %v", v, err)
		}
		none := render(t, q.Select(misc.BoolAnd(q.Val(true))).From(fx.Users).Where(fx.Users.Email.Eq("nobody@example.com")))
		if v, err := pgxexec.One[qt.Null[bool]](ctx, tx, none); err != nil || v.Valid {
			t.Fatalf("bool_and over no rows must be NULL: %+v %v", v, err)
		}
		cc := render(t, q.Select(text.Concat(fx.Users.Email, q.Val("-x"))).From(fx.Users).Where(fx.Users.Email.Eq("g1@example.com")))
		if v, err := pgxexec.One[qt.Null[string]](ctx, tx, cc); err != nil || !v.Valid || v.V != "g1@example.com-x" {
			t.Fatalf("concat: %+v %v", v, err)
		}
	})
}

// Generated row sources and the new text-form types against a real server:
// a scalar set-returning function, a function with typed OUT columns, a
// jsonpath argument and a snapshot result.
func TestGeneratedRowSourcesAndTextTypes(t *testing.T) {
	p := pool(t)
	inRollbackTx(t, p, func(ctx context.Context, tx pgx.Tx) {
		x := q.NewField[int32]("x")
		s := num.GenerateSeriesInt4Int4(q.Val(int32(1)), q.Val(int32(3))).As("s", x)
		rows, err := pgxexec.All[int32](ctx, tx, render(t, q.Select(q.RefNum(s, x)).From(s).OrderBy(q.RefNum(s, x).Asc())))
		if err != nil || len(rows) != 3 || rows[0] != 1 || rows[2] != 3 {
			t.Fatalf("generate_series: %v %v", rows, err)
		}

		cols := sys.PgGetKeywordsCols
		ext := sys.PgGetKeywords().As("e", cols.Fields()...)
		names, err := pgxexec.All[qt.Null[string]](ctx, tx, render(t, q.Select(q.Ref(ext, cols.Word)).From(ext).Limit(1)))
		if err != nil || len(names) != 1 || !names[0].Valid {
			t.Fatalf("pg_get_keywords: %v %v", names, err)
		}

		doc := q.Val(qt.JSONB[json.RawMessage]{V: json.RawMessage(`{"a":[1,2]}`)})
		vars := q.Val(qt.JSONB[json.RawMessage]{V: json.RawMessage(`{}`)})
		ok, err := pgxexec.One[qt.Null[bool]](ctx, tx, render(t, q.Select(
			jsonb.JsonbPathExists(doc, q.Val(qt.JSONPath("$.a[*] ? (@ > 1)")), vars, q.Val(true)))))
		if err != nil || !ok.Valid || !ok.V {
			t.Fatalf("jsonb_path_exists: %+v %v", ok, err)
		}

		c, err := pgxexec.One[qt.Char](ctx, tx, render(t, q.Select(q.Val(qt.Char('z')))))
		if err != nil || c != 'z' {
			t.Fatalf(`"char" round trip: %q %v`, c, err)
		}
		sv, err := pgxexec.One[qt.TSVector](ctx, tx, render(t, q.Select(
			fts.SetweightTsvectorChar(q.Val(qt.TSVector("'a':1")), q.Val(qt.Char('A'))))))
		if err != nil || sv != "'a':1A" {
			t.Fatalf("setweight: %q %v", sv, err)
		}

		snap, err := pgxexec.One[qt.Snapshot](ctx, tx, render(t, q.Select(sys.PgCurrentSnapshot())))
		if err != nil || strings.Count(string(snap), ":") < 2 {
			t.Fatalf("pg_current_snapshot: %q %v", snap, err)
		}
	})
}

// Text-form types whose pgx codec decodes to a struct must still scan and bind.
func TestTextFormTypesRoundTrip(t *testing.T) {
	p := pool(t)
	inRollbackTx(t, p, func(ctx context.Context, tx pgx.Tx) {
		check := func(name string, got, want string, err error) {
			t.Helper()
			if err != nil || got != want {
				t.Errorf("%s: %q (want %q) %v", name, got, want, err)
			}
		}
		b, err := pgxexec.One[qt.Bit](ctx, tx, render(t, q.Select(q.Val(qt.Bit("1011")))))
		check("varbit", string(b), "1011", err)
		pt, err := pgxexec.One[qt.Point](ctx, tx, render(t, q.Select(q.Val(qt.Point("(1,2)")))))
		check("point", string(pt), "(1,2)", err)
		bx, err := pgxexec.One[qt.Box](ctx, tx, render(t, q.Select(q.Val(qt.Box("(2,2),(1,1)")))))
		check("box", string(bx), "(2,2),(1,1)", err)
		tq, err := pgxexec.One[qt.TSQuery](ctx, tx, render(t, q.Select(q.Val(qt.TSQuery("'a' & 'b'")))))
		check("tsquery", string(tq), "'a' & 'b'", err)
		tv, err := pgxexec.One[qt.TSVector](ctx, tx, render(t, q.Select(q.Val(qt.TSVector("'a':1 'b':2")))))
		check("tsvector", string(tv), "'a':1 'b':2", err)
	})
}
