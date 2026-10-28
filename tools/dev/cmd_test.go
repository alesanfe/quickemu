// cmd_test.go — coverage for the qedev subcommands using the classic
// Go helper-process stub: execCommand is replaced by a command that
// re-executes the test binary, whose stdout/exit-code come from env.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestHelperProcess runs only inside the re-executed stub; it prints
// STUB_STDOUT and exits with STUB_RC.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_STUB") != "1" {
		return
	}
	fmt.Print(os.Getenv("STUB_STDOUT"))
	rc := 0
	fmt.Sscanf(os.Getenv("STUB_RC"), "%d", &rc)
	os.Exit(rc)
}

// stubExec swaps execCommand for a helper whose behaviour comes from
// the given stdout/exit-code, optionally per-substring-of-args.
func stubExec(t *testing.T, stdout string, rc int) *[][]string {
	t.Helper()
	var calls [][]string
	old := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		calls = append(calls, append([]string{name}, args...))
		out := stdout
		code := rc
		joined := strings.Join(args, " ")
		// Per-arg overrides: STUB_MAP="substr=>stdout;;substr2=>stdout2"
		for _, m := range strings.Split(os.Getenv("STUB_MAP"), ";;") {
			kv := strings.SplitN(m, "=>", 2)
			if len(kv) == 2 && strings.Contains(joined, kv[0]) {
				out = kv[1]
			}
		}
		// STUB_FAIL=substr makes only matching commands exit 1.
		if f := os.Getenv("STUB_FAIL"); f != "" && strings.Contains(joined, f) {
			code = 1
		}
		cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcess")
		cmd.Env = append(os.Environ(), "GO_STUB=1",
			"STUB_STDOUT="+out, fmt.Sprintf("STUB_RC=%d", code))
		return cmd
	}
	t.Cleanup(func() { execCommand = old })
	return &calls
}

func TestCmdBuildAndTest(t *testing.T) {
	calls := stubExec(t, "main", 0) // go list → "main"
	if rc := cmdBuild(); rc != 0 {
		t.Fatalf("build rc=%d", rc)
	}
	if rc := cmdTest(); rc != 0 {
		t.Fatalf("test rc=%d", rc)
	}
	if rc := cmdSpecs(); rc != 0 {
		t.Fatalf("specs rc=%d", rc)
	}
	if len(*calls) == 0 {
		t.Fatal("no subprocess calls recorded")
	}
	// Failing build propagates rc=1 (list still succeeds via STUB_FAIL).
	t.Setenv("STUB_FAIL", "build")
	stubExec(t, "main", 0)
	if rc := cmdBuild(); rc != 1 {
		t.Fatalf("failing build rc=%d", rc)
	}
	t.Setenv("STUB_FAIL", "")
}

func TestCmdCheck(t *testing.T) {
	stubExec(t, "main", 0)
	if rc := cmdCheck(); rc != 0 {
		t.Fatalf("check rc=%d", rc)
	}
	stubExec(t, "", 1)
	if rc := cmdCheck(); rc != 1 {
		t.Fatalf("failing check rc=%d", rc)
	}
}

func TestCmdProbeAndCatalog(t *testing.T) {
	stubExec(t, "", 0)
	if rc := cmdProbe(); rc != 0 {
		t.Fatalf("probe rc=%d", rc)
	}
	stubExec(t, "", 1)
	if rc := cmdProbe(); rc != 1 {
		t.Fatalf("failing probe rc=%d", rc)
	}
	// check-catalog: valid JSON → 0, bad JSON → 1, empty → 1.
	stubExec(t, `[{"a":1}]`, 0)
	if rc := cmdCheckCatalog(); rc != 0 {
		t.Fatalf("catalog rc=%d", rc)
	}
	stubExec(t, `not json`, 0)
	if rc := cmdCheckCatalog(); rc != 1 {
		t.Fatalf("bad json rc=%d", rc)
	}
	stubExec(t, `[]`, 0)
	if rc := cmdCheckCatalog(); rc != 1 {
		t.Fatalf("empty rc=%d", rc)
	}
	stubExec(t, "", 1)
	if rc := cmdCheckCatalog(); rc != 1 {
		t.Fatalf("exec fail rc=%d", rc)
	}
}

func TestCmdSbomBenchRelease(t *testing.T) {
	stubExec(t, "", 0)
	if rc := cmdSbom(nil); rc != 0 {
		t.Fatalf("sbom rc=%d", rc)
	}
	if rc := cmdSbom([]string{"1.2.3"}); rc != 0 {
		t.Fatalf("sbom ver rc=%d", rc)
	}
	// hyperfine absent → 1.
	if _, err := exec.LookPath("hyperfine"); err != nil {
		if rc := cmdBench(); rc != 1 {
			t.Fatalf("bench rc=%d", rc)
		}
	}
	if rc := cmdReleaseNotes(); rc != 0 {
		t.Fatalf("release-notes rc=%d", rc)
	}
}

func TestCmdMetrics(t *testing.T) {
	if rc := cmdMetrics(nil); rc != 2 {
		t.Fatalf("usage rc=%d", rc)
	}
	if rc := cmdMetrics([]string{"nope"}); rc != 2 {
		t.Fatalf("unknown rc=%d", rc)
	}
	stubExec(t, "abc123", 0)
	if rc := cmdMetrics([]string{"chaoss"}); rc != 0 {
		t.Fatalf("chaoss rc=%d", rc)
	}
	if rc := cmdMetrics([]string{"record"}); rc != 0 {
		t.Fatalf("record rc=%d", rc)
	}
	// trend reads the real repo history.jsonl — rc 1 means a regression
	// was detected in recorded metrics, which is legitimate output.
	if rc := cmdMetrics([]string{"trend"}); rc != 0 && rc != 1 {
		t.Fatalf("trend rc=%d", rc)
	}
}

func TestMetricTrendSeries(t *testing.T) {
	r := root()
	dir := filepath.Join(r, "docs", "quality")
	os.MkdirAll(dir, 0o755)
	f := filepath.Join(dir, "history.jsonl")
	orig, _ := os.ReadFile(f)
	t.Cleanup(func() {
		if orig == nil {
			os.Remove(f)
		} else {
			os.WriteFile(f, orig, 0o644)
		}
	})
	os.WriteFile(f, []byte(
		`{"commit":"a","coverage":"10","specs":1}`+"\n"+
			`{"commit":"b","coverage":"20","specs":3}`+"\n"), 0o644)
	if rc := metricTrend(); rc != 0 {
		t.Fatalf("trend rc=%d (growing metric should not regress)", rc)
	}
}

func TestIsMainAndGoTool(t *testing.T) {
	stubExec(t, "main", 0)
	if !isMain(t.TempDir()) {
		t.Fatal("isMain should be true for main pkg")
	}
	stubExec(t, "other", 0)
	if isMain(t.TempDir()) {
		t.Fatal("isMain should be false")
	}
	// goTool: real PATH first.
	if p := goTool(); p == "" {
		t.Fatal("goTool empty")
	}
}

// TestMain implements the helper-exec seam: QE_DEV_HELPER=1 makes the
// re-executed binary rewrite argv from DEV_HELPER_ARGS and run main().
// -test.run=^$ keeps the helper from re-running the suite.
func TestMain(m *testing.M) {
	if os.Getenv("QE_DEV_HELPER") == "1" {
		os.Args = append([]string{os.Args[0]},
			strings.Split(os.Getenv("DEV_HELPER_ARGS"), "\x1f")...)
		main()
		return
	}
	os.Exit(m.Run())
}

func TestMainDispatch(t *testing.T) {
	stub := stubExec(t, "main", 0) // in-helper go/git/quickget stubs
	_ = stub
	run := func(args ...string) int {
		cmd := exec.Command(os.Args[0], "-test.run=^$")
		env := append(os.Environ(), "QE_DEV_HELPER=1",
			"DEV_HELPER_ARGS="+strings.Join(args, "\x1f"))
		// The helper must also see the stub — re-point execCommand there
		// is impossible across processes, so pass through STUB envs and
		// let TestMain install a real-stub first. Instead the helper runs
		// the real commands; that's fine for usage/unknown (rc=2) which
		// never reach subprocesses.
		cmd.Env = env
		err := cmd.Run()
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		return 0
	}
	if rc := run(); rc != 2 {
		t.Fatalf("no-args rc=%d", rc)
	}
	if rc := run("boguscmd"); rc != 2 {
		t.Fatalf("unknown rc=%d", rc)
	}
	if rc := run("metrics"); rc != 2 {
		t.Fatalf("metrics usage rc=%d", rc)
	}
	if rc := run("metrics", "bogus"); rc != 2 {
		t.Fatalf("metrics bogus rc=%d", rc)
	}
}
