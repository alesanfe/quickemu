// args_display_test — display/audio/input arg-generation tests (split of args2_test.go).
package main

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDisplayAndAudioPaths(t *testing.T) {
	dir := t.TempDir()
	// vnc: -vnc is an emit-tail arg (vncArgs) — it also publishes the
	// RFB endpoint in .ports (run mode).
	c := Conf{"display": "vnc", "vnc_listen": "127.0.0.1:3",
		"_vmdir": dir, "_vmname": "v", "_run": "1"}
	a := vncArgs(c, dir)
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
	if d := audioDriver("x86_64"); d != "pipewire" {
		t.Fatalf("pipewire %q", d)
	}
	os.Remove(filepath.Join(rt, "pipewire-0"))
	os.WriteFile(filepath.Join(rt, "pulse-native"), nil, 0o600)
	os.MkdirAll(filepath.Join(rt, "pulse"), 0o755)
	os.WriteFile(filepath.Join(rt, "pulse", "native"), nil, 0o600)
	if d := audioDriver("x86_64"); d != "pa" {
		t.Fatalf("pa %q", d)
	}
	os.RemoveAll(filepath.Join(rt, "pulse"))
	if d := audioDriver("x86_64"); d != "alsa" {
		t.Fatalf("alsa %q", d)
	}
}

func TestInputAudioNetArgs(t *testing.T) {
	regEnv(t)
	// networkArgs auto-allocates ssh_port over real host TCP state —
	// stub the probe (and the ssh-port exhaustion gate needs it too).
	oldListen := netListen
	netListen = func(_, _ string) (net.Listener, error) {
		return net.Listen("tcp", "127.0.0.1:0")
	}
	defer func() { netListen = oldListen }()
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
	// network=slirp: vm_net_mode maps it to "user" for the ports gate
	// (SSH forward is still planned) but Bash's emit switch hits the
	// non-empty catch-all — a bridge netdev with br=slirp.
	na = networkArgs(Conf{"_vmdir": dir2, "_vmname": "t", "network": "slirp", "ssh_port": "22999"},
		OsTuning{NetDevice: "virtio-net-pci"})
	if !has(na, "bridge,br=slirp") || has(na, "user,hostname=") {
		t.Fatalf("slirp must emit the bridge netdev: %v", na)
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
	_ = ramArgs(Conf{"ram": "8G"}, "linux", "vm.conf")
	_ = ramArgs(Conf{"ram": "512M"}, "linux", "vm.conf")
	_ = ramArgs(Conf{}, "linux", "vm.conf")
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

// DISPLAY_DEVICE table parity (display.sh).
func TestDisplayDeviceMatrix(t *testing.T) {
	cases := []struct{ os_, arch, disp, want string }{
		{"freebsd", "x86_64", "gtk", "VGA"},
		{"openbsd", "x86_64", "none", "VGA"},
		{"templeos", "x86_64", "sdl", "VGA"},
		{"linux_old", "x86_64", "gtk", "vmware-svga"},
		{"solaris", "x86_64", "gtk", "vmware-svga"},
		{"linux", "aarch64", "gtk", "virtio-gpu-pci"},
		{"linux", "x86_64", "none", "virtio-gpu"},
		{"linux", "x86_64", "spice", "virtio-gpu"},
		{"linux", "x86_64", "spice-app", "virtio-gpu"},
		{"linux", "x86_64", "gtk", "virtio-vga"},
		{"macos", "x86_64", "none", "vmware-svga"},
		{"macos", "x86_64", "sdl", "vmware-svga"},
		{"windows", "aarch64", "sdl", "virtio-gpu-pci"},
		{"windows", "x86_64", "spice", "qxl-vga"},
		{"windows", "x86_64", "none", "qxl-vga"},
		{"windows", "x86_64", "vnc", "virtio-vga"},
		{"windows-server", "x86_64", "gtk", "virtio-vga"},
		{"reactos", "x86_64", "gtk", "qxl-vga"},
	}
	for _, tc := range cases {
		if got := displayDevice(Conf{}, tc.os_, tc.arch, tc.disp); got != tc.want {
			t.Errorf("%s/%s/%s: got %s, want %s", tc.os_, tc.arch, tc.disp, got, tc.want)
		}
	}
	// conf override always wins.
	if got := displayDevice(Conf{"video": "ati-vga"}, "linux", "x86_64", "gtk"); got != "ati-vga" {
		t.Fatalf("conf video override lost: %s", got)
	}
}

// GL device swap is gated on  listing the variant.
func TestDisplayGLDeviceProbe(t *testing.T) {
	stub := t.TempDir()
	os.WriteFile(filepath.Join(stub, "qemu-system-x86_64"),
		[]byte("#!/bin/sh\necho 'name \"virtio-vga-gl\", bus PCI'\n"), 0o755)
	t.Setenv("PATH", stub+string(os.PathListSeparator)+os.Getenv("PATH"))
	a := displayArgs(Conf{"display": "sdl", "gl": "on"}, t.TempDir(), "linux", "x86_64")
	if !has(a, "virtio-vga-gl") {
		t.Fatalf("supported gl variant must be used: %v", a)
	}
	os.WriteFile(filepath.Join(stub, "qemu-system-x86_64"),
		[]byte("#!/bin/sh\necho 'name \"virtio-vga\", bus PCI'\n"), 0o755)
	a = displayArgs(Conf{"display": "sdl", "gl": "on"}, t.TempDir(), "linux", "x86_64")
	if !has(a, "virtio-vga") || has(a, "virtio-vga-gl") {
		t.Fatalf("unsupported gl variant must keep base device: %v", a)
	}
	// gl=off never swaps even when supported.
	os.WriteFile(filepath.Join(stub, "qemu-system-x86_64"),
		[]byte("#!/bin/sh\necho 'name \"virtio-vga-gl\", bus PCI'\n"), 0o755)
	a = displayArgs(Conf{"display": "sdl", "gl": "off"}, t.TempDir(), "linux", "x86_64")
	if has(a, "virtio-vga-gl") {
		t.Fatalf("gl=off must not swap device: %v", a)
	}
}

// xres/yres whitelist, VRAM per family, max_outputs, -vga none, ramfb.
func TestDisplayVideoOptions(t *testing.T) {
	dir := t.TempDir()
	// qxl-vga (windows+spice): res-capable + qxl vram + -vga none.
	a := displayArgs(Conf{"display": "spice", "access": "remote"}, dir, "windows", "x86_64")
	j := strings.Join(a, " ")
	for _, want := range []string{"qxl-vga,xres=1280,yres=800",
		"ram_size=65536", "vram_size=65536", "vgamem_mb=64"} {
		if !strings.Contains(j, want) {
			t.Fatalf("spice/qxl missing %q: %v", want, a)
		}
	}

	// spice-app gets -vga none too; vnc does NOT.
	if !func() bool {
		for i, arg := range a[:len(a)-1] {
			if arg == "-vga" && a[i+1] == "none" {
				return true
			}
		}
		return false
	}() {
		t.Fatalf("spice must emit -vga none: %v", a)
	}
	vgaNone := func(args []string) bool {
		for i, arg := range args[:len(args)-1] {
			if arg == "-vga" && args[i+1] == "none" {
				return true
			}
		}
		return false
	}
	a = displayArgs(Conf{"display": "spice-app"}, dir, "linux", "x86_64")
	if !vgaNone(a) {
		t.Fatalf("spice-app must emit -vga none: %v", a)
	}
	a = displayArgs(Conf{"display": "vnc"}, dir, "linux", "x86_64")
	if vgaNone(a) {
		t.Fatalf("vnc must not emit -vga none: %v", a)
	}
	// VGA family: vgamem_mb=256, no xres.
	a = displayArgs(Conf{"display": "none"}, dir, "freebsd", "x86_64")
	if !has(a, "vgamem_mb=256") || strings.Contains(strings.Join(a, " "), "xres") {
		t.Fatalf("VGA device: %v", a)
	}
	// bochs-display: 64M vgamem + res-capable.
	a = displayArgs(Conf{"display": "sdl", "video": "bochs-display"}, dir, "linux", "x86_64")
	if !has(a, "vgamem=67108864") || !has(a, "xres=1280") {
		t.Fatalf("bochs: %v", a)
	}
	// max_outputs appends to the device opts.
	a = displayArgs(Conf{"display": "none", "max_outputs": "2"}, dir, "linux", "x86_64")
	if !has(a, "max_outputs=2") {
		t.Fatalf("max_outputs: %v", a)
	}
	// fullscreen → --full-screen arg after the device.
	a = displayArgs(Conf{"display": "sdl", "fullscreen": "true"}, dir, "linux", "x86_64")
	exact := false
	for _, arg := range a {
		if arg == "--full-screen" {
			exact = true
		}
	}
	if !exact {
		t.Fatalf("fullscreen must emit --full-screen arg: %v", a)
	}
	// aarch64 non-virtio-gpu gets ramfb first.
	a = displayArgs(Conf{"display": "none"}, dir, "freebsd", "aarch64")
	if len(a) < 2 || a[0] != "-device" || a[1] != "ramfb" {
		t.Fatalf("aarch64 non-gpu device must prepend ramfb: %v", a)
	}
	a = displayArgs(Conf{"display": "none"}, dir, "linux", "aarch64")
	if has(a, "ramfb") {
		t.Fatalf("virtio-gpu-pci must not emit ramfb: %v", a)
	}
}

// SPICE render selection (display.sh).
func TestDisplaySpiceRender(t *testing.T) {
	dir := t.TempDir()
	a := displayArgs(Conf{"display": "spice", "access": "local", "gl": "on"}, dir, "linux", "x86_64")
	if !has(a, "egl-headless,rendernode=/dev/dri/renderD128") {
		t.Fatalf("local spice+gl must use egl-headless: %v", a)
	}
	a = displayArgs(Conf{"display": "spice", "access": "local", "gl": "off"}, dir, "linux", "x86_64")
	if !has(a, "none") || has(a, "egl-headless") {
		t.Fatalf("local spice without gl renders none: %v", a)
	}
	a = displayArgs(Conf{"display": "spice", "access": "remote", "gl": "on"}, dir, "linux", "x86_64")
	if has(a, "egl-headless") {
		t.Fatalf("remote spice must not use egl-headless: %v", a)
	}
}
