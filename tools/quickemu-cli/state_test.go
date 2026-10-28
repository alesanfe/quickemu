// state_test.go: lifecycle/state-machine contract — stale detection,
// foreign-pid rejection, pidless-lock reclaim (lifecycle_machine /
// chaos / adversarial spec parity, ported to the Go internals).
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
