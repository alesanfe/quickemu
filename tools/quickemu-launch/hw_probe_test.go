package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Host-probe coverage: the probes degrade gracefully when their
// /proc, /sys or PATH sources are absent, so every branch below runs
// on a plain Linux CI host without KVM/usb/DMI fixtures.

func TestSmbdPresent(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "smbd")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if !smbdPresent() {
		t.Error("smbd on PATH not detected")
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := os.Stat("/usr/sbin/smbd"); os.IsNotExist(err) && smbdPresent() {
		t.Error("smbdPresent true with no PATH match and no /usr/sbin fallback")
	}
}

func TestHostHasKVMProbe(t *testing.T) {
	// Result depends on the host device — the call itself must not
	// panic and must agree with /dev/kvm's openability.
	got := hostHasKVM()
	f, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0)
	if err != nil && got {
		t.Error("hostHasKVM true with /dev/kvm not openable")
	}
	if err == nil {
		f.Close()
	}
}

func TestHostCPUVendorProbe(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("procfs probe")
	}
	if v := hostCPUVendor(); v != "GenuineIntel" && v != "AuthenticAMD" &&
		v != "HygonGenuine" && v != "" {
		t.Errorf("unexpected vendor %q", v)
	}
}

func TestHostHasFlagProbe(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("procfs probe")
	}
	if !hostHasFlag("fpu") {
		t.Error("fpu flag missing from cpuinfo — impossible on x86/ARM")
	}
	if hostHasFlag("definitely_not_a_cpu_flag_qe") {
		t.Error("bogus flag reported present")
	}
}

func TestHostHypervisorProbe(t *testing.T) {
	// WSL/containers lack /sys/class/dmi — the probe must return ""
	// instead of failing.
	_ = hostHypervisor()
}

func TestDaemonSockPidFallback(t *testing.T) {
	// No fuser owner and no /proc comm match → the caller's fallback
	// pid comes back untouched.
	if got := daemonSockPid("/nonexistent-qe.sock", "qe-nosuchdaemon", 43210); got != 43210 {
		t.Errorf("fallback pid lost: %d", got)
	}
}

func TestPortRegistryReserveRoundtrip(t *testing.T) {
	reg := t.TempDir()
	t.Setenv("QE_PORT_REGISTRY", reg)
	portRegistryReserve(45678, os.Getpid(), "inst-x", "vm-y")
	b, err := os.ReadFile(filepath.Join(reg, "45678"))
	if err != nil {
		t.Fatalf("reservation not recorded: %v", err)
	}
	if !strings.Contains(string(b), "inst-x vm-y") {
		t.Errorf("record missing instance/vm fields: %q", b)
	}
	if !portRegistryTaken(45678) {
		t.Error("live reservation not seen as taken")
	}
	portRegistryRelease(os.Getpid())
	if portRegistryTaken(45678) {
		t.Error("release did not drop the reservation")
	}
}

func TestApplyCPUPinningGuards(t *testing.T) {
	// Early-return guards: empty/bad mapping, non-linux, no CPU comms.
	applyCPUPinning(os.Getpid(), Conf{})
	applyCPUPinning(os.Getpid(), Conf{"cpu_pinning": "bogus"})
	// Valid mapping — own task list scanned, no "CPU" comms → no-op.
	applyCPUPinning(os.Getpid(), Conf{"cpu_pinning": "0,1"})
}

func TestHostMacosTSCUnstableGuards(t *testing.T) {
	if u, _ := hostMacosTSCUnstable(Conf{"guest_os": "linux"}); u {
		t.Error("non-macos guest flagged unstable")
	}
	if u, _ := hostMacosTSCUnstable(Conf{"guest_os": "macos", "macos_release": "mojave"}); u {
		t.Error("pre-Ventura macos flagged unstable")
	}
	// Ventura+ on whatever host — the AMD/cpuinfo probe decides; on
	// Intel hosts it must be false.
	if v := hostCPUVendor(); v == "GenuineIntel" {
		if u, _ := hostMacosTSCUnstable(Conf{"guest_os": "macos", "macos_release": "ventura"}); u {
			t.Error("Intel host flagged AMD-only TSC issue")
		}
	}
}

func TestCocoaGLESNonDarwin(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("probe is darwin-only")
	}
	if cocoaGLES() {
		t.Error("cocoaGLES true on non-darwin")
	}
}

func TestCheckUSBPassthroughGuards(t *testing.T) {
	// Empty list and malformed entries are skipped silently; a vid:pid
	// that lsusb cannot resolve prints "not found" and is dropped.
	checkUSBPassthrough(Conf{})
	checkUSBPassthrough(Conf{"usb_devices": "notavpid anotherbad"})
	checkUSBPassthrough(Conf{"usb_devices": "ffff:ffff"})
}

func TestStorageEchoBranches(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"d.qcow2", "i.iso", "r.img", "f.iso"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte{0}, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// disk + iso + fixed_iso all present
	storageEcho(Conf{"disk_img": "d.qcow2", "iso": "i.iso", "fixed_iso": "f.iso"}, dir)
	// iso absent → recovery img echoed instead
	storageEcho(Conf{"img": "r.img"}, dir)
	// empty conf → silent
	storageEcho(Conf{}, dir)
}

// Regression: a prefix-matching line without ":" panicked on the old
// SplitN(l,":",2)[1] index — a crafted cpuinfo line like "vendor_idless"
// matched the prefix but carried no separator.
func TestColonValue(t *testing.T) {
	if got := colonValue("vendor_idless", "vendor_id"); got != "" {
		t.Fatalf("no-colon line must yield \"\", got %q", got)
	}
	if got := colonValue("model name : GenuineIntel x", "model name"); got != "GenuineIntel x" {
		t.Fatalf("value parse: %q", got)
	}
	if got := colonValue("other: x", "model name"); got != "" {
		t.Fatalf("non-matching prefix: %q", got)
	}
	if got := colonValue("Socket(s):  2", "Socket(s):"); got != "2" {
		t.Fatalf("sockets parse: %q", got)
	}
}
