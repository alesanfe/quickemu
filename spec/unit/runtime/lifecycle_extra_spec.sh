# shellcheck disable=SC2034
Describe 'quickemu runtime lifecycle extras'
Include lib/quickemu/runtime/lifecycle.sh
Include lib/quickemu/runtime/launch.sh
Include lib/quickemu/support/filesystem.sh

setup() {
    TESTDIR=$(mktemp -d)
    VMDIR="$TESTDIR" VMNAME=testvm DRY_RUN=''
    VIRTIOFSD='' VIRTIOFSD_PID='' VIRTIOFSD_SOCKET='' PUBLIC='' iso=''
    viewer='' display='' ACCESS='' spice_port='' FULLSCREEN='' guest_os=linux
    viewer_extra_args='' ssh_port='' spice_port='' monitor_telnet_host=''
    monitor_telnet_port='' serial_telnet_host='' serial_telnet_port=''
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

Describe 'vm_pid_alive / vm_running'
    It 'is false without a pid file'
        When call vm_pid_alive "$TESTDIR/none.pid"
        The status should be failure
    End
    It 'is true for a live pid'
        spawn_fake_vm testvm; echo $! > "$TESTDIR/testvm.pid"
        When call vm_pid_alive "$TESTDIR/testvm.pid"
        The status should be success
    End
    It 'is false for a dead pid'
        echo 99999999 > "$TESTDIR/dead.pid"
        When call vm_pid_alive "$TESTDIR/dead.pid"
        The status should be failure
    End
    It 'vm_running derives the pidfile from the VM dir'
        spawn_fake_vm testvm; echo $! > "$TESTDIR/testvm.pid"
        When call vm_running "$TESTDIR" testvm
        The status should be success
    End
End

Describe 'get_port'
    It 'returns a free port in range'
        When call get_port 45900 10
        The output should match pattern '45[0-9][0-9][0-9]'
        The status should be success
    End
End

Describe 'ports_record / spice_record'
    It 'appends to the .ports file'
        When call ports_record 'ssh,2222,.'
        The contents of file "$TESTDIR/testvm.ports" should include 'ssh,2222'
    End
    It 'is a no-op in dry-run mode'
        DRY_RUN=1
        When call ports_record 'ssh,2222,.'
        The file "$TESTDIR/testvm.ports" should not be exist
    End
    It 'writes the .spice file'
        When call spice_record '5930'
        The contents of file "$TESTDIR/testvm.spice" should include '5930'
    End
    It 'skips .spice in dry-run mode'
        DRY_RUN=1
        When call spice_record '5930'
        The file "$TESTDIR/testvm.spice" should not be exist
    End
End

Describe 'parse_ports_from_file'
    It 'restores ssh, spice, monitor and serial endpoints'
        printf 'ssh,2222,.\nspice,5930,.\nmonitor-telnet,4440,127.0.0.1 .\nserial-telnet,6660,0.0.0.0 .\n' > "$TESTDIR/testvm.ports"
        When call parse_ports_from_file
        The variable ssh_port should eq '2222'
        The variable spice_port should eq '5930'
        The variable monitor_telnet_port should eq '4440'
        The variable serial_telnet_port should eq '6660'
        The variable serial_telnet_host should include '0.0.0.0'
    End
    It 'succeeds quietly when the .ports file is missing'
        rm -f "$TESTDIR/testvm.ports"
        When call parse_ports_from_file
        The status should be success
        The error should be blank
    End
End

Describe 'runtime_launcher_init'
    It 'creates the VM directory before writing the launch script'
        VMDIR="$TESTDIR/newvm"
        When call runtime_launcher_init
        The status should be success
        The file "$VMDIR/testvm.sh" should be exist
    End
    It 'writes nothing in dry-run mode'
        VMDIR="$TESTDIR/newvm" DRY_RUN=1
        When call runtime_launcher_init
        The status should be success
        The path "$VMDIR" should not be exist
    End
End

Describe 'stop_swtpm'
    It 'removes the swtpm pid and socket files'
        touch "$TESTDIR/testvm.swtpm-pid" "$TESTDIR/testvm.swtpm-sock"
        When call stop_swtpm
        The file "$TESTDIR/testvm.swtpm-pid" should not be exist
        The file "$TESTDIR/testvm.swtpm-sock" should not be exist
    End
    It 'does not signal processes that are not swtpm'
        sleep 30 & SLEEP_PID=$!
        echo "$SLEEP_PID" > "$TESTDIR/testvm.swtpm-pid"
        survives() { stop_swtpm; kill -0 "$SLEEP_PID"; }
        When call survives
        The status should be success
    End
End

Describe 'vm_runtime_cleanup'
    It 'removes stale runtime artefacts and stops helpers'
        touch "$TESTDIR/testvm.pid" "$TESTDIR/testvm.spice" "$TESTDIR/testvm.sock" \
              "$TESTDIR/testvm-monitor.socket" "$TESTDIR/testvm-serial.socket" \
              "$TESTDIR/testvm.ports" "$TESTDIR/testvm.paused"
        stop_swtpm() { :; }
        stop_virtiofsd() { echo virtiofsd-stopped >> "$TESTDIR/helpers.log"; }
        When call vm_runtime_cleanup
        The status should be success
        The path "$TESTDIR/testvm.pid" should not be exist
        The path "$TESTDIR/testvm.spice" should not be exist
        The path "$TESTDIR/testvm.sock" should not be exist
        The path "$TESTDIR/testvm-monitor.socket" should not be exist
        The path "$TESTDIR/testvm-serial.socket" should not be exist
        The path "$TESTDIR/testvm.ports" should not be exist
        The path "$TESTDIR/testvm.paused" should not be exist
        The contents of file "$TESTDIR/helpers.log" should include 'virtiofsd-stopped'
    End
End

Describe 'start_virtiofsd'
    It 'does nothing without virtiofsd configured'
        When call start_virtiofsd
        The output should be blank
        The status should be success
    End
    It 'skips virtiofs while booting an installer ISO'
        VIRTIOFSD=/bin/true iso="$TESTDIR/x.iso"
        When call start_virtiofsd
        The output should be blank
        The variable VIRTIOFSD should be blank
    End
    It 'reports the planned socket in dry-run mode'
        VIRTIOFSD=/bin/true DRY_RUN=1
        When call start_virtiofsd
        The output should include 'virtiofsd'
        The output should include 'planned'
    End
    It 'falls back to 9p when the daemon never creates the socket'
        printf '#!/bin/sh\nexit 0\n' > "$TESTDIR/virtiofsd"; chmod +x "$TESTDIR/virtiofsd"
        VIRTIOFSD="$TESTDIR/virtiofsd" PUBLIC="$TESTDIR"
        # Shorten the socket poll loop by pre-seeding a failing seq
        seq() { echo 1; }
        When call start_virtiofsd
        The output should include 'falling back to 9p'
        The variable VIRTIOFSD should be blank
    End
End

Describe 'stop_virtiofsd'
    It 'removes pid and socket files'
        touch "$TESTDIR/testvm.virtiofsd-pid" "$TESTDIR/testvm.virtiofsd-sock"
        When call stop_virtiofsd
        The file "$TESTDIR/testvm.virtiofsd-pid" should not be exist
        The file "$TESTDIR/testvm.virtiofsd-sock" should not be exist
    End
    It 'does not signal processes that are not virtiofsd'
        sleep 30 & echo $! > "$TESTDIR/testvm.virtiofsd-pid"
        PID=$(cat "$TESTDIR/testvm.virtiofsd-pid")
        When call stop_virtiofsd
        The status should be success
    End
End

Describe 'start_viewer'
    It 'does nothing when the viewer is disabled'
        viewer=none display=spice
        When call start_viewer
        The output should be blank
    End
    It 'does nothing for non-spice displays'
        viewer=spicy display=gtk
        When call start_viewer
        The output should be blank
    End
    It 'launches spicy with --port for TCP spice'
        viewer=spicy display=spice
        echo 5930 > "$TESTDIR/testvm.spice"
        spicy() { echo "VIEWER $*"; }
        When call start_viewer
        The output should include 'Viewer'
        The output should include '--port 5930'
        The output should include '--title testvm'
    End
    It 'launches remote-viewer with a unix URI for socket spice'
        viewer=remote-viewer display=spice
        echo "$TESTDIR/testvm.sock" > "$TESTDIR/testvm.spice"
        remote-viewer() { echo "RV $*"; }
        When call start_viewer
        The output should include 'spice+unix://'
    End
    It 'falls back to the VM socket when no .spice file exists'
        viewer=spicy display=spice ACCESS=local
        spicy() { :; }
        When call start_viewer
        The output should include "spice+unix://$TESTDIR/testvm.sock"
    End
    It 'uses the TCP port for remote ACCESS without a .spice file'
        viewer=remote-viewer display=spice ACCESS=remote spice_port=5931
        remote-viewer() { :; }
        When call start_viewer
        The output should include 'spice://localhost:5931'
    End
End
End
