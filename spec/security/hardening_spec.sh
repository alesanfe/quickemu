# shellcheck disable=SC2034
# Hardening: PID identity, delete confinement, archive safety, locking,
# JSON escaping and input validation added in the P0 hardening round.
Describe 'P0 hardening'
Include lib/quickemu/config/parser.sh
Include lib/quickemu/config/loader.sh
Include lib/quickemu/support/filesystem.sh
Include lib/quickemu/support/json.sh
Include lib/quickemu/runtime/monitor.sh
Include lib/quickemu/runtime/lifecycle.sh
Include lib/quickemu/vm/vm.sh
Include lib/quickemu/planning/delete_plan.sh
Include lib/quickemu/cli/commands.sh

setup() {
    TESTDIR=$(mktemp -d)
    VMDIR="$TESTDIR/vm1" VMNAME=vm1
    mkdir -p "$VMDIR"
    printf 'guest_os="linux"\ndisk_img="%s/disk.qcow2"\n' "$VMDIR" >"$TESTDIR/vm1.conf"
    VM="$TESTDIR/vm1.conf" disk_img="$VMDIR/disk.qcow2" monitor=none
    OUTPUT_FORMAT='' SEND_KEYS_ARG='' TYPE_TEXT=''
    SOCKET_MONITOR="$VMDIR/vm1-monitor.socket"
}
cleanup() { rm -rf -- "$TESTDIR" "$TESTDIR.lock" 2>/dev/null; }
BeforeEach 'setup'
AfterEach 'cleanup'
in_subshell() { ( "$@" ); }
spawn_vm_proc() { mkdir -p "$TESTDIR/fakeproc"; cp "$(command -v sleep)" "$TESTDIR/fakeproc/vm1"; "$TESTDIR/fakeproc/vm1" 60 >/dev/null 2>&1 & echo "$!" >"$VMDIR/vm1.pid"; sleep 0.2; }

Describe 'vm_pid_alive identity check'
    It 'rejects a live PID that is not a qemu process'
        sleep 30 >/dev/null 2>&1 &
        echo "$!" >"$VMDIR/vm1.pid"
        When call vm_pid_alive "$VMDIR/vm1.pid"
        The status should be failure
    End
End

Describe 'vm_instance_verify strong identity'
    It 'verifies a published instance as alive'
        spawn_vm_proc
        VM="$TESTDIR/vm1.conf" QEMU=qemu vm_instance_publish "$(cat "$VMDIR/vm1.pid")"
        When call vm_instance_verify "$VMDIR/vm1.pid"
        The output should equal 'alive'
        The status should be success
    End
    It 'reports runtime-corrupt for an empty runtime file'
        spawn_vm_proc
        : >"$VMDIR/vm1.runtime.json"
        When call vm_instance_verify "$VMDIR/vm1.pid"
        The output should equal 'runtime-corrupt'
        The status should be failure
    End
    It 'reports uid-mismatch when the runtime records a different owner'
        spawn_vm_proc
        VM="$TESTDIR/vm1.conf" QEMU=qemu vm_instance_publish "$(cat "$VMDIR/vm1.pid")"
        sed -i 's/"uid":[0-9]*/"uid":99999/' "$VMDIR/vm1.runtime.json"
        When call vm_instance_verify "$VMDIR/vm1.pid"
        The output should equal 'uid-mismatch'
        The status should be failure
    End
    It 'reports pid-reused when the process start-time differs'
        spawn_vm_proc
        VM="$TESTDIR/vm1.conf" QEMU=qemu vm_instance_publish "$(cat "$VMDIR/vm1.pid")"
        sed -i 's/"processStartTime":[0-9]*/"processStartTime":1/' "$VMDIR/vm1.runtime.json"
        When call vm_instance_verify "$VMDIR/vm1.pid"
        The output should equal 'pid-reused'
        The status should be failure
    End
    It 'reports foreign-process for a non-VM process without runtime'
        sleep 60 >/dev/null 2>&1 &
        echo "$!" >"$VMDIR/vm1.pid"
        When call vm_instance_verify "$VMDIR/vm1.pid"
        The output should equal 'foreign-process'
        The status should be failure
    End
    It 'reports dead for a terminated instance'
        spawn_vm_proc
        VM="$TESTDIR/vm1.conf" QEMU=qemu vm_instance_publish "$(cat "$VMDIR/vm1.pid")"
        kill "$(cat "$VMDIR/vm1.pid")" 2>/dev/null
        sleep 0.3
        When call vm_instance_verify "$VMDIR/vm1.pid"
        The output should equal 'dead'
        The status should be failure
    End
End

Describe 'delete_vm path confinement'
    It 'refuses to delete a VMDIR outside the conf directory'
        OUTSIDE="$TESTDIR/outside"
        mkdir -p "$OUTSIDE"
        disk_img="$OUTSIDE/disk.qcow2"
        vm_identify "$VM"
        When call in_subshell delete_vm
        The status should be failure
        The error should include 'refuses'
        The directory "$OUTSIDE" should be exist
    End
    It 'refuses dangerous VMDIR targets like HOME or /'
        disk_img="$HOME/x" vm_identify "$VM"
        When call in_subshell delete_vm
        The status should be failure
        The error should include 'refuses'
    End
    It 'still deletes a legit VMDIR inside the conf directory'
        VMDIR="$TESTDIR/vm1" vm_identify "$VM"
        When call in_subshell delete_vm
        The status should be success
        The output should include 'Deleted'
        The directory "$VMDIR" should not be exist
    End
End

Describe 'import_vm archive validation'
    It 'rejects archives containing path traversal members'
        : >"$TESTDIR/outside_marker"
        mkdir -p "$TESTDIR/pack" && : >"$TESTDIR/pack/evil.conf"
        python3 - "$TESTDIR" <<'EOF'
import tarfile, sys, os
d = sys.argv[1]
with tarfile.open(os.path.join(d, 'evil.tar.gz'), 'w:gz') as t:
    t.add(os.path.join(d, 'pack', 'evil.conf'), arcname='evil.conf')
    t.add(os.path.join(d, 'outside_marker'), arcname='../outside_marker')
EOF
        IMPORT_FILE="$TESTDIR/evil.tar.gz"
        mkdir -p "$TESTDIR/importcwd"
        in_cwd() { ( cd "$TESTDIR/importcwd" && "$@" ); }
        When call in_cwd import_vm
        The status should be failure
        The error should include 'Unsafe path'
        The file "$TESTDIR/importcwd/outside_marker" should not be exist
    End
    It 'rejects archives containing symlink members'
        mkdir -p "$TESTDIR/pack2" && : >"$TESTDIR/pack2/evil.conf"
        python3 - "$TESTDIR" <<'EOF'
import tarfile, sys, os
d = sys.argv[1]
with tarfile.open(os.path.join(d, 'link.tar.gz'), 'w:gz') as t:
    t.add(os.path.join(d, 'pack2', 'evil.conf'), arcname='evil.conf')
    i = tarfile.TarInfo('linkdir/evil'); i.type = tarfile.SYMTYPE; i.linkname = '/etc/passwd'
    t.addfile(i)
EOF
        IMPORT_FILE="$TESTDIR/link.tar.gz"
        mkdir -p "$TESTDIR/importcwd2"
        in_cwd2() { ( cd "$TESTDIR/importcwd2" && "$@" ); }
        When call in_cwd2 import_vm
        The status should be failure
        The error should include 'links or special'
    End
End

Describe 'VM locking'
    It 'blocks a second concurrent operation while the lock is held'
        vm_lock_acquire || return 1
        When call vm_lock_acquire
        The status should be failure
        The error should include 'locked'
    End
    It 'releases the lock so a later operation can proceed'
        vm_lock_acquire || return 1
        vm_lock_release
        When call vm_lock_acquire
        The status should be success
        vm_lock_release
    End
    It 'reclaims a lock whose owner process is gone'
        mkdir -p "$VMDIR/vm1.lock"
        echo 999999 >"$VMDIR/vm1.lock/pid"
        When call vm_lock_acquire
        The status should be success
        vm_lock_release
    End
    It 'emits a stable planId and delete-vm verifies it'
        OUTPUT_FORMAT=json
        When call delete_plan
        The output should include '"planId":"sha256:'
    End
    It 'refuses delete when the planId does not match'
        OUTPUT_FORMAT=''
        DELETE_PLAN_ID='sha256:bogus'
        When call in_subshell delete_vm
        The status should be failure
        The error should include 'stale'
        The directory "$VMDIR" should be exist
    End
    It 'deletes when the planId matches the current plan'
        OUTPUT_FORMAT=''
        ITEMS=()
        delete_plan_items
        DELETE_PLAN_ID=$(delete_plan_id)
        When call in_subshell delete_vm
        The status should be success
        The output should include 'Deleted'
        The directory "$VMDIR" should not be exist
    End
End

Describe 'JSON emitter escaping'
    It 'escapes quotes in vm names for --status json'
        VMNAME='vm"inj' VMDIR="$TESTDIR/x" VM_PID='' OUTPUT_FORMAT=json QE_SCHEMA_VERSION=1
        When call in_subshell status_vm
        The status should be success
        The output should include 'vm\"inj'
    End
    It 'emits valid JSON for names containing quotes'
        OUTPUT_FORMAT=json QE_SCHEMA_VERSION=1
        cd "$TESTDIR" || return 1
        When call list_vms
        The status should be success
        The output should include '"vms"'
        The output should include 'vm1'
    End
End

Describe 'network input validation'
    It 'rejects a malformed macaddr'
        macaddr='not,a,mac'
        When call in_subshell network_param_check
        The status should be failure
        The output should include 'MAC'
    End
    It 'accepts a well-formed macaddr'
        macaddr='52:54:00:AB:51:AE'
        When call network_param_check
        The status should be success
    End
    It 'rejects malformed port_forwards entries'
        port_forwards=('8080')
        When call in_subshell ports_param_check
        The status should be failure
        The output should include 'port forward'
    End
End

Describe 'conf inspection without execution'
    It 'inspect_conf reads values without running the file'
        CANARY="$TESTDIR/pwned"
        printf 'guest_os="linux"\nmal="$(touch %s)"\n' "$CANARY" >"$TESTDIR/vm2.conf"
        When call inspect_conf "$TESTDIR/vm2.conf"
        The status should be success
        The output should include 'linux'
        The file "$CANARY" should not be exist
    End
End
End
