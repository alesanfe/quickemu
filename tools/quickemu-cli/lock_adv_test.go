// lock_adv_test.go: adversarial vm_lock_* cases — corrupt pid
// payloads, missing pid files, lock age. The security property under
// test: a lock must never be reclaimed while its owner can still be
// alive (or while the writer could be mid-creation).
package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func ageLock(t *testing.T, lockDir string, d time.Duration) {
	t.Helper()
	past := time.Now().Add(-d)
	if err := os.Chtimes(lockDir, past, past); err != nil {
		t.Fatal(err)
	}
}

// plantLock creates <vmdir>/c.lock with a pid file containing
// pidContent (or none when hasPid is false), optionally backdated.
func plantLock(t *testing.T, pidContent string, hasPid bool, age time.Duration) string {
	t.Helper()
	lockDir := setupLockVM(t)
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if hasPid {
		if err := os.WriteFile(filepath.Join(lockDir, "pid"), []byte(pidContent), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if age > 0 {
		ageLock(t, lockDir, age)
	}
	return lockDir
}

func TestCorruptPidFreshLockNotReclaimed(t *testing.T) {
	// Garbage/empty pid + fresh lock: the writer may be between mkdir
	// and the pid write — reclaiming now could race the real owner.
	for _, pid := range []string{"garbage", "-5", "", "1.5", "0"} {
		plantLock(t, pid, true, 0)
		if vmLockAcquire() {
			vmLockRelease()
			t.Errorf("pid %q: fresh corrupt lock must not be reclaimed", pid)
		}
	}
}

func TestCorruptPidAgedLockReclaimed(t *testing.T) {
	for _, pid := range []string{"garbage", "-5", "", "1.5", "0"} {
		plantLock(t, pid, true, 3*time.Second)
		if !vmLockAcquire() {
			t.Errorf("pid %q: aged corrupt lock should be reclaimed", pid)
		}
		vmLockRelease()
	}
}

func TestMissingPidLock(t *testing.T) {
	// No pid file, fresh lock → mid-creation window → refuse.
	plantLock(t, "", false, 0)
	if vmLockAcquire() {
		vmLockRelease()
		t.Error("pid-less fresh lock must not be reclaimed")
	}
	// Same, but aged past the creation window → crash remnant → take.
	plantLock(t, "", false, 3*time.Second)
	if !vmLockAcquire() {
		t.Error("pid-less aged lock should be reclaimed")
	}
	vmLockRelease()
}

func TestHugeDeadPidReclaimed(t *testing.T) {
	// PID beyond the kernel max cannot exist → dead owner, immediate
	// reclaim (no age window needed).
	plantLock(t, "99999999", true, 0)
	if !vmLockAcquire() {
		t.Error("impossibly-large pid should be reclaimed as dead")
	}
	vmLockRelease()
}

func TestLiveForeignPidNeverReclaimed(t *testing.T) {
	// pid 1 always exists; EPERM-aware Alive() treats it as alive for
	// a non-root caller — an aged lock must STILL be refused.
	plantLock(t, "1", true, 3*time.Second)
	if vmLockAcquire() {
		vmLockRelease()
		t.Error("live foreign owner's lock must never be reclaimed, even when old")
	}
}
