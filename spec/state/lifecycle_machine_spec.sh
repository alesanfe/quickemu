# shellcheck disable=SC2034
# State-machine tests: model the VM lifecycle (absent -> configured ->
# running -> stopped) and prove valid/invalid transitions behave.
Describe 'VM lifecycle state machine'
Include lib/quickemu/runtime/monitor.sh
Include lib/quickemu/support/filesystem.sh
Include lib/quickemu/runtime/lifecycle.sh

setup() {
    TESTDIR=$(mktemp -d)
    VMDIR="$TESTDIR" VMNAME=vm1 DRY_RUN=''
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

Describe 'absent -> running -> absent transitions'
    It 'absent: vm_running is false without a pid file'
        When call vm_running "$TESTDIR" vm1
        The status should be failure
    End
    It 'running: a live pid makes vm_running true'
        spawn_fake_vm vm1; echo $! > "$TESTDIR/vm1.pid"
        When call vm_running "$TESTDIR" vm1
        The status should be success
    End
    It 'stopped: a dead pid makes vm_running false again'
        echo 99999999 > "$TESTDIR/vm1.pid"
        When call vm_running "$TESTDIR" vm1
        The status should be failure
    End
    It 'records endpoint state only when running (not in plan mode)'
        record_both() { ports_record 'ssh,2222,.'; spice_record '5930'; }
        DRY_RUN=1
        record_both
        The file "$TESTDIR/vm1.ports" should not be exist
        The file "$TESTDIR/vm1.spice" should not be exist
        DRY_RUN=''
        record_both
        The contents of file "$TESTDIR/vm1.ports" should include 'ssh,2222'
        The contents of file "$TESTDIR/vm1.spice" should include '5930'
    End
    It 'restores endpoints recorded by a previous run'
        printf 'ssh,2222,.\nspice,5930,.\n' > "$TESTDIR/vm1.ports"
        When call parse_ports_from_file
        The variable ssh_port should eq '2222'
        The variable spice_port should eq '5930'
    End
    It 'stop clears virtiofsd runtime state'
        touch "$TESTDIR/vm1.virtiofsd-pid" "$TESTDIR/vm1.virtiofsd-sock"
        When call stop_virtiofsd
        The file "$TESTDIR/vm1.virtiofsd-pid" should not be exist
        The file "$TESTDIR/vm1.virtiofsd-sock" should not be exist
        The variable VIRTIOFSD_PID should be blank
    End
End
End
