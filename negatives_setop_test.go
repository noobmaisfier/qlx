package qlx_test

import (
	"errors"
	"math"
	"strings"
	"testing"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
)

// A set operation takes Limit and Offset too; a negative one is refused at
// render time like it is for a plain select, instead of failing on the server.
func TestNegativeLimitOffsetOnSetQueryAreRefused(t *testing.T) {
	base := func() q.SetQuery {
		return q.Union(q.Select(Users.ID).From(Users), q.Select(Orders.ID).From(Orders))
	}
	bad := map[string]q.SetQuery{"Limit": base().Limit(-1), "Offset": base().Offset(-5)}
	for name, st := range bad {
		if _, err := q.Render(q.Dialect{Version: q.PG18}, st); !errors.Is(err, q.ErrShape) || !strings.Contains(err.Error(), name) {
			t.Errorf("%s: err=%v", name, err)
		}
	}
	if _, err := q.Render(q.Dialect{Version: q.PG18}, base().Limit(0).Offset(0)); err != nil {
		t.Errorf("good control refused: %v", err)
	}
}

func TestFrameOffsetAndSampleArgumentsAreChecked(t *testing.T) {
	d := q.Dialect{Version: q.PG18}
	win := q.Window().PartitionBy(Orders.UserID).OrderBy(Orders.ID.Asc())
	for name, fb := range map[string]q.FrameBound{"preceding": q.Preceding(-1), "following": q.Following(-3)} {
		st := q.Select(q.SumBig(Orders.Total).Over(win.Rows(fb, q.CurrentRow)).As("run")).From(Orders)
		if _, err := q.Render(d, st); !errors.Is(err, q.ErrShape) {
			t.Errorf("%s: a negative frame offset gave err=%v", name, err)
		}
	}
	if _, err := q.Render(d, q.Select(q.SumBig(Orders.Total).Over(win.Rows(q.Preceding(0), q.Following(2))).As("run")).From(Orders)); err != nil {
		t.Errorf("good control refused: %v", err)
	}
	nan := math.NaN()
	for _, pct := range []float64{nan, -1, 101} {
		st := q.Select(Users.ID).From(Users.TableSample(q.Bernoulli(pct)))
		if _, err := q.Render(d, st); !errors.Is(err, q.ErrShape) {
			t.Errorf("sample %v: err=%v", pct, err)
		}
	}
}
