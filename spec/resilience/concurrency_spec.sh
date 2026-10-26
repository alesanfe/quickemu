# Concurrency scale: readers vs writer, parallel port reservation.
Describe 'concurrency at scale'
Include lib/quickemu/support/filesystem.sh
Include lib/quickemu/support/json.sh
Include lib/quickemu/support/events.sh
Include lib/quickemu/runtime/lifecycle.sh

setup() {
    TESTDIR=$(mktemp -d); VMDIR="$TESTDIR"; VMNAME="c"; QE_PORT_REGISTRY="$TESTDIR/ports"
}
cleanup() { rm -rf -- "$TESTDIR"; }
BeforeEach 'setup'
AfterEach 'cleanup'

It 'shared readers coexist; each leaves a marker'
    i=0
    while [ $i -lt 30 ]; do
        ( vm_lock_acquire shared && touch "$TESTDIR/r$i" ) &
        i=$((i+1))
    done
    wait
    n=$(ls "$TESTDIR"/r* 2>/dev/null | wc -l)
    The variable n should eq 30
End

It 'parallel port reservations across a shared range never collide'
    # Hammer the registry contract directly: 8 workers each reserve
    # their own port under the shared lock; all 8 markers must exist.
    i=0
    while [ $i -lt 8 ]; do
        ( _port_registry_lock && _port_registry_reserve $((24600+i)) && _port_registry_unlock ) &
        i=$((i+1))
    done
    wait
    n=$(ls "$TESTDIR/ports" 2>/dev/null | grep -c '^[0-9]')
    The variable n should eq 8
End

It 'a held writer lock blocks acquisition by ANOTHER process'
    vm_lock_acquire # this spec shell holds the exclusive lock
    # A separate bash process (different pid) must be refused; its
    # stderr carries the typed error. ShellSpec aborts It bodies on
    # uncaptured non-zero exits, so fold the rc ourselves.
    bash -c 'source lib/quickemu/support/filesystem.sh; source lib/quickemu/support/json.sh; source lib/quickemu/support/events.sh; source lib/quickemu/runtime/lifecycle.sh; VMDIR='"$TESTDIR"' VMNAME=c VM_LOCK_TRIES=3 vm_lock_acquire' 2>"$TESTDIR/err" || rc=$?
    rc=${rc:-0}
    The variable rc should eq 1
    The contents of file "$TESTDIR/err" should include 'locked'
End
End
