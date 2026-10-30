// tools/quickemu-cli/usage.go: usage text
package main

import (
	"fmt"
)

func usage() {
	fmt.Println(`quickemu — create and control VMs (Go CLI)
Usage: quickemu --vm <conf> [action flags]

Actions: --start --dry-run|--echo --status --status-quo --pause --resume
  --shutdown --reset --stop --kill --restart --list-vms --delete-vm
  --delete-disk --clone <n> --linked-clone <d> --rename <n> --move <dir>
  --backup --backup-list --backup-verify <f> --backup-restore <f>
  --backup-prune [n] --export [f] --import <f> --trash --trash-list
  --trash-empty --trash-restore <vm> --snapshot <act> [tag] --external
  --snapshot-tree --snapshot-verify --monitor-cmd <c> --send-keys <k>
  --type <t> --screenshot [f] --disk-compact --disk-resize <sz>
  --spice-console --vnc --publicdir --serial-console --monitor-console
  --hotplug-usb <v:p> --hotplug-disk <f> --hotunplug <id> --persist
  --autostart --shortcut --delete-shortcut --doctor|--check --fix
  --delete-plan --plan-id <id> --inspect-conf --migrate-conf --stats
Host: --capabilities --host-info --inventory --vfio-scan --events [n]
  --plugins --plugin <n> [args] --op-list --op-status --op-watch
Conf overrides: --display --viewer --width --height --ssh-port
  --spice-port --monitor --serial --keyboard --mouse --usb-controller
  --sound-card --public-dir --profile --offline --format json|text`)
}
