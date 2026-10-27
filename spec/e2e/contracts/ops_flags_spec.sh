#!/usr/bin/env bash
# E2E-CONTRACT-OPS: management flags converge on a usable contract
# surface in both backends. Added after the 2026-10 audit found three
# real defects in flags with zero coverage: --op-status dropped its id
# entirely (Go), --op-cancel passed it positionally instead of --id
# (Go), and --plugin on a missing plugin looped forever (Bash).
Describe 'ops flags contract @contract'
    Include spec/e2e/support/e2e_lib.sh

    setup() {
        ws="$(e2e_sandbox "$(e2e_id opsflags)")"
        vm_fixture "$ws" >/dev/null
        CONF="$ws/vm.conf"
    }
    BeforeEach 'setup'

    It '--snapshot-verify succeeds on a healthy VM dir'
        When run "${QE}" --vm "${CONF}" --snapshot-verify
        The status should be success
        The output should include 'Snapshot chain verified'
    End

    It '--trash-empty succeeds'
        When run "${QE}" --vm "${CONF}" --trash-empty
        The status should be success
        The output should include 'Trash is empty'
    End

    It '--plugins succeeds (empty list is fine)'
        When run "${QE}" --vm "${CONF}" --plugins
        The status should be success
    End

    It '--operation-status reports NOT_FOUND for a missing op'
        When run "${QE}" --vm "${CONF}" --operation-status op-999 --format json
        The status should be failure
        The output should include 'OPERATION_NOT_FOUND'
    End

    It '--operation-cancel reports NOT_FOUND for a missing op'
        When run "${QE}" --vm "${CONF}" --operation-cancel op-999 --format json
        The status should be failure
        The output should include 'OPERATION_NOT_FOUND'
    End

    It '--plugin with a missing plugin errors and terminates'
        # Regression: Bash used to loop forever re-dispatching on
        # failure. timeout catches a hung parse loop.
        When run timeout 15 "${QE}" --vm "${CONF}" --plugin bogus-plugin
        The status should be failure
        The error should include 'not found on PATH'
    End

    It '--backup-verify rejects a missing archive'
        When run "${QE}" --vm "${CONF}" --backup-verify /nonexistent.tar.zst
        The status should be failure
        The error should include 'not found'
    End

    It '--hotunplug fails cleanly when the VM is stopped'
        # Bash: "ERROR! No QMP channel" on stderr; Go: qmp.execute
        # QMP_UNAVAILABLE envelope on stdout. Merge streams so the
        # assertion holds on either backend.
        When run sh -c '"${1}" --vm "${2}" --hotunplug usb1 2>&1' _ "${QE}" "${CONF}"
        The status should be failure
        The output should include 'QMP'
    End

    It '--echo prints a qemu command line without launching'
        When run "${QE}" --vm "${CONF}" --echo
        The status should be success
        The output should include 'qemu-system'
    End
End
