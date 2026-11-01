// tools/quickemu-cli/args_norm.go: flag->conf mapping, alias/known-flags tables, arg normalization
package main

import (
	"strings"
)

var (
	vmConf        string
	outFmt        = "text"
	actions       []string
	confOpts      = map[string]string{} // CLI overrides merged into the conf
	extra         = map[string]string{} // action args (--monitor-cmd value etc.)
	trashFlag     bool                  // --trash: delete_vm stages to XDG trash
	statusQuoFlag bool                  // --status-quo: persist confOpts into the conf
	externalFlag  bool                  // --external: external qcow2 snapshot
	doctorFix     bool                  // --fix: doctor attempts repairs
	persistFlag   bool                  // --persist: record hotplug in the conf
	shortcutOpts  []string              // SHORTCUT_OPTIONS accumulated for --shortcut
	snapshotAct   string                // --snapshot <action>
	snapshotTag   string                // --snapshot <action> <tag>
	memFlag       bool                  // --memory: snapshot carries guest RAM
	quiesceFlag   bool                  // --quiesce: fsfreeze around snapshot
	snapshotDesc  string                // --snapshot-desc <text>
	policyVerb    string                // --snapshot-policy <run|status|units>
	dryRunFlag    bool                  // --dry-run/--echo
)

// confKeyFor maps a --flag name to its .conf key (options.sh parity):
// the shared case strips dashes into underscores, except
// --usb-passthrough which feeds the usb_devices list.
func confKeyFor(flag string) string {
	if flag == "usb-passthrough" {
		return "usb_devices"
	}
	return strings.ReplaceAll(flag, "-", "_")
}

// setConfOpt records a CLI conf override; list-valued keys accumulate
// (Bash usb_devices+=("v")).
func setConfOpt(flag, val string) {
	k := confKeyFor(flag)
	if k == "usb_devices" && confOpts[k] != "" {
		confOpts[k] += " " + val
		return
	}
	confOpts[k] = val
}

// subcommandAliases: options.sh line-16 parity — `quickemu status
// --vm x` is rewritten to `quickemu --status --vm x`. Unknown bare
// words fall through to the normal unknown-parameter error.
var subcommandAliases = map[string]bool{
	"status": true, "vfio-scan": true, "disk-compact": true, "move": true,
	"pause": true, "resume": true, "suspend": true, "reset": true, "shutdown": true,
	"kill": true, "restart": true, "delete-vm": true, "delete-plan": true,
	"list-vms": true, "capabilities": true, "host-info": true,
	"migrate-conf": true, "inspect-conf": true, "backup": true,
	"backup-list": true, "trash": true, "trash-list": true, "clone": true,
	"rename": true, "snapshot-apply": true, "snapshot-create": true,
	"snapshot-delete": true, "snapshot-info": true, "snapshot-list": true,
	"snapshot-policy": true,
	"backup-incr":     true, "backup-incr-list": true,
	"monitor-cmd": true, "send-keys": true, "type": true, "screenshot": true,
	"viewer": true, "fullscreen": true, "status-quo": true, "vnc": true,
	"spice-console": true, "publicdir": true, "serial-console": true,
	"monitor-console": true, "ignore-msrs": true,
	"ignore-msrs-always": true, "create-disk": true, "disk-resize": true,
	"disk-resize-commit": true, "backup-verify": true, "backup-restore": true,
	"trash-restore": true, "op-status": true, "operation-status": true,
	"operation-list": true, "operation-cancel": true, "op-watch": true,
	"operation-watch": true, "op-list": true,
}

// knownFlags: every --flag the parse switch accepts. Gates the
// -x -> --x normalisation so flag-shaped VALUES pass through raw.
var knownFlags = func() map[string]bool {
	m := map[string]bool{}
	for _, f := range strings.Fields(`--access --autostart --backup --backup-incr
--backup-incr-list --backup-list
--backup-prune --backup-restore --backup-verify --braille --capabilities
--check --clone --cpu-pinning --delete --delete-disk --delete-plan
--delete-shortcut --delete-vm --disk-compact --disk-resize --display
--doctor --dry-run --echo --events --export --external --extra-args
--extra_args --fix --format --full --full-screen --fullscreen --height
--help --host-info --hotplug-disk --hotplug-usb --hotunplug --ignore-msrs
--ignore-msrs-always --ignore-tsc-warning --import --inspect-conf
--inventory --keyboard --keyboard-layout --keyboard_layout --kill
--linked-clone --list --list-vms --migrate-conf --monitor --monitor-cmd
--monitor-console --monitor-telnet-host --monitor-telnet-port --mouse
--move --offline --op-cancel --op-list --op-status --op-watch
--operation-cancel --operation-list --operation-status --operation-watch
--pause --persist --plan-id --plugin --plugins --profile --public-dir
--publicdir --rename --reset --restart --resume --screenshot --send-keys
--serial --serial-console --serial-telnet-host --serial-telnet-port
--shortcut --shutdown --snapshot --snapshot-apply --snapshot-create
--snapshot-delete --snapshot-list --snapshot-tree --snapshot-verify
--memory --quiesce --snapshot-desc --snapshot-policy
--sound-card --sound-duplex --spice-console --spice-port --ssh-access --suspend
--ssh-port --start --stats --status --status-quo --stop --trash
--trash-empty --trash-list --trash-restore --type --usb-controller
--usb-passthrough --version --vfio-scan --viewer --vm --vnc --width`) {
		m[f] = true
	}
	return m
}()

// normalizeArgs: Bash accepts -flag and --flag equally, plus
// --flag=value. The -x -> --x rewrite applies ONLY to real flag
// names — a value that merely looks like a flag (--extra_args
// "-accel tcg") must reach the parser untouched.
func normalizeArgs(args []string) []string {
	var norm []string
	for _, a := range args {
		if strings.HasPrefix(a, "--") && strings.Contains(a, "=") {
			kv := strings.SplitN(a, "=", 2)
			norm = append(norm, kv[0], kv[1])
		} else if strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && knownFlags["-"+a] {
			norm = append(norm, "-"+a)
		} else {
			norm = append(norm, a)
		}
	}
	return norm
}
