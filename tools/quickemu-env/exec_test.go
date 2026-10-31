package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestErrf(t *testing.T) {
	var rc int
	out := captureOut(t, func() {
		rc = errf("boom %d", 42)
	})
	if rc != 1 || !strings.Contains(out, "ERROR! boom 42") {
		t.Fatalf("errf rc=%d out=%q", rc, out)
	}
}

func TestEmitOK(t *testing.T) {
	out := captureOut(t, func() {
		emitOK("env.test", map[string]any{"k": "v"})
	})
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("envelope: %v\n%s", err, out)
	}
	if doc["schemaVersion"] != float64(1) || doc["command"] != "env.test" ||
		doc["success"] != true {
		t.Fatalf("envelope = %v", doc)
	}
	data := doc["data"].(map[string]any)
	if data["k"] != "v" {
		t.Fatalf("data = %v", data)
	}
	meta := doc["meta"].(map[string]any)
	if meta["tool"] != "quickemu-env/"+version || meta["quickemuVersion"] == "" {
		t.Fatalf("meta = %v", meta)
	}
}

func TestEmitErrs(t *testing.T) {
	out := captureOut(t, func() {
		emitErrs("env.validate", envErrs{{1, 2, "first"}, {3, 4, "second"}})
	})
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("envelope: %v\n%s", err, out)
	}
	if doc["success"] != false {
		t.Fatalf("success = %v", doc["success"])
	}
	errs := doc["errors"].([]any)
	if len(errs) != 2 {
		t.Fatalf("errors = %v", errs)
	}
	first := errs[0].(map[string]any)
	if first["code"] != "ENV_INVALID" || first["message"] != "first" ||
		first["severity"] != "error" {
		t.Fatalf("error = %v", first)
	}
}

func TestRunToolFindError(t *testing.T) {
	stub(t, &findTool, func(string) (string, error) {
		return "", fmt.Errorf("not here")
	})
	_, err := runTool("whatever", "", nil)
	if err == nil || !strings.Contains(err.Error(), "not here") {
		t.Fatalf("runTool find err = %v", err)
	}
}

func TestRunToolRealExec(t *testing.T) {
	script := fakeScript(t, "faketool", `echo "got:$*"`)
	stub(t, &findTool, func(string) (string, error) { return script, nil })
	out, err := runTool("faketool", "", nil, "a", "b c")
	if err != nil || out != "got:a b c\n" {
		t.Fatalf("runTool out = %q err=%v", out, err)
	}
}

func TestRunToolDirAndEnv(t *testing.T) {
	script := fakeScript(t, "faketool", `pwd; echo "env=$QE_ENV_TEST"`)
	stub(t, &findTool, func(string) (string, error) { return script, nil })
	dir := t.TempDir()
	out, err := runTool("faketool", dir, []string{"QE_ENV_TEST=hello"}, "x")
	if err != nil {
		t.Fatalf("runTool: %v", err)
	}
	// pwd reports the requested working dir (resolve symlinks — macOS
	// tempdirs point elsewhere).
	want, _ := os.Readlink(dir)
	if want == "" {
		want = dir
	}
	got := strings.TrimSpace(strings.Split(out, "\n")[0])
	gotRes, _ := os.Readlink(got)
	if gotRes != "" {
		got = gotRes
	}
	if got != want {
		t.Fatalf("dir = %q want %q", got, want)
	}
	if !strings.Contains(out, "env=hello") {
		t.Fatalf("env not passed: %q", out)
	}
}

func TestRunToolChildError(t *testing.T) {
	script := fakeScript(t, "faketool", "echo partial; exit 7")
	stub(t, &findTool, func(string) (string, error) { return script, nil })
	out, err := runTool("faketool", "", nil)
	if err == nil || out != "partial\n" {
		t.Fatalf("runTool out=%q err=%v", out, err)
	}
}
