// download_test.go — populateCache overrides, extractArchive formats
// and the webRedirect/webCheck helpers over httptest.
package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
)

func TestPopulateCacheOverrides(t *testing.T) {
	cat := `[{"name":"alpine","releases":[{"release":"3.20"}]}]`
	// QUICKGETC_CATALOG wins outright.
	f := filepath.Join(t.TempDir(), "cat.json")
	os.WriteFile(f, []byte(cat), 0o600)
	t.Setenv("QUICKGETC_CATALOG", f)
	got := populateCache()
	if len(got) != 1 || got[0].Name != "alpine" {
		t.Fatalf("override %v", got)
	}
	// Plain JSON next to the cache dir also wins (no download).
	t.Setenv("QUICKGETC_CATALOG", "")
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	os.WriteFile(filepath.Join(cache, cacheJSON), []byte(cat), 0o600)
	got = populateCache()
	if len(got) != 1 {
		t.Fatalf("plain-json %v", got)
	}
	// Zstd cache file: written fresh → decoded without download.
	var buf bytes.Buffer
	zw, _ := zstd.NewWriter(&buf)
	zw.Write([]byte(cat))
	zw.Close()
	os.Remove(filepath.Join(cache, cacheJSON))
	os.WriteFile(filepath.Join(cache, cacheFile), buf.Bytes(), 0o600)
	got = populateCache()
	if len(got) != 1 || got[0].Releases[0].Release != "3.20" {
		t.Fatalf("zstd %v", got)
	}
}

func TestExtractArchiveFormats(t *testing.T) {
	dir := t.TempDir()

	// tar: archive containing a .iso member.
	tarPath := filepath.Join(dir, "a.tar")
	tw, _ := os.Create(tarPath)
	w := tar.NewWriter(tw)
	body := []byte("iso-bytes")
	w.WriteHeader(&tar.Header{Name: "inner.iso", Mode: 0o600, Size: int64(len(body))})
	w.Write(body)
	w.Close()
	tw.Close()
	if got := extractArchive("tar", tarPath, dir, "iso"); !strings.HasSuffix(got, "inner.iso") {
		t.Fatalf("tar %q", got)
	}
	// gz roundtrip.
	gzPath := filepath.Join(dir, "b.iso.gz")
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	gw.Write([]byte("gz-bytes"))
	gw.Close()
	os.WriteFile(gzPath, buf.Bytes(), 0o600)
	if got := extractArchive("gz", gzPath, dir, "iso"); !strings.HasSuffix(got, ".iso") {
		t.Fatalf("gz %q", got)
	}
	// xz roundtrip.
	xzPath := filepath.Join(dir, "c.iso.xz")
	buf.Reset()
	xw, _ := xz.NewWriter(&buf)
	xw.Write([]byte("xz-bytes"))
	xw.Close()
	os.WriteFile(xzPath, buf.Bytes(), 0o600)
	if got := extractArchive("xz", xzPath, dir, "iso"); !strings.HasSuffix(got, ".iso") {
		t.Fatalf("xz %q", got)
	}
	// zip.
	zipPath := filepath.Join(dir, "d.zip")
	zf, _ := os.Create(zipPath)
	zwr := zip.NewWriter(zf)
	zw2, _ := zwr.Create("z.iso")
	zw2.Write([]byte("zip-bytes"))
	zwr.Close()
	zf.Close()
	if got := extractArchive("zip", zipPath, dir, "iso"); got == "" {
		t.Fatalf("zip %q", got)
	}
	// Unknown format / missing file → returns input.
	if got := extractArchive("rar", filepath.Join(dir, "none"), dir, "iso"); got == "" {
		t.Fatal("missing must return file")
	}
}

func TestWebHelpers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redir" {
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()
	if got := webRedirect(srv.URL + "/redir"); !strings.HasSuffix(got, "/final") {
		t.Fatalf("redirect %q", got)
	}
	if got := webRedirect("::bad-url::"); got != "::bad-url::" {
		t.Fatalf("bad url %q", got)
	}
	if !webCheck(srv.URL+"/ok", nil) {
		t.Fatal("webCheck 200")
	}
	if webCheck(srv.URL+"/closed", nil) == webCheck("http://127.0.0.1:1/x", nil) {
		// both should be reachable-or-not consistently; at minimum
		// the dead port must fail.
		if webCheck("http://127.0.0.1:1/x", nil) {
			t.Fatal("dead port must fail")
		}
	}
}
