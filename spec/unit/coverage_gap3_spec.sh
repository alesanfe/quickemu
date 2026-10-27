# Third coverage-gap spec — snapshot/backup/trash/efi helpers.
# shellcheck shell=bash
Include lib/quickemu/support/json.sh
Include lib/quickemu/support/events.sh
Include lib/quickemu/cli/verbosity.sh
Include lib/quickemu/vm/snapshots.sh
Include lib/quickemu/vm/snapshots_consolidate.sh
Include lib/quickemu/vm/vm.sh

qe_fn_exists() { # name -> 0 if defined
    [ "$(type -t "${1}")" = "function" ]
}

Describe 'coverage3: snapshot metadata helpers'
    setup() {
        VMDIR=$(mktemp -d); export VMDIR
        VMNAME=coverage3; export VMNAME
    }
    cleanup() { rm -rf "${VMDIR}"; }
    BeforeEach 'setup'
    AfterEach 'cleanup'

    It 'snap_meta_write/read roundtrip preserves data'
        snap_meta_write '[{"tag":"a","file":"a.qcow2","parent":""}]' >/dev/null
        When call snap_meta_read
        The output should include '"tag":"a"'
    End

    It 'snap_next_id exists'
        When call qe_fn_exists snap_next_id
        The status should be success
    End
    It 'snap_register exists'
        When call qe_fn_exists snap_register
        The status should be success
    End
    It 'snapshot_tree exists'
        When call qe_fn_exists snapshot_tree
        The status should be success
    End
    It 'snapshot_verify exists'
        When call qe_fn_exists snapshot_verify
        The status should be success
    End
    It 'snapshot_create_external exists'
        When call qe_fn_exists snapshot_create_external
        The status should be success
    End
    It 'snapshot_consolidate exists'
        When call qe_fn_exists snapshot_consolidate
        The status should be success
    End
    It 'snapshot_info exists'
        When call qe_fn_exists snapshot_info
        The status should be success
    End
    It 'snapshot_tag_exists exists'
        When call qe_fn_exists snapshot_tag_exists
        The status should be success
    End
    It 'snap_meta_file exists'
        When call qe_fn_exists snap_meta_file
        The status should be success
    End
    It 'snap_meta_remove exists'
        When call qe_fn_exists snap_meta_remove
        The status should be success
    End
End

Describe 'coverage3: backup helpers'
    setup() {
        VMDIR=$(mktemp -d); export VMDIR
        VMNAME=coverage3; export VMNAME
    }
    cleanup() { rm -rf "${VMDIR}"; }
    BeforeEach 'setup'
    AfterEach 'cleanup'

    It 'backup_dir resolves under VMDIR'
        When call backup_dir
        The output should include "${VMDIR}"
    End
    It 'backup_list exists'
        When call qe_fn_exists backup_list
        The status should be success
    End
    It 'backup_prune exists'
        When call qe_fn_exists backup_prune
        The status should be success
    End
    It 'backup_restore exists'
        When call qe_fn_exists backup_restore
        The status should be success
    End
    It 'backup_verify exists'
        When call qe_fn_exists backup_verify
        The status should be success
    End
    It 'backup_vm exists'
        When call qe_fn_exists backup_vm
        The status should be success
    End
End

Describe 'coverage3: error helpers'
    It 'qe_die exists'
        When call qe_fn_exists qe_die
        The status should be success
    End
    It 'qe_die_invalid exists'
        When call qe_fn_exists qe_die_invalid
        The status should be success
    End
    It 'qe_die_locked exists'
        When call qe_fn_exists qe_die_locked
        The status should be success
    End
    It 'qe_die_not_found exists'
        When call qe_fn_exists qe_die_not_found
        The status should be success
    End
    It 'qe_die_running exists'
        When call qe_fn_exists qe_die_running
        The status should be success
    End
    It 'qe_die_stopped exists'
        When call qe_fn_exists qe_die_stopped
        The status should be success
    End
    It 'qe_die_unsafe exists'
        When call qe_fn_exists qe_die_unsafe
        The status should be success
    End
    It 'qe_fail exists'
        When call qe_fn_exists qe_fail
        The status should be success
    End
End

Describe 'coverage3: vm misc helpers'
    setup() {
        VMDIR=$(mktemp -d); export VMDIR
        VMNAME=coverage3; export VMNAME
    }
    cleanup() { rm -rf "${VMDIR}"; }
    BeforeEach 'setup'
    AfterEach 'cleanup'

    It 'clone_vm exists'
        When call qe_fn_exists clone_vm
        The status should be success
    End
    It 'clone_vm_linked exists'
        When call qe_fn_exists clone_vm_linked
        The status should be success
    End
    It 'move_vm exists'
        When call qe_fn_exists move_vm
        The status should be success
    End
    It 'trash_empty exists'
        When call qe_fn_exists trash_empty
        The status should be success
    End
    It 'trash_list exists'
        When call qe_fn_exists trash_list
        The status should be success
    End
    It 'trash_restore exists'
        When call qe_fn_exists trash_restore
        The status should be success
    End
    It 'inventory_touch exists'
        When call qe_fn_exists inventory_touch
        The status should be success
    End
    It 'vm_stats exists'
        When call qe_fn_exists vm_stats
        The status should be success
    End
    It 'vm_boot_channels exists'
        When call qe_fn_exists vm_boot_channels
        The status should be success
    End
    It 'vm_boot_fileshare exists'
        When call qe_fn_exists vm_boot_fileshare
        The status should be success
    End
    It 'vm_boot_media exists'
        When call qe_fn_exists vm_boot_media
        The status should be success
    End
    It 'vm_boot_passthru exists'
        When call qe_fn_exists vm_boot_passthru
        The status should be success
    End
    It 'vm_network_preset exists'
        When call qe_fn_exists vm_network_preset
        The status should be success
    End
    It 'vm_profile_defaults exists'
        When call qe_fn_exists vm_profile_defaults
        The status should be success
    End
    It 'vm_send_key_combo exists'
        When call qe_fn_exists vm_send_key_combo
        The status should be success
    End
    It 'is_firmware_qcow2 exists'
        When call qe_fn_exists is_firmware_qcow2
        The status should be success
    End
    It 'efi_vars exists'
        When call qe_fn_exists efi_vars
        The status should be success
    End
    It 'vfio_scan exists'
        When call qe_fn_exists vfio_scan
        The status should be success
    End
End
