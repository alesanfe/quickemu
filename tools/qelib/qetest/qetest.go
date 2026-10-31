// Package qetest hosts the QE_HELPER self-re-exec protocol shared by
// the tool test suites: TestMain rewrites os.Args from QE_HELPER_ARGS
// (\x1f-separated), runs the tool's run(), and reports the rc through
// the QE_HELPER_RC file. Keeping the protocol in one place means a
// change (env names, arg encoding, spawn semantics) lands in every
// suite at once instead of nine coordinated copies.
package qetest

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// HelperMain dispatches the QE_HELPER re-exec branch for TestMain:
//
//	func TestMain(m *testing.M) {
//		if qetest.HelperMain("mytool", run) {
//			return
//		}
//		os.Exit(m.Run())
//	}
func HelperMain(tool string, run func() int) bool {
	if os.Getenv("QE_HELPER") != "1" {
		return false
	}
	a := os.Getenv("QE_HELPER_ARGS")
	os.Args = []string{tool}
	if a != "" {
		os.Args = append(os.Args, strings.Split(a, "\x1f")...)
	}
	_ = os.WriteFile(os.Getenv("QE_HELPER_RC"),
		[]byte(strconv.Itoa(run())), 0o600)
	return true
}

// RunToolFull re-execs the test binary as the QE_HELPER and returns
// stdout, stderr and the helper's rc (from the rc file; -1 when the
// helper never wrote one — e.g. a die()/os.Exit mid-flow, which some
// suites assert on). env vars are appended AFTER the protocol vars so
// callers may override them; dir sets the helper cwd when non-empty.
func RunToolFull(t *testing.T, env []string, dir string, args ...string) (stdout, stderr string, rc int) {
	t.Helper()
	rcf := filepath.Join(t.TempDir(), "rc")
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(), "QE_HELPER=1",
		"QE_HELPER_ARGS="+strings.Join(args, "\x1f"),
		"QE_HELPER_RC="+rcf)
	cmd.Env = append(cmd.Env, env...)
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	err := cmd.Run()
	rc = -1
	if b, rerr := os.ReadFile(rcf); rerr == nil {
		rc, _ = strconv.Atoi(strings.TrimSpace(string(b)))
	}
	if err != nil {
		// Non-fatal: die() paths exit the helper before the rc write.
		t.Logf("helper: %v (rc file %s)", err, rcf)
	}
	return so.String(), se.String(), rc
}

// RunTool is RunToolFull for the common (stdout, rc) shape.
func RunTool(t *testing.T, args ...string) (string, int) {
	out, _, rc := RunToolFull(t, nil, "", args...)
	return out, rc
}

// RunExit re-execs the test binary for helpers whose rc is the
// process exit code (main()-style helpers — no rc file). envVar is
// the suite's arg-passing env name (QC_ARGS/QL_ARGS/…); env vars are
// appended BEFORE the protocol vars. Returns combined stdout+stderr
// and the exit code.
func RunExit(t *testing.T, envVar string, env []string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(append(os.Environ(), env...), "QE_HELPER=1",
		envVar+"="+strings.Join(args, "\x1f"))
	var out, errb strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	rc := 0
	if ee, ok := err.(*exec.ExitError); ok {
		rc = ee.ExitCode()
	}
	return out.String() + errb.String(), rc
}

// CaptureStdout redirects os.Stdout while fn runs and returns what
// was written. A goroutine drains the pipe so output larger than the
// OS pipe buffer cannot deadlock the writer.
func CaptureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	w.Close()
	os.Stdout = old
	return <-done
}

// CaptureStderr is CaptureStdout for os.Stderr — warnings and trust
// messages live on stderr by convention.
func CaptureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	w.Close()
	os.Stderr = old
	return <-done
}
