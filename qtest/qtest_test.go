package qtest_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
	"github.com/noobmaisfier/qlx/qtest"
)

var _ = q.Register("test.users_by_id", q.Select(Users.ID, Users.Email).From(Users).Where(Users.ID.Eq(1)))
var _ = q.Register("test.orders", q.Select(Orders.ID).From(Orders).OrderBy(Orders.ID.Desc()))

func TestSnapshotRoundTripAndDrift(t *testing.T) {
	path := filepath.Join(t.TempDir(), "q.golden")
	d := q.Dialect{Version: q.PG19}
	qtest.Snapshot(t, d, path, true)
	qtest.Snapshot(t, d, path, false) // must pass

	// A new query makes the snapshot stale.
	q.Register("test.extra", q.Select(Users.ID).From(Users))
	ft := &fakeT{}
	qtest.Snapshot(ft, d, path, false)
	if !ft.failed {
		t.Fatal("added query was not detected")
	}
}

func TestVerifyPrepareReportsRejections(t *testing.T) {
	ft := &fakeT{}
	qtest.VerifyPrepare(ft, q.Dialect{Version: q.PG19}, func(_ context.Context, name, sql string) error {
		if strings.Contains(sql, `"orders"`) {
			return errors.New("boom")
		}
		return nil
	})
	if !ft.failed || !strings.Contains(ft.msg, "test.orders") {
		t.Fatalf("rejection not reported: %q", ft.msg)
	}
}

func TestDuplicateRegistrationPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("duplicate name must panic")
		}
	}()
	q.Register("test.orders", q.Select(Orders.ID).From(Orders))
}

type fakeT struct {
	testing.TB
	failed bool
	msg    string
}

func (f *fakeT) Helper()                   {}
func (f *fakeT) Errorf(s string, a ...any) { f.failed = true; f.msg += fmt.Sprintf(s, a...) }
func (f *fakeT) Fatalf(s string, a ...any) { f.failed = true; f.msg += fmt.Sprintf(s, a...) }
