package qlx_test

import (
	"math/rand/v2"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"testing"

	q "github.com/noobmaisfier/qlx"
	. "github.com/noobmaisfier/qlx/fixture"
)

// A grammar-level differential fuzz: build random but type-correct queries
// through the public API, render them, and ask a real PostgreSQL to PREPARE
// every one. The builder's types rule out ill-typed queries, so any server
// rejection is a renderer bug (wrong syntax, missing parentheses, bad
// qualification) or a semantic rule the builder does not yet check (grouping).
//
//	QLX_FUZZ_N=5000 QLX_FUZZ_SEED=7 scripts/with_pg.sh go test -run Differential .
type gen struct{ r *rand.Rand }

func (g gen) pick(n int) int { return g.r.IntN(n) }

type colset struct {
	nums  []q.NumExpr[int64]
	texts []q.TextExpr[string]
}

func usersOrders() colset {
	return colset{
		nums:  []q.NumExpr[int64]{Users.ID.NumExpr, Users.Balance.NumExpr, Orders.ID.NumExpr, Orders.Total.NumExpr, Orders.UserID.NumExpr},
		texts: []q.TextExpr[string]{Users.Email.TextExpr, Users.Email.TextExpr},
	}
}

func (g gen) pred(c colset, depth int) q.Pred {
	if depth > 0 && g.pick(3) == 0 {
		a, b := g.pred(c, depth-1), g.pred(c, depth-1)
		switch g.pick(3) {
		case 0:
			return q.And(a, b)
		case 1:
			return q.Or(a, b)
		default:
			return q.Not(a)
		}
	}
	n := c.nums[g.pick(len(c.nums))]
	t := c.texts[g.pick(len(c.texts))]
	v := int64(g.pick(1000))
	switch g.pick(9) {
	case 0:
		return n.Gt(v)
	case 1:
		return n.Lte(v)
	case 2:
		return n.Between(v, v+10)
	case 3:
		return t.Like("a%")
	case 4:
		return t.ContainsFold("x_y%")
	case 5:
		return n.In(v, v+1, v+2)
	case 6:
		return t.StartsWith("p")
	case 7:
		return q.Exists(q.Select(Orders.ID).From(Orders).Where(Orders.UserID.EqX(Users.ID), Orders.Total.Gt(v)))
	default:
		return q.InSelect(Users.ID.Expr, q.Select(Orders.UserID).From(Orders).Where(Orders.Total.Gt(v)))
	}
}

func (g gen) query() q.Statement {
	c := usersOrders()
	base := func(items ...q.Selectable) q.SelectQuery {
		s := q.Select(items...).From(Users)
		if g.pick(2) == 0 {
			s = s.LeftJoin(Orders).On(Orders.UserID.EqX(Users.ID))
		}
		return s
	}
	var s q.SelectQuery
	orderable, twoCol := false, false
	switch g.pick(6) {
	case 0: // plain
		s = base(Users.ID, Users.Email)
		orderable, twoCol = true, true
	case 1: // grouped
		s = base(Users.ID, q.Count(Users.ID).As("n")).GroupBy(Users.ID)
		orderable = true
		if g.pick(2) == 0 {
			s = s.Having(q.Count(Users.ID).Gt(int64(g.pick(5))))
		}
	case 2: // distinct on
		s = base(Users.Email, Users.ID).DistinctOn(Users.Email).OrderBy(Users.Email.Asc(), Users.ID.Desc())
	case 3: // window
		s = q.Select(Users.ID, q.RowNumber().Over(q.Window().PartitionBy(Users.Tier).OrderBy(Users.ID.Asc())).As("rn")).From(Users)
		orderable = true
	case 4: // aggregate with filter
		s = q.Select(q.Count(Users.ID).Filter(Users.Balance.Gt(int64(g.pick(100)))).As("n")).From(Users)
	default: // subquery source
		sub := q.Subquery(q.Select(Users.ID.As("id")).From(Users).Where(g.pred(colset{nums: c.nums[:2], texts: c.texts[:2]}, 1)), "sq")
		idf := q.NewField[int64]("id")
		return q.Select(q.RefNum(sub, idf)).From(sub)
	}
	// Join-only columns are valid only when the join is present; filters that
	// mention Orders go through a self-contained EXISTS instead.
	s = s.Where(g.pred(colset{nums: c.nums[:2], texts: c.texts[:2]}, 2))
	if g.pick(3) == 0 && orderable {
		s = s.OrderBy(Users.ID.Desc())
	}
	if g.pick(3) == 0 {
		s = s.Limit(int64(1 + g.pick(50))).Offset(int64(g.pick(10)))
	}
	if twoCol && g.pick(3) == 0 {
		return s.UnionAll(q.Select(Users.ID, Users.Email).From(Users))
	}
	return s
}

func TestDifferentialAgainstServer(t *testing.T) {
	env, _ := pgEnv(t)
	n := 1000
	if v, err := strconv.Atoi(os.Getenv("QLX_FUZZ_N")); err == nil {
		n = v
	}
	seed := uint64(1)
	if v, err := strconv.ParseUint(os.Getenv("QLX_FUZZ_SEED"), 10, 64); err == nil {
		seed = v
	}
	g := gen{rand.New(rand.NewPCG(seed, 0xC0FFEE))}
	var script strings.Builder
	var sqls []string
	for i := 0; i < n; i++ {
		st, err := q.Render(q.Dialect{Version: q.PG19}, g.query())
		if err != nil {
			t.Fatalf("query %d does not render: %v", i, err)
		}
		sqls = append(sqls, st.SQL)
		script.WriteString("\\echo #" + strconv.Itoa(i) + "\nPREPARE s AS " + st.SQL + ";\nDEALLOCATE s;\n")
	}
	cmd := exec.Command("psql", "-X", "-q", "-v", "ON_ERROR_STOP=0")
	cmd.Env = env
	cmd.Stdin = strings.NewReader(script.String())
	out, _ := cmd.CombinedOutput()
	cur, bad := -1, 0
	for _, line := range strings.Split(string(out), "\n") {
		switch {
		case strings.HasPrefix(line, "#"):
			cur, _ = strconv.Atoi(line[1:])
		case strings.Contains(line, "ERROR:"):
			bad++
			if bad <= 10 && cur >= 0 {
				t.Errorf("seed %d query #%d rejected: %s\n  %s", seed, cur, strings.TrimSpace(line), sqls[cur])
			}
		}
	}
	t.Logf("%d queries, %d rejected (seed %d)", n, bad, seed)
}

// The shape cache must agree with a full render on every random query, on the
// miss and on the hit. No server needed.
func TestShapeCacheRandomQueries(t *testing.T) {
	n := 3000
	if v, err := strconv.Atoi(os.Getenv("QLX_FUZZ_N")); err == nil {
		n = v
	}
	g := gen{rand.New(rand.NewPCG(7, 0xC0FFEE))}
	c := q.NewShapeCache(512)
	d := q.Dialect{Version: q.PG19}
	for i := 0; i < n; i++ {
		stmt := g.query()
		wantSQL, wantArgs, err := q.SQL(d, stmt)
		if err != nil {
			t.Fatalf("query %d: %v", i, err)
		}
		for pass := 0; pass < 2; pass++ {
			sql, args, err := c.Resolve(d, stmt)
			if err != nil || sql != wantSQL || len(args) != len(wantArgs) {
				t.Fatalf("query %d pass %d:\n got  %s %v (%v)\n want %s %v", i, pass, sql, args, err, wantSQL, wantArgs)
			}
			if !reflect.DeepEqual(args, wantArgs) {
				t.Fatalf("query %d args: %v != %v", i, args, wantArgs)
			}
		}
	}
	t.Logf("%d random queries, %+v", n, c.Stats())
}
