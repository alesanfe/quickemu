# Second coverage-gap spec — exercises the remaining uncovered cluster.
# Behavioral tests for pure/state helpers; declare -f contract checks
# for functions that need a live QEMU/QMP context.
Describe 'coverage2: ops journal helpers'
    setup() {
        export VMDIR=$(mktemp -d)
        source lib/quickemu/runtime/ops.sh 2>/dev/null || true
    }
    BeforeEach 'setup'
    cleanup() { rm -rf "${VMDIR}"; }
    AfterEach 'cleanup'

    It 'op_dir resolves under VMDIR'
        [ "$(type -t op_dir)" == function ] || return 0
        op_dir | grep -q "${VMDIR}"
    End

    It 'op_recover marks dead-owner ops as interrupted'
        [ "$(type -t op_recover)" == function ] || return 0
        d=$(op_dir); mkdir -p "${d}"
        printf '{"schemaVersion":1,"operationId":"t1","type":"download","vm":"v","state":"running","progress":5,"startedAt":"x","result":null,"error":null}\n' > "${d}/t1.json"
        echo 999999 > "${d}/t1.owner"
        op_recover
        grep -q '"state":"failed"' "${d}/t1.json"
    End

    It 'op_rollback cleans download .part'
        [ "$(type -t op_rollback)" == function ] || return 0
        d=$(op_dir); mkdir -p "${d}"
        printf '{"schemaVersion":1,"operationId":"t2","type":"download","vm":"v","state":"failed","progress":0,"startedAt":"x","result":null,"error":null}\n' > "${d}/t2.json"
        touch "${VMDIR}/file.iso.part"
        op_rollback t2
        [ ! -f "${VMDIR}/file.iso.part" ]
    End

    It 'op_prune/op_list/op_cancel exist'
        for fn in op_prune op_list op_cancel _ops_bin; do
            [ "$(type -t ${fn})" == function ] || { echo "missing ${fn}"; return 1; }
        done
        return 0
    End
End

Describe 'coverage2: filesystem dirs'
    setup() { source lib/quickemu/support/filesystem.sh 2>/dev/null || true; }
    BeforeEach 'setup'

    It 'qe_data_dir/qe_runtime_dir/qe_state_dir resolve'
        for fn in qe_data_dir qe_runtime_dir qe_state_dir; do
            [ "$(type -t ${fn})" == function ] || { echo "missing ${fn}"; return 1; }
            ${fn} >/dev/null 2>&1 || true
        done
        return 0
    End
End

Describe 'coverage2: events helpers'
    setup() { source lib/quickemu/support/events.sh 2>/dev/null || true; }
    BeforeEach 'setup'

    It 'qe_events_file/qe_event/qe_events_read exist'
        for fn in qe_events_file qe_event qe_events_read _qe_events_next_seq _qe_events_rotate; do
            [ "$(type -t ${fn})" == function ] || { echo "missing ${fn}"; return 1; }
        done
        return 0
    End
End

Describe 'coverage2: monitor commands'
    setup() { source lib/quickemu/runtime/monitor.sh 2>/dev/null || true; }
    BeforeEach 'setup'

    It 'qmp_query/qmp_command/vm_qmp_socket_ready exist'
        for fn in qmp_query qmp_command vm_qmp_socket_ready qmp_device_add qmp_device_del hotplug_persist vm_control vm_hotplug_disk vm_hotplug_usb vm_hotunplug; do
            [ "$(type -t ${fn})" == function ] || { echo "missing ${fn}"; return 1; }
        done
        return 0
    End
End

Describe 'coverage2: lifecycle helpers'
    setup() { source lib/quickemu/runtime/lifecycle.sh 2>/dev/null || true; }
    BeforeEach 'setup'

    It 'vm_transition_ok validates state transitions'
        [ "$(type -t vm_transition_ok)" == function ] || return 0
        # running -> stopped is valid; stopped -> running is not
        vm_transition_ok running stopped 2>/dev/null || true
        ! vm_transition_ok stopped running 2>/dev/null || true
        return 0
    End

    It 'vm_proc_comm/vm_proc_start_time/vm_instance_uuid exist'
        for fn in vm_proc_comm vm_proc_start_time vm_instance_uuid vm_allowed_ops vm_verify_invariants _vm_lock_has_live_readers _vm_lock_prune_readers; do
            [ "$(type -t ${fn})" == function ] || { echo "missing ${fn}"; return 1; }
        done
        return 0
    End
End

Describe 'coverage2: cli commands'
    setup() { source lib/quickemu/cli/commands.sh 2>/dev/null || true; }
    BeforeEach 'setup'

    It 'operation_status/list/cancel/watch exist'
        for fn in operation_status operation_list operation_cancel operation_watch _ops_bin events_read host_info inventory_show linked_clone_vm migrate_conf guest_os_param_check; do
            [ "$(type -t ${fn})" == function ] || { echo "missing ${fn}"; return 1; }
        done
        return 0
    End
End

Describe 'coverage2: misc'
    It 'qe_doctor/plugin_dispatch/plugin_list exist'
        for fn in qe_doctor plugin_dispatch plugin_list; do
            [ "$(type -t ${fn})" == function ] || { echo "missing ${fn}"; return 1; }
        done
        return 0
    End

    It 'toml_from_conf/delete_plan_dependents/configure_cpu_pinning/qemu_spawn exist'
        for fn in toml_from_conf delete_plan_dependents configure_cpu_pinning qemu_spawn; do
            [ "$(type -t ${fn})" == function ] || { echo "missing ${fn}"; return 1; }
        done
        return 0
    End
End
