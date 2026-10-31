package main

import (
	"fmt"
	"os"
	"runtime"
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
	// FatalErr: deferred configure_os_quirks fatals (macOS SSE/AVX2
	// gates). osQuirks must stay side-effect-free because run() calls
	// it before the CPU/RAM echoes; Bash reaches the check after the
	// BOOT echo, so run() fires it there.
	FatalErr string
}

// osQuirks: configure_os_quirks + the OS branches of configure_cpu/
// configure_bios — resolves per-guest-OS device defaults. Case order
// matches Bash (first match wins).
func osQuirks(c Conf, osName, arch string) OsTuning {
	machineType := "q35"
	if m := os.Getenv("MACHINE_TYPE"); m != "" {
		machineType = m
	}
	t := OsTuning{NetDevice: "rtl8139", SMM: "off", MachineType: machineType,
		Balloon: "-device virtio-balloon"}

	// pre-case overrides
	if osName == "freebsd" || osName == "ghostbsd" {
		t.Mouse = "usb"
	}

	quirkDevices(c, osName, &t)

	// machine type: pc for the legacy guests, q35 otherwise.
	// configure_bios's else branch forces pc for ANY non-efi boot —
	// i440fx/PIIX3 is what SeaBIOS-era guests expect.
	switch {
	case osName == "batocera" || osName == "freedos" || osName == "haiku" || osName == "solaris",
		osName == "kolibrios" || osName == "reactos",
		!strings.Contains(resolvedBoot(c, osName), "efi"):
		t.MachineType = "pc"
	}

	// SMM on for freedos/windows/windows-server and linux+secureboot —
	// Linux hosts only; SMM is unavailable in Homebrew QEMU (Darwin).
	if runtime.GOOS == "linux" {
		switch osName {
		case "freedos", "windows", "windows-server":
			t.SMM = "on"
		case "linux":
			if confOn(c, "secureboot") {
				t.SMM = "on"
			}
		}
	}

	// CPU model/flags: windows x86_64 gets Hyper-V enlightenments;
	// kolibrios/reactos run qemu32; macos hides the hypervisor.
	quirkCPU(c, osName, arch, &t)

	// GUEST_TWEAKS emitted as args
	if osName == "windows" || osName == "windows-server" {
		t.ExtraArgs = append(t.ExtraArgs, "-global", "ICH9-LPC.disable_s3=1")
	}
	if osName == "macos" {
		t.ExtraArgs = append(t.ExtraArgs, "-global", "ICH9-LPC.disable_s3=1", "-global", "ICH9-LPC.acpi-pci-hotplug-with-bridge-support=off", "-device", "isa-applesmc,osk="+rot13("bheuneqjbexolgurfrjbeqfthneqrqcyrnfrqbagfgrny(p)NccyrPbzchgreVap"))
	}
	// machine opts: hpet=off for macos+windows on x86_64 — QEMU>=70 has
	// the machine property; older builds take -no-hpet instead.
	if arch == "x86_64" && (osName == "macos" || osName == "windows" || osName == "windows-server") {
		if v := qemuVersionShort(arch); v < 0 || v >= 70 {
			t.Machine = ",hpet=off"
		} else {
			t.ExtraArgs = append(t.ExtraArgs, "-no-hpet")
		}
	}
	return t
}

// quirkDevices: the first configure_os_quirks switch — per-guest-OS
// device defaults (net/sound/usb/disk/passthrough). Case order
// matches Bash (first match wins).
func quirkDevices(c Conf, osName string, t *OsTuning) {
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
		quirkMacosDevices(c, t)
	}
}

// quirkMacosDevices: the macos_release table inside the device
// switch — virtio stack for Monterey+, ich9 HDA for Big Sur, and the
// pre-virtio fallback (High Sierra/Mojave/unknown release).
func quirkMacosDevices(c Conf, t *OsTuning) {
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
		// Pre-virtio macOS (High Sierra/Mojave/unknown release):
		// no virtio-balloon support (BALLOON="" in os_quirks).
		t.Balloon = ""
		if rel == "catalina" {
			t.MacDiskDev = "virtio-blk-pci"
		} else {
			t.MacDiskDev = "ide-hd,bus=ahci.2"
		}
		t.NetDevice = "vmxnet3"
		t.UsbPassthrough = "usb-ehci"
	}
}

// quirkCPU: the OS branches of configure_cpu's CPU model/flags —
// windows x86_64 gets Hyper-V enlightenments; kolibrios/reactos run
// qemu32; macos hides the hypervisor.
func quirkCPU(c Conf, osName, arch string, t *OsTuning) {
	// tcg_cpu_model="..." pins the emulated model — TCG-only, so a
	// conf shipped for hosts without KVM never displaces `host`
	// under KVM (Fedora 43's MTTCG boot hang set cpu_model=max +
	// tcg_thread=single in conf_gen).
	if m := c.get("tcg_cpu_model", ""); m != "" && c["_accel"] == "tcg" {
		t.CPUFlags = "@" + m
		return
	}
	switch osName {
	case "windows", "windows-server":
		quirkWindowsCPU(c, arch, t)
	case "kolibrios", "reactos":
		t.CPUFlags = "@qemu32"
	case "macos":
		quirkMacosCPU(c, arch, t)
	}
}

// quirkWindowsCPU: windows x86_64 gets Hyper-V enlightenments —
// KVM-specific flags (configure_os_quirks): QEMU>6.0 takes
// hv_passthrough; older builds get the legacy hv_* list.
func quirkWindowsCPU(c Conf, arch string, t *OsTuning) {
	if arch != "x86_64" {
		return
	}
	t.CPUFlags = ",+hypervisor,+invtsc,l3-cache=on"
	if c["_accel"] != "kvm" {
		return
	}
	if v := qemuVersionShort(arch); v < 0 || v > 60 {
		t.CPUFlags += ",migratable=no,hv_passthrough"
		return
	}
	t.CPUFlags += ",migratable=no,hv_frequencies"
	// kvm_pv_unhalt only on the non-Darwin branch
	// (CPU_KVM_UNHALT is empty for hvf hosts).
	if runtime.GOOS != "darwin" {
		t.CPUFlags += ",kvm_pv_unhalt"
	}
	t.CPUFlags += ",hv_reenlightenment,hv_relaxed,hv_spinlocks=8191,hv_stimer,hv_synic,hv_time," +
		"hv_vapic,hv_vendor_id=1234567890ab,hv_vpindex"
}

// quirkMacosCPU — configure_cpu macos branch: Intel host + bare metal
// passes the host model through; anything else emulates Haswell-v2
// with an Intel vendor string so macOS accepts the CPU.
func quirkMacosCPU(c Conf, arch string, t *OsTuning) {
	if arch != "x86_64" {
		return
	}
	intel := hostCPUVendor() == "GenuineIntel"
	hyper := c["_hypervisor"] != ""
	if intel && !hyper {
		t.CPUFlags = ",-pdpe1gb,+hypervisor,vmware-cpuid-freq=on"
	} else {
		t.CPUFlags = "@Haswell-v2,vendor=GenuineIntel,-pdpe1gb,+avx,+sse,+sse2,+ssse3,vmware-cpuid-freq=on"
	}
	// Feature gates only apply under hardware accel — TCG
	// emulates the features (macOS x86_64 on ARM hosts).
	if c["_accel"] == "tcg" {
		return
	}
	rel := c.get("macos_release", "")
	switch rel {
	case "ventura", "sonoma", "sequoia", "tahoe":
		if hostHasFlag("sse4_2") && hostHasFlag("avx2") {
			if !intel && !hyper {
				t.CPUFlags += ",+avx2,+sse4.2"
			}
		} else {
			t.FatalErr = fmt.Sprintf("ERROR! macOS %s requires a CPU with SSE 4.2 and AVX2 support.\n       Try macOS Monterey or Big Sur.", rel)
		}
	case "catalina", "big-sur", "monterey":
		if hostHasFlag("sse4_2") {
			if !intel && !hyper {
				t.CPUFlags += ",+sse4.2"
			}
		} else {
			t.FatalErr = fmt.Sprintf("ERROR! macOS %s requires a CPU with SSE 4.2 support.", rel)
		}
	default:
		if hostHasFlag("sse4_1") {
			if !intel && !hyper {
				t.CPUFlags += ",+sse4.1"
			}
		} else {
			t.FatalErr = fmt.Sprintf("ERROR! macOS %s requires a CPU with SSE 4.1 support.", rel)
		}
	}
	if !intel && !hyper {
		quirkMacosCPUPass(t)
	}
}

// quirkMacosCPUPass — configure_cpu_flag list for non-Intel bare
// metal: pass through every feature the host reports, plus the AMD
// TSC stability flags.
func quirkMacosCPUPass(t *OsTuning) {
	for _, f := range []string{
		"abm", "adx", "aes", "amd-ssbd", "apic", "arat", "bmi1", "bmi2",
		"clflush", "cmov", "cx8", "cx16", "de", "eist", "erms", "f16c",
		"fma", "fp87", "fsgsbase", "fxsr", "invpcid", "invtsc", "lahf_lm",
		"lm", "mca", "mce", "mmx", "movbe", "mpx", "msr", "mtrr", "nx",
		"pae", "pat", "pcid", "pge", "pse", "popcnt", "pse36", "rdrand",
		"rdtscp", "sep", "smep", "syscall", "tsc", "tsc_adjust", "vaes",
		"vbmi2", "vmx", "vpclmulqdq", "x2apic", "xgetbv1", "xsave", "xsaveopt",
	} {
		if hostHasFlag(f) {
			qf := strings.ReplaceAll(f, "_", "-")
			qf = strings.ReplaceAll(qf, "4-", "4.")
			if runtime.GOOS == "darwin" && f == "avx" {
				qf = "AVX1.0"
			}
			t.CPUFlags += ",+" + qf
		} else if f == "pcid" && hostCPUVendor() == "AuthenticAMD" {
			// AMD lacks pcid — disable it explicitly.
			t.CPUFlags += ",-pcid"
		}
	}
	// AMD constant_tsc needs the explicit TSC flags for
	// macOS stability (configure_cpu_flag equivalents).
	if hostCPUVendor() == "AuthenticAMD" && hostHasFlag("invtsc") {
		t.CPUFlags += ",+tsc,+tsc-deadline"
	}
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
