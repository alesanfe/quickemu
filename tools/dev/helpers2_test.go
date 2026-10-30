package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// num(): parses bare floats and "NN%" strings; nil on anything else.
func TestNum(t *testing.T) {
	if v := num(62.5); v == nil || *v != 62.5 {
		t.Fatalf("num(float64) = %v", v)
	}
	if v := num("80%"); v == nil || *v != 80 {
		t.Fatalf("num(80%%) = %v", v)
	}
	if v := num("41.5"); v == nil || *v != 41.5 {
		t.Fatalf("num(41.5) = %v", v)
	}
	for _, bad := range []any{"abc", "", nil, true} {
		if num(bad) != nil {
			t.Fatalf("num(%v) should be nil", bad)
		}
	}
}

func TestLinesUniqOr(t *testing.T) {
	if lines("") != 0 || lines("a") != 1 || lines("a\nb\nc") != 3 {
		t.Fatal("lines miscounts")
	}
	if uniq("") != 0 || uniq("a\nb\na") != 2 {
		t.Fatal("uniq miscounts")
	}
	if or("", "b") != "b" || or("a", "b") != "a" {
		t.Fatal("or mispicks")
	}
}

// goCoverage(): seam'd exec returns canned `go test -cover` output;
// the mean across the fake module tree must be computed.
func TestGoCoverageSeamed(t *testing.T) {
	root := t.TempDir()
	for _, m := range []string{"a", "b"} {
		d := filepath.Join(root, "tools", m)
		os.MkdirAll(d, 0o755)
		os.WriteFile(filepath.Join(d, "go.mod"), []byte("module x/"+m+"\n"), 0o644)
	}
	calls := stubExec(t, "ok mod coverage: 80.0% of statements\n", 0)
	if got := goCoverage(root); got != "80%" {
		t.Fatalf("goCoverage = %q, want 80%%", got)
	}
	if len(*calls) != 2 {
		t.Fatalf("expected 2 exec calls, got %d", len(*calls))
	}
}

// goCoverage(): zero modules -> "0%" (no divide-by-zero).
func TestGoCoverageEmpty(t *testing.T) {
	root := t.TempDir()
	stubExec(t, "", 0)
	if got := goCoverage(root); got != "0%" {
		t.Fatalf("goCoverage(empty) = %q", got)
	}
}

// metricStructure(): read-only tree walk — emits the JSON report with
// every advertised metric key against the real repo.
func TestMetricStructure(t *testing.T) {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	rc := metricStructure()
	w.Close()
	os.Stdout = old
	var buf strings.Builder
	io.Copy(&buf, r)
	r.Close()
	if rc != 0 {
		t.Fatalf("metricStructure rc=%d\n%s", rc, buf.String())
	}
	for _, k := range []string{"loc_product", "loc_test", "test_funcs",
		"go_modules", "exec_command_sites"} {
		if !strings.Contains(buf.String(), k) {
			t.Errorf("metricStructure output missing %q:\n%s", k, buf.String())
		}
	}
}
