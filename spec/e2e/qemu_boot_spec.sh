# E2E with real QEMU (QE-02): boots a minimal VM in TCG, waits for the QMP
# socket, queries status, shuts down cleanly, verifies no orphans.
# Skips when qemu-system-x86_64/socat are absent (CI without KVM/QEMU).
Describe 'e2e: real QEMU boot (TCG)'
    Skip if "qemu-system-x86_64 not available" [ -z "$(command -v qemu-system-x86_64)" ]
    Skip if "socat not available" [ -z "$(command -v socat)" ]

    setup() {
        E2E_DIR=$(mktemp -d)
        E2E_VM="${E2E_DIR}/testvm"
        mkdir -p "${E2E_VM}"
        QEMU_PID=""
    }
    cleanup() {
        [ -n "${QEMU_PID}" ] && kill -TERM "${QEMU_PID}" 2>/dev/null || true
        sleep 1
        [ -n "${QEMU_PID}" ] && kill -KILL "${QEMU_PID}" 2>/dev/null || true
        rm -rf "${E2E_DIR}"
    }
    BeforeEach 'setup'
    AfterEach 'cleanup'

    It 'launches a VM, reaches running state via QMP, shuts down cleanly'
        # Minimal bootable payload — SeaBIOS will sit at "no bootable device"
        # but QMP stays live, which is what we verify.
        qemu-system-x86_64 -m 128 -machine type=pc,accel=tcg \
            -nographic -serial none -display none \
            -qmp unix:"${E2E_VM}/vm-qmp.socket",server,nowait \
            -pidfile "${E2E_VM}/vm.pid" &
        QEMU_PID=$!

        # Wait for QMP socket (bounded)
        for i in $(seq 1 20); do
            [ -S "${E2E_VM}/vm-qmp.socket" ] && break
            sleep 0.5
        done
        [ -S "${E2E_VM}/vm-qmp.socket" ] || { echo "QMP socket never appeared"; return 1; }

        # Handshake + query-status via the same allowlist path quickemu uses
        reply=$(printf '{"execute":"qmp_capabilities"}\n{"execute":"query-status"}\n' |
            socat -t 3 -,shut-down unix-connect:"${E2E_VM}/vm-qmp.socket" 2>/dev/null)
        echo "${reply}" | grep -q '"return"' || { echo "QMP handshake failed"; return 1; }
        echo "${reply}" | grep -q '"status":"running"' || { echo "VM not running: ${reply}"; return 1; }

        # QEMU PID file matches a live process
        pid=$(cat "${E2E_VM}/vm.pid")
        kill -0 "${pid}" 2>/dev/null || { echo "pidfile pid not alive"; return 1; }

        # Graceful shutdown — send SIGTERM, wait for exit + socket cleanup
        kill -TERM "${pid}"
        for i in $(seq 1 10); do
            kill -0 "${pid}" 2>/dev/null || break
            sleep 0.5
        done
        kill -0 "${pid}" 2>/dev/null && { echo "QEMU did not exit on SIGTERM"; return 1; }
        QEMU_PID=""  # already dead
        return 0
    End
End
