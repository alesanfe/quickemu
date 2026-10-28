// api_test.go — auth middleware, JSON writer and vmCmd error paths.
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	old := token
	token = "sekrit"
	defer func() { token = old }()

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
	token = ""
	rec = httptest.NewRecorder()
	ok.ServeHTTP(rec, httptest.NewRequest("GET", "/v1/vms", nil))
	if rec.Code != 204 {
		t.Fatalf("open %d", rec.Code)
	}
}

func TestVmCmdErrors(t *testing.T) {
	// Nonexistent socket/tool must surface an error, not panic.
	old := dataDir
	dataDir = t.TempDir()
	defer func() { dataDir = old }()
	if _, err := vmCmd("novm", "status"); err == nil {
		t.Fatal("expected vmCmd error for missing vm")
	}
}
