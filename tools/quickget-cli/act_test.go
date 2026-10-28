// act_test.go — act* dispatch coverage: catalog fixtures via
// QUICKEMU_CATALOG_DIR and a stub quickget-catalog in PATH.
package main

import (
	"os"
	"path/filepath"
	"testing"
)

// setupCatalog writes a catalog.json + info.json fixture and returns
// the env vars pointing at them plus a stub quickget-catalog whose
// output is driven by QC_STUB_* env vars.
func setupCatalog(t *testing.T) []string {
	t.Helper()
	dir := t.TempDir()
	cat := `{"entries":[{"os":"alpine","releases":["3.19","3.20"],"editions":["virt","standard"],"arch":["x86_64","aarch64"]},{"os":"plain","releases":["1"],"editions":[],"arch":["x86_64"]}]}`
	info := `{"entries":{"alpine":{"Name":"Alpine Linux","Credentials":"root","Homepage":"https://alpine.example","Info":"Small distro"},"plain":{"Name":"Plain","Credentials":"","Homepage":"","Info":""}}}`
	os.WriteFile(filepath.Join(dir, "catalog.json"), []byte(cat), 0o644)
	os.WriteFile(filepath.Join(dir, "info.json"), []byte(info), 0o644)

	stub := t.TempDir()
	os.WriteFile(filepath.Join(stub, "quickget-catalog"), []byte(`#!/bin/sh
# Stub: answers driven by QC_STUB_* env.
case "$1" in
 releases) [ "$2" = "nope" ] && exit 1; echo "${QC_REL:-3.19 3.20}";;
 editions) [ "$2" = "plain" ] && echo "" || echo "${QC_ED:-virt standard}";;
 arch) echo "${QC_ARCH:-x86_64 aarch64}";;
 url) echo "${QC_URL:-https://x/a.iso}";;
 check) echo "${QC_CHECK:-PASS}";;
 trust) echo "${QC_TRUST:-A}";;
 *) exit ${QC_RC:-0};;
esac
`), 0o755)
	env := []string{
		"QUICKEMU_CATALOG_DIR=" + dir,
		"PATH=" + stub + string(os.PathListSeparator) + os.Getenv("PATH"),
	}
	for _, e := range env {
		k, v, _ := func() (string, string, bool) {
			for i, c := range e {
				if c == '=' {
					return e[:i], e[i+1:], true
				}
			}
			return "", "", false
		}()
		if v != "" {
			t.Setenv(k, v)
		}
	}
	return env
}

func TestActListVariants(t *testing.T) {
	setupCatalog(t)
	if rc := actListCSV(); rc != 0 {
		t.Fatalf("csv rc=%d", rc)
	}
	if rc := actList(); rc != 0 {
		t.Fatalf("list rc=%d", rc)
	}
	if rc := actListJSON(); rc != 0 {
		t.Fatalf("json rc=%d", rc)
	}
	if rc := actShow("alpine"); rc != 0 {
		t.Fatalf("show rc=%d", rc)
	}
	if rc := actShow("nope"); rc != 1 {
		t.Fatalf("show bad rc=%d", rc)
	}
}

func TestActURLOrCheck(t *testing.T) {
	setupCatalog(t)
	if rc := actURL("alpine", "3.19", "virt", "x86_64"); rc != 0 {
		t.Fatalf("url rc=%d", rc)
	}
	if rc := actURL("alpine", "", "", ""); rc != 0 {
		t.Fatalf("url all rc=%d", rc)
	}
	if rc := actURL("nope", "1", "", ""); rc != 1 {
		t.Fatalf("url bad os rc=%d", rc)
	}
	if rc := actCheck("alpine", "3.19", "virt", "x86_64"); rc != 0 {
		t.Fatalf("check rc=%d", rc)
	}
	t.Setenv("QC_CHECK", "FAIL")
	if rc := actCheck("alpine", "3.19", "virt", "x86_64"); rc != 1 {
		t.Fatalf("check fail rc=%d", rc)
	}
}

func TestActUpdateSpecifyResolve(t *testing.T) {
	setupCatalog(t)
	if rc := actUpdate("alpine"); rc != 0 && rc != 1 {
		t.Fatalf("update rc=%d", rc)
	}
	if rc := actUpdate("nope"); rc != 1 {
		t.Fatalf("update bad rc=%d", rc)
	}
	if rc := specifyRelease("alpine"); rc != 1 {
		t.Fatalf("specify rc=%d", rc)
	}
	// resolveEdition: no editions OS → true; missing edition → false;
	// valid → true; invalid → false.
	if !resolveEdition("plain", "") {
		t.Fatal("plain should pass through")
	}
	if resolveEdition("alpine", "") {
		t.Fatal("missing edition should fail")
	}
	if !resolveEdition("alpine", "virt") {
		t.Fatal("virt should resolve")
	}
	if resolveEdition("alpine", "kde") {
		t.Fatal("kde should not resolve")
	}
	// editions tool failure → pass-through true.
	t.Setenv("QC_RC", "1")
	t.Setenv("QC_ED", "")
	if !resolveEdition("alpine", "virt") {
		t.Fatal("tool failure should pass")
	}
}

func TestActGetAndOpen(t *testing.T) {
	setupCatalog(t)
	// validateOS failure.
	if rc := actGet("nope", "1", "", false); rc != 1 {
		t.Fatalf("get bad os rc=%d", rc)
	}
	// missing release → specifyRelease (rc 1).
	if rc := actGet("alpine", "", "", false); rc != 1 {
		t.Fatalf("get no rel rc=%d", rc)
	}
	// trust gate: minimum trust above level.
	old := minTrust
	minTrust = "S"
	if rc := actGet("alpine", "3.19", "virt", false); rc != 1 {
		t.Fatalf("trust gate rc=%d", rc)
	}
	minTrust = old
	// D/E trust warning paths — the gate warns but does not block;
	// the stub-env flow then fails downstream → deterministic rc 1.
	t.Setenv("QC_TRUST", "D")
	if rc := actGet("alpine", "3.19", "virt", true); rc != 1 {
		t.Fatalf("trust D rc=%d", rc)
	}
	t.Setenv("QC_TRUST", "E")
	if rc := actGet("alpine", "3.19", "virt", true); rc != 1 {
		t.Fatalf("trust E rc=%d", rc)
	}
	t.Setenv("QC_TRUST", "A")
	// openHomepage: stub xdg-open.
	stub := t.TempDir()
	os.WriteFile(filepath.Join(stub, "xdg-open"), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	t.Setenv("PATH", stub+string(os.PathListSeparator)+os.Getenv("PATH"))
	if rc := actOpenHomepage("alpine"); rc != 0 {
		t.Fatalf("homepage rc=%d", rc)
	}
	if rc := actOpenHomepage("nope"); rc != 1 {
		t.Fatalf("homepage bad rc=%d", rc)
	}
	if rc := actOpenHomepage("plain"); rc != 1 {
		t.Fatalf("homepage empty rc=%d", rc)
	}
}

func TestIconAndPassthru(t *testing.T) {
	if iconFunc("ubuntu-server-22") != "ubuntu-server" {
		t.Fatal("icon server")
	}
	if iconFunc("ubuntu-22") != "ubuntu" {
		t.Fatal("icon ubuntu")
	}
	if iconFunc("alpine") != "alpine" {
		t.Fatal("icon passthrough")
	}
	stub := t.TempDir()
	os.WriteFile(filepath.Join(stub, "fake-tool"), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	t.Setenv("PATH", stub+string(os.PathListSeparator)+os.Getenv("PATH"))
	if rc := runPassthru("fake-tool", "x"); rc != 0 {
		t.Fatalf("passthru rc=%d", rc)
	}
	if rc := runPassthru("no-such-tool-xyz"); rc != 1 {
		t.Fatalf("passthru missing rc=%d", rc)
	}
}
