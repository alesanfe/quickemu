# shellcheck shell=bash
Describe 'trash'
Include lib/quickemu/support/json.sh
Include lib/quickemu/support/filesystem.sh
Include lib/quickemu/support/events.sh
Include lib/quickemu/runtime/monitor.sh
Include lib/quickemu/runtime/ops.sh
Include lib/quickemu/runtime/lifecycle.sh
Include lib/quickemu/vm/vm.sh

setup() { TESTDIR=$(mktemp -d); export XDG_DATA_HOME="$TESTDIR/xdg"; VMNAME=vm1; VMDIR="$TESTDIR/vm1"; VM_PID=""; }
cleanup() { rm -rf -- "$TESTDIR"; }
BeforeEach 'setup'
AfterEach 'cleanup'

    It 'restore rejects an unknown VM name'
        RESTORE_VM="nope"
        When call trash_restore
        The status should be failure
        The error should include 'No trash entry'
    End
    It 'restores a trashed VM to its origin dir'
        ROOT="$(trash_root)"; SLOT="$ROOT/20260101T000000Z-vm1"
        mkdir -p "$SLOT/vm/vm1" "$TESTDIR/origin"
        printf 'guest_os="linux"\n' >"$SLOT/vm/vm1.conf"
        touch "$SLOT/vm/vm1/disk.qcow2"
        printf '{"vm":"vm1","trashedAt":"x","originConfDir":"%s"}\n' "$TESTDIR/origin" >"$SLOT/trash-meta.json"
        RESTORE_VM="vm1"
        When call trash_restore
        The status should be success
        The output should include 'Restored vm1'
        The contents of file "$TESTDIR/origin/vm1.conf" should include 'guest_os'
        The path "$TESTDIR/origin/vm1/disk.qcow2" should be exist
        The path "$SLOT" should not be exist
    End
    It 'refuses to restore over an existing VM'
        ROOT="$(trash_root)"; SLOT="$ROOT/20260101T000000Z-vm1"
        mkdir -p "$SLOT/vm" "$TESTDIR/origin"
        printf 'x\n' >"$SLOT/vm/vm1.conf"; printf 'x\n' >"$TESTDIR/origin/vm1.conf"
        printf '{"vm":"vm1","originConfDir":"%s"}\n' "$TESTDIR/origin" >"$SLOT/trash-meta.json"
        RESTORE_VM="vm1"
        When call trash_restore
        The status should be failure
        The error should include 'already exists'
    End
End
