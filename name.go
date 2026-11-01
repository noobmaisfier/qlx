package qlx

import (
	"hash/fnv"
	"strconv"
	"strings"
)

// QueryName extracts the name Tagged put in front of a rendered statement, or
// "" when there is none.
func QueryName(sql string) string {
	const pre = "/* qlx: "
	if !strings.HasPrefix(sql, pre) {
		return ""
	}
	rest := sql[len(pre):]
	end := strings.Index(rest, " */")
	if end < 0 {
		return ""
	}
	return rest[:end]
}

// Fingerprint identifies the statement's shape: a stable hash of the rendered
// SQL, in which values are placeholders, so every call of one shape has the
// same fingerprint whatever its arguments. It is what traces, metrics and the
// statement cache key on.
func (s Stmt) Fingerprint() string {
	h := fnv.New64a()
	h.Write([]byte(s.SQL))
	return strconv.FormatUint(h.Sum64(), 16)
}
