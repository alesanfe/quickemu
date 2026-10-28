// lastmile_test.go — cmdQuality gate, goCoverage aggregation,
// cmdCheck/cmdBench error branches.
package main

import (
	"os"
	"testing"
)

func TestCmdQualityGate(t *testing.T) {
	// Clean gofmt + all module checks pass → rc 0.
	stubExec(t, "", 0)
	if rc := cmdQuality(nil); rc != 0 {
		t.Fatalf("clean quality rc=%d", rc)
	}
	// --full additionally dispatches the qespec suite.
	stubExec(t, "", 0)
	if rc := cmdQuality([]string{"--full"}); rc != 0 {
		t.Fatalf("full quality rc=%d", rc)
	}
	// Unformatted files → gate fails even with checks green.
	stubExec(t, "main.go\n", 0)
	if rc := cmdQuality(nil); rc != 1 {
		t.Fatalf("unformatted quality rc=%d", rc)
	}
}

func TestCmdCheckFailurePaths(t *testing.T) {
	// vet failing → rc 1, loop continues through remaining modules.
	t.Setenv("STUB_FAIL", "vet")
	stubExec(t, "main", 0)
	if rc := cmdCheck(); rc != 1 {
		t.Fatalf("vet-fail check rc=%d", rc)
	}
	t.Setenv("STUB_FAIL", "")
	// test failing → rc 1.
	t.Setenv("STUB_FAIL", "test -race")
	stubExec(t, "main", 0)
	if rc := cmdCheck(); rc != 1 {
		t.Fatalf("test-fail check rc=%d", rc)
	}
	t.Setenv("STUB_FAIL", "")
}

func TestGoCoverageAggregation(t *testing.T) {
	stubExec(t, "ok  \tpkg\t0.1s\tcoverage: 87.5% of statements\n", 0)
	if got := goCoverage(root()); got != "88%" && got != "87%" {
		t.Fatalf("goCoverage=%q", got)
	}
	// No parseable coverage lines → "0%".
	stubExec(t, "FAIL pkg\n", 1)
	if got := goCoverage(root()); got != "0%" {
		t.Fatalf("empty goCoverage=%q", got)
	}
}

func TestCmdBenchMissingHyperfine(t *testing.T) {
	// hyperfine absent on this host → rc 1; if present the stubbed
	// exec path still returns quickly.
	if _, err := os.Stat("/usr/bin/hyperfine"); err == nil {
		stubExec(t, "", 0)
		_ = cmdBench()
		return
	}
	stubExec(t, "", 0)
	if rc := cmdBench(); rc != 1 {
		t.Fatalf("no-hyperfine bench rc=%d", rc)
	}
}
