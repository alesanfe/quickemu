// main_test.go: probes are exercised directly (they degrade gracefully
// on minimal hosts) and run() through the re-exec helper for the two
// report modes — text and --format=json.
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
		os.Args = []string{"quickreport"}
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

func runTool(t *testing.T, dir string, args ...string) (string, int) {
	t.Helper()
	rcf := filepath.Join(t.TempDir(), "rc")
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Dir = dir
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

func TestProbes(t *testing.T) {
	if uname("-s") == "" {
		t.Error("uname must return kernel name on Linux")
	}
	if probeOSName() == "" {
		t.Error("os name empty")
	}
	if probeMemoryGB() == "" {
		t.Error("memory empty")
	}
	if probeCPUInfo() == "" {
		t.Error("cpuinfo empty")
	}
	if probeQemuArch() == "" {
		t.Error("qemu arch empty")
	}
	// probeQuickemu may return "" (not installed); probeGPUs may be empty.
	_ = probeQuickemu()
	_ = probeGPUs()
	_ = cmdOut("nonexistent-binary-xyz", "--version")
}

func TestHelpers(t *testing.T) {
	if cutAfter("a:b", ":") != "b" || cutAfter("ab", ":") != "ab" {
		t.Error("cutAfter")
	}
	if firstLine("x\ny") != "x" || firstLine("x") != "x" {
		t.Error("firstLine")
	}
	// qrRedact must mask IPv4 and home paths.
	r := qrRedact("ip 192.168.1.7 home /home/alesanfe/x")
	if strings.Contains(r, "192.168.1.7") || strings.Contains(r, "alesanfe") {
		t.Fatalf("redaction leaked: %q", r)
	}
}

func TestReportJSONMode(t *testing.T) {
	dir := t.TempDir()
	out, rc := runTool(t, dir, "--format=json")
	if rc != 0 {
		t.Fatalf("rc=%d %s", rc, out)
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &env); err != nil {
		t.Fatalf("json mode must emit the envelope: %v\n%s", err, out)
	}
	if env["success"] != true || env["data"].(map[string]any)["os"] == "" {
		t.Fatalf("%s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "quickreport.json")); err != nil {
		t.Fatal("json report file must be teed")
	}
}

func TestReportTextMode(t *testing.T) {
	dir := t.TempDir()
	out, _ := runTool(t, dir)
	if !strings.Contains(out, "Distro:") && !strings.Contains(out, "Quickemu missing") {
		t.Fatalf("text report malformed:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "quickreport.txt")); err != nil {
		t.Fatal("text report file must be teed")
	}
}
