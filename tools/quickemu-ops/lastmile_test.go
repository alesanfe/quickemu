// lastmile_test.go — die paths for checkID/must/load/save/runWatch,
// main() via exitFn seam.
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpsDiePaths(t *testing.T) {
	vmdir := t.TempDir()

	// checkID rejects traversal-y ids before touching the fs.
	if _, rc := runTool(t, "status", "--vmdir", vmdir, "--id", "../x"); rc != -1 {
		t.Fatalf("bad id should die, rc=%d", rc)
	}
	// must: well-formed but absent op id → OPERATION_NOT_FOUND.
	if _, rc := runTool(t, "status", "--vmdir", vmdir, "--id", "nosuch"); rc != -1 {
		t.Fatalf("missing op should die, rc=%d", rc)
	}
	// load OP_CORRUPT: existing file, invalid JSON → die on `set`.
	ops := opsDir(vmdir)
	os.MkdirAll(ops, 0o755)
	os.WriteFile(filepath.Join(ops, "bad.json"), []byte("{"), 0o644)
	if _, rc := runTool(t, "set", "--vmdir", vmdir, "--id", "bad", "--state", "running"); rc != -1 {
		t.Fatalf("corrupt op should die, rc=%d", rc)
	}
	// save OP_PERSIST: vmdir is a regular file → opsDir unresolvable.
	vf := filepath.Join(t.TempDir(), "file-vm")
	os.WriteFile(vf, []byte("x"), 0o644)
	if _, rc := runTool(t, "new", "--vmdir", vf, "--type", "export"); rc != -1 {
		t.Fatalf("unwritable ops dir should die, rc=%d", rc)
	}
	// watch on an ops dir that cannot exist → WATCH_UNAVAILABLE.
	if _, rc := runTool(t, "watch", "--vmdir", vf); rc != -1 {
		t.Fatalf("watch on bad dir should die, rc=%d", rc)
	}
}

func TestMainEntry(t *testing.T) {
	oldExit, oldArgs := exitFn, os.Args
	var rc int
	exitFn = func(c int) { rc = c }
	os.Args = []string{"quickemu-ops"}
	t.Cleanup(func() { exitFn, os.Args = oldExit, oldArgs })
	main()
	if rc != 2 {
		t.Fatalf("usage rc=%d", rc)
	}
}
