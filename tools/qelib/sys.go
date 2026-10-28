// Small host/sysfs helpers shared by the tools.
package qelib

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// HostArch returns the normalized quickemu guest arch name for the
// build host: "aarch64" on ARM64, "x86_64" otherwise.
func HostArch() string {
	if runtime.GOARCH == "arm64" {
		return "aarch64"
	}
	return "x86_64"
}

// MemMB parses a "4G"/"512M"/"256K" memory value into megabytes.
func MemMB(s string) int {
	s = strings.TrimSpace(s)
	mult := 1
	if strings.HasSuffix(s, "G") {
		mult, s = 1024, s[:len(s)-1]
	} else if strings.HasSuffix(s, "M") {
		s = s[:len(s)-1]
	} else if strings.HasSuffix(s, "K") {
		return 0 // <1M
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

// HostFreeMemMB: MemAvailable from /proc/meminfo (host_free_mem_mb).
func HostFreeMemMB() int {
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

// XDGData resolves $XDG_DATA_HOME with the ~/.local/share fallback.
func XDGData() string {
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return v
	}
	return filepath.Join(os.Getenv("HOME"), ".local", "share")
}

// PciAttr reads a sysfs attribute of a PCI device.
func PciAttr(dev, attr string) string {
	b, err := os.ReadFile(filepath.Join("/sys/bus/pci/devices", dev, attr))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// PciHex reads a 0x-prefixed sysfs attribute with the prefix stripped.
func PciHex(dev, attr string) string {
	return strings.TrimPrefix(PciAttr(dev, attr), "0x")
}
