// validate.go: .conf validation — migrates guest_os_param_check and the
// launch-time config sanity checks from launch.sh.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
	return errs
}
