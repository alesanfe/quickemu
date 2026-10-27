#!/usr/bin/env bash
# E2E-SECURITY-001: --delete-vm removes only the VM tree; canaries outside
# the VMDIR survive untouched.
Describe 'delete_never_escapes_vm_directory @smoke @security'
    Include spec/e2e/support/e2e_lib.sh

    setup() {
        ws="$(e2e_sandbox "$(e2e_id security)")"
        # conf outside the VMDIR: disk_img relative -> VMDIR = <confdir>/vm
        mkdir -p "$ws/vm/allowed"
        printf 'guest_os="linux"\ndisk_img="vm/disk.qcow2"\nram="256M"\n' >"$ws/vm.conf"
        printf x >"$ws/vm/disk.qcow2"
        printf 'canary' >"$ws/must-survive.txt"
        CONF="$ws/vm.conf"
    }
    BeforeEach 'setup'

    delete_and_check() {
        "${QE}" --vm "${CONF}" --delete-vm >/dev/null 2>&1 || true
        [ ! -d "$ws/vm" ] || { echo "vm dir still present"; return 1; }
        [ "$(cat "$ws/must-survive.txt")" = 'canary' ] || {
            echo "canary was deleted"; return 1; }
    }

    It 'delete removes the vm dir but preserves the canary'
        When call delete_and_check
        The status should be success
    End
End
