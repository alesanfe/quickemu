// endpoints_test.go: the QE-080–083 read-only surface — stats,
// capabilities, cross-VM ops and per-VM events.
package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// apiGet: one GET against the mux in a temp dataDir.
func apiGet(t *testing.T, path string) (int, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	newMux().ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("%s: bad JSON %q", path, rec.Body.String())
	}
	return rec.Code, m
}

func TestHealthAndCapabilities(t *testing.T) {
	dir := t.TempDir()
	old := dataDir
	dataDir = dir
	t.Cleanup(func() { dataDir = old })

	code, m := apiGet(t, "/v1/health")
	if code != 200 || m["data"].(map[string]any)["ok"] != true {
		t.Fatalf("health: %d %v", code, m)
	}
	code, m = apiGet(t, "/v1/capabilities")
	if code != 200 {
		t.Fatalf("capabilities: %d", code)
	}
	d := m["data"].(map[string]any)
	if d["api"] != "v1" {
		t.Fatalf("api field: %v", d["api"])
	}
	if d["backend"] == "" {
		t.Fatal("backend must carry the version string")
	}
	if len(d["commands"].([]any)) == 0 || len(d["endpoints"].([]any)) == 0 {
		t.Fatal("capabilities must enumerate commands + endpoints")
	}
}

func TestHostStatsEndpoint(t *testing.T) {
	dir := t.TempDir()
	old := dataDir
	dataDir = dir
	t.Cleanup(func() { dataDir = old })

	code, m := apiGet(t, "/v1/stats")
	if code != 200 {
		t.Fatalf("stats: %d", code)
	}
	d := m["data"].(map[string]any)
	// Host shape — mem totals + load + data-dir free space gate.
	for _, k := range []string{"mem_total_mb", "mem_avail_mb", "load1", "data_dir_free_ok"} {
		if _, ok := d[k]; !ok {
			t.Fatalf("host stats missing %q: %v", k, d)
		}
	}
}

func TestVMEventsEndpoint(t *testing.T) {
	dir := t.TempDir()
	old := dataDir
	dataDir = dir
	t.Cleanup(func() { dataDir = old })

	vmDir := filepath.Join(dir, "vms", "ev")
	if err := os.MkdirAll(vmDir, 0755); err != nil {
		t.Fatal(err)
	}
	jsonl := `{"seq":1,"ts":"t","event":"vm.started","vm":"ev","pid":1,"data":{}}` + "\n" +
		`{"seq":2,"ts":"t","event":"vm.paused","vm":"ev","pid":1,"data":{}}` + "\n" +
		`{"seq":3,"ts":"t","event":"vm.resumed","vm":"ev","pid":1,"data":{}}` + "\n"
	if err := os.WriteFile(filepath.Join(vmDir, "ev.events.jsonl"), []byte(jsonl), 0644); err != nil {
		t.Fatal(err)
	}

	code, m := apiGet(t, "/v1/vm/ev/events")
	if code != 200 {
		t.Fatalf("events: %d", code)
	}
	d := m["data"].(map[string]any)
	if len(d["events"].([]any)) != 3 || d["latest"].(float64) != 3 {
		t.Fatalf("events: %v", d)
	}

	// since=N resumes — only newer seqs come back.
	_, m = apiGet(t, "/v1/vm/ev/events?since=1")
	d = m["data"].(map[string]any)
	if len(d["events"].([]any)) != 2 || d["latest"].(float64) != 3 {
		t.Fatalf("resume: %v", d)
	}
	_, m = apiGet(t, "/v1/vm/ev/events?since=3")
	d = m["data"].(map[string]any)
	if len(d["events"].([]any)) != 0 || d["latest"].(float64) != 3 {
		t.Fatalf("at-end: %v", d)
	}

	// Missing log = empty page, not an error.
	code, m = apiGet(t, "/v1/vm/noone/events")
	if code != 200 || len(m["data"].(map[string]any)["events"].([]any)) != 0 {
		t.Fatalf("missing log: %d %v", code, m)
	}
}

func TestOpsEndpoint(t *testing.T) {
	dir := t.TempDir()
	old := dataDir
	dataDir = dir
	t.Cleanup(func() { dataDir = old })

	// One op record under a VM's ops dir — the journal layout
	// scanOps walks (<dataDir>/vms/<vm>/ops/*.json).
	opDir := filepath.Join(dir, "vms", "o1", "ops")
	if err := os.MkdirAll(opDir, 0755); err != nil {
		t.Fatal(err)
	}
	rec := `{"operationId":"op-1","type":"start","status":"succeeded","vm":"o1","startedAt":"2024-01-01T00:00:00Z"}`
	if err := os.WriteFile(filepath.Join(opDir, "op-1.json"), []byte(rec), 0644); err != nil {
		t.Fatal(err)
	}

	code, m := apiGet(t, "/v1/ops")
	if code != 200 {
		t.Fatalf("ops: %d", code)
	}
	ops := m["data"].(map[string]any)["operations"].([]any)
	if len(ops) != 1 || ops[0].(map[string]any)["operationId"] != "op-1" {
		t.Fatalf("ops: %v", ops)
	}

	code, m = apiGet(t, "/v1/ops/op-1")
	if code != 200 || m["data"].(map[string]any)["operationId"] != "op-1" {
		t.Fatalf("op by id: %d %v", code, m)
	}
	if code, _ := apiGet(t, "/v1/ops/nonexistent"); code != 404 {
		t.Fatalf("missing op must 404, got %d", code)
	}
	if code, _ := apiGet(t, "/v1/ops/..%2fetc"); code != 400 {
		t.Fatalf("traversal id must 400, got %d", code)
	}
}
