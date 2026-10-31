// fingerprint_test.go: QE-001 — reservation field 5 is the owner's
// recorded start-time: a live pid whose start-time changed was
// recycled and cannot hold the lease (same rule as the VM lock).
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestStaleRecycledStartTime(t *testing.T) {
	pid := os.Getpid()
	st := procStartTime(pid)
	if st == "" {
		t.Skip("/proc start-time unreadable")
	}
	// Correct fingerprint → not stale.
	okMeta := fmt.Sprintf("%d %d vm1 - %s", pid, time.Now().Unix(), st)
	if stale(okMeta) {
		t.Error("live owner + correct start-time must not be stale")
	}
	// Recycled pid: same live pid, someone else's start-time.
	badMeta := fmt.Sprintf("%d %d vm1 - 999999999", pid, time.Now().Unix())
	if !stale(badMeta) {
		t.Error("live owner + wrong start-time (recycled) must be stale")
	}
	// "-" disables the check (legacy records without field 5).
	legacy := fmt.Sprintf("%d %d vm1 - -", pid, time.Now().Unix())
	if stale(legacy) {
		t.Error("field5 '-' must disable the fingerprint check")
	}
}

// reserve --start-time writes field 5 — with the lease slot padded
// to "-" when no expiry was requested (positional layout kept).
func TestReserveStartTimeField(t *testing.T) {
	reg := t.TempDir()
	out, _, rc := runTool(t, "reserve", "--registry", reg, "--port", "54321",
		"--pid", strconv.Itoa(os.Getpid()), "--instance", "vm1",
		"--start-time", "12345")
	if rc != 0 {
		t.Fatalf("reserve rc=%d %s", rc, out)
	}
	b, err := os.ReadFile(filepath.Join(reg, "54321"))
	if err != nil {
		t.Fatal(err)
	}
	f := strings.Fields(string(b))
	if len(f) != 5 || f[3] != "-" || f[4] != "12345" {
		t.Fatalf("record must be 'pid ts inst - starttime', got %q", b)
	}
	// A live pid whose record claims start-time 12345 is stale by
	// definition — the recycle detector sweeps it on the next pass.
	out, _, rc = runTool(t, "list", "--registry", reg)
	if rc != 0 || strings.Contains(out, "54321") {
		t.Fatalf("stale-fingerprint reservation must be swept, list=%q rc=%d", out, rc)
	}
}
