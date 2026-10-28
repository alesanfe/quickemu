// lastmile_test.go — copyFile, testResult/testAll branches.
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestCopyFileRoundtrip(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	os.WriteFile(src, []byte("payload"), 0o644)
	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile: %v", err)
	}
	if b, _ := os.ReadFile(dst); string(b) != "payload" {
		t.Fatal("copyFile corrupted contents")
	}
	if err := copyFile(filepath.Join(dir, "missing"), dst); err == nil {
		t.Fatal("missing src should error")
	}
	if err := copyFile(src, filepath.Join(dir, "no-dir", "x")); err == nil {
		t.Fatal("uncreatable dst should error")
	}
}

func TestTestResultVariants(t *testing.T) {
	testResult("os", "1.0", "", "http://u", "")
	testResult("os", "1.0", "ed", "http://u", "")
	testResult("os", "1.0", "", "http://u", "PASS")
	testResult("os", "1.0", "ed", "http://u", "FAIL")
}

func TestTestAllBranches(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()
	var cat []OSEntry
	if err := json.Unmarshal([]byte(`[{
		"name":"testos","pretty_name":"TestOS",
		"releases":[
		  {"release":"1.0","iso":[{"web":{"url":"`+srv.URL+`/i.iso"}}]},
		  {"release":"2.0","edition":"pro","img":[{"web":{"url":"`+srv.URL+`/i.img"}}]}
		]}]`), &cat); err != nil {
		t.Fatal(err)
	}
	old := operation
	defer func() { operation = old }()

	// show → prints URL rows without probing.
	operation = "show"
	if rc := testAll(cat, "testos"); rc != 0 {
		t.Fatal("testAll show failed")
	}
	// test → webCheck probes each source URL.
	operation = "test"
	if rc := testAll(cat, "testos"); rc != 0 {
		t.Fatal("testAll test failed")
	}
	// Unknown OS → error + supported list.
	if rc := testAll(cat, "nosuch"); rc != 1 {
		t.Fatal("unknown OS should rc=1")
	}
}
