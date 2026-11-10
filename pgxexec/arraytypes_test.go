package pgxexec_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/noobmaisfier/qlx/pgxexec"
	"github.com/noobmaisfier/qlx/qt"
)

// Arrays of types pgx has no codec for must scan and encode: the element's
// OID has to be registered (RegisterRangeTypes for the catalog, LoadTypes for
// what the database defines).
func TestArraysOfUncodedTypes(t *testing.T) {
	p := pool(t)
	ctx := context.Background()
	c, err := p.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Release()
	conn := c.Conn()
	for _, s := range []string{"CREATE EXTENSION IF NOT EXISTS hstore", "CREATE EXTENSION IF NOT EXISTS ltree"} {
		if _, err := conn.Exec(ctx, s); err != nil {
			t.Skipf("%s: %v", s, err)
		}
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	for _, s := range []string{
		"CREATE TYPE zmood AS ENUM ('sad', 'ok')",
		"CREATE DOMAIN zpos AS int4 CHECK (VALUE > 0)",
		"CREATE DOMAIN zpos2 AS zpos",
	} {
		if _, err := tx.Exec(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
	// A connection opened before the types existed learns them with LoadTypes.
	pgxexec.RegisterRangeTypes(conn.TypeMap())
	if err := pgxexec.LoadTypes(ctx, conn); err != nil {
		t.Fatal(err)
	}

	type Mood string
	d1, d2 := qt.DateOf(2024, 1, 1), qt.DateOf(2024, 2, 1)
	i1, i2 := int32(1), int32(5)

	check := func(name string, run func() (got, want any, err error)) {
		t.Helper()
		got, want, err := run()
		if err != nil {
			t.Errorf("%s: %v", name, err)
			return
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %#v, want %#v", name, got, want)
		}
	}
	// round trips a value through "SELECT $1::<sqltype>"
	rt := func(sqlType string, in any, out any) (any, any, error) {
		err := tx.QueryRow(ctx, "SELECT $1::"+sqlType, in).Scan(out)
		return reflect.ValueOf(out).Elem().Interface(), in, err
	}
	// reads a literal: the scan side alone
	lit := func(sql string, out any) error { return tx.QueryRow(ctx, sql).Scan(out) }

	check("money[]", func() (any, any, error) { var o []qt.Money; return rt("money[]", []qt.Money{"$1.00", "$2.50"}, &o) })
	check("timetz[]", func() (any, any, error) {
		var o []qt.TimeTZ
		in := []qt.TimeTZ{}
		var one qt.TimeTZ
		if err := lit("SELECT '10:00+01'::timetz", &one); err != nil {
			return nil, nil, err
		}
		in = append(in, one)
		return rt("timetz[]", in, &o)
	})
	check("macaddr8[]", func() (any, any, error) {
		var o []qt.MacAddr8
		return rt("macaddr8[]", []qt.MacAddr8{"08:00:2b:01:02:03:04:05"}, &o)
	})
	check("tsquery[]", func() (any, any, error) { var o []qt.TSQuery; return rt("tsquery[]", []qt.TSQuery{"'a' & 'b'"}, &o) })
	check("tsvector[]", func() (any, any, error) {
		var o []qt.TSVector
		return rt("tsvector[]", []qt.TSVector{"'a':1 'b':2"}, &o)
	})
	check("enum[]", func() (any, any, error) { var o []Mood; return rt("zmood[]", []Mood{"sad", "ok"}, &o) })
	check("domain[]", func() (any, any, error) { var o []int32; return rt("zpos[]", []int32{1, 2}, &o) })
	check("domain over domain[]", func() (any, any, error) { var o []int32; return rt("zpos2[]", []int32{3}, &o) })
	check("hstore[]", func() (any, any, error) {
		var o []qt.Hstore
		return rt("hstore[]", []qt.Hstore{{"a": qt.Some("1"), "b": qt.None[string]()}}, &o)
	})
	check("ltree[]", func() (any, any, error) { var o []qt.LTree; return rt("ltree[]", []qt.LTree{"a.b", "c"}, &o) })
	check("lquery[]", func() (any, any, error) { var o []qt.LQuery; return rt("lquery[]", []qt.LQuery{"a.*"}, &o) })
	check("ltxtquery[]", func() (any, any, error) { var o []qt.LTxtQuery; return rt("ltxtquery[]", []qt.LTxtQuery{"a & b"}, &o) })
	check("nullable elements", func() (any, any, error) {
		var o []qt.Null[qt.Money]
		return rt("money[]", []qt.Null[qt.Money]{qt.Some(qt.Money("$1.00")), qt.None[qt.Money]()}, &o)
	})

	// multirange arrays
	check("datemultirange[]", func() (any, any, error) {
		var o []qt.Multirange[qt.Date]
		return rt("datemultirange[]", []qt.Multirange[qt.Date]{{{Lower: &d1, Upper: &d2, LowerInc: true}}}, &o)
	})
	check("int4multirange[]", func() (any, any, error) {
		var o []qt.Multirange[int32]
		return rt("int4multirange[]", []qt.Multirange[int32]{{{Lower: &i1, Upper: &i2, LowerInc: true}}}, &o)
	})
	// scalar multirange of dates keeps its content
	check("datemultirange", func() (any, any, error) {
		var o qt.Multirange[qt.Date]
		return rt("datemultirange", qt.Multirange[qt.Date]{{Lower: &d1, Upper: &d2, LowerInc: true}}, &o)
	})

	// int2vector and oidvector are written as qt.Int2Vector / qt.OIDVector in
	// their text form (space separated); a Go slice would be the array form
	// {1,2,3}, which these types reject.
	check("int2vector", func() (any, any, error) { var o qt.Int2Vector; return rt("int2vector", qt.Int2Vector("1 2 3"), &o) })
	check("oidvector", func() (any, any, error) { var o qt.OIDVector; return rt("oidvector", qt.OIDVector("1 2 3"), &o) })
	check("int2vector[]", func() (any, any, error) {
		var o []qt.Int2Vector
		return rt("int2vector[]", []qt.Int2Vector{"1 2", "3"}, &o)
	})

}
