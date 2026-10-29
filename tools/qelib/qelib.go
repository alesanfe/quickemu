// Package qelib: helpers shared by the quickemu Go tools — the
// canonical v1 JSON envelope (schemas/v1/response.schema.json),
// --flag=value argument normalization, EPERM-aware process liveness
// and sibling-tool resolution.
package qelib

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// QEVersion is the product version reported in meta.quickemuVersion.
const QEVersion = "5.0.0"

// NormArgs expands "--flag=value" into "--flag value" so flag.Parse
// accepts both spellings. The output is a fresh slice — writing into
// a[:0] would clobber unread elements.
func NormArgs(a []string) []string {
	out := make([]string, 0, len(a))
	for _, s := range a {
		if strings.HasPrefix(s, "--") {
			if i := strings.IndexByte(s, '='); i > 0 {
				out = append(out, s[:i], s[i+1:])
				continue
			}
		}
		out = append(out, s)
	}
	return out
}

// Alive reports whether pid is a live process. EPERM counts as alive:
// a process owned by another user exists even though we cannot signal
// it — treating it as dead would let locks be stolen.
func Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// QEError is one errors[] element of the v1 envelope.
type QEError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Severity  string `json:"severity"`
	Retryable bool   `json:"retryable"`
}

// QMPCmd marshals a QMP {"execute":...} command object; arguments is
// omitted when nil.
func QMPCmd(execute string, args map[string]any) string {
	m := map[string]any{"execute": execute}
	if args != nil {
		m["arguments"] = args
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// CmdName derives the envelope command field: "<tool>.<action>" from
// argv[1] (post-NormArgs), falling back to the bare tool name.
func CmdName(tool string) string {
	if len(os.Args) > 1 {
		return tool + "." + os.Args[1]
	}
	return tool
}

// Emit prints the canonical v1 envelope: {schemaVersion, command,
// success, data, warnings, errors, meta{quickemuVersion, timestamp,
// operationId?}}.
func Emit(cmd, tool string, success bool, data any, errs any) {
	if errs == nil {
		errs = []any{}
	}
	meta := map[string]any{
		"quickemuVersion": QEVersion,
		"tool":            tool,
		"timestamp":       time.Now().UTC().Format("2006-01-02T15:04:05Z"),
	}
	if op := os.Getenv("OPID"); op != "" {
		meta["operationId"] = op
	} else if op := os.Getenv("OP_ID"); op != "" {
		meta["operationId"] = op
	}
	b, _ := json.Marshal(map[string]any{
		"schemaVersion": 1, "command": cmd, "success": success,
		"data": data, "warnings": []any{}, "errors": errs, "meta": meta,
	})
	fmt.Println(string(b))
}

// EmitOK prints a success envelope.
func EmitOK(cmd, tool string, data any) { Emit(cmd, tool, true, data, []any{}) }

// Fail prints an error envelope with one error[] entry and exits 1.
func Fail(cmd, tool, code, msg string) {
	Emit(cmd, tool, false, nil, []QEError{
		{Code: code, Message: msg, Severity: "error", Retryable: false}})
	os.Exit(1)
}

// selfExe is the executable seam — tests stub it so sibling
// resolution points at a fixture tree.
var selfExe = os.Executable

// FindTool resolves a sibling tool binary: next to this executable
// FIRST (packaged libexec/ + dev-tree layouts — a sibling is the
// version-matched tool this build was shipped with), then ../<name>/
// <name> dev dirs, then PATH as the last resort. PATH first would let
// a stale or hostile earlier-PATH binary impersonate the backend.
func FindTool(name string) (string, error) {
	if exe, err := selfExe(); err == nil {
		dir := filepath.Dir(exe)
		for _, p := range []string{
			filepath.Join(dir, name),
			filepath.Join(dir, "..", name, name),
			// Packaged: <prefix>/bin/<entry> -> <prefix>/libexec/quickemu/<name>
			filepath.Join(dir, "..", "libexec", "quickemu", name),
			filepath.Join(dir, "..", "lib", "quickemu", name),
			// Dev entries: bin/<entry> -> tools/<name>/<name>
			filepath.Join(dir, "..", "tools", name, name),
		} {
			if st, err := os.Stat(p); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
				return p, nil
			}
		}
	}
	if bin, err := exec.LookPath(name); err == nil {
		return bin, nil
	}
	return "", fmt.Errorf("%s: not on PATH", name)
}

// Tool runs a sibling quickemu-* binary, capturing stdout for the
// caller while stderr passes straight through.
func Tool(name string, env []string, args ...string) (string, error) {
	bin, err := FindTool(name)
	if err != nil {
		return "", err
	}
	cmd := execCommand(bin, args...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	return buf.String(), err
}

// DataFile resolves a packaged/dev-tree data file (e.g. catalog/data/
// catalog.json) for every layout: dev bin/<entry>, dev tools/<x>/<x>,
// packaged <prefix>/bin and <prefix>/libexec/quickemu — with data under
// either <root>/catalog or <prefix>/share/quickemu.
// QUICKEMU_DATA_DIR overrides the search root.
func DataFile(rel ...string) string {
	if p := os.Getenv("QUICKEMU_DATA_DIR"); p != "" {
		return filepath.Join(append([]string{p}, rel...)...)
	}
	exe, err := os.Executable()
	if err != nil {
		return filepath.Join(rel...)
	}
	dir := filepath.Dir(exe)
	base := dir
	for up := 0; up <= 3; up++ {
		for _, pre := range []string{base, filepath.Join(base, "share", "quickemu")} {
			p := filepath.Join(append([]string{pre}, rel...)...)
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p
			}
		}
		base = filepath.Dir(base)
	}
	// Old dev-tree guess so error messages stay sensible.
	return filepath.Join(append([]string{
		filepath.Dir(filepath.Dir(filepath.Dir(exe)))}, rel...)...)
}
