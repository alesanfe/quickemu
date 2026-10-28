package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// Regression: a stale runtime.json (hard crash leftovers) used to
// report "running" — the stored pid must be verified alive.
func TestVmStateStaleRuntimeReportsStopped(t *testing.T) {
	old := dataDir
	dataDir = t.TempDir()
	defer func() { dataDir = old }()

	vmDir := filepath.Join(dataDir, "vms", "crashed")
	os.MkdirAll(vmDir, 0o755)
	// pid 2^30 is virtually guaranteed dead.
	os.WriteFile(filepath.Join(vmDir, "crashed.runtime.json"),
		[]byte(`{"pid":1073741824,"monitor":""}`), 0o600)

	rec := httptest.NewRecorder()
	newMux().ServeHTTP(rec, httptest.NewRequest("GET", "/v1/vm/crashed/state", nil))
	var body map[string]any
	json.Unmarshal(rec.Body.Bytes(), &body)
	state := body["data"].(map[string]any)["state"]
	if state != "stopped" {
		t.Fatalf("dead pid must report stopped, got %v", state)
	}
}

func TestVmStateLivePidReportsRunning(t *testing.T) {
	old := dataDir
	dataDir = t.TempDir()
	defer func() { dataDir = old }()

	vmDir := filepath.Join(dataDir, "vms", "alive")
	os.MkdirAll(vmDir, 0o755)
	b, _ := json.Marshal(map[string]any{"pid": os.Getpid(), "monitor": ""})
	os.WriteFile(filepath.Join(vmDir, "alive.runtime.json"), b, 0o600)

	rec := httptest.NewRecorder()
	newMux().ServeHTTP(rec, httptest.NewRequest("GET", "/v1/vm/alive/state", nil))
	var body map[string]any
	json.Unmarshal(rec.Body.Bytes(), &body)
	state := body["data"].(map[string]any)["state"]
	if state != "running" {
		t.Fatalf("live pid must report running, got %v", state)
	}
}

// Regression: vm names are joined into paths — ".." and separators
// must be rejected before touching the filesystem.
func TestVmNameTraversalRejected(t *testing.T) {
	old := dataDir
	dataDir = t.TempDir()
	defer func() { dataDir = old }()

	for _, name := range []string{"..%2f..%2fetc", "..%5cwin", "a%2fb"} {
		rec := httptest.NewRecorder()
		newMux().ServeHTTP(rec,
			httptest.NewRequest("GET", "/v1/vm/"+name+"/state", nil))
		if rec.Code != 400 {
			t.Fatalf("%s must 400, got %d %s", name, rec.Code, rec.Body.String())
		}
	}
}
