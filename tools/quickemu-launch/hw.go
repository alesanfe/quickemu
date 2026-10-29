// hw.go: cpu/ram/bios/network arg building — migrates configure_cpu,
// configure_ram, configure_bios, configure_network from launch.sh.
package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

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

func cpuArgs(c Conf, osName, arch string, t OsTuning) []string {
	cores := c.get("cpu_cores", "")
	if cores == "" {
		n := hostNCores()
		switch {
		case n >= 32:
			cores = "16"
		case n >= 16:
			cores = "8"
		case n >= 8:
			cores = "4"
		case n >= 4:
			cores = "2"
		default:
			cores = "1"
		}
	}
	cpu := "host"
	if strings.HasPrefix(t.CPUFlags, "@") {
		cpu = strings.TrimPrefix(t.CPUFlags, "@") // qemu32 etc: model replaces
	} else {
		cpu += t.CPUFlags
	}
	var a []string
	a = append(a, "-cpu", cpu)
	n := 2
	fmt.Sscanf(cores, "%d", &n)
	threads := 1
	if n >= 2 && hostSMT() {
		threads = 2
	}
	a = append(a, "-smp", fmt.Sprintf("cores=%d,threads=%d,sockets=1", n/threads, threads))
	return a
}

func ramArgs(c Conf) []string {
	ram := c.get("ram", "4G")
	a := []string{"-m", ram}
	if c.get("balloon", "") == "true" {
		a = append(a, "-device", "virtio-balloon")
	}
	return a
}

// firmwareFile resolves an OVMF/AAVMF firmware blob — QE_FIRMWARE_DIR
// (nix/deb packaging) overrides the distro search paths, then the
// usual OVMF and edk2 locations.
func firmwareFile(names ...string) string {
	if d := os.Getenv("QE_FIRMWARE_DIR"); d != "" {
		for _, n := range names {
			if st, err := os.Stat(filepath.Join(d, n)); err == nil && !st.IsDir() {
				return filepath.Join(d, n)
			}
		}
	}
	for _, dir := range []string{"/usr/share/OVMF", "/usr/share/edk2/ovmf",
		"/usr/share/AAVMF", "/usr/share/edk2/aarch64"} {
		for _, n := range names {
			if st, err := os.Stat(filepath.Join(dir, n)); err == nil && !st.IsDir() {
				return filepath.Join(dir, n)
			}
		}
	}
	// Fall back to the first name under /usr/share/OVMF so the QEMU
	// error names the missing file (dry-run needs no real file).
	for _, n := range names {
		if strings.HasSuffix(n, "_CODE.fd") || strings.Contains(n, "CODE") {
			return "/usr/share/OVMF/" + n
		}
	}
	return "/usr/share/OVMF/" + names[0]
}

// seedEFIVars: configure_bios parity — copy the OVMF_VARS template
// into the vmdir so QEMU's writable pflash unit=1 has a real file.
// Idempotent: re-seeding a live VM's vars would reset the boot config,
// so an existing file wins.
func seedEFIVars(c Conf) {
	if c["_run"] != "1" {
		return
	}
	vars := filepath.Join(c["_vmdir"], "OVMF_VARS.fd")
	if _, err := os.Stat(vars); err == nil {
		return
	}
	tmpl := firmwareFile("OVMF_VARS_4M.fd", "OVMF_VARS.fd")
	if confOn(c, "secureboot") {
		if sb := firmwareFile("OVMF_VARS_4M.ms.fd", "OVMF_VARS.secboot.fd",
			"OVMF_VARS.fd"); sb != tmpl {
			tmpl = sb
		}
	}
	b, err := os.ReadFile(tmpl)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: no OVMF_VARS template found (looked in QE_FIRMWARE_DIR + /usr/share/OVMF*)\n")
		return
	}
	if err := os.WriteFile(vars, b, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not seed %s: %v\n", vars, err)
	}
}

func biosArgs(c Conf, osName, arch string) []string {
	var a []string
	if arch == "aarch64" {
		a = append(a, "-bios", firmwareFile("AAVMF_CODE.fd", "QEMU_EFI.fd"))
		return a
	}
	efi := confOn(c, "efi") || strings.Contains(c.get("boot", ""), "efi") || osName == "windows" || osName == "macos"
	if !efi {
		return a
	}
	// Bash configure_bios: OVMF uses TWO pflash drives — CODE (unit=0,
	// readonly) + VARS (unit=1, writable per-VM copy) — plus the secure
	// pflash global. The vars file is copied into the vmdir at launch.
	seedEFIVars(c)
	vars := dotdir(c["_vmdir"]) + "OVMF_VARS.fd"
	a = append(a, "-global", "driver=cfi.pflash01,property=secure,value=on")
	if confOn(c, "secureboot") {
		a = append(a, "-drive", "if=pflash,format=raw,unit=0,file="+
			qemuOptEsc(firmwareFile("OVMF_CODE_4M.secboot.fd", "OVMF_CODE.secboot.fd",
				"OVMF_CODE.fd"))+",readonly=on")
	} else {
		a = append(a, "-drive", "if=pflash,format=raw,unit=0,file="+
			qemuOptEsc(firmwareFile("OVMF_CODE_4M.fd", "OVMF_CODE.fd"))+",readonly=on")
	}
	a = append(a, "-drive", "if=pflash,format=raw,unit=1,file="+qemuOptEsc(vars))
	// swtpm_socket_ready: emit the chardev only when the control socket
	// will exist — always on --dry-run, only after startSwtpm on --run.
	if confOn(c, "tpm") && (c["_run"] == "" || c["_swtpm"] == "1") {
		sock := dotdir(c["_vmdir"]) + c["_vmname"] + ".swtpm-sock"
		if arch == "aarch64" {
			a = append(a, "-chardev", "socket,id=chrtpm,path="+qemuOptEsc(sock),
				"-tpmdev", "emulator,id=tpm0,chardev=chrtpm",
				"-device", "tpm-tis-device,tpmdev=tpm0")
		} else {
			a = append(a, "-chardev", "socket,id=chrtpm,path="+qemuOptEsc(sock),
				"-tpmdev", "emulator,id=tpm0,chardev=chrtpm",
				"-device", "tpm-tis,tpmdev=tpm0")
		}
	}
	return a
}

func networkArgs(c Conf, t OsTuning) []string {
	// vm.sh network emit parity:
	//   network=""      -> user: -device D,netdev=nic[M] -netdev user,hostname=VM[fwd],id=nic
	//   network=none    -> -nic none
	//   network=restrict-> -device D,netdev=nic[M] -netdev user,hostname=VM,restrict=y,id=nic
	//   network=passt   -> -device D,netdev=nic[M] -netdev passt,id=nic
	//   network=<X>     -> bridge: -nic bridge,br=X,helper=<h>,model=D[M]
	// 'bridge' conf key overrides network (vm.sh line ~961).
	net := c.get("network", "")
	if b := c.get("bridge", ""); b != "" {
		net = b
	}
	vm := c["_vmname"]
	mac := ""
	if m := c.get("macaddr", c.get("mac_addr", "")); m != "" && net != "none" {
		mac = ",mac=" + qemuOptEsc(m)
	}
	var a []string
	switch net {
	case "none":
		a = []string{"-nic", "none"}
	case "restrict":
		a = []string{"-device", t.NetDevice + ",netdev=nic" + mac,
			"-netdev", "user,hostname=" + qemuOptEsc(vm) + ",restrict=y,id=nic"}
	case "passt":
		a = []string{"-device", t.NetDevice + ",netdev=nic" + mac,
			"-netdev", "passt,id=nic"}
	case "":
		// user mode: hostfwd appended to the user netdev.
		// configure_ports auto-allocates ssh_port 22220-22229 when unset.
		fwd := ""
		if p := firstFreePort(c.get("ssh_port", ""), 22220, 22229); p != "" {
			fwd = ",hostfwd=tcp:127.0.0.1:" + qemuOptEsc(p) + "-:22"
			portsRecord(c["_vmdir"], c["_vmname"], "ssh,"+p, c["_run"] == "1")
		}
		for _, pf := range strings.Fields(c.get("port_forwards", "")) {
			kv := strings.SplitN(pf, ":", 2)
			if len(kv) == 2 {
				fwd += ",hostfwd=tcp::" + qemuOptEsc(kv[0]) + "-:" + qemuOptEsc(kv[1]) +
					",hostfwd=udp::" + qemuOptEsc(kv[0]) + "-:" + qemuOptEsc(kv[1])
			}
		}
		if pub := c["_public"]; pub != "" {
			fwd += ",smb=" + qemuOptEsc(pub) // Samba share on the user netdev
		}
		a = []string{"-device", t.NetDevice + ",netdev=nic" + mac,
			"-netdev", "user,hostname=" + qemuOptEsc(vm) + fwd + ",id=nic"}
	default:
		// bridge mode: network value IS the interface name
		helper, _ := exec.LookPath("qemu-bridge-helper")
		a = []string{"-nic", "bridge,br=" + qemuOptEsc(net) + ",helper=" + helper + ",model=" + t.NetDevice + mac}
	}

	// Secondary NICs (vm.sh extra_nics): user|restrict|passt|bridge:<iface>,
	// optional parallel extra_macs[i] — one netdev id nicN per entry.
	macs := confList(c, "extra_macs")
	for i, en := range confList(c, "extra_nics") {
		if en == "" {
			continue
		}
		emac := ""
		if i < len(macs) && macs[i] != "" {
			emac = ",mac=" + qemuOptEsc(macs[i])
		}
		switch {
		case strings.HasPrefix(en, "bridge:"):
			helper, _ := exec.LookPath("qemu-bridge-helper")
			a = append(a, "-nic", "bridge,br="+qemuOptEsc(strings.TrimPrefix(en, "bridge:"))+",helper="+helper+",model="+t.NetDevice+emac)
		case en == "user" || en == "restrict" || en == "passt":
			id := fmt.Sprintf("nic%d", i)
			a = append(a, "-device", t.NetDevice+",netdev="+id+emac,
				"-netdev", en+",id="+id)
		default:
			fmt.Fprintf(os.Stderr, " - WARNING! extra_nics[%d] '%s' unknown (user|restrict|bridge:<iface>); skipping.\n", i, en)
		}
	}
	return a
}

// confList parses a bash/TOML array conf value: `("a" "b")`, `[a,b]`
// flattened upstream, or a plain space-separated list.
func confList(c Conf, key string) []string {
	v := strings.TrimSpace(c.get(key, ""))
	if v == "" {
		return nil
	}
	v = strings.TrimPrefix(v, "(")
	v = strings.TrimSuffix(v, ")")
	var out []string
	for _, f := range strings.Fields(v) {
		if f = strings.Trim(f, `"'`); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// portArgs: SSH/SPICE port forwarding — configure_ports. Allocation
// is native (firstFreePort + portsRecord writes the registry file).
func portArgs(c Conf, dir string) []string {
	var a []string
	ssh := c.get("ssh_port", "")
	if ssh != "" {
		// already handled in networkArgs hostfwd — nothing extra
		return a
	}
	spice := c.get("spice_port", "")
	if spice != "" {
		return a // spice args built in displayArgs
	}
	return a
}

// netListen: seam for firstFreePort's TCP probe — tests replace it so
// port scanning stays deterministic on hosts where the scanned range
// is already bound by unrelated processes.
var netListen = net.Listen

// firstFreePort: returns p if set, else the first free TCP port in
// [lo,hi] — get_port(22220,9) parity.
func firstFreePort(p string, lo, hi int) string {
	if p != "" {
		return p
	}
	for port := lo; port <= hi; port++ {
		ln, err := netListen("tcp", fmt.Sprintf(":%d", port))
		if err == nil {
			ln.Close()
			return fmt.Sprint(port)
		}
	}
	return ""
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

// hostCPUVendor: vendor_id from cpuinfo (GenuineIntel|AuthenticAMD|HygonGenuine).
func hostCPUVendor() string {
	b, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return ""
	}
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, "vendor_id") {
			return strings.TrimSpace(strings.SplitN(l, ":", 2)[1])
		}
	}
	return ""
}

// hostHasFlag: vmx/svm/sse4_2/... presence in cpuinfo (check_cpu_flag).
// AMD's invtsc is reported as constant_tsc — same equivalence as Bash.
func hostHasFlag(flag string) bool {
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

// hostTotalMemMB: MemTotal in MiB (host_total_mem_mb).
func hostTotalMemMB() int {
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
