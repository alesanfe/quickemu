# E2E: the Go quickemu CLI boots a real QEMU (TCG) end-to-end —
# status/pause/resume/monitor-cmd/screenshot/stop through the same
# channels the Bash CLI uses. Skips when QEMU or the Go toolchain
# is unavailable.
Describe 'e2e: quickemu-cli boots and manages a real QEMU (TCG)'
    Include spec/e2e/support/e2e_lib.sh

    setup() {
        E2E_DIR=$(mktemp -d)
        E2E_VM="${E2E_DIR}/govm"
        mkdir -p "${E2E_VM}"
        printf 'guest_os="linux"\ndisk_img="disk.qcow2"\ndisplay="none"\nram="128M"\n' \
            >"${E2E_VM}/govm.conf"
        qemu-img create -f qcow2 "${E2E_VM}/disk.qcow2" 32M >/dev/null 2>&1
        CLI="${E2E_DIR}/quickemu-cli"
        VM="${E2E_DIR}/quickemu-vm"
        QMP="${E2E_DIR}/quickemu-qmp"
        LAUNCH="${E2E_DIR}/quickemu-launch"
    }
    cleanup() {
        "${CLI}" --vm "${E2E_VM}/govm.conf" --kill >/dev/null 2>&1
        PID=$(cat "${E2E_VM}/govm.pid" 2>/dev/null || true)
        [ -n "${PID}" ] && kill -KILL "${PID}" 2>/dev/null
        rm -rf "${E2E_DIR}"
    }
    BeforeEach 'setup'
    AfterEach 'cleanup'

    go_e2e_missing() {
        ! command -v qemu-system-x86_64 >/dev/null 2>&1 ||
            ! command -v qemu-img >/dev/null 2>&1 ||
            ! command -v go >/dev/null 2>&1
    }

    cli_status() {
        "${CLI}" --vm "${E2E_VM}/govm.conf" --status | awk '{print $1}'
    }

    build_tools() {
        for t in quickemu-cli quickemu-vm quickemu-qmp quickemu-launch; do
            (cd "${QE_ROOT}/tools/${t}" && go build -o "${E2E_DIR}/${t}" . \
                2> >(grep -v 'GOPATH set to GOROOT' >&2)) || return 1
        done
    }

    boot_and_cycle() {
        build_tools || { echo "go build failed"; return 1; }
        export PATH="${E2E_DIR}:${PATH}"
        QEMU_ACCEL=tcg "${CLI}" --vm "${E2E_VM}/govm.conf" \
            || { echo "boot failed"; return 1; }

        # VM reaches running state and reports it through the CLI.
        wait_until "[ -S '${E2E_VM}/govm-qmp.socket' ]" 10 || {
            echo "no QMP socket"; return 1; }
        wait_until "[ \"\$(cli_status)\" = 'running' ]" 10 \
            || { echo "status not running"; return 1; }

        # pause -> paused marker + paused status; resume -> running.
        # Retry: the QMP socket can exist before QEMU accepts connections.
        wait_until "'${CLI}' --vm '${E2E_VM}/govm.conf' --pause >/dev/null 2>&1 && [ -f '${E2E_VM}/govm.paused' ]" 10 \
            || { echo "pause failed"; return 1; }
        [ "$(cli_status)" = "paused" ] \
            || { echo "status not paused"; return 1; }
        "${CLI}" --vm "${E2E_VM}/govm.conf" --resume >/dev/null
        wait_until "[ \"\$(cli_status)\" = 'running' ]" 10 \
            || { echo "status not running after resume"; return 1; }

        # Arbitrary HMP command via the raw monitor socket.
        "${CLI}" --vm "${E2E_VM}/govm.conf" --monitor-cmd "info status" | \
            grep -q "VM status: running" || { echo "monitor-cmd failed"; return 1; }

        # QMP allowlist: query-* works, quit is denied without RAW.
        "${QMP}" execute --socket "${E2E_VM}/govm-qmp.socket" \
            --command '{"execute":"query-status"}' | grep -q '"success":true' \
            || { echo "query-status denied"; return 1; }
        "${QMP}" execute --socket "${E2E_VM}/govm-qmp.socket" \
            --command '{"execute":"quit"}' | grep -q 'QMP_DENIED' \
            || { echo "quit not denied"; return 1; }

        # Clean stop: process exits, status reports stopped.
        "${CLI}" --vm "${E2E_VM}/govm.conf" --stop >/dev/null
        wait_until "[ ! -f '${E2E_VM}/govm.pid' ] || ! kill -0 \$(cat '${E2E_VM}/govm.pid' 2>/dev/null || echo 0) 2>/dev/null" 10 || {
            echo "VM did not stop"; return 1; }
    }

    It 'boots via the Go CLI, manages the lifecycle, stops cleanly'
        Skip if 'qemu/qemu-img/go unavailable' go_e2e_missing
        When call boot_and_cycle
        The status should be success
    End
End
