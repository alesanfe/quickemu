// quickemu-cli: the unified `quickemu` entry point in Go — replaces
// bin/quickemu + lib/quickemu/cli/*. It parses the same flag surface,
// resolves actions, and dispatches to the Go backend tools
// (launch/vm/qmp/ops/archive) — zero Bash delegation.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alesanfe/quickemu/tools/qelib"
)

func dispatch(act string) func() int {
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
		return actShutdown
	case "reset_vm":
		return actReset
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
		return func() int {
			a := []string{"delete", vmdir()}
			if trashFlag {
				a = append(a, "--trash")
			}
			if extra["plan-id"] != "" {
				a = append(a, "--plan-id="+extra["plan-id"])
			}
			return vmOp(a[0], a[1:]...)()
		}
	case "clone_vm":
		return func() int {
			a := []string{"clone", vmdir(), extra["clone"]}
			if extra["full"] != "" {
				a = append(a, "--full")
			}
			return vmOp(a[0], a[1:]...)()
		}
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
	case "delete_disk":
		return vmOp("delete-disk", vmdir(), pidPath())
	case "export_vm":
		return func() int { return vmOp("export", vmdir(), extra["export"])() }
	case "import_vm":
		return func() int { return vmOp("import", vmdir(), extra["import"])() }
	case "backup_verify":
		return func() int { return vmOp("backup-verify", vmdir(), extra["backup-verify"])() }
	case "backup_restore":
		return func() int { return vmOp("backup-restore", vmdir(), extra["backup-restore"], ".")() }
	case "backup_prune":
		return func() int { return vmOp("backup-prune", vmdir(), extra["backup-prune"])() }
	case "trash_empty":
		return vmOp("trash-empty", vmdir())
	case "trash_restore":
		return func() int { return vmOp("trash-restore", vmdir(), extra["trash-restore"])() }
	case "vfio_scan":
		return vmOp("vfio-scan", vmdir())
	case "autostart_vm":
		return vmOp("autostart", vmdir())
	case "shortcut_create":
		return func() int {
			a := []string{"shortcut", vmdir()}
			return vmOp(a[0], append(a[1:], shortcutOpts...)...)()
		}
	case "delete_shortcut":
		return vmOp("shortcut-delete", vmdir())
	case "snapshot_tree":
		return vmOp("snapshot-tree", vmdir())
	case "snapshot_verify":
		return vmOp("snapshot-verify", vmdir())
	case "snapshot_cmd":
		return actSnapshotCmd
	case "host_info":
		return actHostInfo
	case "capabilities":
		return actCapabilities
	case "inventory_show":
		return actInventory
	case "vm_stats":
		return actStats
	case "inspect_conf":
		return actInspectConf
	case "migrate_conf":
		return actMigrateConf
	case "events_read":
		return actEvents
	case "plugin_list":
		return actPlugins
	case "vm_hotplug_usb":
		return actHotplugUSB
	case "vm_hotplug_disk":
		return actHotplugDisk
	case "vm_hotunplug":
		return actHotunplug
	case "operation_status":
		return opsOp("status", "--id", extra["op-status"])
	case "operation_list":
		return opsOp("list")
	case "operation_cancel":
		return opsOp("cancel", "--id", extra["op-cancel"])
	case "operation_watch":
		return opsOp("watch", extra["op-watch"])
	case "doctor_vm":
		return actDoctor
	case "delete_plan":
		return vmOp("delete-plan", vmdir(), "--format", outFmt)
	case "ignore_msrs_always":
		return actIgnoreMSRsAlways
	case "spice_console":
		return actSpiceConsole
	case "vnc_viewer":
		return actVNC
	case "publicdir_open":
		return actPublicDir
	case "serial_console":
		return func() int { return actTelnet("serial") }
	case "monitor_console":
		return func() int { return actTelnet("monitor") }
	}
	return nil
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	args := os.Args[1:]
	// Subcommand alias: `quickemu <action> ...` -> `quickemu --<action>`.
	if !strings.HasPrefix(args[0], "-") && subcommandAliases[args[0]] {
		args[0] = "--" + args[0]
	}
	args = normalizeArgs(args)
	dryRunFlag = false
	for i := 0; i < len(args); i++ {
		a := args[i]
		val := func() string {
			if i+1 < len(args) {
				i++
				return args[i]
			}
			return ""
		}
		// scOpt accumulates the flag verbatim into the shortcut Exec line
		// (SHORTCUT_OPTIONS parity).
		scOpt := func() { shortcutOpts = append(shortcutOpts, a) }
		scOptVal := func() { shortcutOpts = append(shortcutOpts, a, val()) }
		switch a {
		case "-h", "--help":
			usage()
			return
		case "--version":
			fmt.Println(qeVersion)
			return
		case "--vm":
			vmConf = val()
		case "--format":
			// Bash parity: --format must be human|json ("text" tolerated
			// as the Go spelling of human).
			f := val()
			if f != "json" && f != "human" && f != "text" {
				fmt.Fprintln(os.Stderr, "ERROR! '--format' must be 'human' or 'json'.")
				os.Exit(1)
			}
			if f == "human" {
				f = "text"
			}
			outFmt = f
		case "--dry-run", "--echo":
			dryRunFlag = true
		case "--status":
			actions = append(actions, "status_vm")
		case "--status-quo":
			scOpt()
			confOpts["snapshot"] = "on"
		case "--capabilities":
			actions = append(actions, "capabilities")
		case "--host-info":
			actions = append(actions, "host_info")
		case "--inventory":
			actions = append(actions, "inventory_show")
		case "--stats":
			actions = append(actions, "vm_stats")
		case "--events":
			cur := "0"
			if i+1 < len(args) && isNum(args[i+1]) {
				cur = val()
			}
			extra["events"] = cur
			actions = append(actions, "events_read")
		case "--vfio-scan":
			actions = append(actions, "vfio_scan")
		case "--migrate-conf":
			actions = append(actions, "migrate_conf")
		case "--inspect-conf":
			actions = append(actions, "inspect_conf")
		case "--export":
			extra["export"] = val()
			actions = append(actions, "export_vm")
		case "--import":
			extra["import"] = val()
			actions = append(actions, "import_vm")
		case "--backup-verify":
			extra["backup-verify"] = val()
			actions = append(actions, "backup_verify")
		case "--backup-restore":
			extra["backup-restore"] = val()
			actions = append(actions, "backup_restore")
		case "--backup-prune":
			k := val()
			if k == "" {
				k = "5"
			}
			extra["backup-prune"] = k
			actions = append(actions, "backup_prune")
		case "--trash-empty":
			actions = append(actions, "trash_empty")
		case "--trash":
			trashFlag = true
		case "--trash-restore":
			extra["trash-restore"] = val()
			actions = append(actions, "trash_restore")
		case "--autostart":
			actions = append(actions, "autostart_vm")
		case "--shortcut":
			actions = append(actions, "shortcut_create")
		case "--delete-shortcut":
			actions = append(actions, "delete_shortcut")
		case "--snapshot-tree":
			actions = append(actions, "snapshot_tree")
		case "--snapshot-verify":
			actions = append(actions, "snapshot_verify")
		case "--snapshot":
			snapshotAct = val()
			// Bash options.sh parity: the flag requires an action, and
			// every action except info|list|tree|verify requires a tag.
			if snapshotAct == "" {
				fmt.Fprintln(os.Stderr, "ERROR! '--snapshot' needs an action to perform.")
				os.Exit(1)
			}
			switch snapshotAct {
			case "info", "list", "tree", "verify":
			default:
				snapshotTag = val()
				if snapshotTag == "" {
					fmt.Fprintf(os.Stderr, "ERROR! '--snapshot %s' needs a tag.\n", snapshotAct)
					os.Exit(1)
				}
			}
			actions = append(actions, "snapshot_cmd")
		case "--external":
			externalFlag = true
		case "--full":
			extra["full"] = "1"
		case "--plugin":
			os.Exit(actPlugin(val(), args[i+1:]))
		case "--plugins":
			actions = append(actions, "plugin_list")
		case "--fix":
			doctorFix = true
		case "--profile":
			confOpts["profile"] = val()
		case "--plan-id":
			extra["plan-id"] = val()
		case "--hotplug-usb":
			extra["hotplug-usb"] = val()
			actions = append(actions, "vm_hotplug_usb")
		case "--hotplug-disk":
			extra["hotplug-disk"] = val()
			actions = append(actions, "vm_hotplug_disk")
		case "--hotunplug":
			extra["hotunplug"] = val()
			actions = append(actions, "vm_hotunplug")
		case "--persist":
			persistFlag = true
		case "--offline":
			scOpt()
			confOpts["network"] = "none"
		case "--width", "--height", "--ssh-access", "--ssh-port", "--spice-port",
			"--monitor-telnet-host", "--monitor-telnet-port",
			"--serial-telnet-host", "--serial-telnet-port",
			"--keyboard", "--mouse", "--usb-controller", "--usb-passthrough",
			"--sound-card", "--sound-duplex":
			scOptVal()
			setConfOpt(strings.TrimPrefix(a, "--"), args[i])
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
			scOptVal()
			extra["monitor-cmd"] = args[i]
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
		case "--delete", "--delete-disk":
			actions = append(actions, "delete_disk")
		case "--delete-vm":
			actions = append(actions, "delete_vm")
		case "--delete-plan":
			actions = append(actions, "delete_plan")
		case "--doctor", "--check":
			actions = append(actions, "doctor_vm")
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
			extra["op-status"] = val()
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
			scOptVal()
			confOpts["display"] = args[i]
		case "--access":
			scOptVal()
			confOpts["access"] = args[i]
		case "--public-dir":
			scOptVal()
			confOpts["public_dir"] = args[i]
		case "--monitor":
			scOptVal()
			confOpts["monitor"] = args[i]
		case "--serial":
			scOptVal()
			confOpts["serial"] = args[i]
		case "--viewer":
			scOptVal()
			confOpts["viewer"] = args[i]
		case "--spice-console":
			actions = append(actions, "spice_console")
		case "--vnc":
			actions = append(actions, "vnc_viewer")
		case "--publicdir":
			actions = append(actions, "publicdir_open")
		case "--serial-console":
			actions = append(actions, "serial_console")
		case "--monitor-console":
			actions = append(actions, "monitor_console")
		case "--fullscreen", "--full-screen":
			scOpt()
			confOpts["fullscreen"] = "true"
		case "--braille":
			scOpt()
			confOpts["braille"] = "true"
		case "--keyboard_layout", "--keyboard-layout":
			scOptVal()
			confOpts["keyboard_layout"] = args[i]
		case "--cpu-pinning":
			scOptVal()
			confOpts["cpu_pinning"] = args[i]
		case "--extra_args", "--extra-args":
			scOptVal()
			// extra_args ACCUMULATES in Bash (extra_args+=), repeated
			// flags are joined with a space rather than last-wins.
			if prev := confOpts["extra_args"]; prev != "" {
				confOpts["extra_args"] = prev + " " + args[i]
			} else {
				confOpts["extra_args"] = args[i]
			}
		case "--ignore-tsc-warning":
			confOpts["ignore_tsc_warning"] = "on"
		case "--ignore-msrs-always":
			actions = append(actions, "ignore_msrs_always")
		case "--ignore-msrs":
			confOpts["ignore_msrs"] = "on"
		default:
			if strings.HasPrefix(a, "-") {
				// options.sh parity: the message and usage go to
				// stdout (echo), exit 1 — structured callers rely on
				// an empty-stderr+nonzero-exit shape to classify the
				// failure as a missing envelope.
				fmt.Printf("ERROR! \"%s\" is not a supported parameter.\n", a)
				usage()
				os.Exit(1)
			}
		}
	}
	// commands.sh param-checks: resolution/access/network validation on
	// the merged CLI overrides — errors before any action runs.
	if errs := qelib.ParamCheckConf(confOpts); len(errs) > 0 {
		for _, e := range errs {
			fmt.Println("ERROR! " + e)
		}
		os.Exit(1)
	}
	if vmConf == "" && len(actions) == 0 {
		usage()
		os.Exit(2)
	}
	// --format propagates to delegated tools via the env (Bash reads
	// OUTPUT_FORMAT the same way; quickemu-vm honors it too).
	if outFmt == "json" {
		os.Setenv("OUTPUT_FORMAT", "json")
	}
	// Fixes running VMs when PWD is not relative to the VM directory
	// (bin/quickemu.bash parity): when the configured disk is not
	// reachable from the cwd, operate from the conf's directory so
	// relative paths in the conf resolve the way Bash resolves them.
	if vmConf != "" {
		if abs, err := filepath.Abs(vmConf); err == nil {
			vmConf = abs
		}
		// conf-load parity: Bash dies here ("Virtual machine
		// configuration not found.") before any action runs — the
		// message goes to stdout like every Bash echo.
		if _, err := os.Stat(vmConf); err != nil {
			fmt.Println("ERROR! Virtual machine configuration not found.")
			os.Exit(1)
		}
		disk := confKV()["disk_img"]
		if disk == "" {
			df := confKV()["disk_format"]
			if df == "" {
				df = "qcow2"
			}
			disk = filepath.Join(vmdir(), "disk."+df)
		}
		if _, err := os.Stat(disk); err != nil {
			os.Chdir(filepath.Dir(vmConf))
		}
	}
	// default action: boot the VM (dry-run prints the argv)
	if len(actions) == 0 {
		if dryRunFlag {
			os.Exit(actDryRun())
		}
		os.Exit(actLaunch())
	}
	// Bash main flow: a stale pidfile means an unclean shutdown — clean
	// runtime artefacts before running any action (vm_runtime_cleanup).
	runtimeCleanup()
	rcv := 0
	for _, act := range actions {
		if act == "start" {
			if dryRunFlag {
				rcv = actDryRun()
			} else {
				rcv = actLaunch()
			}
			continue
		}
		// Host-level actions do not need --vm (early-action parity:
		// Bash runs capabilities|list_vms|import_vm before conf load;
		// the rest are host-scope ops that only read machine state).
		hostOnly := map[string]bool{
			"capabilities": true, "host_info": true, "list_vms": true,
			"inventory_show": true, "plugin_list": true, "import_vm": true,
			"trash_list": true, "trash_empty": true, "trash_restore": true,
			"vfio_scan": true,
		}
		if vmConf == "" && !hostOnly[act] {
			fmt.Fprintln(os.Stderr, "ERROR! --vm is required for "+act)
			rcv = 2
			continue
		}
		// vm_lock parity: Bash runs mutating actions under the
		// exclusive VM lock (kill|pause|resume|shutdown|reset|
		// send_keys|type_text|screenshot; vm/fs ops lock inside vmOp).
		locked := map[string]bool{
			"kill_vm": true, "pause_vm": true, "resume_vm": true,
			"shutdown_vm": true, "reset_vm": true, "stop_vm": true,
			"send_keys": true, "type_text": true, "screenshot_vm": true,
			"monitor_cmd": true, // HMP socket is single-client like QMP
		}
		if fn := dispatch(act); fn != nil {
			if locked[act] && !vmLockAcquire() {
				rcv = 1
				continue
			}
			r := fn()
			if locked[act] {
				vmLockRelease()
			}
			if r != 0 {
				rcv = r
			}
		}
	}
	os.Exit(rcv)
}
