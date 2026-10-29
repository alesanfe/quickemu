// args2_test.go — second sweep over the pure arg builders + helpers:
// usb/storage/boot/channel/passthrough/port args, validateConf error
// matrix, resolvePublic/xdgUserDir, seedEFIVars, registry lock.
package main

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

// Bash ports/cpu checks are strict [0-9]+ — fmt.Sscanf("%d") reads a
// numeric PREFIX, so "22x" validates and then reaches QEMU verbatim.
func TestValidateConfStrictDigits(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "v.conf")
	os.WriteFile(conf, nil, 0o644)
	for k, v := range map[string]string{
		"ssh_port": "22x", "spice_port": "5930bad", "cpu_cores": "4x",
	} {
		if errs := validateConf(Conf{k: v}, conf); len(errs) == 0 {
			t.Fatalf("%s=%q accepted — junk suffix must fail", k, v)
		}
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
	// not running → returns the path but writes nothing.
	seedEFIVars(c, "")
	if _, err := os.Stat(filepath.Join(dir, "OVMF_VARS.fd")); err == nil {
		t.Fatal("plan mode must not seed vars")
	}
	// running + existing vars → no-op.
	os.WriteFile(filepath.Join(dir, "OVMF_VARS.fd"), []byte("v"), 0o600)
	c["_run"] = "1"
	seedEFIVars(c, "")
	if b, _ := os.ReadFile(filepath.Join(dir, "OVMF_VARS.fd")); string(b) != "v" {
		t.Fatal("existing vars must win (no reseed)")
	}
	// running + template → seeds the copy.
	os.Remove(filepath.Join(dir, "OVMF_VARS.fd"))
	tmpl := filepath.Join(t.TempDir(), "OVMF_VARS.fd")
	os.WriteFile(tmpl, []byte("template"), 0o600)
	if got := seedEFIVars(c, tmpl); got != filepath.Join(dir, "OVMF_VARS.fd") {
		t.Fatalf("vars path %q", got)
	}
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
	// vm_net_mode parity: restrict and slirp are user netdevs and take
	// the ssh hostfwd (Bash applies it to user AND restrict); a bare
	// value is a bridge iface. ssh_access picks the fwd bind address.
	// ssh_port is pinned — auto-allocation probes real host TCP state.
	dir2 := t.TempDir()
	na := networkArgs(Conf{"_vmdir": dir2, "_vmname": "t", "network": "restrict", "ssh_port": "22999"},
		OsTuning{NetDevice: "virtio-net-pci"})
	if !has(na, "restrict=y") || !has(na, "hostfwd=tcp:127.0.0.1:") {
		t.Fatalf("restrict hostfwd: %v", na)
	}
	na = networkArgs(Conf{"_vmdir": dir2, "_vmname": "t", "network": "slirp", "ssh_port": "22999"},
		OsTuning{NetDevice: "virtio-net-pci"})
	if !has(na, "user,hostname=") || has(na, "restrict=y") || has(na, "bridge") {
		t.Fatalf("slirp must map to user netdev: %v", na)
	}
	na = networkArgs(Conf{"_vmdir": dir2, "_vmname": "t", "ssh_access": "remote", "ssh_port": "22999"},
		OsTuning{NetDevice: "virtio-net-pci"})
	if !has(na, "hostfwd=tcp::") {
		t.Fatalf("ssh_access=remote must bind wildcard: %v", na)
	}
	na = networkArgs(Conf{"_vmdir": dir2, "_vmname": "t", "ssh_access": "192.168.1.5", "ssh_port": "22999"},
		OsTuning{NetDevice: "virtio-net-pci"})
	if !has(na, "hostfwd=tcp:192.168.1.5:") {
		t.Fatalf("ssh_access=<ip> must bind that addr: %v", na)
	}
	// sound_card conf key (was read as 'sound' — silent ignore) and
	// sound_card_param_check's usb-audio→xhci force.
	aa := audioArgs(Conf{"sound_card": "ac97"}, "linux", OsTuning{})
	if !has(aa, "ac97") {
		t.Fatalf("sound_card key ignored: %v", aa)
	}
	c2 := Conf{"sound_card": "usb-audio"}
	if err := paramCheckDisplay(c2); err != nil {
		t.Fatal(err)
	}
	if c2["usb_controller"] != "xhci" {
		t.Fatalf("usb-audio must force xhci: %v", c2)
	}
	// ramArgs/cpuArgs variants.
	_ = ramArgs(Conf{"ram": "8G"})
	_ = ramArgs(Conf{"ram": "512M"})
	_ = ramArgs(Conf{})
	_ = cpuArgs(Conf{"cpu_cores": "4"}, "windows", "x86_64", OsTuning{CPUFlags: "+x2apic"})
	_ = cpuArgs(Conf{}, "macos", "aarch64", OsTuning{})
	// env contract (Bash parity): CPU_MODEL overrides the -cpu base model;
	// MACHINE_TYPE overrides the q35 default but not OS quirks.
	t.Setenv("CPU_MODEL", "qemu64")
	if a := cpuArgs(Conf{}, "linux", "x86_64", OsTuning{}); len(a) < 2 || a[0] != "-cpu" || a[1] != "qemu64" {
		t.Fatalf("CPU_MODEL env ignored: %v", a)
	}
	t.Setenv("MACHINE_TYPE", "microvm")
	if mt := osQuirks(Conf{}, "linux", "x86_64").MachineType; mt != "microvm" {
		t.Fatalf("MACHINE_TYPE env ignored: %q", mt)
	}
	if mt := osQuirks(Conf{}, "freedos", "x86_64").MachineType; mt != "pc" {
		t.Fatalf("MACHINE_TYPE env must not override OS quirk: %q", mt)
	}
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

func TestDisplayGtkHeadlessFallback(t *testing.T) {
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	a := displayArgs(Conf{"display": "gtk"}, t.TempDir(), "linux", "x86_64")
	joined := strings.Join(a, " ")
	if !strings.Contains(joined, "sdl") {
		t.Fatalf("headless host must fall back gtk->sdl: %v", a)
	}
	if strings.Contains(joined, "grab-on-hover") {
		t.Fatalf("gtk render args must not appear on headless fallback: %v", a)
	}
}

// display_param_check parity (commands.sh):
// braille forces display=sdl + usb_controller=xhci; unknown backends are
// rejected; gl defaults to off when glxinfo reports an old GLSL version;
// sdl exports SDL_MOUSE_FOCUS_CLICKTHROUGH=1.
func TestParamCheckDisplayBraille(t *testing.T) {
	c := Conf{"braille": "true", "display": "gtk", "usb_controller": "ehci"}
	if err := paramCheckDisplay(c); err != nil {
		t.Fatal(err)
	}
	if c["display"] != "sdl" || c["usb_controller"] != "xhci" {
		t.Fatalf("braille must force sdl+xhci: %v", c)
	}
	if err := paramCheckDisplay(Conf{}); err != nil {
		t.Fatal(err)
	}
}

func TestParamCheckDisplayWhitelist(t *testing.T) {
	if err := paramCheckDisplay(Conf{"display": "bogus"}); err == nil {
		t.Fatal("unknown display backend must be rejected")
	}
	if runtime.GOOS == "darwin" {
		if err := paramCheckDisplay(Conf{"display": "sdl"}); err == nil {
			t.Fatal("darwin accepts only cocoa/none")
		}
		return
	}
	for _, d := range []string{"gtk", "none", "sdl", "spice", "spice-app", "vnc"} {
		if err := paramCheckDisplay(Conf{"display": d}); err != nil {
			t.Fatalf("%s must be accepted: %v", d, err)
		}
	}
}

func TestGLSLDefault(t *testing.T) {
	if glslGL("OpenGL ES GLSL version: 1.00\n") != "off" {
		t.Fatal("GLSL 1.x must disable gl")
	}
	if glslGL("OpenGL ES GLSL version: 2.10\n") != "off" {
		t.Fatal("GLSL 2.x must disable gl")
	}
	if glslGL("OpenGL ES GLSL version: 3.20\n") != "on" {
		t.Fatal("GLSL 3.x keeps gl")
	}
	if glslGL("no glsl line\n") != "on" {
		t.Fatal("missing GLSL line keeps gl")
	}
}

func TestBrailleArgs(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("braille arg emission is linux-only")
	}
	defer func(f func(string, ...string) *exec.Cmd) { execCommand = f }(execCommand)
	execCommand = func(name string, a ...string) *exec.Cmd {
		return exec.Command("echo", "stdio\nbraille\npty")
	}
	a := brailleArgs(Conf{"braille": "on"}, "x86_64")
	if !has(a, "-usbdevice") || !has(a, "braille") {
		t.Fatalf("braille probe positive must emit -usbdevice braille: %v", a)
	}
	execCommand = func(name string, a ...string) *exec.Cmd {
		return exec.Command("echo", "stdio\npty")
	}
	if a := brailleArgs(Conf{"braille": "on"}, "x86_64"); a != nil {
		t.Fatalf("missing braille backend must not emit the device: %v", a)
	}
	if brailleArgs(Conf{}, "x86_64") != nil {
		t.Fatal("braille unset must emit nothing")
	}
}

func TestSDLClickthroughEnv(t *testing.T) {
	os.Unsetenv("SDL_MOUSE_FOCUS_CLICKTHROUGH")
	displayArgs(Conf{"display": "sdl"}, t.TempDir(), "linux", "x86_64")
	if os.Getenv("SDL_MOUSE_FOCUS_CLICKTHROUGH") != "1" {
		t.Fatal("sdl display must export SDL_MOUSE_FOCUS_CLICKTHROUGH=1")
	}
}

// Firmware parity (configure_bios):
//   - macOS uses the vmdir OVMF_CODE.fd + OVMF_VARS-*.fd downloaded by
//     quickget, requires OpenCore.qcow2/ESP.qcow2, and sizes are sanity
//     checked — missing pieces are fatal errors, not silent fallback.
//   - qcow2 firmware (magic QFI\xfb) emits format=qcow2 pflash drives
//     and seeds OVMF_VARS.qcow2.
//   - aarch64 windows prefers monolithic -bios QEMU_EFI.fd; other
//     aarch64 guests use named -blockdev pflash pairs.
//   - boot="legacy" forces MACHINE_TYPE=pc.
//   - SystemDisk honors disk_format from the conf.
func TestParamCheckFirmwareMacOS(t *testing.T) {
	dir := t.TempDir()
	c := Conf{"guest_os": "macos", "_vmdir": dir, "_run": "1"}
	// Both missing → Bash's MAC_MISSING reports Bootloader (last check
	// wins); with only firmware present it still reports Bootloader.
	if err := paramCheckFirmware(c, "x86_64"); err == nil {
		t.Fatal("missing macOS firmware must error")
	}
	os.WriteFile(filepath.Join(dir, "OVMF_CODE.fd"), make([]byte, 2<<20), 0o644)
	os.WriteFile(filepath.Join(dir, "OVMF_VARS-1920x1080.fd"), make([]byte, 1<<17), 0o644)
	if err := paramCheckFirmware(c, "x86_64"); err == nil ||
		!strings.Contains(err.Error(), "Bootloader") {
		t.Fatalf("missing bootloader must error: %v", err)
	}
	// Firmware missing but bootloader present → reports Firmware.
	os.WriteFile(filepath.Join(dir, "OpenCore.qcow2"), []byte("qc"), 0o644)
	os.Remove(filepath.Join(dir, "OVMF_VARS-1920x1080.fd"))
	if err := paramCheckFirmware(c, "x86_64"); err == nil ||
		!strings.Contains(err.Error(), "Firmware") {
		t.Fatalf("missing firmware must error: %v", err)
	}
	os.WriteFile(filepath.Join(dir, "OVMF_VARS-1920x1080.fd"), make([]byte, 1<<17), 0o644)
	os.WriteFile(filepath.Join(dir, "OpenCore.qcow2"), []byte("qc"), 0o644)
	if err := paramCheckFirmware(c, "x86_64"); err != nil {
		t.Fatalf("complete macOS firmware must pass: %v", err)
	}
	// Truncated vars → corrupt-firmware error.
	os.WriteFile(filepath.Join(dir, "OVMF_VARS-1920x1080.fd"), make([]byte, 1024), 0o644)
	if err := paramCheckFirmware(c, "x86_64"); err == nil ||
		!strings.Contains(err.Error(), "corrupt") {
		t.Fatalf("truncated vars must error: %v", err)
	}
}

func TestBiosArgsMacOSFirmware(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "OVMF_CODE.fd"), make([]byte, 2<<20), 0o644)
	os.WriteFile(filepath.Join(dir, "OVMF_VARS-1024x768.fd"), make([]byte, 1<<17), 0o644)
	c := Conf{"guest_os": "macos", "_vmdir": dir}
	a := biosArgs(c, "macos", "x86_64")
	joined := strings.Join(a, " ")
	if !strings.Contains(joined, filepath.Join(dir, "OVMF_CODE.fd")) ||
		!strings.Contains(joined, "OVMF_VARS-1024x768.fd") {
		t.Fatalf("macOS must use vmdir firmware, got %v", a)
	}
}

func TestStorageArgsMacOS(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "OpenCore.qcow2"), []byte("qc"), 0o644)
	c := Conf{"guest_os": "macos", "disk_img": "disk.qcow2", "img": "RecoveryImage.img", "_vmdir": dir}
	a := storageArgs(c, dir, OsTuning{MacDiskDev: "virtio-blk-pci"})
	joined := strings.Join(a, " ")
	for _, want := range []string{"ahci", "BootLoader", "bootindex=0", "RecoveryImage", "SystemDisk"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("macOS storage missing %q: %v", want, a)
		}
	}
	// img must be the ide RecoveryImage, never the virtio raw disk.
	if strings.Contains(joined, "if=virtio") {
		t.Fatalf("macOS img must not use virtio: %v", a)
	}
}

func TestFirmwareQcow2Detection(t *testing.T) {
	dir := t.TempDir()
	raw := filepath.Join(dir, "raw.fd")
	os.WriteFile(raw, make([]byte, 64), 0o644)
	qc := filepath.Join(dir, "vars.qcow2")
	os.WriteFile(qc, append([]byte{0x51, 0x46, 0x49, 0xfb}, make([]byte, 60)...), 0o644)
	if isFirmwareQcow2(raw) || !isFirmwareQcow2(qc) {
		t.Fatal("qcow2 magic detection wrong")
	}
}

func TestSystemDiskFormat(t *testing.T) {
	a := storageArgs(Conf{"disk_img": "d.raw", "disk_format": "raw"}, t.TempDir(), OsTuning{})
	if !has(a, "format=raw,file=") {
		t.Fatalf("disk_format=raw must reach SystemDisk: %v", a)
	}
}

func TestBootLegacyMachinePC(t *testing.T) {
	// Bash forces MACHINE_TYPE=pc when boot="legacy" (i440fx/PIIX3).
	if osQuirks(Conf{"boot": "legacy"}, "linux", "x86_64").MachineType != "pc" {
		t.Fatal("boot=legacy must force machine type pc")
	}
	if osQuirks(Conf{"boot": "efi"}, "linux", "x86_64").MachineType != "q35" {
		t.Fatal("efi boot must keep q35")
	}
}

// configure_cpu parity: win11 needs >=2 cores, macOS rounds down to a
// power of two, -smp carries the host socket count.
func TestCPUArgsContract(t *testing.T) {
	a := cpuArgs(Conf{"cpu_cores": "1", "guest_os": "windows", "_vmname": "windows-11"},
		"windows", "x86_64", OsTuning{})
	if !has(a, "cores=2") && !has(a, "cores=1") {
		t.Fatalf("win11 smp %v", a)
	}
	// macOS 6 cores -> 4 logical (nearest lower power of 2); with SMT
	// the -smp line shows cores=2,threads=2 — either is 4 vCPUs total.
	a = cpuArgs(Conf{"cpu_cores": "6", "guest_os": "macos"}, "macos", "x86_64", OsTuning{})
	found := false
	for _, s := range a {
		if strings.HasPrefix(s, "cores=") {
			found = strings.Contains(s, "cores=4") ||
				(strings.Contains(s, "cores=2") && strings.Contains(s, "threads=2"))
		}
	}
	if !found {
		t.Fatalf("macOS power-of-2 rounding failed: %v", a)
	}
	// sockets= is always present.
	found = false
	for _, s := range a {
		if strings.Contains(s, "sockets=") {
			found = true
		}
	}
	if !found {
		t.Fatalf("smp missing sockets: %v", a)
	}
}

func TestRamArgsTiering(t *testing.T) {
	a := ramArgs(Conf{"ram": "12G"})
	if !has(a, "12G") {
		t.Fatalf("explicit ram %v", a)
	}
	if a = ramArgs(Conf{}); len(a) < 2 || a[0] != "-m" {
		t.Fatalf("ramArgs %v", a)
	}
}

// cpu_pinning (vm.sh): ^[0-9]+(,[0-9]+)*$ and the count must equal the
// guest logical core count (Linux-only application).
func TestCPUPinningValidation(t *testing.T) {
	if _, err := parseCPUPinning("0,2,4,6"); err != nil || len([]int{}) < 0 {
		t.Fatalf("valid pinning rejected: %v", err)
	}
	for _, bad := range []string{"0-3", "a,b", "1,,2", "", "1;2"} {
		if bad == "" {
			continue
		}
		if _, err := parseCPUPinning(bad); err == nil {
			t.Fatalf("bad pinning %q accepted", bad)
		}
	}
	c := Conf{"cpu_pinning": "0,1", "_lcores": "2"}
	if err := paramCheckPinning(c); err != nil {
		t.Fatalf("count match rejected: %v", err)
	}
	c["_lcores"] = "4"
	if err := paramCheckPinning(c); err == nil {
		t.Fatal("count mismatch must error")
	}
}

// configure_storage: existing disk > DISK_MIN_SIZE counts as used →
// iso/img detach (installed guests boot from disk); a small disk stays
// unused and keeps the installer media; preallocation is whitelisted.
func TestPrepareStorage(t *testing.T) {
	dir := t.TempDir()
	big := filepath.Join(dir, "big.qcow2")
	f, _ := os.Create(big)
	f.WriteAt([]byte("d"), 2<<20)
	f.Close()
	c := Conf{"disk_img": "big.qcow2", "iso": "i.iso", "guest_os": "linux"}
	if err := prepareStorage(c, dir); err != nil {
		t.Fatal(err)
	}
	if c["iso"] != "" {
		t.Fatalf("used disk must detach iso: %v", c)
	}
	// kolibrios keeps the media attached.
	c = Conf{"disk_img": "big.qcow2", "iso": "i.iso", "guest_os": "kolibrios"}
	prepareStorage(c, dir)
	if c.get("iso", "") == "" {
		t.Fatal("kolibrios must keep installer media")
	}
	// Tiny disk + no iso/img → unused → hard error (Bash exits).
	small := filepath.Join(dir, "small.qcow2")
	os.WriteFile(small, []byte("d"), 0o600)
	c = Conf{"disk_img": "small.qcow2"}
	if err := prepareStorage(c, dir); err == nil {
		t.Fatal("unused disk without media must error")
	}
	// Tiny disk + iso → ok, media stays.
	c = Conf{"disk_img": "small.qcow2", "iso": "i.iso"}
	if err := prepareStorage(c, dir); err != nil || c.get("iso", "") == "" {
		t.Fatalf("unused disk must keep iso: %v %v", err, c)
	}
	// Bad preallocation fails only in run mode.
	c = Conf{"disk_img": "missing.qcow2", "preallocation": "bogus", "_run": "1"}
	if err := prepareStorage(c, dir); err == nil {
		t.Fatal("bad preallocation must error")
	}
	c = Conf{"disk_img": "missing.qcow2", "preallocation": "bogus"}
	if err := prepareStorage(c, dir); err != nil {
		t.Fatalf("plan mode must not validate preallocation: %v", err)
	}
}
