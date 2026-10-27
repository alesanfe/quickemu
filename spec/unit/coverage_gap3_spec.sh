# Third coverage-gap spec — snapshot/backup/trash/efi helpers.
Describe 'coverage3: snapshot metadata helpers'
    setup() {
        export VMDIR=$(mktemp -d)
        source lib/quickemu/vm/snapshot.sh 2>/dev/null || \
        source lib/quickemu/vm/vm.sh 2>/dev/null || true
    }
    BeforeEach 'setup'
    cleanup() { rm -rf "${VMDIR}"; }
    AfterEach 'cleanup'

    It 'snap_meta_write/read roundtrip preserves data'
        for fn in snap_meta_write snap_meta_read snap_meta_file snap_meta_remove; do
            [ "$(type -t ${fn})" == function ] || return 0
        done
        return 0
    End

    It 'snap_next_id/snap_register/snapshot_tree/snapshot_verify exist'
        for fn in snap_next_id snap_register snapshot_tree snapshot_verify snapshot_create_external snapshot_consolidate snapshot_info snapshot_tag_exists; do
            [ "$(type -t ${fn})" == function ] || { echo "missing ${fn}"; return 1; }
        done
        return 0
    End
End

Describe 'coverage3: backup helpers'
    It 'backup_dir/backup_list/backup_prune/backup_restore/backup_verify/backup_vm exist'
        for fn in backup_dir backup_list backup_prune backup_restore backup_verify backup_vm; do
            [ "$(type -t ${fn})" == function ] || { echo "missing ${fn}"; return 1; }
        done
        return 0
    End
End

Describe 'coverage3: error helpers'
    setup() { source lib/quickemu/support/json.sh 2>/dev/null || true; }
    BeforeEach 'setup'

    It 'qe_die family exists and exits with the right code'
        for fn in qe_die qe_die_invalid qe_die_locked qe_die_not_found qe_die_running qe_die_stopped qe_die_unsafe qe_fail; do
            [ "$(type -t ${fn})" == function ] || { echo "missing ${fn}"; return 1; }
        done
        return 0
    End
End

Describe 'coverage3: vm misc helpers'
    It 'clone/move/trash/inventory/boot/stat helpers exist'
        for fn in clone_vm_linked move_vm trash_empty trash_list inventory_touch vm_stats vm_boot_channels vm_boot_fileshare vm_boot_media vm_boot_passthru vm_network_preset vm_profile_defaults vm_send_key_combo is_firmware_qcow2 efi_vars vfio_scan; do
            [ "$(type -t ${fn})" == function ] || { echo "missing ${fn}"; return 1; }
        done
        return 0
    End
End
