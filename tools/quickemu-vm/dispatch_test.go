// dispatch_test.go — run() op coverage via the QE_HELPER harness:
// backup roundtrip, trash lifecycle, snapshot dispatch, delete paths,
// resize/compact with fake qemu-img, usage and unknown ops.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDispatchUsageAndUnknown(t *testing.T) {
	if out, rc := runTool(t, nil); rc != 2 {
		t.Fatalf("usage rc=%d %s", rc, out)
	}
	if out, rc := runTool(t, nil, "bogus", t.TempDir()); rc != 2 {
		t.Fatalf("unknown rc=%d %s", rc, out)
	}
}

func TestDispatchBackupTrash(t *testing.T) {
	xdg := t.TempDir()
	env := []string{"XDG_DATA_HOME=" + xdg, "QE_VMCONF="}
	parent, dir := mkVM(t, "btvm")
	_ = parent
	conf := dir + ".conf"
	env = append(env, "QE_VMCONF="+conf)
	os.WriteFile(filepath.Join(dir, "disk.qcow2"), []byte("d"), 0o600)

	// backup → list → verify → restore → prune.
	if out, rc := runTool(t, env, "backup", dir); rc != 0 {
		t.Fatalf("backup rc=%d %s", rc, out)
	}
	out, rc := runTool(t, env, "backup-list", dir)
	if rc != 0 || !strings.Contains(out, ".tar.gz") {
		t.Fatalf("backup-list rc=%d %s", rc, out)
	}
	var backup string
	if m, _ := filepath.Glob(filepath.Join(dir, "backups", "*.tar.gz")); len(m) > 0 {
		backup = m[0]
	}
	if backup == "" {
		t.Fatal("no backup produced")
	}
	if out, rc := runTool(t, env, "backup-verify", dir, backup); rc != 0 {
		t.Fatalf("verify rc=%d %s", rc, out)
	}
	dest := t.TempDir()
	if out, rc := runTool(t, append(env, "HOME="+dest), "backup-restore", dir, backup, dest); rc != 0 {
		t.Fatalf("restore rc=%d %s", rc, out)
	}
	if out, rc := runTool(t, env, "backup-prune", dir, "1"); rc != 0 {
		t.Fatalf("prune rc=%d %s", rc, out)
	}
	// verify failure path.
	bad := filepath.Join(t.TempDir(), "b.tar.gz")
	os.WriteFile(bad, []byte("junk"), 0o600)
	if _, rc := runTool(t, env, "backup-verify", dir, bad); rc == 0 {
		t.Fatal("corrupt backup should fail")
	}
	if _, rc := runTool(t, env, "backup-verify", dir); rc == 0 {
		t.Fatal("missing arg should die")
	}

	// trash: slot "<ts>-<name>" with vm/ payload; restore needs origin dir.
	slot := filepath.Join(xdg, "quickemu", "trash", "1-x")
	vdir := filepath.Join(slot, "vm", "x")
	os.MkdirAll(vdir, 0o755)
	os.WriteFile(filepath.Join(slot, "vm", "x.conf"), []byte("guest_os=\"linux\"\n"), 0o600)
	destDir := t.TempDir()
	os.WriteFile(filepath.Join(slot, "trash-meta.json"),
		[]byte(`{"vm":"x","originConfDir":"`+destDir+`"}`), 0o600)
	if out, rc := runTool(t, env, "trash-list", dir); rc != 0 || !strings.Contains(out, "1-x") {
		t.Fatalf("trash-list rc=%d %s", rc, out)
	}
	if out, rc := runTool(t, env, "trash-restore", dir, "x"); rc != 0 {
		t.Fatalf("trash-restore rc=%d %s", rc, out)
	}
	if _, err := os.Stat(filepath.Join(destDir, "x.conf")); err != nil {
		t.Fatal("conf not restored")
	}
	if _, rc := runTool(t, env, "trash-restore", dir, "ghost"); rc == 0 {
		t.Fatal("missing slot should fail")
	}
	if _, rc := runTool(t, env, "trash-empty", dir); rc != 0 {
		t.Fatalf("trash-empty rc=%d", rc)
	}
	if _, err := os.Stat(filepath.Join(xdg, "quickemu", "trash")); err == nil {
		t.Fatal("trash not emptied")
	}
	if _, rc := runTool(t, env, "trash-empty", dir); rc != 0 {
		t.Fatalf("empty-again rc=%d", rc)
	}
}

func TestDispatchSnapshotAndDisk(t *testing.T) {
	dir := t.TempDir()
	conf := dir + ".conf"
	disk := filepath.Join(dir, "d.qcow2")
	os.WriteFile(disk, []byte("d"), 0o600)
	os.WriteFile(conf, []byte("disk_img=\""+disk+"\"\n"), 0o644)
	env := []string{"QE_VMCONF=" + conf}

	// fake qemu-img in PATH for resize/compact/snapshot-info.
	stub := t.TempDir()
	os.WriteFile(filepath.Join(stub, "qemu-img"),
		[]byte("#!/bin/sh\nif [ \"$1\" = convert ]; then eval \"touch \\${$#}\"; fi\nexit 0\n"), 0o755)
	env = append(env, "PATH="+stub+string(os.PathListSeparator)+os.Getenv("PATH"))

	if _, rc := runTool(t, env, "snapshot-list-json", dir); rc != 0 {
		t.Fatalf("snapshot-list-json rc=%d", rc)
	}
	if _, rc := runTool(t, env, "snapshot-info", dir); rc != 0 {
		t.Fatalf("snapshot-info rc=%d", rc)
	}
	if _, rc := runTool(t, env, "resize", dir); rc != 1 {
		t.Fatalf("resize no-size rc=%d", rc)
	}
	if _, rc := runTool(t, env, "resize", dir, "32G"); rc != 0 {
		t.Fatalf("resize rc=%d", rc)
	}
	if _, rc := runTool(t, env, "compact", dir); rc != 0 {
		t.Fatalf("compact rc=%d", rc)
	}
	// delete-plan: json + text.
	if out, rc := runTool(t, append(env, "OUTPUT_FORMAT=json"), "delete-plan", dir); rc != 0 || !strings.Contains(out, "planId") {
		t.Fatalf("delete-plan rc=%d %s", rc, out)
	}
	if out, rc := runTool(t, env, "delete-plan", dir); rc != 0 || !strings.Contains(out, "Plan id") {
		t.Fatalf("delete-plan text rc=%d %s", rc, out)
	}
	if _, rc := runTool(t, env, "vfio-scan", dir); rc != 0 {
		t.Fatalf("vfio-scan rc=%d", rc)
	}
}

func TestDispatchCloneDelete(t *testing.T) {
	xdg := t.TempDir()
	parent, dir := mkVM(t, "clvm")
	conf := dir + ".conf"
	env := []string{"QE_VMCONF=" + conf, "XDG_DATA_HOME=" + xdg, "HOME=" + parent}

	// clone (config-only: no disk → qemu-img not reached).
	if out, rc := runTool(t, env, "clone", dir, "clvm2"); rc != 0 {
		t.Fatalf("clone rc=%d %s", rc, out)
	}
	if _, err := os.Stat(filepath.Join(parent, "clvm2.conf")); err != nil {
		t.Fatalf("clone conf missing: %v", err)
	}
	// delete --trash moves conf+dir into a trash slot.
	if out, rc := runTool(t, env, "delete", dir, "--trash"); rc != 0 {
		t.Fatalf("delete --trash rc=%d %s", rc, out)
	}
	slots, _ := filepath.Glob(filepath.Join(xdg, "quickemu", "trash", "*-clvm"))
	if len(slots) == 0 {
		t.Fatal("no trash slot created")
	}
	// plan-id mismatch dies.
	_, dir3 := mkVM(t, "pvm")
	env3 := []string{"QE_VMCONF=" + dir3 + ".conf", "XDG_DATA_HOME=" + xdg}
	if _, rc := runTool(t, env3, "delete", dir3, "--plan-id=sha256:wrong"); rc != -1 {
		t.Fatalf("plan mismatch rc=%d", rc)
	}
}

// clone/rename join the new name into a sibling path — traversal or
// separator-bearing names must die before any mkdir/rename runs.
func TestDispatchNameTraversal(t *testing.T) {
	parent, dir := mkVM(t, "tvm")
	conf := dir + ".conf"
	env := []string{"QE_VMCONF=" + conf, "HOME=" + parent, "XDG_DATA_HOME=" + t.TempDir()}
	outside := filepath.Join(filepath.Dir(parent), "escape")

	for _, bad := range []string{"../escape", "a/b", "..", ".", "-x"} {
		if out, rc := runTool(t, env, "clone", dir, bad); rc == 0 {
			t.Fatalf("clone accepted name %q: %s", bad, out)
		}
		if out, rc := runTool(t, env, "rename", dir, bad); rc == 0 {
			t.Fatalf("rename accepted name %q: %s", bad, out)
		}
	}
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Fatal("a name arg escaped the VM parent dir")
	}
	if _, err := os.Stat(filepath.Join(parent, "a")); !os.IsNotExist(err) {
		t.Fatal("clone created a nested dir from a separator name")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatal("a rejected rename still moved the VM")
	}
}

func TestDispatchRenameMoveMisc(t *testing.T) {
	parent, dir := mkVM(t, "rmvm")
	conf := dir + ".conf"
	env := []string{"QE_VMCONF=" + conf, "HOME=" + parent, "XDG_DATA_HOME=" + t.TempDir()}

	if out, rc := runTool(t, env, "rename", dir, "rmvm2"); rc != 0 {
		t.Fatalf("rename rc=%d %s", rc, out)
	}
	dir2 := filepath.Join(parent, "rmvm2")
	if _, err := os.Stat(dir2); err != nil {
		t.Fatal("dir not renamed")
	}
	dst := filepath.Join(t.TempDir(), "moved")
	if out, rc := runTool(t, env, "move", dir2, dst); rc != 0 {
		t.Fatalf("move rc=%d %s", rc, out)
	}
	if _, err := os.Stat(dst); err != nil {
		t.Fatal("not moved")
	}
	if out, rc := runTool(t, env, "profile", dir2, "windows-gaming"); rc != 0 {
		t.Fatalf("profile rc=%d %s", rc, out)
	}
	if _, rc := runTool(t, env, "profile", dir2, "bogus"); rc != -1 {
		t.Fatalf("bad profile rc=%d", rc)
	}
	if _, rc := runTool(t, env, "network-preset", dir2); rc != -1 {
		t.Fatalf("no preset arg rc=%d", rc)
	}
	if out, rc := runTool(t, env, "network-preset", dir2, "isolated"); rc != 0 {
		t.Fatalf("preset rc=%d %s", rc, out)
	}
	stub := t.TempDir()
	os.WriteFile(filepath.Join(stub, "qemu-img"), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	env2 := append(env, "PATH="+stub+string(os.PathListSeparator)+os.Getenv("PATH"))
	if _, rc := runTool(t, env2, "linked-clone", dir2, "a.qcow2", "b.qcow2"); rc != 0 {
		t.Fatalf("linked-clone rc=%d", rc)
	}
	if _, rc := runTool(t, env2, "linked-clone", dir2); rc != -1 {
		t.Fatalf("linked-clone short rc=%d", rc)
	}
	if _, rc := runTool(t, env2, "autostart", dir2); rc != 0 {
		t.Fatalf("autostart rc=%d", rc)
	}
	if _, rc := runTool(t, env2, "shortcut", dir2); rc != 0 {
		t.Fatalf("shortcut rc=%d", rc)
	}
	if _, rc := runTool(t, env2, "shortcut-delete", dir2); rc != 0 {
		t.Fatalf("shortcut-delete rc=%d", rc)
	}
	if _, rc := runTool(t, env2, "snapshot-create", dir2, "snap1"); rc != 0 {
		t.Fatalf("snapshot-create rc=%d", rc)
	}
	if _, rc := runTool(t, env2, "snapshot-create", dir2, "ext1", "--external"); rc != 1 {
		t.Fatalf("external rc=%d", rc)
	}
	if _, rc := runTool(t, env2, "snapshot-apply", dir2, "snap1"); rc != 0 {
		t.Fatalf("snapshot-apply rc=%d", rc)
	}
	if _, rc := runTool(t, env2, "snapshot-delete", dir2, "snap1"); rc != 0 {
		t.Fatalf("snapshot-delete rc=%d", rc)
	}
	if _, rc := runTool(t, env2, "snapshot-tree", dir2); rc != 0 {
		t.Fatalf("snapshot-tree rc=%d", rc)
	}
	if _, rc := runTool(t, env2, "snapshot-verify", dir2); rc != 0 {
		t.Fatalf("snapshot-verify rc=%d", rc)
	}
	if _, rc := runTool(t, env2, "snapshot-consolidate", dir2); rc != -1 {
		t.Fatalf("consolidate rc=%d", rc)
	}
	os.MkdirAll(filepath.Join(dir2, ".snapshots"), 0o755)
	os.WriteFile(filepath.Join(dir2, ".snapshots", "a.json"), []byte(`{"tag":"a"}`), 0o600)
	if out, rc := runTool(t, append(env2, "OUTPUT_FORMAT=json"), "snapshot-list", dir2); rc != 0 || !strings.Contains(out, "a") {
		t.Fatalf("snapshot-list rc=%d %s", rc, out)
	}
	// export writes <name>.quickvm.tar.gz to the helper's cwd — run it
	// in a temp workdir so nothing lands in the module dir.
	work := t.TempDir()
	if out, rc := runToolIn(t, env2, work, "export", dir2); rc != 0 {
		t.Fatalf("export rc=%d %s", rc, out)
	}
	exps, _ := filepath.Glob(filepath.Join(work, "*.quickvm.tar.gz"))
	if len(exps) == 0 {
		t.Fatal("no export produced")
	}
	// clone --full copies the whole dir.
	if _, rc := runTool(t, env2, "clone", dir2, "rmvm3", "--full"); rc != 0 {
		t.Fatalf("clone --full rc=%d", rc)
	}
	// import dispatch (importVm's happy path is covered by unit tests;
	// here the missing-file error branch).
	if _, rc := runTool(t, env2, "import", dir2, filepath.Join(t.TempDir(), "none.tar.gz")); rc != 1 {
		t.Fatalf("import missing rc=%d", rc)
	}
}
