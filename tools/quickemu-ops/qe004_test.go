// qe004_test.go: QE-004 async-ops contract — stable caller-chosen
// ids, queued phase/message/progress fields, host-restart orphan
// reaping, per-VM admission control (concurrency cap + incompatible
// serialization) and retention by age+count.
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alesanfe/quickemu/tools/qelib/qetest"
)

// runOps drives run() in-process — needed where the subprocess seam
// can't reach (procBootID fixture). Captures stdout like RunTool.
func runOps(t *testing.T, args ...string) (string, int) {
	t.Helper()
	old := os.Args
	os.Args = append([]string{"quickemu-ops"}, args...)
	t.Cleanup(func() { os.Args = old })
	rc := 0
	out := qetest.CaptureStdout(t, func() { rc = run() })
	return out, rc
}

func plantOp(t *testing.T, vmdir, id string, o *Op) {
	t.Helper()
	dir := opsDir(vmdir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	o.OperationID = id
	b, err := json.Marshal(o)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, id+".json"),
		append(b, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Stable ids: `new --id` keeps the caller's id verbatim (durable
// across restarts — a supervisor can reconnect to the same entry),
// rejects collisions, and the entry is journaled as queued with the
// phase/message/progress trio persisted.
func TestNewStableIDQueuedFields(t *testing.T) {
	vmdir := t.TempDir()
	out, rc := runTool(t, "new", "--vmdir", vmdir, "--type", "export",
		"--vm", "v", "--id", "backup-2024-01", "--phase", "prepare",
		"--message", "Preparing export")
	if rc != 0 || strings.TrimSpace(out) != "backup-2024-01" {
		t.Fatalf("new --id rc=%d out=%q", rc, out)
	}
	st := opState(t, vmdir, "backup-2024-01")
	if st["state"] != "queued" || st["phase"] != "prepare" ||
		st["message"] != "Preparing export" ||
		st["progress"].(float64) != 0 || st["bootId"] == nil {
		t.Fatalf("queued fields: %v", st)
	}
	// Id collision refused — the journal entry is unique.
	if _, rc := runTool(t, "new", "--vmdir", vmdir, "--type", "export",
		"--vm", "v", "--id", "backup-2024-01"); rc != -1 {
		t.Fatalf("duplicate id must die, rc=%d", rc)
	}
}

// set --phase/--message update only their fields; untouched fields
// persist (a worker reporting phase never clobbers progress).
func TestSetPhaseMessageProgress(t *testing.T) {
	vmdir, id := newOp(t, "v")
	if _, rc := runTool(t, "set", "--vmdir", vmdir, "--id", id,
		"--state", "running", "--phase", "copy", "--message", "Copying disk",
		"--progress", "35"); rc != 0 {
		t.Fatal("set rc")
	}
	st := opState(t, vmdir, id)
	if st["phase"] != "copy" || st["message"] != "Copying disk" ||
		st["progress"].(float64) != 35 {
		t.Fatalf("phase/message/progress: %v", st)
	}
	// Progress-only update keeps phase/message.
	if _, rc := runTool(t, "set", "--vmdir", vmdir, "--id", id,
		"--progress", "80"); rc != 0 {
		t.Fatal("set rc")
	}
	st = opState(t, vmdir, id)
	if st["phase"] != "copy" || st["progress"].(float64) != 80 {
		t.Fatalf("phase must persist across bare progress sets: %v", st)
	}
}

// Host-restart orphan reaping: an op journaled under a different
// bootId can never resume — pids recycle across boots — so recover
// fails it with the exact "host restart" cause. Same-boot ops with a
// live worker survive.
func TestRecoverHostRestart(t *testing.T) {
	vmdir := t.TempDir()
	old := procBootID
	fixture := filepath.Join(t.TempDir(), "boot_id")
	os.WriteFile(fixture, []byte("boot-2\n"), 0o644)
	procBootID = fixture
	t.Cleanup(func() { procBootID = old })

	plantOp(t, vmdir, "old", &Op{SchemaVersion: 1, Type: "export",
		VM: "v", State: "running", BootID: "boot-1",
		StartedAt: time.Now().UTC().Format(time.RFC3339)})
	// Same-boot live worker → untouched.
	plantOp(t, vmdir, "live", &Op{SchemaVersion: 1, Type: "export",
		VM: "v", State: "running", BootID: "boot-2",
		WorkerPid: os.Getpid(), // us: alive by definition
		StartedAt: time.Now().UTC().Format(time.RFC3339)})

	if _, rc := runOps(t, "recover", "--vmdir", vmdir); rc != 0 {
		t.Fatalf("recover rc=%d", rc)
	}
	oldOp := opState(t, vmdir, "old")
	if oldOp["state"] != "failed" {
		t.Fatalf("cross-boot op must fail, got %v", oldOp["state"])
	}
	var e map[string]any
	raw, _ := json.Marshal(oldOp["error"])
	json.Unmarshal(raw, &e)
	if e["code"] != "HOST_RESTART" || e["message"] != "host restart" {
		t.Fatalf("host-restart cause must read exactly 'host restart': %v", e)
	}
	live := opState(t, vmdir, "live")
	if live["state"] != "running" {
		t.Fatalf("same-boot live worker must survive: %v", live)
	}
}

// Dead-worker (same boot) reaping still works alongside the boot-id
// check — the legacy OPERATION_INTERRUPTED path is preserved.
func TestRecoverDeadWorkerStillReaped(t *testing.T) {
	vmdir := t.TempDir()
	plantOp(t, vmdir, "z", &Op{SchemaVersion: 1, Type: "export",
		VM: "v", State: "running", WorkerPid: 99999999,
		BootID:    bootID(), // same boot — only the dead pid decides
		StartedAt: time.Now().UTC().Format(time.RFC3339)})
	if _, rc := runOps(t, "recover", "--vmdir", vmdir); rc != 0 {
		t.Fatalf("recover rc=%d", rc)
	}
	if st := opState(t, vmdir, "z"); st["state"] != "failed" {
		t.Fatalf("dead worker must fail, got %v", st)
	}
}

// QE_OPS_MAX_CONCURRENT bounds active ops per VM — the refused op
// leaves no journal residue.
func TestConcurrencyLimit(t *testing.T) {
	vmdir := t.TempDir()
	if _, _, rc := qetest.RunToolFull(t, []string{"QE_OPS_MAX_CONCURRENT=1"},
		"", "new", "--vmdir", vmdir, "--type", "export", "--vm", "v"); rc != 0 {
		t.Fatal("first op must fit the limit")
	}
	out, se, rc := qetest.RunToolFull(t, []string{"QE_OPS_MAX_CONCURRENT=1"},
		"", "new", "--vmdir", vmdir, "--type", "export", "--vm", "v")
	if rc == 0 || !strings.Contains(out+se, "OPS_LIMIT") {
		t.Fatalf("second op over the cap must die OPS_LIMIT, rc=%d out=%q err=%q", rc, out, se)
	}
	// A different VM is a different bucket.
	if _, _, rc := qetest.RunToolFull(t, []string{"QE_OPS_MAX_CONCURRENT=1"},
		"", "new", "--vmdir", vmdir, "--type", "export", "--vm", "other"); rc != 0 {
		t.Fatal("limit is per-VM")
	}
}

// Incompatible-type serialization: an exclusive op (delete) blocks
// every other op on the VM, and any new exclusive op is refused
// while a non-exclusive one is active.
func TestIncompatibleSerialize(t *testing.T) {
	vmdir := t.TempDir()
	if _, rc := runTool(t, "new", "--vmdir", vmdir, "--type", "delete",
		"--vm", "v", "--id", "del"); rc != 0 {
		t.Fatal("delete op")
	}
	if _, se, rc := qetest.RunToolFull(t, nil, "",
		"new", "--vmdir", vmdir, "--type", "export", "--vm", "v"); rc == 0 {
		t.Fatalf("export under active delete must die, err=%q", se)
	}
	// Two non-exclusive ops coexist.
	vmdir2 := t.TempDir()
	if _, rc := runTool(t, "new", "--vmdir", vmdir2, "--type", "download",
		"--vm", "v", "--id", "a"); rc != 0 {
		t.Fatal("download a")
	}
	if _, rc := runTool(t, "new", "--vmdir", vmdir2, "--type", "status",
		"--vm", "v", "--id", "b"); rc != 0 {
		t.Fatal("status b — non-exclusive ops must coexist")
	}
	// Non-exclusive active + new exclusive → refused.
	if _, _, rc := qetest.RunToolFull(t, nil, "",
		"new", "--vmdir", vmdir2, "--type", "clone", "--vm", "v"); rc == 0 {
		t.Fatal("exclusive clone under active download must die")
	}
}

// Retention: `prune --days` drops aged entries; QE_OPS_KEEP +
// QE_OPS_DAYS drive the auto-sweep on `new` (journal bounded by both
// count and age).
func TestRetentionAgeAndKeep(t *testing.T) {
	vmdir := t.TempDir()
	old := time.Now().Add(-72 * time.Hour).UTC().Format(time.RFC3339)
	plantOp(t, vmdir, "ancient", &Op{SchemaVersion: 1, Type: "export",
		VM: "v", State: "completed", StartedAt: old})
	plantOp(t, vmdir, "fresh", &Op{SchemaVersion: 1, Type: "export",
		VM: "v", State: "completed",
		StartedAt: time.Now().UTC().Format(time.RFC3339)})
	out, rc := runTool(t, "prune", "--vmdir", vmdir, "--days", "1")
	if rc != 0 || !strings.Contains(out, `"pruned":1`) {
		t.Fatalf("prune --days rc=%d out=%q", rc, out)
	}
	if _, err := os.Stat(filepath.Join(opsDir(vmdir), "ancient.json")); !os.IsNotExist(err) {
		t.Error("aged op must be pruned")
	}
	if _, err := os.Stat(filepath.Join(opsDir(vmdir), "fresh.json")); err != nil {
		t.Error("fresh op must survive")
	}

	// Count bound via env auto-sweep: keep=1 drops all but the newest.
	// The new entry gets a name that sorts last — the sweep prunes by
	// name order, so a deterministic id keeps the assertion stable.
	vmdir2 := t.TempDir()
	plantOp(t, vmdir2, "a-old", &Op{SchemaVersion: 1, Type: "export",
		VM: "v", State: "completed", StartedAt: "2020-01-01T00:00:00Z"})
	if _, _, rc := qetest.RunToolFull(t, []string{"QE_OPS_KEEP=1"}, "",
		"new", "--vmdir", vmdir2, "--type", "status", "--vm", "v",
		"--id", "zzz-new"); rc != 0 {
		t.Fatal("new rc")
	}
	ents, _ := filepath.Glob(filepath.Join(opsDir(vmdir2), "*.json"))
	if len(ents) != 1 || filepath.Base(ents[0]) != "zzz-new.json" {
		t.Fatalf("QE_OPS_KEEP=1 must keep only the newest entry, got %v", ents)
	}
}
