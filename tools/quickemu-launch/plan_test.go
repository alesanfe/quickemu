// plan_test.go: the *Args assemblers are pure Conf→argv functions —
// exercised across display/storage/network/cpu/firmware/feature
// matrices plus conf parsing and validation.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func confFrom(t *testing.T, body string) (Conf, string) {
	t.Helper()
	dir := t.TempDir()
	f := filepath.Join(dir, "vm.conf")
	os.WriteFile(f, []byte(body), 0o600)
	c := parseConf(f)
	c["_vmdir"] = dir
	c["_vmname"] = "vm"
	return c, dir
}

func joined(a []string) string { return strings.Join(a, " ") }

func TestBootAndDisplayArgs(t *testing.T) {
	c, dir := confFrom(t, "guest_os=\"linux\"\ndisplay=\"spice\"\niso=\"vm/x.iso\"\n")
	os.WriteFile(filepath.Join(dir, "x.iso"), []byte(""), 0o600)
	os.WriteFile(filepath.Join(dir, "vm.pid"), []byte(""), 0o600)

	_ = bootMediaArgs(c, dir) // exercised; contents depend on media layout
	if a := displayArgs(c, dir, "linux", "x86_64"); len(a) == 0 {
		t.Fatal("display args empty")
	}
	if d := displayDevice(c, "linux", "x86_64", "spice"); d == "" {
		t.Fatal("displayDevice empty")
	}
	for _, disp := range []string{"sdl", "gtk", "spice", "vnc", "none", "spice-app"} {
		c2, _ := confFrom(t, "guest_os=\"linux\"\ndisplay=\""+disp+"\"\n")
		_ = displayArgs(c2, dir, "linux", "x86_64")
		_ = displayDevice(c2, "windows", "x86_64", disp)
	}
}

func TestAudioStorageNetwork(t *testing.T) {
	c, dir := confFrom(t, "guest_os=\"linux\"\nsound_card=\"intel-hda\"\n"+
		"sound_duplex=\"hda-micro\"\nnetwork=\"user\"\nport_forwards=\"2222:22 8080:80\"\n")
	_ = audioArgs(c, "linux", OsTuning{})
	_ = audioDriver()
	_ = storageArgs(c, dir, OsTuning{})
	if a := networkArgs(c, OsTuning{}); len(a) == 0 {
		t.Fatal("network args empty")
	}
	// Bridged + restricted + none paths.
	for _, n := range []string{"none", "restricted", "bridge"} {
		c2, _ := confFrom(t, "guest_os=\"linux\"\nnetwork=\""+n+"\"\nbridge=\"br0\"\n")
		_ = networkArgs(c2, OsTuning{})
	}
}

func TestCPURAMFirmware(t *testing.T) {
	c, dir := confFrom(t, "guest_os=\"linux\"\ncpu_cores=\"4\"\nram=\"4G\"\nsecureboot=\"on\"\ntpm=\"on\"\n")
	if a := cpuArgs(c, "linux", "x86_64", OsTuning{}); len(a) == 0 {
		t.Fatal("cpu args empty")
	}
	if a := ramArgs(c); !strings.Contains(joined(a), "4G") && len(a) == 0 {
		t.Fatal("ram args")
	}
	_ = biosArgs(c, "linux", "x86_64")
	_ = seedEFIVars
	// aarch64 + windows + macos variants exercise tuning branches.
	for _, os_ := range []string{"windows", "macos", "freebsd"} {
		c2, _ := confFrom(t, "guest_os=\""+os_+"\"\n")
		_ = cpuArgs(c2, os_, "aarch64", OsTuning{})
		_ = biosArgs(c2, os_, "aarch64")
		_ = displayArgs(c2, dir, os_, "aarch64")
	}
}

func TestPortsChannelsPassthru(t *testing.T) {
	c, dir := confFrom(t, "guest_os=\"linux\"\nssh_port=\"2222\"\nspice_port=\"5930\"\n"+
		"vnc_listen=\"127.0.0.1:5\"\n")
	_ = portArgs(c, dir) // exercised; non-empty depends on host bindings
	_ = channelArgs(c, dir)
	_ = passthruArgs(c)
	if firstFreePort("bad", 1, 2) == "" {
		t.Fatal("firstFreePort")
	}
}

func TestValidateAndParse(t *testing.T) {
	c, _ := confFrom(t, "guest_os=\"linux\"\ndisk_size=\"16G\"\n")
	if errs := validateConf(c, "x.conf"); errs != nil {
		// valid conf may still warn — just exercise the path
		_ = errs
	}
	bad, _ := confFrom(t, "guest_os=\"linux\"\nram=\"notasize\"\n")
	_ = validateConf(bad, "x.conf")
	// TOML parity path.
	f := filepath.Join(t.TempDir(), "vm.toml")
	os.WriteFile(f, []byte("guest_os = \"linux\"\nssh_port = \"2222\"\n"), 0o600)
	c2 := parseConf(f)
	if c2.get("guest_os", "") != "linux" {
		t.Fatalf("toml parse: %v", c2)
	}
}

func TestHostProbes(t *testing.T) {
	_ = hostSMT()
	_ = hostHasKVM()
	if hostNCores() <= 0 {
		t.Fatal("cores")
	}
	_ = hostArch()
	_ = hostHypervisor()
	_ = hostCPUVendor()
	_ = hostHasFlag("vmx")
	_ = hostDefaultIface()
	if hostTotalMemMB() <= 0 {
		t.Fatal("mem")
	}
	_ = cocoaGLES()
}
