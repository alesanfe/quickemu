package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// allowedOps: vm_allowed_ops parity — the ops the UI may offer per
// state, as a whitespace-separated list split into a JSON array.
func allowedOps(state string) []string {
	var ops string
	switch state {
	case "running":
		ops = "pause resume shutdown powerdown stop kill reset suspend restart " +
			"monitor-cmd send-keys type screenshot " +
			"viewer vnc spice-console serial-console monitor-console fullscreen " +
			"ssh sftp scp ssh-config " +
			"guest-ping guest-info guest-exec guest-push guest-pull guest-ip " +
			"guest-shutdown guest-reboot guest-halt guest-fsfreeze " +
			"hotplug-usb hotplug-disk hotunplug " +
			"snapshot-create snapshot-apply snapshot-delete snapshot-list " +
			"snapshot-tree snapshot-verify snapshot-info snapshot-policy snapshot-consolidate " +
			"backup backup-incr backup-incr-list backup-restore backup-verify backup-prune " +
			"status status-quo publicdir stats events " +
			"op-list op-status op-cancel op-watch " +
			"ignore-msrs-always migrate-conf"
	case "paused":
		ops = "resume shutdown stop kill reset monitor-cmd screenshot " +
			"status publicdir stats events op-list op-status op-cancel op-watch"
	case "suspended":
		ops = "resume status inspect-conf delete-vm trash"
	case "stopped", "stale":
		ops = "start clone linked-clone rename move export import " +
			"delete-vm delete-disk trash trash-list trash-restore trash-empty delete-plan " +
			"autostart shortcut delete-shortcut create-disk disk-resize disk-resize-commit disk-compact " +
			"snapshot-create snapshot-apply snapshot-delete snapshot-list " +
			"snapshot-tree snapshot-verify snapshot-info snapshot-policy snapshot-consolidate " +
			"backup backup-incr-list backup-restore backup-verify backup-prune " +
			"inspect-conf migrate-conf doctor profile status status-quo publicdir " +
			"ssh-config ignore-msrs ignore-msrs-always stats events " +
			"op-list op-status op-cancel op-watch"
	case "crashed", "error":
		ops = "stop kill start status inspect-conf delete-vm trash"
	case "archived":
		ops = "restore trash-list status inspect-conf delete-vm"
	case "unknown":
		ops = "status inspect-conf"
	default:
		ops = "status"
	}
	return strings.Fields(ops)
}

func actStatus() int {
	st, pid := state()
	if outFmt == "json" {
		var p any
		if pid > 0 {
			p = pid
		}
		d := map[string]any{"name": vmname(), "state": st,
			"pid": p, "allowedOps": allowedOps(st)}
		// QE-016: guest-agent addresses ride along in the JSON
		// status when the agent is reachable (best-effort, never
		// stalls a stopped/agentless VM).
		if g := guestStatusInfo(); g != nil {
			d["guest"] = g
		}
		emitOK("vm.status", d)
		return 0
	}
	if pid > 0 {
		fmt.Printf("%s %d\n", st, pid)
	} else {
		fmt.Println(st)
	}
	return 0
}

func actListVMs() int {
	type ent struct {
		Name  string `json:"name"`
		OS    string `json:"guest_os"`
		State string `json:"state"`
		PID   any    `json:"pid"`
	}
	vms := []ent{}
	// state() reads the global vmConf — a listing must not leak its
	// scratch path into later actions on the same command line.
	savedConf := vmConf
	defer func() { vmConf = savedConf }()
	// A VM may appear twice — once as ./name.conf and once as
	// name/name.conf (quickget layout). Identity is the basename, so
	// dedupe by it; top-level confs list first.
	seen := map[string]bool{}
	for _, pat := range []string{"*.conf", "*/*.conf"} {
		for _, f := range glob(pat) {
			name := strings.TrimSuffix(filepath.Base(f), ".conf")
			if seen[name] {
				continue
			}
			seen[name] = true
			gos := "linux"
			if b, err := os.ReadFile(f); err == nil {
				for _, l := range strings.Split(string(b), "\n") {
					if kv := strings.SplitN(l, "=", 2); len(kv) == 2 &&
						strings.TrimSpace(kv[0]) == "guest_os" {
						gos = strings.Trim(strings.TrimSpace(kv[1]), `"' `)
					}
				}
			}
			vmConf = f
			// list_vms uses the light pidfile model (alive + .paused →
			// running/paused, dead pidfile → stale), not the heavier
			// vm_state resolution --status performs.
			st, pid := "stopped", 0
			if _, err := os.Stat(pidPath()); err == nil {
				if vmPidAlive() {
					pid = readPID()
					if paused() {
						st = "paused"
					} else {
						st = "running"
					}
				} else {
					st = "stale"
				}
			}
			var p any
			if pid > 0 {
				p = pid
			}
			vms = append(vms, ent{name, gos, st, p})
		}
	}
	if outFmt == "json" {
		emitOK("vm.list", map[string]any{"vms": vms})
		return 0
	}
	fmt.Printf("%-28s %-16s %-9s %s\n", "VM", "GUEST OS", "STATE", "PID")
	for _, v := range vms {
		fmt.Printf("%-28s %-16s %-9s %v\n", v.Name, v.OS, v.State, v.PID)
	}
	return 0
}
