// main_test.go — exercises the real main() flag parsing via the
// QE_HELPER re-exec pattern (guarded against fork-bomb by the
// dedicated flag + -test.run=^$).
package main

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("QE_HELPER") == "1" {
		os.Args = []string{"quickemu-qmp"}
		if a := os.Getenv("QE_HELPER_ARGS"); a != "" {
			os.Args = append(os.Args, strings.Split(a, "\x1f")...)
		}
		main() // exits the helper itself
		return
	}
	os.Exit(m.Run())
}

func runTool(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "QE_HELPER=1",
		"QE_HELPER_ARGS="+strings.Join(args, "\x1f"))
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	_ = cmd.Run()
	return so.String() + se.String()
}

func TestMainPaths(t *testing.T) {
	// usage → exit 2
	out := runTool(t)
	if !strings.Contains(out, "usage:") {
		t.Fatalf("usage %q", out)
	}
	// unknown subcommand → 2
	if out := runTool(t, "frobnicate"); !strings.Contains(out, "unknown subcommand") {
		t.Fatalf("unknown %q", out)
	}
	// execute without required flags → 2
	if out := runTool(t, "execute"); !strings.Contains(out, "needs --socket and --command") {
		t.Fatalf("flags %q", out)
	}
	// execute over the fake QMP socket → envelope on stdout.
	sock := fakeQMP(t, map[string]map[string]any{
		"query-status": {"return": map[string]any{"status": "running"}},
	})
	out = runTool(t, "execute", "--socket", sock,
		"--command", `{"execute":"query-status"}`, "--timeout", "3s")
	if !strings.Contains(out, `"success":true`) && !strings.Contains(out, "running") {
		t.Fatalf("execute %q", out)
	}
	// execute --raw prints the bare {return:...} line.
	out = runTool(t, "execute", "--socket", sock,
		"--command", `{"execute":"query-status"}`, "--raw")
	if !strings.Contains(out, "running") {
		t.Fatalf("raw %q", out)
	}
	// watch with missing socket → envelope error (exits 1 via emit).
	out = runTool(t, "watch", "--socket", "/tmp/definitely-missing-qmp.sock")
	if !strings.Contains(out, "QMP_UNAVAILABLE") {
		t.Fatalf("watch %q", out)
	}
	// watch without --socket → usage 2.
	if out := runTool(t, "watch"); !strings.Contains(out, "watch needs --socket") {
		t.Fatalf("watch flags %q", out)
	}
}
