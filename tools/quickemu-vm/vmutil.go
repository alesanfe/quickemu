// tools/quickemu-vm/vmutil.go: vm fs/config helpers + exec seam + profile presets
package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/alesanfe/quickemu/tools/qelib"
)

func xdgData() string { return qelib.XDGData() }

// confGet: read a bare key="value" line from the VM's .conf.
// pathWithin: p resolves inside base (no '..' escape).
func pathWithin(base, p string) bool {
	rel, err := filepath.Rel(base, p)
	return err == nil && rel != ".." &&
		!strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// snapTagExists: snapshot_tag_exists parity — `qemu-img snapshot -l`
// output contains the tag as a whole field (grep -qw).
func snapTagExists(disk, tag string) bool {
	if disk == "" {
		return false
	}
	if _, err := os.Stat(disk); err != nil {
		return false
	}
	out, err := execCommand("qemu-img", "snapshot", "-l", disk).Output()
	if err != nil {
		return false
	}
	for _, f := range strings.Fields(string(out)) {
		if f == tag {
			return true
		}
	}
	return false
}

func confGet(dir, key string) string {
	conf := vmConfPath(dir)
	if conf == "" {
		return ""
	}
	f, err := os.Open(conf)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, key+"=") {
			return strings.Trim(strings.SplitN(line, "=", 2)[1], `"'`)
		}
	}
	return ""
}

func confDisk(dir string) string {
	conf := vmConfPath(dir)
	if conf == "" {
		return ""
	}
	f, err := os.Open(conf)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "disk_img=") {
			v := strings.SplitN(line, "=", 2)[1]
			v = strings.Trim(v, `\"'`)
			if filepath.IsAbs(v) {
				return v
			}
			// Bash resolves a relative disk_img against the conf's
			// directory (VMPATH) — VMDIR is derived from its dirname —
			// so '<vm>/disk.qcow2' in a sibling <vm>.conf lands inside
			// the VM dir. Ops act on the resolved path
			// (compact/resize/delete): keep it under the conf's dir or
			// the VM dir so '..' can't hit files outside the VM tree.
			base := filepath.Dir(conf)
			joined := filepath.Join(base, v)
			if !pathWithin(base, joined) && !pathWithin(dir, joined) {
				return ""
			}
			return joined
		}
	}
	return ""
}

// Command construction seam (ADR-0009): tests substitute stubs to
// observe/fake subprocesses. Restore via t.Cleanup; never t.Parallel
// while a stub is installed.
var execCommand = exec.Command

// ProfileDefaults mirrors vm_profile_defaults — conf overrides per preset.
func ProfileDefaults(name string) (map[string]string, error) {
	profiles := map[string]map[string]string{
		"windows-gaming": {"guest_os": "windows", "ram": "8G", "cpu_cores": "6", "video": "virtio-vga-gl"},
		"linux-desktop":  {"guest_os": "linux", "ram": "4G", "cpu_cores": "4"},
		"server":         {"guest_os": "linux", "ram": "4G", "display": "none", "network": "user"},
		"minimal":        {"guest_os": "linux", "ram": "512M", "cpu_cores": "1"},
		"ci":             {"guest_os": "linux", "ram": "2G", "display": "none", "network": "none"},
	}
	p, ok := profiles[name]
	if !ok {
		return nil, fmt.Errorf("unknown profile %q", name)
	}
	return p, nil
}

// NetworkPreset mirrors vm_network_preset — maps preset name to network= value.
func NetworkPreset(name string) (string, error) {
	switch name {
	case "nat", "user":
		return "user", nil
	case "bridge":
		return "bridge", nil
	case "hostonly":
		return "host", nil
	case "isolated", "none":
		return "none", nil
	}
	return "", fmt.Errorf("unknown preset %q", name)
}
