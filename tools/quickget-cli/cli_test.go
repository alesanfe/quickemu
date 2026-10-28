// cli_test.go — in-package coverage of quickget-cli's pure logic:
// CSV row generation, trust grading, combinations, wrap, validation.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixtureCatalog(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	cat := `{"entries":[
	  {"os":"alpine","releases":["3.19","3.20"],"editions":["virt"],"arch":["amd64"]},
	  {"os":"windows","releases":["11"],"editions":[],"arch":["amd64"]},
	  {"os":"plain","releases":["1"],"editions":[],"arch":[]}]}`
	os.WriteFile(filepath.Join(dir, "catalog.json"), []byte(cat), 0o600)
	os.WriteFile(filepath.Join(dir, "info.json"),
		[]byte(`{"entries":{"alpine":{"Name":"Alpine"},"plain":{"Name":"Plain"},"windows":{"Name":"Windows"}}}`), 0o600)
	t.Setenv("QUICKEMU_CATALOG_DIR", dir)
}

func TestLoadAndFind(t *testing.T) {
	fixtureCatalog(t)
	cat := loadCatalog()
	if len(cat) != 3 {
		t.Fatalf("cat %d", len(cat))
	}
	if findEntry(cat, "alpine") == nil || findEntry(cat, "none") != nil {
		t.Fatal("findEntry")
	}
	info := loadInfo()
	if info["alpine"].Name != "Alpine" {
		t.Fatalf("info %v", info["alpine"])
	}
}

func TestCsvRows(t *testing.T) {
	fixtureCatalog(t)
	rows := csvRows(loadCatalog(), loadInfo())
	if len(rows) == 0 {
		t.Fatal("empty rows")
	}
	// windows must expand into language editions.
	win := 0
	for _, r := range rows {
		if r[1] == "windows" {
			win++
			if r[3] == "" {
				t.Fatal("windows row without language edition")
			}
		}
		if len(r) != 7 {
			t.Fatalf("row width %d", len(r))
		}
	}
	if win == 0 {
		t.Fatal("no windows rows")
	}
	// Sorted by OS column.
	for i := 1; i < len(rows); i++ {
		if rows[i-1][1] > rows[i][1] {
			t.Fatal("rows not sorted")
		}
	}
}

func TestTrust(t *testing.T) {
	for _, c := range []struct {
		lvl, min string
		ok       bool
	}{
		{"A", "B", true}, {"B", "B", true}, {"C", "B", false},
		{"A", "E", true}, {"E", "D", false}, {"D", "D", true},
	} {
		if trustOK(c.lvl, c.min) != c.ok {
			t.Errorf("trustOK(%s,%s)", c.lvl, c.min)
		}
	}
	if trustLevel("alpine") == "" {
		t.Fatal("trustLevel empty")
	}
}

func TestCombinationsAndWrap(t *testing.T) {
	e := &Entry{OS: "x", Releases: []string{"1", "2"}, Editions: []string{"a", "b"}}
	if got := combinations(e, "", ""); len(got) != 4 {
		t.Fatalf("combos %v", got)
	}
	if got := combinations(e, "2", "b"); len(got) != 1 || got[0] != [2]string{"2", "b"} {
		t.Fatalf("combo %v", got)
	}
	// No editions → windows langs / single empty edition.
	w := &Entry{OS: "windows", Releases: []string{"11"}}
	if got := combinations(w, "", ""); len(got) < 5 {
		t.Fatalf("win combos %v", got)
	}
	p := &Entry{OS: "plain", Releases: []string{"1"}}
	if got := combinations(p, "", ""); len(got) != 1 || got[0][1] != "" {
		t.Fatalf("plain combos %v", got)
	}
	if out := wrapLines([]string{"aaa", "bbb", "ccc"}, 7); !strings.Contains(out, "\n") {
		t.Fatalf("wrap %q", out)
	}
}

func TestValidateAndGlob(t *testing.T) {
	fixtureCatalog(t)
	if !validateOS("alpine") || validateOS("not-an-os") {
		t.Fatal("validateOS")
	}
	// Stub quickget-catalog on PATH so validateRelease sees "3.20" only.
	bin := t.TempDir()
	stub := filepath.Join(bin, "quickget-catalog")
	os.WriteFile(stub, []byte("#!/bin/sh\necho 3.20\n"), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if !validateRelease("alpine", "3.20") || validateRelease("alpine", "9.9") {
		t.Fatal("validateRelease")
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.iso"), []byte("x"), 0o600)
	os.WriteFile(filepath.Join(dir, "b.conf"), []byte("x"), 0o600)
	if got := globDir(dir); len(got) == 0 {
		t.Fatal("globDir")
	}
}
