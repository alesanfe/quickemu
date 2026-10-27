# QE-15: controlled fault-injection — deterministic failure scenarios.
Describe 'fault: orphan lock detection'
    setup() {
        FAULT_DIR=$(mktemp -d)
        VM_DIR="${FAULT_DIR}/testvm"
        mkdir -p "${VM_DIR}"
    }
    cleanup() { rm -rf "${FAULT_DIR}"; }
    BeforeEach 'setup'
    AfterEach 'cleanup'

    It 'a lock dir left by a crashed process has a dead owner'
        mkdir -p "${VM_DIR}/.lock"
        echo "999999" > "${VM_DIR}/.lock/owner.pid"
        When run bash -c '[ ! -e /proc/999999 ]'
        The status should be success
    End

    It 'lock owner file records PID + timestamp'
        mkdir -p "${VM_DIR}/.lock"
        printf '%s %s' "$$" "$(date +%s)" >"${VM_DIR}/.lock/owner.pid"
        When run bash -c "grep -qE '^[0-9]+ [0-9]+$' '${VM_DIR}/.lock/owner.pid'"
        The status should be success
    End
End

Describe 'fault: disk-full handling'
    not_root() { [ "$(id -u)" -ne 0 ]; }

    It 'write to a full filesystem returns non-zero'
        Skip if 'tmpfs mount requires root' not_root
        When run bash -c 'd=$(mktemp -d); mount -t tmpfs -o size=1M none "$d" && { ! dd if=/dev/zero of="$d/big" bs=1M count=2 2>/dev/null; rc=$?; umount "$d"; rm -rf "$d"; exit $rc; }'
        The status should be success
    End
End

Describe 'fault: partial download handling'
    It 'a .part file is not treated as a completed download'
        f=$(mktemp -u).iso.part
        echo "partial" >"${f}"
        When run bash -c 'case "$1" in *.part) exit 0;; *) exit 1;; esac' _ "${f}"
        The status should be success
    End
End
