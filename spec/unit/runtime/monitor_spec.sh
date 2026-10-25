# shellcheck disable=SC2034
Describe 'quickemu monitor command channel'
Include lib/quickemu/runtime/monitor.sh

setup() {
    TESTDIR=$(mktemp -d)
    FAKEBIN="$TESTDIR/bin"; mkdir -p "$FAKEBIN"
    cp "${SHELLSPEC_PROJECT_ROOT}/spec/fake-bin/socat" "$FAKEBIN/socat"
    chmod +x "$FAKEBIN/socat"
    PATH="$FAKEBIN:$PATH"
    export STUB_SOCAT_LOG="$TESTDIR/socat.log"
    monitor='' SOCKET_MONITOR="$TESTDIR/mon.sock"
    monitor_telnet_host='' monitor_telnet_port=''
}
cleanup() { rm -rf -- "$TESTDIR"; }
BeforeEach 'setup'
AfterEach 'cleanup'

Describe 'monitor_send_cmd'
    It 'rejects an empty message'
        When call monitor_send_cmd ''
        The status should be failure
        The output should include 'Message empty'
    End
    It 'warns when no monitor channel is configured'
        monitor=none
        When call monitor_send_cmd 'info status'
        The status should be failure
        The output should include 'No qemu-monitor channel'
    End
    It 'sends the command through the monitor socket'
        monitor=socket
        When call monitor_send_cmd 'info status'
        The status should be success
        The output should include 'via socket'
        The contents of file "$STUB_SOCAT_LOG" should include 'info status'
    End
    It 'sends the command through telnet'
        monitor=telnet monitor_telnet_host=127.0.0.1 monitor_telnet_port=4440
        When call monitor_send_cmd 'system_powerdown'
        The status should be success
        The output should include 'via telnet'
        The contents of file "$STUB_SOCAT_LOG" should include 'system_powerdown'
    End
End
End
