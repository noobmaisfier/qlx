// Package miniyaml reads the small YAML subset QLX's config uses, so the
// tool needs no dependency: nested mappings by indentation, block lists,
// inline lists, quoted and plain scalars, comments. Anchors, multi-document
// streams, block scalars (| and >) and flow mappings are rejected with an
// error rather than guessed at.
package miniyaml

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type line struct {
	no     int
	indent int
	text   string
}

// Parse decodes a document into maps (map[string]any), lists ([]any) and
// scalars (string, int64, float64, bool, nil).
func Parse(src []byte) (any, error) {
	var lines []line
	for i, raw := range strings.Split(strings.ReplaceAll(string(src), "\r\n", "\n"), "\n") {
		if strings.Contains(raw, "\t") && strings.TrimLeft(raw, " ") != strings.TrimLeft(raw, " \t") {
			return nil, fmt.Errorf("line %d: tabs cannot indent YAML", i+1)
		}
		t := strings.TrimRight(stripComment(raw), " \t")
		if strings.TrimSpace(t) == "" {
			continue
		}
		if strings.TrimSpace(t) == "---" || strings.TrimSpace(t) == "..." {
			if len(lines) == 0 {
				continue
			}
			return nil, fmt.Errorf("line %d: multiple documents are not supported", i+1)
		}
		ind := len(t) - len(strings.TrimLeft(t, " "))
		lines = append(lines, line{i + 1, ind, strings.TrimLeft(t, " ")})
	}
	if len(lines) == 0 {
		return map[string]any{}, nil
	}
	p := &parser{lines: lines}
	v, err := p.block(lines[0].indent)
	if err != nil {
		return nil, err
	}
	if p.pos < len(lines) {
		return nil, fmt.Errorf("line %d: unexpected indentation", lines[p.pos].no)
	}
	return v, nil
}

// ToJSON parses src and re-encodes it as JSON, which the config loader then
// decodes into its struct with unknown keys rejected.
func ToJSON(src []byte) ([]byte, error) {
	v, err := Parse(src)
	if err != nil {
		return nil, err
	}
	return json.Marshal(v)
}

func stripComment(s string) string {
	inS, inD := false, false
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '\'' && !inD:
			inS = !inS
		case c == '"' && !inS:
			if i == 0 || s[i-1] != '\\' {
				inD = !inD
			}
		case c == '#' && !inS && !inD && (i == 0 || s[i-1] == ' ' || s[i-1] == '\t'):
			return s[:i]
		}
	}
	return s
}

type parser struct {
	lines []line
	pos   int
}

func (p *parser) block(indent int) (any, error) {
	l := p.lines[p.pos]
	if l.indent != indent {
		return nil, fmt.Errorf("line %d: unexpected indentation", l.no)
	}
	if strings.HasPrefix(l.text, "- ") || l.text == "-" {
		return p.list(indent)
	}
	return p.mapping(indent)
}

func (p *parser) list(indent int) (any, error) {
	out := []any{}
	for p.pos < len(p.lines) {
		l := p.lines[p.pos]
		if l.indent < indent {
			break
		}
		if l.indent > indent {
			return nil, fmt.Errorf("line %d: unexpected indentation", l.no)
		}
		if !(strings.HasPrefix(l.text, "- ") || l.text == "-") {
			break
		}
		rest := strings.TrimSpace(strings.TrimPrefix(l.text, "-"))
		switch {
		case rest == "":
			p.pos++
			if p.pos < len(p.lines) && p.lines[p.pos].indent > indent {
				v, err := p.block(p.lines[p.pos].indent)
				if err != nil {
					return nil, err
				}
				out = append(out, v)
			} else {
				out = append(out, nil)
			}
		case isKeyValue(rest):
			// "- key: value" opens a mapping whose keys sit two columns in.
			p.lines[p.pos] = line{l.no, indent + 2, rest}
			v, err := p.mapping(indent + 2)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		default:
			v, err := scalar(rest, l.no)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
			p.pos++
		}
	}
	return out, nil
}

func isKeyValue(s string) bool {
	if strings.HasPrefix(s, "[") || strings.HasPrefix(s, "\"") || strings.HasPrefix(s, "'") {
		return false
	}
	i := strings.Index(s, ":")
	return i > 0 && (i == len(s)-1 || s[i+1] == ' ')
}

func (p *parser) mapping(indent int) (any, error) {
	out := map[string]any{}
	for p.pos < len(p.lines) {
		l := p.lines[p.pos]
		if l.indent < indent {
			break
		}
		if l.indent > indent {
			return nil, fmt.Errorf("line %d: unexpected indentation", l.no)
		}
		if !isKeyValue(l.text) {
			return nil, fmt.Errorf("line %d: expected \"key: value\", got %q", l.no, l.text)
		}
		i := strings.Index(l.text, ":")
		key := strings.TrimSpace(l.text[:i])
		if k, err := unquote(key); err == nil {
			key = k
		}
		if _, dup := out[key]; dup {
			return nil, fmt.Errorf("line %d: duplicate key %q", l.no, key)
		}
		rest := strings.TrimSpace(l.text[i+1:])
		p.pos++
		if rest == "" {
			if p.pos < len(p.lines) && (p.lines[p.pos].indent > indent ||
				(p.lines[p.pos].indent == indent && strings.HasPrefix(p.lines[p.pos].text, "- "))) {
				v, err := p.block(p.lines[p.pos].indent)
				if err != nil {
					return nil, err
				}
				out[key] = v
			} else {
				out[key] = nil
			}
			continue
		}
		v, err := scalar(rest, l.no)
		if err != nil {
			return nil, err
		}
		out[key] = v
	}
	return out, nil
}

func unquote(s string) (string, error) {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return strconv.Unquote(s)
	}
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		return strings.ReplaceAll(s[1:len(s)-1], "''", "'"), nil
	}
	return "", fmt.Errorf("not quoted")
}

func scalar(s string, no int) (any, error) {
	switch {
	case strings.HasPrefix(s, "|") || strings.HasPrefix(s, ">"):
		return nil, fmt.Errorf("line %d: block scalars are not supported", no)
	case strings.HasPrefix(s, "&") || strings.HasPrefix(s, "*") || strings.HasPrefix(s, "!"):
		return nil, fmt.Errorf("line %d: anchors, aliases and tags are not supported", no)
	case strings.HasPrefix(s, "{"):
		return nil, fmt.Errorf("line %d: flow mappings are not supported; use an indented mapping", no)
	case strings.HasPrefix(s, "["):
		if !strings.HasSuffix(s, "]") {
			return nil, fmt.Errorf("line %d: unterminated inline list", no)
		}
		inner := strings.TrimSpace(s[1 : len(s)-1])
		out := []any{}
		if inner == "" {
			return out, nil
		}
		for _, part := range splitTop(inner) {
			v, err := scalar(strings.TrimSpace(part), no)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	}
	if u, err := unquote(s); err == nil {
		return u, nil
	}
	switch s {
	case "true", "True":
		return true, nil
	case "false", "False":
		return false, nil
	case "null", "Null", "~":
		return nil, nil
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n, nil
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil && strings.ContainsAny(s, ".eE") {
		return f, nil
	}
	return s, nil
}

// splitTop splits on commas outside quotes.
func splitTop(s string) []string {
	var parts []string
	start, inS, inD := 0, false, false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\'':
			if !inD {
				inS = !inS
			}
		case '"':
			if !inS {
				inD = !inD
			}
		case ',':
			if !inS && !inD {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	return append(parts, s[start:])
}
