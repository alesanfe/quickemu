// state_test.go: lifecycle/state-machine contract — stale detection,
// foreign-pid rejection, pidless-lock reclaim (lifecycle_machine /
// chaos / adversarial spec parity, ported to the Go internals).
package main

import (
	"encoding/json"
	"fmt"
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
	cmd := fakeVMProc(t, dir, "vm1")
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

// TestHelperProcess is not a test — a copy of this binary run with
// GO_QE_FAKE_PROC=1 blocks forever, standing in for a QEMU process.
// Copying /bin/sleep to a VM name dies on coreutils-multicall hosts,
// which dispatch on argv[0]; a copy of this binary does not.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_QE_FAKE_PROC") != "1" {
		return
	}
	// A pending timer keeps the runtime's deadlock detector quiet —
	// a bare select{} could be reported "all goroutines are asleep".
	time.Sleep(3 * time.Minute)
}

// spawnFakeProc plants a runnable copy of this test binary at path
// and starts it as a fake process: /proc comm and cmdline both carry
// the planted name — the same signals a real QEMU shows.
func spawnFakeProc(t *testing.T, path string) *exec.Cmd {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Skip("cannot resolve test binary")
	}
	b, err := os.ReadFile(self)
	if err != nil {
		t.Skip("cannot read test binary")
	}
	if err := os.WriteFile(path, b, 0o755); err != nil {
		t.Skip("cannot plant fake binary")
	}
	cmd := exec.Command(path, "-test.run=^TestHelperProcess$")
	cmd.Env = append(os.Environ(), "GO_QE_FAKE_PROC=1")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot spawn: %v", err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	time.Sleep(50 * time.Millisecond)
	return cmd
}

// fakeVMProc spawns a comm-named process that also carries the vmdir
// token in its cmdline — the same dual proof vmProcOwned uses for a
// real quickemu qemu (-name ...,process=<vm> + drive paths).
func fakeVMProc(t *testing.T, dir, name string) *exec.Cmd {
	t.Helper()
	return spawnFakeProc(t, filepath.Join(dir, name))
}

func writeRuntime(t *testing.T, dir, name string, pid int, st string, uid int) {
	t.Helper()
	rec := fmt.Sprintf(`{"schemaVersion":1,"pid":%d,"uid":%d,"processStartTime":%s,`+
		`"instanceId":"i","vmId":%q,"configPath":"c","createdAt":"t","commandFingerprint":"f"}`+"\n",
		pid, uid, st, name)
	if err := os.WriteFile(filepath.Join(dir, name+".runtime.json"),
		[]byte(rec), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestStateUnknownForForeignPid(t *testing.T) {
	dir := setupStateVM(t)
	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot spawn: %v", err)
	}
	defer cmd.Process.Kill()
	time.Sleep(50 * time.Millisecond)
	os.WriteFile(filepath.Join(dir, "vm1.pid"),
		[]byte(strconv.Itoa(cmd.Process.Pid)), 0644)
	// Live pid that isn't this VM: vm_state reports unknown, not
	// stale (the process is alive — it just isn't ours).
	if st, _ := state(); st != "unknown" {
		t.Errorf("foreign live pid -> unknown, got %q", st)
	}
}

func TestStateRunningWithRuntimeRecord(t *testing.T) {
	dir := setupStateVM(t)
	cmd := fakeVMProc(t, dir, "vm1")
	pid := cmd.Process.Pid
	os.WriteFile(filepath.Join(dir, "vm1.pid"),
		[]byte(strconv.Itoa(pid)), 0644)
	writeRuntime(t, dir, "vm1", pid, procStartTimeJ(pid), os.Getuid())
	st, got := instanceVerify()
	if st != "alive" {
		t.Fatalf("verify: %q want alive (pid %d)", st, got)
	}
	if st, _ := state(); st != "running" {
		t.Errorf("verified live instance -> running, got %q", st)
	}
}

func TestStateUnknownOnRuntimeCorrupt(t *testing.T) {
	dir := setupStateVM(t)
	cmd := fakeVMProc(t, dir, "vm1")
	pid := cmd.Process.Pid
	os.WriteFile(filepath.Join(dir, "vm1.pid"),
		[]byte(strconv.Itoa(pid)), 0644)
	// runtime.json pid != pidfile pid → corrupt record → unknown.
	writeRuntime(t, dir, "vm1", pid+1, "0", os.Getuid())
	if st, _ := state(); st != "unknown" {
		t.Errorf("runtime pid mismatch -> unknown, got %q", st)
	}
}

func TestStateUnknownOnPidReused(t *testing.T) {
	dir := setupStateVM(t)
	cmd := fakeVMProc(t, dir, "vm1")
	pid := cmd.Process.Pid
	os.WriteFile(filepath.Join(dir, "vm1.pid"),
		[]byte(strconv.Itoa(pid)), 0644)
	writeRuntime(t, dir, "vm1", pid, "999999999", os.Getuid())
	if st, _ := state(); st != "unknown" {
		t.Errorf("starttime mismatch -> unknown, got %q", st)
	}
}

func TestStateUnknownOnUidMismatch(t *testing.T) {
	dir := setupStateVM(t)
	cmd := fakeVMProc(t, dir, "vm1")
	pid := cmd.Process.Pid
	os.WriteFile(filepath.Join(dir, "vm1.pid"),
		[]byte(strconv.Itoa(pid)), 0644)
	writeRuntime(t, dir, "vm1", pid, procStartTimeJ(pid), os.Getuid()+1)
	if st, _ := state(); st != "unknown" {
		t.Errorf("uid mismatch -> unknown, got %q", st)
	}
}

func TestStateStaleOnGarbagePidfile(t *testing.T) {
	dir := setupStateVM(t)
	os.WriteFile(filepath.Join(dir, "vm1.pid"), []byte("not-a-pid"), 0644)
	if st, _ := state(); st != "stale" {
		t.Errorf("non-numeric pidfile -> stale, got %q", st)
	}
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

func TestNormalizeArgs(t *testing.T) {
	// -x -> --x only for real flags; flag-shaped values stay verbatim.
	got := normalizeArgs([]string{"--vm", "c", "-status", "-bogus", "--extra_args", "-accel tcg", "-x"})
	want := []string{"--vm", "c", "--status", "-bogus", "--extra_args", "-accel tcg", "-x"}
	if len(got) != len(want) {
		t.Fatalf("len %v != %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("arg %d: %q != %q", i, got[i], want[i])
		}
	}
	// --k=v splits; the v half is verbatim.
	got = normalizeArgs([]string{"--format=json", "--extra_args=-cpu max"})
	want = []string{"--format", "json", "--extra_args", "-cpu max"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("split arg %d: %q != %q", i, got[i], want[i])
		}
	}
}
