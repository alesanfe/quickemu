// verify_test.go — the remaining verify branches: uid, start-time,
// comm (incl. qemu-prefix and 15-char truncation), exe basename,
// plus the usage/invalid-pid paths.
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestHelperProcess is not a test — a copy of this binary run with
// GO_QE_FAKE_PROC=1 sleeps, standing in for a real process. Copying
// /bin/sleep to a chosen name dies on coreutils-multicall hosts,
// which dispatch on argv[0]; a copy of this binary does not.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_QE_FAKE_PROC") != "1" {
		return
	}
	time.Sleep(3 * time.Minute)
}

// spawnSleep plants a copy of this test binary under a chosen name
// so verify can match on comm and exe basename.
func spawnSleep(t *testing.T, name string) (pid int, path string, kill func()) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Skip("cannot resolve test binary")
	}
	b, err := os.ReadFile(self)
	if err != nil {
		t.Skip("read test binary failed")
	}
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, b, 0o755); err != nil {
		t.Skip("copy failed")
	}
	c := exec.Command(p, "-test.run=^TestHelperProcess$")
	c.Env = append(os.Environ(), "GO_QE_FAKE_PROC=1")
	if err := c.Start(); err != nil {
		t.Skip("spawn failed")
	}
	return c.Process.Pid, p, func() { c.Process.Kill(); c.Wait() }
}

func verifyArgs(t *testing.T, args ...string) (string, int) {
	t.Helper()
	return runTool(t, args...)
}

func TestVerifyFlagBranches(t *testing.T) {
	pid, _, kill := spawnSleep(t, "myvmproc")
	defer kill()
	sp := strconv.Itoa(pid)

	// uid match: our own uid.
	out, rc := verifyArgs(t, "verify", "--pid", sp, "--uid",
		strconv.Itoa(os.Getuid()))
	if rc != 0 || !strings.Contains(out, `"success":true`) {
		t.Fatalf("uid match rc=%d %s", rc, out)
	}
	// uid mismatch → UID_MISMATCH in message/code envelope.
	out, rc = verifyArgs(t, "verify", "--pid", sp, "--uid", "999999")
	if rc == 0 || !strings.Contains(out, "MISMATCH") {
		t.Fatalf("uid mismatch %s", out)
	}
	// comm exact.
	out, rc = verifyArgs(t, "verify", "--pid", sp, "--comm", "myvmproc")
	if rc != 0 {
		t.Fatalf("comm rc=%d %s", rc, out)
	}
	// comm mismatch non-qemu.
	out, rc = verifyArgs(t, "verify", "--pid", sp, "--comm", "othername")
	if rc == 0 || !strings.Contains(out, "MISMATCH") {
		t.Fatalf("comm mismatch %s", out)
	}
	// exe basename contains our name.
	out, rc = verifyArgs(t, "verify", "--pid", sp, "--exe", "myvmproc")
	if rc != 0 {
		t.Fatalf("exe rc=%d %s", rc, out)
	}
	out, rc = verifyArgs(t, "verify", "--pid", sp, "--exe", "otherexe")
	if rc == 0 || !strings.Contains(out, "MISMATCH") {
		t.Fatalf("exe mismatch %s", out)
	}
	// start-time: wrong value → mismatch.
	out, rc = verifyArgs(t, "verify", "--pid", sp, "--start-time", "1")
	if rc == 0 {
		t.Fatalf("starttime %s", out)
	}
	// dead pid.
	out, rc = verifyArgs(t, "verify", "--pid", "99999999")
	if rc == 0 || !strings.Contains(out, "DEAD") && !strings.Contains(out, "VERIFY_FAILED") {
		t.Fatalf("dead %s", out)
	}
	// invalid/missing pid → PID_INVALID/usage.
	if _, rc = verifyArgs(t, "verify", "--pid", "0"); rc == 0 {
		t.Fatal("pid 0")
	}
	if _, rc = verifyArgs(t); rc != 2 {
		t.Fatal("usage")
	}
	if _, rc = verifyArgs(t, "bogus"); rc != 2 {
		t.Fatal("bad sub")
	}
}
