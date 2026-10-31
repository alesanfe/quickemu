// quickemu-launch: generate the qemu-system command line from a .conf.
// Migrates the arg-building core of launch.sh — reads the VM config and
// emits the exact QEMU invocation. Launch itself stays Bash-spawned.
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

// warnTelnetExposure flags telnet monitor/serial endpoints bound to a
// non-loopback address — the QEMU monitor has no authentication, so a
// network-reachable binding hands HMP control to the whole network.
// Only provably-loopback bindings stay silent (no DNS lookups here —
// resolving names would stall launch on unreachable resolvers).
func warnTelnetExposure(kind, host string) {
	if host == "" || host == "localhost" {
		return
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return
	}
	fmt.Fprintf(os.Stderr, "WARNING! %s telnet endpoint bound to %q — QEMU monitor is unauthenticated; prefer loopback or %s=socket.\n", kind, host, kind)
}

// launchPlan carries the per-invocation state through the phases —
// conf load → host resolve → configure → emit → launch. Fields fill in
// the same order vm_boot resolves them; the arg slices are configure_*
// outputs the emit phase appends in vm_boot order.
type launchPlan struct {
	confPath   string
	conf       Conf
	dir        string
	vmdir      string
	vmname     string
	osName     string
	arch       string
	runMode    bool
	accel      string
	hypervisor string
	baseModel  string
	machine    string
	t          OsTuning
	cpuA       []string
	ramA       []string
	biosA      []string
	displayA   []string
	audioA     []string
	spiceA     []string
	shareA     []string
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
	p := loadPlan()
	if p == nil {
		return // --validate already reported
	}
	p.resolveHost()
	p.configure()
	p.launch(p.emit())
}

// loadPlan: the conf-load phase — parse + presets, early param checks,
// --validate mode, VM-dir creation and PUBLIC resolution. Returns nil
// when the invocation was fully handled (validate printed its verdict).
func loadPlan() *launchPlan {
	confPath := os.Args[1]
	conf := applyPresets(parseConf(confPath))
	conf["_vmdir"] = vmDir(conf, confPath)
	conf["_vmname"] = strings.TrimSuffix(filepath.Base(confPath), filepath.Ext(confPath))
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
		if errs := validateConf(conf, confPath); len(errs) > 0 {
			for _, e := range errs {
				fmt.Fprintln(os.Stderr, "invalid:", e)
			}
			os.Exit(1)
		}
		fmt.Println("valid")
		return nil
	}
	p := &launchPlan{confPath: confPath, conf: conf}
	p.dir = filepath.Dir(confPath)
	p.vmdir = conf["_vmdir"]
	if err := os.MkdirAll(p.vmdir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR! Cannot create VM dir %s: %v\n", p.vmdir, err)
		os.Exit(1)
	}
	resolvePublic(conf)

	// The virtiofsd spawn itself happens at the start_virtiofsd
	// position in the configure sequence (after storageEcho) — linux
	// guest + resolved PUBLIC + no ISO + daemon available.
	p.runMode = len(os.Args) > 2 && os.Args[2] == "--run"
	// swtpm decision mirrors virtiofsd (configure_tpm): spawn the daemon
	// before arg assembly on --run so the socket exists when the chardev
	// is emitted (swtpm_socket_ready). Plan mode always emits.
	conf["_run"] = ""
	if p.runMode {
		conf["_run"] = "1"
	}
	p.vmname = strings.TrimSuffix(filepath.Base(confPath), filepath.Ext(confPath))
	p.osName = strings.ToLower(conf.get("guest_os", "linux"))
	p.arch = conf.get("arch", "x86_64")
	return p
}

// resolveHost: the vm_boot resolution steps between conf load and the
// configure phase — guest_os recognition warning, run-mode port-state
// reset, accelerator/CPU-model resolution, os_quirks, storage prep and
// the -machine base string.
func (p *launchPlan) resolveHost() {
	conf, osName, arch := p.conf, p.osName, p.arch
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
	// configure_ports resets the per-boot state files before any port
	// record is written (run mode only — plan mode must not touch them).
	if p.runMode {
		os.WriteFile(filepath.Join(p.vmdir, p.vmname+".ports"), nil, 0o644)
		os.Remove(filepath.Join(p.vmdir, p.vmname+".spice"))
		os.Remove(dotdir(p.vmdir) + p.vmname + ".sock")
	}
	p.accel, p.hypervisor, p.baseModel = resolveAccel(conf, arch)
	conf["_accel"] = p.accel
	conf["_hypervisor"] = p.hypervisor
	p.t = osQuirks(conf, osName, arch)
	// configure_storage sits between os_quirks and display in Bash:
	// create missing disks, probe write locks, and drop iso/img when
	// the disk is already used (installed guests boot from disk).
	if err := prepareStorage(conf, p.dir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	p.machine = p.t.MachineType
	if arch == "aarch64" {
		// highmem=on lets virt guests address RAM above 4GB (configure_cpu).
		p.machine = "virt,highmem=on"
		// ARM64 pflash firmware exposes code/vars as named blockdev
		// nodes on the virt machine (vm_boot MACHINE_TYPE suffix) —
		// only when EFI_MODE stayed pflash, i.e. not the -bios path.
		if strings.Contains(resolvedBoot(conf, osName), "efi") &&
			a64MonolithicEDK2(conf, osName) == "" {
			p.machine += ",pflash0=rom,pflash1=efivars"
		}
	}
}

// resolveAccel: the configure_cpu accelerator decision — user QEMU_ACCEL
// override, cross-arch/hypervisor TCG forcing and the /dev/kvm
// reality-check. Returns the accel mode, detected hypervisor and the
// fallback CPU base model ("" when the guest keeps its default).
func resolveAccel(conf Conf, arch string) (accel, hypervisor, baseModel string) {
	// accel resolves in configure_cpu — before os_quirks, whose KVM-only
	// hv_* windows flags need it. TCG is a separate -accel arg; kvm goes
	// inside -machine (the two args+= paths in vm.sh). User QEMU_ACCEL
	// override wins for native guests; cross-arch and qemu/virtualbox
	// hypervisors force tcg.
	// USER_QEMU_ACCEL is honoured on the non-Darwin branch only —
	// configure_cpu forces hvf on macOS hosts regardless.
	accel = "kvm"
	if runtime.GOOS == "darwin" {
		accel = "hvf"
	} else if u := conf.get("qemu_accel", os.Getenv("QEMU_ACCEL")); u != "" {
		accel = u
	}
	hypervisor = hostHypervisor()
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
	return accel, hypervisor, baseModel
}

// ============ configure phase ============
// Bash runs every configure_* first (CPU→RAM→TSC→BIOS→quirks→
// storage→display→audio→ports→fileshare→usb→tpm); each builder
// prints its echoes here so they precede the emit phase, then the
// returned arg slices are appended below in vm_boot order.
func (p *launchPlan) configure() {
	conf, osName, arch := p.conf, p.osName, p.arch
	if p.baseModel != "" && !strings.HasPrefix(p.t.CPUFlags, "@") && p.t.CPUFlags == "" {
		p.t.CPUFlags = "@" + p.baseModel
	}
	p.cpuA = cpuArgs(conf, osName, arch, p.t)
	// MSR alert (ignore_msrs_alert inside configure_cpu): macOS/
	// Windows guests only, Linux hosts only — echoed even on --dry-run.
	if runtime.GOOS != "darwin" &&
		(osName == "macos" || osName == "windows" || osName == "windows-server") {
		if b, err := os.ReadFile("/sys/module/kvm/parameters/ignore_msrs"); err == nil && strings.TrimSpace(string(b)) == "N" {
			fmt.Println(" - MSR:      WARNING! Ignoring unhandled Model-Specific Registers is disabled.")
			fmt.Println("")
			fmt.Println("             echo 1 | sudo tee /sys/module/kvm/parameters/ignore_msrs")
			fmt.Println("")
			fmt.Println("             If you are unable to run macOS or Windows VMs then run the above 👆")
			fmt.Println("             This will enable ignoring of unhandled MSRs until you reboot the host.")
			fmt.Println("             You can make this change permanent by running: 'quickemu --ignore-msrs-always'")
		}
	}
	// vm.sh validates cpu_pinning against the logical core count
	// mid-assembly — same position.
	if err := paramCheckPinning(conf); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	p.ramA = ramArgs(conf, osName, p.confPath)
	p.checkTSC()
	// configure_bios fatal checks: macOS firmware/bootloader presence +
	// sizes, missing EFI firmware, EFI vars writability. Bash position —
	// after the CPU/RAM/TSC echoes, before the BOOT echo. echo → stdout.
	if err := paramCheckFirmware(conf, osName, arch); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	p.biosA = biosArgs(conf, osName, arch)
	// configure_os_quirks SSE/AVX2 fatals fire after the BOOT echo —
	// osQuirks resolved them earlier but Bash ordering puts the check
	// here (echo → stdout).
	if p.t.FatalErr != "" {
		fmt.Println(p.t.FatalErr)
		os.Exit(1)
	}
	storageEcho(conf, p.dir)
	p.planVirtiofs()
	p.displayA = displayArgs(conf, p.dir, osName, arch)
	p.audioA = audioArgs(conf, osName, p.t)
	planPorts(conf, p.t) // -ssh:/-PORTS: echoes + resolve _fwd
	planVsock(conf)      // vsock CID reservation (registry sibling of ports)
	p.spiceA = spiceArgs(conf, p.vmdir, p.vmname, osName)
	p.shareA = fileshareArgs(conf, p.dir)
	// configure_usb + configure_tpm close the Bash configure sequence.
	// swtpm mirrors virtiofsd: spawn before emit on --run so the socket
	// exists when the chardev lands (swtpm_socket_ready); plan always.
	checkUSBPassthrough(conf)
	p.planSwtpm()
}

// checkTSC — plan_check_tsc: macOS Ventura+ on AMD hosts with an unstable
// clocksource can freeze — warns in every mode; dry-run only
// reports, run mode prompts on a TTY or aborts non-interactively.
func (p *launchPlan) checkTSC() {
	conf := p.conf
	tsc, clock := hostMacosTSCUnstable(conf)
	if !tsc {
		return
	}
	fmt.Printf(" - TSC:      WARNING! Unstable TSC detected (clocksource: %s)\n", clock)
	if conf.get("ignore_tsc_warning", "") != "" {
		fmt.Println("             Proceeding anyway due to --ignore-tsc-warning flag.")
		return
	}
	fmt.Printf("             macOS %s may freeze on AMD Ryzen mobile CPUs.\n",
		title(conf.get("macos_release", "")))
	fmt.Println("")
	fmt.Println("             Fix: Add 'tsc=reliable' to kernel boot parameters and reboot.")
	fmt.Println("             Or:  Use macOS Big Sur (11) or Monterey (12) instead.")
	fmt.Println("             See: https://github.com/quickemu-project/quickemu/wiki/03-Create-macOS-virtual-machines#tsc-instability-on-amd-ryzen-mobile-cpus")
	fmt.Println("")
	if !p.runMode {
		return
	}
	if f, err := os.OpenFile(filepath.Join(p.vmdir, p.vmname+".log"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
		if _, werr := fmt.Fprintf(f, "TSC_WARNING: clocksource=%s macos_release=%s cpu_vendor=%s\n",
			clock, conf.get("macos_release", ""), hostCPUVendor()); werr != nil {
			fmt.Fprintf(os.Stderr, "WARNING: could not log TSC warning: %v\n", werr)
		}
		f.Close()
	}
	if st, err := os.Stdin.Stat(); err == nil && st.Mode()&os.ModeCharDevice != 0 {
		fmt.Print("Do you want to continue anyway? [y/N] ")
		var resp string
		fmt.Scanln(&resp)
		switch strings.ToLower(resp) {
		case "y", "yes":
			fmt.Println(" - TSC:      Proceeding despite unstable TSC warning.")
		default:
			fmt.Println(" - TSC:      Aborting. Please apply one of the solutions above.")
			os.Exit(1)
		}
	} else {
		fmt.Println("ERROR! Non-interactive mode detected. Use --ignore-tsc-warning to bypass this check.")
		os.Exit(1)
	}
}

// planVirtiofs — start_virtiofsd runs between configure_storage and
// configure_display in vm_boot: run mode spawns the daemon now so
// the socket exists when QEMU starts; plan mode prints the planned
// socket. Failure falls back to 9p with a warning.
func (p *launchPlan) planVirtiofs() {
	conf := p.conf
	if conf.get("guest_os", "") != "linux" || conf["_public"] == "" || conf.get("iso", "") != "" {
		return
	}
	bin, err := exec.LookPath("virtiofsd")
	if err != nil {
		return
	}
	if p.runMode {
		if startVirtiofsd(bin, conf) {
			conf["_virtiofsd"] = "1"
			conf["_virtiofsd_bin"] = bin
		}
	} else {
		fmt.Printf(" - virtiofsd: %s (planned)\n",
			dotdir(p.vmdir)+p.vmname+".virtiofsd-sock")
		conf["_virtiofsd"] = "1"
	}
}

// planSwtpm — configure_tpm: plan mode prints the (planned) line
// without spawning; run mode spawns the daemon now so the socket
// exists when the chardev lands (swtpm_socket_ready).
func (p *launchPlan) planSwtpm() {
	conf := p.conf
	// tpm_dev passthrough needs no daemon — skip the swtpm spawn.
	if conf.get("tpm_dev", "") != "" {
		return
	}
	if !confOn(conf, "tpm") {
		return
	}
	// plan mode prints the (planned) line without spawning.
	if !p.runMode {
		fmt.Printf(" - TPM:      %s (planned)\n", filepath.Join(p.vmdir, p.vmname+".swtpm-sock"))
		conf["_swtpm"] = "1"
	} else if startSwtpm(conf) {
		conf["_swtpm"] = "1"
	}
}

// ============ emit phase (vm_boot args+= order) ============
func (p *launchPlan) emit() []string {
	conf, osName, arch, accel := p.conf, p.osName, p.arch, p.accel
	args := []string{"qemu-system-" + arch}
	// -name first, Linux only (process renaming is a Linux QEMU feature).
	if runtime.GOOS == "linux" {
		args = append(args, "-name", p.vmname+",process="+p.vmname+",debug-threads=on")
	}
	if accel == "tcg" {
		// tb-size tiers by host RAM: 512M on 16GB+ hosts (vm_boot).
		// tcg_thread="single" opts a guest out of MTTCG — some kernels
		// hang mid-boot on TCG's multi-threaded SMP emulation.
		tb := 256
		if (hostTotalMemMB()+512)/1024 >= 16 {
			tb = 512
		}
		thr := conf.get("tcg_thread", "multi")
		args = append(args, "-accel", fmt.Sprintf("tcg,tb-size=%d,thread=%s", tb, thr))
	}
	switch {
	case arch == "aarch64" && accel == "tcg":
		// SMM/vmport are x86-only; ARM64 emits neither.
		args = append(args, "-machine", p.machine+p.t.Machine)
	case arch == "aarch64":
		args = append(args, "-machine", p.machine+p.t.Machine+",accel="+accel)
	case accel == "tcg":
		args = append(args, "-machine", p.machine+p.t.Machine+",smm="+p.t.SMM+",vmport=off")
	default:
		args = append(args, "-machine", p.machine+p.t.Machine+",smm="+p.t.SMM+",vmport=off,accel="+accel)
	}
	// GUEST_TWEAKS goes right after -machine, before CPU (vm_boot):
	// the kvm-pit global on non-Darwin hosts plus the OS-quirk extras.
	if runtime.GOOS != "darwin" {
		args = append(args, "-global", "kvm-pit.lost_tick_policy=discard")
	}
	args = append(args, p.t.ExtraArgs...)
	args = append(args, p.cpuA...)
	args = append(args, p.ramA...)
	if p.t.Balloon != "" {
		args = append(args, strings.Fields(p.t.Balloon)...)
	}
	// -pidfile closes the machine args+=() group in vm_boot.
	args = append(args, "-pidfile", dotdir(p.vmdir)+p.vmname+".pid")
	args = append(args, rtcArgs(osName)...)
	args = append(args, p.displayA...)
	args = append(args, p.spiceA...)
	// The guest-agent channel is display-independent (QE-010): bus +
	// chardev + org.qemu.guest_agent.0 port for every launch.
	args = append(args, qgaArgs(conf, p.vmdir, p.vmname)...)
	// rng: device first, then the object it references (vm.sh order).
	args = append(args, "-device", "virtio-rng-pci,rng=rng0", "-object", "rng-random,id=rng0,filename=/dev/urandom")
	args = append(args, spiceUSBArgs(arch, p.t)...)
	args = append(args, inputArgs(conf, p.t)...)
	args = append(args, brailleArgs(conf, arch)...)
	args = append(args, p.audioA...)
	args = append(args, networkArgs(conf, p.t)...)
	args = append(args, p.biosA...)
	args = append(args, bootMediaArgs(conf, p.dir)...)
	// pxe=on nets boots before the disk — emitted last so -boot is
	// last-wins over the per-guest order tweaks.
	args = append(args, pxeArgs(conf)...)
	// -object iothread before the virtio-blk devices that use it.
	args = append(args, iothreadArgs(conf)...)
	args = append(args, storageArgs(conf, p.dir, p.t)...)
	args = append(args, p.shareA...)
	args = append(args, usbArgs(conf, p.t)...)
	args = append(args, passthruArgs(conf)...)
	args = append(args, tpmArgs(conf, arch)...)
	args = append(args, watchdogArgs(conf)...)
	args = append(args, vsockArgs(conf)...)
	args = p.monitorChan(args)
	args = p.serialChan(args)
	// QE-020: -incoming for resume-from-suspend — emitted before the
	// extra_args escape hatch so a user override can still win.
	args = append(args, resumeArgs(conf)...)
	// extra_args verbatim append (vm.sh): word-split like the unquoted
	// Bash expansion — the escape hatch for any QEMU flag quickemu
	// does not model (e.g. extra_args="-accel tcg" to force TCG).
	if ea := conf.get("extra_args", ""); ea != "" {
		args = append(args, strings.Fields(ea)...)
	}
	args = append(args, vncArgs(conf, p.vmdir)...)
	return p.directBootArgs(args)
}

// monitorChan: the monitor channel emit — vm_boot_channels parity.
// monitor=socket also gives the QMP socket; telnet/none have no
// structured channel.
func (p *launchPlan) monitorChan(args []string) []string {
	conf, vmdir, vmname := p.conf, p.vmdir, p.vmname
	switch mon := conf.get("monitor", "socket"); mon {
	case "none":
		args = append(args, "-monitor", "none")
		fmt.Printf(" - Monitor:  (off)\n")
	case "telnet":
		args = telnetChan(args, conf, vmdir, vmname, p.runMode,
			"monitor", " - Monitor:  ", "Monitor-Telnet", 4440)
	case "socket":
		args = append(args, "-monitor",
			"unix:"+qemuOptEsc(dotdir(vmdir)+vmname+"-monitor.socket")+",server,nowait")
		// Machine control channel (QMP) next to the HMP socket.
		args = append(args, "-qmp",
			"unix:"+qemuOptEsc(dotdir(vmdir)+vmname+"-qmp.socket")+",server,nowait")
		sockHint(" - Monitor:  ", dotdir(vmdir)+vmname+"-monitor.socket")
	default:
		fmt.Printf("ERROR! \"%s\" is an unknown monitor option.\n", mon)
		os.Exit(1)
	}
	return args
}

// serialChan: the serial channel emit — none for macos/windows
// default; socket otherwise.
func (p *launchPlan) serialChan(args []string) []string {
	conf, vmdir, vmname := p.conf, p.vmdir, p.vmname
	ser := conf.get("serial", "")
	if ser == "" {
		switch p.osName {
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
		args = telnetChan(args, conf, vmdir, vmname, p.runMode,
			"serial", " - Serial:   ", "Serial Telnet", 6660)
	case "socket":
		args = append(args, "-serial",
			"unix:"+qemuOptEsc(dotdir(vmdir)+vmname+"-serial.socket")+",server,nowait")
		sockHint(" - Serial:   ", dotdir(vmdir)+vmname+"-serial.socket")
	default:
		fmt.Printf("ERROR! \"%s\" is an unknown serial option.\n", ser)
		os.Exit(1)
	}
	return args
}

// directBootArgs: kernel/initrd/boot_args override (vm.sh) — direct
// kernel boot, the unattended ubuntu-server flow writes these into the
// conf — plus the NoCloud/cloud-init seed image attached as a cdrom.
func (p *launchPlan) directBootArgs(args []string) []string {
	conf, dir := p.conf, p.dir
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
	return args
}

// launch: --run spawns QEMU through runQEMU after the hardware-
// virtualisation gate; every other mode prints the assembled argv
// (the --dry-run/plan equivalent of vm_boot).
func (p *launchPlan) launch(args []string) {
	if !p.runMode {
		fmt.Println(strings.Join(args, " "))
		return
	}
	// Hardware-virtualisation gate (configure_cpu): only enforced on
	// a real launch — argv printing is the --dry-run equivalent and
	// must not hard-fail on hosts without VT-x/SVM.
	if p.hypervisor == "" && p.accel != "tcg" && p.arch == hostArch() &&
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
	if err := runQEMU(p.confPath, p.vmdir, args, p.conf); err != nil {
		fmt.Fprintln(os.Stderr, "launch failed:", err)
		os.Exit(1)
	}
}
