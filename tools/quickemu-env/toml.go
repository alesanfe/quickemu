// toml.go: a minimal TOML reader for env.toml — tools/quickemu-toml is
// a main package (not importable) and is owned by another workstream,
// so env carries its own line-oriented subset. The subset covers the
// env schema: [table] and [[array-of-table]] headers, bare/quoted
// keys, basic strings ("..." with escapes), literal strings ('...'),
// integers, floats, booleans and (possibly multi-line) arrays of
// scalars. Every value and every section header keeps its line:col so
// validation errors read like compiler diagnostics. Anything outside
// the subset (inline tables, dates, nested arrays, dotted-value keys)
// is a hard parse error, not a silent skip.
package main

import (
	"fmt"
	"strconv"
	"strings"
)

// tKind tags a parsed scalar so envfile.go can type-check fields.
type tKind byte

const (
	tString tKind = iota
	tInt
	tFloat
	tBool
	tArray
)

// tValue is one right-hand side of a `key = value` line.
type tValue struct {
	kind      tKind
	s         string   // tString
	i         int64    // tInt
	f         float64  // tFloat
	b         bool     // tBool
	arr       []tValue // tArray
	line, col int      // position of the first value byte (1-based)
}

// tTable is one [header] body (or one [[header]] element).
type tTable struct {
	vals map[string]tValue
	pos  map[string][2]int // key -> line:col of the key itself
}

func newTable() tTable {
	return tTable{vals: map[string]tValue{}, pos: map[string][2]int{}}
}

// tDoc is a parsed TOML file: section bodies plus the 1-based line of
// each section header (used for "empty section" diagnostics).
type tDoc struct {
	globals tTable              // keys before the first [header]
	singles map[string]tTable   // [name]
	arrays  map[string][]tTable // [[name]]
	hdrLine map[string]int      // first header line per section name
}

// parseErr is a syntax error with a position — validate renders it as
// file:line:col like every other env diagnostic.
type parseErr struct {
	line, col int
	msg       string
}

func (e *parseErr) Error() string { return e.msg }

func perr(line, col int, format string, args ...any) *parseErr {
	return &parseErr{line: line, col: col, msg: fmt.Sprintf(format, args...)}
}

// stripComment removes a trailing # comment, honouring both quote
// styles — a '#' inside "..." or '...' is string content.
func stripComment(l string) string {
	var q byte
	for i := 0; i < len(l); i++ {
		c := l[i]
		switch {
		case q == 0 && (c == '"' || c == '\''):
			q = c
		case q == '"' && c == '\\':
			i++ // escaped char can never close the quote
		case q != 0 && c == q:
			q = 0
		case q == 0 && c == '#':
			return l[:i]
		}
	}
	return l
}

// parseTOML parses the env.toml subset into a tDoc.
func parseTOML(file, src string) (*tDoc, error) {
	d := &tDoc{
		globals: newTable(),
		singles: map[string]tTable{},
		arrays:  map[string][]tTable{},
		hdrLine: map[string]int{},
	}
	cur := d.globals
	lines := strings.Split(src, "\n")
	for ln := 0; ln < len(lines); ln++ {
		raw := lines[ln]
		pre := stripComment(raw)
		// lead = bytes of leading whitespace TrimSpace removed —
		// every column computed on the trimmed line shifts by it.
		lead := len(pre) - len(strings.TrimLeft(pre, " \t"))
		l := strings.TrimSpace(pre)
		if l == "" {
			continue
		}
		keyLn := ln + 1 // physical line of the key (multi-line arrays move ln)
		switch {
		case strings.HasPrefix(l, "[["):
			end := strings.Index(l, "]]")
			if end < 0 {
				return nil, perr(ln+1, lead+1, "unterminated [[table]] header")
			}
			name := strings.TrimSpace(l[2:end])
			if rest := strings.TrimSpace(l[end+2:]); rest != "" {
				return nil, perr(ln+1, lead+end+3, "unexpected text after [[%s]] header", name)
			}
			if name == "" || !keyOK(name) {
				return nil, perr(ln+1, lead+3, "invalid table name %q", name)
			}
			if _, dup := d.singles[name]; dup {
				return nil, perr(ln+1, lead+3, "[[%s]] conflicts with earlier [%s]", name, name)
			}
			t := newTable()
			d.arrays[name] = append(d.arrays[name], t)
			if _, seen := d.hdrLine[name]; !seen {
				d.hdrLine[name] = ln + 1
			}
			cur = t
		case strings.HasPrefix(l, "["):
			end := strings.Index(l, "]")
			if end < 0 {
				return nil, perr(ln+1, lead+1, "unterminated [table] header")
			}
			name := strings.TrimSpace(l[1:end])
			if rest := strings.TrimSpace(l[end+1:]); rest != "" {
				return nil, perr(ln+1, lead+end+2, "unexpected text after [%s] header", name)
			}
			if name == "" || !keyOK(name) {
				return nil, perr(ln+1, lead+2, "invalid table name %q", name)
			}
			if _, dup := d.singles[name]; dup {
				return nil, perr(ln+1, lead+2, "duplicate [%s] table", name)
			}
			if _, dup := d.arrays[name]; dup {
				return nil, perr(ln+1, lead+2, "[%s] conflicts with earlier [[%s]]", name, name)
			}
			t := newTable()
			d.singles[name] = t
			d.hdrLine[name] = ln + 1
			cur = t
		default:
			eq := keyEq(l)
			if eq <= 0 {
				return nil, perr(ln+1, lead+1, "expected key = value or a [table] header")
			}
			key := strings.TrimSpace(l[:eq])
			kcol := lead + 1 // trimmed lines start at the key byte
			if q := key[0]; q == '"' || q == '\'' {
				s, n, err := scanString(l, 0)
				if err != nil || strings.TrimSpace(l[n:eq]) != "" || s == "" {
					return nil, perr(ln+1, lead+1, "invalid quoted key")
				}
				key = s
			} else if !keyOK(key) {
				return nil, perr(ln+1, lead+1, "invalid key %q", key)
			}
			if _, dup := cur.vals[key]; dup {
				pl, pc := cur.pos[key][0], cur.pos[key][1]
				return nil, perr(ln+1, kcol, "duplicate key %q (first at %d:%d)", key, pl, pc)
			}
			valText := l[eq+1:]
			colBase := lead + eq + 2 // column of valText[0] in the raw line
			// Multi-line arrays: keep pulling lines until the bracket
			// depth (counted outside strings) closes.
			for depth := bracketDepth(valText); depth > 0 && ln+1 < len(lines); {
				ln++
				valText += "\n" + stripComment(lines[ln])
				if bracketDepth(valText) <= 0 {
					break
				}
			}
			v, err := parseValue(valText)
			if err != nil {
				if pe, ok := err.(*parseErr); ok {
					// Value-relative positions re-anchor to the key's
					// physical line and column.
					pe.line += keyLn
					pe.col += colBase - 1
					return nil, pe
				}
				return nil, perr(keyLn, colBase, "%s", err)
			}
			v.col = colBase
			v.line = keyLn
			cur.vals[key] = *v
			cur.pos[key] = [2]int{keyLn, kcol}
		}
	}
	return d, nil
}

// keyOK: bare keys are [A-Za-z0-9_-] runs (dotted keys are not part of
// the env subset — quoted keys carry the dots if ever needed).
func keyOK(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' ||
			r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

// keyEq finds the '=' that ends a key — the first '=' outside quotes.
func keyEq(l string) int {
	var q byte
	for i := 0; i < len(l); i++ {
		c := l[i]
		switch {
		case q == 0 && (c == '"' || c == '\''):
			q = c
		case q == '"' && c == '\\':
			i++
		case q != 0 && c == q:
			q = 0
		case q == 0 && c == '=':
			return i
		}
	}
	return -1
}

// bracketDepth counts [ ] balance outside strings; >0 means the array
// continues on the next line.
func bracketDepth(s string) int {
	var q byte
	depth := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case q == 0 && (c == '"' || c == '\''):
			q = c
		case q == '"' && c == '\\':
			i++
		case q != 0 && c == q:
			q = 0
		case q == 0 && c == '[':
			depth++
		case q == 0 && c == ']':
			depth--
		}
	}
	return depth
}

// scanString decodes a basic ("...") or literal ('...') string
// starting at s[i] (the quote). Returns the decoded text and the index
// just past the closing quote.
func scanString(s string, i int) (string, int, error) {
	q := s[i]
	j := i + 1
	var b strings.Builder
	for j < len(s) {
		c := s[j]
		if c == q {
			return b.String(), j + 1, nil
		}
		if q == '"' && c == '\\' {
			if j+1 >= len(s) {
				return "", 0, fmt.Errorf("unterminated escape")
			}
			j++
			switch s[j] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case 'b':
				b.WriteByte('\b')
			case 'f':
				b.WriteByte('\f')
			case '"':
				b.WriteByte('"')
			case '\\':
				b.WriteByte('\\')
			case 'u', 'U':
				n := 4
				if s[j] == 'U' {
					n = 8
				}
				if j+n >= len(s) {
					return "", 0, fmt.Errorf("short \\%c escape", s[j])
				}
				cp, err := strconv.ParseUint(s[j+1:j+1+n], 16, 32)
				if err != nil {
					return "", 0, fmt.Errorf("bad \\%c escape", s[j])
				}
				b.WriteRune(rune(cp))
				j += n
			default:
				return "", 0, fmt.Errorf("unknown escape \\%c", s[j])
			}
			j++
			continue
		}
		if c == '\n' {
			return "", 0, fmt.Errorf("newline in string")
		}
		b.WriteByte(c)
		j++
	}
	return "", 0, fmt.Errorf("unterminated string")
}

// parseValue parses a scalar or single-line-or-more array. The value
// text may span lines (arrays); positions inside errors are relative
// to the value start — the caller re-anchors them.
func parseValue(s string) (*tValue, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, perr(0, 0, "missing value")
	}
	switch s[0] {
	case '"', '\'':
		str, n, err := scanString(s, 0)
		if err != nil {
			return nil, perr(0, 1, "%s", err)
		}
		if strings.TrimSpace(s[n:]) != "" {
			return nil, perr(0, n+1, "trailing text after string")
		}
		return &tValue{kind: tString, s: str}, nil
	case '[':
		return parseArray(s)
	case 't', 'f':
		if s == "true" {
			return &tValue{kind: tBool, b: true}, nil
		}
		if s == "false" {
			return &tValue{kind: tBool, b: false}, nil
		}
		return nil, perr(0, 1, "invalid value %q", firstTok(s))
	}
	if i, err := strconv.ParseInt(strings.ReplaceAll(s, "_", ""), 10, 64); err == nil {
		return &tValue{kind: tInt, i: i}, nil
	}
	if f, err := strconv.ParseFloat(strings.ReplaceAll(s, "_", ""), 64); err == nil {
		return &tValue{kind: tFloat, f: f}, nil
	}
	return nil, perr(0, 1, "unsupported value %q", firstTok(s))
}

func firstTok(s string) string {
	if i := strings.IndexAny(s, " \t\n"); i > 0 {
		return s[:i]
	}
	if len(s) > 24 {
		return s[:24] + "..."
	}
	return s
}

// parseArray parses a [a, b, c] array of scalars — commas inside
// strings do not split. Trailing commas are legal (TOML parity).
func parseArray(s string) (*tValue, error) {
	out := &tValue{kind: tArray}
	i := 1 // skip [
	for {
		// skip whitespace and newlines
		for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r') {
			i++
		}
		if i >= len(s) {
			return nil, perr(0, i, "unterminated array")
		}
		if s[i] == ']' {
			i++
			if strings.TrimSpace(s[i:]) != "" {
				return nil, perr(0, i+1, "trailing text after array")
			}
			return out, nil
		}
		var v *tValue
		var err error
		switch s[i] {
		case '"', '\'':
			str, n, e := scanString(s, i)
			if e != nil {
				return nil, perr(0, i+1, "%s", e)
			}
			v = &tValue{kind: tString, s: str}
			i = n
		case '[':
			return nil, perr(0, i+1, "nested arrays are not supported")
		default:
			j := i
			for j < len(s) && s[j] != ',' && s[j] != ']' {
				j++
			}
			tok := strings.TrimSpace(s[i:j])
			if tok == "" {
				return nil, perr(0, i+1, "empty array element")
			}
			v, err = parseValue(tok)
			if err != nil {
				if pe, ok := err.(*parseErr); ok {
					pe.col += i
				}
				return nil, err
			}
			i = j
		}
		out.arr = append(out.arr, *v)
		for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r') {
			i++
		}
		if i >= len(s) {
			return nil, perr(0, i, "unterminated array")
		}
		if s[i] == ',' {
			i++
			continue
		}
		if s[i] == ']' {
			continue // loop top handles the close
		}
		return nil, perr(0, i+1, "expected ',' or ']' in array")
	}
}

// strs renders a tArray as []string; non-string elements are errors at
// the call site (envfile reports them with the value's position).
func (v tValue) strs() ([]string, error) {
	if v.kind != tArray {
		return nil, fmt.Errorf("expected a list, got %s", v.kindName())
	}
	out := make([]string, 0, len(v.arr))
	for _, e := range v.arr {
		if e.kind != tString {
			return nil, fmt.Errorf("expected a list of strings")
		}
		out = append(out, e.s)
	}
	return out, nil
}

func (v tValue) kindName() string {
	switch v.kind {
	case tString:
		return "string"
	case tInt:
		return "integer"
	case tFloat:
		return "float"
	case tBool:
		return "boolean"
	case tArray:
		return "list"
	}
	return "value"
}
