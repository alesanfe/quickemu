// main2_test.go — run() dispatch paths via the QE_HELPER re-exec
// (guarded flag + -test.run=^$), exercising list/show/install/usage
// against a MARKET_CATALOG fixture.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alesanfe/quickemu/tools/qelib/qetest"
)

func TestMain(m *testing.M) {
	if qetest.HelperMain("quickemu-plugin-market", run) {
		return
	}
	os.Exit(m.Run())
}

var marketEnv []string

func runTool(t *testing.T, args ...string) (string, int) {
	so, se, rc := qetest.RunToolFull(t, marketEnv, "", args...)
	return so + se, rc
}

func TestMarketDispatch(t *testing.T) {
	dir := t.TempDir()
	cat := filepath.Join(dir, "market.json")
	os.WriteFile(cat, []byte(`{"templates":[
	  {"name":"demo","conf":"guest_os=\"linux\"\n\n\n"},
	  {"name":"other","conf":"x=1\n"}]}`), 0o600)
	marketEnv = []string{"MARKET_CATALOG=" + cat}

	out, rc := runTool(t, "list")
	if rc != 0 || !strings.Contains(out, "demo") {
		t.Fatalf("list rc=%d %s", rc, out)
	}
	out, rc = runTool(t, "show", "demo")
	if rc != 0 || !strings.Contains(out, `"name":"demo"`) {
		t.Fatalf("show rc=%d %s", rc, out)
	}
	if _, rc = runTool(t, "show", "missing"); rc != 1 {
		t.Fatalf("show missing rc=%d", rc)
	}
	if _, rc = runTool(t, "show"); rc != 2 {
		t.Fatalf("show usage rc=%d", rc)
	}
	// install writes a normalized conf (single trailing newline).
	out, rc = runTool(t, "install", "demo", dir)
	if rc != 0 || !strings.Contains(out, "installed") {
		t.Fatalf("install rc=%d %s", rc, out)
	}
	b, err := os.ReadFile(filepath.Join(dir, "demo.conf"))
	if err != nil || string(b) != "guest_os=\"linux\"\n" {
		t.Fatalf("conf %q", b)
	}
	if _, rc = runTool(t, "install", "missing", dir); rc != 1 {
		t.Fatalf("install missing rc=%d", rc)
	}
	if _, rc = runTool(t, "install"); rc != 2 {
		t.Fatalf("install usage rc=%d", rc)
	}
	if _, rc = runTool(t); rc != 2 {
		t.Fatalf("usage rc=%d", rc)
	}
	if _, rc = runTool(t, "bogus"); rc != 2 {
		t.Fatalf("unknown rc=%d", rc)
	}
}
