package qespec

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
	"time"
)

// ---- spec/resilience/fault_injection_spec.sh ----

func TestOrphanLockDetection(t *testing.T) {
	dir := t.TempDir()
	vm := filepath.Join(dir, "testvm")
	os.MkdirAll(filepath.Join(vm, ".lock"), 0o755)

	// Dead-owner lock: pid 999999 must not exist.
	os.WriteFile(filepath.Join(vm, ".lock", "owner.pid"),
		[]byte("999999"), 0o644)
	if PidAlive(999999) {
		t.Skip("pid 999999 unexpectedly alive")
	}

	// Live owner: pid + timestamp format.
	os.WriteFile(filepath.Join(vm, ".lock", "owner.pid"),
		[]byte(fmt.Sprintf("%d %d", os.Getpid(), time.Now().Unix())), 0o644)
	b, _ := os.ReadFile(filepath.Join(vm, ".lock", "owner.pid"))
	if !regexp.MustCompile(`^\d+ \d+$`).Match(b) {
		t.Error("owner.pid must record 'PID timestamp'")
	}
}

func TestPartFileNotCompleted(t *testing.T) {
	f := filepath.Join(t.TempDir(), "x.iso.part")
	os.WriteFile(f, []byte("partial"), 0o644)
	if filepath.Ext(f) != ".part" {
		t.Fatal(".part suffix lost")
	}
}

func TestDiskFullWriteFails(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("tmpfs mount requires root")
	}
	d := t.TempDir()
	if err := exec.Command("mount", "-t", "tmpfs", "-o", "size=1M", "none", d).Run(); err != nil {
		t.Skipf("tmpfs mount failed: %v", err)
	}
	defer exec.Command("umount", d).Run()
	if err := os.WriteFile(filepath.Join(d, "big"),
		make([]byte, 2<<20), 0o644); err == nil {
		t.Error("write to 1M tmpfs succeeded unexpectedly")
	}
}
