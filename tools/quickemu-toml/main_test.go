// main_test.go: run() is exercised in-process through a re-exec of the
// instrumented test binary so coverage counts and exit codes are
// preserved (QE_HELPER_* env carries args in and the rc file out).
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alesanfe/quickemu/tools/qelib/qetest"
)

func TestMain(m *testing.M) {
	if qetest.HelperMain("quickemu-toml", run) {
		return
	}
	os.Exit(m.Run())
}

func runTool(t *testing.T, args ...string) (stdout, stderr string, rc int) {
	return qetest.RunToolFull(t, nil, "", args...)
}

func envelope(t *testing.T, out string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &m); err != nil {
		t.Fatalf("bad envelope %q: %v", out, err)
	}
	return m
}

func writeTOML(t *testing.T, body string) string {
	t.Helper()
	f := filepath.Join(t.TempDir(), "vm.toml")
	if err := os.WriteFile(f, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return f
}

const sampleTOML = "guest_os = \"linux\"\n[display]\ntype = \"sdl\"\nports = [1, 2]\n"

func TestParseFlattensSectionsAndArrays(t *testing.T) {
	f := writeTOML(t, sampleTOML)
	out, _, rc := runTool(t, "parse", "--file", f)
	if rc != 0 {
		t.Fatalf("rc=%d", rc)
	}
	env := envelope(t, out)
	if env["success"] != true {
		t.Fatalf("success=%v", env)
	}
	data := env["data"].(map[string]any)
	if data["guest_os"] != "linux" || data["display.type"] != "sdl" {
		t.Fatalf("flat data=%v", data)
	}
}

func TestValidateCountsKeys(t *testing.T) {
	// sampleTOML has no schema_version (QE-110) — the validator must
	// still parse it, count keys and report the missing-version error.
	f := writeTOML(t, sampleTOML)
	out, _, rc := runTool(t, "validate", "--file", f)
	env := envelope(t, out)
	data := env["data"].(map[string]any)
	if rc != 1 || data["keys"].(float64) != 3 || data["valid"] != false {
		t.Fatalf("rc=%d env=%v", rc, env)
	}
	ds := data["diagnostics"].([]any)
	foundVer := false
	for _, d := range ds {
		if strings.Contains(d.(map[string]any)["message"].(string), "schema_version") {
			foundVer = true
		}
	}
	if !foundVer {
		t.Fatalf("no schema_version diagnostic in %v", ds)
	}
	// A versioned document validates clean.
	g := writeTOML(t, "schema_version = 1\nguest_os = \"linux\"\n")
	out, _, rc = runTool(t, "validate", "--file", g)
	env = envelope(t, out)
	if rc != 0 || env["data"].(map[string]any)["valid"] != true {
		t.Fatalf("rc=%d env=%v", rc, env)
	}
}

func TestGetFoundAndMissing(t *testing.T) {
	f := writeTOML(t, sampleTOML)
	out, _, rc := runTool(t, "get", "--file", f, "--key", "guest_os")
	if rc != 0 || strings.TrimSpace(out) != "linux" {
		t.Fatalf("rc=%d out=%q", rc, out)
	}
	_, _, rc = runTool(t, "get", "--file", f, "--key", "nope")
	if rc != 1 {
		t.Fatalf("missing key rc=%d", rc)
	}
	_, _, rc = runTool(t, "get", "--file", f)
	if rc != 1 {
		t.Fatalf("no --key rc=%d", rc)
	}
}

func TestBadInputs(t *testing.T) {
	if _, _, rc := runTool(t, "parse"); rc != 2 {
		t.Fatalf("usage rc")
	}
	if _, _, rc := runTool(t, "parse", "--file", "/nonexistent"); rc != 1 {
		t.Fatalf("missing file rc")
	}
	f := writeTOML(t, "bad = [unclosed\n")
	if _, _, rc := runTool(t, "parse", "--file", f); rc != 1 {
		t.Fatalf("bad toml rc")
	}
	good := writeTOML(t, "a = 1\n")
	if _, se, rc := runTool(t, "frobnicate", "--file", good); rc != 2 || se == "" {
		t.Fatalf("unknown sub rc=%d", rc)
	}
}

func TestFromConfEmitsOrderedTOML(t *testing.T) {
	conf := filepath.Join(t.TempDir(), "vm.conf")
	os.WriteFile(conf, []byte(
		"# comment\nguest_os=\"windows\"\nssh_port='2222'\nfoo=\"bar\"\n"+
			"display = \"sdl\\\"x\"\nempty=\"\"\n"), 0o600)
	out, _, rc := runTool(t, "from-conf", "--file", conf)
	if rc != 0 {
		t.Fatalf("rc=%d", rc)
	}
	for _, want := range []string{
		"# Migrated", "schema_version = 1",
		`guest_os = "windows"`, `ssh_port = "2222"`, `display = "sdl\"x"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in\n%s", want, out)
		}
	}
	// QE-111: unknown keys are preserved under [extra], empty values
	// migrate too — nothing is silently dropped.
	if !strings.Contains(out, "[extra]") || !strings.Contains(out, `foo = "bar"`) ||
		!strings.Contains(out, `empty = ""`) {
		t.Fatalf("[extra] preservation failed:\n%s", out)
	}
	if _, _, rc := runTool(t, "from-conf", "--file", "/nonexistent"); rc != 1 {
		t.Fatalf("rc")
	}
}
