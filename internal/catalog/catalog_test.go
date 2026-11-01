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
