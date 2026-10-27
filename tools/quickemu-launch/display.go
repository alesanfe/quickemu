// display.go: guest display/audio/storage arg building — migrates
// configure_display, configure_audio, configure_storage, configure_os_quirks.
package main

import (
	"fmt"
	"path/filepath"
)

func displayArgs(c Conf, dir, osName, arch string) []string {
	var a []string
	display := c.get("display", "none")
	video := c.get("video", "virtio-gpu")
	if display != "none" && display != "spice" {
		a = append(a, "-display", display)
	}
	if c.get("spice_port", "") != "" {
		a = append(a, "-spice", fmt.Sprintf("port=%s,disable-ticketing=on", c["spice_port"]))
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
	// video device per guest_os
	switch osName {
	case "windows":
		a = append(a, "-device", "virtio-gpu-gl" /* windows uses virtio-gpu */)
	case "macos":
		a = append(a, "-device", "virtio-gpu-gl", "-global", "ICH9-LPC.disable_s3=1", "-global", "ICH9-LPC.acpi-pci-hotplug-with-bridge-support=off")
	default:
		a = append(a, "-device", video)
	}
	return a
}

func audioArgs(c Conf, osName string, t OsTuning) []string {
	sc := c.get("sound", "intel-hda")
	if t.SoundCard != "" {
		sc = t.SoundCard
	}
	duplex := "hda-duplex"
	switch osName {
	case "windows":
		sc = "ich9-intel-hda"
	case "macos":
		sc = "virtio-sound-pci"
	case "linux":
		sc = "intel-hda"
	}
	switch sc {
	case "none":
		return nil
	case "ich9-intel-hda", "intel-hda":
		return []string{"-device", sc + " -device " + duplex + ",audiodev=audio0", "-audiodev", "pa,id=audio0,server=unix:/tmp/pulse-socket"}
	case "usb-audio":
		return []string{"-device", "usb-audio,audiodev=audio0", "-audiodev", "pa,id=audio0,server=unix:/tmp/pulse-socket"}
	case "virtio-sound-pci":
		return []string{"-device", "virtio-sound-pci,audiodev=audio0", "-audiodev", "pa,id=audio0,server=unix:/tmp/pulse-socket"}
	default:
		return []string{"-device", sc}
	}
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
