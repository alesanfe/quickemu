// pipeline_test.go — end-to-end download pipeline: httptest server +
// stub quickget-catalog on PATH exercise fetch/fetchOnce/getOS/genSeedISO.
package main

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/blake2b"
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

// fetchVerify accepts sha256/md5/sha512 by digest length (canaima md5,
// mageia sha512); rejects mismatches.
func TestFetchVerifyAlgos(t *testing.T) {
	payload := []byte("digest-algo-probe")
	f := filepath.Join(t.TempDir(), "p.iso")
	if err := os.WriteFile(f, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	if rc := fetchVerify(f, fmt.Sprintf("%x", md5.Sum(payload))); rc != 0 {
		t.Fatal("md5 verify")
	}
	if rc := fetchVerify(f, fmt.Sprintf("%x", sha512.Sum512(payload))); rc != 0 {
		t.Fatal("sha512 verify")
	}
	if rc := fetchVerify(f, fmt.Sprintf("%x", sha256.Sum256(payload))); rc != 0 {
		t.Fatal("sha256 verify")
	}
	bad := filepath.Join(t.TempDir(), "b.iso")
	os.WriteFile(bad, payload, 0o644)
	if rc := fetchVerify(bad, strings.Repeat("0", 32)); rc != 1 {
		t.Fatal("md5 mismatch must fail")
	}
	// check_hash parity: sha1 length guess (trisquel .sha1 files) and
	// explicit algo:hash prefixes incl. blake2b.
	if rc := fetchVerify(f, fmt.Sprintf("%x", sha1.Sum(payload))); rc != 0 {
		t.Fatal("sha1 (40-hex) must verify, not fall through to sha256")
	}
	if rc := fetchVerify(f, "sha256:"+fmt.Sprintf("%x", sha256.Sum256(payload))); rc != 0 {
		t.Fatal("sha256: prefix")
	}
	if rc := fetchVerify(f, "SHA512:"+fmt.Sprintf("%x", sha512.Sum512(payload))); rc != 0 {
		t.Fatal("case-insensitive SHA512: prefix")
	}
	b2 := blake2b.Sum512(payload)
	if rc := fetchVerify(f, "b2sum:"+fmt.Sprintf("%x", b2)); rc != 0 {
		t.Fatal("b2sum: prefix")
	}
	if rc := fetchVerify(f, "blake2b:"+fmt.Sprintf("%x", b2)); rc != 0 {
		t.Fatal("blake2b: alias")
	}
	// Unknown prefix / unguessable length warn and skip (rc 0), Bash
	// "not checking" — never a hard failure.
	if rc := fetchVerify(f, "crc32:deadbeef"); rc != 0 {
		t.Fatal("unknown algo must warn+skip")
	}
	if rc := fetchVerify(f, "abc"); rc != 0 {
		t.Fatal("unguessable length must warn+skip")
	}
}

// mkconf interpolates --dir/isoFile into key="value" lines without
// escaping — a quote or newline corrupts the conf or injects lines.
// Unsafe values must be rejected, never written.
func TestMkconfRejectsUnsafeValues(t *testing.T) {
	base := t.TempDir()
	cwd, _ := os.Getwd()
	if err := os.Chdir(base); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(cwd)
	for _, dir := range []string{"we\"ird", "two\nlines", "cr\ret"} {
		if rc := mkconf("linux", "24.04", "", "", "", dir, "amd64"); rc == 0 {
			t.Fatalf("mkconf accepted unsafe dir %q", dir)
		}
	}
	// release is interpolated into macos_release="..." — same vector.
	if rc := mkconf("macos", "ventu\"ra", "", "", "", "", "amd64"); rc == 0 {
		t.Fatal("mkconf accepted unsafe release")
	}
	// A sane dir must still work and produce a valid conf.
	if rc := mkconf("linux", "24.04", "", "", "", "good-name", "amd64"); rc != 0 {
		t.Fatal("mkconf rejected a sane dir")
	}
	b, err := os.ReadFile(filepath.Join(base, "good-name.conf"))
	if err != nil {
		t.Fatal("conf not written")
	}
	if !strings.Contains(string(b), "guest_os=\"linux\"") {
		t.Fatalf("conf missing guest_os: %s", b)
	}
}

// --cloud-init parity: Bash runs gen_seed_iso after writing the conf;
// mkconf accepted the flag but silently dropped it.
func TestMkconfCloudInit(t *testing.T) {
	base := t.TempDir()
	cwd, _ := os.Getwd()
	if err := os.Chdir(base); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(cwd)
	t.Setenv("HOME", base)
	t.Setenv("USER", "tester")
	if rc := mkconf("linux", "24.04", "", "", "", "ci-vm", "amd64", true); rc != 0 {
		t.Fatal("mkconf with cloudInit failed")
	}
	b, err := os.ReadFile(filepath.Join(base, "ci-vm.conf"))
	if err != nil {
		t.Fatal("conf not written")
	}
	if !strings.Contains(string(b), "seed=") {
		t.Fatalf("cloud-init conf missing seed= line: %s", b)
	}
	for _, f := range []string{"seed.iso", "user-data", "meta-data"} {
		if _, err := os.Stat(filepath.Join(base, "ci-vm", f)); err != nil {
			t.Fatalf("cloud-init artefact %s missing: %v", f, err)
		}
	}
}
