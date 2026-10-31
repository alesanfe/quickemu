// extras.go: opt-in hardware features (conf keys, not CLI flags) —
// nested virtualization, iothreads, virtio-net multiqueue, watchdog,
// vsock CID allocation, TPM passthrough and PXE boot. Each feature
// validates in validateConf and emits independently.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// ---------- nested virtualization ----------

// sysfsNestedFiles is a var seam so tests can point the probe at a
// fixture tree instead of the host's kvm module state.
var sysfsNestedFiles = []string{
	"/sys/module/kvm_intel/parameters/nested",
	"/sys/module/kvm_amd/parameters/nested",
}

// nestedHostState: "enabled"/"disabled" from the kvm module params —
// "1"/"Y" mean nested virt is on. "unknown" when neither module's
// parameter is readable (non-Linux hosts, kvm unloaded, WSL).
func nestedHostState() string {
	for _, f := range sysfsNestedFiles {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		switch strings.ToUpper(strings.TrimSpace(string(b))) {
		case "1", "Y":
			return "enabled"
		case "0", "N":
			return "disabled"
		}
	}
	return "unknown"
}

// nestedCheck: configure_cpu-adjacent host probe — warn early when
// nested=on cannot be honored. Called from cpuArgs so the warning
// sits with the CPU echoes.
func nestedCheck(c Conf) {
	if runtime.GOOS != "linux" {
		fmt.Println(" - CPU:      WARNING! nested=on cannot be verified on this host platform.")
		return
	}
	switch nestedHostState() {
	case "disabled":
		fmt.Println(" - CPU:      WARNING! nested=on but host nested virtualization is disabled.")
		fmt.Println("             Enable it: echo 1 | sudo tee /sys/module/kvm_*/parameters/nested")
	case "unknown":
		fmt.Println(" - CPU:      WARNING! nested=on but host support is unverifiable (no kvm module parameters).")
	}
}

// nestedCPUFlag: the explicit vmx/svm exposure nested virt needs on
// -cpu host under KVM. "" when the cpu string is not host-based (TCG
// cannot do nested virt) — the caller warns in that case.
func nestedCPUFlag(cpu string) string {
	if !strings.HasPrefix(cpu, "host") {
		return ""
	}
	switch hostCPUVendor() {
	case "GenuineIntel":
		return ",+vmx"
	case "AuthenticAMD", "HygonGenuine":
		return ",+svm"
	}
	return ""
}

// ---------- iothreads ----------

// iothreadCount parses iothreads=N (0 = off). Range validation lives
// in validateConf; here non-numeric junk degrades to off.
func iothreadCount(c Conf) int {
	n := 0
	fmt.Sscanf(c.get("iothreads", ""), "%d", &n)
	if n < 0 || n > 64 {
		return 0
	}
	return n
}

// iothreadArgs: one -object iothread per requested thread, emitted
// before the storage devices that reference them.
func iothreadArgs(c Conf) []string {
	n := iothreadCount(c)
	var a []string
	for i := 0; i < n; i++ {
		a = append(a, "-object", fmt.Sprintf("iothread,id=iothread%d", i))
	}
	return a
}

// iothreadAttach appends iothread=iothreadK to a virtio-blk devspec —
// round-robin so extra disks spread over the requested threads.
// Non-virtio devices (ide-hd, if=ide, ahci) cannot take the property
// and are returned untouched.
func iothreadAttach(devspec string, idx, n int) string {
	if n <= 0 || !strings.HasPrefix(devspec, "virtio-blk") {
		return devspec
	}
	return fmt.Sprintf("%s,iothread=iothread%d", devspec, idx%n)
}

// ---------- virtio-net multiqueue ----------

// mqQueues resolves mq=N|auto into queue pairs, clamped to the vCPU
// count (QEMU needs one queue pair per vCPU at most) and to a
// protocol-sane ceiling. 0 disables.
func mqQueues(c Conf, osName string) int {
	v := c.get("mq", "")
	if v == "" || v == "off" {
		return 0
	}
	n := guestCores(c, osName)
	if v == "auto" {
		if n > 16 {
			n = 16
		}
		return n
	}
	q := 0
	fmt.Sscanf(v, "%d", &q)
	if q < 1 {
		return 0
	}
	if q > n {
		q = n
	}
	if q > 16 {
		q = 16
	}
	return q
}

// mqApply appends mq=on,vectors=2N+2 to every emitted virtio-net
// -device. Bridge -nic entries cannot carry device properties — a
// warning tells the user multiqueue was skipped for them.
func mqApply(a []string, q int, mqVal string) []string {
	if q <= 0 {
		return a
	}
	applied := false
	for i := 1; i < len(a); i++ {
		if a[i-1] == "-device" && strings.HasPrefix(a[i], "virtio-net") {
			a[i] = fmt.Sprintf("%s,mq=on,vectors=%d", a[i], 2*q+2)
			applied = true
		}
	}
	if !applied {
		fmt.Printf(" - WARNING!  mq=%s needs a virtio-net device (bridge/-nic cannot carry it); skipping multiqueue.\n", mqVal)
	}
	return a
}

// ---------- watchdog ----------

// watchdogArgs: -device <model> plus an optional -watchdog-action.
// "virtio" maps to i6300esb (the PCI watchdog QEMU exposes — there is
// no virtio-watchdog model); ib700 is the classic ISA watchdog.
func watchdogArgs(c Conf) []string {
	dev := ""
	switch c.get("watchdog", "") {
	case "ib700":
		dev = "ib700"
	case "virtio", "i6300esb":
		dev = "i6300esb"
	}
	if dev == "" {
		return nil
	}
	a := []string{"-device", dev}
	if act := c.get("watchdog_action", ""); act != "" {
		a = append(a, "-watchdog-action", act)
	}
	return a
}

// ---------- vsock ----------

// cidRegistry: the global vhost-vsock guest-CID registry — same
// mkdir-lock + pid-ownership + dead-owner-reclaim pattern as the port
// registry, kept as a sibling directory so QE_PORT_REGISTRY isolation
// in tests covers both.
func cidRegistry() string {
	if v := os.Getenv("QE_VSOCK_REGISTRY"); v != "" {
		return v
	}
	if v := os.Getenv("QE_PORT_REGISTRY"); v != "" {
		return filepath.Join(filepath.Dir(v), "vsock-cids")
	}
	return filepath.Join(os.TempDir(),
		fmt.Sprintf("quickemu-%d", os.Getuid()), "vsock-cids")
}

// planVsock resolves the guest CID during the configure phase (echo
// placement next to the port forwards) and records the reservation so
// concurrent launches never hand out the same CID twice. vsock=auto
// scans 3..65535 — CIDs 0-2 are reserved (hypervisor/host).
func planVsock(c Conf) {
	v := c.get("vsock", "")
	if v == "" || v == "off" {
		return
	}
	if !registryLock(cidRegistry()) {
		fmt.Fprintln(os.Stderr, "ERROR! Could not acquire the vsock CID allocation lock.")
		return
	}
	defer registryUnlock(cidRegistry())
	if v != "auto" {
		n, _ := strconv.Atoi(v)
		if registryTaken(cidRegistry(), n) {
			fmt.Printf(" - WARNING!  vsock guest-cid %d is already reserved by a live VM.\n", n)
		}
		registryReserve(cidRegistry(), n, os.Getpid(), "", c["_vmname"])
		c["_vsock_cid"] = v
		fmt.Printf(" - vsock:    guest-cid %s\n", v)
		return
	}
	for cid := 3; cid < 65536; cid++ {
		if registryTaken(cidRegistry(), cid) {
			continue
		}
		registryReserve(cidRegistry(), cid, os.Getpid(), "", c["_vmname"])
		c["_vsock_cid"] = fmt.Sprint(cid)
		fmt.Printf(" - vsock:    guest-cid %d (auto)\n", cid)
		return
	}
	fmt.Fprintln(os.Stderr, "ERROR! No free vsock guest CID (3-65535).")
}

// vsockArgs: -device vhost-vsock-pci,guest-cid=N — the CID resolves
// in planVsock during configure; direct callers get the lazy path.
func vsockArgs(c Conf) []string {
	if _, ok := c["_vsock_cid"]; !ok {
		if c.get("vsock", "") != "" && c.get("vsock", "") != "off" {
			planVsock(c)
		}
	}
	if cid := c["_vsock_cid"]; cid != "" {
		return []string{"-device", "vhost-vsock-pci,guest-cid=" + cid}
	}
	return nil
}

// ---------- PXE boot ----------

// pxeArgs: pxe=on nets boots before the disk — iPXE ROMs ship with
// QEMU's virtio-net/e1000 devices by default, so the boot order is
// the whole emit. Appended AFTER bootMediaArgs: -boot is last-wins,
// so PXE overrides the per-guest order tweaks (freedos/reactos).
func pxeArgs(c Conf) []string {
	if !confOn(c, "pxe") {
		return nil
	}
	return []string{"-boot", "order=nc"}
}

// ---------- suspend resume (QE-020) ----------

// resumeArgs: -incoming for a suspend-to-disk resume — resume_from
// points at the gzipped vmstate image the CLI wrote via QMP migrate.
// Emitted late (before extra_args) so the escape hatch can still
// override it; a relative path resolves inside the conf dir.
func resumeArgs(c Conf) []string {
	f := c.get("resume_from", "")
	if f == "" {
		return nil
	}
	return []string{"-incoming", "exec:gzip -dc " + shellQuote(f)}
}

// shellQuote: single-quote for the -incoming exec: command — QEMU
// runs it through /bin/sh; a VM dir with spaces must not split the
// redirect source. An embedded quote becomes '\” (the standard
// shell-safe form).
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// loadvmArgs: -loadvm <tag> boots a stopped VM straight into an
// in-disk memory snapshot (QE-021 apply path). The tag is charset-
// gated like savevm's so a hostile conf cannot smuggle an option.
func loadvmArgs(c Conf) []string {
	tag := c.get("loadvm", "")
	if !snapTagChars.MatchString(tag) {
		return nil
	}
	return []string{"-loadvm", tag}
}

// sandboxArgs — QE-100: sandbox=on|strict|relaxed turns on QEMU's
// seccomp sandbox. strict denies spawn too — incompatible with
// -incoming exec: (suspend resume pipes through gzip -dc), so the
// resume path relaxes spawn with a warning rather than failing.
func sandboxArgs(c Conf) []string {
	mode := c.get("sandbox", "off")
	if mode == "off" || mode == "" {
		return nil
	}
	opts := "on,obsolete=deny,elevateprivileges=deny,resourcecontrol=deny"
	switch mode {
	case "strict":
		if c.get("resume_from", "") != "" {
			fmt.Println(" - WARNING! sandbox=strict cannot deny spawn with -incoming exec: — relaxing spawn.")
		} else {
			opts += ",spawn=deny"
		}
	case "on", "true", "relaxed":
		// 'on' keeps spawn allowed (migration/exec transports need it).
	default:
		fmt.Fprintf(os.Stderr, " - WARNING! unknown sandbox=%q — ignoring.\n", mode)
		return nil
	}
	return []string{"-sandbox", opts}
}
