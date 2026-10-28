Describe 'CLI integration'
Include spec/spec_helper.sh

entry_setup() {
    TESTDIR=$(mktemp -d)
    FAKEBIN="$TESTDIR/bin"; mkdir -p "$FAKEBIN"
    cp "$QE_ROOT/spec/fake-bin/qemu-img" "$QE_ROOT/spec/fake-bin/qemu-system-x86_64" "$FAKEBIN/"
    chmod +x "$FAKEBIN"/*
    export PATH="$FAKEBIN:$PATH"
}
BeforeEach 'entry_setup'

It 'bin/quickemu delegates to the root script'
When call "$QE_ROOT/bin/quickemu" --capabilities
The output should include '"command":"capabilities"'
The status should be success
End

It '--help prints usage'
When call "$QE_ROOT/bin/quickemu" --help
The output should include '--vm'
The status should be success
End

Describe 'entry-point dispatch'
setup() {
    export STUB_QEMU_SLEEP=30
    cd "$TESTDIR" || return 1
    qe() { ( . "$QE_ROOT/bin/quickemu" "$@" ); }
}
cleanup() { cd /; qe_spec_cleanup; }
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

It 'imports a VM archive without an existing --vm configuration'
    mkdir -p src/vm-new && printf 'guest_os=linux\n' > src/vm-new.conf && : > src/vm-new/disk.qcow2
    tar -C src -czf vm-new.tar.gz vm-new.conf vm-new
    mkdir -p dest && cd dest
    When run qe --import "$TESTDIR/vm-new.tar.gz"
    The status should be success
    The output should include 'Imported vm-new'
    # No-manifest archive → integrity/review warnings on stderr.
    The stderr should match pattern '*'
    The file "$TESTDIR/dest/vm-new.conf" should be exist
    The directory "$TESTDIR/dest/vm-new" should be exist
End

It 'refuses --delete-vm while the VM is running'
    mkdir -p victim
    printf 'guest_os="linux"\ndisk_img="victim/disk.qcow2"\n' > victim.conf
    : > victim/disk.qcow2
    spawn_fake_vm victim; echo $! > victim/victim.pid
    When run qe --vm victim.conf --delete-vm
    The status should be failure
    The error should include 'before deleting'
    The directory "$TESTDIR/victim" should be exist
    The file "$TESTDIR/victim.conf" should be exist
End

It 'reports a paused VM through --status after --pause'
    mkdir -p paused-vm
    printf 'guest_os="linux"\ndisk_img="paused-vm/disk.qcow2"\n' > paused-vm.conf
    : > paused-vm/disk.qcow2
    spawn_fake_vm paused-vm; echo $! > paused-vm/paused-vm.pid
    monitor=socket
    : > paused-vm/paused-vm.paused
    When run qe --vm paused-vm.conf --status
    The status should be success
    The output should include 'paused'
End
End
End
