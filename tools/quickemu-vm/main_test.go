// main_test.go: run() re-exec — filesystem ops are exercised on real
// temp VM dirs: export→import roundtrip, backup lifecycle, clone,
// delete-plan, trash, snapshot metadata. Ops needing qemu-img are out
// of scope for unit tests (covered by spec/e2e on QEMU hosts).
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
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
	return runToolIn(t, env, "", args...)
}

// runToolIn is runTool with an explicit helper cwd — for ops that read
// or write cwd-relative files (export/import land in ".").
func runToolIn(t *testing.T, env []string, cwd string, args ...string) (string, int) {
	t.Helper()
	rcf := filepath.Join(t.TempDir(), "rc")
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	if cwd != "" {
		cmd.Dir = cwd
	}
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
	_, dir := mkVM(t, "vm1")
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
	runTool(t, nil, "import", dir, out)
	_ = dest
}

// Regression: exportVm opened every member — a symlink inside the VM dir
// was followed, embedding the link *target's* bytes in the archive (and
// diverging from the import side, which refuses links outright).
func TestExportSkipsSymlinks(t *testing.T) {
	parent, dir := mkVM(t, "vmsym")
	outside := filepath.Join(parent, "secret.img")
	os.WriteFile(outside, []byte("SECRET-DATA"), 0o600)
	if err := os.Symlink(outside, filepath.Join(dir, "leak.iso")); err != nil {
		t.Skip("symlinks unsupported")
	}
	out := filepath.Join(t.TempDir(), "vmsym.quickvm.tar.gz")
	if o, rc := runTool(t, nil, "export", dir, out); rc != 0 {
		t.Fatalf("export rc=%d %s", rc, o)
	}
	f, _ := os.Open(out)
	defer f.Close()
	gz, _ := gzip.NewReader(f)
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err != nil {
			break
		}
		b, _ := io.ReadAll(tr)
		if bytes.Contains(b, []byte("SECRET-DATA")) {
			t.Fatalf("member %q leaked the symlink target's contents", h.Name)
		}
	}
}

// Regression: importVm verified checksum lines by joining the recorded
// path onto the staging dir without a locality check — a sums line for
// "../../x" validated content outside the archive.
func TestImportChecksumTraversal(t *testing.T) {
	// Build a hostile archive in-memory: conf + manifest + a checksums
	// file whose only record points outside the staging root.
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp) // staging lands inside our sandbox
	outside := filepath.Join(tmp, "planted.bin")
	os.WriteFile(outside, []byte("x"), 0o600)
	sum := fmt.Sprintf("%x", sha256.Sum256([]byte("x")))
	manifest := `{"formatVersion":1,"vm":"evil"}`
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	add := func(name, body string) {
		tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body))})
		tw.Write([]byte(body))
	}
	add("evil.conf", "guest_os=\"linux\"\n")
	add("quickvm-manifest.json", manifest)
	add("quickvm-checksums.sha256", sum+"  ../planted.bin\n")
	tw.Close()
	gz.Close()
	in := filepath.Join(t.TempDir(), "evil.quickvm.tar.gz")
	os.WriteFile(in, buf.Bytes(), 0o600)
	dest := t.TempDir()
	o, rc := runToolIn(t, nil, dest, "import", "x", in)
	if rc == 0 {
		t.Fatalf("traversal checksum record must not validate: %s", o)
	}
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
	// keep<=0 falls back to 5 — seed a second backup and prune to 1.
	os.WriteFile(filepath.Join(bk, "2000-01-01-old.quickvm.tar.gz"),
		[]byte("x"), 0o600)
	if o, rc := runTool(t, nil, "backup-prune", dir, "1"); rc != 0 {
		t.Fatalf("prune rc=%d %s", rc, o)
	}
	ents, _ = filepath.Glob(filepath.Join(bk, "*.tar.gz"))
	if len(ents) != 1 {
		t.Fatalf("prune must keep 1, have %d", len(ents))
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
	parent, dir := mkVM(t, "src")
	// No disk_img → "config only" path (qemu-img paths covered by e2e).
	os.WriteFile(filepath.Join(parent, "src.conf"),
		[]byte("guest_os=\"linux\"\n"), 0o600)
	env := []string{"QE_VMCONF=" + filepath.Join(parent, "src.conf")}
	o, rc := runTool(t, env, "clone", dir, "dst")
	if rc != 0 {
		t.Fatalf("clone rc=%d %s", rc, o)
	}
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

func TestBackupKeepEnv(t *testing.T) {
	dir := t.TempDir()
	bk := filepath.Join(dir, "backups")
	os.MkdirAll(bk, 0o755)
	base := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, n := range []string{"a", "b", "c"} {
		f := filepath.Join(bk, "2020-01-0"+fmt.Sprint(i+1)+"-"+n+".quickvm.tar.gz")
		if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		m := base.Add(time.Duration(i) * 24 * time.Hour)
		os.Chtimes(f, m, m)
	}
	// BACKUP_KEEP env beats the positional arg (Bash 5).
	if o, rc := runTool(t, []string{"BACKUP_KEEP=1"}, "backup-prune", dir, "3"); rc != 0 {
		t.Fatalf("prune rc=%d %s", rc, o)
	}
	ents, _ := filepath.Glob(filepath.Join(bk, "*.tar.gz"))
	if len(ents) != 1 {
		t.Fatalf("BACKUP_KEEP=1 must keep 1 of 3, have %d", len(ents))
	}
}
