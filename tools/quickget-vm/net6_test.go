// net6_test.go — last-mile: checkChunklist signature branches (v2 valid,
// v1 RSA-mismatch, digest/chunk/trailing errors), getOS noConf/unattended/
// auto-dir branches, hostArch arm64.
package main

import (
	"crypto/sha256"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// writeChunklist builds a CNKL v1.1 fixture for dmgData with one chunk,
// choosing sigMethod 1 (256-byte RSA sig, here garbage) or 2 (sha256 digest).
func writeChunklist(t *testing.T, dir, name string, dmgData []byte, sigMethod byte, sig []byte) (string, string) {
	t.Helper()
	h := sha256.New()
	hdr := make([]byte, 36)
	copy(hdr[:4], "CNKL")
	binary.LittleEndian.PutUint32(hdr[4:8], 36)
	hdr[8], hdr[9], hdr[10] = 1, 1, sigMethod
	binary.LittleEndian.PutUint64(hdr[12:20], 1) // chunkCount
	binary.LittleEndian.PutUint64(hdr[20:28], 36)
	binary.LittleEndian.PutUint64(hdr[28:36], 36+36)
	h.Write(hdr)
	var rec [36]byte
	binary.LittleEndian.PutUint32(rec[:4], uint32(len(dmgData)))
	sum := sha256.Sum256(dmgData)
	copy(rec[4:36], sum[:])
	h.Write(rec[:])
	if sig == nil {
		sig = h.Sum(nil)
	}
	cl := append(append(hdr, rec[:]...), sig...)
	dmgPath := filepath.Join(dir, name+".dmg")
	clPath := filepath.Join(dir, name+".chunklist")
	if err := os.WriteFile(dmgPath, dmgData, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(clPath, cl, 0o644); err != nil {
		t.Fatal(err)
	}
	return dmgPath, clPath
}

func TestCheckChunklistV2(t *testing.T) {
	dir := t.TempDir()
	payload := []byte("dmg-bytes-for-chunk-zero")

	// Valid sigMethod=2 chunklist: header+record digest = sig.
	dmg, cl := writeChunklist(t, dir, "ok", payload, 2, nil)
	if err := checkChunklist(dmg, cl); err != nil {
		t.Fatalf("valid chunklist: %v", err)
	}

	// Corrupted digest → "chunklist digest mismatch".
	dmg, cl = writeChunklist(t, dir, "badsig", payload, 2, make([]byte, 32))
	if err := checkChunklist(dmg, cl); err == nil {
		t.Fatal("garbage digest should fail")
	}

	// sigMethod=1 with a garbage 256-byte signature → RSA mismatch.
	dmg, cl = writeChunklist(t, dir, "rsa", payload, 1, make([]byte, 256))
	if err := checkChunklist(dmg, cl); err == nil {
		t.Fatal("garbage RSA sig should fail")
	}

	// Chunk sha mismatch: valid chunklist, different dmg bytes.
	dmg, cl = writeChunklist(t, dir, "mm", payload, 2, nil)
	os.WriteFile(dmg, []byte("other-bytes-entirely-diff"), 0o644)
	if err := checkChunklist(dmg, cl); err == nil {
		t.Fatal("chunk content mismatch should fail")
	}

	// Short dmg → chunk read error.
	dmg, cl = writeChunklist(t, dir, "short", payload, 2, nil)
	os.WriteFile(dmg, payload[:8], 0o644)
	if err := checkChunklist(dmg, cl); err == nil {
		t.Fatal("truncated dmg should fail")
	}

	// Trailing dmg bytes after last chunk.
	dmg, cl = writeChunklist(t, dir, "trail", payload, 2, nil)
	os.WriteFile(dmg, append(payload, 0xFF), 0o644)
	if err := checkChunklist(dmg, cl); err == nil {
		t.Fatal("trailing bytes should fail")
	}
}

func TestGetOSBranchCoverage(t *testing.T) {
	payload := []byte("generic-os-image")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(payload)
	}))
	defer srv.Close()
	stub := t.TempDir()
	os.WriteFile(filepath.Join(stub, "quickget-catalog"),
		[]byte("#!/bin/sh\necho '"+srv.URL+"/img.iso'\n"), 0o755)
	t.Setenv("PATH", stub+string(os.PathListSeparator)+os.Getenv("PATH"))

	// noConf: download only, no mkconf.
	old := noConf
	noConf = true
	dir := filepath.Join(t.TempDir(), "noconf-vm")
	if rc := getOS("testos", "1.0", "", dir, "amd64"); rc != 0 {
		t.Fatal("noConf getOS failed")
	}
	if _, err := os.Stat(filepath.Join(dir, "img.iso")); err != nil {
		t.Fatal("image missing under noConf")
	}
	if confs, _ := filepath.Glob(dir + "*.conf"); len(confs) != 0 {
		t.Fatal("noConf must not emit a .conf")
	}
	noConf = false

	// --unattended on a non-ubuntu-server OS → warning, still rc 0.
	oldU := unattendedFlag
	unattendedFlag = true
	if rc := getOS("testos", "1.0", "", filepath.Join(t.TempDir(), "v2"), "amd64"); rc != 0 {
		t.Fatal("unattended-warning getOS failed")
	}
	unattendedFlag = oldU

	// dir == "" → auto-derived "<os>-<release>" dir under cwd.
	base := t.TempDir()
	cwd, _ := os.Getwd()
	os.Chdir(base)
	defer os.Chdir(cwd)
	if rc := getOS("testos", "1.0", "", "", "amd64"); rc != 0 {
		t.Fatal("auto-dir getOS failed")
	}
	if _, err := os.Stat(filepath.Join(base, "testos-1.0", "img.iso")); err != nil {
		t.Fatal("auto-derived dir missing")
	}
	_ = old
}

func TestHostArchVariants(t *testing.T) {
	old := goArch
	defer func() { goArch = old }()
	goArch = "arm64"
	if hostArch() != "arm64" {
		t.Fatal("arm64 should map to arm64")
	}
	goArch = "amd64"
	if hostArch() != "x86_64" {
		t.Fatal("amd64 should map to x86_64")
	}
}
