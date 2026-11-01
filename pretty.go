package qlx

import "strings"

// Pretty returns the statement text laid out for humans: a line per clause,
// subqueries indented. It only changes whitespace outside quoted names and
// literals, so the result is the same statement and safe to log or to paste
// into psql.
func (s Stmt) Pretty() string { return Pretty(s.SQL) }

var clauseWords = []string{
	"SELECT", "FROM", "WHERE", "GROUP BY", "HAVING", "WINDOW", "ORDER BY", "LIMIT", "OFFSET", "FETCH FIRST",
	"UNION ALL", "UNION", "INTERSECT ALL", "INTERSECT", "EXCEPT ALL", "EXCEPT",
	"LEFT JOIN", "RIGHT JOIN", "FULL JOIN", "CROSS JOIN", "JOIN", "ON", "SET", "VALUES", "RETURNING", "USING",
	"WHEN MATCHED", "WHEN NOT MATCHED", "ON CONFLICT", "FOR UPDATE", "FOR NO KEY UPDATE", "FOR SHARE", "FOR KEY SHARE",
}

// Pretty formats rendered SQL; see Stmt.Pretty.
func Pretty(sql string) string {
	var out strings.Builder
	depth := 0
	n := len(sql)
	for i := 0; i < n; {
		c := sql[i]
		switch {
		case c == '"': // quoted identifier, "" escapes
			j := i + 1
			for j < n {
				if sql[j] == '"' {
					if j+1 < n && sql[j+1] == '"' {
						j += 2
						continue
					}
					break
				}
				j++
			}
			out.WriteString(sql[i:min(j+1, n)])
			i = j + 1
		case c == '\'' || (c == 'E' && i+1 < n && sql[i+1] == '\'' && (i == 0 || !isWord(sql[i-1]))):
			j := i
			if c == 'E' {
				j++
			}
			j++ // opening quote
			for j < n {
				if sql[j] == '\\' && c == 'E' {
					j += 2
					continue
				}
				if sql[j] == '\'' {
					if j+1 < n && sql[j+1] == '\'' {
						j += 2
						continue
					}
					break
				}
				j++
			}
			out.WriteString(sql[i:min(j+1, n)])
			i = j + 1
		case c == '(':
			depth++
			out.WriteByte(c)
			i++
		case c == ')':
			if depth > 0 {
				depth--
			}
			out.WriteByte(c)
			i++
		case c == ' ' && i > 0:
			if kw := clauseAt(sql, i+1); kw != "" && !(kw == "ON" && inConflict(sql, i)) {
				out.WriteString("\n" + strings.Repeat("  ", depth))
			} else {
				out.WriteByte(' ')
			}
			i++
		default:
			out.WriteByte(c)
			i++
		}
	}
	return out.String()
}

func isWord(b byte) bool {
	return b == '_' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

func clauseAt(s string, i int) string {
	for _, k := range clauseWords {
		if strings.HasPrefix(s[i:], k) {
			end := i + len(k)
			if end == len(s) || !isWord(s[end]) {
				return k
			}
		}
	}
	return ""
}

// inConflict avoids breaking "ON CONFLICT" into a join-style ON line.
func inConflict(s string, i int) bool { return strings.HasPrefix(s[i+1:], "ON CONFLICT") }
