// net2_test.go — deep offline coverage: Windows Mido flow, macOS
// recovery pipeline, customConf, autoinstall, imageNormalize.
package main

import (
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func stubQemuImg(t *testing.T) {
	t.Helper()
	d := t.TempDir()
	// qemu-img stub: create touches the FILE arg (second-to-last for
	// create/resize … FILE SIZE), convert touches the last (-O dst).
	os.WriteFile(filepath.Join(d, "qemu-img"),
		[]byte("#!/bin/sh\nprev=\"\"; last=\"\"\nfor a; do prev=$last; last=$a; done\ncase \"$1\" in create|resize) touch \"$prev\";; convert) touch \"$last\";; esac\nexit 0\n"), 0o755)
	t.Setenv("PATH", d+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestDownloadWindowsWorkstation(t *testing.T) {
	stubFetchers(t)
	stubProbe(t, map[string]string{
		"software-download/windows": `<option value="1234">Windows 11`,
		"mdt.js":                    `?w=ABCDEF&rticks="+123456"`,
		"getskuinformation":         `{"Skus":[{"Id":1234,"LocalizedLanguage":"English International"}]}`,
		"GetProductDownloadLinks":   `{"ProductDownloadOptions":[{"Uri":"https://dl.microsoft.com/a/b/win-x64.iso?q=1"}]}`,
	})
	dir := t.TempDir()
	name, err := downloadWindowsWorkstation("11", "English International", dir)
	if err != nil || name == "" {
		t.Fatalf("workstation flow: %v %q", err, name)
	}
	if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
		t.Fatal("iso not fetched")
	}
	// no SKU match → error
	_, err = downloadWindowsWorkstation("11", "Klingon", dir)
	if err == nil {
		t.Fatal("unknown language should fail")
	}
	// blocked flow: empty link response → continue-with-error path
	stubProbe(t, map[string]string{
		"software-download/windows": `<option value="1234">Windows 11`,
		"getskuinformation":         `{"Skus":[{"Id":1234,"LocalizedLanguage":"English International"}]}`,
		"GetProductDownloadLinks":   "",
	})
	_, err = downloadWindowsWorkstation("10", "English International", dir)
	if err == nil {
		t.Fatal("blocked flow should return error")
	}
}

func TestDownloadWindowsServer(t *testing.T) {
	stubFetchers(t)
	stubProbe(t, map[string]string{
		"evalcenter": `<a href="https://go.microsoft.com/fwlink/p/?LinkID=111&clcid=0x409&culture=en-us&country=US">a</a>
			<a href="https://go.microsoft.com/fwlink/p/?LinkID=222&clcid=0x409&culture=en-us&country=US">b</a>
			<a href="https://go.microsoft.com/fwlink/p/?LinkID=333&clcid=0x409&culture=en-us&country=US">c</a>
			<a href="https://go.microsoft.com/fwlink/p/?LinkID=444&clcid=0x409&culture=en-us&country=US">d</a>`,
		"fwlink": "→https://dl.example.com/win-srv-2022.iso",
	})
	dir := t.TempDir()
	name, err := downloadWindowsServer("2022", "English", dir, "")
	if err != nil || name == "" {
		t.Fatalf("server flow: %v %q", err, name)
	}
	// enterprise/ltsc variants pick different link indexes.
	_, err = downloadWindowsServer("2022", "English", dir, "enterprise")
	if err != nil {
		t.Fatalf("enterprise: %v", err)
	}
	_, err = downloadWindowsServer("2022", "English", dir, "ltsc")
	if err != nil {
		t.Fatalf("ltsc: %v", err)
	}
	// empty page → error
	stubProbe(t, map[string]string{})
	if _, err = downloadWindowsServer("2022", "English", dir, ""); err == nil {
		t.Fatal("no links should fail")
	}
}

func TestGetMacosFlow(t *testing.T) {
	stubFetchers(t)
	stubQemuImg(t)
	stubProbe(t, map[string]string{
		"RecoveryImage": "https://oscdn.apple.com/x.dmg expires-dmg https://oscdn.apple.com/x.chunklist expires-cl",
	})
	dir := t.TempDir()
	var rel string
	for r := range macBoards {
		rel = r
		break
	}
	rc := getMacos(rel, filepath.Join(dir, "mac"), "amd64")
	if rc != 0 {
		t.Fatalf("getMacos rc=%d", rc)
	}
	if _, err := os.Stat(filepath.Join(dir, "mac", "RecoveryImage.img")); err != nil {
		t.Fatal("no RecoveryImage.img (qemu-img stub should create it)")
	}
}

func TestAppleRecoveryUnits(t *testing.T) {
	stubProbe(t, map[string]string{
		"osrecovery.apple.com": "x",
		"RecoveryImage":        "f1 f2",
	})
	if s, err := appleRecoverySession(); err != nil || s == "" {
		t.Fatalf("session: %v", err)
	}
	fields, err := appleRecoveryInfo("sess", "mlb", "board", "os")
	if err != nil || len(fields) == 0 {
		t.Fatalf("info: %v", err)
	}
	// macFetch writes the body to dest.
	dst := filepath.Join(t.TempDir(), "o.bin")
	if rc := macFetch("https://oscdn.apple.com/x.dmg", dst, "tok"); rc != 0 {
		t.Fatal("macFetch failed")
	}
	if _, err := os.Stat(dst); err != nil {
		t.Fatal("dest not written")
	}
}

func TestCustomConfBranches(t *testing.T) {
	stubFetchers(t)
	oldU := finalURL
	finalURL = func(u string) string { return u } // passthrough
	defer func() { finalURL = oldU }()
	dir := t.TempDir()

	// URL input ending in .iso → fetch + conf.
	if rc := customConf(filepath.Join(dir, "u1"), "https://x/reactos-0.4.iso", ""); rc != 0 {
		t.Fatalf("url iso rc=%d", rc)
	}
	// URL not .iso/.img → error.
	if rc := customConf(filepath.Join(dir, "u2"), "https://x/f.txt", ""); rc != 1 {
		t.Fatal("non-iso url should fail")
	}
	// existing dir → mkdir error.
	existing := filepath.Join(dir, "u3")
	os.Mkdir(existing, 0o755)
	if rc := customConf(existing, "x.iso", ""); rc != 1 {
		t.Fatal("existing dir should fail")
	}
	// local file, OS detection variants.
	for _, tc := range []struct{ name, file, wantOS string }{
		{"freebsd", "FreeBSD-13.iso", "freebsd"},
		{"windows", "windows11.iso", "windows"},
		{"winserver", "windows-server-2022.iso", "windows-server"},
		{"qcow2", "disk.qcow2", "linux"},
		{"vmdk", "d.vmdk", "linux"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := filepath.Join(dir, tc.file)
			os.WriteFile(src, []byte("x"), 0o644)
			vm := filepath.Join(dir, "vm-"+tc.name)
			if rc := customConf(vm, src, "--disable-unattended"); rc != 0 {
				t.Fatalf("customConf rc=%d", rc)
			}
			b, _ := os.ReadFile(vm + ".conf")
			if !strings.Contains(string(b), tc.wantOS) {
				t.Fatalf("conf lacks %s: %s", tc.wantOS, b)
			}
		})
	}
}

func TestGenUbuntuAutoinstallReal(t *testing.T) {
	dir := t.TempDir()
	// Build an ISO holding casper/vmlinuz + casper/initrd — map values
	// are real source file paths on disk.
	kern, initrd := filepath.Join(dir, "vmlinuz"), filepath.Join(dir, "initrd")
	os.WriteFile(kern, []byte("k"), 0o644)
	os.WriteFile(initrd, []byte("i"), 0o644)
	iso := filepath.Join(dir, "u.iso")
	if err := writeISO9660(iso, "u", map[string]string{
		"casper/vmlinuz": kern, "casper/initrd": initrd}); err != nil {
		t.Skip("writeISO9660: " + err.Error())
	}
	vm := filepath.Join(dir, "vm")
	os.MkdirAll(vm, 0o755)
	conf := vm + ".conf"
	os.WriteFile(conf, []byte("guest_os=\"linux\"\n"), 0o644)
	t.Setenv("USER", "tester")
	t.Setenv("HOME", dir)
	rc := genUbuntuAutoinstall(vm, iso, conf)
	if rc != 0 {
		t.Fatalf("autoinstall rc=%d", rc)
	}
	b, _ := os.ReadFile(conf)
	for _, k := range []string{"kernel=", "initrd=", "autoinstall"} {
		if !strings.Contains(string(b), k) {
			t.Fatalf("conf lacks %s", k)
		}
	}
	// bad iso → error.
	rc = genUbuntuAutoinstall(filepath.Join(dir, "v2"),
		filepath.Join(dir, "missing.iso"), conf)
	if rc != 1 {
		t.Fatal("bad iso should fail")
	}
}

func TestImageNormalizeMatrix(t *testing.T) {
	stubQemuImg(t)
	dir := t.TempDir()
	// .gz payload for batocera.
	gzf := filepath.Join(dir, "b.img.gz")
	f, _ := os.Create(gzf)
	w := gzip.NewWriter(f)
	w.Write([]byte("data"))
	w.Close()
	f.Close()
	if got := imageNormalize("batocera", "b.img.gz", dir); got != "b.img" {
		t.Fatalf("batocera: %q", got)
	}
	// .zip for freedos (extractZip already covered).
	os.WriteFile(filepath.Join(dir, "r.iso"), []byte("x"), 0o644)
	imageNormalize("freedos", "r.iso", dir)
	// easyos .img → qemu-img convert (stub creates disk.qcow2).
	os.WriteFile(filepath.Join(dir, "e.img"), []byte("x"), 0o644)
	if got := imageNormalize("easyos", "e.img", dir); got != "e" {
		t.Fatalf("easyos: %q", got)
	}
	// unknown os → passthrough.
	if got := imageNormalize("ubuntu", "u.iso", dir); got != "u.iso" {
		t.Fatalf("passthrough: %q", got)
	}
	// kolibrios .7z invalid → extract fails gracefully.
	os.WriteFile(filepath.Join(dir, "k.7z"), []byte("not-7z"), 0o644)
	imageNormalize("kolibrios", "k.7z", dir)
}
