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
	osName := conf.get("guest_os", "linux")
	arch := conf.get("arch", "x86_64")
	t := osQuirks(conf, osName, arch)
	machine := t.MachineType
	if arch == "aarch64" {
		machine = "virt"
	}

	args := []string{"qemu-system-" + arch}

	// accel + machine: tcg is a separate -accel arg; kvm goes inside
	// -machine (the two args+= paths in vm.sh). Same resolution as
	// Bash configure_cpu: user QEMU_ACCEL override wins for native
	// guests; cross-arch and qemu/virtualbox hypervisors force tcg.
	accel := "kvm"
	if u := conf.get("qemu_accel", os.Getenv("QEMU_ACCEL")); u != "" {
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
		// user override or already downgraded — keep.
	case runtime.GOOS == "darwin":
		accel = "hvf"
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
	if accel == "tcg" {
		args = append(args, "-accel", "tcg,tb-size=256,thread=multi")
		args = append(args, "-machine", machine+t.Machine+",smm="+t.SMM+",vmport=off")
	} else {
		args = append(args, "-machine", machine+t.Machine+",smm="+t.SMM+",vmport=off,accel="+accel)
	}

	// vm_boot order: cpu->ram->bios->os_quirks->storage->display->audio
	// ->ports->fileshare->usb->tpm->boot_media->channels->spawn
	// Base CPU model (aarch64 -> max, cross-arch/hypervisor -> qemu64)
	// applies only when the OS quirk did not already pin a model.
	if baseModel != "" && !strings.HasPrefix(t.CPUFlags, "@") && t.CPUFlags == "" {
		t.CPUFlags = "@" + baseModel
	}
	args = append(args, cpuArgs(conf, osName, arch, t)...)
	args = append(args, ramArgs(conf)...)
	args = append(args, biosArgs(conf, osName, arch)...)
	args = append(args, "-global", "kvm-pit.lost_tick_policy=discard")
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
	args = append(args, spiceArgs(conf, vmdir, vmname, osName, t)...)
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
	// monitor + serial channels — vm_boot_channels parity. monitor=socket
	// also gives the QMP socket; telnet/none have no structured channel.
	switch mon := conf.get("monitor", "socket"); mon {
	case "none":
		args = append(args, "-monitor", "none")
	case "telnet":
		host := conf.get("monitor_telnet_host", "localhost")
		portStr := conf.get("monitor_telnet_port", "4440")
		start, _ := strconv.Atoi(portStr)
		if start <= 0 {
			start = 4440
		}
		port := getPort(start, 9, os.Getpid(), vmname)
		if port <= 0 {
			fmt.Fprintf(os.Stderr, " - Monitor:  All Monitor-Telnet ports have been exhausted.\n")
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
	default:
		fmt.Fprintf(os.Stderr, "ERROR! \"%s\" is an unknown monitor option.\n", mon)
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
			fmt.Fprintf(os.Stderr, " - Serial:   All Serial Telnet ports have been exhausted.\n")
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
	default:
		fmt.Fprintf(os.Stderr, "ERROR! \"%s\" is an unknown serial option.\n", ser)
		os.Exit(1)
	}

	// pidfile + daemonize
	args = append(args, "-name", vmname+",process="+vmname+",debug-threads=on")
	args = append(args, "-pidfile", dotdir(vmdir)+vmname+".pid")
	args = append(args, rtcArgs(osName)...)
	args = append(args, "-device", "virtio-balloon")
	args = append(args, "-object", "rng-random,id=rng0,filename=/dev/urandom", "-device", "virtio-rng-pci,rng=rng0")

	if len(os.Args) > 2 && os.Args[2] == "--run" {
		// Hardware-virtualisation gate (configure_cpu): only enforced on
		// a real launch — argv printing is the --dry-run equivalent and
		// must not hard-fail on hosts without VT-x/SVM.
		if hypervisor == "" && accel != "tcg" && arch == hostArch() && runtime.GOOS == "linux" {
			switch hostCPUVendor() {
			case "GenuineIntel":
				if !hostHasFlag("vmx") {
					fmt.Fprintln(os.Stderr, "ERROR! Intel VT-x support is required.")
					fmt.Fprintln(os.Stderr, "         If virtualisation fails, check your CPU supports hardware virtualisation and it's enabled in firmware.")
					os.Exit(1)
				}
			case "AuthenticAMD", "HygonGenuine":
				if !hostHasFlag("svm") {
					fmt.Fprintln(os.Stderr, "ERROR! AMD SVM support is required.")
					fmt.Fprintln(os.Stderr, "         If virtualisation fails, check your CPU supports hardware virtualisation and it's enabled in firmware.")
					os.Exit(1)
				}
			}
		}
		// MSR alert (ignore_msrs_alert): warn when kvm ignore_msrs=N —
		// macOS/Windows guests crash on unhandled MSRs otherwise.
		if b, err := os.ReadFile("/sys/module/kvm/parameters/ignore_msrs"); err == nil && strings.TrimSpace(string(b)) == "N" {
			fmt.Fprintln(os.Stderr, " - MSR:      WARNING! Ignoring unhandled Model-Specific Registers is disabled.")
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "             echo 1 | sudo tee /sys/module/kvm/parameters/ignore_msrs")
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "             If you are unable to run macOS or Windows VMs then run the above")
			fmt.Fprintln(os.Stderr, "             This will enable ignoring of unhandled MSRs until you reboot the host.")
			fmt.Fprintln(os.Stderr, "             You can make this change permanent by running: 'quickemu --ignore-msrs-always'")
		}
		// TSC gate (plan_check_tsc): macOS Ventura+ on AMD hosts with an
		// unstable clocksource can freeze — warn/abort unless bypassed.
		if tsc, clock := hostMacosTSCUnstable(conf); tsc {
			if conf.get("ignore_tsc_warning", "") == "" {
				fmt.Fprintf(os.Stderr, " - TSC:      WARNING! Unstable TSC detected (clocksource: %s)\n", clock)
				fmt.Fprintln(os.Stderr, "             macOS may freeze on AMD Ryzen mobile CPUs.")
				fmt.Fprintln(os.Stderr, "             Fix: Add 'tsc=reliable' to kernel boot parameters and reboot.")
				fmt.Fprintln(os.Stderr, "             Or:  Use macOS Big Sur (11) or Monterey (12) instead.")
				fmt.Fprintln(os.Stderr, "ERROR! Use ignore_tsc_warning=\"on\" in the conf (or --ignore-tsc-warning) to bypass.")
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, " - TSC:      WARNING! Unstable TSC detected (clocksource: %s), proceeding.\n", clock)
		}
		if err := runQEMU(os.Args[1], vmdir, args); err != nil {
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
