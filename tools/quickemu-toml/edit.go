// edit.go: QE-112 — transactional single-key edits for .conf and
// .toml files.
//
// Pipeline for every set: validate key+value -> patch ONLY the target
// line in the original bytes -> optional history snapshot -> fsync +
// atomic rename (renameio, ADR-0006) -> original permission bits are
// preserved via WithExistingPermissions.
//
// Concurrent-modification detection: the caller snapshots the file
// with NewEditGuard (mtime+size+SHA-256) before preparing the edit;
// the guard is verified again right before the rename, so a writer
// that raced the read-modify-write window is caught instead of
// silently clobbered (TOCTOU between verify and rename is inherent —
// the guard shrinks the window to one syscall pair).
package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/google/renameio/v2"
)

// ErrConcurrent marks a failed EditGuard verification.
var ErrConcurrent = errors.New("file changed concurrently")

// EditGuard is a caller-held snapshot of file identity used to detect
// a concurrent modification between read and commit.
type EditGuard struct {
	mtime int64
	size  int64
	sum   [32]byte
}

// NewEditGuard snapshots path (mtime+size+SHA-256 of content).
func NewEditGuard(path string) (*EditGuard, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return &EditGuard{mtime: st.ModTime().UnixNano(), size: st.Size(),
		sum: sha256.Sum256(b)}, nil
}

// verify errors with ErrConcurrent when the file on disk no longer
// matches the snapshot — mtime+size first (cheap), hash after.
func (g *EditGuard) verify(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	if st.ModTime().UnixNano() != g.mtime || st.Size() != g.size {
		return fmt.Errorf("%w: %s (stat)", ErrConcurrent, path)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if sha256.Sum256(b) != g.sum {
		return fmt.Errorf("%w: %s (hash)", ErrConcurrent, path)
	}
	return nil
}

// SetOptions tune ConfSet/TomlSet.
type SetOptions struct {
	Guard   *EditGuard // expected state; nil disables the concurrency check
	History int        // keep the last N versions under <dir>/.hist; 0 disables
}

// histSave copies path into <dir>/.hist/<base>.<unixnano> and prunes
// the directory to the `keep` newest generations.
func histSave(path string, keep int) error {
	dir := filepath.Join(filepath.Dir(path), ".hist")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	base := filepath.Base(path)
	dst := filepath.Join(dir, fmt.Sprintf("%s.%d", base, time.Now().UnixNano()))
	if err := copyFile(path, dst); err != nil {
		return err
	}
	matches, err := filepath.Glob(filepath.Join(dir, base+".*"))
	if err != nil {
		return err
	}
	sort.Strings(matches) // %d unixnano sorts chronologically
	for len(matches) > keep {
		if err := os.Remove(matches[0]); err != nil {
			return err
		}
		matches = matches[1:]
	}
	return nil
}

// atomicCommit runs the shared tail of every set: history snapshot,
// guard re-verification, then fsync+rename through renameio.
func atomicCommit(path string, patched []byte, o *SetOptions) error {
	if o != nil && o.History > 0 {
		if err := histSave(path, o.History); err != nil {
			return fmt.Errorf("history: %w", err)
		}
	}
	if o != nil && o.Guard != nil {
		if err := o.Guard.verify(path); err != nil {
			return err
		}
	}
	return renameio.WriteFile(path, patched, 0o644,
		renameio.WithExistingPermissions())
}

var confKeyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// validSetValue rejects control characters — a literal newline in a
// value corrupts the file and smuggles extra key lines.
func validSetValue(val string) error {
	if strings.ContainsAny(val, "\r\n\x00") {
		return errors.New("value contains control characters")
	}
	return nil
}

// semanticSetCheck runs the schema type check when key is a known
// declarative key; unknown keys are caller's responsibility (they
// land in [extra]/verbatim space anyway).
func semanticSetCheck(key, val string) error {
	base := key
	if i := strings.LastIndex(key, "."); i >= 0 {
		base = key[i+1:]
	}
	if spec, ok := declarativeKeys[base]; ok {
		var v any = val
		if spec.kind == kindFwdList || spec.kind == kindStrList {
			var items []any
			for _, s := range strings.FieldsFunc(val,
				func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
				items = append(items, s)
			}
			v = items
		}
		if msg := checkValue(base, spec, v); msg != "" {
			return errors.New(msg)
		}
	}
	return nil
}

// ConfSet patches one `key="value"` line inside a .conf. Only the
// matched line is rewritten — comments, ordering, permissions and
// every other byte are preserved. List keys render as bash arrays
// (val split on commas).
func ConfSet(path, key, val string, o *SetOptions) error {
	if !confKeyRe.MatchString(key) {
		return fmt.Errorf("invalid conf key %q", key)
	}
	if err := validSetValue(val); err != nil {
		return err
	}
	if err := semanticSetCheck(key, val); err != nil {
		return err
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if o != nil && o.Guard != nil {
		if err := o.Guard.verify(path); err != nil {
			return err
		}
	}
	var newLine string
	if listKeys[key] {
		var parts []string
		for _, t := range strings.Split(val, ",") {
			parts = append(parts, escapeConf(strings.TrimSpace(t)))
		}
		newLine = key + "=(" + strings.Join(parts, " ") + ")"
	} else {
		newLine = key + "=" + escapeConf(val)
	}
	patched := patchConf(src, key, newLine)
	return atomicCommit(path, patched, o)
}

// patchConf replaces the FIRST `key=...` line (keeping any `export `
// prefix and trailing `# comment`) or appends the assignment when the
// key is absent.
func patchConf(src []byte, key, newAssign string) []byte {
	keyRe := regexp.MustCompile(`^(\s*(?:export\s+)?)` + regexp.QuoteMeta(key) + `\s*=\s*`)
	lines := strings.Split(string(src), "\n")
	for i, raw := range lines {
		m := keyRe.FindStringSubmatchIndex(raw)
		if m == nil {
			continue
		}
		rest := raw[m[1]:]
		// Preserve a trailing comment that follows a closed quote or
		// bare value: `"v" # note`, 'v' # note, v # note.
		keep := ""
		if r := strings.TrimSpace(rest); strings.HasPrefix(r, `"`) || strings.HasPrefix(r, `'`) {
			q := r[0]
			j := 1
			for j < len(r) && r[j] != q {
				if q == '"' && r[j] == '\\' {
					j++
				}
				j++
			}
			if j < len(r) {
				if tail := strings.TrimSpace(r[j+1:]); strings.HasPrefix(tail, "#") {
					keep = " " + tail
				}
			}
		} else if idx := strings.Index(rest, "#"); idx >= 0 {
			keep = " " + strings.TrimSpace(rest[idx:])
		}
		prefix := raw[:m[1]]
		lines[i] = prefix + strings.TrimPrefix(newAssign, "export ") + keep
		return []byte(strings.Join(lines, "\n"))
	}
	s := string(src)
	if s != "" && !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	return []byte(s + newAssign + "\n")
}

// TomlSet patches one `key = value` line inside a .toml. The key may
// be dotted to address a table — only [extra] and [runtime] are
// writable sections (schema-defined). List keys render as TOML
// arrays (val split on commas); integer-typed keys with a numeric
// value render bare; everything else renders as a basic string.
func TomlSet(path, key, val string, o *SetOptions) error {
	sec, k := "", key
	if i := strings.LastIndex(key, "."); i >= 0 {
		sec, k = key[:i], key[i+1:]
		if !sectionNames[sec] {
			return fmt.Errorf("section [%s] is not writable (want extra or runtime)", sec)
		}
	}
	if !confKeyRe.MatchString(k) {
		return fmt.Errorf("invalid toml key %q", key)
	}
	if err := validSetValue(val); err != nil {
		return err
	}
	if err := semanticSetCheck(k, val); err != nil {
		return err
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if o != nil && o.Guard != nil {
		if err := o.Guard.verify(path); err != nil {
			return err
		}
	}
	var newLine string
	switch {
	case listKeys[k]:
		var parts []string
		for _, t := range strings.Split(val, ",") {
			parts = append(parts, escapeTOML(strings.TrimSpace(t)))
		}
		newLine = k + " = [" + strings.Join(parts, ", ") + "]"
	case declarativeKeys[k].kind == kindInt || declarativeKeys[k].kind == kindPort:
		if digitsOnly(val) {
			newLine = k + " = " + val
		} else {
			newLine = k + " = " + escapeTOML(val)
		}
	default:
		newLine = k + " = " + escapeTOML(val)
	}
	patched := patchTOML(src, sec, k, newLine)
	// The patch must leave a parseable document — validate syntax
	// before touching the disk.
	var doc map[string]any
	if _, err := toml.Decode(string(patched), &doc); err != nil {
		return fmt.Errorf("patched document would not parse: %w", err)
	}
	return atomicCommit(path, patched, o)
}

// patchTOML rewrites the `k =` line in section `sec` ("" = root).
// Missing keys insert as early as the schema allows: root keys before
// the first table header, section keys at the section tail, and a
// missing section is created at EOF.
func patchTOML(src []byte, sec, key, newLine string) []byte {
	keyRe := regexp.MustCompile(`^\s*` + regexp.QuoteMeta(key) + `\s*=\s*`)
	lines := strings.Split(string(src), "\n")
	inSec := sec == ""
	secStart, secEnd := -1, len(lines)
	// Locate the target section's line range first. Root (sec=="")
	// ends at the first table header.
	if sec != "" {
		inSec = false
		for i, raw := range lines {
			l := strings.TrimSpace(raw)
			if strings.HasPrefix(l, "[") {
				if inSec {
					secEnd = i
					break
				}
				name := l[1:]
				if end := strings.Index(name, "]"); end >= 0 {
					name = strings.TrimSpace(name[:end])
					if name == sec {
						inSec = true
						secStart = i
					}
				}
			}
		}
	} else {
		for i, raw := range lines {
			if strings.HasPrefix(strings.TrimSpace(raw), "[") {
				secEnd = i
				break
			}
		}
	}
	// Patch in place when the key exists in-range.
	lo := 0
	if secStart >= 0 {
		lo = secStart + 1
	}
	for i := lo; i < secEnd; i++ {
		raw := lines[i]
		m := keyRe.FindStringIndex(raw)
		if m == nil {
			continue
		}
		// Keep a trailing comment: find `#` outside strings.
		tail := raw[m[1]:]
		keep := ""
		if idx := cutComment(tail); idx >= 0 {
			if cm := strings.TrimSpace(tail[idx:]); strings.HasPrefix(cm, "#") {
				keep = " " + cm
			}
		}
		indent := raw[:strings.Index(raw, strings.TrimLeft(raw, " \t"))]
		lines[i] = indent + newLine + keep
		return []byte(strings.Join(lines, "\n"))
	}
	// Insert: root keys go before the first table; section keys go to
	// the section tail; a missing section is appended at EOF.
	if sec == "" {
		for i, raw := range lines {
			if strings.HasPrefix(strings.TrimSpace(raw), "[") {
				lines = append(lines[:i], append([]string{newLine}, lines[i:]...)...)
				return []byte(strings.Join(lines, "\n"))
			}
		}
		s := strings.TrimRight(string(src), "\n")
		if s != "" {
			s += "\n"
		}
		return []byte(s + newLine + "\n")
	}
	if secStart >= 0 {
		// Section tail = last non-blank line before secEnd.
		ins := secEnd
		for ins > lo && strings.TrimSpace(lines[ins-1]) == "" {
			ins--
		}
		lines = append(lines[:ins], append([]string{newLine}, lines[ins:]...)...)
		return []byte(strings.Join(lines, "\n"))
	}
	s := strings.TrimRight(string(src), "\n")
	if s != "" {
		s += "\n"
	}
	return []byte(s + "\n[" + sec + "]\n" + newLine + "\n")
}
