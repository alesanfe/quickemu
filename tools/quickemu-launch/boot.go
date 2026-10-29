// boot.go: boot-variant arg builders — vm_boot_media, vm_boot_fileshare,
// vm_boot_passthru, vm_boot_channels from vm.sh.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func bootMediaArgs(c Conf, dir string) []string {
	var a []string
	iso := c.get("iso", "")
	osn := c.get("guest_os", "")
	// vm_boot_media: boot-order tweaks are per-guest, not per-boot-mode.
	switch {
	case iso != "" && osn == "freedos":
		a = append(a, "-boot", "order=dc")
	case iso != "" && osn == "kolibrios":
		// cdrom index=2 avoids the phantom-iso bug
		a = append(a, "-drive", "media=cdrom,index=2,file="+qemuOptEsc(joinConf(dir, iso)))
	case iso != "" && osn == "reactos":
		a = append(a, "-boot", "order=d",
			"-drive", "if=ide,index=2,media=cdrom,file="+qemuOptEsc(joinConf(dir, iso)))
	case iso != "" && strings.HasPrefix(osn, "windows"):
		if _, err := os.Stat(filepath.Join(dir, "unattended.iso")); err == nil {
			a = append(a, "-drive", "media=cdrom,index=2,file="+qemuOptEsc(filepath.Join(dir, "unattended.iso")))
		}
	}
	if c.get("floppy", "") != "" {
		a = append(a, "-drive", "if=floppy,format=raw,file="+qemuOptEsc(joinConf(dir, c["floppy"])))
	}
	if c.get("driver_iso", "") != "" {
		a = append(a, "-drive", "file="+qemuOptEsc(joinConf(dir, c["driver_iso"]))+",media=cdrom,if=none,id=driver", "-device", "virtio-cdrom,drive=driver")
	}
	return a
}

func passthruArgs(c Conf) []string {
	// usb_devices passthrough is emitted by usbArgs (hostpass stack).
	// gpu_passthrough (vm_boot_passthru): space-separated PCI BDFs bound
	// to vfio-pci; warn (never fatal) on missing IOMMU or wrong driver.
	var a []string
	gp := c.get("gpu_passthrough", "")
	if gp == "" {
		return a
	}
	if entries, err := os.ReadDir("/sys/kernel/iommu_groups"); err != nil || len(entries) == 0 {
		fmt.Printf(" - WARNING! No IOMMU groups found. Enable IOMMU (intel_iommu=on / amd_iommu=on) for GPU passthrough.\n")
	}
	for _, dev := range strings.Fields(gp) {
		if link, err := os.Readlink("/sys/bus/pci/devices/" + dev + "/driver"); err == nil {
			if drv := filepath.Base(link); drv != "vfio-pci" {
				fmt.Printf(" - WARNING! %s is bound to '%s', not vfio-pci.\n", dev, drv)
			}
		}
		fmt.Printf(" - GPU:      Passthrough %s\n", dev)
		a = append(a, "-device", "vfio-pci,host="+qemuOptEsc(dev))
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
