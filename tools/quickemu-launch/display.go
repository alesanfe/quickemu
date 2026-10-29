// display.go: guest display/audio/storage arg building — migrates
// configure_display, configure_audio, configure_storage, configure_os_quirks.
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
	switch display {
	case "sdl", "spice-app":
		if display == "sdl" {
			// commands.sh: grab-on-hover fix — QEMU #541.
			os.Setenv("SDL_MOUSE_FOCUS_CLICKTHROUGH", "1")
		}
		a = append(a, "-display", display+",gl="+qemuOptEsc(gl))
	case "cocoa":
		// display.sh: gl=on + check_cocoa_gl_es_support → gl=es;
		// otherwise bare "cocoa" (gl param omitted entirely).
		if gl == "on" && cocoaGLES() {
			gl = "es"
			a = append(a, "-display", display+",gl=es")
		} else {
			a = append(a, "-display", display)
		}
	case "gtk":
		// display.sh: GTK needs a reachable display server AND QEMU
		// gtk support — headless hosts fall back to SDL.
		if !gtkAvailable() {
			fmt.Println(" - NOTE: GTK display not available, falling back to SDL")
			os.Setenv("SDL_MOUSE_FOCUS_CLICKTHROUGH", "1")
			a = append(a, "-display", "sdl,gl="+qemuOptEsc(gl))
			break
		}
		render := display + ",grab-on-hover=on,zoom-to-fit=off,gl=" + qemuOptEsc(gl)
		if c.get("fullscreen", "") == "true" {
			render += ",full-screen=on"
		}
		a = append(a, "-display", render)
	case "vnc":
		// vm.sh: args+=(-vnc "${vnc_listen}") — host:display pair,
		// default 127.0.0.1:0 from bin/quickemu.
		listen := c.get("vnc_listen", "127.0.0.1:0")
		a = append(a, "-vnc", qemuOptEsc(listen), "-display", "none")
		// Publish the RFB endpoint so frontends can attach a viewer:
		// "host:display" maps onto TCP 5900+display.
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
	case "none":
		a = append(a, "-display", "none")
	case "spice":
		a = append(a, "-display", "none") // egl-headless when gl=on; none is safe default
	default:
		a = append(a, "-display", display)
	}
	if c.get("viewer", "") == "none" {
		a = append(a, "-display", "none")
	}
	// DISPLAY_DEVICE resolution (display.sh): per guest_os + mode.
	dev := displayDevice(c, osName, arch, display)
	if display == "none" {
		a = append(a, "-vga", "none")
	}
	// Bash appends xres/yres to virtio-gpu devices.
	res := ""
	if strings.HasPrefix(dev, "virtio-") {
		res = ",xres=" + qemuOptEsc(c.get("width", "1280")) + ",yres=" + qemuOptEsc(c.get("height", "800"))
	}
	a = append(a, "-device", dev+res)
	return a
}

// displayDevice: the DISPLAY_DEVICE table from display.sh — includes
// the GL variant swap when the display backend supports it and gl!=off.
func displayDevice(c Conf, osName, arch, display string) string {
	if v := c.get("video", ""); v != "" {
		return v
	}
	var dev string
	switch {
	case osName == "linux_old" || strings.HasSuffix(osName, "solaris"):
		dev = "vmware-svga"
	case arch == "aarch64":
		dev = "virtio-gpu-pci"
	case osName == "macos":
		if display == "none" || display == "spice" {
			dev = "qxl-vga"
		} else {
			dev = "virtio-vga"
		}
	case display == "none" || display == "spice" || display == "spice-app":
		dev = "virtio-gpu"
	default:
		dev = "virtio-vga"
	}
	// GL swap: sdl/gtk/cocoa displays with gl!=off use the -gl variant
	// (display.sh: virtio-vga->virtio-vga-gl, gpu->gpu-gl, gpu-pci->gpu-gl-pci).
	gl := c.get("gl", "on")
	if gl != "off" && (display == "sdl" || display == "gtk" || display == "cocoa" || display == "spice-app") {
		switch dev {
		case "virtio-vga":
			dev = "virtio-vga-gl"
		case "virtio-gpu":
			dev = "virtio-gpu-gl"
		case "virtio-gpu-pci":
			dev = "virtio-gpu-gl-pci"
		}
	}
	return dev
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
		backend = audioDriver() + ",id=audio0"
	}
	duplex := c.get("sound_duplex", "hda-micro")
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
func audioDriver() string {
	rt := os.Getenv("XDG_RUNTIME_DIR")
	if rt == "" {
		rt = "/run/user/1000"
	}
	// Bash also gates on the QEMU build supporting the driver
	// (-audio-help lists driver=pa/pipewire when compiled in).
	drvs, _ := execCommand("qemu-system-x86_64", "-audio-help").Output()
	supported := string(drvs)
	if _, err := os.Stat(filepath.Join(rt, "pipewire-0")); err == nil &&
		strings.Contains(supported, "driver=pipewire") {
		return "pipewire"
	}
	if _, err := os.Stat(filepath.Join(rt, "pulse/native")); err == nil &&
		strings.Contains(supported, "driver=pa") {
		return "pa"
	}
	return "alsa"
}

func storageArgs(c Conf, dir string, t OsTuning) []string {
	var a []string
	mac := c.get("guest_os", "") == "macos"
	dfmt := c.get("disk_format", "qcow2")
	if mac {
		// vm.sh macOS emit: AHCI bus + OpenCore/ESP bootloader at
		// bootindex 0, RecoveryImage on ahci.1, SystemDisk last.
		vmdir := c["_vmdir"]
		bl := filepath.Join(vmdir, "OpenCore.qcow2")
		if _, err := os.Stat(bl); err != nil {
			bl = filepath.Join(vmdir, "ESP.qcow2")
		}
		a = append(a, "-device", "ahci,id=ahci",
			"-device", "ide-hd,bus=ahci.0,drive=BootLoader,bootindex=0",
			"-drive", "id=BootLoader,if=none,format=qcow2,file="+qemuOptEsc(bl))
		if d := c.get("img", ""); d != "" {
			a = append(a, "-device", "ide-hd,bus=ahci.1,drive=RecoveryImage",
				"-drive", "id=RecoveryImage,if=none,format=raw,file="+qemuOptEsc(joinConf(dir, d)))
		}
	}
	if d := c.get("disk_img", ""); d != "" {
		dev := "virtio-blk-pci"
		if t.MacDiskDev != "" {
			dev = t.MacDiskDev
		}
		// Bash: split -drive (if=none,id=SystemDisk) + -device pattern.
		a = append(a, "-device", dev+",drive=SystemDisk",
			"-drive", "id=SystemDisk,if=none,format="+dfmt+",file="+qemuOptEsc(joinConf(dir, d))+",discard=unmap,detect-zeroes=unmap,cache=writeback,aio=threads")
	}
	if d := c.get("iso", ""); d != "" {
		// Bash: plain cdrom drive (IDE), indexed.
		a = append(a, "-drive", "media=cdrom,index=0,file="+qemuOptEsc(joinConf(dir, d)))
	}
	if d := c.get("img", ""); d != "" && !mac {
		a = append(a, "-drive", "file="+qemuOptEsc(joinConf(dir, d))+",format=raw,if=virtio")
	}
	if d := c.get("fixed_iso", ""); d != "" {
		a = append(a, "-drive", "file="+qemuOptEsc(joinConf(dir, d))+",media=cdrom")
	}
	if c.get("storage", "") == "nvme" {
		a = append(a, "-device", "nvme,drive=nvme0,serial=nvme0")
	}
	return a
}

// resolvePublic: fileshare_param_check parity — resolves PUBLIC early
// (called before arg building; the user netdev reads it for ,smb=).
func resolvePublic(c Conf) {
	pub := c.get("public_dir", "")
	if pub == "none" {
		return
	}
	if pub == "" {
		pub = xdgUserDir("PUBLICSHARE")
		if pub == "" {
			if h := os.Getenv("HOME"); h != "" {
				if _, err := os.Stat(filepath.Join(h, "Public")); err == nil {
					pub = filepath.Join(h, "Public")
				}
			}
		}
	}
	if pub == "" {
		return
	}
	if st, err := os.Stat(pub); err != nil || !st.IsDir() {
		fmt.Fprintf(os.Stderr, " - WARNING! Public directory does not exist: %s\n", pub)
		return
	}
	c["_public"] = pub
}

func fileshareArgs(c Conf, dir string) []string {
	// configure_file_sharing: no 9p for windows (Samba via netdev
	// instead); windows-server keeps 9p.
	if c.get("guest_os", "") == "windows" {
		return nil
	}
	pub := c["_public"]
	if pub == "" {
		return nil
	}
	tag := "Public-" + strings.ToLower(os.Getenv("USER"))
	if len(tag) > 31 {
		tag = tag[:31]
	}
	// virtiofs preferred over 9p (vm_boot_fileshare): _virtiofsd is set
	// in main when the guest is linux, no ISO is attached, and the
	// daemon is running (--run) or merely resolvable (plan mode).
	if c["_virtiofsd"] == "1" {
		sock := dotdir(c["_vmdir"]) + c["_vmname"] + ".virtiofsd-sock"
		return []string{
			"-object", "memory-backend-file,id=mem,size=" + qemuOptEsc(c.get("ram", "4G")) + ",mem-path=/dev/shm,share=on",
			"-numa", "node,memdev=mem",
			"-chardev", "socket,id=char0,path=" + qemuOptEsc(sock),
			"-device", "vhost-user-fs-pci,queue-size=1024,chardev=char0,tag=" + qemuOptEsc(tag),
		}
	}
	return []string{
		"-fsdev", "local,id=fsdev0,path=" + qemuOptEsc(pub) + ",security_model=mapped-xattr",
		"-device", "virtio-9p-pci,fsdev=fsdev0,mount_tag=" + qemuOptEsc(tag),
	}
}
