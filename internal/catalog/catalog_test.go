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
