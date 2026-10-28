// net4_test.go — final coverage push: guestFor matrix, copyFile,
// decompressor error paths, macFetch/RecoverySession errors, hostArch.
package main

import (
	"compress/gzip"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGuestForMatrix(t *testing.T) {
	cases := []struct{ os, rel, wantG, wantT string }{
		{"batocera", "40", "batocera", "img"},
		{"custom", "x", "custom", "iso"},
		{"dragonflybsd", "6", "dragonflybsd", "iso"},
		{"easyos", "5", "linux", "img"},
		{"openfyde", "1", "linux", "img"},
		{"freebsd", "14", "freebsd", "iso"},
		{"ghostbsd", "24", "freebsd", "iso"},
		{"haiku", "r1", "haiku", "iso"},
		{"freedos", "1.3", "freedos", "iso"},
		{"kolibrios", "1", "kolibrios", "iso"},
		{"netbsd", "10", "netbsd", "iso"},
		{"openbsd", "7", "openbsd", "iso"},
		{"reactos", "0.4", "reactos", "iso"},
		{"templeos", "5", "templeos", "iso"},
		{"macos", "sonoma", "macos", "img"},
		{"openindiana", "h", "solaris", "iso"},
		{"ubuntu", "14.04", "linux_old", "iso"},
		{"ubuntu", "24.04", "linux", "iso"},
		{"ubuntu", "daily-live", "linux", "iso"},
		{"ubuntu-server", "24.04", "linux", "iso"},
		{"windows", "11", "windows", "iso"},
		{"windows-server", "2022", "windows-server", "iso"},
		{"alpine", "3.19", "linux", "iso"},
	}
	for _, c := range cases {
		g, ty := guestFor(c.os, c.rel)
		if g != c.wantG || ty != c.wantT {
			t.Fatalf("%s %s: got %s/%s want %s/%s", c.os, c.rel, g, ty, c.wantG, c.wantT)
		}
	}
}

func TestCopyFileAndDecompressErrors(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "s")
	os.WriteFile(src, []byte("payload"), 0o644)
	dst := filepath.Join(dir, "d")
	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile: %v", err)
	}
	if b, _ := os.ReadFile(dst); string(b) != "payload" {
		t.Fatal("copied content mismatch")
	}
	if err := copyFile(filepath.Join(dir, "none"), filepath.Join(dir, "e")); err == nil {
		t.Fatal("missing src should error")
	}
	// invalid streams → graceful false.
	for name, fn := range map[string]func(string) bool{
		"gz":  decompressGZ,
		"bz2": decompressBZ2,
		"xz":  decompressXZ,
		"zst": decompressZstd,
	} {
		bad := filepath.Join(dir, "x."+name)
		os.WriteFile(bad, []byte("garbage"), 0o644)
		if fn(bad) {
			t.Fatalf("%s: garbage should fail", name)
		}
		if fn(filepath.Join(dir, "missing."+name)) {
			t.Fatalf("%s: missing should fail", name)
		}
	}
	// valid gz round trip for decompressGZ's remaining branch.
	g := filepath.Join(dir, "ok.gz")
	f, _ := os.Create(g)
	w := gzip.NewWriter(f)
	io.WriteString(w, "data")
	w.Close()
	f.Close()
	if !decompressGZ(g) {
		t.Fatal("valid gz failed")
	}
	// extractZip on garbage.
	os.WriteFile(filepath.Join(dir, "b.zip"), []byte("x"), 0o644)
	if extractZip(filepath.Join(dir, "b.zip"), dir) {
		t.Fatal("garbage zip should fail")
	}
	// extract7z on garbage already covered; missing file → false.
	if extract7z(filepath.Join(dir, "none.7z"), dir) {
		t.Fatal("missing 7z should fail")
	}
	// run on a missing binary → error.
	if run("definitely-no-such-bin-xyz") == nil {
		t.Fatal("missing binary should error")
	}
	if run("sh", "-c", "exit 7") == nil {
		t.Fatal("nonzero exit should error")
	}
}

func TestHTTPErrorPaths(t *testing.T) {
	// appleRecoverySession with no session cookie → error.
	old := httpProbe
	httpProbe = &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200,
			Body:   io.NopCloser(strings.NewReader("x")),
			Header: make(http.Header), Request: r}, nil
	})}
	defer func() { httpProbe = old }()
	if _, err := appleRecoverySession(); err == nil {
		t.Fatal("no-cookie response should error")
	}
	// macFetch on ≥400.
	httpProbe = &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 404,
			Body:   io.NopCloser(strings.NewReader("")),
			Header: make(http.Header), Request: r}, nil
	})}
	if rc := macFetch("https://oscdn.apple.com/x", filepath.Join(t.TempDir(), "o"), "t"); rc != 1 {
		t.Fatal("404 should fail")
	}
	// hostArch just reports the runtime arch.
	if a := hostArch(); a == "" {
		t.Fatal("hostArch empty")
	}
}
