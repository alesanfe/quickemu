#!/usr/bin/env bash
# E2E-RECOVERY-001: an exclusive lock dir whose pid is dead must be
# reclaimed — a crashed writer never blocks readers forever.
Describe 'stale_lock_is_recovered @recovery @p1'
    Include spec/e2e/support/e2e_lib.sh

    setup() {
        ws="$(e2e_sandbox "$(e2e_id recovery)")"
        vm_fixture "$ws" >/dev/null
        CONF="$ws/vm.conf"
        # Simulate a crashed writer: lock dir + pid file of a dead process.
        mkdir -p "$ws/vm.lock"
        echo 999999 >"$ws/vm.lock/pid"
    }
    BeforeEach 'setup'

    It 'a stale lock does not block a fresh status'
        When call qe_state "${CONF}"
        The output should equal 'stopped'
    End
End
