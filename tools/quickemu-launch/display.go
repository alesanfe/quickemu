// display.go: guest display/audio/storage arg building — migrates
// configure_display, configure_audio, configure_storage, configure_os_quirks.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func displayArgs(c Conf, dir, osName, arch string) []string {
	var a []string
	display := c.get("display", "none")
	if display != "none" && display != "spice" {
		a = append(a, "-display", display)
	}
	if c.get("viewer", "") == "none" {
		a = append(a, "-display", "none")
	}
	switch display {
	case "sdl", "gtk", "cocoa":
		a = append(a, "-display", display+",gl=on")
	case "vnc":
		a = append(a, "-vnc", ":0")
	case "none":
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
		res = ",xres=" + c.get("width", "1280") + ",yres=" + c.get("height", "800")
	}
	a = append(a, "-device", dev+res)
	return a
}

// displayDevice: the DISPLAY_DEVICE table from display.sh.
func displayDevice(c Conf, osName, arch, display string) string {
	if v := c.get("video", ""); v != "" {
		return v
	}
	switch {
	case osName == "linux_old" || strings.HasSuffix(osName, "solaris"):
		return "vmware-svga"
	case arch == "aarch64":
		return "virtio-gpu-pci"
	case osName == "macos":
		if display == "none" || display == "spice" {
			return "qxl-vga"
		}
		return "virtio-vga"
	case display == "none" || display == "spice" || display == "spice-app":
		return "virtio-gpu"
	default:
		return "virtio-vga"
	}
}

func audioArgs(c Conf, osName string, t OsTuning) []string {
	sc := c.get("sound", "intel-hda")
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
		a = append(a, "-audiodev", backend, "-device", sc, "-device", duplex+",audiodev=audio0")
	case "usb-audio", "virtio-sound-pci", "ac97", "es1370", "sb16":
		a = append(a, "-audiodev", backend, "-device", sc+",audiodev=audio0")
	default:
		a = append(a, "-audiodev", backend, "-device", sc)
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
	if _, err := os.Stat(filepath.Join(rt, "pipewire-0")); err == nil {
		return "pipewire"
	}
	if _, err := os.Stat(filepath.Join(rt, "pulse/native")); err == nil {
		return "pa"
	}
	return "alsa"
}

func storageArgs(c Conf, dir string, t OsTuning) []string {
	var a []string
	if d := c.get("disk_img", ""); d != "" {
		dev := "virtio-blk-pci"
		if t.MacDiskDev != "" {
			dev = t.MacDiskDev
		}
		// Bash: split -drive (if=none,id=SystemDisk) + -device pattern.
		a = append(a, "-device", dev+",drive=SystemDisk",
			"-drive", "id=SystemDisk,if=none,format=qcow2,file="+filepath.Join(dir, d)+",discard=unmap,detect-zeroes=unmap,cache=writeback,aio=threads")
	}
	if d := c.get("iso", ""); d != "" {
		// Bash: plain cdrom drive (IDE), indexed.
		a = append(a, "-drive", "media=cdrom,index=0,file="+filepath.Join(dir, d))
	}
	if d := c.get("img", ""); d != "" {
		a = append(a, "-drive", "file="+filepath.Join(dir, d)+",format=raw,if=virtio")
	}
	if d := c.get("fixed_iso", ""); d != "" {
		a = append(a, "-drive", "file="+filepath.Join(dir, d)+",media=cdrom")
	}
	if c.get("storage", "") == "nvme" {
		a = append(a, "-device", "nvme,drive=nvme0,serial=nvme0")
	}
	return a
}

// fileshareArgs: fileshare_param_check + configure_file_sharing parity —
// 9p fsdev for the public share dir. public_dir conf key, "none"
// disables; auto-detects xdg-user-dir PUBLICSHARE else ~/Public.
func fileshareArgs(c Conf, dir string) []string {
	pub := c.get("public_dir", "")
	if pub == "none" {
		return nil
	}
	if pub == "" {
		if out, err := exec.Command("xdg-user-dir", "PUBLICSHARE").Output(); err == nil {
			pub = strings.TrimSpace(string(out))
		}
		if pub == "" {
			if h := os.Getenv("HOME"); h != "" {
				if _, err := os.Stat(filepath.Join(h, "Public")); err == nil {
					pub = filepath.Join(h, "Public")
				}
			}
		}
	}
	if pub == "" {
		return nil
	}
	if st, err := os.Stat(pub); err != nil || !st.IsDir() {
		fmt.Fprintf(os.Stderr, " - WARNING! Public directory does not exist: %s\n", pub)
		return nil
	}
	tag := "Public-" + strings.ToLower(os.Getenv("USER"))
	if len(tag) > 31 {
		tag = tag[:31]
	}
	return []string{
		"-fsdev", "local,id=fsdev0,path=" + pub + ",security_model=mapped-xattr",
		"-device", "virtio-9p-pci,fsdev=fsdev0,mount_tag=" + tag,
	}
}
