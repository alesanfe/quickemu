// hw_probe.go: host capability probes (kvm/smt/cores/model/vendor/flags/iface/hypervisor/smbd)
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/alesanfe/quickemu/tools/qelib"
	"golang.org/x/sys/unix"
)

// hostSMT reports whether the host has SMT/hyperthreading on —
// Bash reads /sys/devices/system/cpu/smt/control.
func hostSMT() bool {
	b, err := os.ReadFile("/sys/devices/system/cpu/smt/control")
	return err == nil && strings.TrimSpace(string(b)) == "on"
}

// hostHasKVM probes /dev/kvm the way QEMU does: open + the
// KVM_GET_API_VERSION ioctl (0xAE00). A writable device alone is NOT
// sufficient — WSL2 can present /dev/kvm while every ioctl fails
// EPERM (observed after WSL reboots; QEMU then dies instantly).
// The vmx/svm cpuinfo requirement is separate: enforced at --run via
// hostHasFlag.
func hostHasKVM() bool {
	f, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0)
	if err != nil {
		return false
	}
	defer f.Close()
	v, err := unix.IoctlGetInt(int(f.Fd()), 0xAE00)
	return err == nil && v > 0
}

// hasKVM is the accel-gate seam (ADR-0009): tests stub it to cover
// both device-present and device-absent hosts without needing real
// /dev/kvm access.
var hasKVM = hostHasKVM

func hostNCores() int {
	// get_nproc parity — Go's runtime equivalent of `nproc`.
	return runtime.NumCPU()
}

// hostSockets: HOST_CPU_SOCKETS parity — lscpu "Socket(s)", 1 when
// unresolved.
func hostSockets() int {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "lscpu").Output()
	if err != nil {
		return 1
	}
	for _, l := range strings.Split(string(out), "\n") {
		if v := colonValue(l, "Socket(s):"); v != "" {
			n := 1
			fmt.Sscanf(v, "%d", &n)
			return n
		}
	}
	return 1
}

// colonValue: trimmed value after the first ':' for a line carrying
// prefix — "" when absent. A prefix match alone does not guarantee a
// separator: a crafted "vendor_idless"/"model nameless" line would
// panic the old SplitN[1] index.
func colonValue(l, prefix string) string {
	if !strings.HasPrefix(l, prefix) {
		return ""
	}
	_, v, ok := strings.Cut(l, ":")
	if !ok {
		return ""
	}
	return strings.TrimSpace(v)
}

// hostCPUModel: HOST_CPU_MODEL parity — first "model name" from
// cpuinfo (lscpu Model name on any arch).
func hostCPUModel() string {
	b, err := os.ReadFile("/proc/cpuinfo")
	if err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if v := colonValue(l, "model name"); v != "" {
				return v
			}
		}
	}
	return ""
}

// netListen: seam for firstFreePort's TCP probe — tests replace it so
// port scanning stays deterministic on hosts where the scanned range
// is already bound by unrelated processes.
// smbdPresent: configure_file_sharing parity — the Samba guest share
// is only wired when smbd resolves (PATH or the /usr/sbin fallback).
func smbdPresent() bool {
	if _, err := exec.LookPath("smbd"); err == nil {
		return true
	}
	if st, err := os.Stat("/usr/sbin/smbd"); err == nil && st.Mode()&0o111 != 0 {
		return true
	}
	return false
}

// hostArch maps GOARCH to the qemu arch token (x86_64|aarch64).
func hostArch() string { return qelib.HostArch() }

// hostHypervisor: DMI sys_vendor detection — qemu/virtualbox mean we
// are inside a VM (Bash MANUFACTURER case in configure_cpu).
func hostHypervisor() string {
	b, err := os.ReadFile("/sys/class/dmi/id/sys_vendor")
	if err != nil {
		return ""
	}
	v := strings.ToLower(strings.TrimSpace(string(b)))
	if v == "qemu" || v == "virtualbox" {
		return v
	}
	return ""
}

// hostCPUVendor: vendor_id from cpuinfo (GenuineIntel|AuthenticAMD|
// HygonGenuine). Darwin resolves it via sysctl like get_cpu_info —
// machdep.cpu.vendor on Intel, brand_string's first word on arm64.
func hostCPUVendor() string {
	if runtime.GOOS == "darwin" {
		if runtime.GOARCH == "arm64" {
			if out, err := execCommand("sysctl", "-n", "machdep.cpu.brand_string").Output(); err == nil {
				if f := strings.Fields(strings.TrimSpace(string(out))); len(f) > 0 {
					return f[0]
				}
			}
			return ""
		}
		out, err := execCommand("sysctl", "-n", "machdep.cpu.vendor").Output()
		if err != nil {
			return ""
		}
		return strings.ReplaceAll(strings.TrimSpace(string(out)), " ", "")
	}
	b, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return ""
	}
	for _, l := range strings.Split(string(b), "\n") {
		if v := colonValue(l, "vendor_id"); v != "" {
			return v
		}
	}
	return ""
}

// hostHasFlag: vmx/svm/sse4_2/... presence (check_cpu_flag). Linux
// reads cpuinfo — AMD's invtsc is reported as constant_tsc. Darwin
// greps sysctl machdep.cpu.features with the flag mapped like Bash
// (upper, _→., X2APIC→x2APIC, AVX→AVX1.0).
func hostHasFlag(flag string) bool {
	if runtime.GOOS == "darwin" {
		f := strings.ToUpper(strings.ReplaceAll(flag, "_", "."))
		f = strings.ReplaceAll(f, "X2APIC", "x2APIC")
		if f == "AVX" {
			f = "AVX1.0"
		}
		out, err := execCommand("sysctl", "-n", "machdep.cpu.features").Output()
		return err == nil && strings.Contains(string(out), f)
	}
	b, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return false
	}
	if strings.Contains(string(b), " "+flag) {
		return true
	}
	return flag == "invtsc" && hostCPUVendor() == "AuthenticAMD" &&
		strings.Contains(string(b), " constant_tsc")
}

// hostMacosTSCUnstable: host_macos_tsc_unstable — Linux host + AMD +
// macos guest + Ventura+ release + no tsc=reliable + clocksource!=tsc.
// Returns (unstable, clocksource).
func hostMacosTSCUnstable(c Conf) (bool, string) {
	if runtime.GOOS != "linux" || hostCPUVendor() != "AuthenticAMD" {
		return false, ""
	}
	if c.get("guest_os", "") != "macos" {
		return false, ""
	}
	switch c.get("macos_release", "") {
	case "ventura", "sonoma", "sequoia", "tahoe":
	default:
		return false, ""
	}
	if b, err := os.ReadFile("/proc/cmdline"); err == nil && strings.Contains(string(b), "tsc=reliable") {
		return false, ""
	}
	b, err := os.ReadFile("/sys/devices/system/clocksource/clocksource0/current_clocksource")
	if err != nil {
		return false, ""
	}
	clock := strings.TrimSpace(string(b))
	return clock != "tsc", clock
}

// hostDefaultIface: interface owning the default route (host_default_iface).
func hostDefaultIface() string {
	b, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return ""
	}
	for _, l := range strings.Split(string(b), "\n")[1:] {
		f := strings.Fields(l)
		if len(f) >= 2 && f[1] == "00000000" {
			return f[0]
		}
	}
	return ""
}

// hostTotalMemMB: MemTotal in MiB (host_total_mem_mb). Var seam: tests
// stub it to exercise the configure_ram tiering and min_ram gates.
var hostTotalMemMB = func() int {
	if b, err := os.ReadFile("/proc/meminfo"); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(l, "MemTotal:") {
				var kb int
				if _, err := fmt.Sscanf(l, "MemTotal: %d kB", &kb); err == nil {
					return kb / 1024
				}
			}
		}
	}
	return 0
}
