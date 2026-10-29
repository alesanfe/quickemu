package qespec

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// ---- spec/integration/cli/cli_spec.sh ----

func TestCLIDelegation(t *testing.T) {
	WithFakeBin(t)
	Run(t, "quickemu", nil, "--capabilities").Contains(t, `"command":"capabilities"`)
	Run(t, "quickemu", nil, "--help").Contains(t, "--vm")
}

// tarGz writes a gzipped tar of the given {relpath:content} entries.
func tarGz(t *testing.T, path string, entries map[string][]byte) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()
	for name, body := range entries {
		if err := tw.WriteHeader(&tar.Header{
			Name: name, Mode: 0o644, Size: int64(len(body)),
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	_ = io.EOF
}

func TestImportWithoutConf(t *testing.T) {
	d := t.TempDir()
	src := filepath.Join(d, "src")
	arc := filepath.Join(d, "vm-new.tar.gz")
	os.MkdirAll(filepath.Join(src, "vm-new"), 0o755)
	tarGz(t, arc, map[string][]byte{
		"vm-new.conf":       []byte("guest_os=linux\n"),
		"vm-new/disk.qcow2": {},
	})
	dst := filepath.Join(d, "dest")
	os.MkdirAll(dst, 0o755)
	r := Run(t, "quickemu", []RunOpt{InDir(dst)}, "--import", arc)
	if r.Failed() {
		t.Fatalf("import rc=%d\n%s", r.Code, r.Combined())
	}
	r.Contains(t, "Imported vm-new")
	for _, p := range []string{"vm-new.conf", "vm-new"} {
		if _, err := os.Stat(filepath.Join(dst, p)); err != nil {
			t.Errorf("missing %s after import", p)
		}
	}
}

func TestDeleteRunningVMRefused(t *testing.T) {
	d := t.TempDir()
	os.MkdirAll(filepath.Join(d, "victim"), 0o755)
	Write(t, d, "victim.conf",
		"guest_os=\"linux\"\ndisk_img=\"victim/disk.qcow2\"\n")
	Write(t, filepath.Join(d, "victim"), "disk.qcow2", "")
	SpawnFakeVM(t, filepath.Join(d, "victim"), "victim")

	r := Run(t, "quickemu", []RunOpt{InDir(d)}, "--vm", "victim.conf", "--delete-vm")
	if r.OK() {
		t.Error("delete of running VM succeeded")
	}
	r.ErrContains(t, "before deleting")
	for _, p := range []string{"victim", "victim.conf"} {
		if _, err := os.Stat(filepath.Join(d, p)); err != nil {
			t.Errorf("victim %s removed", p)
		}
	}
}

func TestPausedVMStatus(t *testing.T) {
	d := t.TempDir()
	vm := filepath.Join(d, "paused-vm")
	os.MkdirAll(vm, 0o755)
	Write(t, d, "paused-vm.conf",
		"guest_os=\"linux\"\ndisk_img=\"paused-vm/disk.qcow2\"\n")
	Write(t, vm, "disk.qcow2", "")
	SpawnFakeVM(t, vm, "paused-vm")
	Write(t, vm, "paused-vm.paused", "")
	r := Run(t, "quickemu", []RunOpt{InDir(d)}, "--vm", "paused-vm.conf", "--status")
	if r.Failed() {
		t.Fatalf("status rc=%d", r.Code)
	}
	r.Contains(t, "paused")
}

// ---- spec/integration/quickget_cli_spec.sh ----

func TestQuickgetCLIDispatch(t *testing.T) {
	qg := func(args ...string) Result {
		t.Helper()
		return Run(t, "quickget", nil, args...)
	}
	qg("--list").Contains(t, "alpine")
	qg("--list-csv").Contains(t, "Display Name,OS,Release")
	qg("--arch", "arm64", "--list").Contains(t, "OS")
	qg("--version").Contains(t, ".")
	qg("--help").Contains(t, "quickget")
	if r := qg("--bogus-flag"); r.OK() {
		t.Error("--bogus-flag accepted")
	} else {
		r.Contains(t, "Not supported argument")
	}
	if r := qg("not-an-os", "1"); r.OK() {
		t.Error("unknown OS accepted")
	} else {
		r.Contains(t, "not a supported OS")
	}
	if r := qg("alpine"); r.OK() {
		t.Error("missing release accepted")
	} else {
		r.Contains(t, "release")
	}
	if r := qg("--arch", "riscv", "alpine"); r.OK() {
		t.Error("riscv accepted")
	} else {
		r.Contains(t, "Unsupported architecture")
	}
	qg("--show", "alpine").Contains(t, "Alpine")
	if r := qg("alpine", "99"); r.OK() {
		t.Error("alpine 99 accepted")
	} else {
		r.Contains(t, "not a supported release")
	}
	if r := qg("fedora", "42"); r.OK() {
		t.Error("fedora 42 without edition accepted")
	} else {
		r.Contains(t, "edition")
	}
}
