#!/usr/bin/env bash
# E2E-QMP-001: the <vm>-qmp.socket appears once the VM boots (TCG).
Describe 'qmp_socket_ready @qmp @p1 @qemu-real'
    Include spec/e2e/support/e2e_lib.sh

    setup() {
        ws="$(e2e_sandbox "$(e2e_id qmp)")"
        vm_fixture "$ws" >/dev/null
        CONF="$ws/vm.conf"
        SOCK="$ws/vm-qmp.socket"
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

    socket_appears() {
        CPU_MODEL="${E2E_CPU}" QEMU_ACCEL="${E2E_ACCEL}" \
            "${QE}" --vm "${CONF}" >/dev/null 2>&1 &
        QE_PID=$!
        wait_until "[ -S '${SOCK}' ]" "${E2E_TIMEOUT}" || {
            echo "QMP socket ${SOCK} never appeared"; return 1; }
        "${QE}" --vm "${CONF}" --kill >/dev/null 2>&1 || true
        QE_PID=""
    }

    It 'qmp socket appears once the VM boots'
        Skip if 'qemu-system-x86_64 unavailable' qemu_missing
        When call socket_appears
        The status should be success
    End
End
