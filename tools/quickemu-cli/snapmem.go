// snapmem.go: QE-021 — memory snapshots (disk+vmstate inside qcow2)
// and QE-022 policy routing. Disk-only snapshots stay in
// quickemu-vm (qemu-img); memory snapshots need the live monitor,
// which only exists in this process.
//
//	create --memory [--quiesce] → monitor savevm
//	apply  (vmstate snapshot)   → monitor loadvm / -loadvm cold boot
//	delete (vmstate snapshot)   → monitor delvm / qemu-img -d
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"syscall"
	"time"
)

var memSnapTagRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// hmpOut sends one human-monitor-command line and returns its text
// output ("" when the command produced none — HMP savevm answers
// silence on success).
func hmpOut(line string) (string, error) {
	out, err := qmpRaw(qmpCmd("human-monitor-command",
		map[string]any{"command-line": line}))
	if err != nil {
		return "", err
	}
	var rep struct {
		Data struct {
			Return string `json:"return"`
		} `json:"data"`
	}
	if json.Unmarshal([]byte(out), &rep) != nil {
		return "", fmt.Errorf("bad monitor reply: %s", strings.TrimSpace(out))
	}
	return rep.Data.Return, nil
}

// snapshotHasVMState reports whether <tag> carries guest RAM inside
// the disk — qemu-img info's vm-state-size is authoritative.
func snapshotHasVMState(tag string) (bool, error) {
	out, err := tool("quickemu-vm", "snapshot-list-json", vmdir())
	if err != nil {
		return false, err
	}
	var env struct {
		Data struct {
			Snapshots []struct {
				Name      string  `json:"name"`
				VMStateSz float64 `json:"vm-state-size"`
			} `json:"snapshots"`
		} `json:"data"`
	}
	if json.Unmarshal([]byte(out), &env) != nil {
		return false, fmt.Errorf("bad snapshot list")
	}
	for _, s := range env.Data.Snapshots {
		if s.Name == tag {
			return s.VMStateSz > 0, nil
		}
	}
	return false, fmt.Errorf("snapshot %q not found", tag)
}

// snapshotDiskFree: bytes free on the filesystem holding the disk —
// savevm appends ~RAM-size state; refuse when clearly short.
func snapshotDiskFree() int64 {
	var st syscall.Statfs_t
	if syscall.Statfs(vmdir(), &st) != nil {
		return -1
	}
	return int64(st.Bavail) * int64(st.Bsize)
}

// actSnapshotMemCreate: `savevm <tag>` on a running VM. --quiesce
// freezes the guest filesystems first (thaw is guaranteed by the
// client's watchdog even when savevm fails).
func actSnapshotMemCreate() int {
	tag := snapshotTag
	if !memSnapTagRe.MatchString(tag) {
		fmt.Println(" - snapshot: tag must match [A-Za-z0-9._-]+")
		return 1
	}
	if !requireRunning() {
		return 1
	}
	if fmt_ := confKV()["disk_format"]; fmt_ != "" && fmt_ != "qcow2" {
		fmt.Printf(" - snapshot: --memory needs qcow2 disks (disk_format=%q)\n", fmt_)
		return 1
	}
	// Space gate: vmstate is roughly the guest RAM; warn-and-refuse
	// when the filesystem cannot possibly hold it.
	if free := snapshotDiskFree(); free >= 0 {
		var ramMB int64
		fmt.Sscan(confKV()["memory"], &ramMB)
		if ramMB > 0 && free < ramMB*(1<<20) {
			fmt.Printf(" - snapshot: %d MiB free, guest RAM is %d MiB — not enough space for vmstate.\n",
				free/(1<<20), ramMB)
			return 1
		}
	}
	do := func(ctx context.Context) error {
		out, err := hmpOut("savevm " + tag)
		if err != nil {
			return err
		}
		if strings.Contains(out, "Error") || strings.Contains(out, "error") {
			return fmt.Errorf("savevm: %s", strings.TrimSpace(out))
		}
		return nil
	}
	ctx, cancel := guestCtx(2 * time.Minute)
	defer cancel()
	var err error
	if quiesceFlag {
		cl, derr := guestDial(8 * time.Second)
		if derr != nil {
			fmt.Printf(" - snapshot: --quiesce needs the guest agent: %v\n", derr)
			return 1
		}
		err = cl.WithFreeze(ctx, 2*time.Minute, do)
		cl.Close()
	} else {
		err = do(ctx)
	}
	if err != nil {
		fmt.Printf(" - snapshot: %v\n", err)
		return 1
	}
	// Record in the sidecar registry so tree/policy see the type.
	vmOp("snapshot-register", vmdir(), tag, "vmstate", snapshotDesc)()
	qeEvent("vm.snapshot.create", fmt.Sprintf(`{"tag":%q,"memory":true}`, tag))
	fmt.Printf(" - Created memory snapshot '%s' for %s.\n", tag, vmname())
	if outFmt == "json" {
		emitOK("vm.snapshot.create", map[string]any{"vm": vmname(), "tag": tag, "memory": true})
	}
	return 0
}

// actSnapshotMemApply: revert to a vmstate snapshot. Running VM →
// monitor loadvm (instant revert). Stopped VM → cold boot with
// -loadvm <tag> via the conf override channel.
func actSnapshotMemApply() int {
	tag := snapshotTag
	if vmRunning() {
		out, err := hmpOut("loadvm " + tag)
		if err != nil || strings.Contains(out, "Error") || strings.Contains(out, "error") {
			msg := strings.TrimSpace(out)
			if err != nil {
				msg = err.Error()
			}
			fmt.Printf(" - snapshot apply: %s\n", msg)
			return 1
		}
		fmt.Printf(" - %s reverted to snapshot '%s'.\n", vmname(), tag)
		if outFmt == "json" {
			emitOK("vm.snapshot.apply", map[string]any{"vm": vmname(), "tag": tag})
		}
		return 0
	}
	// Stopped: boot straight into the snapshot. The launcher emits
	// -loadvm and the FSM walks the normal boot arc (preparing →
	// starting → running) — stStopped→stStarting is not a legal arc.
	if !machineTransition(stPreparing, "snapshot-apply", "quickemu-cli") ||
		!machineTransition(stStarting, "snapshot-apply", "quickemu-cli") {
		fmt.Println(" - snapshot apply: cannot boot from the current state.")
		return 1
	}
	confOpts["loadvm"] = tag
	out, err := tool("quickemu-launch", applyOverrides(), "--run")
	fmt.Print(out)
	if err != nil {
		machineTransition(stStopped, "snapshot-apply-failed", "quickemu-cli")
		return 1
	}
	machineTransition(stRunning, "snapshot-applied", "quickemu-cli")
	qeEvent("vm.snapshot.apply", fmt.Sprintf(`{"tag":%q}`, tag))
	fmt.Printf(" - %s booted into snapshot '%s'.\n", vmname(), tag)
	return 0
}

// actSnapshotMemDelete: delvm on a running VM (qemu-img -d refuses
// on a live image); stopped VMs keep the qemu-img path.
func actSnapshotMemDelete() int {
	tag := snapshotTag
	if !vmRunning() {
		return vmOp("snapshot-delete", vmdir(), tag)()
	}
	out, err := hmpOut("delvm " + tag)
	if err != nil || strings.Contains(out, "Error") || strings.Contains(out, "error") {
		msg := strings.TrimSpace(out)
		if err != nil {
			msg = err.Error()
		}
		fmt.Printf(" - snapshot delete: %s\n", msg)
		return 1
	}
	vmOp("snapshot-unregister", vmdir(), tag)()
	fmt.Printf(" - Deleted snapshot '%s' from %s.\n", tag, vmname())
	if outFmt == "json" {
		emitOK("vm.snapshot.delete", map[string]any{"vm": vmname(), "tag": tag})
	}
	return 0
}

// snapshotIsMem: branch helper — a vmstate tag diverts apply/delete
// from the qemu-img path. Errors answer false (unknown = disk-only;
// qemu-img emits its own diagnostics).
func snapshotIsMem(tag string) bool {
	mem, err := snapshotHasVMState(tag)
	return err == nil && mem
}

// actSnapshotPolicy: QE-022 — the policy engine lives in
// quickemu-vm (qemu-img internals); the CLI only routes.
func actSnapshotPolicy() int {
	verb := policyVerb
	if verb == "" {
		verb = "run"
	}
	args := []string{"snapshot-policy", vmdir(), verb}
	if dryRunFlag {
		args = append(args, "--dry")
	}
	return vmOp(args[0], args[1:]...)()
}
