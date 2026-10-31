// validate.go: .conf validation — migrates guest_os_param_check and the
// launch-time config sanity checks from launch.sh.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/alesanfe/quickemu/tools/qelib"
)

// guestOSRe: Bash guest_os_param_check accepts any [a-z0-9-]+ token —
// no catalog membership check. Empty warns and assumes linux heuristics.
var guestOSRe = regexp.MustCompile(`^[a-z0-9-]+$`)

// snapTagChars: savevm/loadvm tags reach QEMU through conf keys and
// monitor commands — the same charset quickemu-vm enforces so a
// hostile conf cannot smuggle an option through -loadvm.
var snapTagChars = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// digitsOnly: the Bash port/core checks are strict [0-9]+ — Sscanf("%d")
// accepts a numeric prefix ("22x" → 22) and lets junk reach QEMU.
func digitsOnly(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

func validateConf(c Conf, confPath string) []string {
	var errs []string
	gos := c.get("guest_os", "")
	if gos == "" {
		fmt.Fprintf(os.Stderr, "WARNING! guest_os is not set in %s; assuming 'linux' heuristics.\n", confPath)
	} else if !guestOSRe.MatchString(gos) {
		errs = append(errs, "'"+gos+"' is not a valid guest_os (expected [a-z0-9-]+)")
	}
	if p := c.get("cpu_cores", ""); p != "" {
		n, _ := strconv.Atoi(p)
		if !digitsOnly(p) || n < 1 || n > 256 {
			errs = append(errs, "cpu_cores out of range")
		}
	}
	if c.get("efi", "") != "" && c["efi"] != "true" && c["efi"] != "false" {
		errs = append(errs, "efi must be true/false")
	}
	dir := filepath.Dir(confPath)
	if d := c.get("disk_img", ""); d != "" {
		if _, err := os.Stat(joinConf(dir, d)); err != nil {
			errs = append(errs, "disk_img not found: "+d)
		}
	}
	if d := c.get("iso", ""); d != "" {
		if _, err := os.Stat(joinConf(dir, d)); err != nil {
			errs = append(errs, "iso not found: "+d)
		}
	}
	// ports_param_check: port_forwards entries must be HOST:GUEST
	// numbers; ssh_port/spice_port must be numeric in range.
	fwdRe := regexp.MustCompile(`^[0-9]+:[0-9]+$`)
	for _, pf := range strings.Fields(c.get("port_forwards", "")) {
		if !fwdRe.MatchString(pf) {
			errs = append(errs, "'"+pf+"' is not a valid port forward (expected host:guest numbers)")
		}
	}
	if p := c.get("ssh_port", ""); p != "" {
		n, _ := strconv.Atoi(p)
		if !digitsOnly(p) || n < 1 || n > 65535 {
			errs = append(errs, "ssh_port must be a number in 1-65535")
		}
	}
	if p := c.get("spice_port", ""); p != "" {
		n, _ := strconv.Atoi(p)
		if !digitsOnly(p) || n < 1 || n > 65535 {
			errs = append(errs, "spice_port must be a number in 1-65535")
		}
	}
	// planning gates (plan_check_ram/tpm/net): warnings, not errors —
	// Bash only warns or prompts, never rejects the conf.
	if ram := c.get("ram", ""); ram != "" {
		if want := memMB(ram); want > 0 {
			if free := hostFreeMemMB(); free > 0 && want > free {
				errs = append(errs, fmt.Sprintf("ram=%s exceeds free memory (%dM available)", ram, free))
			}
		}
	}
	if c.get("tpm", "") == "on" {
		if _, err := exec.LookPath("swtpm"); err != nil {
			errs = append(errs, "tpm=on but swtpm not installed")
		}
	}
	switch net := c.get("network", ""); net {
	case "", "none", "restrict", "passt", "slirp":
		// vm_net_mode: ""|slirp→user, none|restrict|passt literal,
		// everything else is a bridge interface name.
	default:
		if _, err := os.Stat("/sys/class/net/" + net); err != nil {
			errs = append(errs, "bridge interface "+net+" does not exist")
		}
	}
	// Feature keys (extras.go): each validates its own value shape —
	// these are pure conf knobs with no Bash counterpart to mirror.
	for _, k := range []string{"qga", "nested", "pxe"} {
		switch c.get(k, "") {
		case "", "on", "off", "true", "false", "yes", "no":
		default:
			errs = append(errs, k+" must be on/off")
		}
	}
	// nested=on: refuse when the host provably has nested virt off —
	// "unknown" (no sysfs access) stays permissive like other probes.
	if confOn(c, "nested") && nestedHostState() == "disabled" {
		errs = append(errs, "nested=on but host nested virtualization is disabled (/sys/module/kvm_*/parameters/nested)")
	}
	if v := c.get("iothreads", ""); v != "" {
		n, _ := strconv.Atoi(v)
		if !digitsOnly(v) || n < 1 || n > 64 {
			errs = append(errs, "iothreads must be a number in 1-64")
		}
	}
	switch mq := c.get("mq", ""); mq {
	case "", "off", "auto":
	default:
		if !digitsOnly(mq) {
			errs = append(errs, "mq must be 'auto' or a number")
		} else if q, _ := strconv.Atoi(mq); q < 1 || q > 64 {
			errs = append(errs, "mq must be in 1-64 or 'auto'")
		} else if cc := c.get("cpu_cores", ""); cc != "" && digitsOnly(cc) {
			if n, _ := strconv.Atoi(cc); q > n {
				errs = append(errs, "mq exceeds cpu_cores (queues must be <= vCPUs)")
			}
		}
	}
	switch w := c.get("watchdog", ""); w {
	case "", "off", "ib700", "virtio", "i6300esb":
	default:
		errs = append(errs, "watchdog must be ib700|virtio")
	}
	switch a := c.get("watchdog_action", ""); a {
	case "", "none", "reset", "shutdown", "poweroff", "pause", "debug", "inject-nmi":
	default:
		errs = append(errs, "watchdog_action must be none|reset|shutdown|poweroff|pause|debug|inject-nmi")
	}
	switch v := c.get("vsock", ""); v {
	case "", "off", "auto":
	default:
		if !digitsOnly(v) {
			errs = append(errs, "vsock must be 'auto' or a numeric guest CID")
		} else if n, _ := strconv.ParseInt(v, 10, 64); n < 3 || n > 4294967294 {
			errs = append(errs, "vsock guest CID must be in 3-4294967294 (0-2 are reserved)")
		}
	}
	if v := c.get("tpm_dev", ""); v != "" {
		if !strings.HasPrefix(v, "passthrough:") || len(v) <= len("passthrough:") {
			errs = append(errs, "tpm_dev must be 'passthrough:<device>'")
		} else if strings.ContainsAny(v, ",\n\r") || !filepath.IsAbs(strings.TrimPrefix(v, "passthrough:")) {
			errs = append(errs, "tpm_dev device must be an absolute path without commas")
		}
	}
	// commands.sh resolution/access/network param-checks applied to the
	// conf on disk — width/height pairing, access/ssh_access tokens and
	// macaddr shape. CLI-side overrides run through the same check.
	errs = append(errs, qelib.ParamCheckConf(map[string]string(c))...)
	return errs
}

// memMB parses ram values like 4G/512M into MB (0 for non-numeric).
func memMB(s string) int { return qelib.MemMB(s) }

// hostFreeMemMB: MemAvailable from /proc/meminfo (host_free_mem_mb).
// QE_HOST_FREE_MEM_MB overrides the live figure — cgroup-limited
// containers often misreport host memory, and tests need a fixed conf
// to validate regardless of the host's current memory pressure.
var hostFreeMemMB = func() int {
	if s := os.Getenv("QE_HOST_FREE_MEM_MB"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return n
		}
	}
	return qelib.HostFreeMemMB()
}
