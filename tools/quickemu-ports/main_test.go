// main_test.go: in-process run() coverage via test-binary re-exec —
// envelope shape, exit codes and registry side effects are all
// asserted, including the reserve/release/find lifecycle.
package main

import (
	"bytes"
	"encoding/json"
	"net"
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
		os.Args = []string{"quickemu-ports"}
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

func runTool(t *testing.T, args ...string) (stdout, stderr string, rc int) {
	t.Helper()
	rcf := filepath.Join(t.TempDir(), "rc")
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "QE_HELPER=1",
		"QE_HELPER_ARGS="+strings.Join(args, "\x1f"),
		"QE_HELPER_RC="+rcf)
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	if err := cmd.Run(); err != nil {
		t.Fatalf("helper: %v", err)
	}
	rc = -1
	if b, err := os.ReadFile(rcf); err == nil {
		rc, _ = strconv.Atoi(strings.TrimSpace(string(b)))
	}
	return so.String(), se.String(), rc
}

func envOK(t *testing.T, out string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &m); err != nil {
		t.Fatalf("bad envelope %q: %v", out, err)
	}
	return m
}

func TestReserveReleaseListLifecycle(t *testing.T) {
	reg := filepath.Join(t.TempDir(), "reg")
	pid := strconv.Itoa(os.Getpid())

	out, _, rc := runTool(t, "reserve", "--registry", reg,
		"--port", "2222", "--pid", pid, "--instance", "vm1")
	if rc != 0 || envOK(t, out)["success"] != true {
		t.Fatalf("reserve rc=%d out=%s", rc, out)
	}

	// Taken port must refuse a second reservation.
	_, _, rc = runTool(t, "reserve", "--registry", reg,
		"--port", "2222", "--pid", pid)
	if rc != 1 {
		t.Fatalf("double reserve rc=%d", rc)
	}

	out, _, rc = runTool(t, "list", "--registry", reg)
	res := envOK(t, out)["data"].(map[string]any)["reservations"].([]any)
	if rc != 0 || len(res) != 1 {
		t.Fatalf("list rc=%d n=%d", rc, len(res))
	}

	out, _, rc = runTool(t, "release", "--registry", reg, "--port", "2222")
	if rc != 0 {
		t.Fatalf("release rc=%d", rc)
	}
	if _, err := os.Stat(filepath.Join(reg, "2222")); !os.IsNotExist(err) {
		t.Fatal("marker must be gone")
	}
}

func TestReserveWithLease(t *testing.T) {
	reg := filepath.Join(t.TempDir(), "reg")
	out, _, rc := runTool(t, "reserve", "--registry", reg, "--port", "5900",
		"--pid", strconv.Itoa(os.Getpid()), "--instance", "vm1", "--lease", "60")
	if rc != 0 {
		t.Fatalf("rc=%d %s", rc, out)
	}
	b, _ := os.ReadFile(filepath.Join(reg, "5900"))
	if len(strings.Fields(string(b))) != 4 {
		t.Fatalf("lease field missing: %q", b)
	}
}

func TestCheckFreeAndBound(t *testing.T) {
	reg := filepath.Join(t.TempDir(), "reg")
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Skip("no tcp")
	}
	defer ln.Close()
	busy := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)

	_, _, rc := runTool(t, "check", "--registry", reg, "--port", busy)
	if rc != 1 {
		t.Fatalf("bound port must fail, rc=%d", rc)
	}
	out, _, rc := runTool(t, "check", "--registry", reg, "--port", "1")
	// port 1 may be unbound-as-unprivileged or taken; accept either but
	// a free one must be an envelope with free=true.
	if rc == 0 && envOK(t, out)["data"].(map[string]any)["free"] != true {
		t.Fatalf("free flag missing: %s", out)
	}
}

func TestFindPicksFirstFreeInRange(t *testing.T) {
	reg := filepath.Join(t.TempDir(), "reg")
	ln, _ := net.Listen("tcp", ":0")
	defer ln.Close()
	p0 := ln.Addr().(*net.TCPAddr).Port
	out, _, rc := runTool(t, "find", "--registry", reg,
		"--lo", strconv.Itoa(p0), "--hi", strconv.Itoa(p0+3))
	if rc != 0 {
		t.Fatalf("find rc=%d", rc)
	}
	got, _ := strconv.Atoi(strings.TrimSpace(out))
	if got <= p0 || got > p0+3 {
		t.Fatalf("find returned %d, want (%d, %d]", got, p0, p0+3)
	}
	// Registry-taken port must be skipped.
	os.WriteFile(filepath.Join(reg, strconv.Itoa(got)),
		[]byte("1 2 i"), 0o600)
	out, _, _ = runTool(t, "find", "--registry", reg,
		"--lo", strconv.Itoa(p0), "--hi", strconv.Itoa(p0+3))
	got2, _ := strconv.Atoi(strings.TrimSpace(out))
	if got2 == got || got2 <= p0 {
		t.Fatalf("find ignored registry marker: %d", got2)
	}
}

func TestFindExhaustedAndBadArgs(t *testing.T) {
	reg := filepath.Join(t.TempDir(), "reg")
	ln, _ := net.Listen("tcp", ":0")
	defer ln.Close()
	p := ln.Addr().(*net.TCPAddr).Port
	_, _, rc := runTool(t, "find", "--registry", reg,
		"--lo", strconv.Itoa(p), "--hi", strconv.Itoa(p))
	if rc != 1 {
		t.Fatalf("exhausted rc=%d", rc)
	}
	if _, _, rc = runTool(t); rc != 2 {
		t.Fatalf("usage rc=%d", rc)
	}
	if _, _, rc = runTool(t, "list"); rc != 1 {
		t.Fatalf("no registry rc=%d", rc)
	}
	if _, _, rc = runTool(t, "reserve", "--registry", reg); rc != 1 {
		t.Fatalf("reserve args rc=%d", rc)
	}
	if _, se, rc := runTool(t, "bogus", "--registry", reg); rc != 2 || se == "" {
		t.Fatalf("unknown rc=%d", rc)
	}
}
