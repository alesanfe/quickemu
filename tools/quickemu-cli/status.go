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
		ops = "pause resume shutdown reset kill status monitor-cmd screenshot send-keys type snapshot-clone export"
	case "paused":
		ops = "resume shutdown kill status monitor-cmd screenshot"
	case "stopped", "stale":
		ops = "start delete clone rename export import snapshot apply create delete info list inspect-conf delete-plan"
	case "archived":
		ops = "restore trash-list status inspect-conf delete"
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
		emitOK("vm.status", map[string]any{"name": vmname(), "state": st,
			"pid": p, "allowedOps": allowedOps(st)})
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
			st, pid := state()
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
