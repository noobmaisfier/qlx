package catalog_test

import (
	"os"
	"strings"
	"testing"

	"github.com/noobmaisfier/qlx/internal/catalog"
	"github.com/noobmaisfier/qlx/internal/pgtool"
)

func server(t *testing.T) *pgtool.Server {
	t.Helper()
	srv, err := pgtool.StartEphemeral("")
	if err != nil {
		t.Skipf("no PostgreSQL server binaries: %v", err)
	}
	t.Cleanup(srv.Close)
	if err := srv.Conn().ExecFile("../../fixture/schema.sql"); err != nil {
		t.Fatal(err)
	}
	return srv
}

func TestLoadAndLock(t *testing.T) {
	srv := server(t)
	s, err := catalog.Load(srv.Conn(), []string{"public"})
	if err != nil {
		t.Fatal(err)
	}
	var members *catalog.Table
	for i := range s.Tables {
		if s.Tables[i].Name == "members" {
			members = &s.Tables[i]
		}
	}
	if members == nil {
		t.Fatal("members table missing")
	}
	byName := map[string]catalog.Column{}
	for _, c := range members.Columns {
		byName[c.Name] = c
	}
	if !byName["tier"].NotNull || byName["settings"].NotNull {
		t.Errorf("nullability wrong: %+v %+v", byName["tier"], byName["settings"])
	}
	if byName["slug"].Generated != "s" || byName["ref"].Identity != "a" || !byName["id"].HasDefault {
		t.Errorf("generated/identity/default wrong: %+v", byName)
	}
	if len(members.PrimaryKey) != 1 || members.PrimaryKey[0] != "id" {
		t.Errorf("primary key: %v", members.PrimaryKey)
	}
	if members.Fingerprint == "" || len(s.Enums) != 1 || len(s.Enums[0].Labels) != 3 {
		t.Errorf("fingerprint %q, enums %+v", members.Fingerprint, s.Enums)
	}
	if len(s.Funcs) != 2 {
		t.Errorf("funcs: %+v", s.Funcs)
	}

	lock1, err := s.Lock()
	if err != nil {
		t.Fatal(err)
	}
	s2, err := catalog.Load(srv.Conn(), []string{"public"})
	if err != nil {
		t.Fatal(err)
	}
	lock2, _ := s2.Lock()
	if string(lock1) != string(lock2) {
		t.Fatal("two loads of the same schema give different schema.lock")
	}
	if _, err := catalog.ParseLock(lock1); err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(lock1), `"members"`, `"membars"`, 1)
	if _, err := catalog.ParseLock([]byte(tampered)); err == nil {
		t.Fatal("tampered lock must fail its hash check")
	}
	_ = os.Stdout
}

// A view column is NOT NULL when the stored query proves it; anything else
// stays nullable (PostgreSQL itself never says: attnotnull is false on views).
func TestViewColumnNullabilityFromTheServer(t *testing.T) {
	srv := server(t)
	sql := `
CREATE SCHEMA vn;
CREATE DOMAIN vn.bal_nn AS bigint NOT NULL;
CREATE TABLE vn.accounts (id bigint PRIMARY KEY, email text NOT NULL, balance bigint NOT NULL DEFAULT 0, note text);
CREATE TABLE vn.items (id bigint PRIMARY KEY, account_id bigint REFERENCES vn.accounts(id), qty int NOT NULL);
CREATE VIEW vn.plain AS SELECT id, email, note FROM vn.accounts;
CREATE VIEW vn.inner_join AS SELECT a.id, i.qty FROM vn.accounts a JOIN vn.items i ON i.account_id = a.id;
CREATE VIEW vn.left_join AS SELECT a.id, i.qty FROM vn.accounts a LEFT JOIN vn.items i ON i.account_id = a.id;
CREATE VIEW vn.right_join AS SELECT a.id, i.qty FROM vn.accounts a RIGHT JOIN vn.items i ON i.account_id = a.id;
CREATE VIEW vn.expr AS SELECT id, balance + 1 AS plus, upper(email) AS up, CASE WHEN id > 1 THEN email END AS c, 1 AS one, count(*) OVER () AS n FROM vn.accounts;
CREATE VIEW vn.uni AS SELECT id, email FROM vn.accounts UNION ALL SELECT id, 'x' FROM vn.accounts;
CREATE VIEW vn.agg AS SELECT email, count(*) AS n, max(balance) AS top FROM vn.accounts GROUP BY email;
CREATE VIEW vn.dom AS SELECT id, balance::vn.bal_nn AS b FROM vn.accounts;
CREATE VIEW vn.over AS SELECT id, email FROM vn.plain;
CREATE VIEW vn.overleft AS SELECT id, qty FROM vn.left_join;
CREATE MATERIALIZED VIEW vn.mat AS SELECT id, email, note FROM vn.accounts;
`
	f := t.TempDir() + "/vn.sql"
	if err := os.WriteFile(f, []byte(sql), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := srv.Conn().ExecFile(f); err != nil {
		t.Fatal(err)
	}
	s, err := catalog.Load(srv.Conn(), []string{"vn"})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, tb := range s.Tables {
		if tb.Kind != "v" && tb.Kind != "m" {
			continue
		}
		var nn []string
		for _, c := range tb.Columns {
			if c.NotNull {
				nn = append(nn, c.Name)
			}
		}
		got[tb.Name] = strings.Join(nn, ",")
	}
	want := map[string]string{
		"plain":      "id,email", // note is nullable in the table
		"inner_join": "id,qty",   // an inner join keeps both sides
		"left_join":  "id",       // qty is on the nullable side
		"right_join": "qty",      // id is on the nullable side
		"expr":       "id,one,n", // arithmetic, functions and CASE prove nothing
		"uni":        "",         // set operations prove nothing
		"agg":        "email,n",  // count never returns NULL, max does
		"dom":        "id,b",     // the NOT NULL domain cast is enforced by PostgreSQL
		"over":       "id,email", // a view of a view
		"overleft":   "id",       // the nullable side stays nullable through the second view
		"mat":        "id,email", // materialized views too
	}
	for name, w := range want {
		if got[name] != w {
			t.Errorf("view %s: NOT NULL columns = %q, want %q", name, got[name], w)
		}
	}
}

// pg_relation_is_updatable and pg_column_is_updatable say which views accept
// INSERT / UPDATE and which of their columns; defaults and identity come from
// the base column when the view is a plain reference to it.
func TestUpdatableViewsFromTheServer(t *testing.T) {
	srv := server(t)
	sql := `
CREATE SCHEMA uv;
CREATE TABLE uv.accounts (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  email text NOT NULL,
  balance bigint NOT NULL DEFAULT 0,
  slug text NOT NULL,
  note text);
CREATE TABLE uv.items (id bigint PRIMARY KEY, account_id bigint REFERENCES uv.accounts(id), qty int NOT NULL);
CREATE VIEW uv.simple AS SELECT id, email, balance, slug, note FROM uv.accounts;
CREATE VIEW uv.calc AS SELECT id, email, balance + 1 AS plus FROM uv.accounts;
CREATE VIEW uv.joined AS SELECT a.id, i.qty FROM uv.accounts a JOIN uv.items i ON i.account_id = a.id;
CREATE VIEW uv.distinct_v AS SELECT DISTINCT email FROM uv.accounts;
CREATE VIEW uv.trig AS SELECT a.id, i.qty FROM uv.accounts a JOIN uv.items i ON i.account_id = a.id;
CREATE FUNCTION uv.noop() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NEW; END $$;
CREATE TRIGGER t_ins INSTEAD OF INSERT ON uv.trig FOR EACH ROW EXECUTE FUNCTION uv.noop();
CREATE VIEW uv.dflt AS SELECT id, email FROM uv.accounts;
ALTER VIEW uv.dflt ALTER COLUMN email SET DEFAULT 'x@example.com';
CREATE MATERIALIZED VIEW uv.mat AS SELECT id, email FROM uv.accounts;
`
	f := t.TempDir() + "/uv.sql"
	if err := os.WriteFile(f, []byte(sql), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := srv.Conn().ExecFile(f); err != nil {
		t.Fatal(err)
	}
	s, err := catalog.Load(srv.Conn(), []string{"uv"})
	if err != nil {
		t.Fatal(err)
	}
	tab := map[string]catalog.Table{}
	for _, tb := range s.Tables {
		tab[tb.Name] = tb
	}
	col := func(view, name string) catalog.Column {
		for _, c := range tab[view].Columns {
			if c.Name == name {
				return c
			}
		}
		t.Fatalf("no column %s.%s", view, name)
		return catalog.Column{}
	}

	for view, want := range map[string]int{"simple": 28, "calc": 28, "joined": 0, "distinct_v": 0, "trig": 8, "dflt": 28, "mat": 0} {
		if got := tab[view].Updatable; got != want {
			t.Errorf("%s: Updatable = %d, want %d", view, got, want)
		}
	}
	if !tab["simple"].CanInsert() || !tab["simple"].CanUpdate() || tab["simple"].ReadOnly() {
		t.Error("a simple view accepts INSERT and UPDATE")
	}
	if tab["joined"].CanInsert() || tab["joined"].CanUpdate() || !tab["joined"].ReadOnly() {
		t.Error("a join view is read-only")
	}
	if !tab["trig"].CanInsert() || tab["trig"].CanUpdate() {
		t.Error("an INSTEAD OF INSERT trigger makes the view insertable, not updatable")
	}
	if tab["mat"].CanInsert() || tab["mat"].CanUpdate() {
		t.Error("a materialized view is read-only")
	}

	if !col("simple", "email").Updatable || !col("simple", "note").Updatable {
		t.Error("plain references are writable")
	}
	if col("calc", "plus").Updatable {
		t.Error("a computed column is not writable")
	}
	// Proven NOT NULL, and the defaults of the base column.
	if e := col("simple", "email"); !e.NotNull || e.HasDefault {
		t.Errorf("email: NOT NULL without a default, got %+v", e)
	}
	if b := col("simple", "balance"); !b.NotNull || !b.HasDefault {
		t.Errorf("balance takes the DEFAULT of the table, got %+v", b)
	}
	if id := col("simple", "id"); !id.NotNull || id.Identity != "a" {
		t.Errorf("id is GENERATED ALWAYS in the table, got %+v", id)
	}
	if n := col("simple", "note"); n.NotNull {
		t.Error("note is nullable")
	}
	if e := col("dflt", "email"); !e.HasDefault {
		t.Error("a default set on the view itself counts")
	}
}

// proallargtypes, pronargdefaults and provariadic: what the generator needs to
// type SRFs with OUT columns, default arguments and VARIADIC.
func TestFunctionSignaturesFromTheServer(t *testing.T) {
	srv := server(t)
	sql := `
CREATE SCHEMA fs;
CREATE FUNCTION fs.join_all(VARIADIC parts text[]) RETURNS text LANGUAGE sql AS 'SELECT array_to_string(parts, '','')';
CREATE FUNCTION fs.greet(who text, greeting text DEFAULT 'hello', punct text DEFAULT '!') RETURNS text LANGUAGE sql AS 'SELECT greeting || '' '' || who || punct';
CREATE FUNCTION fs.pairs(n int, OUT a int, OUT b text) RETURNS SETOF record LANGUAGE sql AS 'SELECT i, i::text FROM generate_series(1, n) i';
CREATE FUNCTION fs.tbl(n int) RETURNS TABLE (x int, y int) LANGUAGE sql AS 'SELECT i, i * 2 FROM generate_series(1, n) i';
CREATE FUNCTION fs.series(n int) RETURNS SETOF int LANGUAGE sql AS 'SELECT generate_series(1, n)';
`
	f := t.TempDir() + "/fs.sql"
	if err := os.WriteFile(f, []byte(sql), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := srv.Conn().ExecFile(f); err != nil {
		t.Fatal(err)
	}
	s, err := catalog.Load(srv.Conn(), []string{"fs"})
	if err != nil {
		t.Fatal(err)
	}
	fn := map[string]catalog.Func{}
	for _, x := range s.Funcs {
		fn[x.Name] = x
	}
	if v := fn["join_all"]; v.VariadicType == 0 || len(v.ArgModes) != 1 || v.ArgModes[0] != "v" {
		t.Errorf("join_all: %+v", v)
	}
	if g := fn["greet"]; g.Defaults != 2 || len(g.ArgTypes) != 3 {
		t.Errorf("greet: %+v", g)
	}
	if p := fn["pairs"]; len(p.AllArgTypes) != 3 || !p.ReturnsSet || len(p.ArgTypes) != 1 || p.ArgModes[1] != "o" {
		t.Errorf("pairs: %+v", p)
	}
	if p := fn["tbl"]; len(p.AllArgTypes) != 3 || p.ArgModes[2] != "t" {
		t.Errorf("tbl: %+v", p)
	}
	if p := fn["series"]; !p.ReturnsSet || len(p.AllArgTypes) != 0 {
		t.Errorf("series: %+v", p)
	}
}
