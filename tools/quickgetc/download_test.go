// download_test.go — populateCache overrides, extractArchive formats
// and the webRedirect/webCheck helpers over httptest.
package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/hex"
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
	// bz2 (no stdlib writer — a real fixture hex blob).
	bz2Hex := "425a68393141592653591309fb6700000419800002100032e6801020" +
		"00310340d029801ea6a83de563a021c2ee48a70a1202613f6ce0"
	bz2Blob, err := hex.DecodeString(bz2Hex)
	if err != nil {
		t.Fatal(err)
	}
	bz2Path := filepath.Join(dir, "e.iso.bz2")
	os.WriteFile(bz2Path, bz2Blob, 0o600)
	if got := extractArchive("bz2", bz2Path, dir, "iso"); !strings.HasSuffix(got, ".iso") {
		t.Fatalf("bz2 %q", got)
	}
	// Unknown format / missing file → returns input.
	if got := extractArchive("rar", filepath.Join(dir, "none"), dir, "iso"); got == "" {
		t.Fatal("missing must return file")
	}
}

// extractArchive joins member names onto vmPath — a tar/zip member
// named "../evil" from a hostile mirror would write outside the VM
// dir (same zip-slip class fixed in quickget-vm).
func TestExtractArchiveSlip(t *testing.T) {
	dir := t.TempDir()
	vmd := filepath.Join(dir, "vm")
	os.MkdirAll(vmd, 0o755)
	outside := filepath.Join(dir, "evil")
	// tar with a traversal member.
	tarPath := filepath.Join(dir, "s.tar")
	tw, _ := os.Create(tarPath)
	w := tar.NewWriter(tw)
	body := []byte("x")
	w.WriteHeader(&tar.Header{Name: "../evil", Mode: 0o600, Size: 1})
	w.Write(body)
	w.Close()
	tw.Close()
	extractArchive("tar", tarPath, vmd, "iso")
	if _, err := os.Stat(outside); err == nil {
		t.Fatal("tar member escaped the VM dir")
	}
	// zip with a traversal member.
	zipPath := filepath.Join(dir, "s.zip")
	zf, _ := os.Create(zipPath)
	zwr := zip.NewWriter(zf)
	zw2, _ := zwr.Create("../evil")
	zw2.Write(body)
	zwr.Close()
	zf.Close()
	extractArchive("zip", zipPath, vmd, "iso")
	if _, err := os.Stat(outside); err == nil {
		t.Fatal("zip member escaped the VM dir")
	}
}

// Regression: extractArchive ignored io.Copy errors — a truncated
// stream produced a partial artifact that the caller installed as the
// image. It must fall back to the untouched archive with no partial
// file left behind.
func TestExtractArchiveTruncated(t *testing.T) {
	dir := t.TempDir()
	// gz: valid header, stream cut mid-body.
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	gw.Write([]byte(strings.Repeat("x", 4096)))
	gw.Close()
	gzPath := filepath.Join(dir, "t.iso.gz")
	full := buf.Bytes()
	if err := os.WriteFile(gzPath, full[:len(full)-30], 0o600); err != nil {
		t.Fatal(err)
	}
	if got := extractArchive("gz", gzPath, dir, "iso"); got != gzPath {
		t.Fatalf("truncated gz must fall back to the archive, got %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "t.iso")); err == nil {
		t.Fatal("partial artifact must not survive")
	}
	// xz: same contract.
	buf.Reset()
	xw, _ := xz.NewWriter(&buf)
	xw.Write([]byte(strings.Repeat("y", 4096)))
	xw.Close()
	xzPath := filepath.Join(dir, "t2.iso.xz")
	full = buf.Bytes()
	if err := os.WriteFile(xzPath, full[:len(full)-30], 0o600); err != nil {
		t.Fatal(err)
	}
	if got := extractArchive("xz", xzPath, dir, "iso"); got != xzPath {
		t.Fatalf("truncated xz must fall back to the archive, got %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "t2.iso")); err == nil {
		t.Fatal("partial xz artifact must not survive")
	}
	// tar: last member's data cut short — the glob must not pick the
	// partial image member.
	var traw bytes.Buffer
	tw2 := tar.NewWriter(&traw)
	good := []byte("ok")
	tw2.WriteHeader(&tar.Header{Name: "a.txt", Mode: 0o600, Size: int64(len(good))})
	tw2.Write(good)
	img := []byte(strings.Repeat("i", 1024))
	tw2.WriteHeader(&tar.Header{Name: "disk.iso", Mode: 0o600, Size: int64(len(img))})
	tw2.Write(img)
	tw2.Close()
	tarPath := filepath.Join(dir, "t.tar")
	data := traw.Bytes()
	// Cut inside disk.iso's data: the tar trailer is 1024 bytes.
	if err := os.WriteFile(tarPath, data[:len(data)-1024-500], 0o600); err != nil {
		t.Fatal(err)
	}
	if got := extractArchive("tar", tarPath, dir, "iso"); got != tarPath {
		t.Fatalf("truncated image member must fall back, got %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "disk.iso")); err == nil {
		t.Fatal("partial member must be removed")
	}
	// bz2: same contract — truncated stream, no partial artifact.
	bz2Hex := "425a68393141592653591309fb6700000419800002100032e6801020" +
		"00310340d029801ea6a83de563a021c2ee48a70a1202613f6ce0"
	bz2Blob, err := hex.DecodeString(bz2Hex)
	if err != nil {
		t.Fatal(err)
	}
	bz2Path := filepath.Join(dir, "t3.iso.bz2")
	if err := os.WriteFile(bz2Path, bz2Blob[:len(bz2Blob)-20], 0o600); err != nil {
		t.Fatal(err)
	}
	if got := extractArchive("bz2", bz2Path, dir, "iso"); got != bz2Path {
		t.Fatalf("truncated bz2 must fall back to the archive, got %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "t3.iso")); err == nil {
		t.Fatal("partial bz2 artifact must not survive")
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
