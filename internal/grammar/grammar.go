// Package grammar reads PostgreSQL's gram.y and turns its productions into a
// ledger, the parser-side counterpart of the pgcat catalog ledger. Every
// production of the nonterminals QLX tracks must be mapped to a QLX
// symbol or to an explicit "unsupported" with a reason; Check fails on any
// production the mapping does not know, so a PostgreSQL release that adds
// syntax breaks CI instead of silently widening the gap.
package grammar

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// Production is one alternative of a nonterminal.
type Production struct {
	NT  string `json:"nt"`
	Alt string `json:"alt"` // space-joined symbols; "" is the empty production
}

// Key is the stable identity of a production in the mapping file.
func (p Production) Key() string {
	if p.Alt == "" {
		return p.NT + ": <empty>"
	}
	return p.NT + ": " + p.Alt
}

// Grammar is the parsed rule section.
type Grammar struct {
	Rules map[string][]Production
	Order []string
}

// Parse extracts the rules between the first two %% markers.
func Parse(src string) (*Grammar, error) {
	first := strings.Index(src, "\n%%")
	if first < 0 {
		return nil, fmt.Errorf("grammar: no %%%% marker")
	}
	body := src[first+3:]
	if i := strings.Index(body, "\n%%"); i >= 0 {
		body = body[:i]
	}
	toks := tokenize(body)
	g := &Grammar{Rules: map[string][]Production{}}
	i := 0
	for i < len(toks) {
		// name ':'
		if i+1 >= len(toks) || toks[i+1] != ":" {
			i++
			continue
		}
		name := toks[i]
		i += 2
		var cur []string
		flush := func() {
			g.Rules[name] = append(g.Rules[name], Production{NT: name, Alt: strings.Join(cur, " ")})
			cur = nil
		}
		for i < len(toks) && toks[i] != ";" {
			if toks[i] == "|" {
				flush()
			} else {
				cur = append(cur, toks[i])
			}
			i++
		}
		flush()
		i++
		g.Order = append(g.Order, name)
	}
	if len(g.Order) == 0 {
		return nil, fmt.Errorf("grammar: no rules found")
	}
	return g, nil
}

// tokenize drops comments, actions ({...}), %prec clauses and keeps
// identifiers, ':', '|', ';' and quoted character tokens.
func tokenize(s string) []string {
	var out []string
	n := len(s)
	for i := 0; i < n; {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '/' && i+1 < n && s[i+1] == '*':
			j := strings.Index(s[i+2:], "*/")
			if j < 0 {
				return out
			}
			i += j + 4
		case c == '/' && i+1 < n && s[i+1] == '/':
			for i < n && s[i] != '\n' {
				i++
			}
		case c == '{':
			depth := 0
			for i < n {
				switch s[i] {
				case '{':
					depth++
				case '}':
					depth--
				case '"', '\'':
					q := s[i]
					i++
					for i < n && s[i] != q {
						if s[i] == '\\' {
							i++
						}
						i++
					}
				case '/':
					if i+1 < n && s[i+1] == '*' {
						j := strings.Index(s[i+2:], "*/")
						if j < 0 {
							return out
						}
						i += j + 3
					}
				}
				i++
				if depth == 0 {
					break
				}
			}
		case c == '\'':
			j := i + 1
			for j < n && s[j] != '\'' {
				if s[j] == '\\' {
					j++
				}
				j++
			}
			out = append(out, s[i:j+1])
			i = j + 1
		case c == ':' || c == '|' || c == ';':
			out = append(out, string(c))
			i++
		case c == '%':
			// %prec NAME
			j := i + 1
			for j < n && isIdent(s[j]) {
				j++
			}
			word := s[i:j]
			i = j
			if word == "%prec" {
				for i < n && (s[i] == ' ' || s[i] == '\t') {
					i++
				}
				for i < n && isIdent(s[i]) {
					i++
				}
			}
		default:
			j := i
			for j < n && isIdent(s[j]) {
				j++
			}
			if j == i {
				i++
				continue
			}
			out = append(out, s[i:j])
			i = j
		}
	}
	return out
}

func isIdent(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// Ledger is the file written by `pgcat grammar`.
type Ledger struct {
	Source      string       `json:"source"`
	Tracked     []string     `json:"tracked"`
	Statements  []string     `json:"statements"` // alternatives of `stmt`
	Productions []Production `json:"productions"`
}

// Build lists the statements and every production of the tracked nonterminals.
func Build(g *Grammar, source string, tracked []string) (*Ledger, error) {
	l := &Ledger{Source: source, Tracked: tracked}
	stmt, ok := g.Rules["stmt"]
	if !ok {
		return nil, fmt.Errorf("grammar: no stmt rule")
	}
	for _, p := range stmt {
		if p.Alt != "" {
			l.Statements = append(l.Statements, p.Alt)
		}
	}
	sort.Strings(l.Statements)
	for _, nt := range tracked {
		ps, ok := g.Rules[nt]
		if !ok {
			return nil, fmt.Errorf("grammar: tracked nonterminal %q does not exist in %s", nt, source)
		}
		l.Productions = append(l.Productions, ps...)
	}
	return l, nil
}

// Entry says what QLX does with a statement or production.
type Entry struct {
	Status string `json:"status"` // supported | partial | unsupported | out-of-scope | unmapped
	Symbol string `json:"symbol,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// Map is the hand-maintained mapping file.
type Map struct {
	Statements  map[string]Entry `json:"statements"`
	Productions map[string]Entry `json:"productions"`
}

// LoadMap reads a mapping file.
func LoadMap(path string) (*Map, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Map
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	if m.Statements == nil {
		m.Statements = map[string]Entry{}
	}
	if m.Productions == nil {
		m.Productions = map[string]Entry{}
	}
	return &m, nil
}

// Problem is one reason a check fails.
type Problem struct{ Kind, Key, Msg string }

func (p Problem) String() string { return p.Kind + ": " + p.Key + ": " + p.Msg }

// Check compares a ledger with the mapping: every statement and production
// must be mapped (no "unmapped"), supported ones need a symbol, the others a
// reason, and mapping entries for syntax that no longer exists are stale.
func Check(l *Ledger, m *Map) []Problem {
	var ps []Problem
	check := func(kind, key string, e Entry, ok bool) {
		switch {
		case !ok:
			ps = append(ps, Problem{kind, key, "not in the mapping file (new syntax: map it or mark it unsupported with a reason)"})
		case e.Status == "unmapped" || e.Status == "":
			ps = append(ps, Problem{kind, key, "still unmapped"})
		case (e.Status == "supported" || e.Status == "partial") && e.Symbol == "":
			ps = append(ps, Problem{kind, key, "status " + e.Status + " needs a symbol"})
		case (e.Status == "unsupported" || e.Status == "out-of-scope" || e.Status == "partial") && e.Reason == "":
			ps = append(ps, Problem{kind, key, "status " + e.Status + " needs a reason"})
		}
	}
	seenS := map[string]bool{}
	for _, s := range l.Statements {
		seenS[s] = true
		e, ok := m.Statements[s]
		check("statement", s, e, ok)
	}
	seenP := map[string]bool{}
	for _, p := range l.Productions {
		seenP[p.Key()] = true
		e, ok := m.Productions[p.Key()]
		check("production", p.Key(), e, ok)
	}
	for k := range m.Statements {
		if !seenS[k] {
			ps = append(ps, Problem{"statement", k, "stale: no longer in the grammar"})
		}
	}
	for k := range m.Productions {
		if !seenP[k] {
			ps = append(ps, Problem{"production", k, "stale: no longer in the grammar"})
		}
	}
	sort.Slice(ps, func(i, j int) bool { return ps[i].String() < ps[j].String() })
	return ps
}

// Fill adds an "unmapped" entry for everything the mapping lacks, so a
// maintainer triages by editing statuses rather than typing keys.
func Fill(l *Ledger, m *Map) int {
	n := 0
	for _, s := range l.Statements {
		if _, ok := m.Statements[s]; !ok {
			m.Statements[s] = Entry{Status: "unmapped"}
			n++
		}
	}
	for _, p := range l.Productions {
		if _, ok := m.Productions[p.Key()]; !ok {
			m.Productions[p.Key()] = Entry{Status: "unmapped"}
			n++
		}
	}
	return n
}

// Coverage counts statuses.
func Coverage(m *Map) map[string]int {
	c := map[string]int{}
	for _, e := range m.Statements {
		c["statements/"+e.Status]++
	}
	for _, e := range m.Productions {
		c["productions/"+e.Status]++
	}
	return c
}

// Union merges ledgers of several PostgreSQL versions: a statement or
// production is present when any version has it, so one mapping file covers
// every pinned tag.
func Union(ls ...*Ledger) *Ledger {
	u := &Ledger{}
	seenS := map[string]bool{}
	seenP := map[string]bool{}
	seenT := map[string]bool{}
	var src []string
	for _, l := range ls {
		src = append(src, l.Source)
		for _, t := range l.Tracked {
			if !seenT[t] {
				seenT[t] = true
				u.Tracked = append(u.Tracked, t)
			}
		}
		for _, s := range l.Statements {
			if !seenS[s] {
				seenS[s] = true
				u.Statements = append(u.Statements, s)
			}
		}
		for _, p := range l.Productions {
			if !seenP[p.Key()] {
				seenP[p.Key()] = true
				u.Productions = append(u.Productions, p)
			}
		}
	}
	u.Source = strings.Join(src, " + ")
	sort.Strings(u.Statements)
	return u
}

// Validate checks a mapping file on its own, without a grammar: no entry may
// be unmapped, supported entries need a symbol, the rest a reason.
func Validate(m *Map) []Problem {
	var ps []Problem
	chk := func(kind, k string, e Entry) {
		switch {
		case e.Status == "" || e.Status == "unmapped":
			ps = append(ps, Problem{kind, k, "unmapped"})
		case (e.Status == "supported" || e.Status == "partial") && e.Symbol == "":
			ps = append(ps, Problem{kind, k, "needs a symbol"})
		case (e.Status == "unsupported" || e.Status == "out-of-scope" || e.Status == "partial") && e.Reason == "":
			ps = append(ps, Problem{kind, k, "needs a reason"})
		case e.Status != "supported" && e.Status != "partial" && e.Status != "unsupported" && e.Status != "out-of-scope":
			ps = append(ps, Problem{kind, k, "unknown status " + e.Status})
		}
	}
	for k, e := range m.Statements {
		chk("statement", k, e)
	}
	for k, e := range m.Productions {
		chk("production", k, e)
	}
	sort.Slice(ps, func(i, j int) bool { return ps[i].String() < ps[j].String() })
	return ps
}

// Cheatsheet renders the mapping as a SQL-to-Go table: for every syntax form
// the DSL entry point, grouped by statement, followed by what is not
// supported and why. It is generated, so it cannot drift from the ledger.
func Cheatsheet(m *Map) string {
	var b strings.Builder
	b.WriteString("# SQL to Go cheat sheet\n\nGenerated by `pgcat grammar -cheatsheet` from ledgers/grammar_map.json; do not edit.\n\n## Statements\n\n| SQL statement (gram.y) | QLX |\n|---|---|\n")
	var ss []string
	for s := range m.Statements {
		ss = append(ss, s)
	}
	sort.Strings(ss)
	for _, s := range ss {
		e := m.Statements[s]
		switch e.Status {
		case "supported", "partial":
			fmt.Fprintf(&b, "| %s | `%s` |\n", s, e.Symbol)
		}
	}
	byNT := map[string][]string{}
	var nts []string
	var keys []string
	for k := range m.Productions {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		e := m.Productions[k]
		if e.Status != "supported" && e.Status != "partial" {
			continue
		}
		nt := k[:strings.Index(k, ":")]
		if _, ok := byNT[nt]; !ok {
			nts = append(nts, nt)
		}
		alt := strings.TrimSpace(k[strings.Index(k, ":")+1:])
		sym := e.Symbol
		if e.Status == "partial" {
			sym += " (partial: " + e.Reason + ")"
		}
		byNT[nt] = append(byNT[nt], "| `"+strings.ReplaceAll(alt, "|", `\|`)+"` | "+strings.ReplaceAll(sym, "|", `\|`)+" |")
	}
	sort.Strings(nts)
	b.WriteString("\n## Clauses and expressions\n")
	for _, nt := range nts {
		fmt.Fprintf(&b, "\n### %s\n\n| Syntax | QLX |\n|---|---|\n%s\n", nt, strings.Join(byNT[nt], "\n"))
	}
	b.WriteString("\n## Not supported\n\n| Production | Why |\n|---|---|\n")
	for _, k := range keys {
		if e := m.Productions[k]; e.Status == "unsupported" {
			fmt.Fprintf(&b, "| `%s` | %s |\n", strings.ReplaceAll(k, "|", `\|`), e.Reason)
		}
	}
	b.WriteString("\n## Out of scope statements\n\n")
	n := 0
	for _, s := range ss {
		if m.Statements[s].Status == "out-of-scope" {
			n++
		}
	}
	fmt.Fprintf(&b, "%d statements are DDL or server administration (decision D2: schema is owned by SQL migrations). They are listed with their reasons in ledgers/grammar_map.json.\n", n)
	return b.String()
}
