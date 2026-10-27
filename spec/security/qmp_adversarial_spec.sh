# QE-04.4: adversarial QMP scenarios — socket substitution, foreign
# process, malformed replies. Validates the validation logic directly.
Describe 'adversarial: qmp socket defenses'
    setup() {
        TMPD=$(mktemp -d)
        source lib/quickemu/runtime/monitor.sh 2>/dev/null || true
        export SOCKET_QMP="${TMPD}/q.sock"
    }
    BeforeEach 'setup'
    cleanup() { rm -rf "${TMPD}"; }
    AfterEach 'cleanup'

    It 'rejects a socket that is a symlink'
        target="${TMPD}/real.sock"
        socat -d -d UNIX-LISTEN:"${target}" EXEC:'/bin/true' & sleep 0.3
        ln -sf "${target}" "${SOCKET_QMP}"
        ! vm_qmp_socket_ready
    End

    It 'rejects a non-socket file'
        echo x > "${SOCKET_QMP}"
        ! vm_qmp_socket_ready
    End

    It 'rejects absent socket'
        ! vm_qmp_socket_ready
    End

    It 'allowlist rejects device-add style commands'
        ! qmp_command_allowed '{"execute":"device_add"}'
        ! qmp_command_allowed '{"execute":"blockdev-add"}'
    End

    It 'allowlist accepts read-only and graceful ops'
        qmp_command_allowed '{"execute":"query-status"}'
        qmp_command_allowed '{"execute":"system_powerdown"}'
        qmp_command_allowed '{"execute":"stop"}'
        qmp_command_allowed '{"execute":"cont"}'
    End
End
