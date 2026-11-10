package codegen

import (
	"fmt"
	"strings"
	"unicode"
)

var initialisms = map[string]string{
	"id": "ID", "url": "URL", "uuid": "UUID", "api": "API", "ip": "IP", "http": "HTTP",
	"https": "HTTPS", "json": "JSON", "jsonb": "JSONB", "html": "HTML", "sql": "SQL",
	"xml": "XML", "uri": "URI", "ttl": "TTL", "cpu": "CPU", "os": "OS", "db": "DB",
	"tls": "TLS", "ssl": "SSL", "ssh": "SSH", "csv": "CSV", "pdf": "PDF", "ui": "UI",
}

var goKeywords = map[string]bool{
	"break": true, "default": true, "func": true, "interface": true, "select": true,
	"case": true, "defer": true, "go": true, "map": true, "struct": true, "chan": true,
	"else": true, "goto": true, "package": true, "switch": true, "const": true,
	"fallthrough": true, "if": true, "range": true, "type": true, "continue": true,
	"for": true, "import": true, "return": true, "var": true,
}

// shadowed are identifiers a parameter must not take because the generated
// file uses them as package names.
var shadowed = map[string]bool{
	"qt": true, "qlx": true, "time": true, "json": true, "netip": true, "net": true,
	"fmt": true, "uuid": true, "errors": true,
	// predeclared identifiers: a parameter of that name would capture the
	// type of the next parameter.
	"int": true, "int8": true, "int16": true, "int32": true, "int64": true,
	"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true, "uintptr": true,
	"string": true, "bool": true, "byte": true, "rune": true, "float32": true, "float64": true,
	"any": true, "error": true, "complex64": true, "complex128": true,
	"nil": true, "true": true, "false": true, "len": true, "cap": true, "new": true, "make": true,
}

func words(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

// camel converts snake_case to an exported Go name.
func camel(s string) string {
	var b strings.Builder
	for _, w := range words(s) {
		lw := strings.ToLower(w)
		if up, ok := initialisms[lw]; ok {
			b.WriteString(up)
			continue
		}
		r := []rune(lw)
		r[0] = unicode.ToUpper(r[0])
		b.WriteString(string(r))
	}
	out := b.String()
	if out == "" {
		return "X"
	}
	// An exported Go name starts with an upper-case letter: digits and
	// scripts without case ("日本語") get a prefix.
	if !unicode.IsUpper([]rune(out)[0]) {
		out = "X" + out
	}
	return out
}

// lowerCamel converts snake_case to a parameter name.
func lowerCamel(s string) string {
	c := camel(s)
	r := []rune(c)
	i := 0
	for i < len(r) && unicode.IsUpper(r[i]) {
		i++
	}
	switch {
	case i == 0:
	case i == len(r) || i == 1:
		for j := 0; j < i; j++ {
			r[j] = unicode.ToLower(r[j])
		}
	default: // leading initialism such as URLPath -> urlPath
		for j := 0; j < i-1; j++ {
			r[j] = unicode.ToLower(r[j])
		}
	}
	out := string(r)
	if goKeywords[out] || shadowed[out] {
		out += "_"
	}
	return out
}

// irregularSingular holds plurals that the suffix rules below get wrong, keyed
// by the last word of the name in lower case. A name is matched on its last
// word only, so order_statuses becomes OrderStatus but abuses is left to the
// suffix rules (it is not a "uses" word).
var irregularSingular = map[string]string{
	"statuses": "status", "buses": "bus", "viruses": "virus", "bonuses": "bonus", "campuses": "campus",
	"censuses": "census", "choruses": "chorus", "circuses": "circus", "cactuses": "cactus",
	"apparatuses": "apparatus", "surpluses": "surplus", "prospectuses": "prospectus", "syllabuses": "syllabus",
	"aliases": "alias", "biases": "bias", "canvases": "canvas", "gases": "gas", "lenses": "lens",
	"analyses": "analysis", "diagnoses": "diagnosis", "theses": "thesis", "hypotheses": "hypothesis",
	"crises": "crisis", "parentheses": "parenthesis", "synopses": "synopsis", "bases": "base",
	"indices": "index", "vertices": "vertex", "matrices": "matrix", "appendices": "appendix",
	"quizzes": "quiz", "heroes": "hero", "potatoes": "potato", "tomatoes": "tomato", "echoes": "echo",
	"people": "person", "children": "child", "men": "man", "women": "woman",
	"mice": "mouse", "geese": "goose", "feet": "foot", "teeth": "tooth", "criteria": "criterion",
	"phenomena": "phenomenon", "media": "medium", "memoranda": "memorandum",
	// "ies" and "es" plurals whose singular is not what the suffix rules give.
	"movies": "movie", "cookies": "cookie", "zombies": "zombie", "selfies": "selfie", "brownies": "brownie",
	"rookies": "rookie", "ties": "tie", "pies": "pie", "dies": "die", "calories": "calorie",
	"caches": "cache", "niches": "niche", "headaches": "headache", "avalanches": "avalanche",
	"shelves": "shelf", "knives": "knife", "wolves": "wolf", "halves": "half",
	// plurals in "us" and "is" that are not Latin singulars.
	"menus": "menu", "gurus": "guru", "emus": "emu", "emojis": "emoji", "taxis": "taxi", "wikis": "wiki", "kiwis": "kiwi",
}

// alreadySingular are names that end in s but are singular: a table is often
// named in the singular, and the "s" rule would cut the name.
var alreadySingular = map[string]bool{
	"alias": true, "canvas": true, "atlas": true, "bias": true, "gas": true, "chaos": true,
	"kudos": true, "pancreas": true, "texas": true, "christmas": true, "lens": true,
	"axis": true, "basis": true, "analysis": true,
}

// uncountable names have no plural form: the row type is the name itself.
var uncountable = map[string]bool{
	"news": true, "series": true, "species": true, "sheep": true, "fish": true, "data": true,
	"metadata": true, "staff": true, "equipment": true, "information": true, "software": true,
	"hardware": true, "money": true, "rice": true, "aircraft": true, "lens": true,
}

// lastWord splits a camel-case name at its last capital letter.
func lastWord(s string) (head, last string) {
	i := strings.LastIndexFunc(s, unicode.IsUpper)
	if i < 0 {
		return "", s
	}
	return s[:i], s[i:]
}

// singular turns a plural table name into a row type name.
func singular(s string) string {
	head, last := lastWord(s)
	l := strings.ToLower(last)
	if uncountable[l] || alreadySingular[l] {
		return s
	}
	if w, ok := irregularSingular[l]; ok {
		r := []rune(w)
		r[0] = unicode.ToUpper(r[0])
		if unicode.IsLower([]rune(last)[0]) { // an all-lower name stays lower
			r[0] = unicode.ToLower(r[0])
		}
		return head + string(r)
	}
	l = strings.ToLower(s)
	switch {
	case strings.HasSuffix(l, "ies") && len(l) > 3:
		return s[:len(s)-3] + "y"
	case strings.HasSuffix(l, "sses"), strings.HasSuffix(l, "xes"), strings.HasSuffix(l, "ches"), strings.HasSuffix(l, "shes"):
		return s[:len(s)-2]
	case strings.HasSuffix(l, "ss"), strings.HasSuffix(l, "us"), strings.HasSuffix(l, "is"):
		return s
	case strings.HasSuffix(l, "s") && len(l) > 1:
		return s[:len(s)-1]
	}
	return s
}

// namer hands out unique top-level identifiers.
//
// With more than one schema, the first one in the config is the principal
// schema: its objects keep their plain names (Users) and the objects of every
// other schema are always prefixed with the schema (BillingUsers), whether or
// not a name would collide. A name therefore depends only on the object and
// on the config, never on what else exists in the database: adding a schema,
// or a table to another schema, cannot make Users mean another table.
type namer struct {
	used      map[string]bool
	principal string
	multi     bool
	tableSym  map[string]string // table symbol -> the schema.table that owns it
	err       error
}

func newNamer(cfg Config) *namer {
	n := &namer{used: map[string]bool{}, tableSym: map[string]string{}}
	if len(cfg.Schemas) > 1 {
		n.multi, n.principal = true, cfg.Schemas[0]
	}
	return n
}

// prefix is what an object of this schema is prefixed with: nothing for the
// principal schema, and for the only schema of a single-schema config.
func (n *namer) prefix(schema string) string {
	if n.multi && schema != n.principal {
		return camel(schema)
	}
	return ""
}

// local is the Go name of a database object called name in schema.
func (n *namer) local(schema, name string) string { return n.prefix(schema) + camel(name) }

// localSingular is local, singular: the name of one row.
func (n *namer) localSingular(schema, name string) string {
	return n.prefix(schema) + singular(camel(name))
}

// claimTable records that the table symbol sym belongs to schema.table. Two
// tables of different schemas that end up with the same name are an error: the
// generator does not pick a winner, because the winner would depend on order.
func (n *namer) claimTable(sym, schema, table string) {
	owner := schema + "." + table
	prev, ok := n.tableSym[sym]
	if !ok {
		n.tableSym[sym] = owner
		return
	}
	if n.err == nil && strings.SplitN(prev, ".", 2)[0] != schema {
		n.err = fmt.Errorf("tables %s and %s would both be called %s in Go: rename one, or leave one out with exclude", prev, owner, sym)
	}
}

func (n *namer) take(name string, fallbackSuffix string) string {
	cand := name
	if n.used[cand] && fallbackSuffix != "" {
		cand = name + fallbackSuffix
	}
	for i := 2; n.used[cand]; i++ {
		cand = name + fallbackSuffix + itoa(i)
	}
	n.used[cand] = true
	return cand
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var d []byte
	for i > 0 {
		d = append([]byte{byte('0' + i%10)}, d...)
		i /= 10
	}
	return string(d)
}
