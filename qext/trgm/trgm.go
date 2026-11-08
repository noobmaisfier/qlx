// Package trgm holds helpers for the pg_trgm extension: fuzzy matching of
// text by trigrams, which is what makes typo-tolerant search and fast
// ILIKE '%word%' possible with a GIN or GiST index.
//
// The helpers use pg_trgm's operators (%, <%, <->), because only the operators
// can use a trigram index.
//
//	q.Select(Users.ID, trgm.Distance(Users.Name, "jon")).
//		From(Users).
//		Where(trgm.Similar(Users.Name, "jon")).
//		OrderBy(trgm.Distance(Users.Name, "jon").Asc())
//
// The match threshold is a session setting: pg_trgm.similarity_threshold for
// Similar (0.3 by default) and pg_trgm.word_similarity_threshold for
// WordSimilar (0.6). Set it with SetSimilarityThreshold.
package trgm

import (
	"strconv"

	"github.com/noobmaisfier/qlx"
	"github.com/noobmaisfier/qlx/pgcat/ext/pgtrgm"
	"github.com/noobmaisfier/qlx/qt"
)

// Similar is col % text: the two share more trigrams than the threshold.
func Similar(col qlx.Operand[string], text string) qlx.Pred {
	return qlx.BoolExpr{Expr: pgtrgm.OpPctTextText(col, qlx.Val(text))}.Pred()
}

// WordSimilar is text <% col: text is similar to a whole word, or a run of
// words, of col. Better than Similar for a short query against a long text.
func WordSimilar(col qlx.Operand[string], text string) qlx.Pred {
	return qlx.BoolExpr{Expr: pgtrgm.OpLtPctTextText(qlx.Val(text), col)}.Pred()
}

// SimilarIf is Similar for an optional filter: absent when text is unset or empty.
func SimilarIf(col qlx.Operand[string], text qt.Opt[string]) qlx.Pred {
	if !text.Set || text.V == "" {
		return qlx.Pred{}
	}
	return Similar(col, text.V)
}

// Distance is col <-> text, 1 minus the similarity: 0 identical, 1 nothing in
// common. Order by it ascending to get the closest first, with an index.
func Distance(col qlx.Operand[string], text string) qlx.Expr[float32] {
	return pgtrgm.OpLtMinusGtTextText(col, qlx.Val(text))
}

// Similarity is the similarity score between 0 and 1 (similarity()); for the
// SELECT list. It does not use an index: filter with Similar.
func Similarity(col qlx.Operand[string], text string) qlx.Expr[float32] {
	return pgtrgm.Similarity(col, qlx.Val(text))
}

// SetSimilarityThreshold sets pg_trgm.similarity_threshold for the current
// transaction (the threshold of Similar).
func SetSimilarityThreshold(v float64) qlx.Command {
	return qlx.SetConfig("pg_trgm.similarity_threshold", strconv.FormatFloat(v, 'f', -1, 64), true)
}

// SetWordSimilarityThreshold sets pg_trgm.word_similarity_threshold for the
// current transaction (the threshold of WordSimilar).
func SetWordSimilarityThreshold(v float64) qlx.Command {
	return qlx.SetConfig("pg_trgm.word_similarity_threshold", strconv.FormatFloat(v, 'f', -1, 64), true)
}
