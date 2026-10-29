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
	if display == "vnc" {
		// args+=(-vnc "${vnc_listen}") in the emit tail — host:display
		// pair, default 127.0.0.1:0 from bin/quickemu.
		listen := c.get("vnc_listen", "127.0.0.1:0")
		a = append(a, "-vnc", qemuOptEsc(listen))
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
	}
	if c.get("viewer", "") == "none" {
		a = append(a, "-display", "none")
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

// prepareStorage: configure_storage — create a missing disk_img via
// qemu-img (preallocation whitelist + per-format create options),
// write-lock probe on existing images, and the DISK_USED heuristic:
// an effectively-empty disk keeps booting the installer media while a
// used one drops iso/img so the guest boots from disk.
func prepareStorage(c Conf, dir string) error {
	d := c.get("disk_img", "")
	if d == "" {
		return nil
	}
	p := joinConf(dir, d)
	dfmt := c.get("disk_format", "qcow2")
	size := c.get("disk_size", "16G")
	pre := c.get("preallocation", "off")
	if _, err := os.Stat(p); err != nil {
		if c["_run"] != "1" {
			fmt.Printf("             Would create %s (%s, %s)\n", p, dfmt, size)
			return nil
		}
		os.MkdirAll(filepath.Dir(p), 0o755)
		switch pre {
		case "off", "metadata", "falloc", "full":
		default:
			return fmt.Errorf("ERROR! %s is an unsupported disk preallocation option.", pre)
		}
		opts := ""
		switch dfmt {
		case "qcow2":
			opts = "lazy_refcounts=on,preallocation=" + pre + ",nocow=on"
		case "raw":
			opts = "preallocation=" + pre
		}
		qi, err := exec.LookPath("qemu-img")
		if err != nil {
			return fmt.Errorf("ERROR! qemu-img not found — cannot create %s", p)
		}
		argv := []string{"create", "-q", "-f", dfmt, "-o", opts, p, size}
		if err := exec.Command(qi, argv...).Run(); err != nil {
			return fmt.Errorf("ERROR! Failed to create %s using %s format.", p, dfmt)
		}
		if c.get("iso", "") == "" && c.get("img", "") == "" {
			return fmt.Errorf("ERROR! You haven't specified a .iso or .img image to boot from.")
		}
		fmt.Printf("             Just created, booting from %s%s\n",
			c.get("iso", ""), c.get("img", ""))
		return nil // DISK_USED=no — installer media stays attached.
	}
	// Existing image: the launch path implies the VM is stopped, so
	// probe the write lock (qemu-img info) — a held lock means another
	// process has the disk open.
	qi, err := exec.LookPath("qemu-img")
	if err != nil {
		return fmt.Errorf("ERROR! qemu-img not found — cannot inspect %s", p)
	}
	if err := exec.Command(qi, "info", p).Run(); err != nil {
		return fmt.Errorf("             Failed to get \"write\" lock. Is another process using the disk?")
	}
	if pre == "off" {
		used := true
		// A qcow2 overlay is physically small but fully populated via
		// its backing file — treat as used regardless of size.
		out, _ := exec.Command(qi, "info", "--output=json", p).Output()
		if !strings.Contains(string(out), `"backing-filename"`) {
			if fi, err := os.Stat(p); err == nil && fi.Size() <= 197632*8 {
				used = false
			}
		}
		if !used {
			fmt.Printf("             Looks unused, booting from %s%s\n",
				c.get("iso", ""), c.get("img", ""))
			if c.get("iso", "") == "" && c.get("img", "") == "" {
				return fmt.Errorf("ERROR! You haven't specified a .iso or .img image to boot from.")
			}
			return nil
		}
	}
	// DISK_USED=yes: a used disk means the OS is installed — detach
	// installer media so the guest boots from disk (kolibrios keeps
	// its floppy/iso attached on every boot).
	if c.get("guest_os", "") != "kolibrios" {
		c["iso"] = ""
		c["img"] = ""
	}
	// STATUS_QUO: -snapshot means writes are ephemeral — announce it
	// once the installer media has settled.
	if confOn(c, "snapshot") && c.get("iso", "") == "" && c.get("img", "") == "" {
		fmt.Printf("             Existing disk state will be preserved, no writes will be committed.\n")
	}
	return nil
}

func storageArgs(c Conf, dir string, t OsTuning) []string {
	var a []string
	osn := c.get("guest_os", "")
	mac := osn == "macos"
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
		fmt.Printf(" - Disk:     %s (%s)\n", joinConf(dir, d), c.get("disk_size", "16G"))
		dev := "virtio-blk-pci"
		if t.MacDiskDev != "" {
			dev = t.MacDiskDev
		}
		sys := "-drive id=SystemDisk,if=none,format=" + dfmt + ",file=" +
			qemuOptEsc(joinConf(dir, d)) + ",discard=unmap,detect-zeroes=unmap,cache=writeback,aio=threads"
		// Per-guest SystemDisk emit (vm.sh): ahci/ide variants, then
		// the virtio default; aarch64 gets bootindex=2 so the installer
		// CD-ROM (bootindex=1) wins during installation.
		switch {
		case mac:
			a = append(a, "-device", dev+",drive=SystemDisk", sys)
		case osn == "kolibrios":
			a = append(a, "-device", "ahci,id=ahci",
				"-device", "ide-hd,bus=ahci.0,drive=SystemDisk", sys)
		case osn == "batocera":
			if img := c.get("img", ""); img != "" {
				a = append(a, "-device", "virtio-blk-pci,drive=BootDisk",
					"-drive", "id=BootDisk,if=none,format=raw,file="+qemuOptEsc(joinConf(dir, img)))
			}
			a = append(a, "-device", "virtio-blk-pci,drive=SystemDisk", sys)
		case osn == "reactos" || osn == "templeos":
			// ATA/IDE only — no -device, a plain if=ide disk.
			a = append(a, "-drive", "if=ide,index=0,media=disk,file="+qemuOptEsc(joinConf(dir, d)))
		case osn == "windows-server":
			a = append(a, "-device", "ide-hd,drive=SystemDisk", sys)
		case c.get("arch", "x86_64") == "aarch64":
			a = append(a, "-device", "virtio-blk-pci,drive=SystemDisk,bootindex=2", sys)
		default:
			a = append(a, "-device", dev+",drive=SystemDisk", sys)
		}
	}
	// vm_boot_media emits kolibrios/reactos ISOs at index=2 (phantom-
	// iso workaround); the generic emit must skip them.
	specialISO := osn == "kolibrios" || osn == "reactos"
	a64 := c.get("arch", "x86_64") == "aarch64"
	if a64 && (c.get("iso", "") != "" || c.get("fixed_iso", "") != "") {
		// virt machine has no IDE — CD-ROMs hang off virtio-scsi.
		a = append(a, "-device", "virtio-scsi-pci,id=scsi0")
	}
	if d := c.get("iso", ""); d != "" && !specialISO {
		if a64 {
			// bootindex=1: UEFI boots the installer before the disk.
			a = append(a, "-device", "scsi-cd,drive=cd0,bus=scsi0.0,bootindex=1",
				"-drive", "id=cd0,if=none,format=raw,media=cdrom,readonly=on,file="+qemuOptEsc(joinConf(dir, d)))
		} else {
			// Bash: plain cdrom drive (IDE), indexed.
			a = append(a, "-drive", "media=cdrom,index=0,file="+qemuOptEsc(joinConf(dir, d)))
		}
		if _, err := os.Stat(joinConf(dir, d)); err == nil {
			fmt.Printf(" - Boot ISO: %s\n", joinConf(dir, d))
		}
	}
	// img only becomes a drive for macOS (RecoveryImage, above) and
	// batocera (BootDisk, above) — for everyone else it is a marker
	// Bash only echoes, and only when there is no boot ISO (elif).
	if d := c.get("img", ""); d != "" && !mac && c.get("iso", "") == "" {
		if _, err := os.Stat(joinConf(dir, d)); err == nil {
			fmt.Printf(" - Recovery: %s\n", joinConf(dir, d))
		}
	}
	if d := c.get("fixed_iso", ""); d != "" {
		if a64 {
			a = append(a, "-device", "scsi-cd,drive=cd1,bus=scsi0.0,bootindex=3",
				"-drive", "id=cd1,if=none,format=raw,media=cdrom,readonly=on,file="+qemuOptEsc(joinConf(dir, d)))
		} else {
			a = append(a, "-drive", "media=cdrom,index=1,file="+qemuOptEsc(joinConf(dir, d)))
		}
		if _, err := os.Stat(joinConf(dir, d)); err == nil {
			fmt.Printf(" - CD-ROM:   %s\n", joinConf(dir, d))
		}
	}
	// extra_disks: optional :format suffix (default qcow2); missing
	// files warn and skip, never fatal.
	for i, ed := range confList(c, "extra_disks") {
		if ed == "" {
			continue
		}
		edu, ef := ed, "qcow2"
		if j := strings.LastIndex(ed, ":"); j >= 0 {
			edu, ef = ed[:j], ed[j+1:]
		}
		ep := joinConf(dir, edu)
		if _, err := os.Stat(ep); err != nil {
			fmt.Printf(" - WARNING! extra_disks[%d] '%s' not found; skipping.\n", i, edu)
			continue
		}
		id := fmt.Sprintf("ExtraDisk%d", i)
		a = append(a, "-device", "virtio-blk-pci,drive="+id,
			"-drive", "id="+id+",if=none,format="+ef+",file="+qemuOptEsc(ep)+
				",discard=unmap,detect-zeroes=unmap,cache=writeback,aio=threads")
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
	osn := c.get("guest_os", "")
	pub := c["_public"]
	// configure_file_sharing echoes run whenever PUBLIC resolves —
	// even for windows guests (WebDAV hint still prints there).
	if pub != "" {
		switch osn {
		case "macos":
			if d := c.get("display", "none"); d == "none" || d == "spice" || d == "spice-app" {
				fmt.Printf(" - WebDAV:   On guest: build spice-webdavd (https://gitlab.gnome.org/GNOME/phodav/-/merge_requests/24)\n")
				fmt.Printf(" - WebDAV:   On guest: Finder -> Connect to Server -> http://localhost:9843/\n")
			}
		default:
			fmt.Printf(" - WebDAV:   On guest: dav://localhost:9843/\n")
		}
	}
	if pub == "" {
		return nil
	}
	// no 9p for windows (Samba via netdev instead); windows-server keeps it.
	var a []string
	if osn != "windows" {
		tag := "Public-" + strings.ToLower(os.Getenv("USER"))
		if len(tag) > 31 {
			tag = tag[:31]
		}
		// virtiofs preferred over 9p (vm_boot_fileshare): _virtiofsd is
		// set in main when the guest is linux, no ISO is attached, and
		// the daemon is running (--run) or merely resolvable (plan mode).
		if c["_virtiofsd"] == "1" && qemuHasVhostFS(c) {
			sock := dotdir(c["_vmdir"]) + c["_vmname"] + ".virtiofsd-sock"
			fmt.Printf(" - virtiofs: On guest: sudo mount -t virtiofs %s ~/%s\n", tag, filepath.Base(pub))
			a = []string{
				"-object", "memory-backend-file,id=mem,size=" + qemuOptEsc(c.get("ram", "4G")) + ",mem-path=/dev/shm,share=on",
				"-numa", "node,memdev=mem",
				"-chardev", "socket,id=char0,path=" + qemuOptEsc(sock),
				"-device", "vhost-user-fs-pci,queue-size=1024,chardev=char0,tag=" + qemuOptEsc(tag),
			}
		} else {
			if c["_virtiofsd"] == "1" {
				// QEMU lacks the device — warn, stop the daemon, fall to 9p.
				fmt.Printf(" - WARNING! QEMU does not support vhost-user-fs-pci; falling back to 9p.\n")
				stopDaemon(c["_vmdir"], c["_vmname"], "virtiofsd")
				c["_virtiofsd"] = ""
			}
			switch osn {
			case "linux":
				fmt.Printf(" - 9P:       On guest: sudo mount -t 9p -o trans=virtio,version=9p2000.L,msize=512000 %s ~/%s\n", tag, filepath.Base(pub))
			case "macos":
				fmt.Printf(" - 9P:       On guest: sudo mount_9p %s\n", tag)
				if fi, err := os.Stat(pub); err == nil && fi.Mode().Perm() != 0o777 {
					fmt.Printf(" - 9P:       On host:  chmod 777 %s\n", pub)
					fmt.Printf("             Required for macOS integration 👆\n")
				}
			}
			a = []string{
				"-fsdev", "local,id=fsdev0,path=" + qemuOptEsc(pub) + ",security_model=mapped-xattr",
				"-device", "virtio-9p-pci,fsdev=fsdev0,mount_tag=" + qemuOptEsc(tag),
			}
		}
	}
	// configure_file_sharing's smbd echo runs last for every guest —
	// the ,smb= option itself is already on the user netdev.
	if smbdPresent() {
		fmt.Printf(" - smbd:     On guest: smb://10.0.2.4/qemu\n")
	}
	return a
}

// qemuHasVhostFS: vm_boot_fileshare parity — probe `qemu -device help`
// for the quoted device name; an unresolvable binary keeps the
// virtiofs decision (QEMU will report its own error at spawn).
func qemuHasVhostFS(c Conf) bool {
	qemu, err := exec.LookPath("qemu-system-" + c.get("arch", "x86_64"))
	if err != nil {
		return true
	}
	out, _ := execCommand(qemu, "-device", "help").CombinedOutput()
	return strings.Contains(string(out), `"vhost-user-fs-pci"`)
}
