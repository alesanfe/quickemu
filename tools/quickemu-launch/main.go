// quickemu-launch: generate the qemu-system command line from a .conf.
// Migrates the arg-building core of launch.sh — reads the VM config and
// emits the exact QEMU invocation. Launch itself stays Bash-spawned.
package main

import (
	"fmt"
	"github.com/BurntSushi/toml"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/alesanfe/quickemu/tools/qelib"
)

type Conf map[string]string

// joinConf resolves a conf-file value against the conf's directory.
// filepath.Join does NOT reset on an absolute second operand (unlike
// POSIX path resolution), so absolute values must be kept verbatim —
// a conf may point anywhere on the filesystem (disk_img="/tmp/...").
func joinConf(dir, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(dir, p)
}

// qemuOptEsc: QEMU option lists split on ',' — a conf path or name
// containing a comma would otherwise inject bogus options
// (file=d,ro=on opens "d"; hostname=x,hostfwd=... adds a forward).
// QEMU's documented escape inside option strings is ',,'.
func qemuOptEsc(v string) string {
	return strings.ReplaceAll(v, ",", ",,")
}

func parseConf(path string) Conf {
	if strings.HasSuffix(path, ".toml") {
		return parseTomlConf(path)
	}
	c := Conf{}
	b, err := os.ReadFile(path)
	if err != nil {
		return c
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		kv := strings.SplitN(line, "=", 2)
		if len(kv) != 2 {
			continue
		}
		v := strings.Trim(strings.TrimSpace(kv[1]), `"'`)
		c[strings.TrimSpace(kv[0])] = v
	}
	return c
}

func (c Conf) get(k, def string) string {
	if v, ok := c[k]; ok && v != "" {
		return v
	}
	return def
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: quickemu-launch <conf> [--dry-run]")
		os.Exit(2)
	}
	// --reap <pid> <dir> <name>: detached cleanup watcher spawned by
	// runQEMU — the disowned `while kill -0` + vm_runtime_cleanup loop.
	if os.Args[1] == "--reap" && len(os.Args) >= 5 {
		pid := 0
		fmt.Sscanf(os.Args[2], "%d", &pid)
		if len(os.Args) >= 6 {
			fmt.Sscanf(os.Args[5], "%d", &launcherPid)
		}
		reapVM(pid, os.Args[3], os.Args[4])
		return
	}
	conf := applyPresets(parseConf(os.Args[1]))
	conf["_vmdir"] = vmDir(conf, os.Args[1])
	conf["_vmname"] = strings.TrimSuffix(filepath.Base(os.Args[1]), filepath.Ext(os.Args[1]))
	// display_param_check (commands.sh): braille->sdl+xhci, platform
	// display whitelist, glxinfo gl default — before any arg assembly.
	if err := paramCheckDisplay(conf); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// The name goes verbatim into -name (whose parser splits on ','
	// with no escape) — a comma-bearing conf name cannot be launched
	// correctly, so fail here rather than boot under a mangled name.
	if strings.Contains(conf["_vmname"], ",") {
		fmt.Fprintf(os.Stderr, "ERROR! VM name %q contains a comma — rename the conf file.\n", conf["_vmname"])
		os.Exit(1)
	}
	if len(os.Args) > 2 && os.Args[2] == "--validate" {
		if errs := validateConf(conf, os.Args[1]); len(errs) > 0 {
			for _, e := range errs {
				fmt.Fprintln(os.Stderr, "invalid:", e)
			}
			os.Exit(1)
		}
		fmt.Println("valid")
		return
	}
	dir := filepath.Dir(os.Args[1])
	vmdir := conf["_vmdir"]
	if err := os.MkdirAll(vmdir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR! Cannot create VM dir %s: %v\n", vmdir, err)
		os.Exit(1)
	}
	resolvePublic(conf)

	// virtiofs decision happens before arg assembly (start_virtiofsd):
	// linux guest + resolved PUBLIC + no ISO + daemon available. On
	// --run the daemon is actually spawned now; a failed start falls
	// back to 9p. Plan mode just checks the binary resolves.
	runMode := len(os.Args) > 2 && os.Args[2] == "--run"
	if conf.get("guest_os", "") == "linux" && conf["_public"] != "" && conf.get("iso", "") == "" {
		if bin, err := exec.LookPath("virtiofsd"); err == nil {
			if !runMode || startVirtiofsd(bin, conf) {
				conf["_virtiofsd"] = "1"
			}
		}
	}
	// swtpm decision mirrors virtiofsd (configure_tpm): spawn the daemon
	// before arg assembly on --run so the socket exists when the chardev
	// is emitted (swtpm_socket_ready). Plan mode always emits.
	conf["_run"] = ""
	if runMode {
		conf["_run"] = "1"
	}
	if confOn(conf, "tpm") && (!runMode || startSwtpm(conf)) {
		conf["_swtpm"] = "1"
	}
	vmname := strings.TrimSuffix(filepath.Base(os.Args[1]), filepath.Ext(os.Args[1]))
	osName := strings.ToLower(conf.get("guest_os", "linux"))
	// vm_boot warns on unrecognised guest_os values — 'custom' configs
	// may use arbitrary values, so warn only (generic defaults apply).
	switch osName {
	case "batocera", "freedos", "haiku", "kolibrios", "linux", "linux_old",
		"macos", "reactos", "solaris", "templeos", "windows", "windows-server":
	default:
		if !strings.HasSuffix(osName, "bsd") {
			fmt.Printf(" - WARNING!  '%s' is not a recognised guest_os value; using generic defaults.\n", osName)
		}
	}
	arch := conf.get("arch", "x86_64")
	// configure_ports resets the per-boot state files before any port
	// record is written (run mode only — plan mode must not touch them).
	if runMode {
		os.WriteFile(filepath.Join(vmdir, vmname+".ports"), nil, 0o644)
		os.Remove(filepath.Join(vmdir, vmname+".spice"))
		os.Remove(dotdir(vmdir) + vmname + ".sock")
	}
	// configure_bios fatal checks: macOS firmware/bootloader presence +
	// sizes, EFI vars writability. Runs before any arg assembly.
	if err := paramCheckFirmware(conf, arch); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// accel resolves in configure_cpu — before os_quirks, whose KVM-only
	// hv_* windows flags need it. TCG is a separate -accel arg; kvm goes
	// inside -machine (the two args+= paths in vm.sh). User QEMU_ACCEL
	// override wins for native guests; cross-arch and qemu/virtualbox
	// hypervisors force tcg.
	// USER_QEMU_ACCEL is honoured on the non-Darwin branch only —
	// configure_cpu forces hvf on macOS hosts regardless.
	accel := "kvm"
	if runtime.GOOS == "darwin" {
		accel = "hvf"
	} else if u := conf.get("qemu_accel", os.Getenv("QEMU_ACCEL")); u != "" {
		accel = u
	}
	hypervisor := hostHypervisor()
	baseModel := ""
	switch {
	case arch == "aarch64":
		baseModel = "max"
		if hostArch() != "aarch64" {
			accel = "tcg"
		}
	case arch != hostArch(), hypervisor != "":
		accel = "tcg"
		baseModel = "qemu64"
	}
	// Accelerator reality-check for the kvm default: macOS accelerates
	// via hvf (there is no /dev/kvm), and a Linux host without the
	// device — WSL1, containers, VT-x off — used to emit accel=kvm
	// and fail inside QEMU. Fall back to TCG with a visible warning.
	switch {
	case accel != "kvm":
		// user override, hvf, or already downgraded — keep.
	case !hasKVM():
		accel = "tcg"
		if qelib.IsWSL() {
			fmt.Fprintln(os.Stderr, "WARNING! WSL without /dev/kvm — using slow TCG emulation.")
			fmt.Fprintln(os.Stderr, "         KVM on WSL2 needs nested virtualization; WSL1 has no KVM.")
		} else {
			fmt.Fprintln(os.Stderr, "WARNING! /dev/kvm unavailable — using slow TCG emulation.")
		}
	}
	// User-forced TCG on a native x86 guest also drops to qemu64 —
	// the `host` CPU model exists only under KVM (configure_cpu).
	if accel == "tcg" && baseModel == "" && arch == "x86_64" {
		baseModel = "qemu64"
	}
	conf["_accel"] = accel
	conf["_hypervisor"] = hypervisor
	t := osQuirks(conf, osName, arch)
	// configure_storage sits between os_quirks and display in Bash:
	// create missing disks, probe write locks, and drop iso/img when
	// the disk is already used (installed guests boot from disk).
	if err := prepareStorage(conf, dir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// configure_usb_passthrough: probe each declared vid:pid via lsusb —
	// missing devices are dropped from the passthrough list; present but
	// unwritable ones abort the launch (host udev rules required).
	checkUSBPassthrough(conf)
	machine := t.MachineType
	if arch == "aarch64" {
		// highmem=on lets virt guests address RAM above 4GB (configure_cpu).
		machine = "virt,highmem=on"
		// ARM64 pflash firmware exposes code/vars as named blockdev
		// nodes on the virt machine (vm_boot MACHINE_TYPE suffix) —
		// only when EFI_MODE stayed pflash, i.e. not the -bios path.
		if strings.Contains(resolvedBoot(conf, osName), "efi") &&
			a64MonolithicEDK2(conf, osName) == "" {
			machine += ",pflash0=rom,pflash1=efivars"
		}
	}

	args := []string{"qemu-system-" + arch}

	if accel == "tcg" {
		// tb-size tiers by host RAM: 512M on 16GB+ hosts (vm_boot).
		tb := 256
		if (hostTotalMemMB()+512)/1024 >= 16 {
			tb = 512
		}
		args = append(args, "-accel", fmt.Sprintf("tcg,tb-size=%d,thread=multi", tb))
		if arch == "aarch64" {
			// SMM/vmport are x86-only; ARM64 emits neither.
			args = append(args, "-machine", machine+t.Machine)
		} else {
			args = append(args, "-machine", machine+t.Machine+",smm="+t.SMM+",vmport=off")
		}
	} else {
		if arch == "aarch64" {
			args = append(args, "-machine", machine+t.Machine+",accel="+accel)
		} else {
			args = append(args, "-machine", machine+t.Machine+",smm="+t.SMM+",vmport=off,accel="+accel)
		}
	}

	// vm_boot order: cpu->ram->bios->os_quirks->storage->display->audio
	// ->ports->fileshare->usb->tpm->boot_media->channels->spawn
	// Base CPU model (aarch64 -> max, cross-arch/hypervisor -> qemu64)
	// applies only when the OS quirk did not already pin a model.
	if baseModel != "" && !strings.HasPrefix(t.CPUFlags, "@") && t.CPUFlags == "" {
		t.CPUFlags = "@" + baseModel
	}
	args = append(args, cpuArgs(conf, osName, arch, t)...)
	// vm.sh validates cpu_pinning against the logical core count
	// mid-assembly — same position.
	if err := paramCheckPinning(conf); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	args = append(args, ramArgs(conf)...)
	args = append(args, biosArgs(conf, osName, arch)...)
	// GUEST_TWEAKS kvm-pit global: launch.sh adds it on the non-Darwin
	// branch only (macOS hosts take the hvf path instead).
	if runtime.GOOS != "darwin" {
		args = append(args, "-global", "kvm-pit.lost_tick_policy=discard")
	}
	args = append(args, t.ExtraArgs...)
	if t.Balloon != "" {
		args = append(args, strings.Fields(t.Balloon)...)
	}
	args = append(args, inputArgs(conf, t)...)
	args = append(args, brailleArgs(conf, arch)...)
	args = append(args, usbArgs(conf, t)...)
	args = append(args, storageArgs(conf, dir, t)...)
	args = append(args, displayArgs(conf, dir, osName, arch)...)
	args = append(args, audioArgs(conf, osName, t)...)
	args = append(args, networkArgs(conf, t)...)
	args = append(args, portArgs(conf, dir)...)
	args = append(args, spiceArgs(conf, vmdir, vmname, osName, arch, t)...)
	args = append(args, fileshareArgs(conf, dir)...)
	args = append(args, passthruArgs(conf)...)
	args = append(args, bootMediaArgs(conf, dir)...)
	args = append(args, channelArgs(conf, dir)...)
	// extra_args verbatim append (vm.sh): word-split like the unquoted
	// Bash expansion — the escape hatch for any QEMU flag quickemu
	// does not model (e.g. extra_args="-accel tcg" to force TCG).
	if ea := conf.get("extra_args", ""); ea != "" {
		args = append(args, strings.Fields(ea)...)
	}
	// kernel/initrd/boot_args override (vm.sh): direct kernel boot —
	// the unattended ubuntu-server flow writes these into the conf.
	if k := conf.get("kernel", ""); k != "" {
		args = append(args, "-kernel", joinConf(dir, k))
		if i := conf.get("initrd", ""); i != "" {
			args = append(args, "-initrd", joinConf(dir, i))
		}
		if b := conf.get("boot_args", ""); b != "" {
			args = append(args, "-append", b)
		}
	}
	// NoCloud/cloud-init seed image attached as a cdrom (vm.sh).
	if s := conf.get("seed", ""); s != "" {
		sp := joinConf(dir, s)
		if _, err := os.Stat(sp); err == nil {
			args = append(args, "-drive", "file="+qemuOptEsc(sp)+",media=cdrom,if=virtio,format=raw")
		} else {
			fmt.Printf(" - WARNING! seed '%s' not found, skipping.\n", s)
		}
	}
	// monitor + serial channels — vm_boot_channels parity. monitor=socket
	// also gives the QMP socket; telnet/none have no structured channel.
	switch mon := conf.get("monitor", "socket"); mon {
	case "none":
		args = append(args, "-monitor", "none")
		fmt.Printf(" - Monitor:  (off)\n")
	case "telnet":
		host := conf.get("monitor_telnet_host", "localhost")
		portStr := conf.get("monitor_telnet_port", "4440")
		start, _ := strconv.Atoi(portStr)
		if start <= 0 {
			start = 4440
		}
		port := getPort(start, 9, os.Getpid(), vmname)
		if port <= 0 {
			fmt.Printf(" - Monitor:  All Monitor-Telnet ports have been exhausted.\n")
		} else {
			args = append(args, "-monitor",
				fmt.Sprintf("telnet:%s:%d,server,nowait", qemuOptEsc(host), port))
			fmt.Printf(" - Monitor:  On host:  telnet %s %d\n", host, port)
			portsRecord(vmdir, vmname,
				fmt.Sprintf("monitor-telnet,%d,%s", port, host), runMode)
		}
	case "socket":
		args = append(args, "-monitor",
			"unix:"+qemuOptEsc(dotdir(vmdir)+vmname+"-monitor.socket")+",server,nowait")
		// Machine control channel (QMP) next to the HMP socket.
		args = append(args, "-qmp",
			"unix:"+qemuOptEsc(dotdir(vmdir)+vmname+"-qmp.socket")+",server,nowait")
		if _, err := exec.LookPath("socat"); err == nil {
			fmt.Printf(" - Monitor:  On host:  socat -,echo=0,icanon=0 unix-connect:%s\n", dotdir(vmdir)+vmname+"-monitor.socket")
		} else if _, err := exec.LookPath("nc"); err == nil {
			fmt.Printf(" - Monitor:  On host:  nc -U \"%s\"\n", dotdir(vmdir)+vmname+"-monitor.socket")
		}
	default:
		fmt.Printf("ERROR! \"%s\" is an unknown monitor option.\n", mon)
		os.Exit(1)
	}
	// serial: none for macos/windows default; socket otherwise.
	ser := conf.get("serial", "")
	if ser == "" {
		switch osName {
		case "macos", "windows", "windows-server":
			ser = "none"
		default:
			ser = "socket"
		}
	}
	switch ser {
	case "none":
		args = append(args, "-serial", "none")
	case "telnet":
		host := conf.get("serial_telnet_host", "localhost")
		portStr := conf.get("serial_telnet_port", "6660")
		start, _ := strconv.Atoi(portStr)
		if start <= 0 {
			start = 6660
		}
		port := getPort(start, 9, os.Getpid(), vmname)
		if port <= 0 {
			fmt.Printf(" - Serial:   All Serial Telnet ports have been exhausted.\n")
		} else {
			args = append(args, "-serial",
				fmt.Sprintf("telnet:%s:%d,server,nowait", qemuOptEsc(host), port))
			fmt.Printf(" - Serial:   On host:  telnet %s %d\n", host, port)
			portsRecord(vmdir, vmname,
				fmt.Sprintf("serial-telnet,%d,%s", port, host), runMode)
		}
	case "socket":
		args = append(args, "-serial",
			"unix:"+qemuOptEsc(dotdir(vmdir)+vmname+"-serial.socket")+",server,nowait")
		if _, err := exec.LookPath("socat"); err == nil {
			fmt.Printf(" - Serial:   On host:  socat -,echo=0,icanon=0 unix-connect:%s\n", dotdir(vmdir)+vmname+"-serial.socket")
		} else if _, err := exec.LookPath("nc"); err == nil {
			fmt.Printf(" - Serial:   On host:  nc -U \"%s\"\n", dotdir(vmdir)+vmname+"-serial.socket")
		}
	default:
		fmt.Printf("ERROR! \"%s\" is an unknown serial option.\n", ser)
		os.Exit(1)
	}

	// pidfile + daemonize. Process renaming is a Linux-only QEMU
	// feature (vm_boot OS_KERNEL gate).
	if runtime.GOOS == "linux" {
		args = append(args, "-name", vmname+",process="+vmname+",debug-threads=on")
	}
	args = append(args, "-pidfile", dotdir(vmdir)+vmname+".pid")
	args = append(args, rtcArgs(osName)...)
	// rng: device first, then the object it references (vm.sh order).
	args = append(args, "-device", "virtio-rng-pci,rng=rng0", "-object", "rng-random,id=rng0,filename=/dev/urandom")

	if len(os.Args) > 2 && os.Args[2] == "--run" {
		// Hardware-virtualisation gate (configure_cpu): only enforced on
		// a real launch — argv printing is the --dry-run equivalent and
		// must not hard-fail on hosts without VT-x/SVM.
		if hypervisor == "" && accel != "tcg" && arch == hostArch() &&
			(runtime.GOOS == "linux" || runtime.GOOS == "darwin") {
			switch hostCPUVendor() {
			case "GenuineIntel":
				if !hostHasFlag("vmx") {
					fmt.Println("ERROR! Intel VT-x support is required.")
					os.Exit(1)
				}
			case "AuthenticAMD", "HygonGenuine":
				if !hostHasFlag("svm") {
					fmt.Println("ERROR! AMD SVM support is required.")
					os.Exit(1)
				}
			default:
				// ARM hosts have no x86 virt flags (handled by the
				// hypervisor); anything else gets the Bash warning.
				if hostArch() != "aarch64" {
					fmt.Printf("WARNING! Unknown CPU vendor '%s' - cannot verify virtualisation support.\n", hostCPUVendor())
					fmt.Println("         If virtualisation fails, check your CPU supports hardware virtualisation and it's enabled in firmware.")
				}
			}
		}
		// MSR alert (ignore_msrs_alert): macOS/Windows guests only —
		// warn (stdout echo) when kvm ignore_msrs=N.
		if osName == "macos" || osName == "windows" || osName == "windows-server" {
			if b, err := os.ReadFile("/sys/module/kvm/parameters/ignore_msrs"); err == nil && strings.TrimSpace(string(b)) == "N" {
				fmt.Println(" - MSR:      WARNING! Ignoring unhandled Model-Specific Registers is disabled.")
				fmt.Println("")
				fmt.Println("             echo 1 | sudo tee /sys/module/kvm/parameters/ignore_msrs")
				fmt.Println("")
				fmt.Println("             If you are unable to run macOS or Windows VMs then run the above")
				fmt.Println("             This will enable ignoring of unhandled MSRs until you reboot the host.")
				fmt.Println("             You can make this change permanent by running: 'quickemu --ignore-msrs-always'")
			}
		}
		// TSC gate (plan_check_tsc): macOS Ventura+ on AMD hosts with an
		// unstable clocksource can freeze — warn/abort unless bypassed.
		// Bash echoes the whole block to stdout and logs TSC_WARNING
		// to the VM log; non-interactive stdin aborts by default.
		if tsc, clock := hostMacosTSCUnstable(conf); tsc {
			fmt.Printf(" - TSC:      WARNING! Unstable TSC detected (clocksource: %s)\n", clock)
			if f, err := os.OpenFile(filepath.Join(vmdir, vmname+".log"),
				os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
				fmt.Fprintf(f, "TSC_WARNING: clocksource=%s macos_release=%s cpu_vendor=%s\n",
					clock, conf.get("macos_release", ""), hostCPUVendor())
				f.Close()
			}
			if conf.get("ignore_tsc_warning", "") == "" {
				fmt.Println("             macOS may freeze on AMD Ryzen mobile CPUs.")
				fmt.Println("")
				fmt.Println("             Fix: Add 'tsc=reliable' to kernel boot parameters and reboot.")
				fmt.Println("             Or:  Use macOS Big Sur (11) or Monterey (12) instead.")
				fmt.Println("             See: https://github.com/quickemu-project/quickemu/wiki/03-Create-macOS-virtual-machines#tsc-instability-on-amd-ryzen-mobile-cpus")
				fmt.Println("")
				fmt.Println("ERROR! Non-interactive mode detected. Use --ignore-tsc-warning to bypass this check.")
				os.Exit(1)
			}
			fmt.Println("             Proceeding anyway due to --ignore-tsc-warning flag.")
		}
		if err := runQEMU(os.Args[1], vmdir, args, conf); err != nil {
			fmt.Fprintln(os.Stderr, "launch failed:", err)
			os.Exit(1)
		}
		return
	}
	fmt.Println(strings.Join(args, " "))
}

// parseTomlConf loads a .toml VM config via BurntSushi/toml — the same
// library quickemu-toml wraps (Bash toml.sh parity). The flat config
// model keeps scalar top-level keys only; nested tables/arrays are
// ignored with the same warning as loader.sh.
func parseTomlConf(path string) Conf {
	c := Conf{}
	var doc map[string]any
	if _, err := toml.DecodeFile(path, &doc); err != nil {
		return c
	}
	for k, v := range doc {
		switch x := v.(type) {
		case string:
			c[k] = x
		case bool:
			if x {
				c[k] = "true"
			}
		case int64, float64:
			c[k] = fmt.Sprintf("%v", x)
		case []any:
			// TOML array -> bash-array literal so confList parses both.
			parts := make([]string, 0, len(x))
			for _, e := range x {
				parts = append(parts, fmt.Sprintf("%q", fmt.Sprint(e)))
			}
			c[k] = "(" + strings.Join(parts, " ") + ")"
		default:
			fmt.Fprintf(os.Stderr, "WARNING! Ignoring nested TOML key %q (flat config model).\n", k)
		}
	}
	return c
}

// dotdir: Bash launches with cwd=vmdir so artifacts render as ./name —
// emit the same prefix when the conf sits in the current directory.
func dotdir(dir string) string {
	if dir == "." {
		return "./"
	}
	return dir + "/"
}

// vmDir: Bash vm_identify parity — runtime state (pidfile, sockets,
// .ports, .log, .runtime.json, locks) lives in the disk's directory:
// dirname(disk_img) resolved against the conf directory for relative
// paths. With no disk_img configured the quickget convention applies:
// a per-VM directory named after the conf, alongside it.
func vmDir(conf Conf, confPath string) string {
	dir := filepath.Dir(confPath)
	name := strings.TrimSuffix(filepath.Base(confPath), filepath.Ext(confPath))
	if d := strings.TrimSpace(conf.get("disk_img", "")); d != "" {
		if filepath.IsAbs(d) {
			return filepath.Dir(d)
		}
		// Relative disk_img must stay inside the conf dir — "../x"
		// would scatter .pid/.sock/.ports outside the VM tree, and
		// diverge from the CLI's vmdir() which clamps the same way.
		j := filepath.Join(dir, d)
		if rel, err := filepath.Rel(dir, j); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return filepath.Dir(j)
		}
	}
	return filepath.Join(dir, name)
}

// confOn: quickemu conf booleans are "on" (with "true"/"yes" tolerated).
func confOn(c Conf, key string) bool {
	v := c.get(key, "")
	return v == "on" || v == "true" || v == "yes"
}
