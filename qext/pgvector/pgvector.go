// Package pgvector holds helpers for the pgvector extension: distances with
// one name for every vector type, and the session settings that tune an index
// search.
//
// The distances use pgvector's operators (<->, <=>, <#>, <+>) and not the
// functions (l2_distance...), because only the operators let PostgreSQL use an
// HNSW or IVFFlat index for ORDER BY ... LIMIT.
//
//	q.Select(Items.ID).
//		From(Items).
//		OrderBy(pgvector.Cosine(Items.Embedding, query).Asc()).
//		Limit(10)
//
// The query vector is sent as a parameter cast to the vector type, looked up on
// the search_path (the extension is normally installed in public).
package pgvector

import (
	"strconv"

	"github.com/noobmaisfier/qlx"
	"github.com/noobmaisfier/qlx/pgcat/ext/vector"
	"github.com/noobmaisfier/qlx/qt"
)

// Vec is the dense vector types: vector and halfvec.
type Vec interface{ qt.Vector | qt.HalfVector }

var (
	typVector  = qlx.NewPgType[qt.Vector]("", "vector")
	typHalfvec = qlx.NewPgType[qt.HalfVector]("", "halfvec")
)

// Distance is a distance between a column and a query vector. NULL when either
// is NULL.
type Distance = qlx.Expr[qt.Null[float64]]

// pick runs the form of the operator that matches the column's type. The query
// is cast to the extension's type, since a bare parameter would be ambiguous
// between the overloads.
func pick[T Vec](col qlx.Operand[T], query T,
	fv func(a, b qlx.Operand[qt.Vector]) Distance,
	fh func(a, b qlx.Operand[qt.HalfVector]) Distance) Distance {
	switch c := any(col).(type) {
	case qlx.Operand[qt.Vector]:
		return fv(c, qlx.Cast(qlx.Val(any(query).(qt.Vector)), typVector))
	case qlx.Operand[qt.HalfVector]:
		return fh(c, qlx.Cast(qlx.Val(any(query).(qt.HalfVector)), typHalfvec))
	}
	panic("pgvector: unreachable column type")
}

// L2 is the Euclidean distance, col <-> query. Smaller is closer.
func L2[T Vec](col qlx.Operand[T], query T) Distance {
	return pick(col, query,
		func(a, b qlx.Operand[qt.Vector]) Distance { return vector.OpLtMinusGtVectorVector(a, b) },
		func(a, b qlx.Operand[qt.HalfVector]) Distance { return vector.OpLtMinusGtHalfvecHalfvec(a, b) })
}

// Cosine is the cosine distance, col <=> query (0 identical, 2 opposite).
func Cosine[T Vec](col qlx.Operand[T], query T) Distance {
	return pick(col, query,
		func(a, b qlx.Operand[qt.Vector]) Distance { return vector.OpLtEqGtVectorVector(a, b) },
		func(a, b qlx.Operand[qt.HalfVector]) Distance { return vector.OpLtEqGtHalfvecHalfvec(a, b) })
}

// NegativeInnerProduct is col <#> query: the inner product times -1, so that
// "smaller is closer" holds as for the other distances. Multiply by -1 to get
// the inner product.
func NegativeInnerProduct[T Vec](col qlx.Operand[T], query T) Distance {
	return pick(col, query,
		func(a, b qlx.Operand[qt.Vector]) Distance { return qlx.Nullable(vector.OpLtHashGtVectorVector(a, b)) },
		func(a, b qlx.Operand[qt.HalfVector]) Distance {
			return qlx.Nullable(vector.OpLtHashGtHalfvecHalfvec(a, b))
		})
}

// L1 is the taxicab distance, col <+> query.
func L1[T Vec](col qlx.Operand[T], query T) Distance {
	return pick(col, query,
		func(a, b qlx.Operand[qt.Vector]) Distance { return vector.OpLtPlusGtVectorVector(a, b) },
		func(a, b qlx.Operand[qt.HalfVector]) Distance { return vector.OpLtPlusGtHalfvecHalfvec(a, b) })
}

// SetEfSearch sets hnsw.ef_search for the current transaction: how many
// candidates an HNSW index search keeps. Higher is more accurate and slower
// (pgvector's default is 40). Run it first in the transaction that searches.
func SetEfSearch(n int) qlx.Command { return qlx.SetConfig("hnsw.ef_search", strconv.Itoa(n), true) }

// SetProbes sets ivfflat.probes for the current transaction: how many lists an
// IVFFlat index search visits (pgvector's default is 1).
func SetProbes(n int) qlx.Command { return qlx.SetConfig("ivfflat.probes", strconv.Itoa(n), true) }
