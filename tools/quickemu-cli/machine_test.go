// machine_test.go: QE-003 — formal lifecycle state machine: valid
// transition table, persisted state+last-transition+cause+actor,
// structured transition events, and startup reconciliation that
// distinguishes qemu crash / requested stop / guest shutdown.
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func writeMachineRecord(t *testing.T, dir, state, cause string) {
	t.Helper()
	rec := machineRec{SchemaVersion: 1, State: state,
		LastTo: state, Cause: cause, Actor: "test"}
	b, _ := json.Marshal(rec)
	if err := os.WriteFile(filepath.Join(dir, "vm1.state.json"),
		append(b, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
}

func readMachineRecord(t *testing.T, dir string) machineRec {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "vm1.state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m machineRec
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// canTransition encodes the whole table — spot-check the pairs that
// guard the dangerous edges: nothing may jump straight to running
// except through starting, and terminal states can't restart alone.
func TestCanTransitionTable(t *testing.T) {
	ok := [][2]string{
		{stDefined, stPreparing}, {stPreparing, stStarting},
		{stStarting, stRunning}, {stRunning, stPaused},
		{stPaused, stRunning}, {stRunning, stStopping},
		{stStopping, stStopped}, {stRunning, stCrashed},
		{stStarting, stCrashed}, {stRunning, stSaving},
		{stSaving, stRunning}, {stSuspended, stStopped},
		{stDefined, stDeleting}, {stStopped, stDeleting},
		{stDefined, stImporting}, {stStopped, stExporting},
		{stError, stRecovering}, {stRecovering, stStopped},
		{stPaused, stSuspended}, {stSaving, stSuspended},
		{stSuspended, stStarting}, {stRunning, stSuspended},
	}
	for _, p := range ok {
		if !canTransition(p[0], p[1]) {
			t.Errorf("%s -> %s must be allowed", p[0], p[1])
		}
	}
	bad := [][2]string{
		{stDefined, stRunning}, {stStopped, stRunning},
		{stStopped, stPaused}, {stDefined, stPaused},
		{stStopped, stStarting}, {stStopped, stSaving},
		{"bogus", stRunning}, {stRunning, "bogus"},
		{stRunning, stImporting}, {stExporting, stRunning},
	}
	for _, p := range bad {
		if canTransition(p[0], p[1]) {
			t.Errorf("%s -> %s must be rejected", p[0], p[1])
		}
	}
	// error is a universal target; "" may adopt any observed state.
	if !canTransition(stDefined, stError) || !canTransition("", stRunning) {
		t.Error("error target / empty-source adoption must hold")
	}
}

func TestMachineTransitionPersists(t *testing.T) {
	dir := setupStateVM(t)
	writeMachineRecord(t, dir, stRunning, "")
	if !machineTransition(stPaused, "pause", "tester") {
		t.Fatal("running -> paused must succeed")
	}
	m := readMachineRecord(t, dir)
	if m.State != stPaused || m.LastFrom != stRunning ||
		m.LastTo != stPaused || m.Cause != "pause" || m.Actor != "tester" {
		t.Fatalf("record not persisted: %+v", m)
	}
	// Invalid transition refused — record untouched.
	if machineTransition(stDeleting, "delete", "tester") {
		t.Fatal("paused -> deleting must be refused")
	}
	m = readMachineRecord(t, dir)
	if m.State != stPaused {
		t.Fatalf("invalid transition mutated state: %+v", m)
	}
	// Events: transition + rejection both logged to jsonl.
	ev, _ := os.ReadFile(filepath.Join(dir, "vm1.events.jsonl"))
	s := string(ev)
	if !strings.Contains(s, `"vm.transition"`) ||
		!strings.Contains(s, `"to":"`+stPaused+`"`) {
		t.Fatalf("transition event missing: %s", s)
	}
}

// machineReconcile — the boot-time truth table: a dead pidfile pid
// with the machine recorded running is a qemu crash — exactly the
// wrapper kill -9 case (nothing witnessed the exit).
func TestMachineReconcileCrashAfterKill9(t *testing.T) {
	dir := setupStateVM(t)
	writeMachineRecord(t, dir, stRunning, "")
	os.WriteFile(filepath.Join(dir, "vm1.pid"), []byte("99999999"), 0644)
	if got := machineReconcile(); got != stCrashed {
		t.Fatalf("dead qemu + recorded running -> %q want crashed", got)
	}
	m := readMachineRecord(t, dir)
	if m.Cause != "process-died" {
		t.Fatalf("crash cause must be recorded, got %+v", m)
	}
	// The recovery arc ran: a recovering marker event then crashed.
	ev, _ := os.ReadFile(filepath.Join(dir, "vm1.events.jsonl"))
	if !strings.Contains(string(ev), stRecovering) {
		t.Error("crash path must pass through recovering")
	}
}

// A requested stop that completed while the recorder was still up
// (recorded stopping, cause set) is NOT a crash — it lands stopped.
func TestMachineReconcileOrderlyStop(t *testing.T) {
	dir := setupStateVM(t)
	writeMachineRecord(t, dir, stStopping, "user-kill")
	os.WriteFile(filepath.Join(dir, "vm1.pid"), []byte("99999999"), 0644)
	if got := machineReconcile(); got != stStopped {
		t.Fatalf("dead pid + recorded stopping -> %q want stopped", got)
	}
	m := readMachineRecord(t, dir)
	if !strings.HasPrefix(m.Cause, "stop-completed") {
		t.Fatalf("requested stop must record stop-completed, got %+v", m)
	}
}

// No machine memory at all + a stale pidfile — the bare crash
// remnant case — reconciles to crashed, not silently stopped.
func TestMachineReconcileBareStaleIsCrash(t *testing.T) {
	dir := setupStateVM(t)
	os.WriteFile(filepath.Join(dir, "vm1.pid"), []byte("99999999"), 0644)
	if got := machineReconcile(); got != stCrashed {
		t.Fatalf("stale pidfile, no record -> %q want crashed", got)
	}
}

func TestMachineReconcileRunning(t *testing.T) {
	dir := setupStateVM(t)
	cmd := fakeVMProc(t, dir, "vm1")
	pid := cmd.Process.Pid
	writeRuntime(t, dir, "vm1", pid, procStartTimeJ(pid), os.Getuid())
	writeMachineRecord(t, dir, stRunning, "")
	os.WriteFile(filepath.Join(dir, "vm1.pid"),
		[]byte(strconv.Itoa(pid)), 0644)
	if got := machineReconcile(); got != stRunning {
		t.Fatalf("live verified qemu + recorded running -> %q want running", got)
	}
}

// The fingerprint verdict drives reconciliation too: a live but
// unverifiable pid (recycled) with the record claiming running is
// an error — the record may not keep asserting a live state it
// cannot prove (QE-001 feeds QE-003).
func TestMachineReconcileUnverifiableIsError(t *testing.T) {
	dir := setupStateVM(t)
	cmd := fakeVMProc(t, dir, "vm1")
	pid := cmd.Process.Pid
	writeRuntime(t, dir, "vm1", pid, "999999999", os.Getuid()) // reused
	writeMachineRecord(t, dir, stRunning, "")
	os.WriteFile(filepath.Join(dir, "vm1.pid"),
		[]byte(strconv.Itoa(pid)), 0644)
	if got := machineReconcile(); got != stError {
		t.Fatalf("live unverifiable pid -> %q want error", got)
	}
}

// Orphaned record with no pidfile at all adopts the observed truth
// (stopped) — the record is evidence, not authority.
func TestMachineReconcileOrphanState(t *testing.T) {
	dir := setupStateVM(t)
	writeMachineRecord(t, dir, stRunning, "")
	if got := machineReconcile(); got != stStopped {
		t.Fatalf("no pidfile + recorded running -> %q want stopped", got)
	}
}

// machineOpWrap: exclusive ops bracket themselves with an op state
// and restore the prior state on success; failure lands in error.
func TestMachineOpWrap(t *testing.T) {
	dir := setupStateVM(t)
	writeMachineRecord(t, dir, stStopped, "")
	r := machineOpWrap(stExporting, "export", func() int { return 0 })
	if r != 0 {
		t.Fatal("wrap rc")
	}
	m := readMachineRecord(t, dir)
	if m.State != stStopped || m.Cause != "export-done" {
		t.Fatalf("success must restore prev state: %+v", m)
	}
	r = machineOpWrap(stExporting, "export", func() int { return 1 })
	if r != 1 {
		t.Fatal("wrap rc passthrough")
	}
	m = readMachineRecord(t, dir)
	if m.State != stError || m.Cause != "export-failed" {
		t.Fatalf("failure must land in error: %+v", m)
	}
}

// A trashed VM (archived) never gets a machine record — reconcile
// must not resurrect it.
func TestMachineReconcileArchivedSkips(t *testing.T) {
	dir := setupStateVM(t)
	trash := t.TempDir()
	if err := os.MkdirAll(filepath.Join(trash, "vm1"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("QE_TRASH_DIR", trash)
	if got := machineReconcile(); got != "archived" {
		t.Fatalf("trashed VM -> %q want archived", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "vm1.state.json")); !os.IsNotExist(err) {
		t.Error("reconcile must not write a record for archived VMs")
	}
}
