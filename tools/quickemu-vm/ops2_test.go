// ops2_test.go — second sweep: fs copy helpers, profiles/presets,
// autostart/shortcut artifacts, trash restore, vfio inventory.
package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
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

// The vm name and conf path land verbatim in a systemd unit and a
// .desktop Exec line — a newline injects new keys (ExecStart=/evil)
// and a space in the conf path silently splits the command.
// The internal snapshot ops pass the tag verbatim to
// `qemu-img snapshot <flag> <tag> <disk>` — a tag like "-d" lands in
// option position and reshapes the command (only the --external path
// applied snapTagRe). Tags must match [A-Za-z0-9._-]+ and not start
// with '-'.
func TestSnapshotTagInjection(t *testing.T) {
	parent, dir := mkVM(t, "snapvm")
	_ = parent
	conf := dir + ".conf"
	env := []string{"QE_VMCONF=" + conf}
	bin, mark := markerQemuImg(t)
	env = append(env, "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, bad := range []string{"-d", "-a", "--foo", "a b", "x/y", ""} {
		out, rc := runTool(t, env, "snapshot-create", dir, bad)
		if rc == 0 {
			t.Fatalf("tag %q accepted: %s", bad, out)
		}
	}
	if b, err := os.ReadFile(mark); err == nil {
		t.Fatalf("hostile tag reached qemu-img: %s", b)
	}
}

// snapshots.json is untrusted content: a crafted "file" field points
// `qemu-img commit` and os.Remove at a foreign qcow2 — node.File must
// stay inside the VM dir (same class as the ops/operationId fix).
func TestSnapshotConsolidateForeignFile(t *testing.T) {
	_, dir := mkVM(t, "snapvm")
	outside := t.TempDir()
	victim := filepath.Join(outside, "victim.qcow2")
	if err := os.WriteFile(victim, []byte("img"), 0o600); err != nil {
		t.Fatal(err)
	}
	meta := `[{"id":"x","tag":"evil","file":` + strconv.Quote(victim) + `,"parent":"p"}]`
	if err := os.WriteFile(filepath.Join(dir, "snapshots.json"), []byte(meta), 0o600); err != nil {
		t.Fatal(err)
	}
	bin, mark := markerQemuImg(t)
	conf := dir + ".conf"
	env := []string{"QE_VMCONF=" + conf,
		"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH")}
	out, rc := runTool(t, env, "snapshot-consolidate", dir, "evil")
	if rc == 0 {
		t.Fatalf("consolidate accepted a foreign overlay path: %s", out)
	}
	if _, err := os.Stat(victim); err != nil {
		t.Fatalf("foreign file touched: %v", err)
	}
	if b, err := os.ReadFile(mark); err == nil {
		t.Fatalf("qemu-img ran against a foreign path: %s", b)
	}
}

// trash-restore took the name verbatim into filepath.Join for both
// the source (slot/vm/<name>) and destination (confDir/<name>) — a
// "../x" name moves files from a crafted slot outside the conf dir.
func TestTrashRestoreNameTraversal(t *testing.T) {
	parent, dir := mkVM(t, "trashvm")
	_ = parent
	conf := dir + ".conf"
	// Plant a trash slot named "evil-.." whose nested layout the
	// traversal resolves: pattern *-../escape -> <root>/evil-../escape,
	// then srcConf = slot/vm/../escape.conf -> slot/escape.conf, dst =
	// confDir/../escape.conf — outside every intended root.
	data := t.TempDir()
	trash := filepath.Join(data, "quickemu", "trash")
	slot := filepath.Join(trash, "evil-..", "escape")
	os.MkdirAll(filepath.Join(slot, "vm"), 0o755)
	os.WriteFile(filepath.Join(slot, "escape.conf"), []byte("x=1\n"), 0o600)
	os.WriteFile(filepath.Join(slot, "trash-meta.json"),
		[]byte(`{"originConfDir":"`+filepath.Dir(conf)+`"}`), 0o600)
	env := []string{"QE_VMCONF=" + conf, "XDG_DATA_HOME=" + data}
	out, rc := runTool(t, env, "trash-restore", dir, "../escape")
	if rc == 0 {
		t.Fatalf("traversal name accepted: %s", out)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(conf), "..", "escape.conf")); err == nil {
		t.Fatal("conf escaped into the parent dir")
	}
}

func TestAutostartShortcutInjection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))

	conf := filepath.Join(home, "vm.conf")
	os.WriteFile(conf, []byte("guest_os=\"linux\"\n"), 0o600)

	// Newline in the name flows into the unit filename and the
	// Description/ExecStart lines — refuse, don't write.
	if rc := autostartCreate("x\nExecStart=/evil", conf); rc == 0 {
		t.Fatal("newline in name must fail")
	}
	// Same for the desktop Exec line via opts.
	if rc := shortcutCreate("vm", conf, "a\nExec=/evil"); rc == 0 {
		t.Fatal("newline in opts must fail")
	}
	ents, _ := filepath.Glob(filepath.Join(home, ".config", "systemd", "user", "*.service"))
	for _, e := range ents {
		if b, _ := os.ReadFile(e); strings.Contains(string(b), "/evil") {
			t.Fatalf("injected unit key written: %s", e)
		}
	}
	dents, _ := filepath.Glob(filepath.Join(home, ".local", "share", "applications", "*.desktop"))
	for _, e := range dents {
		if b, _ := os.ReadFile(e); strings.Contains(string(b), "Exec=/evil") {
			t.Fatalf("injected Exec key written: %s", e)
		}
	}

	// A conf path with spaces must be quoted in ExecStart, not split.
	d2 := filepath.Join(home, "my vms")
	os.MkdirAll(d2, 0o755)
	conf2 := filepath.Join(d2, "vm.conf")
	os.WriteFile(conf2, []byte("guest_os=\"linux\"\n"), 0o600)
	if rc := autostartCreate("spaced", conf2); rc != 0 {
		t.Fatalf("autostart rc=%d", rc)
	}
	unit := filepath.Join(home, ".config", "systemd", "user", "quickemu-spaced.service")
	b, err := os.ReadFile(unit)
	if err != nil {
		t.Fatal("unit not written")
	}
	if !strings.Contains(string(b), `"`+conf2+`"`) {
		t.Fatalf("conf path with spaces must be quoted in ExecStart:\n%s", b)
	}
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
