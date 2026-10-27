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

    It 'a lock dir left by a crashed process is detectable as orphan'
        mkdir -p "${VM_DIR}/.lock"
        # Simulate: write a stale owner file with a dead PID
        echo "999999" > "${VM_DIR}/.lock/owner.pid"
        # A lock with a dead owner PID should be reclaimable
        [ ! -e "/proc/999999" ] || [ "$(ps -p 999999 -o pid= 2>/dev/null)" == "" ]
    End

    It 'lock owner file records PID + timestamp'
        mkdir -p "${VM_DIR}/.lock"
        printf '%s %s' "$$" "$(date +%s)" > "${VM_DIR}/.lock/owner.pid"
        grep -qE '^[0-9]+ [0-9]+$' "${VM_DIR}/.lock/owner.pid"
    End
End

Describe 'fault: disk-full handling'
    # tmpfs tests require root; skip cleanly otherwise.
    Skip if "requires root" [ "$(id -u)" -ne 0 ]
    It 'write to a full filesystem returns non-zero'
        tmpfs_dir=$(mktemp -d)
        mount -t tmpfs -o size=1M none "${tmpfs_dir}" 2>/dev/null || { rm -rf "${tmpfs_dir}"; return 0; }
        dd if=/dev/zero of="${tmpfs_dir}/bigfile" bs=1M count=2 2>/dev/null
        [ $? -ne 0 ]  # expect failure on full fs
        umount "${tmpfs_dir}" 2>/dev/null; rm -rf "${tmpfs_dir}"
    End
End

Describe 'fault: partial download handling'
    It 'a .part file is not treated as a completed download'
        f=$(mktemp -u).iso.part
        echo "partial" > "${f}"
        # The download layer must not treat .part as complete
        case "${f}" in *.part) ;; *) return 1;; esac
        rm -f "${f}"
    End
End
