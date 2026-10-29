// ops.go: the remaining vm.sh surface — export_vm, import_vm,
// autostart_create, shortcut_create/delete_shortcut, backup_*,
// trash_*, vfio_scan, snapshot_info, delete_disk.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
)

// runtimeExts: artifacts excluded from exports/delete-plans.
var runtimeExts = []string{".pid", ".sock", ".socket", ".ports", ".spice", ".monitor", ".lock", ".runtime.json"}

func num(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case json.Number:
		f, _ := n.Float64()
		return f
	}
	return 0
}

// vmRunningDir: vm_running parity — the VM's pidfile exists, the pid
// is alive AND the process belongs to the VM (comm matches the name,
// its 15-char truncation or qemu*; comm-unreadable hosts fall back to
// plain liveness, same as the Bash legacy path).
func vmRunningDir(dir, name string) bool {
	b, err := os.ReadFile(filepath.Join(dir, name+".pid"))
	if err != nil {
		return false
	}
	var pid int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(b)), "%d", &pid); err != nil || pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil || proc.Signal(syscall.Signal(0)) != nil {
		return false
	}
	comm, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		return true // cannot verify identity — liveness decides
	}
	c := strings.TrimSpace(string(comm))
	short := name
	if len(short) > 15 {
		short = short[:15]
	}
	return c == name || c == short || strings.HasPrefix(c, "qemu")
}

// vmStoppedOrDie: shared VM_MUST_BE_STOPPED guard for every op that
// hands the VM disk to qemu-img — resize/compact/consolidate corrupt
// a live image exactly the way snapshot -c does (write races and, in
// compact's case, an atomic rename under the running guest).
func vmStoppedOrDie(dir, verb string) {
	name := filepath.Base(dir)
	if c := vmConfPath(dir); c != "" {
		name = strings.TrimSuffix(filepath.Base(c), ".conf")
	}
	if vmRunningDir(dir, name) {
		die("VM_MUST_BE_STOPPED", "Stop "+name+" before "+verb+".")
	}
}

// deleteDisk: remove the VM disk + EFI vars + shortcut when the VM is
// stopped (delete_disk parity).
func deleteDisk(dir, pidFile string) int {
	if pidFile != "" {
		if b, err := os.ReadFile(pidFile); err == nil && len(b) > 0 {
			die("VM_MUST_BE_STOPPED", "Stop the VM before deleting its disk.")
		}
	}
	name := filepath.Base(dir)
	if c := vmConfPath(dir); c != "" {
		name = strings.TrimSuffix(filepath.Base(c), ".conf")
	}
	info("Deleting " + name + " virtual hard disk")
	disk := confDisk(dir)
	if disk == "" {
		info(" - disk_img not found. Doing nothing.")
		return 0
	}
	os.Remove(disk)
	// Remove any EFI vars, but not for macOS (Bash removes them
	// unconditionally — macos uses OVMF_VARS-1920x1080.fd which the
	// glob also catches, same as Bash).
	for _, pat := range []string{"OVMF_VARS*.fd", name + "-vars.fd"} {
		for _, f := range glob(dir, pat) {
			os.Remove(f)
		}
	}
	info(" - Deleted " + disk)
	shortcutDelete(name)
	okMap(map[string]any{"deleted": disk})
	return 0
}

func glob(dir, pat string) []string {
	m, _ := filepath.Glob(filepath.Join(dir, pat))
	return m
}

// ---- external snapshots (snapshots.sh + snapshots_consolidate.sh) ----

// snapshotCreateExternal: qcow2 overlay backed by the current disk,
// repoint conf disk_img at the overlay, register metadata.
// snapTagRe allowlists snapshot tags — they land in overlay
// filenames, inside the conf's disk_img="..." string, and in the
// option position of `qemu-img snapshot <flag> <tag>` — a leading
// dash makes it another option, so it is excluded by snapTagOk.
var snapTagRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func snapTagOk(tag string) bool {
	return snapTagRe.MatchString(tag) && !strings.HasPrefix(tag, "-")
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	return 1
}
