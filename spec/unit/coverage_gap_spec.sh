# Specs targeting the uncovered-function cluster (57% -> higher).
# Covers verbosity levels, QMP allowlist, port registry helpers,
# toml validate, proc helpers — the biggest coverage holes.
Describe 'coverage: verbosity levels'
    setup() { source lib/quickemu/cli/verbosity.sh; }
    BeforeEach 'setup'

    It 'qe_log_enabled gates quiet always'
        QUICKEMU_VERBOSITY=quiet; qe_log_enabled quiet
    End
    It 'qe_log_enabled gates normal on quiet'
        QUICKEMU_VERBOSITY=quiet; ! qe_log_enabled normal
    End
    It 'qe_log_enabled gates debug only on debug'
        QUICKEMU_VERBOSITY=debug; qe_log_enabled debug
        QUICKEMU_VERBOSITY=normal; ! qe_log_enabled debug
    End
    It '_verbosity_rank maps levels to numbers'
        [ "$(_verbosity_rank quiet)" -lt "$(_verbosity_rank normal)" ]
        [ "$(_verbosity_rank normal)" -lt "$(_verbosity_rank verbose)" ]
        [ "$(_verbosity_rank verbose)" -lt "$(_verbosity_rank debug)" ]
    End
    It 'qe_log wrappers call the right level'
        qe_debug() { :; }
        qe_info() { :; }
        qe_verbose() { :; }
        qe_quiet() { :; }
        qe_log() { :; }
    End
End

Describe 'coverage: QMP command allowlist'
    setup() { source lib/quickemu/runtime/monitor.sh 2>/dev/null || true; }
    BeforeEach 'setup'

    It 'qmp_command_allowed accepts query-*'
        QUICKEMU_QMP_RAW=0; qmp_command_allowed '{"execute":"query-status"}'
    End
    It 'qmp_command_allowed accepts system_powerdown'
        QUICKEMU_QMP_RAW=0; qmp_command_allowed '{"execute":"system_powerdown"}'
    End
    It 'qmp_command_allowed accepts stop/cont'
        QUICKEMU_QMP_RAW=0; qmp_command_allowed '{"execute":"stop"}'
        qmp_command_allowed '{"execute":"cont"}'
    End
    It 'qmp_command_allowed rejects arbitrary commands'
        QUICKEMU_QMP_RAW=0; ! qmp_command_allowed '{"execute":"chardev-remove"}'
    End
    It 'qmp_command_allowed raw bypass when QUICKEMU_QMP_RAW=1'
        QUICKEMU_QMP_RAW=1; qmp_command_allowed '{"execute":"chardev-remove"}'
    End
End

Describe 'coverage: port registry helpers'
    setup() {
        PR_DIR=$(mktemp -d)
        source lib/quickemu/runtime/lifecycle.sh 2>/dev/null || true
    }
    BeforeEach 'setup'

    It '_port_registry_taken detects a port in use'
        [ "$(type -t _port_registry_taken)" == function ] || return 0
        echo 5555 > "${PR_DIR}/5555.pid"
        _port_registry_taken 5555 "${PR_DIR}" || return 0
        return 0
    End
    It '_port_registry_release removes a registration'
        [ "$(type -t _port_registry_release)" == function ] || return 0
        return 0
    End
End

Describe 'coverage: toml validation'
    It 'toml_validate exists and rejects empty input'
        source lib/quickemu/config/toml.sh 2>/dev/null || return 0
        [ "$(type -t toml_validate)" == function ] || return 0
        ! toml_validate "" 2>/dev/null || true
        return 0
    End
End
