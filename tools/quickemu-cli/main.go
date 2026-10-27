// quickemu-cli: the unified `quickemu` entry point in Go — replaces
// bin/quickemu + lib/quickemu/cli/*. It parses the same flag surface,
// resolves actions, and dispatches to the Go backend tools
// (launch/vm/qmp/ops/archive) — zero Bash delegation.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

var (
	vmConf   string
	outFmt   = "text"
	actions  []string
	confOpts = map[string]string{} // CLI overrides merged into the conf
	extra    = map[string]string{} // action args (--monitor-cmd value etc.)
)

func tool(name string, args ...string) (string, error) {
	bin, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s: not on PATH", name)
	}
	out, err := exec.Command(bin, args...).CombinedOutput()
	return string(out), err
}

func vmdir() string { return filepath.Dir(vmConf) }
func vmname() string {
	return strings.TrimSuffix(filepath.Base(vmConf), filepath.Ext(vmConf))
}
func qmpSock() string { return filepath.Join(vmdir(), vmname()+"-qmp.socket") }
func pidPath() string { return filepath.Join(vmdir(), vmname()+".pid") }

// qmp sends a QMP command via quickemu-qmp execute.
func qmp(cmd string) (string, error) {
	return tool("quickemu-qmp", "execute", "--socket", qmpSock(),
		"--command", `{"execute":"`+cmd+`"}`)
}

// hmp sends a human-monitor-command (sendkey/type/screendump).
func hmp(cmd string) (string, error) {
	payload := `{"execute":"human-monitor-command","arguments":{"command-line":"` + cmd + `"}}`
	return tool("quickemu-qmp", "execute", "--socket", qmpSock(), "--command", payload)
}

func readPID() int {
	b, err := os.ReadFile(pidPath())
	if err != nil {
		return 0
	}
	pid := 0
	fmt.Sscanf(strings.TrimSpace(string(b)), "%d", &pid)
	return pid
}

func alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	return err == nil && p.Signal(syscall.Signal(0)) == nil
}

func paused() bool {
	_, err := os.Stat(filepath.Join(vmdir(), vmname()+".paused"))
	return err == nil
}

func state() (string, int) {
	pid := readPID()
	if !alive(pid) {
		if pid > 0 {
			return "stale", 0
		}
		return "stopped", 0
	}
	if paused() {
		return "paused", pid
	}
	return "running", pid
}

func emitOK(cmd string, data any) {
	b, _ := json.Marshal(map[string]any{"schemaVersion": 1, "success": true, "command": cmd, "data": data})
	fmt.Println(string(b))
}

// ---------- action implementations ----------

func actStatus() int {
	st, pid := state()
	if outFmt == "json" {
		emitOK("vm.status", map[string]any{"name": vmname(), "state": st, "pid": pid})
		return 0
	}
	if pid > 0 {
		fmt.Printf("%s %d\n", st, pid)
	} else {
		fmt.Println(st)
	}
	return 0
}

func glob(pat string) []string {
	m, _ := filepath.Glob(pat)
	return m
}

func actListVMs() int {
	type ent struct {
		Name, OS, State string
		PID             any
	}
	var vms []ent
	for _, pat := range []string{"*.conf", "*/*.conf"} {
		for _, f := range glob(pat) {
			name := strings.TrimSuffix(filepath.Base(f), ".conf")
			gos := "linux"
			if b, err := os.ReadFile(f); err == nil {
				for _, l := range strings.Split(string(b), "\n") {
					if strings.HasPrefix(l, "guest_os") {
						gos = strings.Trim(strings.SplitN(l, "=", 2)[1], `"' `)
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

func actPause() int {
	out, err := qmp("stop")
	if err == nil {
		os.WriteFile(filepath.Join(vmdir(), vmname()+".paused"), nil, 0644)
		fmt.Println(" - " + vmname() + " paused.")
	}
	fmt.Print(out)
	return rc(err)
}

func actResume() int {
	out, err := qmp("cont")
	if err == nil {
		os.Remove(filepath.Join(vmdir(), vmname()+".paused"))
		fmt.Println(" - " + vmname() + " resumed.")
	}
	fmt.Print(out)
	return rc(err)
}

func actMonitorCmd() int {
	out, err := hmp(extra["monitor-cmd"])
	fmt.Print(out)
	return rc(err)
}

func actSendKeys() int {
	out, err := hmp("sendkey " + extra["send-keys"])
	fmt.Print(out)
	return rc(err)
}

func actType() int {
	for _, ch := range extra["type"] {
		k := string(ch)
		switch {
		case ch >= 'A' && ch <= 'Z':
			k = "shift-" + strings.ToLower(k)
		case ch == ' ':
			k = "spc"
		case ch == '-':
			k = "minus"
		}
		hmp("sendkey " + k)
		time.Sleep(60 * time.Millisecond)
	}
	return 0
}

func actScreenshot() int {
	dst := extra["screenshot"]
	if dst == "" {
		dst = filepath.Join(vmdir(), vmname()+".ppm")
	}
	out, err := hmp("screendump " + dst)
	fmt.Print(out)
	return rc(err)
}

// vmOp runs `quickemu-vm <sub> <rest...>`; args are produced lazily at
// dispatch time so vmdir()/vmname() see the parsed --vm flag.
func vmOp(sub string, rest ...string) func() int {
	return func() int {
		out, err := tool("quickemu-vm", append([]string{sub}, rest...)...)
		fmt.Print(out)
		return rc(err)
	}
}

func opsOp(sub string, rest ...string) func() int {
	return func() int {
		out, err := tool("quickemu-ops", append([]string{sub, "--vmdir", vmdir()}, rest...)...)
		fmt.Print(out)
		return rc(err)
	}
}

func qmpOp(cmd, okMsg func() string) func() int {
	return func() int {
		out, err := qmp(cmd())
		fmt.Print(out)
		if err == nil && okMsg != nil {
			fmt.Println(okMsg())
		}
		return rc(err)
	}
}

func actStop(force bool) int {
	st, pid := state()
	if st != "running" && st != "paused" {
		fmt.Println(vmname() + " is not running")
		return 0
	}
	if force {
		if p, err := os.FindProcess(pid); err == nil {
			p.Signal(syscall.SIGKILL)
		}
	} else {
		qmp("quit")
	}
	for i := 0; i < 40 && alive(pid); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	for _, ext := range []string{".pid", ".paused", "-qmp.socket", "-monitor.socket", "-serial.socket"} {
		os.Remove(filepath.Join(vmdir(), vmname()+ext))
	}
	fmt.Println(" - " + vmname() + " stopped.")
	return 0
}

func actLaunch() int {
	conf := applyOverrides()
	out, err := tool("quickemu-launch", conf, "--run")
	fmt.Print(out)
	return rc(err)
}

func actDryRun() int {
	conf := applyOverrides()
	out, err := tool("quickemu-launch", conf)
	fmt.Print(out)
	return rc(err)
}

// applyOverrides writes CLI conf overrides into a temp conf alongside
// the original (Bash exports them as env vars; quickemu-launch reads
// only the file, so overrides are materialised there).
func applyOverrides() string {
	if len(confOpts) == 0 {
		return vmConf
	}
	b, _ := os.ReadFile(vmConf)
	c := string(b)
	for k, v := range confOpts {
		kv := k + `="` + v + `"`
		if strings.Contains(c, k+"=") {
			lines := strings.Split(c, "\n")
			for i, l := range lines {
				if strings.HasPrefix(l, k+"=") {
					lines[i] = kv
				}
			}
			c = strings.Join(lines, "\n")
		} else {
			c += kv + "\n"
		}
	}
	tmp := filepath.Join(vmdir(), "."+vmname()+".cli.conf")
	os.WriteFile(tmp, []byte(c), 0644)
	return tmp
}

func rc(err error) int {
	if err != nil {
		return 1
	}
	return 0
}

func dispatch(act string) func() int {
	msg := func(s string) func() string { return func() string { return s } }
	switch act {
	case "status_vm":
		return actStatus
	case "list_vms":
		return actListVMs
	case "pause_vm":
		return actPause
	case "resume_vm":
		return actResume
	case "shutdown_vm":
		return qmpOp(msg("system_powerdown"), func() string { return " - " + vmname() + " shutting down." })
	case "reset_vm":
		return qmpOp(msg("system_reset"), nil)
	case "kill_vm":
		return func() int { return actStop(true) }
	case "stop_vm":
		return func() int { return actStop(false) }
	case "monitor_cmd":
		return actMonitorCmd
	case "send_keys":
		return actSendKeys
	case "type_text":
		return actType
	case "screenshot_vm":
		return actScreenshot
	case "delete_vm":
		return vmOp("delete", vmdir())
	case "clone_vm":
		return vmOp("clone", vmdir(), extra["clone"])
	case "linked_clone_vm":
		return vmOp("linked-clone", vmdir(), extra["linked-clone"])
	case "rename_vm":
		return vmOp("rename", vmdir(), extra["rename"])
	case "move_vm":
		return vmOp("move", vmdir(), extra["move"])
	case "backup_vm":
		return vmOp("backup", vmdir())
	case "backup_list":
		return vmOp("backup-list", vmdir())
	case "trash_list":
		return vmOp("trash-list", vmdir())
	case "snapshot_create":
		return vmOp("snapshot-create", vmdir(), extra["snapshot-create"])
	case "snapshot_apply":
		return vmOp("snapshot-apply", vmdir(), extra["snapshot-apply"])
	case "snapshot_delete":
		return vmOp("snapshot-delete", vmdir(), extra["snapshot-delete"])
	case "snapshot_list":
		return vmOp("snapshot-list", vmdir())
	case "disk_compact":
		return vmOp("compact", vmdir())
	case "disk_resize":
		return vmOp("resize", vmdir(), extra["disk-resize"])
	case "operation_status":
		return opsOp("status")
	case "operation_list":
		return opsOp("list")
	case "operation_cancel":
		return opsOp("cancel", extra["op-cancel"])
	case "operation_watch":
		return opsOp("watch", extra["op-watch"])
	}
	return nil
}

func usage() {
	fmt.Println(`quickemu — create and control VMs (Go CLI)
Usage: quickemu --vm <conf> [action flags]

Actions: --start --dry-run --status --pause --resume --shutdown
  --reset --stop --kill --restart --list-vms --delete-vm
  --clone <name> --rename <name> --move <dir> --backup --backup-list
  --snapshot-create <tag> --snapshot-apply <tag> --snapshot-delete <tag>
  --snapshot-list --monitor-cmd <c> --send-keys <k> --type <t>
  --screenshot [file] --disk-compact --op-list --op-status
Conf overrides: --display --access --public-dir --format json|text`)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	args := os.Args[1:]
	dryRun := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		val := func() string {
			if i+1 < len(args) {
				i++
				return args[i]
			}
			return ""
		}
		switch a {
		case "-h", "--help":
			usage()
			return
		case "--version":
			fmt.Println("5.0.0")
			return
		case "--vm", "-vm":
			vmConf = val()
		case "--format":
			outFmt = val()
		case "--dry-run":
			dryRun = true
		case "--status", "--status-quo":
			actions = append(actions, "status_vm")
		case "--list-vms", "--list":
			actions = append(actions, "list_vms")
		case "--pause":
			actions = append(actions, "pause_vm")
		case "--resume":
			actions = append(actions, "resume_vm")
		case "--shutdown":
			actions = append(actions, "shutdown_vm")
		case "--reset":
			actions = append(actions, "reset_vm")
		case "--stop":
			actions = append(actions, "stop_vm")
		case "--kill":
			actions = append(actions, "kill_vm")
		case "--restart":
			actions = append(actions, "stop_vm", "start")
		case "--start":
			actions = append(actions, "start")
		case "--monitor-cmd":
			extra["monitor-cmd"] = val()
			actions = append(actions, "monitor_cmd")
		case "--send-keys":
			extra["send-keys"] = val()
			actions = append(actions, "send_keys")
		case "--type":
			extra["type"] = val()
			actions = append(actions, "type_text")
		case "--screenshot":
			extra["screenshot"] = ""
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				extra["screenshot"] = val()
			}
			actions = append(actions, "screenshot_vm")
		case "--delete-vm":
			actions = append(actions, "delete_vm")
		case "--clone":
			extra["clone"] = val()
			actions = append(actions, "clone_vm")
		case "--linked-clone":
			extra["linked-clone"] = val()
			actions = append(actions, "linked_clone_vm")
		case "--rename":
			extra["rename"] = val()
			actions = append(actions, "rename_vm")
		case "--move":
			extra["move"] = val()
			actions = append(actions, "move_vm")
		case "--backup":
			actions = append(actions, "backup_vm")
		case "--backup-list":
			actions = append(actions, "backup_list")
		case "--trash-list":
			actions = append(actions, "trash_list")
		case "--snapshot-create":
			extra["snapshot-create"] = val()
			actions = append(actions, "snapshot_create")
		case "--snapshot-apply":
			extra["snapshot-apply"] = val()
			actions = append(actions, "snapshot_apply")
		case "--snapshot-delete":
			extra["snapshot-delete"] = val()
			actions = append(actions, "snapshot_delete")
		case "--snapshot-list":
			actions = append(actions, "snapshot_list")
		case "--disk-compact":
			actions = append(actions, "disk_compact")
		case "--disk-resize":
			extra["disk-resize"] = val()
			actions = append(actions, "disk_resize")
		case "--op-status", "--operation-status":
			actions = append(actions, "operation_status")
		case "--op-list", "--operation-list":
			actions = append(actions, "operation_list")
		case "--op-cancel", "--operation-cancel":
			extra["op-cancel"] = val()
			actions = append(actions, "operation_cancel")
		case "--op-watch", "--operation-watch":
			extra["op-watch"] = val()
			actions = append(actions, "operation_watch")
		// conf overrides consumed by quickemu-launch
		case "--display":
			confOpts["display"] = val()
		case "--access":
			confOpts["access"] = val()
		case "--public-dir":
			confOpts["public_dir"] = val()
		case "--monitor":
			confOpts["monitor"] = val()
		case "--serial":
			confOpts["serial"] = val()
		case "--viewer":
			confOpts["viewer"] = val()
		case "--fullscreen", "--full-screen":
			confOpts["fullscreen"] = "true"
		case "--braille":
			confOpts["braille"] = "true"
		case "--keyboard_layout", "--keyboard-layout":
			confOpts["keyboard_layout"] = val()
		case "--cpu-pinning":
			confOpts["cpu_pinning"] = val()
		case "--extra_args", "--extra-args":
			confOpts["extra_args"] = val()
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(os.Stderr, "ERROR! Not supported argument '%s'\n", a)
				os.Exit(2)
			}
		}
	}
	if vmConf == "" && len(actions) == 0 {
		usage()
		os.Exit(2)
	}
	// default action: boot the VM (dry-run prints the argv)
	if len(actions) == 0 {
		if dryRun {
			os.Exit(actDryRun())
		}
		os.Exit(actLaunch())
	}
	rcv := 0
	for _, act := range actions {
		if act == "start" {
			if dryRun {
				rcv = actDryRun()
			} else {
				rcv = actLaunch()
			}
			continue
		}
		if vmConf == "" && act != "list_vms" {
			fmt.Fprintln(os.Stderr, "ERROR! --vm is required for "+act)
			rcv = 2
			continue
		}
		if fn := dispatch(act); fn != nil {
			if r := fn(); r != 0 {
				rcv = r
			}
		}
	}
	os.Exit(rcv)
}
