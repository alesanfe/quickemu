// hw.go: cpu/ram/bios/network arg building — migrates configure_cpu,
// configure_ram, configure_bios, configure_network from launch.sh.
package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
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
	n := 2
	fmt.Sscanf(cores, "%d", &n)
	// Windows 11 minimum: 2 cores (issue #1423) — keyed off the VM
	// name, same as Bash.
	if osName == "windows" && n < 2 {
		vm := c["_vmname"]
		if strings.Contains(vm, "windows-11") || strings.Contains(vm, "win11") {
			fmt.Printf(" - CPU:      Adjusting CPU cores from %d to 2 (Windows 11 minimum requirement)\n", n)
			n = 2
		}
	}
	// macOS only boots with power-of-2 core counts — nearest lower one.
	if osName == "macos" {
		for p := 1024; p >= 1; p >>= 1 {
			if p <= n {
				n = p
				break
			}
		}
	}
	cpu := "host"
	if m := os.Getenv("CPU_MODEL"); m != "" {
		cpu = m
	}
	if strings.HasPrefix(t.CPUFlags, "@") {
		cpu = strings.TrimPrefix(t.CPUFlags, "@") // qemu32 etc: model replaces
	} else {
		cpu += t.CPUFlags
	}
	// AMD hosts get topoext for every non-macOS x86_64 guest.
	if arch == "x86_64" && osName != "macos" && hostCPUVendor() == "AuthenticAMD" {
		cpu += ",topoext"
	}
	var a []string
	a = append(a, "-cpu", cpu)
	threads := 1
	if n >= 2 && hostSMT() {
		threads = 2
	}
	// cpu_pinning consumes the logical core count + thread factor later
	// (validate in run, apply post-spawn).
	c["_lcores"] = fmt.Sprint(n / threads)
	c["_threads"] = fmt.Sprint(threads)
	sockets := hostSockets()
	a = append(a, "-smp", fmt.Sprintf("cores=%d,threads=%d,sockets=%d", n/threads, threads, sockets))
	fmt.Printf(" - CPU:      %s\n", hostCPUModel())
	fmt.Printf(" - CPU VM:   %s, %d Socket(s), %d Core(s), %d Thread(s)\n",
		strings.SplitN(cpu, ",", 2)[0], sockets, n/threads, threads)
	return a
}

// hostSockets: HOST_CPU_SOCKETS parity — lscpu "Socket(s)", 1 when
// unresolved.
func hostSockets() int {
	out, err := exec.Command("lscpu").Output()
	if err != nil {
		return 1
	}
	for _, l := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(l, "Socket(s):") {
			n := 1
			fmt.Sscanf(strings.TrimSpace(strings.SplitN(l, ":", 2)[1]), "%d", &n)
			return n
		}
	}
	return 1
}

// hostCPUModel: HOST_CPU_MODEL parity — first "model name" from
// cpuinfo (lscpu Model name on any arch).
func hostCPUModel() string {
	b, err := os.ReadFile("/proc/cpuinfo")
	if err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(l, "model name") {
				return strings.TrimSpace(strings.SplitN(l, ":", 2)[1])
			}
		}
	}
	return ""
}

func ramArgs(c Conf) []string {
	ram := c.get("ram", "")
	if ram == "" {
		// configure_ram tiering on host GiB: ≥128→32G, ≥64→16G,
		// ≥16→8G, ≥8→4G, else 2G.
		ram = "2G"
		switch gb := hostTotalMemMB() / 1024; {
		case gb >= 128:
			ram = "32G"
		case gb >= 64:
			ram = "16G"
		case gb >= 16:
			ram = "8G"
		case gb >= 8:
			ram = "4G"
		}
	}
	a := []string{"-m", ram}
	if c.get("balloon", "") == "true" {
		a = append(a, "-device", "virtio-balloon")
	}
	fmt.Printf(" - RAM VM:   %s RAM\n", ram)
	return a
}

// firmwareDirs: the search roots Bash builds via SHARE_PATH
// (get_qemu_share_path). QE_FIRMWARE_DIR (nix/deb packaging) wins.
func firmwareDirs() []string {
	var dirs []string
	if d := os.Getenv("QE_FIRMWARE_DIR"); d != "" {
		dirs = append(dirs, d)
	}
	share := "/usr/share"
	if q, err := exec.LookPath("qemu-system-x86_64"); err == nil {
		if rp, err := filepath.EvalSymlinks(q); err == nil {
			share = filepath.Join(filepath.Dir(filepath.Dir(rp)), "share")
		}
	}
	for _, d := range []string{"OVMF", "edk2/ovmf", "OVMF/x64", "edk2-ovmf",
		"edk2-ovmf/x64", "edk2/x64", "qemu", "AAVMF", "edk2/aarch64"} {
		dirs = append(dirs, filepath.Join(share, d))
	}
	return dirs
}

// firmwareFile resolves the first existing firmware blob — kept for
// single-file lookups (aarch64 -bios, the plan-mode fallback).
func firmwareFile(names ...string) string {
	for _, dir := range firmwareDirs() {
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

// firmwarePair: Bash's ovmfs=() ordered pair search — the first
// (code,vars) pair where BOTH files exist in the same directory wins.
// Returns "" when nothing resolves so callers can pick a mode or fail.
func firmwarePair(pairs [][2]string) (code, vars string) {
	for _, dir := range firmwareDirs() {
		for _, p := range pairs {
			c, v := filepath.Join(dir, p[0]), filepath.Join(dir, p[1])
			cOK := false
			if st, err := os.Stat(c); err == nil && !st.IsDir() {
				cOK = true
			}
			vOK := false
			if st, err := os.Stat(v); err == nil && !st.IsDir() {
				vOK = true
			}
			if cOK && vOK {
				return c, v
			}
		}
	}
	return "", ""
}

// isFirmwareQcow2: is_firmware_qcow2 parity — NixOS/packaged EDK2
// ships qcow2-format firmware (magic QFI\xfb); the pflash drive must
// declare format=qcow2 or QEMU treats it as raw.
func isFirmwareQcow2(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	var b [4]byte
	if _, err := io.ReadFull(f, b[:]); err != nil {
		return false
	}
	return b[0] == 0x51 && b[1] == 0x46 && b[2] == 0x49 && b[3] == 0xfb
}

// x86FirmwarePairs: the Bash ovmfs list collapsed to the (code,vars)
// basename pairs present in the Go search roots.
func x86FirmwarePairs(secureboot bool) [][2]string {
	if secureboot {
		return [][2]string{
			{"OVMF_CODE_4M.secboot.fd", "OVMF_VARS_4M.ms.fd"},
			{"OVMF_CODE.secboot.fd", "OVMF_VARS.secboot.fd"},
			{"OVMF_CODE.secboot.fd", "OVMF_VARS.fd"},
			{"ovmf-x86_64-smm-ms-code.bin", "ovmf-x86_64-smm-ms-vars.bin"},
			{"edk2-x86_64-secure-code.fd", "edk2-i386-vars.fd"},
			{"OVMF_CODE.secboot.4m.fd", "OVMF_VARS.4m.fd"},
			{"OVMF_CODE_4M.secboot.qcow2", "OVMF_VARS_4M.secboot.qcow2"},
		}
	}
	return [][2]string{
		{"OVMF_CODE_4M.fd", "OVMF_VARS_4M.fd"},
		{"OVMF_CODE.fd", "OVMF_VARS.fd"},
		{"ovmf-x86_64-4m-code.bin", "ovmf-x86_64-4m-vars.bin"},
		{"edk2-x86_64-code.fd", "edk2-i386-vars.fd"},
		{"OVMF_CODE.4m.fd", "OVMF_VARS.4m.fd"},
		{"OVMF_CODE_4M.qcow2", "OVMF_VARS_4M.qcow2"},
	}
}

// aarch64FirmwarePairs: AAVMF/edk2 pflash pairs for ARM64 guests.
func aarch64FirmwarePairs() [][2]string {
	return [][2]string{
		{"AAVMF_CODE.fd", "AAVMF_VARS.fd"},
		{"QEMU_CODE.fd", "QEMU_VARS.fd"},
		{"QEMU_EFI-pflash.raw", "vars-template-pflash.raw"},
		{"edk2-aarch64-code.fd", "edk2-arm-vars.fd"},
	}
}

// efiVarsName: the in-vmdir vars filename — .qcow2 when the seeded
// template is qcow2 firmware (Bash writes OVMF_VARS.qcow2 then).
func efiVarsName(vmdir, template string) string {
	if template != "" && isFirmwareQcow2(template) {
		return filepath.Join(vmdir, "OVMF_VARS.qcow2")
	}
	return filepath.Join(vmdir, "OVMF_VARS.fd")
}

// seedEFIVars: configure_bios + efi_vars parity — copy the VARS
// template into the vmdir so QEMU's writable pflash unit=1 has a real
// file. Idempotent: re-seeding a live VM's vars would reset the boot
// config, so an existing file wins. Returns the vars path used.
func seedEFIVars(c Conf, template string) string {
	vmdir := c["_vmdir"]
	vars := efiVarsName(vmdir, template)
	if c["_run"] != "1" {
		return vars
	}
	// Legacy migrations (configure_bios): a prior release stored the
	// vars under the VM name or the 4M suffix — rename, don't reseed.
	if _, err := os.Stat(vars); err != nil {
		for _, old := range []string{c["_vmname"] + "-vars.fd", "OVMF_VARS_4M.fd"} {
			op := filepath.Join(vmdir, old)
			if _, err := os.Stat(op); err == nil {
				os.Rename(op, vars)
				break
			}
		}
	}
	if _, err := os.Stat(vars); err == nil {
		return vars
	}
	if template == "" {
		fmt.Fprintf(os.Stderr, "warning: no OVMF_VARS template found (looked in QE_FIRMWARE_DIR + /usr/share/OVMF*)\n")
		return vars
	}
	b, err := os.ReadFile(template)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not read %s: %v\n", template, err)
		return vars
	}
	if err := os.WriteFile(vars, b, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not seed %s: %v\n", vars, err)
	}
	return vars
}

// paramCheckFirmware: the fatal half of configure_bios — macOS needs
// the quickget-downloaded OVMF + OpenCore/ESP bootloader, and the
// files are size-checked (truncated downloads produce unusable pflash).
// For generic EFI guests the seeded vars must be a writeable file.
func paramCheckFirmware(c Conf, arch string) error {
	if c.get("guest_os", "") == "macos" {
		vmdir := c["_vmdir"]
		code := filepath.Join(vmdir, "OVMF_CODE.fd")
		vars := ""
		for _, n := range []string{"OVMF_VARS-1024x768.fd", "OVMF_VARS-1920x1080.fd"} {
			if _, err := os.Stat(filepath.Join(vmdir, n)); err == nil {
				vars = filepath.Join(vmdir, n)
				break
			}
		}
		missing := ""
		if _, err := os.Stat(code); err != nil || vars == "" {
			missing = "Firmware"
		}
		bl := ""
		for _, n := range []string{"OpenCore.qcow2", "ESP.qcow2"} {
			if _, err := os.Stat(filepath.Join(vmdir, n)); err == nil {
				bl = filepath.Join(vmdir, n)
				break
			}
		}
		if bl == "" {
			missing = "Bootloader"
		}
		if missing != "" {
			return fmt.Errorf("ERROR! macOS %s was not found.\n       Use 'quickget' to download the required files.", missing)
		}
		if fi, err := os.Stat(code); err == nil && fi.Size() < 1048576 {
			return fmt.Errorf("ERROR! macOS firmware in %s appears to be corrupt or incomplete.\n       Delete OVMF_CODE.fd and OVMF_VARS*.fd, then re-run quickget.", vmdir)
		}
		if fi, err := os.Stat(vars); err == nil && fi.Size() < 131072 {
			return fmt.Errorf("ERROR! macOS firmware in %s appears to be corrupt or incomplete.\n       Delete OVMF_CODE.fd and OVMF_VARS*.fd, then re-run quickget.", vmdir)
		}
		return nil
	}
	// EFI guests: the vars file in the vmdir must be a writeable
	// regular file once it exists — Bash deletes it and asks for a
	// re-run (it is a seeded copy, re-seeding is safe).
	if c["_run"] == "1" {
		for _, name := range []string{"OVMF_VARS.fd", "OVMF_VARS.qcow2"} {
			vars := filepath.Join(c["_vmdir"], name)
			fi, err := os.Stat(vars)
			if err != nil {
				continue
			}
			f, err := os.OpenFile(vars, os.O_WRONLY, 0)
			if !fi.Mode().IsRegular() || err != nil {
				os.Remove(vars)
				return fmt.Errorf(" - EFI:      ERROR! %s is not a regular file or not writeable.\n             Deleting %s. Please re-run quickemu.", vars, vars)
			}
			f.Close()
		}
	}
	return nil
}

func biosArgs(c Conf, osName, arch string) []string {
	var a []string
	vmdir := c["_vmdir"]
	mac := osName == "macos"
	efi := confOn(c, "efi") || strings.Contains(c.get("boot", ""), "efi") ||
		osName == "windows" || mac
	if !efi {
		fmt.Printf(" - BOOT:     Legacy BIOS (%s)\n", title(osName))
		return a
	}
	if arch == "aarch64" {
		// Fedora's monolithic EDK2 for Windows ARM64 — loaded via -bios,
		// no persistent pflash store (configure_bios EFI_MODE=bios).
		if osName == "windows" && !confOn(c, "secureboot") {
			if f := firmwareFile("edk2/aarch64/QEMU_EFI.fd", "QEMU_EFI.fd"); f != "" {
				fmt.Printf(" - BOOT:     EFI (%s), firmware (%s), mode (-bios), SecureBoot (off).\n",
					title(osName), f)
				return []string{"-bios", f}
			}
		}
		code, tmpl := firmwarePair(aarch64FirmwarePairs())
		vars := seedEFIVars(c, tmpl)
		if code == "" {
			code = tmpl
			if code == "" {
				code = "/usr/share/AAVMF/AAVMF_CODE.fd"
			}
		}
		fmt.Printf(" - BOOT:     EFI (%s), OVMF (%s), EFI Vars (%s), SecureBoot (%s).\n",
			title(osName), code, vars, c.get("secureboot", "off"))
		// ARM64 pflash uses named blockdev nodes on the virt machine —
		// no cfi.pflash01 secure global (x86 SMM-specific).
		return []string{
			"-blockdev", "node-name=rom,driver=file,filename=" + qemuOptEsc(code) + ",read-only=true",
			"-blockdev", "node-name=efivars,driver=file,filename=" + qemuOptEsc(vars),
		}
	}
	var code, vars string
	if mac {
		// macOS uses the OSX-KVM firmware quickget downloaded into the
		// vmdir (paramCheckFirmware already enforced presence/sizes).
		code = filepath.Join(vmdir, "OVMF_CODE.fd")
		vars = filepath.Join(vmdir, "OVMF_VARS-1024x768.fd")
		if _, err := os.Stat(vars); err != nil {
			vars = filepath.Join(vmdir, "OVMF_VARS-1920x1080.fd")
		}
	} else {
		// Bash configure_bios: OVMF uses TWO pflash drives — CODE
		// (unit=0, readonly) + VARS (unit=1, writable per-VM copy).
		code, tmpl := firmwarePair(x86FirmwarePairs(confOn(c, "secureboot")))
		if code == "" {
			// Plan-mode fallback so the emitted argv still names a file.
			if confOn(c, "secureboot") {
				code = firmwareFile("OVMF_CODE_4M.secboot.fd", "OVMF_CODE.secboot.fd", "OVMF_CODE.fd")
			} else {
				code = firmwareFile("OVMF_CODE_4M.fd", "OVMF_CODE.fd")
			}
		}
		vars = seedEFIVars(c, tmpl)
	}
	codeFmt, varsFmt := "raw", "raw"
	if isFirmwareQcow2(code) {
		codeFmt = "qcow2"
	}
	if isFirmwareQcow2(vars) {
		varsFmt = "qcow2"
	}
	if mac {
		fmt.Printf(" - BOOT:     EFI (macOS), OVMF (%s), SecureBoot (%s).\n",
			filepath.Base(code), c.get("secureboot", "off"))
	} else {
		fmt.Printf(" - BOOT:     EFI (%s), OVMF (%s), EFI Vars (%s), SecureBoot (%s).\n",
			title(osName), code, vars, c.get("secureboot", "off"))
	}
	a = append(a, "-global", "driver=cfi.pflash01,property=secure,value=on")
	a = append(a, "-drive", "if=pflash,format="+codeFmt+",unit=0,file="+qemuOptEsc(code)+",readonly=on")
	a = append(a, "-drive", "if=pflash,format="+varsFmt+",unit=1,file="+qemuOptEsc(vars))
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
	case "restrict", "", "slirp":
		// user + restrict modes both take hostfwd (launch.sh): restrict
		// isolates the guest but forwards still reach it. configure_ports
		// auto-allocates ssh_port 22220-22229 when unset.
		fwd := ""
		if p := firstFreePort(c.get("ssh_port", ""), 22220, 22229); p != "" {
			// ssh_access picks the bind address (local→loopback,
			// remote→wildcard, otherwise a specific interface IP).
			addr, sshHost := "127.0.0.1", "localhost"
			switch sa := c.get("ssh_access", ""); sa {
			case "", "local":
			case "remote":
				addr = ""
			default:
				addr, sshHost = sa, sa
			}
			fwd = ",hostfwd=tcp:" + addr + ":" + qemuOptEsc(p) + "-:22"
			portsRecord(c["_vmdir"], c["_vmname"], "ssh,"+p, c["_run"] == "1")
			fmt.Printf(" - ssh:      On host:  ssh user@%s -p %s\n", sshHost, p)
		} else {
			fmt.Println(" - ssh:      All ssh ports have been exhausted.")
		}
		if pfs := strings.Fields(c.get("port_forwards", "")); len(pfs) > 0 {
			fmt.Println(" - PORTS:    Port forwards requested:")
			for _, pf := range pfs {
				kv := strings.SplitN(pf, ":", 2)
				if len(kv) == 2 {
					fmt.Printf("              - %s => %s\n", kv[0], kv[1])
					fwd += ",hostfwd=tcp::" + qemuOptEsc(kv[0]) + "-:" + qemuOptEsc(kv[1]) +
						",hostfwd=udp::" + qemuOptEsc(kv[0]) + "-:" + qemuOptEsc(kv[1])
				}
			}
		}
		if net == "restrict" {
			fwd += ",restrict=y"
		} else if pub := c["_public"]; pub != "" {
			fwd += ",smb=" + qemuOptEsc(pub) // Samba share on the user netdev
		}
		a = []string{"-device", t.NetDevice + ",netdev=nic" + mac,
			"-netdev", "user,hostname=" + qemuOptEsc(vm) + fwd + ",id=nic"}
	case "passt":
		a = []string{"-device", t.NetDevice + ",netdev=nic" + mac,
			"-netdev", "passt,id=nic"}
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

// parseCPUPinning: the Bash ^[0-9]+(,[0-9]+)*$ validation — a
// comma-separated list of host CPUs, one per guest logical core.
func parseCPUPinning(v string) ([]int, error) {
	if v == "" {
		return nil, nil
	}
	var out []int
	for _, f := range strings.Split(v, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(f))
		if err != nil || n < 0 {
			return nil, fmt.Errorf(" - ERROR!  Couldn't parse CPU pinning: '%s', only comma-separated list is supported", v)
		}
		out = append(out, n)
	}
	return out, nil
}

// paramCheckPinning: vm.sh validates the count against
// GUEST_CPU_LOGICAL_CORES during arg assembly (Linux only).
func paramCheckPinning(c Conf) error {
	v := c.get("cpu_pinning", "")
	if v == "" || runtime.GOOS != "linux" {
		return nil
	}
	m, err := parseCPUPinning(v)
	if err != nil {
		return err
	}
	lc := 1
	fmt.Sscanf(c["_lcores"], "%d", &lc)
	if len(m) != lc {
		return fmt.Errorf(" - ERROR!  Number of host cores for pinning should be equal to VM core count (%d != %d)", len(m), lc)
	}
	return nil
}

// applyCPUPinning: configure_cpu_pinning — walks the QEMU task list
// for "CPU <n>/KVM" vCPU threads and pins each to its mapped host CPU
// via taskset (threads of one core share the host CPU).
func applyCPUPinning(pid int, c Conf) {
	mapping, err := parseCPUPinning(c.get("cpu_pinning", ""))
	if err != nil || len(mapping) == 0 || runtime.GOOS != "linux" {
		return
	}
	threads := 1
	fmt.Sscanf(c["_threads"], "%d", &threads)
	tids, err := filepath.Glob(fmt.Sprintf("/proc/%d/task/*", pid))
	if err != nil {
		return
	}
	sort.Strings(tids)
	var guestCPUs []string
	idx := 0
	for _, td := range tids {
		comm, err := os.ReadFile(filepath.Join(td, "comm"))
		if err != nil || !strings.HasPrefix(string(comm), "CPU") {
			continue
		}
		coreIdx := idx / threads
		if coreIdx < len(mapping) {
			exec.Command("taskset", "-cp", fmt.Sprint(mapping[coreIdx]),
				filepath.Base(td)).Run()
		}
		if idx%threads == 0 {
			guestCPUs = append(guestCPUs, fmt.Sprint(coreIdx))
		}
		idx++
	}
	if idx > 0 {
		fmt.Printf(" - CPU Pinning: Bind guest cores to host cores (%s -> %s)\n",
			strings.Join(guestCPUs, ","), c.get("cpu_pinning", ""))
	}
}

// title: Bash ${var^} — capitalize the first letter (guest names in
// the BOOT status line).
func title(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
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
