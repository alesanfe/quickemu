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
    SOCK_READY=1
}
cleanup() { rm -rf -- "$TESTDIR"; }
BeforeEach 'setup'
AfterEach 'cleanup'

Describe 'monitor_send_cmd'
    # The real vm_monitor_socket_ready performs the -S test; the seam is only
    # stubbed by the positive-path tests below it.
    It 'fails cleanly when the monitor socket is absent'
        monitor=socket
        When call monitor_send_cmd 'info status'
        The status should be failure
        The output should include 'No qemu-monitor socket'
    End
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
    vm_monitor_socket_ready() { return "${SOCK_READY}"; }
    It 'sends the command through the monitor socket'
        monitor=socket SOCK_READY=0
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
    It 'fails with a clear error when socat is missing'
        monitor=socket SOCK_READY=0
        command() { [ "$2" = 'socat' ] && return 1; builtin command "$@"; }
        When call monitor_send_cmd 'info status'
        The status should be failure
        The output should include 'socat is required'
    End
    It 'prints the monitor reply to stdout'
        monitor=socket SOCK_READY=0
        printf '#!/bin/sh\ncat >/dev/null\nprintf "QEMU 8.2.2 monitor - type help for more information\\n(qemu) \\n(qemu) running\\n"\n' > "$FAKEBIN/socat2"
        chmod +x "$FAKEBIN/socat2"
        socat() { "$FAKEBIN/socat2"; }
        When call monitor_send_cmd 'info status'
        The status should be success
        The output should include 'running'
        The output should not include '(qemu)'
    End
End
End
