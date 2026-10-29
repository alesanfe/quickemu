// ops2_test.go — second sweep: fs copy helpers, profiles/presets,
// autostart/shortcut artifacts, trash restore, vfio inventory.
package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestFsHelpers(t *testing.T) {
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, "sub"), 0o755)
	os.WriteFile(filepath.Join(src, "a.txt"), []byte("aaa"), 0o600)
	os.WriteFile(filepath.Join(src, "sub", "b.txt"), []byte("bb"), 0o600)
	dst := t.TempDir() + "/copy"
	if err := cpDir(src, dst); err != nil {
		t.Fatalf("cpDir %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "sub", "b.txt")); string(b) != "bb" {
		t.Fatal("cpDir content")
	}
	// Symlinks must be preserved as links (mv/cp -r parity) — a VM
	// shared-folder link must not drag the whole target into the copy.
	if runtime.GOOS != "windows" {
		os.Symlink("sub", filepath.Join(src, "link"))
		dst2 := t.TempDir() + "/copy2"
		if err := cpDir(src, dst2); err != nil {
			t.Fatalf("cpDir symlink %v", err)
		}
		l, err := os.Readlink(filepath.Join(dst2, "link"))
		if err != nil || l != "sub" {
			t.Fatalf("symlink not preserved (readlink=%q err=%v)", l, err)
		}
	}
	cf := filepath.Join(t.TempDir(), "f.txt")
	if err := cpFile(filepath.Join(src, "a.txt"), cf, 0o600); err != nil {
		t.Fatalf("cpFile %v", err)
	}
	if err := cpFile(filepath.Join(src, "none"), cf, 0o600); err == nil {
		t.Fatal("cpFile missing")
	}
	// tarGz roundtrip.
	arc := filepath.Join(t.TempDir(), "x.tar.gz")
	if err := tarGz(src, arc); err != nil {
		t.Fatalf("tarGz %v", err)
	}
	if st, _ := os.Stat(arc); st == nil || st.Size() == 0 {
		t.Fatal("empty tar")
	}
}

func TestProfilesAndPresets(t *testing.T) {
	for _, p := range []string{"windows-gaming", "linux-desktop",
		"server", "minimal", "ci"} {
		m, err := ProfileDefaults(p)
		if err != nil || m["guest_os"] == "" {
			t.Fatalf("profile %q %v", p, err)
		}
	}
	if _, err := ProfileDefaults("nope"); err == nil {
		t.Fatal("unknown profile")
	}
	for _, n := range []string{"nat", "user", "bridge", "hostonly", "isolated", "none"} {
		if v, err := NetworkPreset(n); err != nil || v == "" {
			t.Fatalf("preset %q %v", n, err)
		}
	}
	if _, err := NetworkPreset("bogus"); err == nil {
		t.Fatal("unknown preset")
	}
}

func TestAutostartShortcut(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	conf := filepath.Join(home, "vm.conf")
	os.WriteFile(conf, []byte("guest_os=\"linux\"\n"), 0o600)
	if rc := autostartCreate("vm1", conf); rc != 0 {
		t.Fatalf("autostart rc=%d", rc)
	}
	unit := filepath.Join(home, ".config", "systemd", "user", "quickemu-vm1.service")
	if b, err := os.ReadFile(unit); err != nil || !strings.Contains(string(b), "--vm") {
		t.Fatalf("unit %v", err)
	}
	if rc := shortcutCreate("vm1", conf, ""); rc != 0 {
		t.Fatalf("shortcut rc=%d", rc)
	}
	dt := filepath.Join(home, ".local", "share", "applications", "vm1.desktop")
	if _, err := os.Stat(dt); err != nil {
		t.Fatal("desktop file")
	}
	shortcutDelete("vm1")
	if _, err := os.Stat(dt); !os.IsNotExist(err) {
		t.Fatal("desktop not removed")
	}
	shortcutDelete("vm1") // idempotent
}

func TestTrashRestore(t *testing.T) {
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	home := t.TempDir()
	t.Setenv("HOME", home)
	// No entry → rc 1.
	if rc := trashRestore("ghost"); rc != 1 {
		t.Fatalf("missing rc=%d", rc)
	}
	// Seed a trash slot.
	slot := filepath.Join(data, "quickemu", "trash", "1-vmx")
	os.MkdirAll(filepath.Join(slot, "vm", "vmx"), 0o755)
	os.WriteFile(filepath.Join(slot, "vm", "vmx.conf"), []byte("guest_os=\"linux\"\n"), 0o600)
	os.WriteFile(filepath.Join(slot, "trash-meta.json"),
		[]byte(`{"vm":"vmx","originConfDir":"`+home+`"}`), 0o600)
	// Restore moves dir+conf back to the origin dir.
	if rc := trashRestore("vmx"); rc != 0 {
		t.Fatalf("restore rc=%d", rc)
	}
	if _, err := os.Stat(filepath.Join(home, "vmx.conf")); err != nil {
		t.Fatal("conf not restored")
	}
	if _, err := os.Stat(slot); !os.IsNotExist(err) {
		t.Fatal("slot not cleaned")
	}
}

func TestVfioScan(t *testing.T) {
	old := outputJSON
	outputJSON = true
	if rc := vfioScan(""); rc != 0 {
		t.Fatalf("vfio json rc=%d", rc)
	}
	outputJSON = false
	if rc := vfioScan("0000:01:00.0"); rc != 0 {
		t.Fatalf("vfio text rc=%d", rc)
	}
	outputJSON = old
}

func TestConfGetDisk(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "vm.conf"),
		[]byte("disk_img=\"d.qcow2\"\nguest_os=\"linux\"\n"), 0o600)
	if got := confGet(dir, "guest_os"); got != "linux" {
		t.Fatalf("confGet %q", got)
	}
	if got := confDisk(dir); !strings.Contains(got, "qcow2") {
		t.Fatalf("confDisk %q", got)
	}
	if xdgData() == "" || vmVersion() == "" {
		t.Fatal("xdg/version")
	}
}
