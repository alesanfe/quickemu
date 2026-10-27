# shellcheck disable=SC2034
# QE-04.4: adversarial QMP scenarios — socket substitution, foreign
# process, malformed replies. Validates the validation logic directly.
Describe 'adversarial: qmp socket defenses'
    Include lib/quickemu/runtime/monitor.sh

    setup() {
        TMPD=$(mktemp -d)
        export SOCKET_QMP="${TMPD}/q.sock"
        SOCAT_PID=""
    }
    cleanup() {
        [ -n "${SOCAT_PID}" ] && kill "${SOCAT_PID}" 2>/dev/null
        rm -rf "${TMPD}"
    }
    BeforeEach 'setup'
    AfterEach 'cleanup'

    It 'rejects a socket that is a symlink'
        target="${TMPD}/real.sock"
        # fd closure is mandatory: an inherited stdout pipe hangs the runner.
        socat -d -d UNIX-LISTEN:"${target}" EXEC:'/bin/true' >/dev/null 2>&1 &
        SOCAT_PID=$!
        sleep 0.3
        ln -sf "${target}" "${SOCKET_QMP}"
        When call vm_qmp_socket_ready
        The status should be failure
    End

    It 'rejects a non-socket file'
        echo x >"${SOCKET_QMP}"
        When call vm_qmp_socket_ready
        The status should be failure
    End

    It 'rejects absent socket'
        When call vm_qmp_socket_ready
        The status should be failure
    End

    It 'allowlist rejects device-add style commands'
        QUICKEMU_QMP_RAW=0
        When call qmp_command_allowed '{"execute":"device_add"}'
        The status should be failure
    End

    It 'allowlist rejects blockdev-add'
        QUICKEMU_QMP_RAW=0
        When call qmp_command_allowed '{"execute":"blockdev-add"}'
        The status should be failure
    End

    It 'allowlist accepts read-only and graceful ops'
        QUICKEMU_QMP_RAW=0
        When call qmp_command_allowed '{"execute":"query-status"}'
        The status should be success
    End

    It 'allowlist accepts system_powerdown'
        QUICKEMU_QMP_RAW=0
        When call qmp_command_allowed '{"execute":"system_powerdown"}'
        The status should be success
    End
End
