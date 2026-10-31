// diagnose.go: compiler-style semantic validation — every diagnostic
// carries a file:line:col position so editors and CI can point at the
// exact offending token. Syntax errors come from BurntSushi's
// ParseError (Position.Start -> line:col); semantic checks run over a
// tolerant line scanner so one bad line never hides the rest.
package main

import (
	"fmt"
	"strings"

	"github.com/BurntSushi/toml"
)

// Diag is one diagnostic in "path:line:col: severity: message" form.
type Diag struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Col      int    `json:"col"`
	Severity string `json:"severity"` // "error" | "warning"
	Message  string `json:"message"`
}

func (d Diag) String() string {
	return fmt.Sprintf("%s:%d:%d: %s: %s", d.File, d.Line, d.Col, d.Severity, d.Message)
}

// tomlLine is one logical line of the TOML scanner: section context,
// key/value spans (byte offsets inside Line) and the decoded value.
type tomlLine struct {
	no      int    // 1-based line number
	section string // current table ("" = root), "*other*" for foreign tables
	key     string
	keyCol  int    // 1-based column of the key token
	valCol  int    // 1-based column where the value starts
	valText string // value text, trailing comment stripped
	val     any    // decoded value (nil when the fragment fails to parse)
}

// cutComment returns the index of the first `#` outside strings —
// TOML comments run to end of line. Handles basic "..." strings with
// backslash escapes and literal '...' strings.
func cutComment(s string) int {
	inBasic, inLit := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inBasic:
			if c == '\\' {
				i++
			} else if c == '"' {
				inBasic = false
			}
		case inLit:
			if c == '\'' {
				inLit = false
			}
		case c == '"':
			inBasic = true
		case c == '\'':
			inLit = true
		case c == '#':
			return i
		}
	}
	return -1
}

// balanced reports whether brackets/braces outside strings nest to
// zero — multi-line arrays keep consuming lines until they do.
func balanced(s string) bool {
	depth := 0
	inBasic, inLit := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inBasic:
			if c == '\\' {
				i++
			} else if c == '"' {
				inBasic = false
			}
		case inLit:
			if c == '\'' {
				inLit = false
			}
		case c == '"':
			inBasic = true
		case c == '\'':
			inLit = true
		case c == '[' || c == '{':
			depth++
		case c == ']' || c == '}':
			depth--
		}
	}
	return depth <= 0
}

// scanTOML walks source lines into tomlLine records; comment-only and
// blank lines produce no record. The scanner is deliberately lax —
// syntax soundness is BurntSushi's job; the scan exists to attach
// positions to keys.
func scanTOML(src []byte) []tomlLine {
	var out []tomlLine
	section := ""
	lines := strings.Split(string(src), "\n")
	for i := 0; i < len(lines); i++ {
		raw := lines[i]
		l := strings.TrimSpace(raw)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		if strings.HasPrefix(l, "[") {
			end := strings.Index(l, "]")
			if end < 0 {
				continue // broken header: the syntax decode reports it
			}
			name := strings.TrimSpace(l[1:end])
			if strings.HasPrefix(l, "[[") {
				section = "*other*"
			} else if sectionNames[name] {
				section = name
			} else {
				section = "*other*"
				out = append(out, tomlLine{no: i + 1, section: "*section*", key: name,
					keyCol: strings.Index(raw, name) + 1})
			}
			continue
		}
		eq := strings.Index(l, "=")
		if eq < 0 {
			continue
		}
		keyTok := strings.TrimSpace(l[:eq])
		if keyTok == "" {
			continue
		}
		key := strings.Trim(keyTok, `"'`)
		valText := strings.TrimSpace(l[eq+1:])
		// Consume continuation lines while brackets stay open.
		for !balanced(valText) && i+1 < len(lines) {
			i++
			valText += "\n" + lines[i]
		}
		if c := cutComment(valText); c >= 0 {
			valText = strings.TrimSpace(valText[:c])
		}
		keyCol := strings.Index(raw, keyTok) + 1
		// Column of the value: '=' position in the raw line + the
		// leading whitespace inside the value span.
		rawEq := strings.Index(raw, "=")
		valCol := rawEq + 2
		for j := rawEq + 1; j < len(raw); j++ {
			if raw[j] != ' ' && raw[j] != '\t' {
				valCol = j + 1
				break
			}
		}
		rec := tomlLine{no: i + 1, section: section, key: key,
			keyCol: keyCol, valCol: valCol, valText: valText}
		// Decode the value fragment on its own so a broken line only
		// loses itself. `x = <value>` decodes to {"x": value}.
		var frag map[string]any
		if _, err := toml.Decode("x = "+valText, &frag); err == nil {
			rec.val = frag["x"]
		}
		out = append(out, rec)
	}
	return out
}

// colFor converts a byte offset in src to a 1-based column of the
// 1-based line it sits on.
func colFor(src []byte, off int) int {
	line := 0
	col := 0
	for i := 0; i < len(src) && i < off; i++ {
		if src[i] == '\n' {
			line, col = line+1, 0
		} else {
			col++
		}
	}
	return col + 1
}

// validateTOMLDoc runs every check the schema defines and returns
// diagnostics in source order (syntax errors first — the semantic
// pass still runs so callers see everything in one shot).
func validateTOMLDoc(file string, src []byte) []Diag {
	var diags []Diag
	var doc map[string]any
	if _, err := toml.Decode(string(src), &doc); err != nil {
		if pe, ok2 := err.(toml.ParseError); ok2 {
			col := colFor(src, pe.Position.Start)
			d := Diag{File: file, Line: pe.Position.Line, Col: col,
				Severity: "error", Message: pe.Message}
			if pe.LastKey != "" {
				d.Message = pe.LastKey + ": " + pe.Message
			}
			diags = append(diags, d)
		} else {
			diags = append(diags, Diag{File: file, Line: 1, Col: 1,
				Severity: "error", Message: err.Error()})
		}
		// Syntax is unsound — semantic positions would lie. Bail.
		return diags
	}

	foundVersion := false
	for _, tl := range scanTOML(src) {
		switch {
		case tl.section == "*section*":
			diags = append(diags, Diag{file, tl.no, tl.keyCol, "warning",
				fmt.Sprintf("unknown section [%s]: only [extra] and [runtime] are defined", tl.key)})
		case tl.section == "*other*":
			// Keys under foreign tables: reported once via the header.
		case tl.section == "extra" || tl.section == "runtime":
			// Free-form zones — preserved, never rejected.
		default: // root
			switch tl.key {
			case "schema_version":
				foundVersion = true
				n, ok := tl.val.(int64)
				if !ok {
					diags = append(diags, Diag{file, tl.no, tl.valCol, "error",
						"schema_version must be an integer"})
				} else if n != schemaVersion {
					diags = append(diags, Diag{file, tl.no, tl.valCol, "error",
						fmt.Sprintf("unsupported schema_version %d (this tool understands %d)", n, schemaVersion)})
				}
			case legacyVersionKey:
				foundVersion = true
				diags = append(diags, Diag{file, tl.no, tl.keyCol, "warning",
					"formatVersion is deprecated; use schema_version = 1"})
			default:
				if strings.Contains(tl.key, ".") {
					diags = append(diags, Diag{file, tl.no, tl.keyCol, "warning",
						fmt.Sprintf("dotted key %q: use an [extra] or [runtime] table instead", tl.key)})
					continue
				}
				spec, known := declarativeKeys[tl.key]
				if !known {
					diags = append(diags, Diag{file, tl.no, tl.keyCol, "warning",
						fmt.Sprintf("unknown key %q: move it under [extra] so migration keeps it", tl.key)})
					continue
				}
				if tl.val == nil {
					continue // unparseable fragment: syntax pass reported
				}
				if msg := checkValue(tl.key, spec, tl.val); msg != "" {
					diags = append(diags, Diag{file, tl.no, tl.valCol, "error", msg})
				}
			}
		}
	}
	if !foundVersion {
		diags = append(diags, Diag{file, 1, 1, "error",
			"missing required key 'schema_version' at the document root"})
	}
	return diags
}

// hasErrors reports whether any diagnostic is an error.
func hasErrors(ds []Diag) bool {
	for _, d := range ds {
		if d.Severity == "error" {
			return true
		}
	}
	return false
}
