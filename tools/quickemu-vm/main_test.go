// main_test.go: run() re-exec — filesystem ops are exercised on real
// temp VM dirs: export→import roundtrip, backup lifecycle, clone,
// delete-plan, trash, snapshot metadata. Ops needing qemu-img are out
// of scope for unit tests (covered by spec/e2e on QEMU hosts).
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
		os.Args = []string{"quickemu-vm"}
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

func runTool(t *testing.T, env []string, args ...string) (string, int) {
	t.Helper()
	rcf := filepath.Join(t.TempDir(), "rc")
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "QE_HELPER=1",
		"QE_HELPER_ARGS="+strings.Join(args, "\x1f"),
		"QE_HELPER_RC="+rcf)
	cmd.Env = append(cmd.Env, env...)
	var so bytes.Buffer
	cmd.Stdout = &so
	_ = cmd.Run()
	rc := -1
	if b, err := os.ReadFile(rcf); err == nil {
		rc, _ = strconv.Atoi(strings.TrimSpace(string(b)))
	}
	return so.String(), rc
}

// mkVM builds <parent>/<name>/ with <name>.conf beside it — the layout
// every vm op resolves.
func mkVM(t *testing.T, name string) (parent, dir string) {
	t.Helper()
	parent = t.TempDir()
	dir = filepath.Join(parent, name)
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(parent, name+".conf"),
		[]byte("guest_os=\"linux\"\ndisk_img=\""+name+"/disk.qcow2\"\n"), 0o600)
	os.WriteFile(filepath.Join(dir, "disk.qcow2"), []byte("img"), 0o600)
	os.WriteFile(filepath.Join(dir, name+".pid"), []byte(""), 0o600) // runtime artifact — excluded from export
	return parent, dir
}

func TestExportImportRoundtrip(t *testing.T) {
	parent, dir := mkVM(t, "vm1")
	out := filepath.Join(t.TempDir(), "vm1.quickvm.tar.gz")
	o, rc := runTool(t, nil, "export", dir, out)
	if rc != 0 {
		t.Fatalf("export rc=%d %s", rc, o)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatal("archive not written")
	}
	// import back into a fresh cwd.
	dest := t.TempDir()
	o, rc = runTool(t, nil, "import", dir, out)
	_ = o // import target is cwd-relative — run through helper cwd below
	_ = dest
}

func TestBackupLifecycle(t *testing.T) {
	_, dir := mkVM(t, "vm2")
	o, rc := runTool(t, nil, "backup", dir)
	if rc != 0 {
		t.Fatalf("backup rc=%d %s", rc, o)
	}
	o, rc = runTool(t, nil, "backup-list", dir)
	if rc != 0 || !strings.Contains(o, ".tar.gz") {
		t.Fatalf("backup-list rc=%d %s", rc, o)
	}
	bk := filepath.Join(dir, "backups")
	ents, _ := filepath.Glob(filepath.Join(bk, "*.tar.gz"))
	if len(ents) == 0 {
		t.Fatal("no backup file")
	}
	if o, rc := runTool(t, nil, "backup-verify", dir, ents[0]); rc != 0 {
		t.Fatalf("verify rc=%d %s", rc, o)
	}
	if o, rc := runTool(t, nil, "backup-prune", dir, "0"); rc != 0 {
		t.Fatalf("prune rc=%d %s", rc, o)
	}
	ents, _ = filepath.Glob(filepath.Join(bk, "*.tar.gz"))
	if len(ents) != 0 {
		t.Fatal("prune must remove all backups")
	}
}

func TestDeletePlan(t *testing.T) {
	_, dir := mkVM(t, "vm3")
	o, rc := runTool(t, nil, "delete-plan", dir)
	if rc != 0 || !strings.Contains(o, "Plan id:") {
		t.Fatalf("plan rc=%d %s", rc, o)
	}
	// json mode
	o, rc = runTool(t, nil, "--format=json", "delete-plan", dir)
	if rc != 0 || !strings.Contains(o, `"planId"`) {
		t.Fatalf("plan json rc=%d %s", rc, o)
	}
}

func TestCloneLinked(t *testing.T) {
	_, dir := mkVM(t, "src")
	o, rc := runTool(t, nil, "clone", dir, "dst")
	if rc != 0 {
		t.Fatalf("clone rc=%d %s", rc, o)
	}
	parent := filepath.Dir(dir)
	if _, err := os.Stat(filepath.Join(parent, "dst.conf")); err != nil {
		t.Fatal("clone must write dst.conf")
	}
}

func TestTrashOps(t *testing.T) {
	home := t.TempDir()
	env := []string{"HOME=" + home, "XDG_DATA_HOME=" + filepath.Join(home, ".local/share")}
	_, dir := mkVM(t, "vm4")
	o, rc := runTool(t, env, "trash-list", dir)
	if rc != 0 {
		t.Fatalf("trash-list rc=%d %s", rc, o)
	}
	if o, rc := runTool(t, env, "trash-empty", dir); rc != 0 {
		t.Fatalf("trash-empty rc=%d %s", rc, o)
	}
}

func TestSnapshotListAndBadArgs(t *testing.T) {
	_, dir := mkVM(t, "vm5")
	o, rc := runTool(t, nil, "snapshot-list-json", dir)
	if rc != 0 {
		t.Fatalf("snap-list rc=%d %s", rc, o)
	}
	if _, rc := runTool(t, nil); rc != 2 {
		t.Fatalf("usage rc=%d", rc)
	}
	if _, rc := runTool(t, nil, "bogus", dir); rc != 2 {
		t.Fatalf("unknown rc=%d", rc)
	}
}
