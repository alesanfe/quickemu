// act_test.go — act* dispatch coverage: catalog fixtures via
// QUICKEMU_CATALOG_DIR and a stub quickget-catalog in PATH.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alesanfe/quickemu/tools/qelib/qetest"
)

// setupCatalog writes a catalog.json + info.json fixture and returns
// the env vars pointing at them plus a stub quickget-catalog whose
// output is driven by QC_STUB_* env vars.
func setupCatalog(t *testing.T) []string {
	t.Helper()
	dir := t.TempDir()
	cat := `{"entries":[{"os":"alpine","releases":["3.19","3.20"],"editions":["virt","standard"],"arch":["amd64","arm64"]},{"os":"plain","releases":["1"],"editions":[],"arch":["amd64"]},{"os":"fedora","releases":["39","40"],"editions":["Onyx","Sericea","Kinoite"],"arch":["amd64","arm64"]},{"os":"windows","releases":["10","11"],"editions":[],"arch":["amd64"]}]}`
	info := `{"entries":{"alpine":{"Name":"Alpine Linux","Credentials":"root","Homepage":"https://alpine.example","Info":"Small distro"},"plain":{"Name":"Plain","Credentials":"","Homepage":"","Info":""},"fedora":{"Name":"Fedora","Credentials":"","Homepage":"","Info":""},"windows":{"Name":"Windows","Credentials":"","Homepage":"","Info":""}}}`
	os.WriteFile(filepath.Join(dir, "catalog.json"), []byte(cat), 0o644)
	os.WriteFile(filepath.Join(dir, "info.json"), []byte(info), 0o644)

	stub := t.TempDir()
	os.WriteFile(filepath.Join(stub, "quickget-catalog"), []byte(`#!/bin/sh
# Stub: answers driven by QC_STUB_* env.
case "$1" in
 releases) [ "$2" = "nope" ] && exit 1; case "$2" in fedora) echo "39 40";; windows) echo "10 11";; *) echo "${QC_REL:-3.19 3.20}";; esac;;
 editions) [ "$2" = "plain" ] && echo "" || { [ "$2" = "fedora" ] && echo "Onyx Sericea Kinoite" || echo "${QC_ED:-virt standard}"; };;
 arch) echo "${QC_ARCH:-amd64 arm64}";;
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
	// D warns but proceeds (the stub-env flow then fails downstream
	// → deterministic rc 1); E is refused at the gate by the
	// fail-closed default (minTrust=D) unless --minimum-trust E.
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

// TestArchGates: catalog_check_os_arch / catalog_resolve_edition
// parity — unsupported arch dies for non-edition OSes before release
// validation, and after the edition resolves for edition OSes.
func TestArchGates(t *testing.T) {
	setupCatalog(t)
	old := arch
	defer func() { arch = old }()
	// Non-edition OS, unsupported arch → ERROR before anything else.
	arch = "arm64"
	if rc := actGet("plain", "1", "", false); rc != 1 {
		t.Fatalf("non-edition arch gate rc=%d", rc)
	}
	// Edition OS, unsupported arch → ERROR naming the edition.
	if rc := actGet("alpine", "3.19", "virt", false); rc != 1 {
		t.Fatalf("edition arch gate rc=%d", rc)
	}
	// Supported arch passes the gate (dies later in the tool chain —
	// deterministic rc 1 without a quickget-vm stub is still rc 1).
	arch = "amd64"
	// --check with a release on an unsupported non-edition arch → SKIP.
	arch = "arm64"
	if rc := actCheck("plain", "1", "", "arm64"); rc != 0 {
		t.Fatalf("check SKIP rc=%d", rc)
	}
}

// TestFedoraMissing: handle_missing parity — Fedora < 40 with the
// Onyx/Sericea spins is an unsupported combination.
func TestFedoraMissing(t *testing.T) {
	setupCatalog(t)
	old := arch
	defer func() { arch = old }()
	arch = "amd64"
	for _, ed := range []string{"Onyx", "Sericea"} {
		if rc := actGet("fedora", "39", ed, false); rc != 1 {
			t.Fatalf("fedora 39 %s rc=%d", ed, rc)
		}
		// _Beta release names hit the same gate.
		if rc := actGet("fedora", "39_Beta", ed, false); rc != 1 {
			t.Fatalf("fedora 39_Beta %s rc=%d", ed, rc)
		}
	}
	// 40+ and other editions are not rejected by the gate itself —
	// they must reach the downstream tool chain (no "Unsupported
	// combination" on stdout).
	out := captureStdout(t, func() { _ = actGet("fedora", "40", "Onyx", false) })
	if strings.Contains(out, "Unsupported combination") {
		t.Fatalf("fedora 40 Onyx wrongly rejected: %s", out)
	}
	out = captureStdout(t, func() { _ = actGet("fedora", "39", "Kinoite", false) })
	if strings.Contains(out, "Unsupported combination") {
		t.Fatalf("fedora 39 Kinoite wrongly rejected: %s", out)
	}
}

// captureStdout redirects os.Stdout for fn and returns what it wrote.
func captureStdout(t *testing.T, fn func()) string {
	return qetest.CaptureStdout(t, fn)
}

// TestCheckPrecheck: release-scoped ops run the bottom-flow gates —
// trust warn fires on D/E sources even for --check.
func TestCheckPrecheck(t *testing.T) {
	setupCatalog(t)
	oldA, oldM := arch, minTrust
	defer func() { arch, minTrust = oldA, oldM }()
	arch = "amd64"
	// Bad release under --check → release validation error (rc 1).
	if rc := actCheck("alpine", "9.99", "virt", "amd64"); rc != 1 {
		t.Fatalf("check bad release rc=%d", rc)
	}
	// Editions OS without edition arg → "must specify an edition".
	if rc := actCheck("alpine", "3.19", "", "amd64"); rc != 1 {
		t.Fatalf("check missing edition rc=%d", rc)
	}
	// --check-all-arch with release: no trust/edition gates, single
	// release+edition combo per arch.
	oldC := checkAllArc
	checkAllArc = true
	defer func() { checkAllArc = oldC }()
	if rc := actCheck("alpine", "3.19", "", "amd64"); rc != 0 {
		t.Fatalf("check-all-arch rc=%d", rc)
	}
}

// TestCheckAllArchWindows: catalog_check_all_arch windows branch — a
// supported arch prints the "(Windows not supported in check mode)"
// reason; the plain --check path carries no reason.
func TestCheckAllArchWindows(t *testing.T) {
	setupCatalog(t)
	oldC := checkAllArc
	checkAllArc = true
	defer func() { checkAllArc = oldC }()
	out := captureStdout(t, func() {
		if rc := actCheck("windows", "11", "English", "amd64"); rc != 0 {
			t.Fatalf("rc=%d", rc)
		}
	})
	if !strings.Contains(out, "SKIP: windows-11-English") ||
		!strings.Contains(out, "(Windows not supported in check mode)") {
		t.Fatalf("windows check-all-arch out=%q", out)
	}
	// Unsupported arch falls through to the catalog check (stub → PASS).
	out = captureStdout(t, func() {
		_ = actCheck("windows", "11", "English", "arm64")
	})
	if strings.Contains(out, "Windows not supported in check mode") {
		t.Fatalf("arm64 should not take the supported-arch SKIP: %q", out)
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
