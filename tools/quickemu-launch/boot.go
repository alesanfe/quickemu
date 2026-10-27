// boot.go: boot-variant arg builders — vm_boot_media, vm_boot_fileshare,
// vm_boot_passthru, vm_boot_channels from vm.sh.
package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

func bootMediaArgs(c Conf, dir string) []string {
	var a []string
	switch c.get("boot", "") {
	case "efi":
		a = append(a, "-boot", "order=d,once=d")
	case "legacy":
		a = append(a, "-boot", "order=c,once=c")
	case "network":
		a = append(a, "-boot", "order=n,once=n")
	}
	if c.get("floppy", "") != "" {
		a = append(a, "-drive", "file="+filepath.Join(dir, c["floppy"])+",if=floppy")
	}
	if c.get("driver_iso", "") != "" {
		a = append(a, "-drive", "file="+filepath.Join(dir, c["driver_iso"])+",media=cdrom,if=none,id=driver", "-device", "virtio-cdrom,drive=driver")
	}
	return a
}

func passthruArgs(c Conf) []string {
	var a []string
	for _, d := range strings.Fields(c.get("usb_devices", "")) {
		parts := strings.Split(d, ":")
		if len(parts) == 2 {
			a = append(a, "-device", fmt.Sprintf("usb-host,vendorid=0x%s,productid=0x%s", parts[0], parts[1]))
		}
	}
	if c.get("vfio", "") == "true" {
		for _, dev := range strings.Fields(c.get("vfio_devices", "")) {
			a = append(a, "-device", "vfio-pci,host="+dev)
		}
	}
	return a
}

func channelArgs(c Conf, dir string) []string {
	var a []string
	if c.get("agent", "") == "true" || c.get("guest_agent", "") == "true" {
		a = append(a, "-chardev", "qemu-vdagent,used=on,id=qga0,name=vdagent,clipboard=on")
		a = append(a, "-device", "virtio-serial-pci,id=virtio-serial0")
		a = append(a, "-chardev", "spicevmc,name=vdagent,id=vdagent")
		a = append(a, "-device", "virtserialport,chardev=vdagent,name=com.redhat.spice.0")
	}
	return a
}
