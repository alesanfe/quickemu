// fingerprint_test.go: QE-001 strong process identity — runtime.json
// must pin pid+start-time+exe+uid+conf+vmid+args-hash, and a live pid
// whose /proc evidence contradicts the record is never trusted:
// recycled pids, swapped binaries, forged argv and tampered state
// files all map to "unknown" (ops refuse to act on them).
package main

import (
	"crypto/sha256"
	"encoding/hex"
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

// writeRuntimeFull plants a runtime.json with the strong-fingerprint
// fields; pass "" to omit a field entirely.
func writeRuntimeFull(t *testing.T, dir, name string, pid int, startTime,
	exe, argsHash, cmdFp, confPath, vmId string) {
	t.Helper()
	rec := map[string]any{
		"schemaVersion": 1, "pid": pid, "uid": os.Getuid(),
		"processStartTime": startTime, "createdAt": "t",
	}
	if exe != "" {
		rec["exe"] = exe
	}
	if argsHash != "" {
		rec["argsHash"] = argsHash
	}
	if cmdFp != "" {
		rec["commandFingerprint"] = cmdFp
	}
	if confPath != "" {
		rec["configPath"] = confPath
	}
	if vmId != "" {
		rec["vmId"] = vmId
	}
	b, _ := json.Marshal(rec)
	if err := os.WriteFile(filepath.Join(dir, name+".runtime.json"),
		append(b, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
}

func readRuntime(t *testing.T, dir, name string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name+".runtime.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// TestFingerprintRejectsRecycledPid: the canonical PID-reuse case —
// process A's fingerprint is recorded, A dies, process B (same comm,
// same pid slot in the pidfile) inherits the record. The recorded
// start-time/args hash belong to A, so B must be refused even though
// it is alive and vm-named.
func TestFingerprintRejectsRecycledPid(t *testing.T) {
	dir := setupStateVM(t)
	a := fakeVMProc(t, dir, "vm1")
	pidA := a.Process.Pid
	stA := procStartTimeJ(pidA)
	a.Process.Kill()
	a.Wait()
	// B takes over the instance: new pid in the pidfile, but the
	// runtime record carries A's start-time — the kernel's jiffies
	// are the one value a recycled pid can never reproduce (same
	// argv also means the args hash is identical, so start-time is
	// the only detector here).
	b := fakeVMProc(t, dir, "vm1")
	pidB := b.Process.Pid
	os.WriteFile(filepath.Join(dir, "vm1.pid"),
		[]byte(strconv.Itoa(pidB)), 0644)
	writeRuntimeFull(t, dir, "vm1", pidB, stA,
		mustExe(t, pidB), procArgsHash(pidB), "", "", "")
	if st, _ := instanceVerify(); st != "pid-reused" {
		t.Fatalf("recycled pid must be refused: %q want pid-reused", st)
	}
	if s, _ := state(); s != "unknown" {
		t.Errorf("recycled pid -> state unknown, got %q", s)
	}
}

// TestFingerprintArgsHashMismatch: same start-time, wrong args hash —
// a forged cmdline (different argv) must fail the hash check even
// when every weaker proof passes.
func TestFingerprintArgsHashMismatch(t *testing.T) {
	dir := setupStateVM(t)
	cmd := fakeVMProc(t, dir, "vm1")
	pid := cmd.Process.Pid
	os.WriteFile(filepath.Join(dir, "vm1.pid"),
		[]byte(strconv.Itoa(pid)), 0644)
	writeRuntimeFull(t, dir, "vm1", pid, procStartTimeJ(pid),
		mustExe(t, pid), "sha256:"+strings.Repeat("0", 64), "", "", "")
	if st, _ := instanceVerify(); st != "fingerprint-mismatch" {
		t.Fatalf("wrong argsHash -> %q want fingerprint-mismatch", st)
	}
	if vmPidAlive() {
		t.Error("vmPidAlive must refuse a fingerprint-mismatched pid")
	}
	if s, _ := state(); s != "unknown" {
		t.Errorf("fingerprint mismatch -> state unknown, got %q", s)
	}
}

// TestFingerprintExeMismatch: the recorded real exe path no longer
// resolves to the running binary — a swapped/relocated image is not
// the process the record describes.
func TestFingerprintExeMismatch(t *testing.T) {
	dir := setupStateVM(t)
	cmd := fakeVMProc(t, dir, "vm1")
	pid := cmd.Process.Pid
	os.WriteFile(filepath.Join(dir, "vm1.pid"),
		[]byte(strconv.Itoa(pid)), 0644)
	writeRuntimeFull(t, dir, "vm1", pid, procStartTimeJ(pid),
		"/usr/bin/some-other-qemu", "", "", "", "")
	if st, _ := instanceVerify(); st != "exe-mismatch" {
		t.Fatalf("wrong exe -> %q want exe-mismatch", st)
	}
}

// TestFingerprintExeDeleted: binary unlinked after exec — the record
// can never be corroborated on disk, so the process is unverifiable.
func TestFingerprintExeDeleted(t *testing.T) {
	dir := setupStateVM(t)
	src, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("no sleep binary")
	}
	fake := filepath.Join(dir, "vm1")
	if b, _ := os.ReadFile(src); os.WriteFile(fake, b, 0755) != nil {
		t.Skip("cannot plant fake binary")
	}
	cmd := exec.Command(fake, "60")
	if err := cmd.Start(); err != nil {
		t.Skip("spawn failed")
	}
	t.Cleanup(func() { cmd.Process.Kill() })
	time.Sleep(50 * time.Millisecond)
	if err := os.Remove(fake); err != nil {
		t.Skip("cannot unlink binary")
	}
	pid := cmd.Process.Pid
	if _, deleted := procExe(pid); !deleted {
		t.Skip("platform does not report deleted exe links")
	}
	os.WriteFile(filepath.Join(dir, "vm1.pid"),
		[]byte(strconv.Itoa(pid)), 0644)
	writeRuntimeFull(t, dir, "vm1", pid, procStartTimeJ(pid),
		"", procArgsHash(pid), "", "", "")
	if st, _ := instanceVerify(); st != "exe-deleted" {
		t.Fatalf("deleted binary -> %q want exe-deleted", st)
	}
}

// TestFingerprintCommandFp: the launcher's sha256(argv0|conf|vmId)
// fingerprint recomputes against the live cmdline — a record naming
// a different conf or vm must fail.
func TestFingerprintCommandFp(t *testing.T) {
	dir := setupStateVM(t)
	cmd := fakeVMProc(t, dir, "vm1")
	pid := cmd.Process.Pid
	os.WriteFile(filepath.Join(dir, "vm1.pid"),
		[]byte(strconv.Itoa(pid)), 0644)
	a0 := cmdlineArgv0(pid)
	confPath := "/vms/vm1/vm1.conf"
	sum := sha256.Sum256([]byte(a0 + "|" + confPath + "|vm1"))
	fp := "sha256:" + hex.EncodeToString(sum[:])
	writeRuntimeFull(t, dir, "vm1", pid, procStartTimeJ(pid),
		"", "", fp, confPath, "vm1")
	if st, _ := instanceVerify(); st != "alive" {
		t.Fatalf("correct command fingerprint -> %q want alive", st)
	}
	// Same process, record claiming a different VM — hash refuses.
	writeRuntimeFull(t, dir, "vm1", pid, procStartTimeJ(pid),
		"", "", fp, confPath, "vm2")
	if st, _ := instanceVerify(); st != "fingerprint-mismatch" {
		t.Fatalf("wrong vmId -> %q want fingerprint-mismatch", st)
	}
}

// TestFingerprintAugmentRecords: after a successful verify the CLI
// augments the record with the real exe path and the full-args hash —
// the fields the Bash runtime.json predates — and keeps them
// consistent on later verifies.
func TestFingerprintAugmentRecords(t *testing.T) {
	dir := setupStateVM(t)
	cmd := fakeVMProc(t, dir, "vm1")
	pid := cmd.Process.Pid
	os.WriteFile(filepath.Join(dir, "vm1.pid"),
		[]byte(strconv.Itoa(pid)), 0644)
	// Bare record: pid + start-time only (Bash-era minimum).
	writeRuntimeFull(t, dir, "vm1", pid, procStartTimeJ(pid),
		"", "", "", "", "")
	if st, _ := instanceVerify(); st != "alive" {
		t.Fatalf("verify -> %q", st)
	}
	m := readRuntime(t, dir, "vm1")
	ex, _ := procExe(pid)
	if m["exe"] != ex {
		t.Fatalf("augment recorded exe %v want %v", m["exe"], ex)
	}
	if m["argsHash"] != procArgsHash(pid) {
		t.Fatalf("augment recorded argsHash %v", m["argsHash"])
	}
	if m["uid"] == nil || m["configPath"] == "" || m["vmId"] == "" {
		t.Fatalf("augment must backfill uid/configPath/vmId: %v", m)
	}
	// Second verify still passes against its own augmentation.
	if st, _ := instanceVerify(); st != "alive" {
		t.Fatalf("re-verify against augmented record -> %q", st)
	}
}

// TestFingerprintTamperedState: hand-edited runtime.json fields that
// disagree with /proc are rejections, not warnings — the state file
// is evidence, not authority.
func TestFingerprintTamperedState(t *testing.T) {
	dir := setupStateVM(t)
	cmd := fakeVMProc(t, dir, "vm1")
	pid := cmd.Process.Pid
	os.WriteFile(filepath.Join(dir, "vm1.pid"),
		[]byte(strconv.Itoa(pid)), 0644)
	// Legit record first, then tamper with the args hash on disk.
	writeRuntimeFull(t, dir, "vm1", pid, procStartTimeJ(pid),
		"", procArgsHash(pid), "", "", "")
	if st, _ := instanceVerify(); st != "alive" {
		t.Fatalf("baseline verify -> %q", st)
	}
	m := readRuntime(t, dir, "vm1")
	m["argsHash"] = "sha256:" + fmt.Sprintf("%064x", 1)
	b, _ := json.Marshal(m)
	os.WriteFile(filepath.Join(dir, "vm1.runtime.json"), append(b, '\n'), 0600)
	if st, _ := instanceVerify(); st != "fingerprint-mismatch" {
		t.Fatalf("tampered argsHash -> %q want fingerprint-mismatch", st)
	}
}

func mustExe(t *testing.T, pid int) string {
	t.Helper()
	ex, deleted := procExe(pid)
	if ex == "" || deleted {
		t.Skip("exe readlink unavailable")
	}
	return ex
}
