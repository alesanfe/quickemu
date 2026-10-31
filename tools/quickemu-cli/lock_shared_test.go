// lock_shared_test.go: QE-002 shared/exclusive lock contract —
// shared.<pid> files under <vm>.lock.shared vs the <vm>.lock mkdir.
// Cross-process mutual exclusion, fingerprint-based orphan reclaim,
// configurable timeout and the holder detail in lock errors.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alesanfe/quickemu/tools/qelib/qetest"
)

// liveForeignProc returns a pid that is alive but is NOT this test
// binary — a foreign claimant for planted lock records — plus its
// kill switch so tests can end the "process" mid-scenario.
func liveForeignProc(t *testing.T) (int, func()) {
	t.Helper()
	c := exec.Command("sleep", "60")
	if err := c.Start(); err != nil {
		t.Skip("no sleep binary")
	}
	kill := func() { c.Process.Kill(); c.Wait() }
	t.Cleanup(kill)
	time.Sleep(30 * time.Millisecond)
	return c.Process.Pid, kill
}

// plantShared writes a shared.<pid> claim with the given content.
func plantShared(t *testing.T, name, meta string) {
	t.Helper()
	sd := lockSharedDir()
	if err := os.MkdirAll(sd, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sd, name), []byte(meta), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestSharedLockBasics(t *testing.T) {
	setupLockVM(t)
	if !vmLockAcquireShared("status") {
		t.Fatal("shared acquire must succeed on a free VM")
	}
	mine := filepath.Join(lockSharedDir(), fmt.Sprintf("shared.%d", os.Getpid()))
	b, err := os.ReadFile(mine)
	if err != nil {
		t.Fatalf("shared.%d file missing: %v", os.Getpid(), err)
	}
	f := strings.Fields(string(b))
	if len(f) < 3 || f[0] != strconv.Itoa(os.Getpid()) || f[2] != "status" {
		t.Fatalf("shared record wrong: %q", b)
	}
	// Nested acquire is idempotent (same pid re-enters).
	if !vmLockAcquireShared("info") {
		t.Fatal("nested shared acquire must succeed")
	}
	vmLockReleaseShared()
	if _, err := os.Stat(lockSharedDir()); !os.IsNotExist(err) {
		t.Error("last shared release must remove the shared lock dir")
	}
}

// A live foreign shared holder blocks the exclusive lock; once its
// file goes stale (dead pid / wrong start-time) the lock frees.
func TestExclusiveBlockedBySharedHolder(t *testing.T) {
	setupLockVM(t)
	fp, kill := liveForeignProc(t)
	plantShared(t, fmt.Sprintf("shared.%d", fp),
		fmt.Sprintf("%d %s %s", fp, procStartTimeJ(fp), "status"))
	if vmLockAcquireOp("delete") {
		vmLockRelease()
		t.Fatal("exclusive acquire must refuse while a live shared holder exists")
	}
	// Holder dies → file swept → exclusive takes over.
	kill()
	if !vmLockAcquireOp("delete") {
		t.Fatal("exclusive acquire must reclaim dead shared claims")
	}
	defer vmLockRelease()
}

// A recycled pid must never hold a lock: same live pid, wrong
// start-time in the record → stale, swept, lock proceeds (QE-001
// fingerprint applied to lock owners).
func TestSharedHolderRejectsRecycledPid(t *testing.T) {
	setupLockVM(t)
	fp, _ := liveForeignProc(t)
	plantShared(t, fmt.Sprintf("shared.%d", fp), fmt.Sprintf("%d 999999999 status", fp))
	if !vmLockAcquireOp("delete") {
		t.Fatal("stale-fingerprint shared claim must be reclaimed")
	}
	defer vmLockRelease()
}

// The same fingerprint rule applies to the exclusive "owner" record:
// a live pid with a recycled start-time is a stale writer.
func TestExclusiveOwnerRejectsRecycledPid(t *testing.T) {
	lockDir := setupLockVM(t)
	fp, _ := liveForeignProc(t)
	if err := os.MkdirAll(lockDir, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(lockDir, "pid"), []byte(strconv.Itoa(fp)), 0644)
	os.WriteFile(filepath.Join(lockDir, "owner"),
		[]byte(fmt.Sprintf("%d 999999999 delete", fp)), 0644)
	if !vmLockAcquireOp("stop") {
		t.Fatal("exclusive owner with recycled start-time must be reclaimed")
	}
	defer vmLockRelease()
}

// A verified-live exclusive owner blocks shared readers — and the
// error names the holding operation.
func TestSharedBlockedByExclusive(t *testing.T) {
	setupLockVM(t)
	if !vmLockAcquireOp("delete") {
		t.Fatal("exclusive acquire")
	}
	defer vmLockRelease()
	t.Setenv("VM_LOCK_TRIES", "2")
	errOut := qetest.CaptureStderr(t, func() {
		if vmLockAcquireShared("status") {
			t.Error("shared acquire must refuse a live exclusive owner")
		}
	})
	if !strings.Contains(errOut, "locked") || !strings.Contains(errOut, "delete") {
		t.Fatalf("lock error must name the holding op, got %q", errOut)
	}
}

// Multi-process mutual exclusion: a helper child holds the shared
// lock — the parent's exclusive acquire refuses, its shared acquire
// joins the same lock (concurrent readers).
func TestSharedExclusiveMultiProcess(t *testing.T) {
	if os.Getenv("QE_LOCK_CHILD") == "1" {
		// Child side: point vmConf at the parent's VM, hold the shared
		// lock briefly so the parent can contend — the pid in the
		// filename is OURS, no planting.
		vmConf = os.Getenv("QE_LOCK_VM")
		if !vmLockAcquireShared("child-reader") {
			os.Exit(3)
		}
		fmt.Println("HELD")
		time.Sleep(1500 * time.Millisecond)
		vmLockReleaseShared()
		os.Exit(0)
	}
	setupLockVM(t)
	child := exec.Command(os.Args[0], "-test.run=TestSharedExclusiveMultiProcess")
	child.Env = append(os.Environ(), "QE_LOCK_CHILD=1",
		"QE_LOCK_VM="+vmConf)
	out, err := child.StdoutPipe()
	if err != nil || child.Start() != nil {
		t.Skip("cannot spawn helper")
	}
	defer child.Process.Kill()
	// Wait for HELD — the child signal that its file exists.
	go func() {
		buf := make([]byte, 8)
		out.Read(buf)
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(lockSharedDir()); err == nil {
			if ents, _ := os.ReadDir(lockSharedDir()); len(ents) > 0 {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Skip("child did not take the shared lock")
		}
		time.Sleep(20 * time.Millisecond)
	}
	// Exclusive blocked by the child's shared claim.
	t.Setenv("VM_LOCK_TRIES", "1")
	if vmLockAcquireOp("delete") {
		vmLockRelease()
		t.Fatal("exclusive must refuse while a live child holds shared")
	}
	// Shared joins concurrently — readers never starve readers.
	if !vmLockAcquireShared("status") {
		t.Fatal("shared must join a live shared lock")
	}
	vmLockReleaseShared()
	child.Wait()
}

// QE_LOCK_TIMEOUT configures the wait window; a 0.1s budget fails
// fast against a live exclusive holder.
func TestLockTimeoutEnv(t *testing.T) {
	setupLockVM(t)
	if !vmLockAcquireOp("stop") {
		t.Fatal("acquire")
	}
	defer vmLockRelease()
	t.Setenv("QE_LOCK_TIMEOUT", "0.1")
	start := time.Now()
	if vmLockAcquireShared("status") {
		t.Fatal("shared must refuse under a live exclusive lock")
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("timeout honoured too loosely: %v", d)
	}
}

// The exclusive lock's owner record names the op — the lock error
// reports "held by <op> (pid N)" for diagnostics.
func TestExclusiveHolderReport(t *testing.T) {
	lockDir := setupLockVM(t)
	fp, _ := liveForeignProc(t)
	if err := os.MkdirAll(lockDir, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(lockDir, "pid"), []byte(strconv.Itoa(fp)), 0644)
	os.WriteFile(filepath.Join(lockDir, "owner"),
		[]byte(fmt.Sprintf("%d %s export", fp, procStartTimeJ(fp))), 0644)
	got := exclusiveHolder(lockDir)
	if !strings.Contains(got, "export") || !strings.Contains(got, strconv.Itoa(fp)) {
		t.Fatalf("holder report %q must name op+pid", got)
	}
	// sharedHolderDesc on live shared claims.
	plantShared(t, fmt.Sprintf("shared.%d", fp),
		fmt.Sprintf("%d %s status", fp, procStartTimeJ(fp)))
	if d := sharedHolderDesc(); !strings.Contains(d, "status") {
		t.Fatalf("shared holder report %q", d)
	}
}
