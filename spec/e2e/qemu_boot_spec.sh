# E2E with real QEMU (QE-02): boots a minimal VM in TCG, waits for the QMP
# socket, queries status, shuts down cleanly, verifies no orphans.
# Skips when qemu-system-x86_64/socat are absent (CI without KVM/QEMU).
Describe 'e2e: real QEMU boot (TCG)'
    Include spec/e2e/support/e2e_lib.sh

    setup() {
        E2E_DIR=$(mktemp -d)
        E2E_VM="${E2E_DIR}/testvm"
        mkdir -p "${E2E_VM}"
        QEMU_PID=""
    }
    cleanup() {
        [ -n "${QEMU_PID}" ] && kill -TERM "${QEMU_PID}" 2>/dev/null
        sleep 1
        [ -n "${QEMU_PID}" ] && kill -KILL "${QEMU_PID}" 2>/dev/null
        rm -rf "${E2E_DIR}"
    }
    BeforeEach 'setup'
    AfterEach 'cleanup'

    qemu_missing() {
        ! command -v qemu-system-x86_64 >/dev/null 2>&1 ||
            ! command -v socat >/dev/null 2>&1
    }

    boot_scenario() {
        # Minimal bootable payload — SeaBIOS will sit at "no bootable
        # device" but QMP stays live, which is what we verify.
        qemu-system-x86_64 -m 128 -machine type=pc,accel=tcg \
            -nographic -serial none -display none \
            -qmp unix:"${E2E_VM}/vm-qmp.socket",server,nowait \
            -pidfile "${E2E_VM}/vm.pid" >/dev/null 2>&1 &
        QEMU_PID=$!

        wait_until "[ -S '${E2E_VM}/vm-qmp.socket' ]" 10 || {
            echo "QMP socket never appeared"; return 1; }

        # Handshake + query-status via the same channel quickemu uses.
        reply=$(printf '{"execute":"qmp_capabilities"}\n{"execute":"query-status"}\n' |
            socat -t 3 -,shut-down unix-connect:"${E2E_VM}/vm-qmp.socket" 2>/dev/null)
        grep -q '"return"' <<<"${reply}" || { echo "QMP handshake failed"; return 1; }
        grep -qE '"status":[[:space:]]*"running"' <<<"${reply}" || {
            echo "VM not running: ${reply}"; return 1; }

        # QEMU pid file matches a live process.
        pid=$(cat "${E2E_VM}/vm.pid")
        kill -0 "${pid}" 2>/dev/null || { echo "pidfile pid not alive"; return 1; }

        # Graceful shutdown — SIGTERM, wait for exit.
        kill -TERM "${pid}"
        wait_until "! kill -0 ${pid} 2>/dev/null" 10 || {
            echo "QEMU did not exit on SIGTERM"; return 1; }
        QEMU_PID=""
    }

    It 'launches a VM, reaches running state via QMP, shuts down cleanly'
        Skip if 'qemu-system-x86_64 or socat unavailable' qemu_missing
        When call boot_scenario
        The status should be success
    End
End
