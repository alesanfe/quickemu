#!/usr/bin/env bash
# E2E-LIFECYCLE-001: TCG VM reaches running and stops cleanly.
# Levels: L2 (QMP running). Oracle set: exit code + state + pid + socket.
Describe 'start_tcg_vm_reaches_running_state @smoke @critical @qemu-real @tcg'
    Include spec/e2e/support/e2e_lib.sh

    setup() {
        ws="$(e2e_sandbox "$(e2e_id lifecycle)")"
        vm_fixture "$ws" >/dev/null
        CONF="$ws/vm.conf"
        QE_PID=""
    }
    cleanup() {
        [ -n "${QE_PID}" ] && kill -KILL "${QE_PID}" 2>/dev/null
        [ -f "${CONF}" ] && "${QE}" --vm "${CONF}" --kill >/dev/null 2>&1
        e2e_cleanup "$ws"
    }
    BeforeEach 'setup'
    AfterEach 'cleanup'

    qemu_missing() { ! command -v qemu-system-x86_64 >/dev/null 2>&1; }

    start_and_stop() {
        CPU_MODEL="${E2E_CPU}" QEMU_ACCEL="${E2E_ACCEL}" \
            "${QE}" --vm "${CONF}" >/dev/null 2>&1 &
        QE_PID=$!
        wait_until '[ "$(qe_state "${CONF}")" = running ]' "${E2E_TIMEOUT}" || {
            echo "VM never reached running"; return 1; }
        "${QE}" --vm "${CONF}" --kill >/dev/null 2>&1 || true
        wait_until '[ "$(qe_state "${CONF}")" != running ]' 15 || {
            echo "VM did not stop"; return 1; }
        QE_PID=""
        # No residual monitor/QMP sockets after kill.
        [ -z "$(find "${ws}" -name '*.sock*' -print -quit)" ] || {
            echo "residual socket left"; return 1; }
        e2e_artifact "${ws}" start-stop passed lifecycle
    }

    It 'launches, reports running via the contract, stops leaving no residue'
        Skip if 'qemu-system-x86_64 unavailable' qemu_missing
        When call start_and_stop
        The status should be success
    End
End
