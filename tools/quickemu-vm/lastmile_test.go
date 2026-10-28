// lastmile_test.go — backupList/exitCode/num/vmRunningDir/
// deletePlanDependents/trashList branches, main via exitFn.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestBackupListBranches(t *testing.T) {
	dir := t.TempDir()
	bdir := backupDir(dir)
	os.MkdirAll(bdir, 0o755)
	old := time.Now()
	_ = old
	// one backup file with a size to report
	bf := filepath.Join(bdir, "20250101T000000Z-vm.quickvm.tar.gz")
	os.WriteFile(bf, []byte("tarbytes"), 0o644)

	oldJSON := outputJSON
	defer func() { outputJSON = oldJSON }()
	outputJSON = true
	if rc := backupList(dir); rc != 0 {
		t.Fatal("json backupList failed")
	}
	outputJSON = false
	if rc := backupList(dir); rc != 0 {
		t.Fatal("text backupList failed")
	}
}

func TestExitCodeAndNum(t *testing.T) {
	if exitCode(nil) != 0 {
		t.Fatal("nil should be 0")
	}
	err := exec.Command("sh", "-c", "exit 7").Run()
	if exitCode(err) != 7 {
		t.Fatalf("exit code propagation: %v", err)
	}
	if exitCode(fmt.Errorf("boom")) != 1 {
		t.Fatal("generic error should be 1")
	}
	if num(3.5) != 3.5 {
		t.Fatal("float64")
	}
	if num(json.Number("2.5")) != 2.5 {
		t.Fatal("json.Number")
	}
	if num("x") != 0 {
		t.Fatal("other type should be 0")
	}
}

func TestVMRunningDirBranches(t *testing.T) {
	dir := t.TempDir()
	// no pidfile → false
	if vmRunningDir(dir, "vm") {
		t.Fatal("no pidfile should be false")
	}
	// garbage pidfile → false
	os.WriteFile(filepath.Join(dir, "vm.pid"), []byte("notapid"), 0o644)
	if vmRunningDir(dir, "vm") {
		t.Fatal("garbage pid should be false")
	}
	// pid of a definitely-dead process → false
	os.WriteFile(filepath.Join(dir, "vm.pid"), []byte("4194304"), 0o644)
	if vmRunningDir(dir, "vm") {
		t.Fatal("dead pid should be false")
	}
}

func TestDeletePlanDependentsStub(t *testing.T) {
	dir := t.TempDir()
	// two qcow2s; the child reports a backing file inside the dir.
	os.WriteFile(filepath.Join(dir, "base.qcow2"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "child.qcow2"), []byte("x"), 0o644)
	old := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		out := `{"backing-filename":"` + filepath.Join(dir, "base.qcow2") + `"}`
		return exec.Command("sh", "-c", "printf '%s' '"+out+"'")
	}
	defer func() { execCommand = old }()
	stub := t.TempDir()
	os.WriteFile(filepath.Join(stub, "qemu-img"), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	t.Setenv("PATH", stub+string(os.PathListSeparator)+os.Getenv("PATH"))
	if got := deletePlanDependents(dir); len(got) == 0 {
		t.Fatal("expected dependent entry")
	}
}

func TestTrashListBranches(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	root := trashRoot()
	slot := filepath.Join(root, "20250101T000000Z-vm")
	os.MkdirAll(slot, 0o755)
	os.WriteFile(filepath.Join(slot, "trash-meta.json"), []byte(`{"vm":"oldvm"}`), 0o644)
	slot2 := filepath.Join(root, "20250102T000000Z-vm2")
	os.MkdirAll(slot2, 0o755) // no meta → "{}" branch

	oldJSON := outputJSON
	defer func() { outputJSON = oldJSON }()
	outputJSON = true
	if trashList() != 0 {
		t.Fatal("json trashList")
	}
	outputJSON = false
	if trashList() != 0 {
		t.Fatal("text trashList")
	}
}

func TestMainEntryVM(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"quickemu-vm"}
	defer func() { os.Args = oldArgs }()
	// main exits through os.Exit — covered via the helper harness
	// (runTool) elsewhere; here assert usage rc directly.
	if rc := run(); rc != 2 {
		t.Fatalf("usage rc=%d", rc)
	}
}
