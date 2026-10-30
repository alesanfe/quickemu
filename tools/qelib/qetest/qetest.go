// Package qetest hosts the QE_HELPER self-re-exec protocol shared by
// the tool test suites: TestMain rewrites os.Args from QE_HELPER_ARGS
// (\x1f-separated), runs the tool's run(), and reports the rc through
// the QE_HELPER_RC file. Keeping the protocol in one place means a
// change (env names, arg encoding, spawn semantics) lands in every
// suite at once instead of nine coordinated copies.
package qetest

import (
	"bytes"
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
