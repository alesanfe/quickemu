// net3_test.go — osTweaks matrix, real-ISO extraction, normalize and
// customConf unattended branches.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
)

func TestOsTweaksMatrix(t *testing.T) {
	cases := []struct {
		os, rel, ed, want string
	}{
		{"alma", "9", "", `disk_size="32G"`},
		{"nixos", "24", "", `disk_size="32G"`},
		{"archlinux", "", "", `tpm="on"`},
		{"archlinux", "", "", `secureboot="on"`},
		{"openindiana", "", "", `boot="legacy"`},
		{"batocera", "", "", `disk_size="8G"`},
		{"bazzite", "", "", `disk_size="64G"`},
		{"qubes", "", "", `disk_size="128G"`},
		{"openbsd", "", "", `boot="legacy"`},
		{"slackware", "", "", `boot="legacy"`},
		{"debian", "12", "", `tpm="on"`},
		{"debian", "11", "", `secureboot="on"`},
		{"deepin", "", "", `ram="4G"`},
		{"freedos", "", "", `ram="256M"`},
		{"kolibrios", "", "", `ram="128M"`},
		{"templeos", "", "", `ram="512M"`},
		{"slint", "", "", `disk_size="50G"`},
		{"slitaz", "", "", `ram="512M"`},
		{"ubuntu-server", "22.04", "", `tpm="on"`},
		{"ubuntu-server", "24.04", "", `disk_size="10G"`},
		{"vanillaos", "", "", `disk_size="64G"`},
		{"zorin", "17", "education64", `disk_size="32G"`},
		{"zorin", "17", "core64", ""},
		{"reactos", "", "", `disk_size="12G"`},
		{"macos", "monterey", "", `macos_release="monterey"`},
		{"macos", "monterey", "", "cpu_cores=2"},
		{"macos", "sonoma", "", `disk_size="128G"`},
		{"elementary", "8.1", "", `display="spice"`},
		{"elementary", "8.0", "", ""},
		{"proxmox-ve", "", "", `disk_size="20G"`},
		{"ubuntu", "daily-live", "", `disk_size="32G"`},
		{"windows", "11", "", `tpm="on"`},
		{"windows", "10", "", `disk_size="64G"`},
		{"windows-server", "2022", "", `secureboot="off"`},
		{"unknownos", "1", "", ""},
	}
	for _, c := range cases {
		got := strings.Join(osTweaks(c.os, c.rel, c.ed), " ")
		if c.want != "" && !strings.Contains(got, c.want) {
			t.Fatalf("%s %s %s: %q lacks %q", c.os, c.rel, c.ed, got, c.want)
		}
	}
}

func buildISO(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	m := map[string]string{}
	for isoPath, content := range files {
		src := filepath.Join(dir, strings.ReplaceAll(isoPath, "/", "_"))
		os.WriteFile(src, []byte(content), 0o644)
		m[isoPath] = src
	}
	iso := filepath.Join(dir, "t.iso")
	if err := writeISO9660(iso, "t", m); err != nil {
		t.Fatalf("writeISO9660: %v", err)
	}
	return iso
}

func TestIsoExtractPaths(t *testing.T) {
	iso := buildISO(t, map[string]string{
		"/casper/vmlinuz": "kernel", "/casper/initrd": "initrd",
		"/other/x.txt": "x"})
	out := t.TempDir()
	if !isoExtractPure(iso, out, "casper") {
		t.Fatal("isoExtractPure failed on real ISO")
	}
	for _, f := range []string{"vmlinuz", "initrd"} {
		b, err := os.ReadFile(filepath.Join(out, f))
		if err != nil || len(b) == 0 {
			t.Fatalf("%s not extracted", f)
		}
	}
	// wrong prefix → false.
	if isoExtractPure(iso, t.TempDir(), "nope") {
		t.Fatal("wrong prefix should fail")
	}
	// not an ISO → false.
	bad := filepath.Join(t.TempDir(), "b.iso")
	os.WriteFile(bad, []byte("nope"), 0o644)
	if isoExtractPure(bad, t.TempDir(), "casper") {
		t.Fatal("garbage iso should fail")
	}
	// missing file → false.
	if isoExtractPure(filepath.Join(t.TempDir(), "x.iso"), t.TempDir(), "") {
		t.Fatal("missing iso should fail")
	}
	// bootfiles wrapper on the good ISO.
	out2 := t.TempDir()
	if !isoExtractBootfiles(iso, out2, "casper") {
		t.Fatal("isoExtractBootfiles failed")
	}
	// fallback: non-ISO → stub isoinfo on PATH.
	stub := t.TempDir()
	os.WriteFile(filepath.Join(stub, "isoinfo"),
		[]byte("#!/bin/sh\necho DATA\n"), 0o755)
	t.Setenv("PATH", stub+string(os.PathListSeparator)+os.Getenv("PATH"))
	// Non-ISO + stub isoinfo → the fallback path accepts it.
	out3 := t.TempDir()
	if !isoExtractBootfiles(bad, out3, "casper") {
		t.Error("isoinfo fallback should extract")
	}
}

func TestNormalizeCompressed(t *testing.T) {
	stubQemuImg(t)
	dir := t.TempDir()
	// openfyde .xz — real xz stream.
	xzf := filepath.Join(dir, "o.img.xz")
	f, _ := os.Create(xzf)
	w, _ := xz.NewWriter(f)
	w.Write([]byte("img"))
	w.Close()
	f.Close()
	if got := imageNormalize("openfyde", "o.img.xz", dir); got != "disk.qcow2" {
		t.Fatalf("openfyde: %q", got)
	}
	// redoxos .zst — real zstd stream.
	zf := filepath.Join(dir, "r.img.zst")
	f2, _ := os.Create(zf)
	zw, _ := zstd.NewWriter(f2)
	zw.Write([]byte("img"))
	zw.Close()
	f2.Close()
	if got := imageNormalize("redoxos", "r.img.zst", dir); got != "r.img" {
		t.Fatalf("redoxos: %q", got)
	}
	// dragonfly .bz2 invalid → decompress fails, name kept.
	os.WriteFile(filepath.Join(dir, "d.iso.bz2"), []byte("bad"), 0o644)
	imageNormalize("dragonflybsd", "d.iso.bz2", dir)
	// freedos .zip real — extractZip needs valid zip; make one.
	zipF := filepath.Join(dir, "fd.zip")
	os.WriteFile(zipF, testZip(t), 0o644)
	imageNormalize("freedos", "fd.zip", dir)
}

func testZip(t *testing.T) []byte {
	t.Helper()
	// minimal valid zip (end-of-central-directory only).
	return []byte{80, 75, 5, 6, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
}

func TestCustomConfUnattended(t *testing.T) {
	stubFetchers(t)
	dir := t.TempDir()
	// --unattended forces unattendedWindows even if the ISO marker
	// isn't detected (flag short-circuits detection).
	src := filepath.Join(dir, "windows-11.iso")
	os.WriteFile(src, []byte("x"), 0o644)
	vm := filepath.Join(dir, "wunatt")
	rc := customConf(vm, src, "--unattended")
	if rc != 0 {
		t.Fatalf("unattended customConf rc=%d", rc)
	}
}

func TestGetOSNoConfAndTweaks(t *testing.T) {
	stubFetchers(t)
	dir := t.TempDir()
	// noConf short-circuits after the download.
	stub := t.TempDir()
	os.WriteFile(filepath.Join(stub, "quickget-catalog"),
		[]byte("#!/bin/sh\necho 'https://x/img.iso'\n"), 0o755)
	t.Setenv("PATH", stub+string(os.PathListSeparator)+os.Getenv("PATH"))
	old := noConf
	noConf = true
	defer func() { noConf = old }()
	d := filepath.Join(dir, "nc")
	if rc := getOS("testos", "1.0", "", d, "amd64"); rc != 0 {
		t.Fatalf("getOS noConf rc=%d", rc)
	}
	if _, err := os.Stat(d + ".conf"); err == nil {
		t.Fatal("noConf should not write conf")
	}
}
