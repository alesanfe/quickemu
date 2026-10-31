// conflib.go: .conf parsing and the conf <-> TOML converters.
//
// A .conf is a Bash fragment quickemu used to source — the parser is
// purely lexical and NEVER executes anything: lines are classified as
// comment, `key="value"`, `key=(a b)` arrays or non-migratable (which
// produce explicit warnings naming the line).
package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

// confEntry is one migrated assignment plus the comment block that
// preceded it (comments travel with their key through reordering).
type confEntry struct {
	key      string
	vals     []string // scalar -> one element; bash array -> N
	isArray  bool
	comments []string // raw '#'-prefixed lines
	line     int      // 1-based source line of the assignment
}

// confDoc is a parsed .conf: entries in source order, comments left
// dangling after the last entry, and non-migratable warnings.
type confDoc struct {
	entries  []confEntry
	trailing []string
	warnings []string
}

var confAssignRe = regexp.MustCompile(`^\s*(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*=`)

// shellTokens splits a bash array body `"a" 'b' c` honouring quotes
// and backslash escapes; ok=false on unbalanced quoting.
func shellTokens(s string) (toks []string, ok bool) {
	for i := 0; i < len(s); {
		c := s[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			i++
			continue
		}
		var tok strings.Builder
		for i < len(s) {
			c = s[i]
			switch c {
			case ' ', '\t', '\n', '\r':
				goto done
			case '\\':
				if i+1 < len(s) {
					i++
					tok.WriteByte(s[i])
				}
				i++
			case '"', '\'':
				q := c
				i++
				for i < len(s) && s[i] != q {
					if q == '"' && s[i] == '\\' && i+1 < len(s) {
						i++
					}
					tok.WriteByte(s[i])
					i++
				}
				if i >= len(s) {
					return nil, false // unbalanced quote
				}
				i++
			default:
				tok.WriteByte(c)
				i++
			}
		}
	done:
		toks = append(toks, tok.String())
	}
	return toks, true
}

// unquoteConf strips the quoting of a scalar value: "a\"b" -> a"b,
// 'x' -> x, bare -> bare. The value may carry a trailing comment
// (`key="v" # note`) — returned separately.
func unquoteConf(raw string) (val, comment string, warn string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", ""
	}
	var end int
	var b strings.Builder
	switch raw[0] {
	case '"', '\'':
		q := raw[0]
		for end = 1; end < len(raw); end++ {
			c := raw[end]
			if q == '"' && c == '\\' && end+1 < len(raw) {
				end++
				b.WriteByte(raw[end])
				continue
			}
			if c == q {
				break
			}
			b.WriteByte(c)
		}
		if end >= len(raw) {
			return "", "", "unbalanced quoting; value not migrated"
		}
		val = b.String()
		rest := strings.TrimSpace(raw[end+1:])
		if strings.HasPrefix(rest, "#") {
			comment = rest
		}
		return val, comment, ""
	default:
		// Bare value: read to whitespace or comment.
		for end = 0; end < len(raw); end++ {
			c := raw[end]
			if c == ' ' || c == '\t' || c == '#' {
				break
			}
			if c == '\\' && end+1 < len(raw) {
				end++
			}
			b.WriteByte(raw[end])
		}
		rest := strings.TrimSpace(raw[end:])
		if strings.HasPrefix(rest, "#") {
			comment = rest
		}
		return b.String(), comment, ""
	}
}

// parseConf lexes a .conf — no execution, no expansion. Lines that
// are neither blank, comments, `key="value"`/`key=(…)` assignments
// (optionally `export`-prefixed) produce explicit warnings.
func parseConf(src []byte) confDoc {
	var d confDoc
	lines := strings.Split(string(src), "\n")
	var pending []string
	flush := func() []string {
		c := pending
		pending = nil
		return c
	}
	for i := 0; i < len(lines); i++ {
		raw := lines[i]
		l := strings.TrimSpace(raw)
		switch {
		case l == "":
			// Blank lines break comment blocks loose — keep them in
			// pending so spacing between blocks survives.
			pending = append(pending, "")
		case strings.HasPrefix(l, "#"):
			pending = append(pending, strings.TrimRight(raw, " \t\r"))
		default:
			m := confAssignRe.FindStringSubmatch(raw)
			if m == nil {
				d.warnings = append(d.warnings,
					fmt.Sprintf("line %d: not a key=\"value\" assignment — not migrated: %s", i+1, l))
				flush()
				continue
			}
			if strings.HasPrefix(l, "export ") {
				d.warnings = append(d.warnings,
					fmt.Sprintf("line %d: 'export' prefix dropped during migration", i+1))
			}
			key := m[1]
			rest := raw[strings.Index(raw, "=")+1:]
			e := confEntry{key: key, line: i + 1, comments: flush()}
			if strings.HasPrefix(strings.TrimSpace(rest), "(") {
				// Bash array — may span lines until the close paren.
				body := rest
				for !strings.Contains(body, ")") && i+1 < len(lines) {
					i++
					body += "\n" + lines[i]
				}
				open := strings.Index(body, "(")
				cls := strings.LastIndex(body, ")")
				if cls <= open {
					d.warnings = append(d.warnings,
						fmt.Sprintf("line %d: unbalanced array for %q — not migrated", e.line, key))
					continue
				}
				toks, ok := shellTokens(body[open+1 : cls])
				if !ok {
					d.warnings = append(d.warnings,
						fmt.Sprintf("line %d: unbalanced quoting in array %q — not migrated", e.line, key))
					continue
				}
				e.vals, e.isArray = toks, true
				if c := strings.TrimSpace(body[cls+1:]); strings.HasPrefix(c, "#") {
					e.comments = append(e.comments, c)
				}
			} else {
				v, comment, warn := unquoteConf(rest)
				if warn != "" {
					d.warnings = append(d.warnings,
						fmt.Sprintf("line %d: %s — %q not migrated", e.line, warn, key))
					continue
				}
				e.vals = []string{v}
				if strings.ContainsAny(v, "$`") {
					d.warnings = append(d.warnings,
						fmt.Sprintf("line %d: %q contains shell substitution — migrated literally (never executed)", e.line, key))
				}
				if comment != "" {
					e.comments = append(e.comments, comment)
				}
			}
			d.entries = append(d.entries, e)
		}
	}
	d.trailing = flush()
	return d
}

// kv returns the flattened key -> value view of a confDoc. Array
// entries keep the bash `("a" "b")` spelling so a map compare is
// spelling-stable across the conf -> toml -> conf round trip.
func (d confDoc) kv() map[string]string {
	m := map[string]string{}
	for _, e := range d.entries {
		if e.isArray {
			var b strings.Builder
			b.WriteString("(")
			for i, v := range e.vals {
				if i > 0 {
					b.WriteByte(' ')
				}
				b.WriteString(`"` + v + `"`)
			}
			b.WriteString(")")
			m[e.key] = b.String()
		} else {
			m[e.key] = e.vals[0]
		}
	}
	return m
}

// escapeTOML renders a Go string as a TOML basic string (with quotes).
func escapeTOML(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\t", `\t`, "\r", `\r`)
	return `"` + r.Replace(s) + `"`
}

// escapeConf renders a Go string inside .conf double quotes — the
// four expansions Bash performs must be escaped or the value changes
// meaning when sourced.
func escapeConf(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, `$`, `\$`, "`", "\\`")
	return `"` + r.Replace(s) + `"`
}

// confToTOML renders a parsed .conf as schema-v1 TOML text. Known
// declarative keys emit in tomlKeyOrder (canonical ordering), unknown
// keys keep their source order under [extra] so nothing is dropped.
// Comment blocks travel with the key they preceded.
func confToTOML(d confDoc) string {
	var b strings.Builder
	b.WriteString("# Migrated by quickemu config migrate\n")
	b.WriteString("schema_version = 1\n")

	byKey := map[string]confEntry{}
	var order []string // source order for tie-breaking
	for _, e := range d.entries {
		if _, dup := byKey[e.key]; !dup {
			order = append(order, e.key)
		}
		byKey[e.key] = e // last wins — Bash semantics
	}

	emit := func(e confEntry) {
		for _, c := range e.comments {
			if c == "" {
				b.WriteByte('\n')
			} else {
				b.WriteString(c + "\n")
			}
		}
		if e.isArray {
			var parts []string
			for _, v := range e.vals {
				parts = append(parts, escapeTOML(v))
			}
			fmt.Fprintf(&b, "%s = [%s]\n", e.key, strings.Join(parts, ", "))
		} else {
			fmt.Fprintf(&b, "%s = %s\n", e.key, escapeTOML(e.vals[0]))
		}
	}

	done := map[string]bool{}
	wrote := false
	for _, k := range tomlKeyOrder {
		if e, ok := byKey[k]; ok {
			emit(e)
			done[k], wrote = true, true
		}
	}
	// Known keys outside the canonical order keep source order.
	for _, k := range order {
		if done[k] {
			continue
		}
		if e := byKey[k]; knownKey(e.key) {
			emit(e)
			done[k] = true
		}
	}
	var extras []confEntry
	for _, k := range order {
		if !done[k] {
			extras = append(extras, byKey[k])
		}
	}
	if len(extras) > 0 {
		if wrote {
			b.WriteByte('\n')
		}
		b.WriteString("# Unrecognised keys preserved verbatim (QE-111)\n[extra]\n")
		for _, e := range extras {
			emit(e)
		}
	}
	if t := d.trailing; len(t) > 0 {
		b.WriteByte('\n')
		for _, c := range t {
			if c != "" {
				b.WriteString(c + "\n")
			}
		}
	}
	return b.String()
}

// tomlToConf renders TOML source back as .conf lines. Declarative
// root keys and [extra] keys emit `key="v"` (arrays become bash
// arrays); [runtime] and foreign tables are skipped — runtime state
// never round-trips. Comment lines are re-emitted verbatim. It
// returns the conf text and warnings for values a .conf cannot hold
// (multiline strings, non-scalar types).
func tomlToConf(src []byte) (string, []string) {
	var b strings.Builder
	var warns []string
	section := ""
	skip := false
	var pending []string // comment lines awaiting the next key
	emitComments := func() {
		for _, c := range pending {
			b.WriteString(c + "\n")
		}
		pending = nil
	}
	lines := strings.Split(string(src), "\n")
	for i := 0; i < len(lines); i++ {
		raw := strings.TrimRight(lines[i], " \t\r")
		l := strings.TrimSpace(raw)
		switch {
		case l == "":
			pending = append(pending, "")
		case strings.HasPrefix(l, "#"):
			pending = append(pending, raw)
		case strings.HasPrefix(l, "["):
			end := strings.Index(l, "]")
			if end < 0 {
				warns = append(warns, fmt.Sprintf("line %d: malformed table header — skipped", i+1))
				continue
			}
			name := strings.TrimSpace(l[1:end])
			section = name
			skip = name != "extra" // [extra] rejoins the root namespace
			pending = nil          // section headers drop pending comments
		default:
			eq := strings.Index(l, "=")
			if eq < 0 {
				if l != "" {
					warns = append(warns, fmt.Sprintf("line %d: not an assignment — skipped: %s", i+1, l))
				}
				continue
			}
			keyTok := strings.TrimSpace(l[:eq])
			key := strings.Trim(keyTok, `"'`)
			valText := strings.TrimSpace(l[eq+1:])
			for !balanced(valText) && i+1 < len(lines) {
				i++
				valText += "\n" + lines[i]
			}
			if c := cutComment(valText); c >= 0 {
				if cm := strings.TrimSpace(valText[c:]); section != "runtime" {
					pending = append(pending, cm)
				}
				valText = strings.TrimSpace(valText[:c])
			}
			if section == "runtime" || (skip && section != "extra" && section != "") {
				pending = nil
				continue
			}
			if key == "schema_version" || key == legacyVersionKey {
				pending = nil
				continue // schema metadata is not a conf key
			}
			var frag map[string]any
			if _, err := toml.Decode("x = "+valText, &frag); err != nil {
				warns = append(warns, fmt.Sprintf("line %d: %q has an unparseable value — skipped", i+1, key))
				continue
			}
			emitComments()
			switch v := frag["x"].(type) {
			case []any:
				var parts []string
				okAll := true
				for _, e := range v {
					s, ok := strVal(e)
					if !ok {
						okAll = false
						break
					}
					parts = append(parts, escapeConf(s))
				}
				if !okAll {
					warns = append(warns, fmt.Sprintf("line %d: %q array has non-scalar elements — skipped", i+1, key))
					continue
				}
				fmt.Fprintf(&b, "%s=(%s)\n", key, strings.Join(parts, " "))
			default:
				s, ok := strVal(v)
				if !ok {
					warns = append(warns, fmt.Sprintf("line %d: %q value cannot be represented in .conf — skipped", i+1, key))
					continue
				}
				fmt.Fprintf(&b, "%s=%s\n", key, escapeConf(s))
			}
		}
	}
	// Trailing comments survive too.
	emitComments()
	return b.String(), warns
}
