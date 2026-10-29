// state_test.go: lifecycle/state-machine contract — stale detection,
// foreign-pid rejection, pidless-lock reclaim (lifecycle_machine /
// chaos / adversarial spec parity, ported to the Go internals).
package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// setupStateVM returns the VM state dir — vmdir() resolves to
// <confdir>/<vmname> when the conf sets no disk_img (the quickget
// per-VM-dir convention), so state files live one level deeper.
func setupStateVM(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	vmConf = filepath.Join(dir, "vm1.conf")
	if err := os.WriteFile(vmConf, nil, 0644); err != nil {
		t.Fatal(err)
	}
	vd := vmdir()
	if err := os.MkdirAll(vd, 0755); err != nil {
		t.Fatal(err)
	}
	return vd
}

func TestStateAbsentIsStopped(t *testing.T) {
	setupStateVM(t)
	if st, _ := state(); st != "stopped" {
		t.Errorf("no pidfile -> state must be stopped, got %q", st)
	}
}

func TestStateStaleForDeadPid(t *testing.T) {
	dir := setupStateVM(t)
	cmd := exec.Command("sleep", "0")
	if err := cmd.Run(); err != nil {
		t.Skipf("cannot spawn: %v", err)
	}
	os.WriteFile(filepath.Join(dir, "vm1.pid"),
		[]byte(strconv.Itoa(cmd.Process.Pid)), 0644)
	if st, _ := state(); st != "stale" {
		t.Errorf("dead pid -> state must be stale, got %q", st)
	}
	if vmPidAlive() {
		t.Error("dead pid must not report alive")
	}
}

func TestPidAliveRejectsForeignProcess(t *testing.T) {
	dir := setupStateVM(t)
	// A live process that is NOT this VM (comm != vm1, not qemu*).
	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot spawn: %v", err)
	}
	defer cmd.Process.Kill()
	time.Sleep(50 * time.Millisecond)
	os.WriteFile(filepath.Join(dir, "vm1.pid"),
		[]byte(strconv.Itoa(cmd.Process.Pid)), 0644)
	if vmPidAlive() {
		t.Error("live but foreign pid (sleep) must be rejected by comm identity")
	}
}

func TestPidAliveAcceptsVMNamedProcess(t *testing.T) {
	dir := setupStateVM(t)
	// Spawn a binary whose comm matches the VM name — same trick the
	// Bash spec used (QEMU renames itself via -name ...,process=<vm>).
	fake := filepath.Join(dir, "vm1")
	src, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("no sleep binary")
	}
	b, _ := os.ReadFile(src)
	os.WriteFile(fake, b, 0755)
	cmd := exec.Command(fake, "60")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot spawn: %v", err)
	}
	defer cmd.Process.Kill()
	time.Sleep(50 * time.Millisecond)
	os.WriteFile(filepath.Join(dir, "vm1.pid"),
		[]byte(strconv.Itoa(cmd.Process.Pid)), 0644)
	if !vmPidAlive() {
		t.Error("comm==vmname pid must report alive")
	}
	if st, _ := state(); st != "running" {
		t.Errorf("live vm-named pid -> running, got %q", st)
	}
}

func TestPidlessAgedLockReclaimed(t *testing.T) {
	setupStateVM(t)
	lockDir := filepath.Join(vmdir(), "vm1.lock")
	os.MkdirAll(filepath.Join(lockDir, "nested"), 0755)
	// Age the lock past the 2s pidless window.
	old := time.Now().Add(-3 * time.Second)
	os.Chtimes(lockDir, old, old)
	if !vmLockAcquire() {
		t.Error("pidless lock older than the grace window must be reclaimed")
	}
	defer vmLockRelease()
}

func TestStateArchivedViaTrashDir(t *testing.T) {
	setupStateVM(t)
	trash := t.TempDir()
	if err := os.MkdirAll(filepath.Join(trash, "vm1"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("QE_TRASH_DIR", trash)
	if st, _ := state(); st != "archived" {
		t.Errorf("QE_TRASH_DIR/<vm> dir must yield archived, got %q", st)
	}
}

func TestEventsRotate(t *testing.T) {
	dir := setupStateVM(t)
	t.Setenv("QE_EVENTS_MAX_BYTES", "10")
	f := filepath.Join(dir, "vm1.events.jsonl")
	if err := os.WriteFile(f, []byte("{\"seq\":9}\n123456789012"), 0644); err != nil {
		t.Fatal(err)
	}
	qeEvent("vm.test", "")
	if _, err := os.Stat(f + ".1"); err != nil {
		t.Fatal("oversized events file was not rotated to .1")
	}
	b, _ := os.ReadFile(f)
	if !strings.Contains(string(b), "\"seq\":1") {
		t.Errorf("post-rotation file must restart seq, got %q", b)
	}
}

func TestInventoryTouch(t *testing.T) {
	setupStateVM(t)
	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)
	t.Setenv("QE_INSTANCE_ID", "inst-42")
	inventoryTouch("running")
	b, err := os.ReadFile(filepath.Join(xdg, "quickemu", "vms.json"))
	if err != nil {
		t.Fatalf("vms.json not written: %v", err)
	}
	var m map[string]struct {
		VM, State string
	}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	e, ok := m["inst-42"]
	if !ok || e.VM != "vm1" || e.State != "running" {
		t.Errorf("inventory entry wrong: %+v", m)
	}
}
