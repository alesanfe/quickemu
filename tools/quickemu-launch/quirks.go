package main

import "strings"

// OsTuning: the variables configure_os_quirks resolves in Bash —
// consumed by the downstream configure_* builders (same two-phase
// design: quirks resolve, builders emit).
type OsTuning struct {
	NetDevice      string
	SoundCard      string
	UsbController  string
	Balloon        string
	MacDiskDev     string
	UsbPassthrough string
	Keyboard       string
	Mouse          string
	ExtraArgs      []string
}

// osQuirks: configure_os_quirks parity — resolves per-guest-OS device
// defaults. Case order matches Bash (first match wins).
func osQuirks(c Conf, osName, arch string) OsTuning {
	t := OsTuning{NetDevice: "rtl8139"} // catch-all default

	// pre-case overrides
	if osName == "batocera" || osName == "haiku" || osName == "kolibrios" {
		t.NetDevice = "rtl8139"
	}
	if osName == "freebsd" || osName == "ghostbsd" {
		t.Mouse = "usb"
	}

	switch {
	case osName == "windows-server":
		t.NetDevice = "e1000"
	case strings.HasSuffix(osName, "bsd") || strings.HasPrefix(osName, "linux") || osName == "windows":
		t.NetDevice = "virtio-net-pci"
	case osName == "freedos":
		t.SoundCard = "sb16"
		t.NetDevice = "pcnet"
	case strings.HasSuffix(osName, "solaris"):
		t.UsbController = "xhci"
		t.SoundCard = "ac97"
	case osName == "reactos":
		t.NetDevice = "e1000"
		t.Keyboard = "ps2"
	case osName == "templeos":
		t.NetDevice = "e1000"
		t.Keyboard = "ps2"
		t.Mouse = "ps2"
		t.SoundCard = "none"
		t.UsbController = "none"
	case osName == "macos":
		rel := c.get("macos_release", "")
		switch rel {
		case "monterey", "ventura", "sonoma", "sequoia", "tahoe":
			t.Balloon = "-device virtio-balloon"
			t.MacDiskDev = "virtio-blk-pci"
			t.NetDevice = "virtio-net-pci"
			t.UsbPassthrough = "nec-usb-xhci"
			t.ExtraArgs = append(t.ExtraArgs, "-global", "nec-usb-xhci.msi=off")
			t.SoundCard = "virtio-sound-pci"
			t.UsbController = "xhci"
		case "big-sur":
			t.Balloon = "-device virtio-balloon"
			t.MacDiskDev = "virtio-blk-pci"
			t.NetDevice = "virtio-net-pci"
			t.UsbPassthrough = "nec-usb-xhci"
			t.ExtraArgs = append(t.ExtraArgs, "-global", "nec-usb-xhci.msi=off")
			t.SoundCard = "ich9-intel-hda"
			t.UsbController = "xhci"
		default:
			if rel == "catalina" {
				t.MacDiskDev = "virtio-blk-pci"
			} else {
				t.MacDiskDev = "ide-hd,bus=ahci.2"
			}
			t.NetDevice = "vmxnet3"
			t.UsbPassthrough = "usb-ehci"
		}
	}

	// global flags / extras (emitted as args, not variables)
	if osName == "windows" || osName == "windows-server" {
		t.ExtraArgs = append(t.ExtraArgs, "-global", "kvm-pit.lost_tick_policy=discard")
	}
	if osName == "macos" {
		t.ExtraArgs = append(t.ExtraArgs, "-global", "ICH9-LPC.disable_s3=1", "-global", "ICH9-LPC.acpi-pci-hotplug-with-bridge-support=off")
	}
	if osName == "linux" && c.get("snapshot", "") == "true" {
		t.ExtraArgs = append(t.ExtraArgs, "-snapshot")
	}
	return t
}
