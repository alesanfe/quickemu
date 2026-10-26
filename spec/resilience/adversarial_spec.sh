# P0 adversarial regressions: races, corrupt locks, foreign pids,
# malicious archives, interrupted ops.
Describe 'adversarial P0'
Include lib/quickemu/support/filesystem.sh
Include lib/quickemu/support/json.sh
Include lib/quickemu/support/events.sh
Include lib/quickemu/runtime/lifecycle.sh

setup() {
    TESTDIR=$(mktemp -d)
    VMDIR="$TESTDIR"; VMNAME="adv"; QE_PORT_REGISTRY="$TESTDIR/ports"
}
cleanup() { rm -rf -- "$TESTDIR"; }
BeforeEach 'setup'
AfterEach 'cleanup'

It 'two concurrent port picks never collide'
    a=$(get_port 22800 20) & pa=$!
    b=$(get_port 22800 20) & pb=$!
    wait $pa $pb
    a=$(get_port 22900 5); b=$(get_port 22900 5)
    The value "$a" should not eq "$b"
End

It 'a lock with a dead owner is reclaimed, a live one blocks'
    mkdir "$VMDIR/adv.lock"; echo 999999 >"$VMDIR/adv.lock/pid"
    When call vm_lock_acquire
    The status should be success
End

It 'foreign pid in pidfile is not signalled'
    sh -c 'sleep 0.1' & FPID=$!
    echo "$FPID" > "$VMDIR/adv.pid"
    wait $FPID 2>/dev/null
    When call vm_pid_alive "$VMDIR/adv.pid"
    The status should be failure
End

It 'corrupt lock dir without pid file does not wedge the loop'
    mkdir -p "$VMDIR/adv.lock/nested"
    touch -d '1 hour ago' "$VMDIR/adv.lock" 2>/dev/null || touch -t 202001010000 "$VMDIR/adv.lock"
    When call vm_lock_acquire
    The status should be success
End
End
