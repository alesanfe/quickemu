package qespec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---- spec/functional/quickget_entry_spec.sh ----
// Real bin/quickget entry; hermetic except the --check probes which
// stay network-dependent (pinned to one release).

func qgSetup(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	WithFakeBin(t) // qemu-img + xdg-open stubs
	return dir
}

func qg(t *testing.T, dir string, args ...string) Result {
	t.Helper()
	return Run(t, "quickget", []RunOpt{InDir(dir)}, args...)
}

func TestQuickgetInformational(t *testing.T) {
	d := qgSetup(t)
	qg(t, d, "--version").Contains(t, ".")
	qg(t, d, "--help").Contains(t, "quickget")
	qg(t, d, "--show", "alpine").Contains(t, "Alpine Linux", "alpinelinux.org")
	qg(t, d, "--show").Contains(t, "Alpine Linux", "Zorin")
	if r := qg(t, d, "--open-homepage", "alpine"); r.Failed() {
		t.Fatalf("open-homepage rc=%d", r.Code)
	}
	r := qg(t, d, "--open-homepage", "nosuchos")
	if r.OK() {
		t.Error("unknown OS open-homepage succeeded")
	}
	r.Contains(t, "must specify an operating system")
}

func TestQuickgetListings(t *testing.T) {
	d := qgSetup(t)
	qg(t, d, "--list").Contains(t, "alma")
	qg(t, d, "--list-csv").Contains(t, "Display Name,OS,Release")
	qg(t, d, "--list-json").Contains(t, `"OS":`)
	qg(t, d, "list").Contains(t, "alma")
}

func TestQuickgetURLOps(t *testing.T) {
	d := qgSetup(t)
	qg(t, d, "--url", "alma").Contains(t, "alma-")
	if testing.Short() {
		t.Skip("network-dependent --check probes")
	}
	qg(t, d, "--check", "alma", "9", "minimal").Contains(t, "alma-")
	qg(t, d, "--arch", "arm64", "--check", "alma", "9", "minimal").Contains(t, "alma-9")
	qg(t, d, "--check-all-arch", "alma", "9", "minimal").Contains(t, "alma-")
}

func TestQuickgetValidation(t *testing.T) {
	d := qgSetup(t)

	r := qg(t, d, "--bogus")
	if r.OK() {
		t.Error("--bogus succeeded")
	}
	r.Contains(t, "Not supported argument")

	r = qg(t, d, "--arch", "riscv", "alpine")
	if r.OK() {
		t.Error("riscv accepted")
	}
	r.Contains(t, "Unsupported architecture")

	r = qg(t, d, "nosuchos", "1")
	if r.OK() {
		t.Error("unknown OS accepted")
	}
	r.Contains(t, "not a supported OS")

	r = qg(t, d, "alma")
	if r.OK() {
		t.Error("missing release accepted")
	}
	r.Contains(t, "must specify a release", "AlmaLinux")

	r = qg(t, d, "alma", "7")
	if r.OK() {
		t.Error("bad release accepted")
	}
	r.Contains(t, "not a supported release")

	r = qg(t, d, "alma", "9")
	if r.OK() {
		t.Error("missing edition accepted")
	}
	r.Contains(t, "must specify an edition")

	r = qg(t, d, "alma", "9", "bogus")
	if r.OK() {
		t.Error("bad edition accepted")
	}
	r.Contains(t, "not a supported")

	r = qg(t, d, "debian", "9999")
	if r.OK() {
		t.Error("bad debian release accepted")
	}
	r.ErrContains(t, "integrity not verifiable")
	r.Contains(t, "not a supported release")
}

func TestQuickgetCreateConfig(t *testing.T) {
	d := qgSetup(t)

	Write(t, d, "my-linux.iso", "iso-bytes")
	r := qg(t, d, "--create-config", "custom", filepath.Join(d, "my-linux.iso"))
	r.Contains(t, "custom.conf")
	if _, err := os.Stat(filepath.Join(d, "custom.conf")); err != nil {
		t.Error("custom.conf not created")
	}
	if b, _ := os.ReadFile(filepath.Join(d, "custom.conf")); !strings.Contains(string(b), `guest_os="linux"`) {
		t.Error("conf missing guest_os")
	}

	Write(t, d, "random.iso", "iso-bytes")
	qg(t, d, "--create-config", "winvm", filepath.Join(d, "random.iso"),
		"--disable-unattended").Contains(t, "Selecting OS")

	r = qg(t, d, "--create-config", "newvm", filepath.Join(d, "gone.iso"))
	if r.OK() {
		t.Error("missing image accepted")
	}
	r.Contains(t, "valid URL or path")

	os.MkdirAll(filepath.Join(d, "existing"), 0o755)
	Write(t, d, "a.iso", "x")
	r = qg(t, d, "--create-config", "existing", filepath.Join(d, "a.iso"))
	if r.OK() {
		t.Error("existing dir accepted")
	}
	r.Contains(t, "Could not create directory")

	Write(t, d, "readme.txt", "x")
	r = qg(t, d, "--create-config", "txtvm", filepath.Join(d, "readme.txt"))
	if r.OK() {
		t.Error(".txt accepted")
	}
	r.Contains(t, "ISO,IMG and QCOW2")
}

func TestQuickgetDownloadFlow(t *testing.T) {
	d := qgSetup(t)
	www := t.TempDir()
	Write(t, www, "tiny.iso", "iso-bytes")
	srv := HTTPServer(t, www)

	r := qg(t, d, "--create-config", "dlvm", srv.URL+"/tiny.iso")
	if r.Failed() {
		t.Fatalf("download create-config rc=%d: %s", r.Code, r.Combined())
	}
	for _, p := range []string{"dlvm.conf", "dlvm/tiny.iso"} {
		if _, err := os.Stat(filepath.Join(d, p)); err != nil {
			t.Errorf("missing %s", p)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(d, "dlvm.conf")); !strings.Contains(string(b), `guest_os="linux"`) {
		t.Error("conf missing guest_os")
	}
}
