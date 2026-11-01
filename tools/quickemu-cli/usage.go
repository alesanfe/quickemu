// tools/quickemu-cli/usage.go: usage text
package main

import (
	"fmt"
)

func usage() {
	fmt.Println(`quickemu — create and control VMs (Go CLI)
Usage: quickemu --vm <conf> [action flags]

Actions: --start --dry-run|--echo --status --status-quo --pause --resume
  --shutdown --powerdown --reset --stop --kill --restart --suspend
  --list-vms|--list --delete-vm --delete-disk|--delete --clone <n>
  --linked-clone <d> --rename <n> --move <dir> --export [f] --import <f>
  --backup --backup-incr --backup-incr-list --backup-list
  --backup-verify <f> --backup-restore <f> --backup-prune [n]
  --trash --trash-list --trash-empty --trash-restore <vm>
  --snapshot <act> [tag]|--snapshot-create|--snapshot-apply|--snapshot-delete
  --snapshot-list --snapshot-tree --snapshot-verify --snapshot-desc <t>
  --snapshot-policy <spec> --memory --quiesce --external
  --monitor-cmd <c> --send-keys <k> --type <t> --screenshot [f]
  --disk-compact --disk-resize <sz> --disk-resize-commit|--resize-commit
  --spice-console --vnc --publicdir --serial-console --monitor-console
  --hotplug-usb <v:p> --hotplug-disk <f> --hotunplug <id> --persist
  --autostart --shortcut --delete-shortcut --doctor|--check --fix
  --delete-plan --plan-id <id> --inspect-conf --migrate-conf --stats
Host: --capabilities --host-info --inventory --vfio-scan --events [n]
  --plugins --plugin <n> [args] --op-list --op-status --op-cancel --op-watch
Conf overrides: --display --viewer --width --height --ssh-port
  --spice-port --access --monitor --serial --keyboard --keyboard-layout
  --mouse --usb-controller --usb-passthrough --sound-card --sound-duplex
  --public-dir --profile --offline --braille --cpu-pinning --fullscreen
  --extra-args --ignore-msrs --ignore-msrs-always --ignore-tsc-warning
  --monitor-telnet-host --monitor-telnet-port --serial-telnet-host
  --serial-telnet-port --ssh-access --format json|text
Misc: --version --help`)
}
