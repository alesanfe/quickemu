// args_parse.go: the argv flag scan (options.sh parity), split into
// per-family helpers. parseArgs is a thin router; every flag lives in
// exactly one helper, so dispatch order is irrelevant.
package main

import (
	"fmt"
	"os"
	"strings"
)

// argParser: cursor state for one parseArgs pass. val() shifts the
// NEXT token as the current flag's value (Bash option-argument
// consumption — normalizeArgs already split --flag=value into two
// tokens, so values are always a separate argv element).
type argParser struct {
	args []string
	i    int
}

// val consumes and returns the next token ("" at end of argv).
func (p *argParser) val() string {
	if p.i+1 < len(p.args) {
		p.i++
		return p.args[p.i]
	}
	return ""
}

// scOpt accumulates the flag verbatim into the shortcut Exec line
// (SHORTCUT_OPTIONS parity).
func (p *argParser) scOpt() { shortcutOpts = append(shortcutOpts, p.args[p.i]) }

// scOptVal is scOpt plus the flag's consumed value.
func (p *argParser) scOptVal() {
	shortcutOpts = append(shortcutOpts, p.args[p.i], p.val())
}

// parseArgs: options.sh parity — one pass over argv filling the
// actions list, confOpts overrides, extra action args and the flag
// globals. Unknown "-" tokens error to stdout with usage, exit 1.
func parseArgs(args []string) {
	// `quickemu guest <verb> ...` / `quickemu ip ...` — the QGA
	// subcommand parses in guest_parse.go before the flag scan.
	if guestIntercept(args) {
		return
	}
	// `quickemu ssh|scp|sftp|ssh-config|ssh-forget ...` — QE-030–033;
	// ssh_parse.go owns the verb table.
	if sshIntercept(args) {
		return
	}
	// `quickemu config <verb> ...` — QE-111/112: config-file
	// operations (migrate/set/get/validate) delegate to the
	// quickemu-toml sibling; see config_migrate.go.
	if args[0] == "config" {
		os.Exit(actConfig(args[1:]))
	}
	// Subcommand alias: `quickemu <action> ...` -> `quickemu --<action>`.
	if !strings.HasPrefix(args[0], "-") && subcommandAliases[args[0]] {
		args[0] = "--" + args[0]
	}
	args = normalizeArgs(args)
	dryRunFlag = false
	p := &argParser{args: args}
	for ; p.i < len(p.args); p.i++ {
		a := p.args[p.i]
		switch {
		case parseMetaFlags(p, a):
		case parseInfoFlags(p, a):
		case parseBackupFlags(p, a):
		case parseSnapshotFlags(p, a):
		case parseVMOpFlags(p, a):
		case parseOpsFlags(p, a):
		case parseLifecycleFlags(p, a):
		case parseMonitorFlags(p, a):
		case parseConfFlags(p, a):
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
}

// parseMetaFlags: process-wide flags — immediate exits (-h/--version),
// the --vm target, --format/--dry-run parse modifiers and global
// option state (--external/--full/--fix/--profile/--plan-id/--persist).
func parseMetaFlags(p *argParser, a string) bool {
	switch a {
	case "-h", "--help":
		usage()
		os.Exit(0)
	case "--version":
		fmt.Println(qeVersion)
		os.Exit(0)
	case "--vm":
		vmConf = p.val()
	case "--format":
		// Bash parity: --format must be human|json ("text" tolerated
		// as the Go spelling of human).
		f := p.val()
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
	case "--external":
		externalFlag = true
	case "--full":
		extra["full"] = "1"
	case "--plugin":
		os.Exit(actPlugin(p.val(), p.args[p.i+1:]))
	case "--fix":
		doctorFix = true
	case "--profile":
		confOpts["profile"] = p.val()
	case "--plan-id":
		extra["plan-id"] = p.val()
	case "--persist":
		persistFlag = true
	default:
		return false
	}
	return true
}

// parseInfoFlags: read-only/host-scope action flags.
func parseInfoFlags(p *argParser, a string) bool {
	switch a {
	case "--status":
		actions = append(actions, "status_vm")
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
		if p.i+1 < len(p.args) && isNum(p.args[p.i+1]) {
			cur = p.val()
		}
		extra["events"] = cur
		actions = append(actions, "events_read")
	case "--vfio-scan":
		actions = append(actions, "vfio_scan")
	case "--migrate-conf":
		actions = append(actions, "migrate_conf")
	case "--inspect-conf":
		actions = append(actions, "inspect_conf")
	case "--plugins":
		actions = append(actions, "plugin_list")
	case "--list-vms", "--list":
		actions = append(actions, "list_vms")
	case "--doctor", "--check":
		actions = append(actions, "doctor_vm")
	case "--ignore-msrs-always":
		actions = append(actions, "ignore_msrs_always")
	default:
		return false
	}
	return true
}

// parseBackupFlags: export/import, backup, trash and shortcut actions.
func parseBackupFlags(p *argParser, a string) bool {
	switch a {
	case "--export":
		extra["export"] = p.val()
		actions = append(actions, "export_vm")
	case "--import":
		extra["import"] = p.val()
		actions = append(actions, "import_vm")
	case "--backup-verify":
		extra["backup-verify"] = p.val()
		actions = append(actions, "backup_verify")
	case "--backup-restore":
		extra["backup-restore"] = p.val()
		actions = append(actions, "backup_restore")
	case "--backup-prune":
		k := p.val()
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
		extra["trash-restore"] = p.val()
		actions = append(actions, "trash_restore")
	case "--autostart":
		actions = append(actions, "autostart_vm")
	case "--shortcut":
		actions = append(actions, "shortcut_create")
	case "--delete-shortcut":
		actions = append(actions, "delete_shortcut")
	case "--backup":
		actions = append(actions, "backup_vm")
	case "--backup-list":
		actions = append(actions, "backup_list")
	case "--trash-list":
		actions = append(actions, "trash_list")
	default:
		return false
	}
	return true
}

// parseSnapshotFlags: snapshot action flags (the bare --snapshot
// sub-action keeps the Bash action/tag requirement checks).
func parseSnapshotFlags(p *argParser, a string) bool {
	switch a {
	case "--snapshot-tree":
		actions = append(actions, "snapshot_tree")
	case "--snapshot-verify":
		actions = append(actions, "snapshot_verify")
	case "--snapshot":
		snapshotAct = p.val()
		// Bash options.sh parity: the flag requires an action, and
		// every action except info|list|tree|verify requires a tag.
		if snapshotAct == "" {
			fmt.Fprintln(os.Stderr, "ERROR! '--snapshot' needs an action to perform.")
			os.Exit(1)
		}
		switch snapshotAct {
		case "info", "list", "tree", "verify":
		default:
			snapshotTag = p.val()
			if snapshotTag == "" {
				fmt.Fprintf(os.Stderr, "ERROR! '--snapshot %s' needs a tag.\n", snapshotAct)
				os.Exit(1)
			}
		}
		actions = append(actions, "snapshot_cmd")
	case "--snapshot-create":
		extra["snapshot-create"] = p.val()
		actions = append(actions, "snapshot_create")
	case "--snapshot-apply":
		extra["snapshot-apply"] = p.val()
		actions = append(actions, "snapshot_apply")
	case "--snapshot-delete":
		extra["snapshot-delete"] = p.val()
		actions = append(actions, "snapshot_delete")
	case "--snapshot-list":
		actions = append(actions, "snapshot_list")
	case "--memory":
		memFlag = true
	case "--quiesce":
		quiesceFlag = true
	case "--snapshot-desc":
		snapshotDesc = p.val()
	case "--snapshot-policy":
		policyVerb = p.val()
		if policyVerb == "" {
			policyVerb = "run"
		}
		actions = append(actions, "snapshot_policy")
	default:
		return false
	}
	return true
}

// parseVMOpFlags: destructive/admin VM ops (delete, clone, rename,
// move, disk compact/resize).
func parseVMOpFlags(p *argParser, a string) bool {
	switch a {
	case "--delete", "--delete-disk":
		actions = append(actions, "delete_disk")
	case "--delete-vm":
		actions = append(actions, "delete_vm")
	case "--delete-plan":
		actions = append(actions, "delete_plan")
	case "--clone":
		extra["clone"] = p.val()
		actions = append(actions, "clone_vm")
	case "--linked-clone":
		extra["linked-clone"] = p.val()
		actions = append(actions, "linked_clone_vm")
	case "--rename":
		extra["rename"] = p.val()
		actions = append(actions, "rename_vm")
	case "--move":
		extra["move"] = p.val()
		actions = append(actions, "move_vm")
	case "--disk-compact":
		actions = append(actions, "disk_compact")
	case "--disk-resize":
		extra["disk-resize"] = p.val()
		actions = append(actions, "disk_resize")
	default:
		return false
	}
	return true
}

// parseOpsFlags: op-journal queries and device hotplug.
func parseOpsFlags(p *argParser, a string) bool {
	switch a {
	case "--hotplug-usb":
		extra["hotplug-usb"] = p.val()
		actions = append(actions, "vm_hotplug_usb")
	case "--hotplug-disk":
		extra["hotplug-disk"] = p.val()
		actions = append(actions, "vm_hotplug_disk")
	case "--hotunplug":
		extra["hotunplug"] = p.val()
		actions = append(actions, "vm_hotunplug")
	case "--op-status", "--operation-status":
		extra["op-status"] = p.val()
		actions = append(actions, "operation_status")
	case "--op-list", "--operation-list":
		actions = append(actions, "operation_list")
	case "--op-cancel", "--operation-cancel":
		extra["op-cancel"] = p.val()
		actions = append(actions, "operation_cancel")
	case "--op-watch", "--operation-watch":
		extra["op-watch"] = p.val()
		actions = append(actions, "operation_watch")
	default:
		return false
	}
	return true
}

// parseLifecycleFlags: power-state actions (--restart queues
// stop_vm + start like Bash; bare --start queues "start").
func parseLifecycleFlags(p *argParser, a string) bool {
	switch a {
	case "--pause":
		actions = append(actions, "pause_vm")
	case "--resume":
		actions = append(actions, "resume_vm")
	case "--suspend":
		actions = append(actions, "vm_suspend")
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
	default:
		return false
	}
	return true
}

// parseMonitorFlags: guest-interaction actions (monitor/keys/type/
// screenshot) and console/viewer launchers.
func parseMonitorFlags(p *argParser, a string) bool {
	switch a {
	case "--monitor-cmd":
		p.scOptVal()
		extra["monitor-cmd"] = p.args[p.i]
		actions = append(actions, "monitor_cmd")
	case "--send-keys":
		extra["send-keys"] = p.val()
		actions = append(actions, "send_keys")
	case "--type":
		extra["type"] = p.val()
		actions = append(actions, "type_text")
	case "--screenshot":
		extra["screenshot"] = ""
		if p.i+1 < len(p.args) && !strings.HasPrefix(p.args[p.i+1], "-") {
			extra["screenshot"] = p.val()
		}
		actions = append(actions, "screenshot_vm")
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
	default:
		return false
	}
	return true
}

// parseConfFlags: flags that merge into the conf as CLI overrides
// (conf overrides consumed by quickemu-launch); scOpt/scOptVal keep
// the SHORTCUT_OPTIONS accumulation verbatim.
func parseConfFlags(p *argParser, a string) bool {
	switch a {
	case "--status-quo":
		p.scOpt()
		confOpts["snapshot"] = "on"
	case "--offline":
		p.scOpt()
		confOpts["network"] = "none"
	case "--width", "--height", "--ssh-access", "--ssh-port", "--spice-port",
		"--monitor-telnet-host", "--monitor-telnet-port",
		"--serial-telnet-host", "--serial-telnet-port",
		"--keyboard", "--mouse", "--usb-controller", "--usb-passthrough",
		"--sound-card", "--sound-duplex":
		p.scOptVal()
		setConfOpt(strings.TrimPrefix(a, "--"), p.args[p.i])
	case "--display":
		p.scOptVal()
		confOpts["display"] = p.args[p.i]
	case "--access":
		p.scOptVal()
		confOpts["access"] = p.args[p.i]
	case "--public-dir":
		p.scOptVal()
		confOpts["public_dir"] = p.args[p.i]
	case "--monitor":
		p.scOptVal()
		confOpts["monitor"] = p.args[p.i]
	case "--serial":
		p.scOptVal()
		confOpts["serial"] = p.args[p.i]
	case "--viewer":
		p.scOptVal()
		confOpts["viewer"] = p.args[p.i]
	case "--fullscreen", "--full-screen":
		p.scOpt()
		confOpts["fullscreen"] = "true"
	case "--braille":
		p.scOpt()
		confOpts["braille"] = "true"
	case "--keyboard_layout", "--keyboard-layout":
		p.scOptVal()
		confOpts["keyboard_layout"] = p.args[p.i]
	case "--cpu-pinning":
		p.scOptVal()
		confOpts["cpu_pinning"] = p.args[p.i]
	case "--extra_args", "--extra-args":
		p.scOptVal()
		// extra_args ACCUMULATES in Bash (extra_args+=), repeated
		// flags are joined with a space rather than last-wins.
		if prev := confOpts["extra_args"]; prev != "" {
			confOpts["extra_args"] = prev + " " + p.args[p.i]
		} else {
			confOpts["extra_args"] = p.args[p.i]
		}
	case "--ignore-tsc-warning":
		confOpts["ignore_tsc_warning"] = "on"
	case "--ignore-msrs":
		confOpts["ignore_msrs"] = "on"
	default:
		return false
	}
	return true
}
