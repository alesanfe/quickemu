// pipeline_test.go — end-to-end download pipeline: httptest server +
// stub quickget-catalog on PATH exercise fetch/fetchOnce/getOS/genSeedISO.
package main

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFetchPaths(t *testing.T) {
	payload := []byte("fake-iso-bytes-0123456789")
	sum := fmt.Sprintf("%x", sha256.Sum256(payload))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/missing") {
			w.WriteHeader(404)
			return
		}
		if rg := r.Header.Get("Range"); rg != "" {
			// "bytes=N-" → resume from N.
			var n int
			fmt.Sscanf(rg, "bytes=%d-", &n)
			if n >= len(payload) {
				w.WriteHeader(416)
				return
			}
			w.Header().Set("Content-Range",
				fmt.Sprintf("bytes %d-%d/%d", n, len(payload)-1, len(payload)))
			w.WriteHeader(206)
			w.Write(payload[n:])
			return
		}
		w.Write(payload)
	}))
	defer srv.Close()
	dir := t.TempDir()

	// clean fetch + verified sha256
	if rc := fetch(srv.URL+"/img.iso", filepath.Join(dir, "a.iso"), sum); rc != 0 {
		t.Fatal("fetch ok failed")
	}
	// wrong scheme rejected
	if rc := fetch("file:///etc/passwd", filepath.Join(dir, "b"), ""); rc != 1 {
		t.Fatal("file:// scheme should fail")
	}
	// sha mismatch → .part removed, rc 1
	if rc := fetch(srv.URL+"/img.iso", filepath.Join(dir, "c.iso"),
		strings.Repeat("0", 64)); rc != 1 {
		t.Fatal("bad sha should fail")
	}
	// resume: pre-stage half the payload, server honors Range
	part := filepath.Join(dir, "d.iso.part")
	os.WriteFile(part, payload[:8], 0o644)
	if rc := fetch(srv.URL+"/img.iso", filepath.Join(dir, "d.iso"), sum); rc != 0 {
		t.Fatal("resume fetch failed")
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "d.iso")); string(b) != string(payload) {
		t.Fatal("resumed file corrupted")
	}
}

func TestGetOSPipeline(t *testing.T) {
	payload := []byte("vm-image-content")
	sum := fmt.Sprintf("%x", sha256.Sum256(payload))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/SHA256SUMS":
			fmt.Fprintf(w, "%s  img.iso\n", sum)
		default:
			w.Write(payload)
		}
	}))
	defer srv.Close()

	// stub quickget-catalog printing "url hashurl".
	stub := t.TempDir()
	os.WriteFile(filepath.Join(stub, "quickget-catalog"),
		[]byte("#!/bin/sh\necho '"+srv.URL+"/img.iso "+srv.URL+"/SHA256SUMS"+"'\n"), 0o755)
	// no zsync on PATH → .zsync URLs fall back to plain fetch.
	t.Setenv("PATH", stub+string(os.PathListSeparator)+os.Getenv("PATH"))

	dir := filepath.Join(t.TempDir(), "testvm")
	if rc := getOS("testos", "1.0", "", dir, "amd64"); rc != 0 {
		t.Fatal("getOS failed")
	}
	if _, err := os.Stat(filepath.Join(dir, "img.iso")); err != nil {
		t.Fatal("image not downloaded")
	}
	// mkconf emitted the .conf beside the dir.
	confs, _ := filepath.Glob(dir + "*.conf")
	if len(confs) == 0 {
		confs, _ = filepath.Glob(filepath.Join(dir, "*.conf"))
	}
	if len(confs) == 0 {
		t.Fatal("no .conf emitted")
	}

	// url-resolution failure propagates.
	os.WriteFile(filepath.Join(stub, "quickget-catalog"),
		[]byte("#!/bin/sh\nexit 1\n"), 0o755)
	if rc := getOS("testos", "1.0", "", filepath.Join(t.TempDir(), "v2"), ""); rc != 1 {
		t.Fatal("catalog failure should fail getOS")
	}
}

func TestGenSeedISO(t *testing.T) {
	dir := t.TempDir()
	vm := filepath.Join(dir, "vm1")
	os.MkdirAll(vm, 0o755)
	conf := vm + ".conf"
	os.WriteFile(conf, []byte("guest_os=\"linux\"\n"), 0o644)
	t.Setenv("HOME", dir)
	t.Setenv("USER", "tester")
	if rc := genSeedISO(vm, conf); rc != 0 {
		t.Fatal("genSeedISO failed")
	}
	for _, f := range []string{"user-data", "meta-data", "seed.iso"} {
		if _, err := os.Stat(filepath.Join(vm, f)); err != nil {
			t.Fatalf("%s missing", f)
		}
	}
	if b, _ := os.ReadFile(conf); !strings.Contains(string(b), "seed=") {
		t.Fatal("conf lacks seed=")
	}
}
