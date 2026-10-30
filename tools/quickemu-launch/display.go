// display.go: guest display/audio/storage arg building — migrates
// configure_display, configure_audio, configure_storage, configure_os_quirks.
//
// Error text in this file is the Bash user-facing contract: it is
// printed verbatim to stdout, so it keeps its punctuation.
//
//lint:file-ignore ST1005 Bash parity error strings keep punctuation.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

func displayArgs(c Conf, dir, osName, arch string) []string {
	var a []string
	display := c.get("display", "none")
	gl := c.get("gl", "on")
	// DISPLAY_RENDER by mode (display.sh) — also settles the effective
	// gl used by the device swap and the - Display: echo.
	var render string
	switch display {
	case "sdl", "spice-app":
		if display == "sdl" {
			// commands.sh: grab-on-hover fix — QEMU #541.
			os.Setenv("SDL_MOUSE_FOCUS_CLICKTHROUGH", "1")
		}
		render = display + ",gl=" + gl
	case "cocoa":
		// gl=on + check_cocoa_gl_es_support → gl=es; otherwise bare
		// "cocoa" (gl param omitted entirely, gl forced off).
		if gl == "on" && cocoaGLES() {
			gl = "es"
			render = display + ",gl=es"
		} else {
			gl = "off"
			render = display
		}
	case "gtk":
		// GTK needs a reachable display server AND the QEMU gtk
		// backend — headless hosts fall back to SDL.
		if !gtkAvailable() {
			fmt.Println(" - NOTE: GTK display not available, falling back to SDL")
			display = "sdl"
			os.Setenv("SDL_MOUSE_FOCUS_CLICKTHROUGH", "1")
			render = "sdl,gl=" + gl
			break
		}
		render = display + ",grab-on-hover=on,zoom-to-fit=off,gl=" + gl
		if confOn(c, "fullscreen") {
			render += ",full-screen=on"
		}
	case "none":
		render, gl = "none", "off" // no backend → no GL context
	case "spice":
		// Local SPICE + GL: egl-headless provides the VirGL context
		// (gl stays on); remote/local without gl renders none.
		access := c.get("access", "local")
		switch {
		case (access == "" || access == "local") && gl == "on":
			render = "egl-headless,rendernode=/dev/dri/renderD128"
		case access == "" || access == "local":
			render = "none"
		default:
			render, gl = "none", "off"
		}
	case "vnc":
		render, gl = "none", "off"
	default:
		render = display
	}
	// DISPLAY_DEVICE + GL swap (probe-gated) + VirGL echo.
	dev := displayDevice(c, osName, arch, display)
	virgl := "off"
	if gl != "off" && (dev == "virtio-vga" || dev == "virtio-gpu" || dev == "virtio-gpu-pci") {
		virgl = "on"
		gldev := map[string]string{
			"virtio-vga":     "virtio-vga-gl",
			"virtio-gpu":     "virtio-gpu-gl",
			"virtio-gpu-pci": "virtio-gpu-gl-pci",
		}[dev]
		if qemuHasDevice(arch, gldev) {
			dev = gldev
		}
	}
	fmt.Printf(" - Display:  %s, %s, GL (%s), VirGL (%s)", strings.ToUpper(display), dev, gl, virgl)
	// xres/yres lands on the res-capable device set only.
	xres := c.get("width", "1280")
	yres := c.get("height", "800")
	res := ""
	switch {
	case strings.HasPrefix(dev, "virtio-"), dev == "qxl", dev == "qxl-vga", dev == "bochs-display":
		res = ",xres=" + qemuOptEsc(xres) + ",yres=" + qemuOptEsc(yres)
		fmt.Printf(" @ (%s x %s)\n", xres, yres)
	default:
		fmt.Println(" ")
	}
	// VRAM per legacy device family (display.sh).
	vram := ""
	switch dev {
	case "bochs-display":
		vram = ",vgamem=67108864"
	case "qxl", "qxl-vga":
		vram = ",ram_size=65536,vram_size=65536,vgamem_mb=64"
	case "ati-vga", "cirrus-vga", "VGA", "vmware-svga":
		vram = ",vgamem_mb=256"
	}
	if mo := c.get("max_outputs", ""); mo != "" {
		vram += ",max_outputs=" + qemuOptEsc(mo)
	}
	// Emit order (vm.sh): ${VIDEO} -display ${RENDER} — ramfb first,
	// -vga none second, the device, then FULLSCREEN, then -display.
	if arch == "aarch64" && !strings.HasPrefix(dev, "virtio-gpu") && !strings.HasPrefix(dev, "virtio-gpu-pci") {
		a = append(a, "-device", "ramfb")
	}
	if display == "none" || display == "spice" || display == "spice-app" {
		a = append(a, "-vga", "none")
	}
	a = append(a, "-device", dev+res+vram)
	if confOn(c, "fullscreen") {
		a = append(a, "--full-screen")
	}
	a = append(a, "-display", render)
	if c.get("viewer", "") == "none" {
		a = append(a, "-display", "none")
	}
	return a
}

// vncArgs: the -vnc emit — vm_boot appends it in the emit tail, after
// extra_args and before kernel/initrd. "host:display" pair, default
// 127.0.0.1:0 from bin/quickemu.
func vncArgs(c Conf, dir string) []string {
	if c.get("display", "none") != "vnc" {
		return nil
	}
	listen := c.get("vnc_listen", "127.0.0.1:0")
	a := []string{"-vnc", qemuOptEsc(listen)}
	fmt.Printf(" - VNC:      Listening on %s\n", listen)
	// Publish the RFB endpoint: "host:display" → TCP 5900+display.
	host, disp, found := strings.Cut(listen, ":")
	if !found {
		host, disp = "127.0.0.1", listen
	}
	if host == "" {
		host = "127.0.0.1"
	}
	if n, err := strconv.Atoi(disp); err == nil {
		portsRecord(dir, c["_vmname"],
			fmt.Sprintf("vnc,%d,%s", 5900+n, host), c["_run"] == "1")
	}
	return a
}

// displayDevice: the DISPLAY_DEVICE table from display.sh — the raw
// guest/mode pick; the -gl variant swap happens in displayArgs, gated
// on `qemu -device help` actually listing the variant.
func displayDevice(c Conf, osName, arch, display string) string {
	if v := c.get("video", ""); v != "" {
		return v
	}
	switch {
	case strings.HasSuffix(osName, "bsd") || osName == "templeos":
		return "VGA"
	case osName == "linux_old" || osName == "solaris":
		return "vmware-svga"
	case osName == "linux":
		if arch == "aarch64" {
			return "virtio-gpu-pci"
		}
		if display == "none" || display == "spice" || display == "spice-app" {
			return "virtio-gpu"
		}
		return "virtio-vga"
	case osName == "macos":
		// Native VMware display driver; aligns with OSX-KVM.
		return "vmware-svga"
	case osName == "windows" || osName == "windows-server":
		if arch == "aarch64" {
			return "virtio-gpu-pci"
		}
		if display == "none" || display == "spice" {
			return "qxl-vga"
		}
		return "virtio-vga"
	default:
		return "qxl-vga"
	}
}

// qemuDeviceHelp: raw `qemu -device help` output — shared by the GL
// variant and smartcard probes. An unresolvable binary reports "".
func qemuDeviceHelp(arch string) string {
	qemu, err := exec.LookPath("qemu-system-" + arch)
	if err != nil {
		return ""
	}
	out, _ := execCommand(qemu, "-device", "help").CombinedOutput()
	return string(out)
}

// qemuHasDevice: `qemu -device help` probe for a quoted device name —
// gates the virtio -gl variant swap. An unresolvable binary keeps the
// base device (Bash's grep would find nothing the same way).
func qemuHasDevice(arch, name string) bool {
	return strings.Contains(qemuDeviceHelp(arch), `"`+name+`"`)
}

// cocoaGLES mirrors check_cocoa_gl_es_support (macOS-only): probe
// gtkAvailable: display.sh parity — GTK is usable only when a display
// server is reachable AND the QEMU build ships the gtk backend.
func gtkAvailable() bool {
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		return false
	}
	qemu, err := exec.LookPath("qemu-system-x86_64")
	if err != nil {
		return true // no x86 binary to probe — let QEMU decide
	}
	out, _ := execCommand(qemu, "-display", "help", "-M", "none").CombinedOutput()
	for _, l := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(l) == "gtk" {
			return true
		}
	}
	return false
}

// paramCheckDisplay: display_param_check (commands.sh) — --braille
// forces SDL (braille chardev needs it) plus an XHCI controller for
// full-speed braille devices; the display backend is whitelisted per
// platform; unset gl defaults via the glxinfo GLSL probe.
func paramCheckDisplay(c Conf) error {
	if confOn(c, "braille") {
		c["display"] = "sdl"
		c["usb_controller"] = "xhci"
	}
	// sound_card_param_check: USB audio requires an XHCI controller.
	if c.get("sound_card", "") == "usb-audio" {
		c["usb_controller"] = "xhci"
	}
	display := c.get("display", "none")
	if runtime.GOOS == "darwin" {
		if display != "cocoa" && display != "none" {
			return fmt.Errorf("ERROR! Requested output '%s' but only 'cocoa' and 'none' are avalible on macOS.", display)
		}
	} else {
		switch display {
		case "gtk", "none", "sdl", "spice", "spice-app", "vnc":
		default:
			return fmt.Errorf("ERROR! Requested output '%s' is not recognised.", display)
		}
	}
	if c.get("gl", "") == "" {
		c["gl"] = defaultGL()
	}
	return nil
}

// glslGL parses glxinfo output for the "OpenGL ES GLSL" line: GLSL
// 1.x/2.x cannot drive QEMU's GL, anything newer (or absent) keeps it.
func glslGL(out string) string {
	for _, l := range strings.Split(out, "\n") {
		if !strings.Contains(l, "OpenGL ES GLSL") {
			continue
		}
		f := strings.Fields(l)
		if len(f) == 0 {
			break
		}
		v := f[len(f)-1]
		if strings.HasPrefix(v, "1") || strings.HasPrefix(v, "2") {
			return "off"
		}
		return "on"
	}
	return "on"
}

// defaultGL: Bash `command -v glxinfo` gate — absent tool keeps gl on.
func defaultGL() string {
	bin, err := exec.LookPath("glxinfo")
	if err != nil {
		return "on"
	}
	out, _ := execCommand(bin).Output()
	return glslGL(string(out))
}

// brailleArgs: vm.sh — --braille emits -usbdevice braille on Linux when
// the QEMU build ships the braille chardev, else a visible warning.
func brailleArgs(c Conf, arch string) []string {
	if !confOn(c, "braille") || runtime.GOOS != "linux" {
		return nil
	}
	qemu := "qemu-system-" + arch
	if p, err := exec.LookPath(qemu); err == nil {
		qemu = p
	}
	out, _ := execCommand(qemu, "-chardev", "help").CombinedOutput()
	if strings.Contains(string(out), "braille") {
		return []string{"-usbdevice", "braille"}
	}
	fmt.Fprintf(os.Stderr, " - WARNING!  %s does not support -chardev braille \n", qemu)
	return nil
}

// `qemu -display cocoa,gl=es -M none` for OpenGL rejection, then fall
// back to scanning for ANGLE's libEGL.dylib near the QEMU prefix or
// in DYLD_LIBRARY_PATH.
func cocoaGLES() bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	qemu, err := exec.LookPath("qemu-system-x86_64")
	if err != nil {
		return false
	}
	out, _ := execCommand(qemu, "-display", "cocoa,gl=es", "-M", "none").CombinedOutput()
	s := string(out)
	if !strings.Contains(strings.ToLower(s), "opengl support was not enabled") && !strings.Contains(s, "does not accept") {
		return true // QEMU accepted gl=es
	}
	real, _ := filepath.EvalSymlinks(qemu)
	prefix := filepath.Dir(filepath.Dir(real))
	libs := []string{
		prefix + "/lib/libEGL.dylib",
		"/opt/homebrew/lib/libEGL.dylib",
		"/usr/local/lib/libEGL.dylib",
	}
	if dl := os.Getenv("DYLD_LIBRARY_PATH"); dl != "" {
		for _, p := range strings.Split(dl, ":") {
			libs = append(libs, p+"/libEGL.dylib")
		}
	}
	for _, l := range libs {
		if _, err := os.Stat(l); err == nil {
			return true
		}
	}
	return false
}

func audioArgs(c Conf, osName string, t OsTuning) []string {
	sc := c.get("sound_card", "intel-hda")
	if t.SoundCard != "" {
		sc = t.SoundCard
	}
	duplex := c.get("sound_duplex", "hda-micro")
	// configure_audio prints this unconditionally — even for none.
	fmt.Printf(" - Sound:    %s (%s)\n", sc, duplex)
	if sc == "none" {
		return nil
	}
	// AUDIO_DEV by display mode (display.sh): spice for headless/spice,
	// coreaudio for cocoa, else host driver (pipewire>pa>alsa by socket).
	display := c.get("display", "none")
	var backend string
	switch display {
	case "cocoa":
		backend = "coreaudio,id=audio0"
	case "none", "spice", "spice-app":
		backend = "spice,id=audio0"
	default:
		backend = audioDriver(c.get("arch", "x86_64")) + ",id=audio0"
	}
	var a []string
	switch sc {
	case "ich9-intel-hda", "intel-hda":
		a = append(a, "-audiodev", backend, "-device", qemuOptEsc(sc), "-device", qemuOptEsc(duplex)+",audiodev=audio0")
	case "usb-audio", "virtio-sound-pci", "ac97", "es1370", "sb16":
		a = append(a, "-audiodev", backend, "-device", qemuOptEsc(sc)+",audiodev=audio0")
	default:
		a = append(a, "-audiodev", backend, "-device", qemuOptEsc(sc))
	}
	return a
}

// audioDriver: pipewire > pa > alsa, selected by host socket presence —
// same precedence as configure_display (QEMU -audio-help check skipped:
// driver availability varies by build, sockets are the stable signal).
func audioDriver(arch string) string {
	rt := os.Getenv("XDG_RUNTIME_DIR")
	if rt == "" {
		rt = "/run/user/1000"
	}
	// Bash also gates on the QEMU build supporting the driver
	// (-audio-help lists driver=pa/pipewire when compiled in) — and
	// PipeWire needs QEMU >= 8.1, where the backend first shipped.
	qemu := "qemu-system-" + arch
	drvs, _ := execCommand(qemu, "-audio-help").Output()
	supported := string(drvs)
	if v := qemuVersionShort(arch); v < 0 || v >= 81 {
		if _, err := os.Stat(filepath.Join(rt, "pipewire-0")); err == nil &&
			strings.Contains(supported, "driver=pipewire") {
			return "pipewire"
		}
	}
	if _, err := os.Stat(filepath.Join(rt, "pulse/native")); err == nil &&
		strings.Contains(supported, "driver=pa") {
		return "pa"
	}
	return "alsa"
}
