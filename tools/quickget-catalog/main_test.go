// main_test.go: run() re-exec against a fixture catalog pointed to by
// QUICKEMU_CATALOG/QUICKEMU_INFO — every non-network subcommand is
// exercised (scrape paths need live web and stay in e2e).
package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("QE_HELPER") == "1" {
		a := os.Getenv("QE_HELPER_ARGS")
		os.Args = []string{"quickget-catalog"}
		if a != "" {
			os.Args = append(os.Args, strings.Split(a, "\x1f")...)
		}
		rc := run()
		_ = os.WriteFile(os.Getenv("QE_HELPER_RC"),
			[]byte(strconv.Itoa(rc)), 0o600)
		return
	}
	os.Exit(m.Run())
}

var catEnv []string

func runTool(t *testing.T, args ...string) (string, int) {
	t.Helper()
	rcf := filepath.Join(t.TempDir(), "rc")
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "QE_HELPER=1",
		"QE_HELPER_ARGS="+strings.Join(args, "\x1f"),
		"QE_HELPER_RC="+rcf)
	cmd.Env = append(cmd.Env, catEnv...)
	var so bytes.Buffer
	cmd.Stdout = &so
	_ = cmd.Run()
	rc := -1
	if b, err := os.ReadFile(rcf); err == nil {
		rc, _ = strconv.Atoi(strings.TrimSpace(string(b)))
	}
	return so.String(), rc
}

func TestFixtureSetup(t *testing.T) {
	dir := t.TempDir()
	cat := `{"entries":[
	 {"os":"alpine","releases":["3.19","3.20"],"editions":["virt","standard"],
	  "arch":["amd64","aarch64"],"url":"https://cdn.alpine.org/x.iso","hash":"abc",
	  "scrape":false,"status":"active","image_type":"iso",
	  "download_mode":"direct","license":"redistributable","boot_mode":"both",
	  "support_level":"stable"},
	 {"os":"tiny","releases":[],"editions":[],"arch":[],
	  "url":"http://mirror.example/t.iso","scrape":false}]}`
	os.WriteFile(filepath.Join(dir, "catalog.json"), []byte(cat), 0o600)
	info := `{"entries":{"alpine":{"name":"Alpine Linux","credentials":"u:p",
	 "homepage":"https://alpinelinux.org","info":"mini"},"tiny":{}}}`
	os.WriteFile(filepath.Join(dir, "info.json"), []byte(info), 0o600)
	catEnv = []string{
		"QUICKEMU_CATALOG=" + filepath.Join(dir, "catalog.json"),
		"QUICKEMU_INFO=" + filepath.Join(dir, "info.json"),
	}

	out, rc := runTool(t, "list")
	if rc != 0 || !strings.Contains(out, "alpine") || !strings.Contains(out, "tiny") {
		t.Fatalf("list rc=%d %s", rc, out)
	}
	out, rc = runTool(t, "releases", "alpine")
	if rc != 0 || strings.TrimSpace(out) != "3.19 3.20" {
		t.Fatalf("releases rc=%d %q", rc, out)
	}
	out, rc = runTool(t, "editions", "alpine")
	if rc != 0 || strings.TrimSpace(out) != "virt standard" {
		t.Fatalf("editions rc=%d %q", rc, out)
	}
	out, rc = runTool(t, "arch", "alpine")
	if rc != 0 || strings.TrimSpace(out) != "amd64 aarch64" {
		t.Fatalf("arch rc=%d %q", rc, out)
	}
	out, rc = runTool(t, "arch", "tiny") // empty → amd64 default
	if rc != 0 || strings.TrimSpace(out) != "amd64" {
		t.Fatalf("arch default %q", out)
	}
	out, rc = runTool(t, "url", "alpine", "3.20")
	if rc != 0 || !strings.Contains(out, "x.iso") {
		t.Fatalf("url rc=%d %s", rc, out)
	}
	// trust grades
	out, rc = runTool(t, "trust", "alpine")
	if rc != 0 || strings.TrimSpace(out) != "C" {
		t.Fatalf("trust https %q", out)
	}
	out, _ = runTool(t, "trust", "tiny")
	if strings.TrimSpace(out) != "E" {
		t.Fatalf("trust http %q", out)
	}
	out, _ = runTool(t, "trust", "nope")
	if strings.TrimSpace(out) != "E" {
		t.Fatalf("trust missing %q", out)
	}
	// info
	out, rc = runTool(t, "info", "alpine")
	if rc != 0 || !strings.Contains(out, "Alpine Linux|u:p|") {
		t.Fatalf("info rc=%d %s", rc, out)
	}
	out, rc = runTool(t, "info-json", "alpine")
	if rc != 0 || !strings.Contains(out, `"name":"Alpine Linux"`) {
		t.Fatalf("info-json %s", out)
	}
	// meta: per-os view and all-entries map
	out, rc = runTool(t, "meta", "alpine")
	if rc != 0 || !strings.Contains(out, `"status":"active"`) ||
		!strings.Contains(out, `"image_type":"iso"`) {
		t.Fatalf("meta alpine rc=%d %s", rc, out)
	}
	out, rc = runTool(t, "meta")
	if rc != 0 || !strings.Contains(out, `"alpine"`) ||
		!strings.Contains(out, `"tiny"`) {
		t.Fatalf("meta all rc=%d %s", rc, out)
	}
	if _, rc = runTool(t, "meta", "nope"); rc != 1 {
		t.Fatalf("meta missing rc=%d", rc)
	}
	// check: unsupported arch short-circuits as SKIP without network.
	out, rc = runTool(t, "check", "alpine", "3.20", "virt", "riscv64")
	if rc != 0 || !strings.Contains(out, "SKIP") {
		t.Fatalf("check skip rc=%d %s", rc, out)
	}
	// unknown os / usage / unknown sub
	if _, rc = runTool(t, "releases", "nope"); rc != 1 {
		t.Fatalf("missing os rc=%d", rc)
	}
	if _, rc = runTool(t); rc != 2 {
		t.Fatalf("usage rc=%d", rc)
	}
	if _, rc = runTool(t, "bogus"); rc != 2 {
		t.Fatalf("unknown rc=%d", rc)
	}
}
