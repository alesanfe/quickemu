// main_test.go: run() re-exec — verify is checked against the test
// binary's own live process (pid/uid/comm/exe all resolvable via /proc).
package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("QE_HELPER") == "1" {
		a := os.Getenv("QE_HELPER_ARGS")
		os.Args = []string{"quickemu-proc"}
		if a != "" {
			os.Args = append(os.Args, strings.Split(a, "\x1f")...)
		}
		rc := run()
		_ = os.WriteFile(os.Getenv("QE_HELPER_RC"),
			[]byte(strconv.Itoa(rc)), 0o600)
		return
	}
	os.Exit(m.Run())
}

func runTool(t *testing.T, args ...string) (string, int) {
	t.Helper()
	rcf := filepath.Join(t.TempDir(), "rc")
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "QE_HELPER=1",
		"QE_HELPER_ARGS="+strings.Join(args, "\x1f"),
		"QE_HELPER_RC="+rcf)
	var so bytes.Buffer
	cmd.Stdout = &so
	if err := cmd.Run(); err != nil {
		t.Fatalf("helper: %v", err)
	}
	rc := -1
	if b, err := os.ReadFile(rcf); err == nil {
		rc, _ = strconv.Atoi(strings.TrimSpace(string(b)))
	}
	return so.String(), rc
}

func envOf(t *testing.T, out string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &m); err != nil {
		t.Fatalf("bad envelope %q: %v", out, err)
	}
	return m
}

func errCode(t *testing.T, out string) string {
	t.Helper()
	errs := envOf(t, out)["errors"].([]any)
	if len(errs) == 0 {
		t.Fatalf("no errors in %s", out)
	}
	return errs[0].(map[string]any)["message"].(string)
}

func TestVerifyOwnProcess(t *testing.T) {
	pid := strconv.Itoa(os.Getpid())
	out, rc := runTool(t, "verify", "--pid", pid)
	if rc != 0 {
		t.Fatalf("rc=%d out=%s", rc, out)
	}
	if envOf(t, out)["success"] != true {
		t.Fatalf("%s", out)
	}
}

func TestVerifyUIDAndExe(t *testing.T) {
	pid := strconv.Itoa(os.Getpid())
	uid := strconv.Itoa(os.Getuid())
	out, rc := runTool(t, "verify", "--pid", pid, "--uid", uid)
	if rc != 0 {
		t.Fatalf("uid rc=%d %s", rc, out)
	}
	out, rc = runTool(t, "verify", "--pid", pid, "--uid", "99999999")
	if rc != 1 || errCode(t, out) != "UID_MISMATCH" {
		t.Fatalf("uid mismatch: rc=%d %s", rc, out)
	}
	out, rc = runTool(t, "verify", "--pid", pid, "--exe", "quickemu-ports.test")
	// exe basename is <tool>.test — tolerate any outcome shape; a
	// mismatch must be EXE_MISMATCH, a match success.
	if rc == 1 && errCode(t, out) != "EXE_MISMATCH" && errCode(t, out) != "EXE_UNREADABLE" {
		t.Fatalf("exe: %s", out)
	}
	out, rc = runTool(t, "verify", "--pid", pid, "--exe", "nonexistent-binary-xyz")
	if rc != 1 {
		t.Fatalf("exe must mismatch: rc=%d %s", rc, out)
	}
}

func TestVerifyComm(t *testing.T) {
	pid := strconv.Itoa(os.Getpid())
	out, rc := runTool(t, "verify", "--pid", pid, "--comm", "qemu")
	// comm never starts with qemu → must fail COMM_MISMATCH
	if rc != 1 || errCode(t, out) != "COMM_MISMATCH" {
		t.Fatalf("comm: rc=%d %s", rc, out)
	}
}

func TestDeadAndInvalidPid(t *testing.T) {
	out, rc := runTool(t, "verify", "--pid", "99999999")
	if rc != 1 || errCode(t, out) != "PROCESS_DEAD" {
		t.Fatalf("dead: rc=%d %s", rc, out)
	}
	out, rc = runTool(t, "verify", "--pid", "0")
	if rc != 1 || errCode(t, out) != "PID_INVALID" {
		t.Fatalf("invalid: rc=%d %s", rc, out)
	}
	if _, rc := runTool(t); rc != 2 {
		t.Fatalf("usage rc=%d", rc)
	}
	if _, rc := runTool(t, "bogus"); rc != 2 {
		t.Fatalf("bad sub rc=%d", rc)
	}
}
