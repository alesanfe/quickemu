// validate.go: .conf validation — migrates guest_os_param_check and the
// launch-time config sanity checks from launch.sh.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var validOS = map[string]bool{
	"linux": true, "windows": true, "macos": true, "bsd": true, "android": true, "kolibrios": true, "reactos": true, "haiku": true,
}

func validateConf(c Conf, confPath string) []string {
	var errs []string
	gos := c.get("guest_os", "")
	if gos == "" {
		errs = append(errs, "guest_os required")
	} else if !validOS[strings.ToLower(gos)] {
		errs = append(errs, "guest_os '"+gos+"' not in known set")
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
