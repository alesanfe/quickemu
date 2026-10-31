// api_extra_test.go — remaining endpoint branches: per-VM stats via
// the mux, events cursor edge cases, vm-cmd body validation, the
// ops glob's unreadable-entry skip, the listener-error path in main
// and the authThrottle known-host path.
package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------- /v1/vm/<n>/stats ----------

func TestVMStatsEndpoint(t *testing.T) {
	dir := withDataDir(t)

	// Stopped VM — the handler resolves the dir, probes, reports.
	code, m := apiGet(t, "/v1/vm/ghost/stats")
	if code != 200 {
		t.Fatalf("stats %d", code)
	}
	d := m["data"].(map[string]any)
	if d["vm"] != "ghost" || d["state"] != "stopped" || d["alive"] != false {
		t.Fatalf("stopped stats %v", d)
	}

	// Live VM — pid file evidence makes probeVM report running; the
	// (default real) procfs either samples it or degrades, but the
	// request must always succeed with the base shape.
	vmDir := filepath.Join(dir, "vms", "live")
	if err := os.MkdirAll(vmDir, 0o755); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(map[string]any{"pid": os.Getpid()})
	if err := os.WriteFile(filepath.Join(vmDir, "live.runtime.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	code, m = apiGet(t, "/v1/vm/live/stats")
	if code != 200 {
		t.Fatalf("live stats %d", code)
	}
	d = m["data"].(map[string]any)
	if d["state"] != "running" || d["alive"] != true {
		t.Fatalf("live stats %v", d)
	}
	for _, k := range []string{"cpu_pct", "rss_mb", "threads", "disk_rd_b",
		"disk_wr_b", "uptime_s", "qmp", "qga", "net"} {
		if _, ok := d[k]; !ok {
			t.Fatalf("stats missing %q: %v", k, d)
		}
	}
}

// ---------- /v1/vm/<n>/events edge cases ----------

func TestVMEventsEdgeCases(t *testing.T) {
	dir := withDataDir(t)
	vmDir := filepath.Join(dir, "vms", "ev")
	if err := os.MkdirAll(vmDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A corrupt line and a blank line ride along — both skipped.
	jsonl := `{"seq":1,"event":"a"}` + "\n" +
		"\n" +
		"{corrupt-line\n" +
		`{"seq":2,"event":"b"}` + "\n" +
		`{"seq":"x","event":"c"}` + "\n" +
		`{"seq":3,"event":"d"}` + "\n"
	if err := os.WriteFile(filepath.Join(vmDir, "ev.events.jsonl"), []byte(jsonl), 0o644); err != nil {
		t.Fatal(err)
	}

	code, m := apiGet(t, "/v1/vm/ev/events")
	if code != 200 {
		t.Fatalf("events %d", code)
	}
	d := m["data"].(map[string]any)
	// seq"x" fails the struct unmarshal → skipped; corrupt skipped.
	if len(d["events"].([]any)) != 3 || d["latest"].(float64) != 3 {
		t.Fatalf("events %v", d)
	}

	// since with garbage/negative/zero values → ignored, full page.
	for _, q := range []string{"abc", "-2", "0", "2.5"} {
		code, m = apiGet(t, "/v1/vm/ev/events?since="+q)
		if code != 200 {
			t.Fatalf("since=%s %d", q, code)
		}
		d = m["data"].(map[string]any)
		if len(d["events"].([]any)) != 3 {
			t.Fatalf("since=%s events %v", q, d)
		}
	}
}

// ---------- /v1/vm/<n>/ops + /state leftovers ----------

func TestVMSubresourceEdges(t *testing.T) {
	dir := withDataDir(t)
	vmDir := filepath.Join(dir, "vms", "v")
	opsDir := filepath.Join(vmDir, "ops")
	if err := os.MkdirAll(opsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Corrupt journal entry → skipped in the listing.
	writeFile(t, filepath.Join(opsDir, "good.json"), `{"operationId":"op-1"}`)
	writeFile(t, filepath.Join(opsDir, "bad.json"), "{corrupt")
	code, m := apiGet(t, "/v1/vm/v/ops")
	if code != 200 {
		t.Fatalf("ops %d", code)
	}
	if ops := m["data"].(map[string]any)["operations"].([]any); len(ops) != 1 {
		t.Fatalf("ops %v", ops)
	}
	// No ops dir at all → empty list.
	code, m = apiGet(t, "/v1/vm/v2/ops")
	if code != 200 || len(m["data"].(map[string]any)["operations"].([]any)) != 0 {
		t.Fatalf("empty ops %d %v", code, m)
	}

	// state: runtime.json present but pid<=0 → stopped.
	v2 := filepath.Join(dir, "vms", "v2")
	if err := os.MkdirAll(v2, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(v2, "v2.runtime.json"), `{"pid":0}`)
	code, m = apiGet(t, "/v1/vm/v2/state")
	if code != 200 || m["data"].(map[string]any)["state"] != "stopped" {
		t.Fatalf("pid0 state %d %v", code, m)
	}
	// Corrupt runtime.json → pid defaults to 0 → stopped.
	writeFile(t, filepath.Join(v2, "v2.runtime.json"), "{corrupt")
	_, m = apiGet(t, "/v1/vm/v2/state")
	if m["data"].(map[string]any)["state"] != "stopped" {
		t.Fatalf("corrupt state %v", m)
	}
}

// ---------- /v1/vm-cmd body validation ----------

func TestVMCmdValidation(t *testing.T) {
	withDataDir(t)
	oldTok := tokens
	tokens = nil
	t.Cleanup(func() { tokens = oldTok })

	post := func(body string) (int, map[string]any) {
		t.Helper()
		req := httptest.NewRequest("POST", "/v1/vm-cmd/x", strings.NewReader(body))
		rec := httptest.NewRecorder()
		newMux().ServeHTTP(rec, req)
		var m map[string]any
		json.Unmarshal(rec.Body.Bytes(), &m)
		return rec.Code, m
	}

	if code, _ := post(`{"vm":"a/b","cmd":"start"}`); code != 400 {
		t.Fatalf("slash vm %d", code)
	}
	if code, _ := post(`{"vm":"a\\b","cmd":"start"}`); code != 400 {
		t.Fatalf("backslash vm %d", code)
	}
	if code, _ := post(`{"vm":"a..b","cmd":"start"}`); code != 400 {
		t.Fatalf("dotdot vm %d", code)
	}
	if code, _ := post(`{"vm":"v","cmd":""}`); code != 400 {
		t.Fatalf("empty cmd %d", code)
	}
	// Oversized body → MaxBytesReader decode failure → 400.
	big := `{"vm":"` + strings.Repeat("a", 1<<20+10) + `","cmd":"start"}`
	if code, _ := post(big); code != 400 {
		t.Fatalf("oversized body %d", code)
	}
	// Wrong method → 405.
	req := httptest.NewRequest("PUT", "/v1/vm-cmd/x", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	newMux().ServeHTTP(rec, req)
	if rec.Code != 405 {
		t.Fatalf("PUT %d", rec.Code)
	}

	// Authed + valid: the write runs (and reports the tool error).
	tokens = []string{"0123456789abcdef"}
	req = httptest.NewRequest("POST", "/v1/vm-cmd/v",
		strings.NewReader(`{"vm":"v","cmd":"start"}`))
	req.Header.Set("Authorization", "Bearer 0123456789abcdef")
	rec = httptest.NewRecorder()
	newMux().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("authed cmd %d %s", rec.Code, rec.Body.String())
	}
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["data"].(map[string]any)["error"]; !ok {
		t.Fatalf("cmd result %v", m)
	}
}

// ---------- authThrottle: known host at the cap ----------

func TestAuthThrottleKnownHostAtCap(t *testing.T) {
	authMu.Lock()
	authFails = map[string]*failRec{}
	authMu.Unlock()
	t.Cleanup(func() {
		authMu.Lock()
		authFails = map[string]*failRec{}
		authMu.Unlock()
	})
	// Seed the host first, then fill the table to the cap.
	authThrottle("203.0.113.1:9")
	for i := 0; i < authFailMaxHosts; i++ {
		authThrottle(fmt.Sprintf("198.%d.%d.%d:9", i>>16&0xff, i>>8&0xff, i&0xff))
	}
	// The known host still gets its allowance — the cap only denies
	// NEW entries (no eviction of in-progress records).
	if !authThrottle("203.0.113.1:9") {
		t.Fatal("known host must keep its allowance at the cap")
	}
	// A brand-new host is denied while the table stays full.
	if authThrottle("203.0.113.99:9") {
		t.Fatal("new host must be denied at the cap")
	}
	// A request whose RemoteAddr has no port still throttles sanely.
	if !authThrottle("203.0.113.1") {
		t.Fatal("port-less addr must resolve to the host itself")
	}
}

// ---------- main/openListener error paths ----------

func TestMainListenFailure(t *testing.T) {
	// Port out of range → openListener fails → rc 1.
	if rc, _ := runMain(t, "--listen", "127.0.0.1:99999"); rc != 1 {
		t.Fatalf("bad port rc=%d", rc)
	}
	// Unix socket in a missing dir → openListener fails → rc 1.
	if rc, _ := runMain(t, "--unix", "/nonexistent-qe-dir/x.sock"); rc != 1 {
		t.Fatalf("bad unix rc=%d", rc)
	}
	// serveAPI returning false → main returns without serving.
	oldExit, oldServe, oldArgs := exitFn, serveAPI, os.Args
	oldTok := tokens
	tokens = nil
	rc := -1
	exitFn = func(c int) { rc = c }
	serveAPI = func(ln net.Listener, listen string) bool {
		ln.Close()
		return false
	}
	os.Args = []string{"quickemu-api", "--listen", "127.0.0.1:0"}
	t.Cleanup(func() {
		exitFn, serveAPI, os.Args = oldExit, oldServe, oldArgs
		tokens = oldTok
	})
	main()
	if rc != -1 {
		t.Fatalf("serve-false must not exit, rc=%d", rc)
	}
	// A valid inline --token reaches the serve path.
	if rc, served := runMain(t, "--token", "0123456789abcdef",
		"--listen", "127.0.0.1:0"); rc != 0 || !served {
		t.Fatalf("inline token rc=%d served=%v", rc, served)
	}
}

// The real serveAPI body: a dead accept loop must be reported
// (anything but ErrServerClosed → false).
func TestServeAPISeam(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ln.Close() // dead listener → Serve errors immediately
	if serveAPI(ln, "127.0.0.1:0") {
		t.Fatal("closed listener must report serve failure")
	}
}
