#!/usr/bin/env bash
# E2E-SECURITY-001: delete never escapes the VM directory (canary files).
. "$(dirname "$0")/../support/e2e_lib.sh"

Describe 'delete_never_escapes_vm_directory @smoke @security'

ws="$(e2e_sandbox "$(e2e_id security)")"
mkdir -p "$ws/vm/allowed"
canary="$ws/must-survive.txt"
echo 'canary' >"$canary"

It 'delete removes the vm dir but preserves the canary'
printf 'guest_os="linux"\ndisk_img="disk.qcow2"\n' >"$ws/vm/vm.conf"
touch "$ws/vm/disk.qcow2"
"$QE" --vm "$ws/vm/vm.conf" --delete-vm --yes >/dev/null 2>&1 || true
[ ! -d "$ws/vm/allowed" ] && [ "$(cat "$canary")" = 'canary' ]
The status should equal 0
End
End
