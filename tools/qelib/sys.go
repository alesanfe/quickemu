// Small host/sysfs helpers shared by the tools.
package qelib

import (
	"crypto/rand"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/shirou/gopsutil/v4/mem"
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

// HostFreeMemMB: host_free_mem_mb — available memory via gopsutil,
// so macOS and the BSDs report real values, not only Linux
// /proc/meminfo.
func HostFreeMemMB() int {
	v, err := mem.VirtualMemory()
	if err != nil {
		return 0
	}
	return int(v.Available / (1024 * 1024))
}

// XDGData resolves $XDG_DATA_HOME with the ~/.local/share fallback.
func XDGData() string {
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return v
	}
	return filepath.Join(os.Getenv("HOME"), ".local", "share")
}

// XDGCache resolves $XDG_CACHE_HOME with the ~/.cache fallback.
func XDGCache() string {
	if v := os.Getenv("XDG_CACHE_HOME"); v != "" {
		return v
	}
	return filepath.Join(os.Getenv("HOME"), ".cache")
}

// XDGConfig resolves $XDG_CONFIG_HOME with the ~/.config fallback.
func XDGConfig() string {
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		return v
	}
	return filepath.Join(os.Getenv("HOME"), ".config")
}

// ProcOSRelease is a fixture seam: tests point it at a canned
// osrelease file (or "" to simulate unreadable procfs).
var ProcOSRelease = "/proc/sys/kernel/osrelease"

// IsWSL reports Windows Subsystem for Linux: WSL_DISTRO_NAME /
// WSL_INTEROP env, or "microsoft" in the kernel release string.
// WSL1 never exposes /dev/kvm; WSL2 needs nested virtualization.
func IsWSL() bool {
	if os.Getenv("WSL_DISTRO_NAME") != "" || os.Getenv("WSL_INTEROP") != "" {
		return true
	}
	b, err := os.ReadFile(ProcOSRelease)
	return err == nil && strings.Contains(strings.ToLower(string(b)), "microsoft")
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

// QuickgetArch normalises a runtime arch string to the quickget
// download-arch name: "arm64" on ARM64, "amd64" otherwise (quickget
// catalogs use distro naming, not QEMU's x86_64/aarch64).
func QuickgetArch(ra string) string {
	if ra == "arm64" {
		return "arm64"
	}
	return "amd64"
}

// CryptSalt: random SHA512-crypt salt chars — shared by
// quickget-vm's unattend generators and quickprovision's profile pwHash.
func CryptSalt(n int) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789./"
	b := make([]byte, n)
	rb := make([]byte, n)
	if _, err := rand.Read(rb); err != nil {
		copy(rb, "quickemu")
	}
	for i := range b {
		b[i] = chars[int(rb[i])%len(chars)]
	}
	return string(b)
}

// ProfileDefaults: vm_profile_defaults parity — preset conf values.
// Shared by quickemu-vm (create) and quickemu-launch (boot) so a new
// profile exists in both paths from a single definition.
var profileDefaults = map[string]map[string]string{
	"windows-gaming": {"guest_os": "windows", "ram": "8G", "cpu_cores": "6", "video": "virtio-vga-gl"},
	"linux-desktop":  {"guest_os": "linux", "ram": "4G", "cpu_cores": "4"},
	"server":         {"guest_os": "linux", "ram": "4G", "display": "none", "network": "user"},
	"minimal":        {"guest_os": "linux", "ram": "512M", "cpu_cores": "1"},
	"ci":             {"guest_os": "linux", "ram": "2G", "display": "none", "network": "none"},
}

// ProfileDefaults returns the preset conf overrides for name.
func ProfileDefaults(name string) (map[string]string, bool) {
	p, ok := profileDefaults[name]
	return p, ok
}

// ArchSupported: is_arch_supported parity — an empty catalog arch
// list means the Bash default (amd64 only). Shared by quickget-cli
// (validate) and quickget-catalog.
func ArchSupported(list []string, arch string) bool {
	if len(list) == 0 {
		list = []string{"amd64"}
	}
	for _, a := range list {
		if a == arch {
			return true
		}
	}
	return false
}

// CopyFile: byte copy src -> dst (create/truncate).
func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	// Close can surface delayed-alloc write failures (ENOSPC/NFS) —
	// a short copy must not read as success.
	return out.Close()
}

// MoveFile: rename, falling back to copy for cross-device moves
// (mv semantics — the source remove is best-effort like Bash's mv).
func MoveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := CopyFile(src, dst); err != nil {
		return err
	}
	os.Remove(src)
	return nil
}
