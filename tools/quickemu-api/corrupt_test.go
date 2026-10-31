package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// Regression: /v1/vms served data:null when vms.json was corrupt —
// json.Unmarshal's error was ignored so a zero-value nil body was
// serialised instead of degrading like the missing-file path.
func TestVmsEndpointCorruptJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "vms.json"), []byte("{not-json"), 0o644); err != nil {
		t.Fatal(err)
	}
	old, oldTok := dataDir, tokens
	dataDir, tokens = dir, nil
	defer func() { dataDir, tokens = old, oldTok }()

	srv := httptest.NewServer(newMux())
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/v1/vms")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var env map[string]any
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("response not JSON: %v (%s)", err, body)
	}
	d, ok := env["data"].(map[string]any)
	if !ok || d == nil {
		t.Fatalf("data is %v, want empty object for corrupt vms.json", env["data"])
	}
}

// Regression: http.Serve ran with no timeouts — a stalled request on
// a TCP bind (--insecure-allow-exposed) would hold the conn forever.
func TestAPIServerTimeouts(t *testing.T) {
	srv := apiServer()
	if srv.ReadHeaderTimeout <= 0 {
		t.Fatalf("ReadHeaderTimeout unset — slowloris on TCP binds")
	}
	if srv.ReadTimeout <= 0 {
		t.Fatalf("ReadTimeout unset")
	}
	if srv.Handler == nil {
		t.Fatalf("handler unset")
	}
}
