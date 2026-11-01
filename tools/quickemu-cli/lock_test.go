// lock_test.go: vm_lock_* contract — exclusive ownership, foreign
// refusal and stale-owner reclaim (concurrency_spec.sh parity).
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

func setupLockVM(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	vmConf = filepath.Join(dir, "c.conf")
	if err := os.WriteFile(vmConf, nil, 0644); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(vmdir(), "c.lock")
}

func TestExclusiveLockBlocksForeignOwner(t *testing.T) {
	lockDir := setupLockVM(t)
	if !vmLockAcquire() {
		t.Fatal("first acquire must succeed")
	}
	defer vmLockRelease()
	// Hand the lock to a foreign (live, not-us) owner — both records,
	// as a real foreign acquire would have written them.
	for _, f := range []string{"pid", "owner"} {
		if err := os.WriteFile(filepath.Join(lockDir, f), []byte("1"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if vmLockAcquire() {
		t.Error("second acquire must be refused while a live owner holds the lock")
	}
}

func TestExclusiveLockIsReentrantForSelf(t *testing.T) {
	lockDir := setupLockVM(t)
	if !vmLockAcquire() {
		t.Fatal("first acquire must succeed")
	}
	// A nested acquire by the verified owner (same pid + start-time)
	// must deepen — sub-ops dispatched under an act-level hold
	// (vmOpSilent, mem-snapshot flows) cannot deadlock on their own
	// claim.
	if !vmLockAcquireOp("snapshot-register") {
		t.Fatal("nested acquire by the owner must succeed, not deadlock")
	}
	vmLockRelease() // inner release — the outer hold must survive
	if _, err := os.Stat(lockDir); err != nil {
		t.Fatal("inner release must not drop the outer hold")
	}
	vmLockRelease() // outermost release frees the lock
	if _, err := os.Stat(lockDir); !os.IsNotExist(err) {
		t.Fatal("outermost release must free the lock dir")
	}
}

func TestStaleLockIsReclaimed(t *testing.T) {
	lockDir := setupLockVM(t)
	// A reaped child gives us a guaranteed-dead pid.
	cmd := exec.Command("sleep", "0")
	if err := cmd.Run(); err != nil {
		t.Skipf("cannot spawn: %v", err)
	}
	dead := cmd.Process.Pid
	if err := os.MkdirAll(lockDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lockDir, "pid"),
		[]byte(strconv.Itoa(dead)), 0644); err != nil {
		t.Fatal(err)
	}
	if !vmLockAcquire() {
		t.Error("stale lock (dead owner pid) must be reclaimed")
	}
	defer vmLockRelease()
}

func TestLockReleaseFreesDir(t *testing.T) {
	lockDir := setupLockVM(t)
	if !vmLockAcquire() {
		t.Fatal("acquire")
	}
	vmLockRelease()
	if _, err := os.Stat(lockDir); !os.IsNotExist(err) {
		t.Error("release must remove the lock dir")
	}
}
