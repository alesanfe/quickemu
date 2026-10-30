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
	// aarch64: virt machine has no IDE — CD-ROMs hang off virtio-scsi.
	// Bash clears iso for kolibrios/reactos before this check, so the
	// controller only appears when the generic cd0 emit will follow.
	a64 := c.get("arch", "x86_64") == "aarch64"
	genISO := c.get("iso", "") != "" && osn != "kolibrios" && osn != "reactos"
	if a64 && (genISO || c.get("fixed_iso", "") != "") {
		a = append(a, "-device", "virtio-scsi-pci,id=scsi0")
	}
	// vm_boot_media emits kolibrios/reactos ISOs at index=2 (phantom-
	// iso workaround, handled above); the generic emit must skip them.
	if d := c.get("iso", ""); d != "" && genISO {
		if a64 {
			// bootindex=1: UEFI boots the installer before the disk.
			a = append(a, "-device", "scsi-cd,drive=cd0,bus=scsi0.0,bootindex=1",
				"-drive", "id=cd0,if=none,format=raw,media=cdrom,readonly=on,file="+qemuOptEsc(joinConf(dir, d)))
		} else {
			// Bash: plain cdrom drive (IDE), indexed.
			a = append(a, "-drive", "media=cdrom,index=0,file="+qemuOptEsc(joinConf(dir, d)))
		}
	}
	if d := c.get("fixed_iso", ""); d != "" {
		if a64 {
			a = append(a, "-device", "scsi-cd,drive=cd1,bus=scsi0.0,bootindex=3",
				"-drive", "id=cd1,if=none,format=raw,media=cdrom,readonly=on,file="+qemuOptEsc(joinConf(dir, d)))
		} else {
			a = append(a, "-drive", "media=cdrom,index=1,file="+qemuOptEsc(joinConf(dir, d)))
		}
	}
	// driver_iso removed: no Bash counterpart and virtio-cdrom is not a
	// real QEMU device — a conf using it would abort the launch.
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

// channelArgs removed: Bash has no agent/guest_agent conf keys — the
// qemu-vdagent chardev was orphaned (no device referenced qga0) and
// the virtserialport duplicated com.redhat.spice.0 from the SPICE
// block. The guest-agent channel already emits inside -spice.
