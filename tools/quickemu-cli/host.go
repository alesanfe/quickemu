package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/alesanfe/quickemu/tools/qelib"
	"golang.org/x/sys/unix"
)

// unameArch maps Go runtime.GOARCH to uname -m spellings.
func unameArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	default:
		return runtime.GOARCH
	}
}

// unameFields: `uname -s -r -m` without spawning uname — same kernel
// info via the uname(2) syscall (x/sys/unix).
func unameFields(flags string) string {
	var u unix.Utsname
	if unix.Uname(&u) != nil {
		return ""
	}
	m := map[byte]string{
		's': unix.ByteSliceToString(u.Sysname[:]),
		'r': unix.ByteSliceToString(u.Release[:]),
		'v': unix.ByteSliceToString(u.Version[:]),
		'm': unix.ByteSliceToString(u.Machine[:]),
		'n': unix.ByteSliceToString(u.Nodename[:]),
	}
	var out []string
	for _, f := range strings.Fields(flags) {
		for _, c := range []byte(strings.TrimPrefix(f, "-")) {
			if v, ok := m[c]; ok && v != "" {
				out = append(out, v)
			}
		}
	}
	return strings.Join(out, " ")
}

// actCapabilities: static contract envelope (capabilities parity —
// keep in sync with schemas/v1/capabilities.schema.json).
func actCapabilities() int {
	emitOK("capabilities", map[string]any{
		"applicationVersion": qeVersion,
		"capabilities": map[string]bool{
			"structuredCatalog": true, "structuredStatus": true, "listVms": true,
			"snapshots": true, "structuredSnapshots": true, "monitorCommands": true,
			"sendKeys": true, "screenshot": true, "clone": true, "rename": true,
			"exportImport": true, "diskOps": true, "downloadProgress": false,
			"safeDeletePlan": true, "verifiableDeletePlan": true, "qmpChannel": true,
			"instanceIdentity": true, "sharedExclusiveLocks": true,
			"portRegistry": true, "verifiedArchives": true, "structuredEvents": true,
			"asyncOperations": true, "tomlConfig": true, "hostInventory": true,
		},
	})
	return 0
}

// actHostInfo: host_info parity — machine-readable host inventory
// (acceleration, deps, machine facts) for QuickGUI.
func actHostInfo() int {
	kvm := fileExists("/dev/kvm")
	iommu := false
	if ents, err := os.ReadDir("/sys/kernel/iommu_groups"); err == nil {
		for _, e := range ents {
			if sub, err := os.ReadDir(filepath.Join("/sys/kernel/iommu_groups", e.Name())); err == nil && len(sub) > 0 {
				iommu = true
			}
		}
	}
	var gpus []string
	for i, l := range pciGPUs() {
		if i == 8 {
			break
		}
		f := strings.Fields(l)
		if len(f) > 1 {
			gpus = append(gpus, strings.Join(f[1:], " "))
		}
	}
	qemuVer := ""
	if q, err := exec.LookPath("qemu-system-x86_64"); err == nil {
		if out, err := execCommand(q, "--version").Output(); err == nil {
			qemuVer = strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
		}
	}
	has := func(name string) bool { _, e := exec.LookPath(name); return e == nil }
	globAny := func(pats ...string) bool {
		for _, p := range pats {
			if m, _ := filepath.Glob(p); len(m) > 0 {
				return true
			}
		}
		return false
	}
	var memTotal, hugePages int
	if b, err := os.ReadFile("/proc/meminfo"); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(l, "MemTotal:") {
				fmt.Sscanf(l, "MemTotal: %d kB", &memTotal)
			}
			if strings.HasPrefix(l, "HugePages_Total:") {
				fmt.Sscanf(l, "HugePages_Total: %d", &hugePages)
			}
		}
	}
	freeKb := int64(0)
	var st syscall.Statfs_t
	if syscall.Statfs(qelib.XDGData(), &st) == nil {
		freeKb = int64(st.Bavail) * int64(st.Bsize) / 1024
	}
	kernel := unameFields("-s -r")
	emitOK("host.info", map[string]any{
		"kernel": kernel, "arch": unameArch(), "cpuCores": runtime.NumCPU(),
		"memTotalKb": memTotal, "kvm": kvm, "iommu": iommu, "qemu": qemuVer,
		"qemuAarch64": has("qemu-system-aarch64"), "qemuImg": has("qemu-img"),
		"swtpm": has("swtpm"),
		"ovmf":  globAny("/usr/share/OVMF*/OVMF_CODE.fd", "/usr/share/edk2/ovmf/OVMF_CODE.fd"),
		"aavmf": globAny("/usr/share/AAVMF*", "/usr/share/edk2/aarch64/QEMU_EFI.fd"),
		"passt": has("passt"), "virtiofsd": has("virtiofsd"), "spicy": has("spicy"),
		"remoteViewer": has("remote-viewer"), "freeKb": freeKb,
		"hugePages": hugePages, "gpus": gpus,
	})
	return 0
}

// actInventory: --inventory — the global VM registry at
// $XDG_DATA_HOME/quickemu/vms.json.
func actInventory() int {
	db := filepath.Join(qelib.XDGData(), "quickemu", "vms.json")
	b, err := os.ReadFile(db)
	if err != nil {
		if outFmt == "json" {
			fmt.Println("{}")
		} else {
			fmt.Println("No VMs registered yet.")
		}
		return 0
	}
	if outFmt == "json" {
		fmt.Println(string(b))
		return 0
	}
	var m map[string]struct {
		VM, State, Dir, LastSeen string
	}
	if json.Unmarshal(b, &m) == nil {
		for _, v := range m {
			fmt.Printf(" - %s [%s] %s (seen %s)\n", v.VM, v.State, v.Dir, v.LastSeen)
		}
	}
	return 0
}

// actStats: vm_stats parity — QMP cpus/block/balloon in one envelope.
func actStats() int {
	if _, err := os.Stat(qmpSock()); err != nil {
		if outFmt == "json" {
			emitFail("vm.stats", "VM_MUST_BE_RUNNING", "no running QMP")
		} else {
			fmt.Fprintln(os.Stderr, "ERROR! VM is not running (no QMP socket).")
		}
		return 1
	}
	get := func(cmd string) json.RawMessage {
		out, err := qmp(cmd)
		if err != nil {
			return json.RawMessage("null")
		}
		var env struct {
			Data json.RawMessage `json:"data"`
		}
		if json.Unmarshal([]byte(out), &env) == nil && env.Data != nil {
			return env.Data
		}
		return json.RawMessage("null")
	}
	out := map[string]json.RawMessage{
		"cpus": get("query-cpus-fast"), "block": get("query-blockstats"), "memory": get("query-balloon"),
	}
	b, _ := json.Marshal(out)
	if outFmt == "json" {
		emitOK("vm.stats", json.RawMessage(b))
		return 0
	}
	var m map[string]any
	json.Unmarshal(b, &m)
	var ncpu int
	if c, ok := m["cpus"].(map[string]any); ok {
		if r, ok := c["return"].([]any); ok {
			ncpu = len(r)
		}
	}
	bal := "n/a"
	if c, ok := m["memory"].(map[string]any); ok {
		if r, ok := c["return"].(map[string]any); ok {
			if a, ok := r["actual"]; ok {
				bal = fmt.Sprintf("%v", a)
			}
		}
	}
	fmt.Printf("CPUs: %d  Balloon: %s\n", ncpu, bal)
	return 0
}
