package main

import (
	"os"
	"strings"
)

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
	Machine        string // extra -machine opts e.g. ",hpet=off"
	SMM            string // on|off — "" = unset (defaults off)
	CPUFlags       string // appended to -cpu model
	MachineType    string // q35|pc — "" = q35
}

// osQuirks: configure_os_quirks + the OS branches of configure_cpu/
// configure_bios — resolves per-guest-OS device defaults. Case order
// matches Bash (first match wins).
func osQuirks(c Conf, osName, arch string) OsTuning {
	machineType := "q35"
	if m := os.Getenv("MACHINE_TYPE"); m != "" {
		machineType = m
	}
	t := OsTuning{NetDevice: "rtl8139", SMM: "off", MachineType: machineType}

	// pre-case overrides
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

	// machine type: pc for the legacy guests, q35 otherwise.
	// configure_bios also forces pc whenever boot="legacy" — the
	// i440fx/PIIX3 chipset is what SeaBIOS-era guests expect.
	switch {
	case osName == "batocera" || osName == "freedos" || osName == "haiku" || osName == "solaris",
		strings.Contains(c.get("boot", ""), "legacy"):
		t.MachineType = "pc"
	}

	// SMM on for freedos/windows/windows-server (Linux host) and
	// linux+secureboot — configure_cpu.
	switch osName {
	case "freedos", "windows", "windows-server":
		t.SMM = "on"
	case "linux":
		if confOn(c, "secureboot") {
			t.SMM = "on"
		}
	}

	// CPU model/flags: windows x86_64 gets Hyper-V enlightenments;
	// kolibrios/reactos run qemu32; macos hides the hypervisor.
	switch osName {
	case "windows", "windows-server":
		if arch == "x86_64" {
			t.CPUFlags = ",+hypervisor,+invtsc,l3-cache=on"
		}
	case "kolibrios", "reactos":
		t.CPUFlags = "@qemu32"
	case "macos":
		if arch == "x86_64" {
			t.CPUFlags = ",-pdpe1gb,+hypervisor,vmware-cpuid-freq=on,vendor=GenuineIntel"
		}
	}

	// machine opts: hpet=off for macos+windows on x86_64 (QEMU>=70)
	if arch == "x86_64" && (osName == "macos" || osName == "windows" || osName == "windows-server") {
		t.Machine = ",hpet=off"
	}

	// GUEST_TWEAKS emitted as args
	if osName == "windows" || osName == "windows-server" {
		t.ExtraArgs = append(t.ExtraArgs, "-global", "ICH9-LPC.disable_s3=1")
	}
	if osName == "macos" {
		t.ExtraArgs = append(t.ExtraArgs, "-global", "ICH9-LPC.disable_s3=1", "-global", "ICH9-LPC.acpi-pci-hotplug-with-bridge-support=off", "-device", "isa-applesmc,osk="+rot13("bheuneqjbexolgurfrjbeqfthneqrqcyrnfrqbagfgrny(p)NccyrPbzchgreVap"))
	}
	// --status-quo (snapshot=on) appends -snapshot to every guest's
	// drive args in Bash — not linux-only.
	if confOn(c, "snapshot") {
		t.ExtraArgs = append(t.ExtraArgs, "-snapshot")
	}
	return t
}

// rtcArgs: windows/freedos/reactos keep localtime+driftfix; the rest utc.
func rtcArgs(osName string) []string {
	switch osName {
	case "windows", "windows-server", "reactos", "freedos":
		return []string{"-rtc", "base=localtime,clock=host,driftfix=slew"}
	default:
		return []string{"-rtc", "base=utc,clock=host"}
	}
}

// rot13: decodes the Apple SMC OSK key the same way Bash does with tr.
func rot13(s string) string {
	b := []byte(s)
	for i, r := range b {
		switch {
		case r >= 'a' && r <= 'm', r >= 'A' && r <= 'M':
			b[i] = r + 13
		case r >= 'n' && r <= 'z', r >= 'N' && r <= 'Z':
			b[i] = r - 13
		}
	}
	return string(b)
}
