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
	default:
		if _, err := os.Stat("/sys/class/net/" + net); err != nil {
			errs = append(errs, "bridge interface "+net+" does not exist")
		}
	}
	return errs
}

// memMB parses ram values like 4G/512M into MB (0 for non-numeric).
func memMB(s string) int { return qelib.MemMB(s) }

// hostFreeMemMB: MemAvailable from /proc/meminfo (host_free_mem_mb).
func hostFreeMemMB() int { return qelib.HostFreeMemMB() }
