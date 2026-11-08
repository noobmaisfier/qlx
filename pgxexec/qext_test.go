package pgxexec_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	q "github.com/noobmaisfier/qlx"
	"github.com/noobmaisfier/qlx/pgxexec"
	hstoreh "github.com/noobmaisfier/qlx/qext/hstore"
	ltreeh "github.com/noobmaisfier/qlx/qext/ltree"
	"github.com/noobmaisfier/qlx/qext/pgvector"
	"github.com/noobmaisfier/qlx/qext/textsearch"
	"github.com/noobmaisfier/qlx/qext/trgm"
	"github.com/noobmaisfier/qlx/qt"
)

// The helpers of qext against a real server: what they return, not only the
// SQL they render.

func TestTextSearchHelpers(t *testing.T) {
	p := pool(t)
	inRollbackTx(t, p, func(ctx context.Context, tx pgx.Tx) {
		doc := textsearch.Document("english",
			textsearch.Weighted(textsearch.A, q.Val("Quick brown foxes")),
			textsearch.Weighted(textsearch.B, q.Val("The lazy dog jumped over the fence")))
		matches := func(text string) bool {
			t.Helper()
			ok, err := pgxexec.One[bool](ctx, tx, render(t, q.Select(q.Test(textsearch.Search(doc, "english", q.Val(text))))))
			if err != nil {
				t.Fatalf("%q: %v", text, err)
			}
			return ok
		}
		for text, want := range map[string]bool{
			"quick fox":        true,  // stemming: foxes ~ fox
			`"brown foxes"`:    true,  // a phrase
			`"fox brown"`:      false, // wrong order
			"quick -dog":       false, // exclusion
			"quick or zebra":   true,
			"zebra":            false,
			`broken "unclosed`: false, // malformed text is not an error
			"((( & | !":        false,
			"jumping":          true, // jumped ~ jumping
		} {
			if got := matches(text); got != want {
				t.Errorf("Search(%q) = %v, want %v", text, got, want)
			}
		}

		// A title match (weight A) ranks above a body match (weight B).
		query := textsearch.Web("english", q.Val("fox"))
		titleHit := textsearch.Document("english", textsearch.Weighted(textsearch.A, q.Val("fox")), textsearch.Weighted(textsearch.B, q.Val("nothing")))
		bodyHit := textsearch.Document("english", textsearch.Weighted(textsearch.A, q.Val("nothing")), textsearch.Weighted(textsearch.B, q.Val("fox")))
		rt, _ := pgxexec.One[float32](ctx, tx, render(t, q.Select(textsearch.Rank(titleHit, query))))
		rb, _ := pgxexec.One[float32](ctx, tx, render(t, q.Select(textsearch.Rank(bodyHit, query))))
		if !(rt > rb && rb > 0) {
			t.Errorf("rank: title %v, body %v", rt, rb)
		}

		// The highlight options are data: quotes in them cannot break out.
		h, err := pgxexec.One[string](ctx, tx, render(t, q.Select(textsearch.Headline("english",
			q.Val("The quick brown fox"), query, textsearch.Highlight{StartSel: `<mark "x">`, StopSel: "</mark>", MaxWords: 5}))))
		if err != nil || !strings.Contains(h, `<mark "x">fox</mark>`) {
			t.Errorf("headline: %q %v", h, err)
		}
		h, err = pgxexec.One[string](ctx, tx, render(t, q.Select(textsearch.Headline("", q.Val("a fox"), query, textsearch.Highlight{}))))
		if err != nil || !strings.Contains(h, "<b>fox</b>") {
			t.Errorf("default headline: %q %v", h, err)
		}
	})
}

func TestPGVectorHelpers(t *testing.T) {
	p := pool(t)
	inRollbackTx(t, p, func(ctx context.Context, tx pgx.Tx) {
		if _, err := tx.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS vector"); err != nil {
			t.Skipf("pgvector not available: %v", err)
		}
		vt := q.NewPgType[qt.Vector]("", "vector")
		ht := q.NewPgType[qt.HalfVector]("", "halfvec")
		col := q.Cast(q.Val(qt.Vector{1, 2}), vt)
		hcol := q.Cast(q.Val(qt.HalfVector{1, 2}), ht)
		get := func(e q.Selectable) float64 {
			t.Helper()
			v, err := pgxexec.One[qt.Null[float64]](ctx, tx, render(t, q.Select(e)))
			if err != nil || !v.Valid {
				t.Fatalf("%v %+v", err, v)
			}
			return v.V
		}
		if d := get(pgvector.L2(col, qt.Vector{4, 6})); d != 5 {
			t.Errorf("L2 = %v, want 5", d)
		}
		if d := get(pgvector.Cosine(col, qt.Vector{2, 4})); d > 1e-9 {
			t.Errorf("Cosine of parallel vectors = %v, want 0", d)
		}
		if d := get(pgvector.NegativeInnerProduct(col, qt.Vector{3, 4})); d != -11 {
			t.Errorf("NegativeInnerProduct = %v, want -11", d)
		}
		if d := get(pgvector.L1(col, qt.Vector{4, 6})); d != 7 {
			t.Errorf("L1 = %v, want 7", d)
		}
		if d := get(pgvector.L2(hcol, qt.HalfVector{4, 6})); d != 5 {
			t.Errorf("L2 on halfvec = %v, want 5", d)
		}

		// The settings run, and the nearest row is the one asked for.
		for _, c := range []q.Command{pgvector.SetEfSearch(80), pgvector.SetProbes(5)} {
			if _, err := pgxexec.Exec(ctx, tx, render(t, c)); err != nil {
				t.Fatalf("%v", err)
			}
		}
		if _, err := tx.Exec(ctx, "CREATE TEMP TABLE items (id int, emb vector(2))"); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, "INSERT INTO items VALUES (1,'[0,0]'),(2,'[1,1]'),(3,'[5,5]')"); err != nil {
			t.Fatal(err)
		}
		var id int
		sql := "SELECT id FROM items ORDER BY emb " + "<->" + " CAST($1 AS vector) LIMIT 1"
		if err := tx.QueryRow(ctx, sql, "[4,4]").Scan(&id); err != nil || id != 3 {
			t.Errorf("nearest = %d %v", id, err)
		}
	})
}

func TestTrgmHelpers(t *testing.T) {
	p := pool(t)
	inRollbackTx(t, p, func(ctx context.Context, tx pgx.Tx) {
		if _, err := tx.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS pg_trgm"); err != nil {
			t.Skipf("pg_trgm not available: %v", err)
		}
		word := func(s string) q.Operand[string] { return q.Cast(q.Val(s), q.PgText) }
		ok := func(p q.Pred) bool {
			t.Helper()
			v, err := pgxexec.One[bool](ctx, tx, render(t, q.Select(q.Test(p))))
			if err != nil {
				t.Fatal(err)
			}
			return v
		}
		if !ok(trgm.Similar(word("jonathan"), "jonathon")) || ok(trgm.Similar(word("jonathan"), "zzzzzz")) {
			t.Error("Similar")
		}
		if !ok(trgm.WordSimilar(word("the quick brown fox"), "quic")) {
			t.Error("WordSimilar")
		}
		d, err := pgxexec.One[float32](ctx, tx, render(t, q.Select(trgm.Distance(word("hello"), "hello"))))
		if err != nil || d != 0 {
			t.Errorf("Distance of equal text = %v %v", d, err)
		}
		s, err := pgxexec.One[float32](ctx, tx, render(t, q.Select(trgm.Similarity(word("hello"), "hello"))))
		if err != nil || s != 1 {
			t.Errorf("Similarity of equal text = %v %v", s, err)
		}
		// The threshold setting takes effect for Similar.
		if _, err := pgxexec.Exec(ctx, tx, render(t, trgm.SetSimilarityThreshold(0.99))); err != nil {
			t.Fatal(err)
		}
		if ok(trgm.Similar(word("jonathan"), "jonathon")) {
			t.Error("a 0.99 threshold should reject a near match")
		}
	})
}

func TestLtreeHelpers(t *testing.T) {
	p := pool(t)
	inRollbackTx(t, p, func(ctx context.Context, tx pgx.Tx) {
		if _, err := tx.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS ltree"); err != nil {
			t.Skipf("ltree not available: %v", err)
		}
		lt := q.NewPgType[qt.LTree]("", "ltree")
		path := func(s string) q.Operand[qt.LTree] { return q.Cast(q.Val(qt.LTree(s)), lt) }
		ok := func(p q.Pred) bool {
			t.Helper()
			v, err := pgxexec.One[bool](ctx, tx, render(t, q.Select(q.Test(p))))
			if err != nil {
				t.Fatal(err)
			}
			return v
		}
		if !ok(ltreeh.IsDescendant(path("a.b.c"), path("a.b"))) || ok(ltreeh.IsDescendant(path("a.b"), path("a.b.c"))) {
			t.Error("IsDescendant")
		}
		if !ok(ltreeh.IsAncestor(path("a.b"), path("a.b.c"))) || ok(ltreeh.IsAncestor(path("a.b.c"), path("a.b"))) {
			t.Error("IsAncestor")
		}
		if !ok(ltreeh.Matches(path("top.science.physics"), "*.science.*")) || ok(ltreeh.Matches(path("top.art"), "*.science.*")) {
			t.Error("Matches")
		}
		if n, err := pgxexec.One[int32](ctx, tx, render(t, q.Select(ltreeh.Depth(path("a.b.c"))))); err != nil || n != 3 {
			t.Errorf("Depth = %d %v", n, err)
		}
	})
}

func TestHstoreHelpers(t *testing.T) {
	p := pool(t)
	inRollbackTx(t, p, func(ctx context.Context, tx pgx.Tx) {
		if _, err := tx.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS hstore"); err != nil {
			t.Skipf("hstore not available: %v", err)
		}
		ht := q.NewPgType[qt.Hstore]("", "hstore")
		col := q.Cast(q.Val(qt.Hstore{"a": qt.Some("1"), "b": qt.Some("2")}), ht)
		ok := func(p q.Pred) bool {
			t.Helper()
			v, err := pgxexec.One[bool](ctx, tx, render(t, q.Select(q.Test(p))))
			if err != nil {
				t.Fatal(err)
			}
			return v
		}
		if v, err := pgxexec.One[qt.Null[string]](ctx, tx, render(t, q.Select(hstoreh.Get(col, "b")))); err != nil || !v.Valid || v.V != "2" {
			t.Errorf("Get = %+v %v", v, err)
		}
		if v, err := pgxexec.One[qt.Null[string]](ctx, tx, render(t, q.Select(hstoreh.Get(col, "zz")))); err != nil || v.Valid {
			t.Errorf("Get of a missing key = %+v %v, want NULL", v, err)
		}
		if !ok(hstoreh.HasKey(col, "a")) || ok(hstoreh.HasKey(col, "zz")) {
			t.Error("HasKey")
		}
		if !ok(hstoreh.Contains(col, qt.Hstore{"a": qt.Some("1")})) || ok(hstoreh.Contains(col, qt.Hstore{"a": qt.Some("9")})) {
			t.Error("Contains")
		}
		m := hstoreh.Merge(col, qt.Hstore{"b": qt.Some("20"), "c": qt.Some("3")})
		if v, err := pgxexec.One[qt.Null[string]](ctx, tx, render(t, q.Select(hstoreh.Get(m, "b")))); err != nil || v.V != "20" {
			t.Errorf("Merge: b = %+v %v", v, err)
		}
	})
}
