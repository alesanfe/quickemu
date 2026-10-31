// log_test.go — QE-104: structured action logging (JSONL shape,
// rotation, env override).
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestQlogWritesJSONL(t *testing.T) {
	vd := withConf(t, "")
	p := filepath.Join(vd, "x.log.jsonl")
	t.Setenv("QE_LOG_JSONL", p)
	qlog("info", "action.start", `{"action":"status_vm"}`)
	qlog("error", "action.fail", `{"action":"kill_vm","rc":1}`)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 2 {
		t.Fatalf("lines %d: %s", len(lines), b)
	}
	var r0, r1 map[string]any
	json.Unmarshal([]byte(lines[0]), &r0)
	json.Unmarshal([]byte(lines[1]), &r1)
	if r0["event"] != "action.start" || r0["level"] != "info" ||
		r0["vm"] != "vm1" || r0["ts"] == "" {
		t.Fatalf("rec0 %v", r0)
	}
	if r1["event"] != "action.fail" || r1["level"] != "error" {
		t.Fatalf("rec1 %v", r1)
	}
	data, _ := r1["data"].(map[string]any)
	if data["rc"] != 1.0 {
		t.Fatalf("rc data %v", data)
	}
}

func TestQlogRotation(t *testing.T) {
	vd := withConf(t, "")
	p := filepath.Join(vd, "big.log.jsonl")
	t.Setenv("QE_LOG_JSONL", p)
	// Pre-grow the file past the cap → next write rotates to .1.
	os.WriteFile(p, make([]byte, logMaxBytes+1), 0o644)
	qlog("info", "test", "")
	if _, err := os.Stat(p + ".1"); err != nil {
		t.Fatal("rotation did not happen")
	}
	st, _ := os.Stat(p)
	if st.Size() > 1024 {
		t.Fatalf("rotated log still huge: %d", st.Size())
	}
}

func TestQlogNoPath(t *testing.T) {
	old := vmConf
	vmConf = ""
	defer func() { vmConf = old }()
	// No VM + no env → no-op, no panic.
	qlog("info", "noop", "")
}

func TestQlogDefaultPath(t *testing.T) {
	vd := withConf(t, "")
	t.Setenv("QE_LOG_JSONL", "")
	qlog("info", "default.path", "")
	if _, err := os.Stat(filepath.Join(vd, "vm1.log.jsonl")); err != nil {
		t.Fatalf("default log not written: %v", err)
	}
}
