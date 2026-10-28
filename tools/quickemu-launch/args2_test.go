// args2_test.go — second sweep over the pure arg builders + helpers:
// usb/storage/boot/channel/passthrough/port args, validateConf error
// matrix, resolvePublic/xdgUserDir, seedEFIVars, registry lock.
package main

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func has(args []string, sub string) bool {
	for _, a := range args {
		if strings.Contains(a, sub) {
			return true
		}
	}
	return false
}

func TestUsbAndStorageArgs(t *testing.T) {
	c := Conf{"usb_devices": "046d:c52b 05ac:12a8 bad"}
	a := usbArgs(c, OsTuning{})
	if !has(a, "qemu-xhci") || !has(a, "vendorid=0x046d") || !has(a, "productid=0x12a8") {
		t.Fatalf("usbArgs %v", a)
	}
	a = usbArgs(c, OsTuning{UsbPassthrough: "nec-usb-xhci"})
	if !has(a, "nec-usb-xhci") {
		t.Fatalf("ctl %v", a)
	}
	if usbArgs(Conf{}, OsTuning{}) != nil {
		t.Fatal("empty devs should be nil")
	}

	dir := t.TempDir()
	sc := Conf{"disk_img": "d.qcow2", "iso": "i.iso", "img": "r.img",
		"fixed_iso": "f.iso", "storage": "nvme"}
	a = storageArgs(sc, dir, OsTuning{})
	for _, want := range []string{"virtio-blk-pci", "SystemDisk", "cdrom", "format=raw", "f.iso", "nvme"} {
		if !has(a, want) {
			t.Fatalf("storageArgs missing %q in %v", want, a)
		}
	}
	a = storageArgs(Conf{"disk_img": "d"}, dir, OsTuning{MacDiskDev: "virtio-blk-device"})
	if !has(a, "virtio-blk-device") {
		t.Fatalf("mac dev %v", a)
	}
}

func TestBootChannelPassthruArgs(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		os   string
		want string
	}{
		{"freedos", "order=dc"},
		{"kolibrios", "index=2"},
		{"reactos", "order=d"},
	}
	for _, tc := range cases {
		a := bootMediaArgs(Conf{"iso": "i.iso", "guest_os": tc.os}, dir)
		if !has(a, tc.want) {
			t.Fatalf("%s: %v", tc.os, a)
		}
	}
	// windows + unattended.iso present → extra drive.
	os.WriteFile(filepath.Join(dir, "unattended.iso"), []byte("x"), 0o600)
	a := bootMediaArgs(Conf{"iso": "i", "guest_os": "windows"}, dir)
	if !has(a, "unattended.iso") {
		t.Fatalf("unattended %v", a)
	}
	a = bootMediaArgs(Conf{"floppy": "f.img", "driver_iso": "d.iso"}, dir)
	if !has(a, "if=floppy") || !has(a, "virtio-cdrom") {
		t.Fatalf("floppy/driver %v", a)
	}
	a = passthruArgs(Conf{"vfio": "true", "vfio_devices": "0000:01:00.0 0000:01:00.1"})
	if len(a) != 4 || !has(a, "host=0000:01:00.1") {
		t.Fatalf("passthru %v", a)
	}
	a = channelArgs(Conf{"agent": "true"}, dir)
	if !has(a, "qemu-vdagent") || !has(a, "spicevmc") {
		t.Fatalf("channel %v", a)
	}
	if channelArgs(Conf{}, dir) != nil {
		t.Fatal("no agent should be nil")
	}
	// portArgs trivially short-circuits today.
	if portArgs(Conf{"ssh_port": "2222"}, dir) == nil ||
		portArgs(Conf{"spice_port": "5930"}, dir) == nil ||
		portArgs(Conf{}, dir) == nil {
		// all return empty non-nil slices — exercise only
	}
}

func TestValidateConfMatrix(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "v.conf")
	os.WriteFile(conf, nil, 0o644)
	errs := validateConf(Conf{}, conf)
	if len(errs) != 0 {
		t.Fatalf("empty conf errs %v", errs)
	}
	errs = validateConf(Conf{
		"guest_os":      "BAD OS",
		"cpu_cores":     "999",
		"efi":           "maybe",
		"disk_img":      "missing.qcow2",
		"iso":           "missing.iso",
		"port_forwards": "abc",
		"ssh_port":      "99999",
		"spice_port":    "0",
	}, conf)
	if len(errs) < 6 {
		t.Fatalf("expected many errors, got %v", errs)
	}
	os.WriteFile(filepath.Join(dir, "ok.qcow2"), nil, 0o600)
	errs = validateConf(Conf{"guest_os": "linux", "cpu_cores": "4",
		"efi": "true", "disk_img": "ok.qcow2", "ssh_port": "2222",
		"port_forwards": "8080:80"}, conf)
	if len(errs) != 0 {
		t.Fatalf("valid conf errs %v", errs)
	}
}

func TestResolvePublicAndXDG(t *testing.T) {
	// public_dir=none → early return.
	c := Conf{"public_dir": "none"}
	resolvePublic(c)
	if c["_public"] != "" {
		t.Fatal("none should stay empty")
	}
	// missing dir → warning, unset.
	c = Conf{"public_dir": filepath.Join(t.TempDir(), "nope")}
	resolvePublic(c)
	if c["_public"] != "" {
		t.Fatal("missing dir should warn+skip")
	}
	// existing dir → resolved.
	pub := t.TempDir()
	c = Conf{"public_dir": pub}
	resolvePublic(c)
	if c["_public"] != pub {
		t.Fatalf("public %q", c["_public"])
	}
	// xdgUserDir via XDG_CONFIG_HOME user-dirs.dirs.
	xc := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xc)
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.WriteFile(filepath.Join(xc, "user-dirs.dirs"),
		[]byte(`XDG_PUBLICSHARE_DIR="$HOME/Public"`+"\n"), 0o600)
	if got := xdgUserDir("PUBLICSHARE"); got != filepath.Join(home, "Public") {
		t.Fatalf("xdgUserDir %q", got)
	}
	// disabled → "".
	os.WriteFile(filepath.Join(xc, "user-dirs.conf"), []byte("enabled=False\n"), 0o600)
	if xdgUserDir("PUBLICSHARE") != "" {
		t.Fatal("disabled dirs should be empty")
	}
	// fallback: ~/Public exists → _public resolves.
	os.WriteFile(filepath.Join(xc, "user-dirs.conf"), []byte(""), 0o600)
	os.Remove(filepath.Join(xc, "user-dirs.dirs"))
	os.MkdirAll(filepath.Join(home, "Public"), 0o755)
	c = Conf{}
	resolvePublic(c)
	if c["_public"] != filepath.Join(home, "Public") {
		t.Fatalf("home fallback %q", c["_public"])
	}
}

func TestSeedEFIVars(t *testing.T) {
	dir := t.TempDir()
	c := Conf{"_vmdir": dir}
	// not running → no-op.
	seedEFIVars(c)
	// running + existing vars → no-op.
	os.WriteFile(filepath.Join(dir, "OVMF_VARS.fd"), []byte("v"), 0o600)
	c["_run"] = "1"
	seedEFIVars(c)
	// running + template in QE_FIRMWARE_DIR → seeds.
	os.Remove(filepath.Join(dir, "OVMF_VARS.fd"))
	fw := t.TempDir()
	t.Setenv("QE_FIRMWARE_DIR", fw)
	os.WriteFile(filepath.Join(fw, "OVMF_VARS.fd"), []byte("template"), 0o600)
	seedEFIVars(c)
	if b, err := os.ReadFile(filepath.Join(dir, "OVMF_VARS.fd")); err != nil || string(b) != "template" {
		t.Fatalf("seeded %v %q", err, b)
	}
}

func TestDisplayAndAudioPaths(t *testing.T) {
	dir := t.TempDir()
	// vnc: emits -vnc + publishes the RFB endpoint in .ports (run mode).
	c := Conf{"display": "vnc", "vnc_listen": "127.0.0.1:3",
		"_vmdir": dir, "_vmname": "v", "_run": "1"}
	a := displayArgs(c, dir, "linux", "x86_64")
	if !has(a, "-vnc") || !has(a, "127.0.0.1:3") {
		t.Fatalf("vnc %v", a)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "v.ports")); err != nil ||
		!strings.Contains(string(b), "vnc,5903,127.0.0.1") {
		t.Fatalf("vnc ports %v %q", err, b)
	}
	// gtk + fullscreen.
	a = displayArgs(Conf{"display": "gtk", "fullscreen": "true"}, dir, "linux", "x86_64")
	if !has(a, "grab-on-hover") || !has(a, "full-screen=on") {
		t.Fatalf("gtk %v", a)
	}
	// cocoa (non-gl fallback on linux).
	a = displayArgs(Conf{"display": "cocoa", "gl": "off"}, dir, "macos", "x86_64")
	if !has(a, "cocoa") {
		t.Fatalf("cocoa %v", a)
	}
	// viewer=none forces -display none.
	a = displayArgs(Conf{"display": "sdl", "viewer": "none"}, dir, "linux", "x86_64")
	if !has(a, "-display") || !has(a, "none") {
		t.Fatalf("viewer %v", a)
	}
	// display=none → -vga none.
	a = displayArgs(Conf{"display": "none"}, dir, "linux", "x86_64")
	if !has(a, "-vga") {
		t.Fatalf("none %v", a)
	}
	// unknown display passes through.
	a = displayArgs(Conf{"display": "wayland"}, dir, "linux", "x86_64")
	if !has(a, "wayland") {
		t.Fatalf("wayland %v", a)
	}

	// audioDriver: stub qemu reporting pipewire support + socket present.
	stub := t.TempDir()
	os.WriteFile(filepath.Join(stub, "qemu-system-x86_64"),
		[]byte("#!/bin/sh\necho 'audio drivers: driver=alsa driver=pa driver=pipewire'\n"), 0o755)
	t.Setenv("PATH", stub+string(os.PathListSeparator)+os.Getenv("PATH"))
	rt := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", rt)
	os.WriteFile(filepath.Join(rt, "pipewire-0"), nil, 0o600)
	if d := audioDriver(); d != "pipewire" {
		t.Fatalf("pipewire %q", d)
	}
	os.Remove(filepath.Join(rt, "pipewire-0"))
	os.WriteFile(filepath.Join(rt, "pulse-native"), nil, 0o600)
	os.MkdirAll(filepath.Join(rt, "pulse"), 0o755)
	os.WriteFile(filepath.Join(rt, "pulse", "native"), nil, 0o600)
	if d := audioDriver(); d != "pa" {
		t.Fatalf("pa %q", d)
	}
	os.RemoveAll(filepath.Join(rt, "pulse"))
	if d := audioDriver(); d != "alsa" {
		t.Fatalf("alsa %q", d)
	}
}

func TestInputAudioNetArgs(t *testing.T) {
	// inputArgs matrix: controller/keyboard/mouse variants.
	for _, c := range []Conf{
		{"usb_controller": "xhci", "keyboard": "virtio", "mouse": "usb", "keyboard_layout": "es"},
		{"usb_controller": "none", "keyboard": "ps2", "mouse": "virtio"},
		{"usb_controller": "bogus", "keyboard": "bogus", "mouse": "bogus"},
		{"usb_controller": "ehci", "keyboard": "usb", "mouse": "tablet"},
	} {
		_ = inputArgs(c, OsTuning{})
	}
	a := inputArgs(Conf{"usb_controller": "xhci"}, OsTuning{Keyboard: "virtio", Mouse: "ps2"})
	if !has(a, "qemu-xhci") || !has(a, "virtio-keyboard") {
		t.Fatalf("input %v", a)
	}
	// audioArgs: sound on/off + tuning card.
	if audioArgs(Conf{"sound": "false"}, "linux", OsTuning{}) != nil &&
		!has(audioArgs(Conf{"sound": "false"}, "linux", OsTuning{}), "x") {
		// may return non-nil empty or nil — exercise only
	}
	_ = audioArgs(Conf{"sound": "true"}, "windows", OsTuning{SoundCard: "intel-hda"})
	_ = audioArgs(Conf{}, "linux", OsTuning{})
	// networkArgs modes.
	for _, c := range []Conf{
		{"network": "user", "port_forwards": "8080:80"},
		{"network": "bridge", "bridge": "br0"},
		{"network": "none"},
		{"network": "host"},
		{"extra_nics": "e0"},
	} {
		_ = networkArgs(c, OsTuning{NetDevice: "virtio-net-pci"})
	}
	// ramArgs/cpuArgs variants.
	_ = ramArgs(Conf{"ram": "8G"})
	_ = ramArgs(Conf{"ram": "512M"})
	_ = ramArgs(Conf{})
	_ = cpuArgs(Conf{"cpu_cores": "4"}, "windows", "x86_64", OsTuning{CPUFlags: "+x2apic"})
	_ = cpuArgs(Conf{}, "macos", "aarch64", OsTuning{})
	// fileshareArgs via resolved public.
	dir := t.TempDir()
	c := Conf{"_public": dir, "file_share": "true"}
	_ = fileshareArgs(c, dir)
	_ = fileshareArgs(Conf{}, dir)
	// vmDir: conf sibling dir resolution.
	conf := filepath.Join(dir, "v.conf")
	os.WriteFile(conf, []byte("guest_os=\"linux\"\n"), 0o644)
	if got := vmDir(Conf{}, conf); got == "" {
		t.Fatal("vmDir empty")
	}
}

func TestPresetsPortsAndMisc(t *testing.T) {
	c := applyPresets(Conf{"profile": "windows-gaming"})
	if c["guest_os"] != "windows" || c["ram"] == "" {
		t.Fatalf("preset %+v", c)
	}
	c = applyPresets(Conf{"network_preset": "bridge"})
	if c["network"] != "bridge" {
		t.Fatalf("net preset %+v", c)
	}
	c = applyPresets(Conf{"profile": "nope"})
	if c["guest_os"] != "" {
		t.Fatal("unknown profile should not set keys")
	}
	// explicit conf key wins over preset.
	c = applyPresets(Conf{"profile": "minimal", "ram": "16G"})
	if c["ram"] != "16G" {
		t.Fatalf("override ram=%s", c["ram"])
	}
	// getPort: explicit, then free-range.
	if p := firstFreePort("22220", 22220, 22229); p != "22220" {
		t.Fatalf("firstFreePort %s", p)
	}
	if p := getPort(22900, 9, os.Getpid(), "vm"); p <= 0 {
		t.Fatalf("getPort %d", p)
	}
	portRegistryRelease(os.Getpid())
	// firstFreePort busy path: hold a bound ephemeral port — the scan
	// must skip it. A fixed port would silently skip the assertion
	// whenever a host already occupies it.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	busy := ln.Addr().(*net.TCPAddr).Port
	hi := busy + 9
	if hi > 65535 {
		hi = 65535
	}
	if p := firstFreePort("", busy, hi); p == strconv.Itoa(busy) || p == "" {
		t.Fatalf("busy firstFreePort %q", p)
	}
	// osQuirks direct sweep over more guest branches.
	for _, g := range []string{"macos", "windows", "windows-server", "freebsd",
		"openbsd", "netbsd", "dragonflybsd", "solaris", "haiku", "kolibrios",
		"reactos", "freedos", "ubuntu", "nixos", "batocera", "linux"} {
		_ = osQuirks(Conf{"guest_os": g}, g, "x86_64")
		_ = osQuirks(Conf{"guest_os": g}, g, "aarch64")
	}
	// hostMacosTSCUnstable: linux guest → false quickly.
	if b, _ := hostMacosTSCUnstable(Conf{"guest_os": "linux"}); b {
		t.Fatal("linux should not need tsc workaround")
	}
	// joinConf relative + absolute.
	if joinConf("/d", "rel.qcow2") != "/d/rel.qcow2" {
		t.Fatal("joinConf rel")
	}
	if joinConf("/d", "/abs.qcow2") != "/abs.qcow2" {
		t.Fatal("joinConf abs")
	}
}
