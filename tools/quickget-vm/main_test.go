package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGuestForTable(t *testing.T) {
	cases := []struct{ os, rel, wantG, wantT string }{
		{"windows", "11", "windows", "iso"},
		{"macos", "sonoma", "macos", "img"},
		{"ubuntu", "15.10", "linux_old", "iso"},
		{"ubuntu", "24.04", "linux", "iso"},
		{"freebsd", "14", "freebsd", "iso"},
		{"openindiana", "latest", "solaris", "iso"},
		{"debian", "12", "linux", "iso"},
	}
	for _, c := range cases {
		g, ty := guestFor(c.os, c.rel)
		if g != c.wantG || ty != c.wantT {
			t.Fatalf("%s %s -> %s/%s, want %s/%s", c.os, c.rel, g, ty, c.wantG, c.wantT)
		}
	}
}

func TestOsTweaks(t *testing.T) {
	j := strings.Join(osTweaks("windows", "11", ""), " ")
	if !strings.Contains(j, `tpm="on"`) || !strings.Contains(j, `disk_size="64G"`) {
		t.Fatalf("win11 tweaks: %s", j)
	}
	j = strings.Join(osTweaks("debian", "12", ""), " ")
	if !strings.Contains(j, `tpm="on"`) || !strings.Contains(j, `secureboot="on"`) {
		t.Fatalf("debian12 tweaks: %s", j)
	}
	j = strings.Join(osTweaks("macos", "monterey", ""), " ")
	if !strings.Contains(j, `macos_release="monterey"`) || !strings.Contains(j, "cpu_cores=2") {
		t.Fatalf("monterey tweaks: %s", j)
	}
	j = strings.Join(osTweaks("reactos", "0.4", ""), " ")
	if !strings.Contains(j, `ram="2048M"`) {
		t.Fatalf("reactos tweaks: %s", j)
	}
}

func TestMkconfDiskImageOverride(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(old)
	rc := mkconf("batocera", "39", "", "batocera.img", "", "batocera-39", "")
	if rc != 0 {
		t.Fatal("mkconf failed")
	}
	b, _ := os.ReadFile("batocera-39.conf")
	s := string(b)
	if !strings.Contains(s, `disk_img="batocera-39/batocera.img"`) {
		t.Fatalf("img should become the VM disk: %s", s)
	}
	if !strings.Contains(s, `disk_format="raw"`) {
		t.Fatalf("img should set raw format: %s", s)
	}
	if strings.Contains(s, `iso=`) {
		t.Fatalf("no iso expected for disk image: %s", s)
	}
	if !strings.Contains(s, `disk_size="8G"`) {
		t.Fatalf("batocera tweak missing: %s", s)
	}
}

func TestMkconfFixedIso(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(old)
	rc := mkconf("windows", "11", "", "win.iso", "virtio.iso", "windows-11", "")
	if rc != 0 {
		t.Fatal("mkconf failed")
	}
	b, _ := os.ReadFile("windows-11.conf")
	s := string(b)
	if !strings.Contains(s, `fixed_iso="windows-11/virtio.iso"`) {
		t.Fatalf("fixed_iso missing: %s", s)
	}
	if !strings.Contains(s, `tpm="on"`) {
		t.Fatalf("win11 tpm missing: %s", s)
	}
	if st, err := os.Stat(filepath.Join("windows-11")); err != nil || !st.IsDir() {
		t.Fatal("vmdir not created")
	}
}
