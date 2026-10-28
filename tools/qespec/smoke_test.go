package qespec

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// ---- spec/smoke/smoke_spec.sh ----

func TestSmokeEntries(t *testing.T) {
	for _, e := range []string{"quickemu", "quickget", "quickgetc", "quickreport"} {
		bin := Bin(t, e)
		b, err := os.ReadFile(bin)
		if err != nil {
			t.Fatal(err)
		}
		if len(b) < 4 || string(b[:4]) != "\x7fELF" {
			t.Errorf("%s is not a compiled binary", e)
		}
	}
}

func TestSmokeToolSources(t *testing.T) {
	ents, err := os.ReadDir(filepath.Join(Root(), "tools"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		d := filepath.Join(Root(), "tools", e.Name())
		found := false
		fs, _ := os.ReadDir(d)
		for _, f := range fs {
			if filepath.Ext(f.Name()) == ".go" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("tools/%s has no .go files", e.Name())
		}
	}
}

func TestSmokeBasicInvocations(t *testing.T) {
	Run(t, "quickemu", nil, "--version").Contains(t, ".")
	Run(t, "quickemu", nil, "--capabilities").Contains(t, `"schemaVersion"`)
	Run(t, "quickemu", nil, "--help").Contains(t, "--vm")
	Run(t, "quickget", nil, "--help").Contains(t, "quickget")
	Run(t, "quickgetc", nil, "--version").Contains(t, "0.1.0")
}

// TestCatalogJSON replaces tools/check-catalog: --list-json must emit
// a non-empty JSON array.
func TestCatalogJSON(t *testing.T) {
	out := Run(t, "quickget", nil, "--list-json").Stdout
	var entries []any
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatalf("--list-json emitted invalid JSON: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("--list-json emitted an empty catalog")
	}
}
