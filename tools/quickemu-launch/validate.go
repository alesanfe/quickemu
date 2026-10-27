// validate.go: .conf validation — migrates guest_os_param_check and the
// launch-time config sanity checks from launch.sh.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// guestOSRe: Bash guest_os_param_check accepts any [a-z0-9-]+ token —
// no catalog membership check. Empty warns and assumes linux heuristics.
var guestOSRe = regexp.MustCompile(`^[a-z0-9-]+$`)

func validateConf(c Conf, confPath string) []string {
	var errs []string
	gos := c.get("guest_os", "")
	if gos == "" {
		fmt.Fprintf(os.Stderr, "WARNING! guest_os is not set in %s; assuming 'linux' heuristics.\n", confPath)
	} else if !guestOSRe.MatchString(gos) {
		errs = append(errs, "'"+gos+"' is not a valid guest_os (expected [a-z0-9-]+)")
	}
	if c.get("cpu_cores", "") != "" {
		var n int
		if fmt.Sscanf(c["cpu_cores"], "%d", &n); n < 1 || n > 256 {
			errs = append(errs, "cpu_cores out of range")
		}
	}
	if c.get("efi", "") != "" && c["efi"] != "true" && c["efi"] != "false" {
		errs = append(errs, "efi must be true/false")
	}
	dir := filepath.Dir(confPath)
	if d := c.get("disk_img", ""); d != "" {
		if _, err := os.Stat(filepath.Join(dir, d)); err != nil {
			errs = append(errs, "disk_img not found: "+d)
		}
	}
	if d := c.get("iso", ""); d != "" {
		if _, err := os.Stat(filepath.Join(dir, d)); err != nil {
			errs = append(errs, "iso not found: "+d)
		}
	}
	if p := c.get("ssh_port", ""); p != "" {
		var n int
		if fmt.Sscanf(p, "%d", &n); n < 1 || n > 65535 {
			errs = append(errs, "ssh_port out of range")
		}
	}
	if p := c.get("spice_port", ""); p != "" {
		var n int
		if fmt.Sscanf(p, "%d", &n); n < 1 || n > 65535 {
			errs = append(errs, "spice_port out of range")
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
func memMB(s string) int {
	s = strings.TrimSpace(s)
	mult := 1
	if strings.HasSuffix(s, "G") {
		mult, s = 1024, s[:len(s)-1]
	} else if strings.HasSuffix(s, "M") {
		s = s[:len(s)-1]
	} else if strings.HasSuffix(s, "K") {
		return 0
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n * mult
}

// hostFreeMemMB: MemAvailable from /proc/meminfo (host_free_mem_mb).
func hostFreeMemMB() int {
	if b, err := os.ReadFile("/proc/meminfo"); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(l, "MemAvailable:") {
				var kb int
				if _, err := fmt.Sscanf(l, "MemAvailable: %d kB", &kb); err == nil {
					return kb / 1024
				}
			}
		}
	}
	return 0
}
