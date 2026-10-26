# shellcheck disable=SC2034
# Resilience tests: corrupt inputs, missing dependencies and partial
# state must degrade to clean errors, never to silent corruption.
Describe 'resilience to degraded inputs'
Include spec/spec_helper.sh
Include lib/quickemu/support/filesystem.sh
Include lib/quickemu/config/parser.sh
Include lib/quickemu/support/filesystem.sh
Include lib/quickemu/config/loader.sh
Include lib/quickemu/support/filesystem.sh
Include lib/quickemu/support/validation.sh
Include lib/quickemu/support/filesystem.sh
Include lib/quickemu/support/json.sh
Include lib/quickemu/support/filesystem.sh
Include lib/quickemu/runtime/monitor.sh

setup() {
    TESTDIR=$(mktemp -d)
}
cleanup() { rm -rf -- "$TESTDIR"; }
BeforeEach 'setup'
AfterEach 'cleanup'

Describe 'corrupt or odd conf lines'
    It 'returns the raw line when there is no = separator'
        When call conf_parse_value 'guest_os'
        The output should eq 'guest_os'
        The status should be success
    End
    It 'survives a value containing ='
        When call conf_parse_value 'extra_args="-device x=y"'
        The output should eq '-device x=y'
    End
    It 'conf_get ignores comment lines'
        printf '# comment\nguest_os="linux"\n' > "$TESTDIR/vm.conf"
        When call conf_get "$TESTDIR/vm.conf" guest_os
        The output should eq 'linux'
    End
End

Describe 'missing dependencies'
    It 'monitor_send_cmd survives a missing socat'
        monitor=socket SOCKET_MONITOR=/nonexistent
        mkdir -p "$TESTDIR/none"
        no_socat() { ( PATH="$TESTDIR/none"; monitor=socket SOCKET_MONITOR=/x monitor_send_cmd 'info status' ); }
        When run no_socat
        # socat missing -> clean, explicit failure (no hang, no silent corruption)
        The status should be failure
        The output should include 'socat is required'
    End
End

Describe 'interrupted or partial state'
    It 'monitor_send_cmd handles a dead monitor socket'
        monitor=socket SOCKET_MONITOR=/nonexistent.sock
        # socat must exist so the channel checks are what fail
        printf '#!/bin/sh\ncat >/dev/null\n' >"$TESTDIR/socat" && chmod +x "$TESTDIR/socat"
        PATH="$TESTDIR:$PATH"
        When call monitor_send_cmd 'info status'
        The status should be failure
        The output should include 'No qemu-monitor socket'
    End
End
End
