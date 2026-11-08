// Package textsearch holds helpers for PostgreSQL full-text search: building a
// tsquery from what a user typed, building a weighted tsvector from columns,
// matching, ranking and highlighting.
//
// Every helper expands to the generated wrappers of pgcat/fts, so the version
// ranges of the functions still apply. User text is always a bound parameter.
//
// For a table that is searched often, store the document in a column
// (`GENERATED ALWAYS AS (...) STORED`) and index it with GIN; search it with
// Search or Match. Document builds the same expression for the generated
// column's definition, or for a search on a small table; an expression that is
// built per query cannot use a GIN index unless it is the indexed expression.
//
//	q.Select(Posts.ID, textsearch.Rank(Posts.Doc, query)).
//		From(Posts).
//		Where(textsearch.Search(Posts.Doc, "english", q.Val(userText))).
//		OrderBy(textsearch.Rank(Posts.Doc, query).Desc())
package textsearch

import (
	"strconv"
	"strings"

	"github.com/noobmaisfier/qlx"
	"github.com/noobmaisfier/qlx/pgcat/fts"
	"github.com/noobmaisfier/qlx/qt"
)

// Config is the name of a text search configuration ("english", "simple",
// "french"). The empty value means the server's default_text_search_config.
type Config = qt.RegConfig

func cfgVal(c Config) qlx.Operand[qt.RegConfig] { return qlx.Val(c) }

// Web parses text the way a web search box does (websearch_to_tsquery):
// words are ANDed, "quoted phrases" are phrases, `or` is OR, `-word` excludes.
// It never fails on malformed input, which makes it the right choice for text
// a user typed.
func Web(c Config, text qlx.Operand[string]) qlx.Expr[qt.TSQuery] {
	if c == "" {
		return fts.WebsearchToTsqueryText(text)
	}
	return fts.WebsearchToTsqueryRegconfigText(cfgVal(c), text)
}

// Plain ANDs every word of text (plainto_tsquery); operators in the text are
// ignored. It never fails on malformed input.
func Plain(c Config, text qlx.Operand[string]) qlx.Expr[qt.TSQuery] {
	if c == "" {
		return fts.PlaintoTsqueryText(text)
	}
	return fts.PlaintoTsqueryRegconfigText(cfgVal(c), text)
}

// Phrase matches the words of text next to each other, in order
// (phraseto_tsquery).
func Phrase(c Config, text qlx.Operand[string]) qlx.Expr[qt.TSQuery] {
	if c == "" {
		return fts.PhrasetoTsqueryText(text)
	}
	return fts.PhrasetoTsqueryRegconfigText(cfgVal(c), text)
}

// Syntax parses text with the full tsquery syntax (to_tsquery): `&`, `|`, `!`,
// `<->`, `:*` prefixes. PostgreSQL raises a syntax error on malformed input,
// so do not use it directly on text a user typed; use Web.
func Syntax(c Config, text qlx.Operand[string]) qlx.Expr[qt.TSQuery] {
	if c == "" {
		return fts.ToTsqueryText(text)
	}
	return fts.ToTsqueryRegconfigText(cfgVal(c), text)
}

// Weight ranks a part of a document: A counts most, D least. The zero value
// is "no weight" (PostgreSQL's default, D).
type Weight struct{ c qt.Char }

// The four weights.
var (
	A = Weight{'A'}
	B = Weight{'B'}
	C = Weight{'C'}
	D = Weight{'D'}
)

// Part is one text of a document with its weight.
type Part struct {
	text qlx.Operand[string]
	w    Weight
}

// Weighted makes a Part from a text column or expression, nullable or not. A
// NULL counts as the empty text.
func Weighted[T string | qt.Null[string]](w Weight, o qlx.Operand[T]) Part {
	switch v := any(o).(type) {
	case qlx.Operand[string]:
		return Part{v, w}
	case qlx.Operand[qt.Null[string]]:
		return Part{qlx.Coalesce(v, ""), w}
	}
	panic("textsearch: unreachable operand type")
}

// Document builds a tsvector from weighted parts:
//
//	Document("english", Weighted(A, Posts.Title), Weighted(B, Posts.Body))
//
// is setweight(to_tsvector(cfg, title), 'A') || setweight(to_tsvector(cfg, body), 'B').
func Document(c Config, parts ...Part) qlx.Expr[qt.TSVector] {
	vec := func(p Part) qlx.Expr[qt.TSVector] {
		var e qlx.Expr[qt.TSVector]
		if c == "" {
			e = fts.ToTsvectorText(p.text)
		} else {
			e = fts.ToTsvectorRegconfigText(cfgVal(c), p.text)
		}
		if p.w.c != 0 {
			e = fts.SetweightTsvectorChar(e, qlx.Val(p.w.c))
		}
		return e
	}
	if len(parts) == 0 {
		return vec(Part{text: qlx.Val("")})
	}
	doc := vec(parts[0])
	for _, p := range parts[1:] {
		doc = fts.OpPipePipeTsvectorTsvector(doc, vec(p))
	}
	return doc
}

// Match is doc @@ query.
func Match(doc qlx.Operand[qt.TSVector], query qlx.Operand[qt.TSQuery]) qlx.Pred {
	return qlx.BoolExpr{Expr: fts.OpAtAtTsvectorTsquery(doc, query)}.Pred()
}

// Search matches doc against what a user typed, parsed with Web.
func Search(doc qlx.Operand[qt.TSVector], c Config, text qlx.Operand[string]) qlx.Pred {
	return Match(doc, Web(c, text))
}

// SearchIf is Search for an optional filter: when text is unset or blank the
// predicate is absent, and Where skips it.
func SearchIf(doc qlx.Operand[qt.TSVector], c Config, text qt.Opt[string]) qlx.Pred {
	if !text.Set || strings.TrimSpace(text.V) == "" {
		return qlx.Pred{}
	}
	return Search(doc, c, qlx.Val(text.V))
}

// Rank is ts_rank: how often and how heavily the query terms occur.
func Rank(doc qlx.Operand[qt.TSVector], query qlx.Operand[qt.TSQuery]) qlx.Expr[float32] {
	return fts.TsRankTsvectorTsquery(doc, query)
}

// RankCover is ts_rank_cd: like Rank, and it also counts how close together
// the terms are. Slower, better for phrases.
func RankCover(doc qlx.Operand[qt.TSVector], query qlx.Operand[qt.TSQuery]) qlx.Expr[float32] {
	return fts.TsRankCdTsvectorTsquery(doc, query)
}

// Highlight describes how Headline marks the matches. The zero value is
// PostgreSQL's default (<b>...</b>, 35 words at most).
type Highlight struct {
	StartSel, StopSel       string // markers around a match
	MaxWords, MinWords      int    // length of the excerpt
	MaxFragments            int    // more than 0 returns several fragments
	FragmentDelimiter       string
	HighlightAllTheDocument bool
}

// options renders the ts_headline option string. The result is bound as a
// parameter, never written into the SQL; values are quoted for PostgreSQL's
// option parser.
// defaultMinWords is PostgreSQL's default for ts_headline MinWords.
const defaultMinWords = 15

func (h Highlight) options() string {
	var parts []string
	str := func(k, v string) {
		if v != "" {
			parts = append(parts, k+`="`+strings.ReplaceAll(v, `"`, `""`)+`"`)
		}
	}
	num := func(k string, v int) {
		if v > 0 {
			parts = append(parts, k+"="+strconv.Itoa(v))
		}
	}
	str("StartSel", h.StartSel)
	str("StopSel", h.StopSel)
	num("MaxWords", h.MaxWords)
	min := h.MinWords
	if min == 0 && h.MaxWords > 1 && h.MaxWords <= defaultMinWords {
		// The server insists MinWords < MaxWords and defaults MinWords to 15,
		// so a small MaxWords alone would be rejected. Lower the floor with it.
		min = h.MaxWords - 1
	}
	num("MinWords", min)
	num("MaxFragments", h.MaxFragments)
	str("FragmentDelimiter", h.FragmentDelimiter)
	if h.HighlightAllTheDocument {
		parts = append(parts, "HighlightAll=true")
	}
	return strings.Join(parts, ", ")
}

// Headline is ts_headline: an excerpt of text with the matches marked. Give it
// the original text, not the tsvector.
func Headline(c Config, text qlx.Operand[string], query qlx.Operand[qt.TSQuery], h Highlight) qlx.Expr[string] {
	opts := h.options()
	switch {
	case c == "" && opts == "":
		return fts.TsHeadlineTextTsquery(text, query)
	case c == "":
		return fts.TsHeadlineTextTsqueryText(text, query, qlx.Val(opts))
	case opts == "":
		return fts.TsHeadlineRegconfigTextTsquery(cfgVal(c), text, query)
	}
	return fts.TsHeadlineRegconfigTextTsqueryText(cfgVal(c), text, query, qlx.Val(opts))
}
