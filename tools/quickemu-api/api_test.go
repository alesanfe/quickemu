// api_test.go — auth middleware, JSON writer and vmCmd error paths.
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	write(rec, 201, map[string]any{"ok": true})
	if rec.Code != 201 {
		t.Fatalf("code %d", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["ok"] != true {
		t.Fatalf("body %v %s", err, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct == "" {
		t.Fatal("no content-type")
	}
}

func TestAuth(t *testing.T) {
	old := tokens
	tokens = []string{"sekrit"}
	defer func() { tokens = old }()

	ok := auth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(204)
	})
	// No token → 401.
	rec := httptest.NewRecorder()
	ok.ServeHTTP(rec, httptest.NewRequest("GET", "/v1/vms", nil))
	if rec.Code != 401 {
		t.Fatalf("unauth %d", rec.Code)
	}
	// Bearer token → next handler runs.
	req := httptest.NewRequest("GET", "/v1/vms", nil)
	req.Header.Set("Authorization", "Bearer sekrit")
	rec = httptest.NewRecorder()
	ok.ServeHTTP(rec, req)
	if rec.Code != 204 {
		t.Fatalf("auth %d", rec.Code)
	}
	// Wrong token → 401.
	req.Header.Set("Authorization", "Bearer nope")
	rec = httptest.NewRecorder()
	ok.ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Fatalf("bad token %d", rec.Code)
	}
	// Empty token config → open (no auth) path.
	tokens = nil
	rec = httptest.NewRecorder()
	ok.ServeHTTP(rec, httptest.NewRequest("GET", "/v1/vms", nil))
	if rec.Code != 204 {
		t.Fatalf("open %d", rec.Code)
	}
}

func TestEndpoints(t *testing.T) {
	dir := t.TempDir()
	old := dataDir
	dataDir = dir
	defer func() { dataDir = old }()
	tokens = nil // read-only mode
	srv := httptest.NewServer(newMux())
	defer srv.Close()

	get := func(path string) (int, map[string]any) {
		t.Helper()
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var m map[string]any
		json.NewDecoder(resp.Body).Decode(&m)
		return resp.StatusCode, m
	}

	if code, m := get("/v1/health"); code != 200 || m["data"].(map[string]any)["ok"] != true {
		t.Fatalf("health %d %v", code, m)
	}
	// No vms.json → empty data.
	if code, m := get("/v1/vms"); code != 200 || m["data"].(map[string]any) == nil {
		t.Fatalf("vms %d %v", code, m)
	}
	// With registry file.
	os.WriteFile(filepath.Join(dir, "vms.json"), []byte(`{"a":{"state":"stopped"}}`), 0o600)
	if code, m := get("/v1/vms"); code != 200 || m["data"].(map[string]any)["a"] == nil {
		t.Fatalf("vms2 %d %v", code, m)
	}
	// State: missing runtime.json → stopped.
	if code, m := get("/v1/vm/a/state"); code != 200 ||
		m["data"].(map[string]any)["state"] != "stopped" {
		t.Fatalf("state %d %v", code, m)
	}
	// Dead-pid runtime.json → still stopped.
	os.MkdirAll(filepath.Join(dir, "vms", "a"), 0o755)
	os.WriteFile(filepath.Join(dir, "vms", "a", "a.runtime.json"),
		[]byte(`{"pid":99999999}`), 0o600)
	if _, m := get("/v1/vm/a/state"); m["data"].(map[string]any)["state"] != "stopped" {
		t.Fatalf("dead state %v", m)
	}
	// Live pid → running.
	os.WriteFile(filepath.Join(dir, "vms", "a", "a.runtime.json"),
		[]byte(`{"pid":1}`), 0o600)
	if _, m := get("/v1/vm/a/state"); m["data"].(map[string]any)["state"] != "running" {
		t.Fatalf("live state %v", m)
	}
	// Ops listing.
	os.MkdirAll(filepath.Join(dir, "vms", "a", "ops"), 0o755)
	os.WriteFile(filepath.Join(dir, "vms", "a", "ops", "o1.json"), []byte(`{"op":"start"}`), 0o600)
	if _, m := get("/v1/vm/a/ops"); len(m["data"].(map[string]any)["operations"].([]any)) != 1 {
		t.Fatalf("ops %v", m)
	}
	// Traversal + glob metachars + unknown sub. A "*" vm name isn't a
	// path traversal but it IS a glob pattern — /v1/vm/*/ops would list
	// every VM's operations.
	if code, _ := get("/v1/vm/..%2fetc/state"); code != 400 {
		t.Fatalf("traversal %d", code)
	}
	if code, m := get("/v1/vm/*/ops"); code != 400 {
		ops := m["data"].(map[string]any)["operations"].([]any)
		if len(ops) > 0 {
			t.Fatalf("glob name leaked %d foreign ops", len(ops))
		}
		t.Fatalf("glob name should be rejected, got %d", code)
	}
	if code, _ := get("/v1/vm/a/bogus"); code != 404 {
		t.Fatalf("unknown sub %d", code)
	}
	// vm-cmd: GET → 405; bad body → 400; disallowed cmd → 400;
	// no token configured → 403.
	resp, _ := http.Get(srv.URL + "/v1/vm-cmd/a")
	if resp.StatusCode != 405 {
		t.Fatalf("method %d", resp.StatusCode)
	}
	resp.Body.Close()
	post := func(body string) int {
		t.Helper()
		r, err := http.Post(srv.URL+"/v1/vm-cmd/a", "application/json",
			strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer r.Body.Close()
		return r.StatusCode
	}
	if c := post(`not json`); c != 400 {
		t.Fatalf("bad body %d", c)
	}
	if c := post(`{"vm":"a","cmd":"rm -rf /"}`); c != 400 {
		t.Fatalf("bad cmd %d", c)
	}
	if c := post(`{"vm":"a","cmd":"start"}`); c != 403 {
		t.Fatalf("no-token write %d", c)
	}
	// With token → vmCmd fails (no quickemu), but the write is allowed.
	tokens = []string{"t"}
	defer func() { tokens = nil }()
	r, _ := http.NewRequest("POST", srv.URL+"/v1/vm-cmd/a",
		strings.NewReader(`{"vm":"a","cmd":"start"}`))
	r.Header.Set("Authorization", "Bearer t")
	r.Header.Set("Content-Type", "application/json")
	resp2, err := http.DefaultClient.Do(r)
	if err != nil || resp2.StatusCode != 200 {
		t.Fatalf("authed write %v", err)
	}
	resp2.Body.Close()
}

func TestOpenListener(t *testing.T) {
	// TCP on an ephemeral port.
	ln, err := openListener("127.0.0.1:0")
	if err != nil {
		t.Fatalf("tcp %v", err)
	}
	ln.Close()
	// TCP bind failure (bad address).
	if _, err := openListener("256.256.256.256:1"); err == nil {
		t.Fatal("bad addr must fail")
	}
	// Unix socket bind + chmod.
	sock := filepath.Join(t.TempDir(), "api.sock")
	ln, err = openListener(sock)
	if err != nil {
		t.Fatalf("unix %v", err)
	}
	// Second bind on the live socket must be refused.
	if _, err := openListener(sock); err == nil {
		t.Fatal("live socket steal must fail")
	}
	ln.Close()
	// Stale socket file (listener gone) → unlinked and rebound.
	if ln, err = openListener(sock); err != nil {
		t.Fatalf("rebind stale %v", err)
	}
	ln.Close()
}

func TestVmCmdErrors(t *testing.T) {
	// Nonexistent socket/tool must surface an error, not panic.
	old := dataDir
	dataDir = t.TempDir()
	defer func() { dataDir = old }()
	if _, err := vmCmd("novm", "status"); err == nil {
		t.Fatal("expected vmCmd error for missing vm")
	}
	// With stub tools on PATH the command dispatch resolves and runs.
	bin := t.TempDir()
	for _, n := range []string{"quickemu-qmp", "quickemu-launch", "quickemu-cli"} {
		os.WriteFile(filepath.Join(bin, n),
			[]byte("#!/bin/sh\necho ran-$0\n"), 0o755)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, c := range []string{"start", "shutdown", "pause", "resume",
		"stop", "kill", "reset"} {
		out, err := vmCmd("a", c)
		// The stub runs → output captured; unknown branches return
		// an explicit error rather than nil.
		if out == "" && err == nil {
			t.Fatalf("vmCmd(%q) empty", c)
		}
	}
}
