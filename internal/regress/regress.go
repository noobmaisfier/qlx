// Package regress is the regression-suite ledger of the plan: statements from
// PostgreSQL's own regression tests (src/test/regress/sql) are split, the
// query-side ones are translated by sql2go into QLX code, and the outcome
// of every single statement is recorded so that a failure is tracked
// individually and a regression (something that used to pass and no longer
// does) fails CI.
//
// Stages, each recorded per statement:
//
//	skipped-ddl    DDL, out of scope by decision D2
//	skipped-other  psql-only or server-administration statements
//	untranslated   sql2go could not express it (reason recorded)
//	translated     sql2go produced code; the generated case file then goes
//	               through qtest.RunRegress, which compiles, renders,
//	               normalises and compares, or falls back to PREPARE on a
//	               live server when the parser does not know the syntax yet
package regress

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// Stmt is one statement of a regression file.
type Stmt struct {
	File  string `json:"file"`
	Index int    `json:"index"` // 1-based within the file
	Line  int    `json:"line"`
	SQL   string `json:"sql"`
}

// ID is stable across runs as long as the statement text does not change.
func (s Stmt) ID() string {
	h := sha256.Sum256([]byte(s.SQL))
	return fmt.Sprintf("%s#%d-%s", s.File, s.Index, hex.EncodeToString(h[:4]))
}

// Split cuts SQL text into statements the way psql does for these files:
// semicolons end a statement outside quotes, dollar quotes and comments;
// backslash meta-commands and comments are dropped.
func Split(file, src string) []Stmt {
	var out []Stmt
	var cur strings.Builder
	line, startLine := 1, 0
	n := len(src)
	emit := func() {
		sql := strings.TrimSpace(cur.String())
		cur.Reset()
		if sql == "" {
			startLine = 0
			return
		}
		out = append(out, Stmt{File: file, Index: len(out) + 1, Line: startLine, SQL: sql})
		startLine = 0
	}
	atLineStart := true
	for i := 0; i < n; {
		c := src[i]
		if c == '\n' {
			line++
			atLineStart = true
			cur.WriteByte(c)
			i++
			continue
		}
		if atLineStart && c == '\\' && strings.TrimSpace(cur.String()) == "" { // psql meta-command
			for i < n && src[i] != '\n' {
				i++
			}
			continue
		}
		if c != ' ' && c != '\t' {
			atLineStart = false
		}
		if startLine == 0 && c != ' ' && c != '\t' && c != '\n' && !(c == '-' && i+1 < n && src[i+1] == '-') && !(c == '/' && i+1 < n && src[i+1] == '*') {
			startLine = line
		}
		switch {
		case c == '-' && i+1 < n && src[i+1] == '-':
			for i < n && src[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < n && src[i+1] == '*':
			depth := 0
			for i < n {
				if src[i] == '/' && i+1 < n && src[i+1] == '*' {
					depth++
					i += 2
				} else if src[i] == '*' && i+1 < n && src[i+1] == '/' {
					depth--
					i += 2
					if depth == 0 {
						break
					}
				} else {
					if src[i] == '\n' {
						line++
					}
					i++
				}
			}
		case c == '\'' || c == '"':
			j := i + 1
			for j < n {
				if src[j] == c {
					if j+1 < n && src[j+1] == c {
						j += 2
						continue
					}
					break
				}
				if src[j] == '\n' {
					line++
				}
				j++
			}
			if j < n {
				j++
			}
			cur.WriteString(src[i:j])
			i = j
		case c == '$':
			j := i + 1
			for j < n && (src[j] == '_' || src[j] >= 'a' && src[j] <= 'z' || src[j] >= 'A' && src[j] <= 'Z' || src[j] >= '0' && src[j] <= '9') {
				j++
			}
			if j < n && src[j] == '$' {
				tag := src[i : j+1]
				end := strings.Index(src[j+1:], tag)
				if end >= 0 {
					stop := j + 1 + end + len(tag)
					line += strings.Count(src[i:stop], "\n")
					cur.WriteString(src[i:stop])
					i = stop
					continue
				}
			}
			cur.WriteByte(c)
			i++
		case c == ';':
			emit()
			i++
		default:
			cur.WriteByte(c)
			i++
		}
	}
	emit()
	return out
}

// Kind is the coarse class of a statement.
type Kind int

// The classes.
const (
	QuerySide Kind = iota
	DDL
	Other
)

var ddlWords = map[string]bool{
	"CREATE": true, "ALTER": true, "DROP": true, "COMMENT": true, "SECURITY": true, "REINDEX": false,
	"IMPORT": true, "REASSIGN": true, "GRANT": false, "REVOKE": false, "DO": true, "LOAD": true,
}

var queryWords = map[string]bool{
	"SELECT": true, "WITH": true, "VALUES": true, "TABLE": true, "INSERT": true, "UPDATE": true, "DELETE": true, "MERGE": true,
	"EXPLAIN": true, "COPY": true, "PREPARE": true, "EXECUTE": true, "DEALLOCATE": true, "DECLARE": true, "FETCH": true, "MOVE": true,
	"CLOSE": true, "BEGIN": true, "START": true, "COMMIT": true, "END": true, "ROLLBACK": true, "SAVEPOINT": true, "RELEASE": true,
	"SET": true, "RESET": true, "SHOW": true, "LISTEN": true, "UNLISTEN": true, "NOTIFY": true, "CALL": true, "TRUNCATE": true,
	"LOCK": true, "VACUUM": true, "ANALYZE": true, "ANALYSE": true, "CHECKPOINT": true, "DISCARD": true, "REFRESH": true, "CLUSTER": true,
	"REINDEX": true, "GRANT": true, "REVOKE": true, "REPACK": true, "WAIT": true,
}

// Classify looks at the first keyword (and, for CREATE ... AS, nothing more:
// CREATE TABLE AS is DDL by decision D2).
func Classify(sql string) Kind {
	w := strings.ToUpper(firstWord(sql))
	switch {
	case queryWords[w]:
		return QuerySide
	case ddlWords[w]:
		return DDL
	case strings.HasPrefix(sql, "("):
		return QuerySide
	}
	return Other
}

func firstWord(s string) string {
	s = strings.TrimLeft(s, " \t\r\n")
	i := 0
	for i < len(s) && (s[i] == '_' || s[i] >= 'a' && s[i] <= 'z' || s[i] >= 'A' && s[i] <= 'Z') {
		i++
	}
	return s[:i]
}

// Translator turns one statement into QLX code. Warnings are kept.
type Translator func(sql string) (code string, warnings []string, err error)

// Status of one statement.
const (
	SkippedDDL   = "skipped-ddl"
	SkippedOther = "skipped-other"
	Untranslated = "untranslated"
	Translated   = "translated"
)

// Entry is one ledger row.
type Entry struct {
	ID       string   `json:"id"`
	File     string   `json:"file"`
	Line     int      `json:"line"`
	Status   string   `json:"status"`
	Reason   string   `json:"reason,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
	Code     string   `json:"-"`
	SQL      string   `json:"-"`
}

// Ledger is the file written by `qlx regress`.
type Ledger struct {
	Source  string  `json:"source"`
	Entries []Entry `json:"entries"`
}

// Build runs the first two stages over files (name -> content).
func Build(source string, files map[string]string, tr Translator) *Ledger {
	l := &Ledger{Source: source}
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		for _, s := range Split(name, files[name]) {
			e := Entry{ID: s.ID(), File: s.File, Line: s.Line, SQL: s.SQL}
			switch Classify(s.SQL) {
			case DDL:
				e.Status, e.Reason = SkippedDDL, "DDL (decision D2)"
			case Other:
				e.Status, e.Reason = SkippedOther, "not a query-side statement: "+firstWord(s.SQL)
			default:
				code, warns, err := tr(s.SQL)
				if err != nil {
					e.Status, e.Reason = Untranslated, firstLine(err.Error())
				} else {
					e.Status, e.Code, e.Warnings = Translated, code, warns
				}
			}
			l.Entries = append(l.Entries, e)
		}
	}
	return l
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// Counts summarises by status.
func (l *Ledger) Counts() map[string]int {
	c := map[string]int{}
	for _, e := range l.Entries {
		c[e.Status]++
	}
	return c
}

// Regressions lists entries that were translated (or better) in old and are
// untranslated now, plus entries that vanished from the source.
func Regressions(old, cur *Ledger) []string {
	now := map[string]Entry{}
	for _, e := range cur.Entries {
		now[e.ID] = e
	}
	var out []string
	for _, o := range old.Entries {
		if o.Status != Translated {
			continue
		}
		n, ok := now[o.ID]
		switch {
		case !ok:
			out = append(out, o.ID+": no longer present (statement text changed: re-record the ledger)")
		case n.Status != Translated:
			out = append(out, o.ID+": was translated, now "+n.Status+" ("+n.Reason+")")
		}
	}
	sort.Strings(out)
	return out
}

// Save writes the ledger as JSON.
func (l *Ledger) Save(path string) error {
	b, err := json.MarshalIndent(l, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// Load reads a ledger.
func Load(path string) (*Ledger, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var l Ledger
	return &l, json.Unmarshal(b, &l)
}

// EmitCases writes a Go file declaring one case per translated statement, for
// qtest.RunRegress. pkg is the package of the file; imports are what the
// translated snippets need besides qlx.
func EmitCases(pkg, importPrefix string, l *Ledger, imports []string) string {
	var b strings.Builder
	b.WriteString("// Code generated by qlx regress; DO NOT EDIT.\n\npackage " + pkg + "\n\nimport (\n")
	b.WriteString("\t\"github.com/noobmaisfier/qlx\"\n\t\"github.com/noobmaisfier/qlx/qtest\"\n")
	for _, im := range imports {
		if im != "github.com/noobmaisfier/qlx" && im != "github.com/noobmaisfier/qlx/qtest" {
			fmt.Fprintf(&b, "\t%q\n", im)
		}
	}
	b.WriteString(")\n\nvar _ = qlx.PG18\n\n// RegressCases are the translated statements.\nvar RegressCases = []qtest.RegressCase{\n")
	for _, e := range l.Entries {
		if e.Status != Translated {
			continue
		}
		fmt.Fprintf(&b, "\t{ID: %q, Original: %q, Build: func() qlx.Statement {\n", e.ID, e.SQL)
		for _, ln := range strings.Split(strings.TrimRight(e.Code, "\n"), "\n") {
			b.WriteString("\t\t" + ln + "\n")
		}
		b.WriteString("\t\treturn stmt\n\t}},\n")
	}
	b.WriteString("}\n")
	return b.String()
}
