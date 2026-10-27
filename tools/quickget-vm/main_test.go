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

func TestUnattendedISODetect(t *testing.T) {
	for _, ok := range []string{
		"Win10_22H2_English_x64.iso",
		"Win11_23H2_Spanish_x64v1.iso",
		"Win10_2009_x32.iso",
	} {
		if !isUnattendedWindowsISO(ok) {
			t.Fatalf("%s should be a retail windows iso", ok)
		}
	}
	for _, no := range []string{"ubuntu-24.04.iso", "windows-server-2022.iso", "Win10_x64.iso"} {
		if isUnattendedWindowsISO(no) {
			t.Fatalf("%s should not match", no)
		}
	}
}

func TestUnattendXMLRender(t *testing.T) {
	xml := strings.ReplaceAll(unattendXML, "${CULTURE}", "es-ES")
	xml = strings.ReplaceAll(xml, "${LANGID}", "0c0a")
	xml = strings.ReplaceAll(xml, "${DRIVER_DIR}", "w11")
	xml = strings.ReplaceAll(xml, "${QXLDOD_PATH}", "")
	if strings.Contains(xml, "${") {
		t.Fatal("unsubstituted placeholders remain")
	}
	for _, want := range []string{"es-ES", `E:\NetKVM\w11\amd64`, "BypassTPMCheck", "spice-vdagent"} {
		if !strings.Contains(xml, want) {
			t.Fatalf("missing %q", want)
		}
	}
	if strings.Contains(xml, "qxldod") {
		t.Fatal("w11 must not include qxldod (no w11 build)")
	}
}

func TestWinCulturesTable(t *testing.T) {
	if c := winCultures["Spanish"]; c != [2]string{"es-ES", "0c0a"} {
		t.Fatalf("spanish culture: %v", c)
	}
	if len(winCultures) != 37 {
		t.Fatalf("culture table should have 37 entries (Bash named arms), got %d", len(winCultures))
	}
}

func TestCustomConfLocalISO(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(old)
	if err := os.WriteFile("some-linux.iso", []byte("fake"), 0644); err != nil {
		t.Fatal(err)
	}
	if rc := customConf("myvm", "some-linux.iso", ""); rc != 0 {
		t.Fatal("customConf failed")
	}
	b, err := os.ReadFile("myvm.conf")
	if err != nil {
		t.Fatal("conf not written")
	}
	s := string(b)
	if !strings.Contains(s, `guest_os="linux"`) || !strings.Contains(s, `iso="myvm/some-linux.iso"`) {
		t.Fatalf("conf content: %s", s)
	}
	if _, err := os.Stat("some-linux.iso"); err == nil {
		t.Fatal("input should have been moved into the vm dir")
	}
	if _, err := os.Stat("myvm/some-linux.iso"); err != nil {
		t.Fatal("image not inside vm dir")
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
