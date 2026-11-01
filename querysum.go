package qlx

import "strings"

// QueryInfo is what a log line, metric or trace needs to know about a rendered
// statement without ever looking at its arguments.
type QueryInfo struct {
	// Name is the label given with Tagged ("" when untagged).
	Name string
	// Operation is the statement's verb in upper case: SELECT, INSERT, UPDATE,
	// DELETE, MERGE, COPY, CALL... For a WITH query it is the verb of the main
	// statement. "" when the text is empty.
	Operation string
}

// Label is a low-cardinality name for the statement: Name when tagged,
// otherwise Operation. It is what to use as a span name or metric attribute;
// the SQL text itself is not (it varies with the query's shape).
func (i QueryInfo) Label() string {
	if i.Name != "" {
		return i.Name
	}
	return i.Operation
}

const tagPrefix = "/* qlx: "

// DescribeSQL extracts the Tagged name and the operation from SQL text as
// produced by Render. It does not parse SQL: it reads the leading tag comment
// and then the first keyword (skipping a WITH list by paren depth).
func DescribeSQL(sql string) QueryInfo {
	var info QueryInfo
	s := strings.TrimLeft(sql, " \t\r\n")
	if strings.HasPrefix(s, tagPrefix) {
		if end := strings.Index(s, " */"); end > 0 {
			info.Name = s[len(tagPrefix):end]
			s = strings.TrimLeft(s[end+3:], " \t\r\n")
		}
	}
	word, rest := firstWord(s)
	if !strings.EqualFold(word, "WITH") {
		info.Operation = strings.ToUpper(word)
		return info
	}
	// WITH [RECURSIVE] name AS (...) [, name AS (...)] <verb> ...
	depth := 0
	for i := 0; i < len(rest); i++ {
		switch c := rest[i]; {
		case c == '\'' || c == '"':
			// skip a quoted string or identifier ('' and "" escape themselves)
			j := i + 1
			for j < len(rest) {
				if rest[j] == c {
					if j+1 < len(rest) && rest[j+1] == c {
						j += 2
						continue
					}
					break
				}
				j++
			}
			i = j
		case c == '(':
			depth++
		case c == ')':
			depth--
		case depth == 0 && isLetter(c):
			j := i
			for j < len(rest) && (isLetter(rest[j]) || rest[j] == '_') {
				j++
			}
			switch w := strings.ToUpper(rest[i:j]); w {
			case "SELECT", "INSERT", "UPDATE", "DELETE", "MERGE", "VALUES", "TABLE":
				info.Operation = w
				return info
			}
			i = j - 1
		}
	}
	info.Operation = "WITH"
	return info
}

func isLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

func firstWord(s string) (word, rest string) {
	i := 0
	for i < len(s) && (isLetter(s[i]) || s[i] == '_') {
		i++
	}
	return s[:i], s[i:]
}
