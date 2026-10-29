package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A catalog URL points at a third-party server — the zip member names
// are untrusted. extractZip joins zf.Name straight into dest, so a
// "../../evil" member writes outside the VM dir (zip-slip).
func TestExtractZipSlip(t *testing.T) {
	parent := t.TempDir()
	dest := filepath.Join(parent, "vm")
	os.MkdirAll(dest, 0o755)
	zp := filepath.Join(parent, "img.zip")
	f, err := os.Create(zp)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for _, name := range []string{"../evil.txt", "ok/disk.img"} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte("x"))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()

	extractZip(zp, dest)

	if _, err := os.Stat(filepath.Join(parent, "evil.txt")); err == nil {
		t.Fatal("zip-slip: member wrote outside dest")
	}
	if _, err := os.Stat(filepath.Join(dest, "ok", "disk.img")); err != nil {
		t.Fatal("legitimate nested member should extract")
	}
}

// $USER is interpolated verbatim into cloud-init YAML (users: - name:)
// and a sudoers echo line — a value with a newline injects extra
// users/keys; one with " breaks the sudoers quoting. Same for a vmPath
// with spaces/newlines in the hostname fields.
func TestGenSeedISOSanitizesUser(t *testing.T) {
	dir := t.TempDir()
	vmPath := filepath.Join(dir, "vm")
	os.MkdirAll(vmPath, 0o755)
	confFile := filepath.Join(dir, "vm.conf")
	os.WriteFile(confFile, []byte("guest_os=\"linux\"\n"), 0o600)
	t.Setenv("USER", "x\n  - name: root")
	t.Setenv("HOME", dir) // no ssh keys picked up
	if rc := genSeedISO(vmPath, confFile); rc != 0 {
		t.Fatalf("genSeedISO rc=%d", rc)
	}
	b, err := os.ReadFile(filepath.Join(vmPath, "user-data"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "name: root") {
		t.Fatalf("cloud-init user injection via $USER:\n%s", b)
	}
}
