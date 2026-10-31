// fingerprint_test.go: QE-001 — --exe-path and --args-hash extend
// verify past liveness: the real binary path and the sha256 of the
// raw cmdline pin a process so a recycled pid with different argv
// (or a swapped image) fails closed.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestVerifyExePathAndArgsHash(t *testing.T) {
	pid, kill := spawnSleep(t, "qefp")
	defer kill()
	sp := strconv.Itoa(pid)

	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil || len(raw) == 0 {
		t.Skip("/proc cmdline unreadable")
	}
	sum := sha256.Sum256(raw)
	hash := "sha256:" + hex.EncodeToString(sum[:])
	ex, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil || ex == "" {
		t.Skip("/proc exe unreadable")
	}

	// Both proofs pass for the real recorded values.
	out, rc := verifyArgs(t, "verify", "--pid", sp,
		"--exe-path", ex, "--args-hash", hash)
	if rc != 0 || !strings.Contains(out, `"success":true`) {
		t.Fatalf("correct fingerprint rc=%d %s", rc, out)
	}
	// Wrong exe path → EXE_MISMATCH.
	out, rc = verifyArgs(t, "verify", "--pid", sp,
		"--exe-path", "/usr/bin/qemu-system-x86_64")
	if rc == 0 || !strings.Contains(out, "EXE_MISMATCH") {
		t.Fatalf("wrong exe-path rc=%d %s", rc, out)
	}
	// A recycled-pid stand-in: alive, right comm, wrong argv hash.
	out, rc = verifyArgs(t, "verify", "--pid", sp,
		"--args-hash", "sha256:"+strings.Repeat("0", 64))
	if rc == 0 || !strings.Contains(out, "ARGS_MISMATCH") {
		t.Fatalf("wrong args-hash rc=%d %s", rc, out)
	}
}

// A binary unlinked after exec can never be corroborated — the
// recorded fingerprint is unverifiable, so verify fails EXE_DELETED
// rather than trust the live pid.
func TestVerifyExeDeleted(t *testing.T) {
	src, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("no sleep")
	}
	p := filepath.Join(t.TempDir(), "qedel2")
	b, _ := os.ReadFile(src)
	if err := os.WriteFile(p, b, 0o755); err != nil {
		t.Skip("copy failed")
	}
	cmd := exec.Command(p, "60")
	if err := cmd.Start(); err != nil {
		t.Skip("spawn failed")
	}
	defer func() { cmd.Process.Kill(); cmd.Wait() }()
	time.Sleep(50 * time.Millisecond)
	if err := os.Remove(p); err != nil {
		t.Skip("unlink failed")
	}
	out, rc := verifyArgs(t, "verify", "--pid",
		strconv.Itoa(cmd.Process.Pid), "--exe-path", p)
	if rc == 0 || !strings.Contains(out, "EXE_DELETED") {
		t.Fatalf("deleted exe rc=%d %s", rc, out)
	}
}

// Unreadable procfs evidence must fail closed: pid 1 (foreign-owned)
// refuses exe-path verification; an empty cmdline (kernel thread /
// zombie) refuses the args hash; an invalid pid and a dead pid are
// both plain refusals.
func TestVerifyRefusalBranches(t *testing.T) {
	out, rc := verifyArgs(t, "verify", "--pid", "1", "--exe-path", "/sbin/init")
	if rc == 0 || !strings.Contains(out, "EXE_") {
		t.Fatalf("foreign pid exe check rc=%d %s", rc, out)
	}
	out, rc = verifyArgs(t, "verify", "--pid", "0")
	if rc == 0 || !strings.Contains(out, "PID_INVALID") {
		t.Fatalf("bad pid rc=%d %s", rc, out)
	}
	out, rc = verifyArgs(t, "verify", "--pid", "99999999")
	if rc == 0 || !strings.Contains(out, "PROCESS_DEAD") {
		t.Fatalf("dead pid rc=%d %s", rc, out)
	}
	if _, rc = verifyArgs(t); rc == 0 {
		t.Fatal("no args must print usage and fail")
	}
}
