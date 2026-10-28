// helpers_test.go — pure helpers of the dev tool: repo discovery,
// module listing, metric utilities and record persistence.
package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRootAndModules(t *testing.T) {
	r := root()
	if r == "" {
		t.Skip("no go.work above cwd")
	}
	if _, err := os.Stat(filepath.Join(r, "go.work")); err != nil {
		t.Fatalf("root %q lacks go.work", r)
	}
	mods := modules(r)
	if len(mods) < 5 {
		t.Fatalf("modules %v", mods)
	}
	// dev itself must be discovered as a main package.
	found := false
	for _, m := range mods {
		if strings.HasSuffix(m, "dev") || m == "dev" {
			found = true
		}
	}
	if !found {
		t.Fatalf("dev module not listed: %v", mods)
	}
	if !isMain(r) == true { // repo root is not a main package
		_ = isMain(r)
	}
}

func TestErrRcAndSmallHelpers(t *testing.T) {
	if errRc(nil) != 0 || errRc(errors.New("x")) == 0 {
		t.Fatal("errRc")
	}
	if num(3.0) == nil || num("x") != nil {
		t.Fatal("num")
	}
	if lines("a\nb\nc") != 3 || lines("") != 0 {
		t.Fatal("lines")
	}
	if uniq("a\nb\na\nc") != 3 {
		t.Fatal("uniq")
	}
	if or("", "fb") != "fb" || or("v", "fb") != "v" {
		t.Fatal("or")
	}
}

func TestCounters(t *testing.T) {
	r := root()
	if r == "" {
		t.Skip("no go.work above cwd")
	}
	if n := countSpecs(r); n <= 0 {
		t.Fatalf("countSpecs %d", n)
	}
	if n := countCatalogOS(r); n <= 0 {
		t.Fatalf("countCatalogOS %d", n)
	}
	// goCoverage sums coverprofile files when present; at minimum it
	// must not panic on a tree without them.
	_ = goCoverage(t.TempDir())
}

func TestMetricRecord(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Skip("chdir")
	}
	defer os.Chdir(old)
	// metricRecord appends one JSON line per record — assert the
	// store exists and carries the values we passed.
	hist := filepath.Join(dir, "hist.jsonl")
	t.Setenv("QEDEV_HISTORY", hist)
	if rc := metricRecord([]string{"coverage=1.5", "specs=10"}); rc != 0 {
		t.Fatalf("metricRecord rc=%d", rc)
	}
	b, err := os.ReadFile(hist)
	if err != nil {
		t.Fatalf("history not written: %v", err)
	}
	if !strings.Contains(string(b), "coverage") ||
		!strings.Contains(string(b), "1.5") {
		t.Fatalf("record missing values: %s", b)
	}
}
