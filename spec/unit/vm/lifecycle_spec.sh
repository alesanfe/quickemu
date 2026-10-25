Describe 'VM runtime lifecycle'
Include lib/quickemu/runtime/monitor.sh
Include lib/quickemu/runtime/lifecycle.sh

setup() {
    TESTDIR=$(mktemp -d)
    VMDIR="$TESTDIR"
    VMNAME=vm-test
    VIRTIOFSD=''
    VIRTIOFSD_PID=''
    VIRTIOFSD_SOCKET=''
    iso=''
    DRY_RUN=''
    PUBLIC=''
    viewer=none
    display=sdl
    spice_port=''
    monitor_telnet_port=''
    monitor_telnet_host=''
    serial_telnet_port=''
    serial_telnet_host=''
    ssh_port=''
    guest_os=linux
    FULLSCREEN=''
    viewer_extra_args=''
    ACCESS=local
    FAKE_BIN="$TESTDIR/bin"
    mkdir -p "$FAKE_BIN"
}
cleanup() { qe_spec_cleanup; }
BeforeEach 'setup'
AfterEach 'cleanup'
# QEMU renames its process via -name <vm>,process=<vm>; spawn a fake VM
# whose comm matches so vm_pid_alive's identity check passes.
spawn_fake_vm() {
    # Copy sleep under the VM name so the process comm matches what QEMU's
    # -name <vm>,process=<vm> renaming produces (exec -a does not change
    # comm on all platforms).
    local name="${1:-${VMNAME:-vm-test}}" fb="${FAKE_BIN:-${FAKEBIN:-${TESTDIR}/bin}}"
    mkdir -p "$fb"; cp /bin/sleep "$fb/${name}"; "$fb/${name}" "${2:-60}" >/dev/null 2>&1 &
}

# Records its argv into $ARGV_FILE for later assertion.
make_spy() { printf '#!/bin/sh\nprintf "%%s\\n" "$@" >"%s"\n' "$ARGV_FILE" >"$FAKE_BIN/$1"; chmod +x "$FAKE_BIN/$1"; }

Describe 'start_virtiofsd'
It 'does nothing when virtiofsd is not configured'
    VIRTIOFSD=''
    When call start_virtiofsd
    The status should be success
    The variable VIRTIOFSD_SOCKET should eq ''
End
It 'clears virtiofsd when booting the installer ISO'
    VIRTIOFSD=/bin/true iso="$TESTDIR/installer.iso"
    When call start_virtiofsd
    The status should be success
    The variable VIRTIOFSD should eq ''
End
It 'reports the planned socket without touching the filesystem in dry-run'
    VIRTIOFSD=/bin/true DRY_RUN=1
    When call start_virtiofsd
    The status should be success
    The output should include 'planned'
    The variable VIRTIOFSD_SOCKET should include "$VMNAME.virtiofsd-sock"
    The file "$VMDIR/$VMNAME.virtiofsd-sock" should not be exist
End
It 'falls back to 9p with a clear warning when the daemon cannot create the socket'
    VIRTIOFSD=/bin/false
    When call start_virtiofsd
    The status should be success
    The output should include 'falling back to 9p'
    The variable VIRTIOFSD should eq ''
End
End

Describe 'stop_virtiofsd'
It 'terminates a live virtiofsd and removes its runtime files'
    # Fake ps reports the probed process as virtiofsd; kill is a bash builtin.
    printf '#!/bin/sh\necho virtiofsd\n' >"$FAKE_BIN/ps"
    chmod +x "$FAKE_BIN/ps"
    PATH="$FAKE_BIN:$PATH"
    sleep 60 >/dev/null 2>&1 </dev/null &
    DAEMON_PID=$!
    VIRTIOFSD_PID=$DAEMON_PID
    echo "$DAEMON_PID" >"$VMDIR/$VMNAME.virtiofsd-pid"
    : >"$VMDIR/$VMNAME.virtiofsd-sock"
    When call stop_virtiofsd
    The status should be success
    The file "$VMDIR/$VMNAME.virtiofsd-pid" should not be exist
    The file "$VMDIR/$VMNAME.virtiofsd-sock" should not be exist
    The variable VIRTIOFSD_PID should eq ''
End
It 'removes a stale pid file left by an unclean shutdown'
    echo 999999 >"$VMDIR/$VMNAME.virtiofsd-pid"
    When call stop_virtiofsd
    The status should be success
    The file "$VMDIR/$VMNAME.virtiofsd-pid" should not be exist
End
It 'does nothing when no daemon was started'
    When call stop_virtiofsd
    The status should be success
End
End

Describe 'start_viewer'
Parameters
#   viewer         spice_file        access   expected_fragment
    spicy          '/vm.sock'        local    '--uri=spice+unix:///vm.sock'
    remote-viewer  '/vm.sock'        local    'spice+unix:///vm.sock'
    spicy          '5930'            local    '--port'
    remote-viewer  '5930'            local    'spice://localhost:5930'
    spicy          -                 remote   '--port'
    remote-viewer  -                 remote   'spice://localhost:5931'
End
It 'builds the viewer command line from the runtime spice state'
    ARGV_FILE="$TESTDIR/argv"
    make_spy "$1"
    viewer="$1"
    display=spice
    spice_port=5931
    [ "$2" != '-' ] && printf '%s' "$2" >"$VMDIR/$VMNAME.spice"
    ACCESS="$3"
    PATH="$FAKE_BIN:$PATH"
    When call start_viewer
    The status should be success
    The output should include 'Viewer:'
    # The viewer runs in the background; poll for the spy's argv file.
    for _ in $(seq 1 50); do [ -f "$ARGV_FILE" ] && break; sleep 0.2; done
    The file "$ARGV_FILE" should be exist
    The contents of file "$ARGV_FILE" should include "$4"
    The contents of file "$ARGV_FILE" should include "$VMNAME"
End
End

Describe 'start_viewer early returns and options'
It 'returns early when the viewer is disabled'
    viewer=none display=spice
    When call start_viewer
    The status should be success
    The output should be blank
End
It 'returns early for non-SPICE displays'
    viewer=remote-viewer display=sdl
    When call start_viewer
    The status should be success
    The output should be blank
End
It 'omits the shared folder flag for macOS guests'
    ARGV_FILE="$TESTDIR/argv"
    make_spy spicy
    viewer=spicy display=spice guest_os=macos PUBLIC=/shared
    printf '/vm.sock' >"$VMDIR/$VMNAME.spice"
    PATH="$FAKE_BIN:$PATH"
    When call start_viewer
    The status should be success
    The output should include 'Viewer:'
    The contents of file "$ARGV_FILE" should not include 'spice-shared-dir'
End
It 'forwards fullscreen and extra arguments to the viewer'
    ARGV_FILE="$TESTDIR/argv"
    make_spy remote-viewer
    viewer=remote-viewer display=spice FULLSCREEN=--full-screen viewer_extra_args='--hotkeys=ctrl'
    printf '5932' >"$VMDIR/$VMNAME.spice"
    PATH="$FAKE_BIN:$PATH"
    # The viewer is spawned in the background; wait for the spy to record argv
    viewer_wait() { start_viewer; for _ in $(seq 1 50); do [ -f "$ARGV_FILE" ] && break; sleep 0.1; done; }
    When call viewer_wait
    The status should be success
    The output should include 'Viewer:'
    The contents of file "$ARGV_FILE" should include '--full-screen'
    The contents of file "$ARGV_FILE" should include '--hotkeys=ctrl'
End
End

Describe 'VM pidfiles'
It 'reports a live process from its pidfile'
    spawn_fake_vm
    echo $! >"$TESTDIR/vm-test.pid"
    When call vm_pid_alive "$TESTDIR/vm-test.pid"
    The status should be success
End
It 'reports a missing pidfile as not running'
    When call vm_pid_alive "$TESTDIR/nope.pid"
    The status should be failure
End
It 'reports a dead process as not running'
    echo 999999 >"$TESTDIR/dead.pid"
    When call vm_pid_alive "$TESTDIR/dead.pid"
    The status should be failure
End
It 'derives the pidfile name from the VM directory'
    spawn_fake_vm
    echo $! >"$TESTDIR/$VMNAME.pid"
    When call vm_running "$TESTDIR" "$VMNAME"
    The status should be success
End
End

Describe 'host port allocation'
It 'skips a port that is already bound'
    # The listener signals readiness via a flag file instead of relying on
    # platform-specific /proc/net/tcp parsing or fixed sleeps.
    python3 - 5930 "$TESTDIR/ready" >/dev/null 2>&1 <<'EOF' &
import socket,sys,time
s=socket.socket(); s.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1)
s.bind(('127.0.0.1',int(sys.argv[1]))); s.listen(5)
open(sys.argv[2],'w').close(); time.sleep(10)
EOF
    for _ in $(seq 1 100); do [ -f "$TESTDIR/ready" ] && break; sleep 0.1; done
    [ -f "$TESTDIR/ready" ] || return 1
    When call get_port 5930 5
    The status should be success
    The output should not eq '5930'
End
It 'returns the first free port in the range'
    When call get_port 15930 5
    The status should be success
    The output should eq '15930'
End
End

Describe 'runtime port bookkeeping'
It 'parses recorded ssh spice and telnet endpoints'
    cat >"$VMDIR/$VMNAME.ports" <<'EOF'
ssh,22220,127.0.0.1
spice,5930,127.0.0.1
monitor-telnet,4440,127.0.0.1
serial-telnet,6660,127.0.0.1
EOF
    When call parse_ports_from_file
    The status should be success
    The variable ssh_port should eq '22220'
    The variable spice_port should eq '5930'
    The variable monitor_telnet_port should eq '4440'
    The variable serial_telnet_port should eq '6660'
    The variable monitor_telnet_host should include '127.0.0.1'
End
It 'appends runtime port records unless planning'
    When call ports_record 'ssh,22220,127.0.0.1'
    The status should be success
    The contents of file "$VMDIR/$VMNAME.ports" should eq 'ssh,22220,127.0.0.1'
End
It 'does not write port records in dry-run mode'
    DRY_RUN=1
    When call ports_record 'ssh,22220,127.0.0.1'
    The status should be success
    The file "$VMDIR/$VMNAME.ports" should not be exist
End
It 'records where the SPICE viewer should connect'
    When call spice_record '/run/vm.sock'
    The status should be success
    The contents of file "$VMDIR/$VMNAME.spice" should eq '/run/vm.sock'
End
It 'does not write the spice endpoint in dry-run mode'
    DRY_RUN=1
    When call spice_record '/run/vm.sock'
    The status should be success
    The file "$VMDIR/$VMNAME.spice" should not be exist
End
End
End
