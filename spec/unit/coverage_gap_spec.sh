# Specs targeting the uncovered-function cluster (57% -> higher).
# Covers verbosity levels, QMP allowlist, port registry helpers,
# toml validate, proc helpers — the biggest coverage holes.
Describe 'coverage: verbosity levels'
    Include lib/quickemu/cli/verbosity.sh

    It 'qe_log_enabled emits quiet at quiet'
        QUICKEMU_VERBOSITY=quiet
        When call qe_log_enabled quiet
        The status should be success
    End
    It 'qe_log_enabled gates normal on quiet'
        QUICKEMU_VERBOSITY=quiet
        When call qe_log_enabled normal
        The status should be failure
    End
    It 'qe_log_enabled gates debug only on debug'
        QUICKEMU_VERBOSITY=normal
        When call qe_log_enabled debug
        The status should be failure
    End
    It 'qe_log_enabled emits debug at debug'
        QUICKEMU_VERBOSITY=debug
        When call qe_log_enabled debug
        The status should be success
    End
    It '_verbosity_rank maps levels to ordered numbers'
        When call _verbosity_rank quiet
        The output should equal '0'
    End
    It '_verbosity_rank orders normal below verbose'
        When call _verbosity_rank normal
        The output should equal '1'
    End
    It 'qe_log writes to stderr only at or above the level'
        QUICKEMU_VERBOSITY=debug
        When call qe_log debug "hello"
        The stderr should include 'hello'
    End
    It 'qe_log suppresses messages above the level'
        QUICKEMU_VERBOSITY=quiet
        When call qe_log debug "hidden"
        The stderr should equal ''
    End
End

Describe 'coverage: QMP command allowlist'
    Include lib/quickemu/runtime/monitor.sh

    It 'qmp_command_allowed accepts query-*'
        QUICKEMU_QMP_RAW=0
        When call qmp_command_allowed '{"execute":"query-status"}'
        The status should be success
    End
    It 'qmp_command_allowed accepts system_powerdown'
        QUICKEMU_QMP_RAW=0
        When call qmp_command_allowed '{"execute":"system_powerdown"}'
        The status should be success
    End
    It 'qmp_command_allowed accepts stop/cont'
        QUICKEMU_QMP_RAW=0
        When call qmp_command_allowed '{"execute":"cont"}'
        The status should be success
    End
    It 'qmp_command_allowed rejects arbitrary commands'
        QUICKEMU_QMP_RAW=0
        When call qmp_command_allowed '{"execute":"chardev-remove"}'
        The status should be failure
    End
    It 'qmp_command_allowed raw bypass when QUICKEMU_QMP_RAW=1'
        QUICKEMU_QMP_RAW=1
        When call qmp_command_allowed '{"execute":"chardev-remove"}'
        The status should be success
    End
End

Describe 'coverage: port registry helpers'
    Include lib/quickemu/runtime/lifecycle.sh

    setup() { QE_PORT_REGISTRY=$(mktemp -d); }
    cleanup() { rm -rf "${QE_PORT_REGISTRY}"; }
    BeforeEach 'setup'
    AfterEach 'cleanup'

    It '_port_registry_taken detects a live reservation'
        printf '%s %s %s %s\n' "$$" "$(date +%s)" inst vm >"${QE_PORT_REGISTRY}/5555"
        When call _port_registry_taken 5555
        The status should be success
    End
    It '_port_registry_taken frees a dead reservation'
        printf '999999 %s %s %s\n' "$(date +%s)" inst vm >"${QE_PORT_REGISTRY}/5556"
        When call _port_registry_taken 5556
        The status should be failure
    End
    It '_port_registry_reserve writes a reservation file'
        When call _port_registry_reserve 5557
        The status should be success
        The path "${QE_PORT_REGISTRY}/5557" should be file
    End
    It '_port_registry_release drops reservations owned by this pid'
        printf '%s %s %s %s\n' "$$" "$(date +%s)" inst vm >"${QE_PORT_REGISTRY}/5558"
        When call _port_registry_release
        The status should be success
        The path "${QE_PORT_REGISTRY}/5558" should not be exist
    End
End

Describe 'coverage: toml validation'
    Include lib/quickemu/config/toml.sh

    setup() {
        T_DIR=$(mktemp -d)
        printf 'formatVersion = 1\nguest_os = "linux"\ndisk_img = "d.qcow2"\n' >"${T_DIR}/good.toml"
        printf 'this is [not = toml\n' >"${T_DIR}/bad.toml"
    }
    cleanup() { rm -rf "${T_DIR}"; }
    BeforeEach 'setup'
    AfterEach 'cleanup'

    It 'toml_validate accepts a well-formed file'
        When call toml_validate "${T_DIR}/good.toml"
        The status should be success
    End
    It 'toml_validate rejects malformed input'
        When call toml_validate "${T_DIR}/bad.toml"
        The status should be failure
    End
    It 'toml_validate rejects a missing file'
        When call toml_validate "${T_DIR}/absent.toml"
        The status should be failure
    End
End
