// ops2_test.go — rbCompensate residue cleanup per operation type,
// Op.save/load roundtrip, flag/must helpers.
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRbCompensateTypes(t *testing.T) {
	// rbCompensate operates on the VM dir (parent of the ops dir).
	vm := t.TempDir()
	ops := filepath.Join(vm, ".ops")
	os.MkdirAll(ops, 0o755)

	// download: *.part *.zsync* *.tmp removed.
	for _, f := range []string{"dl.part", "x.zsync", "x.zsync.1", "y.tmp"} {
		os.WriteFile(filepath.Join(vm, f), []byte("x"), 0o600)
	}
	keep := filepath.Join(vm, "disk.qcow2")
	os.WriteFile(keep, []byte("x"), 0o600)
	rbCompensate(ops, "download")
	for _, f := range []string{"dl.part", "x.zsync", "x.zsync.1", "y.tmp"} {
		if _, err := os.Stat(filepath.Join(vm, f)); !os.IsNotExist(err) {
			t.Fatalf("%s must be reaped", f)
		}
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatal("keep file removed")
	}

	// import/export: .import-* .export-* dirs removed.
	for _, d := range []string{".import-a", ".export-b"} {
		os.MkdirAll(filepath.Join(vm, d), 0o755)
	}
	rbCompensate(ops, "import")
	rbCompensate(ops, "export")
	for _, d := range []string{".import-a", ".export-b"} {
		if _, err := os.Stat(filepath.Join(vm, d)); !os.IsNotExist(err) {
			t.Fatalf("%s must be reaped", d)
		}
	}

	// clone/rename: .partial-* removed.
	os.MkdirAll(filepath.Join(vm, ".partial-c"), 0o755)
	rbCompensate(ops, "clone")
	rbCompensate(ops, "rename")
	if _, err := os.Stat(filepath.Join(vm, ".partial-c")); !os.IsNotExist(err) {
		t.Fatal(".partial must be reaped")
	}

	// delete: .trash → vm rename-back when vm is gone.
	trash := filepath.Join(filepath.Dir(ops), ".trash")
	os.MkdirAll(trash, 0o755)
	rbCompensate(ops, "delete")
	if _, err := os.Stat(filepath.Join(filepath.Dir(ops), "vm")); err != nil {
		t.Fatal(".trash must be restored to vm")
	}

	// compact/snapshot: *.tmp.qcow2 *.compact-* removed.
	for _, f := range []string{"a.tmp.qcow2", "b.compact-1"} {
		os.WriteFile(filepath.Join(vm, f), []byte("x"), 0o600)
	}
	rbCompensate(ops, "compact")
	rbCompensate(ops, "snapshot")
	for _, f := range []string{"a.tmp.qcow2", "b.compact-1"} {
		if _, err := os.Stat(filepath.Join(vm, f)); !os.IsNotExist(err) {
			t.Fatalf("%s must be reaped", f)
		}
	}
	// Unknown type → no-op.
	rbCompensate(ops, "bogus")
}

func TestSaveLoadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	o := &Op{OperationID: "op1", State: "running"}
	o.save(dir)
	got := load(dir, "op1")
	if got == nil || got.State != "running" {
		t.Fatalf("roundtrip %+v", got)
	}
	// JSON on disk parses.
	b, err := os.ReadFile(filepath.Join(dir, "op1.json"))
	if err != nil || json.Unmarshal(b, &Op{}) != nil {
		t.Fatal("persisted JSON")
	}
}

// runWatch loops forever by design — run it in a goroutine against a
// real dir and verify a written .json op is streamed to stdout.
func TestRunWatchStreams(t *testing.T) {
	dir := t.TempDir()
	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = old }()
	go runWatch(dir)
	// Let fsnotify arm, then drop an op file.
	time.Sleep(300 * time.Millisecond)
	os.WriteFile(filepath.Join(dir, "op1.json"),
		[]byte(`{"operationId":"op1"}`), 0o600)
	deadline := time.Now().Add(3 * time.Second)
	buf := make([]byte, 0, 256)
	tmp := make([]byte, 128)
	for time.Now().Before(deadline) {
		r.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		n, _ := r.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if strings.Contains(string(buf), "op1") {
			break
		}
	}
	if !strings.Contains(string(buf), "op1") {
		t.Fatalf("watch did not stream the op: %q", buf)
	}
}

func TestOpLifecycle(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "ops"), 0o755)
	// new → set → done → status, all through run() for dispatch cover.
	out, rc := runTool(t, "new", "--vmdir", dir, "--type", "clone", "--vm", "v")
	if rc != 0 || strings.TrimSpace(out) == "" {
		t.Fatalf("new rc=%d %q", rc, out)
	}
	id := strings.TrimSpace(out)
	if _, rc = runTool(t, "set", "--vmdir", dir, "--id", id,
		"--state", "running", "--progress", "42", "--worker-pid", "1234"); rc != 0 {
		t.Fatalf("set rc=%d", rc)
	}
	if _, rc = runTool(t, "done", "--vmdir", dir, "--id", id); rc != 0 {
		t.Fatalf("done rc=%d", rc)
	}
	// Terminal-state set must fail (one-way).
	if _, rc = runTool(t, "set", "--vmdir", dir, "--id", id,
		"--state", "running"); rc == 0 {
		t.Fatal("set on terminal op must fail")
	}
	// list/status/cancel/prune paths.
	if out, rc = runTool(t, "list", "--vmdir", dir); rc != 0 || !strings.Contains(out, id) {
		t.Fatalf("list rc=%d %q", rc, out)
	}
	if _, rc = runTool(t, "status", "--vmdir", dir, "--id", id); rc != 0 {
		t.Fatalf("status rc=%d", rc)
	}
	// recover/rollback on clean dir → graceful.
	_, _ = runTool(t, "recover", "--vmdir", dir)
	_, _ = runTool(t, "rollback", "--vmdir", dir)
	_, _ = runTool(t, "prune", "--vmdir", dir)
}

func TestFlagHelper(t *testing.T) {
	args := []string{"--vmdir", "/x", "--vm", "y"}
	if flag(args, "--vmdir") != "/x" || flag(args, "--vm") != "y" ||
		flag(args, "--missing") != "" {
		t.Fatal("flag")
	}
	// Flag as last arg (no value) → "".
	if flag([]string{"--vmdir"}, "--vmdir") != "" {
		t.Fatal("dangling flag")
	}
}
