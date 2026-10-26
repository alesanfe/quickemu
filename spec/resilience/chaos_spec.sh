# Chaos: kill -9 leaves debris; next boot/lock must recover, not wedge.
Describe 'chaos recovery'
Include lib/quickemu/support/filesystem.sh
Include lib/quickemu/support/json.sh
Include lib/quickemu/support/events.sh
Include lib/quickemu/runtime/lifecycle.sh

setup() {
    TESTDIR=$(mktemp -d); VMDIR="$TESTDIR"; VMNAME="ch"
}
cleanup() { rm -rf -- "$TESTDIR"; }
BeforeEach 'setup'
AfterEach 'cleanup'

It 'lock owned by a kill -9 victim is reclaimed, not wedged'
    # Simulate a crashed locker: lockdir + pid of a dead process.
    mkdir -p "${VMDIR}/ch.lock"
    bash -c 'sleep 0.1' & DEAD=$!
    wait $DEAD 2>/dev/null
    echo "${DEAD}" >"${VMDIR}/ch.lock/pid"
    VM_LOCK_TRIES=5
    When call vm_lock_acquire
    The status should be success
End

It 'runtime files for a dead pid report stale, not running'
    # A real crashed VM leaves BOTH: pidfile + runtime.json.
    bash -c 'sleep 0.1' & DEAD=$!
    wait $DEAD 2>/dev/null
    echo "${DEAD}" >"${VMDIR}/ch.pid"
    printf '{"instanceId":"x","vmId":"ch","pid":%s,"processStartTime":1,"uid":0,"commandFingerprint":"sha256:x","configPath":"c","createdAt":"t"}\n' "${DEAD}" >"${VMDIR}/ch.runtime.json"
    When call vm_state
    The output should eq stale
End
End
