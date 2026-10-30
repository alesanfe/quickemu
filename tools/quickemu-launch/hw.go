// hw.go: cpu/ram/bios/network arg building — migrates configure_cpu,
// configure_ram, configure_bios, configure_network from launch.sh.
//
// Error text in this file is the Bash user-facing contract: it is
// printed verbatim to stdout, so it keeps its punctuation.
//
//lint:file-ignore ST1005 Bash parity error strings keep punctuation.
package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

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

func ramArgs(c Conf, osName, vm string) []string {
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

	// configure_ram min_ram: auto-tiered RAM below the guest minimum
	// is fatal; an explicit-but-low ram= only warns. ${RAM_VM//G/}
	// strips G suffixes only — non-G values are not comparable in
	// Bash either (the -lt test errors and the gate skips).
	var osPretty string
	var minRAM int
	switch osName {
	case "windows", "windows-server":
		osPretty, minRAM = "Windows", 4
	case "macos":
		osPretty, minRAM = "macOS", 8
	}
	if minRAM > 0 {
		if n, err := strconv.Atoi(strings.ReplaceAll(ram, "G", "")); err == nil && n < minRAM {
			if c.get("ram", "") == "" {
				fmt.Printf("             ERROR! The guest virtual machine has been allocated insufficient RAM to run %s.\n", osPretty)
				fmt.Printf("             You can override the guest RAM allocation by adding 'ram=%dG' to %s\n", minRAM, vm)
				os.Exit(1)
			}
			fmt.Printf("             WARNING! You have allocated less than the recommended amount of RAM to run %s.\n", osPretty)
		}
	}
	return a
}

// resolvedBoot: the conf `boot` value with the upstream "efi" default —
// vm_boot lowercases it, and configure_bios forces macos to efi before
// the *efi* glob decides the branch.
func resolvedBoot(c Conf, osName string) string {
	if osName == "macos" {
		return "efi"
	}
	return strings.ToLower(c.get("boot", "efi"))
}

func biosArgs(c Conf, osName, arch string) []string {
	var a []string
	vmdir := c["_vmdir"]
	mac := osName == "macos"
	efi := strings.Contains(resolvedBoot(c, osName), "efi")
	if !efi {
		fmt.Printf(" - BOOT:     Legacy BIOS (%s)\n", title(osName))
		return a
	}
	if arch == "aarch64" {
		// Fedora's monolithic EDK2 for Windows ARM64 — loaded via -bios,
		// no persistent pflash store (configure_bios EFI_MODE=bios).
		if f := a64MonolithicEDK2(c, osName); f != "" {
			fmt.Printf(" - BOOT:     EFI (%s), firmware (%s), mode (-bios), SecureBoot (off).\n",
				title(osName), f)
			return []string{"-bios", f}
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
		// paramCheckFirmware already exits when no pair resolves, so
		// the firmwareFile fallback only fires for direct callers.
		var tmpl string
		code, tmpl = firmwarePair(x86FirmwarePairs(confOn(c, "secureboot")))
		if code == "" {
			if confOn(c, "secureboot") {
				code = firmwareFile("OVMF_CODE_4M.secboot.fd", "OVMF_CODE.secboot.fd", "OVMF_CODE.fd")
			} else {
				code = firmwareFile("OVMF_CODE_4M.fd", "OVMF_CODE.fd")
			}
		}
		vars = seedEFIVars(c, tmpl)
	}
	// configure_bios resolves an EFI_CODE symlink before emitting —
	// QEMU gets the real file, with a warning echo.
	if fi, err := os.Lstat(code); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		fmt.Printf(" - EFI:      WARNING! %s is a symlink.\n", code)
		fmt.Print("             Resolving to... ")
		if rp, err := filepath.EvalSymlinks(code); err == nil {
			code = rp
		}
		fmt.Println(code)
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
	return a
}

// tpmArgs: the swtpm chardev emit — vm_boot_passthru emits it after the
// vfio block. swtpm_socket_ready: emit only when the control socket
// will exist — always on --dry-run, only after startSwtpm on --run.
func tpmArgs(c Conf, arch string) []string {
	if !confOn(c, "tpm") || (c["_run"] != "" && c["_swtpm"] != "1") {
		return nil
	}
	sock := dotdir(c["_vmdir"]) + c["_vmname"] + ".swtpm-sock"
	dev := "tpm-tis,tpmdev=tpm0"
	if arch == "aarch64" {
		dev = "tpm-tis-device,tpmdev=tpm0"
	}
	return []string{"-chardev", "socket,id=chrtpm,path=" + qemuOptEsc(sock),
		"-tpmdev", "emulator,id=tpm0,chardev=chrtpm",
		"-device", dev}
}

// planPorts: configure_ports parity — resolves the SSH hostfwd, the
// port_forwards list, the restrict flag and the Samba share into the
// c["_fwd"] netdev suffix, printing the -ssh:/-PORTS: echoes. Called
// during the configure phase so the echoes precede the emit phase.
func planPorts(c Conf, t OsTuning) {
	net := c.get("network", "")
	if b := c.get("bridge", ""); b != "" {
		net = b
	}
	if net != "" && net != "restrict" && net != "slirp" {
		return // none/bridge/passt have no user-netdev forwards
	}
	fwd := ""
	if p := firstFreePort(c.get("ssh_port", ""), 22220, 22229, c["_vmname"]); p != "" {
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
		// qe_die "vm.port" PORT_BUSY — auto-allocation found no
		// free port in 22220-22229: the launch cannot proceed.
		fmt.Fprintf(os.Stderr, "ERROR! No free SSH forward port (range 22220-22229).\n")
		os.Exit(1)
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
	// configure_file_sharing appends smb= to NET for every mode —
	// restrict included — and the emit puts ,restrict=y last:
	// user,hostname=VM,hostfwd...,smb=...,restrict=y
	if pub := c["_public"]; pub != "" && smbdPresent() {
		fwd += ",smb=" + qemuOptEsc(pub) // Samba share on the user netdev
	}
	if net == "restrict" {
		fwd += ",restrict=y"
	}
	c["_fwd"] = fwd
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
	if m := c.get("macaddr", c.get("mac_addr", "")); m != "" {
		if net == "none" {
			fmt.Printf(" - WARNING!  macaddr '%s' is ignored because networking is disabled.\n", m)
		} else {
			mac = ",mac=" + qemuOptEsc(m)
		}
	}
	var a []string
	switch net {
	case "none":
		fmt.Printf(" - Network:  Disabled\n")
		a = []string{"-nic", "none"}
	case "restrict", "":
		// fwd is resolved in planPorts at the configure_ports position;
		// callers that skipped it (tests) get the lazy resolve here.
		if _, ok := c["_fwd"]; !ok {
			planPorts(c, t)
		}
		fwd := c["_fwd"]
		if net == "restrict" {
			fmt.Printf(" - Network:  Restricted (%s)\n", t.NetDevice)
		} else {
			fmt.Printf(" - Network:  User (%s)\n", t.NetDevice)
		}
		a = []string{"-device", t.NetDevice + ",netdev=nic" + mac,
			"-netdev", "user,hostname=" + qemuOptEsc(vm) + fwd + ",id=nic"}
	case "passt":
		fmt.Printf(" - Network:  passt (%s)\n", t.NetDevice)
		if v := qemuVersionShort(c.get("arch", "x86_64")); v >= 0 && v < 101 {
			fmt.Printf("ERROR! 'passt' networking requires QEMU 10.1 or newer.\n")
			os.Exit(1)
		}
		a = []string{"-device", t.NetDevice + ",netdev=nic" + mac,
			"-netdev", "passt,id=nic"}
	default:
		// bridge mode: network value IS the interface name
		fmt.Printf(" - Network:  Bridged (%s)\n", net)
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
			iface := strings.TrimPrefix(en, "bridge:")
			fmt.Printf(" - Network:  Bridged NIC%d (%s)\n", i, iface)
			helper, _ := exec.LookPath("qemu-bridge-helper")
			a = append(a, "-nic", "bridge,br="+qemuOptEsc(iface)+",helper="+helper+",model="+t.NetDevice+emac)
		case en == "user" || en == "restrict" || en == "passt":
			id := fmt.Sprintf("nic%d", i)
			fmt.Printf(" - Network:  %s NIC%d (%s)\n", en, i, t.NetDevice)
			a = append(a, "-device", t.NetDevice+",netdev="+id+emac,
				"-netdev", en+",id="+id)
		default:
			fmt.Printf(" - WARNING! extra_nics[%d] '%s' unknown (user|restrict|bridge:<iface>); skipping.\n", i, en)
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

// qemuVersionShort: QEMU_VER_SHORT parity — major and minor digits
// concatenated, patch dropped (6.0 → 60, 6.5 → 65, 10.1 → 101).
// Unresolvable binary → -1 so version gates stay permissive.
func qemuVersionShort(arch string) int {
	qemu, err := exec.LookPath("qemu-system-" + arch)
	if err != nil {
		return -1
	}
	out, _ := execCommand(qemu, "--version").Output()
	for _, f := range strings.Fields(string(out)) {
		parts := strings.SplitN(f, ".", 3)
		if len(parts) >= 2 && parts[0] != "" && parts[0][0] >= '0' && parts[0][0] <= '9' {
			v, err := strconv.Atoi(parts[0] + parts[1])
			if err == nil {
				return v
			}
		}
	}
	return -1
}

// var seam: tests stub net.Listen to reserve/free without
// touching real sockets.
var netListen = net.Listen

// firstFreePort: returns p if set, else the first free TCP port in
// [lo,hi] — the ssh/SPICE callers of get_port parity: the pick runs
// under the registry lock, skips reservations owned by live VMs and
// records its own so concurrent instances cannot double-allocate.
// vmname lands in the reservation record (pid ts instance vm).
func firstFreePort(p string, lo, hi int, vmname string) string {
	if p != "" {
		return p
	}
	if !portRegistryLock() {
		fmt.Fprintln(os.Stderr, "ERROR! Could not acquire the port allocation lock.")
		return ""
	}
	defer portRegistryUnlock()
	for port := lo; port <= hi; port++ {
		if portRegistryTaken(port) {
			continue
		}
		ln, err := netListen("tcp", fmt.Sprintf(":%d", port))
		if err == nil {
			ln.Close()
			portRegistryReserve(port, os.Getpid(), "", vmname)
			return fmt.Sprint(port)
		}
	}
	fmt.Fprintf(os.Stderr, "ERROR! No free host port in range %d-%d.\n", lo, hi)
	return ""
}

// title: Bash ${var^} — capitalize the first letter (guest names in
// the BOOT status line).
func title(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
