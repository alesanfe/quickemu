// args_firmware_test — firmware/BIOS/cpu/ram arg-generation tests (split of args2_test.go).
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSeedEFIVars(t *testing.T) {
	dir := t.TempDir()
	c := Conf{"_vmdir": dir}
	// not running → returns the path but writes nothing.
	seedEFIVars(c, "")
	if _, err := os.Stat(filepath.Join(dir, "OVMF_VARS.fd")); err == nil {
		t.Fatal("plan mode must not seed vars")
	}
	// running + existing vars → no-op.
	os.WriteFile(filepath.Join(dir, "OVMF_VARS.fd"), []byte("v"), 0o600)
	c["_run"] = "1"
	seedEFIVars(c, "")
	if b, _ := os.ReadFile(filepath.Join(dir, "OVMF_VARS.fd")); string(b) != "v" {
		t.Fatal("existing vars must win (no reseed)")
	}
	// running + template → seeds the copy.
	os.Remove(filepath.Join(dir, "OVMF_VARS.fd"))
	tmpl := filepath.Join(t.TempDir(), "OVMF_VARS.fd")
	os.WriteFile(tmpl, []byte("template"), 0o600)
	if got := seedEFIVars(c, tmpl); got != filepath.Join(dir, "OVMF_VARS.fd") {
		t.Fatalf("vars path %q", got)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "OVMF_VARS.fd")); err != nil || string(b) != "template" {
		t.Fatalf("seeded %v %q", err, b)
	}
}

// Firmware parity (configure_bios):
//   - macOS uses the vmdir OVMF_CODE.fd + OVMF_VARS-*.fd downloaded by
//     quickget, requires OpenCore.qcow2/ESP.qcow2, and sizes are sanity
//     checked — missing pieces are fatal errors, not silent fallback.
//   - qcow2 firmware (magic QFI\xfb) emits format=qcow2 pflash drives
//     and seeds OVMF_VARS.qcow2.
//   - aarch64 windows prefers monolithic -bios QEMU_EFI.fd; other
//     aarch64 guests use named -blockdev pflash pairs.
//   - boot="legacy" forces MACHINE_TYPE=pc.
//   - SystemDisk honors disk_format from the conf.
func TestParamCheckFirmwareMacOS(t *testing.T) {
	dir := t.TempDir()
	c := Conf{"guest_os": "macos", "_vmdir": dir, "_run": "1"}
	// Both missing → Bash's MAC_MISSING reports Bootloader (last check
	// wins); with only firmware present it still reports Bootloader.
	if err := paramCheckFirmware(c, "macos", "x86_64"); err == nil {
		t.Fatal("missing macOS firmware must error")
	}
	os.WriteFile(filepath.Join(dir, "OVMF_CODE.fd"), make([]byte, 2<<20), 0o644)
	os.WriteFile(filepath.Join(dir, "OVMF_VARS-1920x1080.fd"), make([]byte, 1<<17), 0o644)
	if err := paramCheckFirmware(c, "macos", "x86_64"); err == nil ||
		!strings.Contains(err.Error(), "Bootloader") {
		t.Fatalf("missing bootloader must error: %v", err)
	}
	// Firmware missing but bootloader present → reports Firmware.
	os.WriteFile(filepath.Join(dir, "OpenCore.qcow2"), []byte("qc"), 0o644)
	os.Remove(filepath.Join(dir, "OVMF_VARS-1920x1080.fd"))
	if err := paramCheckFirmware(c, "macos", "x86_64"); err == nil ||
		!strings.Contains(err.Error(), "Firmware") {
		t.Fatalf("missing firmware must error: %v", err)
	}
	os.WriteFile(filepath.Join(dir, "OVMF_VARS-1920x1080.fd"), make([]byte, 1<<17), 0o644)
	os.WriteFile(filepath.Join(dir, "OpenCore.qcow2"), []byte("qc"), 0o644)
	if err := paramCheckFirmware(c, "macos", "x86_64"); err != nil {
		t.Fatalf("complete macOS firmware must pass: %v", err)
	}
	// Truncated vars → corrupt-firmware error.
	os.WriteFile(filepath.Join(dir, "OVMF_VARS-1920x1080.fd"), make([]byte, 1024), 0o644)
	if err := paramCheckFirmware(c, "macos", "x86_64"); err == nil ||
		!strings.Contains(err.Error(), "corrupt") {
		t.Fatalf("truncated vars must error: %v", err)
	}
}

func TestBiosArgsMacOSFirmware(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "OVMF_CODE.fd"), make([]byte, 2<<20), 0o644)
	os.WriteFile(filepath.Join(dir, "OVMF_VARS-1024x768.fd"), make([]byte, 1<<17), 0o644)
	c := Conf{"guest_os": "macos", "_vmdir": dir}
	a := biosArgs(c, "macos", "x86_64")
	joined := strings.Join(a, " ")
	if !strings.Contains(joined, filepath.Join(dir, "OVMF_CODE.fd")) ||
		!strings.Contains(joined, "OVMF_VARS-1024x768.fd") {
		t.Fatalf("macOS must use vmdir firmware, got %v", a)
	}
}

func TestFirmwareQcow2Detection(t *testing.T) {
	dir := t.TempDir()
	raw := filepath.Join(dir, "raw.fd")
	os.WriteFile(raw, make([]byte, 64), 0o644)
	qc := filepath.Join(dir, "vars.qcow2")
	os.WriteFile(qc, append([]byte{0x51, 0x46, 0x49, 0xfb}, make([]byte, 60)...), 0o644)
	if isFirmwareQcow2(raw) || !isFirmwareQcow2(qc) {
		t.Fatal("qcow2 magic detection wrong")
	}
}

func TestBootLegacyMachinePC(t *testing.T) {
	// Bash forces MACHINE_TYPE=pc when boot="legacy" (i440fx/PIIX3).
	if osQuirks(Conf{"boot": "legacy"}, "linux", "x86_64").MachineType != "pc" {
		t.Fatal("boot=legacy must force machine type pc")
	}
	if osQuirks(Conf{"boot": "efi"}, "linux", "x86_64").MachineType != "q35" {
		t.Fatal("efi boot must keep q35")
	}
	// configure_cpu: kolibrios/reactos also get MACHINE_TYPE=pc.
	for _, g := range []string{"kolibrios", "reactos"} {
		if mt := osQuirks(Conf{"guest_os": g}, g, "x86_64").MachineType; mt != "pc" {
			t.Fatalf("%s must use machine type pc, got %s", g, mt)
		}
	}
}

// a64MonolithicEDK2: EFI_MODE=bios probe — only resolves when the
// Fedora monolithic EDK2 blob exists under a firmware root.
func TestA64MonolithicEDK2(t *testing.T) {
	fw := t.TempDir()
	t.Setenv("QE_FIRMWARE_DIR", fw)
	c := Conf{"guest_os": "windows"}
	if got := a64MonolithicEDK2(c, "windows"); got != "" {
		t.Fatalf("absent EDK2 must not resolve -bios mode: %s", got)
	}
	os.MkdirAll(filepath.Join(fw, "edk2", "aarch64"), 0o755)
	os.WriteFile(filepath.Join(fw, "edk2", "aarch64", "QEMU_EFI.fd"), []byte("x"), 0o644)
	if got := a64MonolithicEDK2(c, "windows"); got == "" {
		t.Fatal("present EDK2 must resolve -bios mode")
	}
	if got := a64MonolithicEDK2(Conf{"guest_os": "windows", "secureboot": "on"}, "windows"); got != "" {
		t.Fatalf("secureboot must stay pflash: %s", got)
	}
	if got := a64MonolithicEDK2(Conf{"guest_os": "linux"}, "linux"); got != "" {
		t.Fatalf("non-windows must stay pflash: %s", got)
	}
}

// configure_cpu parity: win11 needs >=2 cores, macOS rounds down to a
// power of two, -smp carries the host socket count.
func TestCPUArgsContract(t *testing.T) {
	a := cpuArgs(Conf{"cpu_cores": "1", "guest_os": "windows", "_vmname": "windows-11"},
		"windows", "x86_64", OsTuning{})
	if !has(a, "cores=2") && !has(a, "cores=1") {
		t.Fatalf("win11 smp %v", a)
	}
	// macOS 6 cores -> 4 logical (nearest lower power of 2); with SMT
	// the -smp line shows cores=2,threads=2 — either is 4 vCPUs total.
	a = cpuArgs(Conf{"cpu_cores": "6", "guest_os": "macos"}, "macos", "x86_64", OsTuning{})
	found := false
	for _, s := range a {
		if strings.HasPrefix(s, "cores=") {
			found = strings.Contains(s, "cores=4") ||
				(strings.Contains(s, "cores=2") && strings.Contains(s, "threads=2"))
		}
	}
	if !found {
		t.Fatalf("macOS power-of-2 rounding failed: %v", a)
	}
	// sockets= is always present.
	found = false
	for _, s := range a {
		if strings.Contains(s, "sockets=") {
			found = true
		}
	}
	if !found {
		t.Fatalf("smp missing sockets: %v", a)
	}
}

func TestRamArgsTiering(t *testing.T) {
	a := ramArgs(Conf{"ram": "12G"}, "linux", "vm.conf")
	if !has(a, "12G") {
		t.Fatalf("explicit ram %v", a)
	}
	if a = ramArgs(Conf{}, "linux", "vm.conf"); len(a) < 2 || a[0] != "-m" {
		t.Fatalf("ramArgs %v", a)
	}
}

// cpu_pinning (vm.sh): ^[0-9]+(,[0-9]+)*$ and the count must equal the
// guest logical core count (Linux-only application).
func TestCPUPinningValidation(t *testing.T) {
	if m, err := parseCPUPinning("0,2,4,6"); err != nil || len(m) != 4 || m[1] != 2 {
		t.Fatalf("valid pinning rejected: %v (%v)", err, m)
	}
	for _, bad := range []string{"0-3", "a,b", "1,,2", "", "1;2"} {
		if bad == "" {
			continue
		}
		if _, err := parseCPUPinning(bad); err == nil {
			t.Fatalf("bad pinning %q accepted", bad)
		}
	}
	c := Conf{"cpu_pinning": "0,1", "_lcores": "2"}
	if err := paramCheckPinning(c); err != nil {
		t.Fatalf("count match rejected: %v", err)
	}
	c["_lcores"] = "4"
	if err := paramCheckPinning(c); err == nil {
		t.Fatal("count mismatch must error")
	}
}

// configure_os_quirks version gates: windows on KVM gets hv_passthrough
// on QEMU>6.0 (legacy hv_* list below), and hpet=off lands in -machine
// on QEMU>=70 (-no-hpet arg below). _accel is stashed by main().
func TestOsQuirksVersionGates(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stub script needs a POSIX shell")
	}
	stub := t.TempDir()
	t.Setenv("PATH", stub+string(os.PathListSeparator)+os.Getenv("PATH"))
	mkver := func(v string) {
		os.WriteFile(filepath.Join(stub, "qemu-system-x86_64"),
			[]byte("#!/bin/sh\necho 'QEMU emulator version "+v+"'\n"), 0o755)
	}
	mkver("10.1.0")
	c := Conf{"_accel": "kvm"}
	tu := osQuirks(c, "windows", "x86_64")
	if !strings.Contains(tu.CPUFlags, "hv_passthrough") ||
		strings.Contains(tu.CPUFlags, "hv_frequencies") {
		t.Fatalf("QEMU 10.1 wants hv_passthrough: %s", tu.CPUFlags)
	}
	if tu.Machine != ",hpet=off" {
		t.Fatalf("QEMU 10.1 wants ,hpet=off: %q", tu.Machine)
	}
	mkver("5.2.0")
	tu = osQuirks(Conf{"_accel": "kvm"}, "windows", "x86_64")
	if !strings.Contains(tu.CPUFlags, "hv_frequencies") ||
		strings.Contains(tu.CPUFlags, "hv_passthrough") {
		t.Fatalf("QEMU 5.2 wants the legacy hv_* list: %s", tu.CPUFlags)
	}
	if tu.Machine == ",hpet=off" || !has(tu.ExtraArgs, "-no-hpet") {
		t.Fatalf("QEMU 5.2 wants -no-hpet arg: %q %v", tu.Machine, tu.ExtraArgs)
	}
	// 6.x: new hv flags but still -no-hpet (65 < 70).
	mkver("6.5.2")
	tu = osQuirks(Conf{"_accel": "kvm"}, "windows", "x86_64")
	if !strings.Contains(tu.CPUFlags, "hv_passthrough") ||
		has(tu.ExtraArgs, "-no-hpet") == false || tu.Machine == ",hpet=off" {
		t.Fatalf("QEMU 6.5: hv_passthrough + -no-hpet: %s %v", tu.CPUFlags, tu.ExtraArgs)
	}
	// TCG: no hv flags at all.
	mkver("10.1.0")
	tu = osQuirks(Conf{"_accel": "tcg"}, "windows", "x86_64")
	if strings.Contains(tu.CPUFlags, "hv_") {
		t.Fatalf("TCG must not emit hv flags: %s", tu.CPUFlags)
	}
}

// aarch64 -machine emit (vm_boot): no x86 smm/vmport; pflash blockdev
// nodes on the virt machine when boot=efi.
func TestAarch64MachineEmit(t *testing.T) {
	c := richConf(t, "arch=\"aarch64\"\nboot=\"efi\"\nnetwork=\"none\"\ndisplay=\"none\"\nspice_port=\"5930\"\n")
	out, rc := launch(t, nil, c, "--dry-run")
	if rc != 0 {
		t.Fatalf("rc=%d %s", rc, out)
	}
	if !strings.Contains(out, "-machine virt,highmem=on,pflash0=rom,pflash1=efivars") {
		t.Fatalf("aarch64 efi machine: %s", out)
	}
	if strings.Contains(out, "smm=") || strings.Contains(out, "vmport") {
		t.Fatalf("aarch64 must not emit x86-only machine opts: %s", out)
	}
	// boot unset resolves to the upstream "efi" default; only an
	// explicit legacy boot takes the non-EFI branch.
	c2 := richConf(t, "arch=\"aarch64\"\nboot=\"legacy\"\nnetwork=\"none\"\ndisplay=\"none\"\nspice_port=\"5931\"\n")
	out, rc = launch(t, nil, c2, "--dry-run")
	if rc != 0 {
		t.Fatalf("rc=%d %s", rc, out)
	}
	if strings.Contains(out, "pflash0") {
		t.Fatalf("non-efi aarch64 must not emit pflash nodes: %s", out)
	}
}

// virtio-balloon: default for all guests; only dropped for pre-virtio
// macOS releases (os_quirks BALLOON="").
func TestBalloonPerGuest(t *testing.T) {
	c := richConf(t, "")
	out, _ := launch(t, nil, c, "--dry-run")
	if n := strings.Count(out, "-device virtio-balloon"); n != 1 {
		t.Fatalf("linux balloon count=%d: %s", n, out)
	}
	for _, rel := range []struct {
		name string
		want int
	}{
		{"monterey", 1}, {"big-sur", 1}, {"", 0}, {"catalina", 0},
	} {
		extra := "guest_os=\"macos\"\ndisplay=\"none\"\nnetwork=\"none\"\nspice_port=\"5932\"\n"
		if rel.name != "" {
			extra += "macos_release=\"" + rel.name + "\"\n"
		}
		cc := richConf(t, extra)
		dir := filepath.Dir(cc)
		os.WriteFile(filepath.Join(dir, "OVMF_CODE.fd"), make([]byte, 2<<20), 0o644)
		os.WriteFile(filepath.Join(dir, "OVMF_VARS-1920x1080.fd"), make([]byte, 1<<17), 0o644)
		os.WriteFile(filepath.Join(dir, "OpenCore.qcow2"), []byte("qc"), 0o644)
		out, rc := launch(t, nil, cc, "--dry-run")
		if rc != 0 {
			t.Fatalf("macos %s rc=%d %s", rel.name, rc, out)
		}
		if n := strings.Count(out, "-device virtio-balloon"); n != rel.want {
			t.Fatalf("macos %s balloon count=%d want %d: %s", rel.name, n, rel.want, out)
		}
	}
}

// vm_boot warns (stdout) on unrecognised guest_os values.
func TestGuestOsUnknownWarning(t *testing.T) {
	c := richConf(t, "guest_os=\"plan9\"\n")
	out, rc := launch(t, nil, c, "--dry-run")
	if rc != 0 {
		t.Fatalf("rc=%d %s", rc, out)
	}
	if !strings.Contains(out, "not a recognised guest_os value") {
		t.Fatalf("missing guest_os warning: %s", out)
	}
}

// configure_ram min_ram (launch.sh): auto-tiered RAM below the guest
// minimum is fatal; an explicit-but-low ram= only warns.
func TestRamMinGuestGate(t *testing.T) {
	// Explicit low RAM on Windows warns but does not abort.
	warn := captureStdout(t, func() {
		_ = ramArgs(Conf{"ram": "2G"}, "windows", "vm.conf")
	})
	if !strings.Contains(warn, "WARNING! You have allocated less than the recommended amount of RAM to run Windows.") {
		t.Fatalf("windows low-ram warning: %q", warn)
	}
	// macOS with sufficient explicit RAM: no warning, no abort.
	ok := captureStdout(t, func() {
		_ = ramArgs(Conf{"ram": "8G"}, "macos", "vm.conf")
	})
	if strings.Contains(ok, "WARNING") || strings.Contains(ok, "ERROR") {
		t.Fatalf("macos 8G should be silent: %q", ok)
	}
	// Non-restricted guests never warn.
	lx := captureStdout(t, func() {
		_ = ramArgs(Conf{"ram": "1G"}, "linux", "vm.conf")
	})
	if strings.Contains(lx, "WARNING") {
		t.Fatalf("linux should not warn: %q", lx)
	}
	// Non-G values are uncomparable — the gate must skip (Bash
	//  errors on -lt and the check is bypassed).
	m := captureStdout(t, func() {
		_ = ramArgs(Conf{"ram": "4096M"}, "windows", "vm.conf")
	})
	if strings.Contains(m, "WARNING") || strings.Contains(m, "ERROR") {
		t.Fatalf("4096M should bypass the gate: %q", m)
	}
}

// configure_ram fatal path: an auto-tiered RAM below min_ram exits 1
// (Bash ). Runs ramArgs in a subprocess so the exit is
// observable; hostTotalMemMB is stubbed to a low-RAM host.
func TestRamMinAutoFatal(t *testing.T) {
	if os.Getenv("QE_RAM_FATAL_CHILD") == "1" {
		hostTotalMemMB = func() int { return 2 * 1024 } // 2 GiB host
		_ = ramArgs(Conf{}, "windows", "vm.conf")
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestRamMinAutoFatal$")
	cmd.Env = append(os.Environ(), "QE_RAM_FATAL_CHILD=1")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected exit 1, got 0: %s", out)
	}
	if !strings.Contains(string(out), "insufficient RAM to run Windows") {
		t.Fatalf("missing ERROR text: %s", out)
	}
}

// Regression: a shadowed code variable inside biosArgs once emitted
// file=<empty> even though firmwarePair resolved the pair — the
// resolved path must reach the pflash drive.
func TestBiosArgsResolvedCodePath(t *testing.T) {
	fw := t.TempDir()
	code := filepath.Join(fw, "OVMF_CODE_4M.fd")
	os.WriteFile(code, []byte{1}, 0o644)
	os.WriteFile(filepath.Join(fw, "OVMF_VARS_4M.fd"), []byte{1}, 0o644)
	t.Setenv("QE_FIRMWARE_DIR", fw)
	c := Conf{"guest_os": "linux", "_vmdir": t.TempDir()}
	a := biosArgs(c, "linux", "x86_64")
	if !has(a, "if=pflash,format=raw,unit=0,file="+qemuOptEsc(code)+",readonly=on") {
		t.Fatalf("pflash unit0 must carry the resolved code path: %v", a)
	}
}

// configure_bios resolves an EFI_CODE symlink with a warning; the
// emitted pflash file= gets the realpath, not the link.
func TestBiosArgsSymlinkResolution(t *testing.T) {
	fw := t.TempDir()
	real := filepath.Join(fw, "real_code.fd")
	os.WriteFile(real, []byte{1}, 0o644)
	link := filepath.Join(fw, "OVMF_CODE_4M.fd")
	os.Symlink(real, link)
	os.WriteFile(filepath.Join(fw, "OVMF_VARS_4M.fd"), []byte{1}, 0o644)
	t.Setenv("QE_FIRMWARE_DIR", fw)
	c := Conf{"guest_os": "linux", "_vmdir": t.TempDir()}
	out := captureStdout(t, func() {
		a := biosArgs(c, "linux", "x86_64")
		if !has(a, "file="+qemuOptEsc(real)) {
			t.Fatalf("pflash must use the resolved path: %v", a)
		}
	})
	if !strings.Contains(out, "is a symlink") {
		t.Fatalf("missing symlink warning: %s", out)
	}
}

// configure_bios exits when no EFI firmware resolves — also on
// --dry-run (only the vars copy is dry-run gated). A stub qemu binary
// relocates the share root so every search dir misses deterministically.
func TestParamCheckFirmwareMissing(t *testing.T) {
	bin := t.TempDir()
	os.WriteFile(filepath.Join(bin, "qemu-system-x86_64"), []byte("#!/bin/sh\n"), 0o755)
	t.Setenv("PATH", bin)
	t.Setenv("QE_FIRMWARE_DIR", t.TempDir())
	c := Conf{"guest_os": "linux", "_vmdir": t.TempDir()}
	if err := paramCheckFirmware(c, "linux", "x86_64"); err == nil ||
		!strings.Contains(err.Error(), "no EFI firmware found") {
		t.Fatalf("missing OVMF must error: %v", err)
	}
	c["secureboot"] = "on"
	if err := paramCheckFirmware(c, "linux", "x86_64"); err == nil ||
		!strings.Contains(err.Error(), "SecureBoot capable") {
		t.Fatalf("missing secboot firmware must error: %v", err)
	}
	// Legacy boot needs no firmware — no check at all.
	if err := paramCheckFirmware(Conf{"guest_os": "linux", "boot": "legacy"}, "linux", "x86_64"); err != nil {
		t.Fatalf("legacy boot must skip the firmware check: %v", err)
	}
	// aarch64 likewise errors without an AAVMF pair.
	if err := paramCheckFirmware(Conf{"guest_os": "linux"}, "linux", "aarch64"); err == nil {
		t.Fatal("missing AAVMF must error")
	}
}
