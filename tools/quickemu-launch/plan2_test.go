// plan2_test.go — second sweep: quirks matrix, presets, spice/usb/
// input args, toml conf parsing, ports registry, misc helpers.
package main

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestQuirksMatrix(t *testing.T) {
	// spiceArgs probes real TCP ports for headless variants — stub the
	// probe so host port state cannot flake the sweep.
	oldListen := netListen
	netListen = func(_, _ string) (net.Listener, error) {
		return net.Listen("tcp", "127.0.0.1:0")
	}
	defer func() { netListen = oldListen }()
	for _, osName := range []string{"linux", "windows", "windows-server",
		"macos", "freebsd", "haiku", "kolibrios", "reactos", "solaris",
		"openbsd", "netbsd", "dragonflybsd", "freedos", "batocera",
		"openindiana", "qnx", "9front", "unknown-os"} {
		for _, arch := range []string{"x86_64", "aarch64"} {
			c, _ := confFrom(t, "guest_os=\""+osName+"\"\n")
			tu := osQuirks(c, osName, arch)
			_ = cpuArgs(c, osName, arch, tu)
			_ = audioArgs(c, osName, tu)
			_ = networkArgs(c, tu)
			_ = spiceArgs(c, "/tmp", "vm", osName, tu)
			_ = inputArgs(c, tu)
			_ = usbArgs(c, tu)
			_ = biosArgs(c, osName, arch)
		}
	}
	if a := rtcArgs("linux"); len(a) == 0 {
		t.Fatal("rtcArgs")
	}
	if rot13("Hello") != "Uryyb" || rot13(rot13("abc")) != "abc" {
		t.Fatal("rot13")
	}
}

func TestPresetsAndConfHelpers(t *testing.T) {
	c, _ := confFrom(t, "guest_os=\"linux\"\n")
	for _, p := range []string{"default", "soho", "dev", "test", "none"} {
		c2, _ := confFrom(t, "guest_os=\"linux\"\nprofile=\""+p+"\"\n")
		_ = applyPresets(c2)
	}
	_ = applyPresets(c)
	if !confOn(Conf{"tpm": "on"}, "tpm") || confOn(Conf{"tpm": "off"}, "tpm") ||
		confOn(Conf{}, "missing") {
		t.Fatal("confOn")
	}
	if l := confList(Conf{"usb_devices": `("a" "b")`}, "usb_devices"); len(l) == 0 {
		t.Fatal("confList")
	}
	if got := dotdir("/x/y/z"); got == "" {
		t.Fatal("dotdir")
	}
	if got := joinConf("/d", "f.iso"); got == "" {
		t.Fatal("joinConf")
	}
}

func TestTomlConfAndVmDir(t *testing.T) {
	dir := t.TempDir()
	toml := filepath.Join(dir, "vm.toml")
	os.WriteFile(toml, []byte("guest_os = \"linux\"\nram = \"4G\"\n"), 0o600)
	c := parseTomlConf(toml)
	if c["guest_os"] != "linux" || c["ram"] != "4G" {
		t.Fatalf("toml conf %v", c)
	}
	if vd := vmDir(c, toml); vd == "" {
		t.Fatal("vmDir")
	}
	// disk_img overrides the derived dir.
	c["disk_img"] = "/data/vm/d.qcow2"
	if vd := vmDir(c, toml); vd != "/data/vm" {
		t.Fatalf("vmDir disk_img %q", vd)
	}
	// A relative disk_img escaping the conf dir must not pull runtime
	// state outside it — same clamp as quickemu-cli's vmdir() so both
	// sides agree where .pid/.sock/.ports live.
	c["disk_img"] = "../out/d.qcow2"
	want := filepath.Join(dir, "vm")
	if vd := vmDir(c, toml); vd != want {
		t.Fatalf("vmDir must clamp ../ escape to %q, got %q", want, vd)
	}
	// A nested relative disk_img stays under the conf dir.
	c["disk_img"] = "vm/disks/d.qcow2"
	if vd := vmDir(c, toml); vd != filepath.Join(dir, "vm", "disks") {
		t.Fatalf("vmDir nested %q", vd)
	}
}

func TestPortsHelpers(t *testing.T) {
	// firstFreePort: no registry → returns start.
	if p := firstFreePort("0", 5900, 5910); p == "" {
		t.Fatal("firstFreePort")
	}
	// Registry in a temp XDG dir.
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	_ = portRegistry()
	if portRegistryLock() {
		portRegistryReserve(5900, os.Getpid(), "i", "vm")
		portRegistryUnlock()
	}
	if !portRegistryTaken(5900) {
		t.Fatal("reserved port must be taken")
	}
	portRegistryRelease(os.Getpid())
	if portRegistryTaken(5900) {
		t.Fatal("released port still taken")
	}
	if p := getPort(5900, 100, os.Getpid(), "vm"); p <= 0 {
		t.Fatal("getPort")
	}
	// portsRecord / spiceRecord write state files (runMode-gated).
	dir := t.TempDir()
	portsRecord(dir, "vm", "ssh,2222,127.0.0.1", true)
	spiceRecord(dir, "vm", "localhost:5930", true)
	portsRecord(dir, "vm", "noop", false) // no-op branch
	spiceRecord(dir, "vm", "noop", false)
	if b, _ := os.ReadFile(filepath.Join(dir, "vm.ports")); !strings.Contains(string(b), "2222") {
		t.Fatalf("ports record %s", b)
	}
}

// .ports/.spice are a cross-tool contract (quickgui parses them
// line-by-line): a conf-derived value carrying '\n' injects a fake
// record — e.g. monitor_telnet_host="x\nssh,22" fabricates an ssh
// line. Records must stay single-line.
func TestPortsRecordRejectsNewlines(t *testing.T) {
	dir := t.TempDir()
	portsRecord(dir, "vm", "monitor-telnet,4440,x\nssh,22", true)
	b, err := os.ReadFile(filepath.Join(dir, "vm.ports"))
	if err == nil && strings.Contains(string(b), "ssh,22") {
		t.Fatalf("injected record persisted: %q", b)
	}
	spiceRecord(dir, "vm", "5930\n--evil", true)
	if b, err := os.ReadFile(filepath.Join(dir, "vm.spice")); err == nil && strings.Contains(string(b), "evil") {
		t.Fatalf("injected spice record persisted: %q", b)
	}
}

func TestHostProbesAndMisc(t *testing.T) {
	_ = hostArch()
	_ = hostSMT()
	_ = hostHasKVM()
	_ = hostNCores()
	_ = hostHypervisor()
	_ = hostCPUVendor()
	_ = hostHasFlag("vmx")
	_ = hostTotalMemMB()
	_ = hostDefaultIface()
	_ = xdgUserDir("DESKTOP")
	// Firmware lookup: known names resolve (or empty string — host
	// dependent); the point is exercising the search.
	_ = firmwareFile("OVMF_CODE.fd", "AAVMF_CODE.fd")
	// macOS TSC check: pure decision on host facts.
	c, _ := confFrom(t, "guest_os=\"macos\"\n")
	_, _ = hostMacosTSCUnstable(c)
}

func TestPassthruChannelFileshare(t *testing.T) {
	c, dir := confFrom(t, "guest_os=\"linux\"\ngpu_passthrough=\"0000:01:00.0\"\n"+
		"public_dir=\"pub\"\n")
	os.MkdirAll(filepath.Join(dir, "pub"), 0o755)
	_ = passthruArgs(c)
	_ = channelArgs(c, dir)
	resolvePublic(c)
	_ = fileshareArgs(c, dir)
	// seedEFIVars on missing vars dir → no panic.
	_ = biosArgs(c, "linux", "aarch64")
}

func TestValidateConf(t *testing.T) {
	c, _ := confFrom(t, "guest_os=\"linux\"\nram=\"4G\"\n")
	_ = validateConf(c, "vm.conf")
	bad, _ := confFrom(t, "guest_os=\"linux\"\nram=\"notaram\"\ncpu_cores=\"abc\"\n")
	if w := validateConf(bad, "vm.conf"); len(w) == 0 {
		t.Fatal("bad conf must warn")
	}
}
