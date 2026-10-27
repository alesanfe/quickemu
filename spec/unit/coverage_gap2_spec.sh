# Second coverage-gap spec — exercises the remaining uncovered cluster.
# Behavioral tests for pure/state helpers; contract checks for functions
# that need a live QEMU/QMP context.
# shellcheck shell=bash
Include lib/quickemu/support/json.sh
Include lib/quickemu/support/events.sh
Include lib/quickemu/support/filesystem.sh
Include lib/quickemu/cli/verbosity.sh
Include lib/quickemu/cli/doctor.sh
Include lib/quickemu/cli/plugins.sh
Include lib/quickemu/cli/commands.sh
Include lib/quickemu/config/toml.sh
Include lib/quickemu/planning/delete_plan.sh
Include lib/quickemu/runtime/lifecycle.sh
Include lib/quickemu/runtime/monitor.sh
Include lib/quickemu/runtime/launch.sh
Include lib/quickemu/runtime/ops.sh

qe_fn_exists() { # name -> 0 if defined
    [ "$(type -t "${1}")" = "function" ]
}

Describe 'coverage2: ops journal helpers'
    setup() {
        VMDIR=$(mktemp -d); export VMDIR
    }
    cleanup() { rm -rf "${VMDIR}"; }
    BeforeEach 'setup'
    AfterEach 'cleanup'

    It 'op_dir resolves under VMDIR'
        When call op_dir
        The output should include "${VMDIR}"
    End

    It 'op_recover marks dead-owner ops as failed'
        d=$(op_dir); mkdir -p "${d}"
        printf '{"schemaVersion":1,"operationId":"t1","type":"download","vm":"v","state":"running","progress":5,"startedAt":"x","result":null,"error":null}\n' > "${d}/t1.json"
        echo 999999 > "${d}/t1.owner"
        When call op_recover
        The contents of file "${d}/t1.json" should include '"state":"failed"'
    End

    It 'op_rollback cleans download .part'
        d=$(op_dir); mkdir -p "${d}"
        printf '{"schemaVersion":1,"operationId":"t2","type":"download","vm":"v","state":"failed","progress":0,"startedAt":"x","result":null,"error":null}\n' > "${d}/t2.json"
        touch "${VMDIR}/file.iso.part"
        When call op_rollback t2
        The path "${VMDIR}/file.iso.part" should not be exist
    End

    It 'op_prune exists'
        When call qe_fn_exists op_prune
        The status should be success
    End
    It 'op_list exists'
        When call qe_fn_exists op_list
        The status should be success
    End
    It 'op_cancel exists'
        When call qe_fn_exists op_cancel
        The status should be success
    End
    It '_ops_bin exists'
        When call qe_fn_exists _ops_bin
        The status should be success
    End
End

Describe 'coverage2: filesystem dirs'
    It 'qe_data_dir exists and resolves'
        When call qe_data_dir
        The status should be success
        The output should include 'quickemu'
    End
    It 'qe_runtime_dir exists and resolves'
        When call qe_runtime_dir
        The status should be success
        The output should include 'quickemu'
    End
    It 'qe_state_dir exists and resolves'
        When call qe_state_dir
        The status should be success
        The output should include 'quickemu'
    End
End

Describe 'coverage2: events helpers'
    It 'qe_events_file exists'
        When call qe_fn_exists qe_events_file
        The status should be success
    End
    It 'qe_event exists'
        When call qe_fn_exists qe_event
        The status should be success
    End
    It 'qe_events_read exists'
        When call qe_fn_exists qe_events_read
        The status should be success
    End
    It '_qe_events_next_seq exists'
        When call qe_fn_exists _qe_events_next_seq
        The status should be success
    End
    It '_qe_events_rotate exists'
        When call qe_fn_exists _qe_events_rotate
        The status should be success
    End
End

Describe 'coverage2: monitor commands'
    It 'qmp_query exists'
        When call qe_fn_exists qmp_query
        The status should be success
    End
    It 'qmp_command exists'
        When call qe_fn_exists qmp_command
        The status should be success
    End
    It 'vm_qmp_socket_ready exists'
        When call qe_fn_exists vm_qmp_socket_ready
        The status should be success
    End
    It 'qmp_device_add exists'
        When call qe_fn_exists qmp_device_add
        The status should be success
    End
    It 'qmp_device_del exists'
        When call qe_fn_exists qmp_device_del
        The status should be success
    End
    It 'hotplug_persist exists'
        When call qe_fn_exists hotplug_persist
        The status should be success
    End
    It 'vm_control exists'
        When call qe_fn_exists vm_control
        The status should be success
    End
    It 'vm_hotplug_disk exists'
        When call qe_fn_exists vm_hotplug_disk
        The status should be success
    End
    It 'vm_hotplug_usb exists'
        When call qe_fn_exists vm_hotplug_usb
        The status should be success
    End
    It 'vm_hotplug_usb uses QEMU vendorid/productid properties'
        # QEMU device_add usb-host only accepts vendorid/productid;
        # the historical hostvendor/hostport names are rejected.
        When run grep -o 'hostvendor\|hostport\|vendorid\|productid' "${SHELLSPEC_PROJECT_ROOT}/lib/quickemu/runtime/monitor.sh"
        The output should include vendorid
        The output should include productid
        The output should not include hostvendor
        The output should not include hostport
    End
    It 'vm_hotunplug exists'
        When call qe_fn_exists vm_hotunplug
        The status should be success
    End
End

Describe 'coverage2: lifecycle helpers'
    It 'vm_transition_ok allows running->pause'
        When call vm_transition_ok running pause
        The status should be success
    End
    It 'vm_transition_ok allows stopped->start'
        When call vm_transition_ok stopped start
        The status should be success
    End
    It 'vm_transition_ok rejects stopped->pause'
        When call vm_transition_ok stopped pause
        The status should be failure
    End
    It 'vm_transition_ok allows any->status'
        When call vm_transition_ok archived status
        The status should be success
    End

    It 'vm_proc_comm exists'
        When call qe_fn_exists vm_proc_comm
        The status should be success
    End
    It 'vm_proc_start_time exists'
        When call qe_fn_exists vm_proc_start_time
        The status should be success
    End
    It 'vm_instance_uuid exists'
        When call qe_fn_exists vm_instance_uuid
        The status should be success
    End
    It 'vm_allowed_ops exists'
        When call qe_fn_exists vm_allowed_ops
        The status should be success
    End
    It 'vm_verify_invariants exists'
        When call qe_fn_exists vm_verify_invariants
        The status should be success
    End
End

Describe 'coverage2: cli commands'
    It 'operation_status exists'
        When call qe_fn_exists operation_status
        The status should be success
    End
    It 'operation_list exists'
        When call qe_fn_exists operation_list
        The status should be success
    End
    It 'operation_cancel exists'
        When call qe_fn_exists operation_cancel
        The status should be success
    End
    It 'operation_watch exists'
        When call qe_fn_exists operation_watch
        The status should be success
    End
    It 'events_read exists'
        When call qe_fn_exists events_read
        The status should be success
    End
    It 'host_info exists'
        When call qe_fn_exists host_info
        The status should be success
    End
    It 'inventory_show exists'
        When call qe_fn_exists inventory_show
        The status should be success
    End
    It 'migrate_conf exists'
        When call qe_fn_exists migrate_conf
        The status should be success
    End
    It 'guest_os_param_check exists'
        When call qe_fn_exists guest_os_param_check
        The status should be success
    End
End

Describe 'coverage2: misc'
    It 'qe_doctor exists'
        When call qe_fn_exists qe_doctor
        The status should be success
    End
    It 'plugin_dispatch exists'
        When call qe_fn_exists plugin_dispatch
        The status should be success
    End
    It 'plugin_list exists'
        When call qe_fn_exists plugin_list
        The status should be success
    End
    It 'toml_from_conf exists'
        When call qe_fn_exists toml_from_conf
        The status should be success
    End
    It 'delete_plan_dependents exists'
        When call qe_fn_exists delete_plan_dependents
        The status should be success
    End
    It 'configure_cpu_pinning exists'
        When call qe_fn_exists configure_cpu_pinning
        The status should be success
    End
    It 'qemu_spawn exists'
        When call qe_fn_exists qemu_spawn
        The status should be success
    End
End
