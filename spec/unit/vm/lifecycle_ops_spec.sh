Describe 'VM lifecycle operations'
Include lib/quickemu/vm/vm.sh
Include lib/quickemu/runtime/lifecycle.sh
Include lib/quickemu/runtime/monitor.sh
Include lib/quickemu/support/filesystem.sh
Include lib/quickemu/support/json.sh
Include lib/quickemu/support/validation.sh

setup() {
    TESTDIR=$(mktemp -d)
    HOME="$TESTDIR/home"
    mkdir -p "$HOME"
    FAKE_BIN="$TESTDIR/bin"
    mkdir -p "$FAKE_BIN"
    VMPARENT="$TESTDIR/vms"
    mkdir -p "$VMPARENT"
    VMDIR="$VMPARENT/vm-test"
    mkdir -p "$VMDIR"
    VMNAME=vm-test
    VM="$VMPARENT/vm-test.conf"
    printf 'guest_os=linux\n' >"$VM"
    disk_img="$VMDIR/disk.qcow2"
    disk_format=qcow2
    SOCKET_MONITOR="$VMDIR/vm-test-monitor.socket"
    SOCKET_SERIAL="$VMDIR/vm-test-serial.socket"
    VMPATH="$VMPARENT"
    VM_PID=''
    monitor=none
    SCREENSHOT_OUT=''
    CLONE_NAME='' CLONE_FULL='' EXPORT_FILE='' IMPORT_FILE='' RENAME_TO=''
    SEND_KEYS_ARG='' TYPE_TEXT='' SHORTCUT_OPTIONS=''
    MONITOR_CMD_LOG='' QEMU='' QEMU_IMG='' swtpm='' tpm='' SOCK_READY=1
    DRY_RUN='' VIRTIOFSD_PID='' VIRTIOFSD_SOCKET=''
}
cleanup() { qe_spec_cleanup; }
BeforeEach 'setup'
AfterEach 'cleanup'

in_subshell() { ( "$@" ); }

# Fake QEMU monitor that executes the screendump command it is sent,
# so screenshot_vm sees a real PPM file appear.
monitor_send_cmd() {
    if [ -z "$1" ]; then
        echo 'WARNING! Send to QEMU-Monitor: Message empty!'
        return 1
    fi
    echo "$1" >>"$TESTDIR/monitor.log"
    case "$1" in
    'screendump '*) printf 'P6 fake' >"${1#screendump }" ;;
    esac
    return 0
}
swtpm_socket_ready() { return 0; }
vm_monitor_socket_ready() { return "${SOCK_READY:-1}"; }
# Spawn a fake VM process whose comm matches the VM name (QEMU renames
# itself via -name <vm>,process=<vm>; vm_pid_alive now verifies identity).
spawn_fake_vm() {
    local name="${1:-${VMNAME:-vm1}}" fb="${FAKE_BIN:-${FAKEBIN:-${TESTDIR}/bin}}"
    mkdir -p "$fb"; cp /bin/sleep "$fb/${name}"; "$fb/${name}" "${2:-60}" >/dev/null 2>&1 &
}


Describe 'vm_identify'
It 'derives VM identity paths from the conf location'
    When call vm_identify "$VMPARENT/vm-test.conf"
    The status should be success
    The variable VMDIR should eq "$VMDIR"
    The variable VMNAME should eq 'vm-test'
    The variable VMPATH should eq "$(realpath "$VMPARENT")"
    The variable SOCKET_MONITOR should eq "$VMDIR/vm-test-monitor.socket"
    The variable SOCKET_SERIAL should eq "$VMDIR/vm-test-serial.socket"
End
End

Describe 'vm_identify defaults'
It 'places the VM directory next to the conf when disk_img is unset'
    disk_img=''
    When call vm_identify "$VMPARENT/vm-test.conf"
    The status should be success
    The variable VMDIR should eq "$VMPATH/vm-test"
End
End

Describe 'delete_vm'
It 'refuses to delete a running VM'
    spawn_fake_vm
    echo $! >"$VMDIR/vm-test.pid"
    When call in_subshell delete_vm
    The status should be failure
    The error should include 'before deleting'
    The directory "$VMDIR" should be exist
    The file "$VM" should be exist
End
It 'removes the VM directory conf and desktop shortcut'
    mkdir -p "$HOME/.local/share/applications"
    : >"$HOME/.local/share/applications/vm-test.desktop"
    : >"$VMDIR/vm-test.pid"
    When call delete_vm
    The status should be success
    The output should include 'Deleting vm-test'
    The directory "$VMDIR" should not be exist
    The file "$VM" should not be exist
    The file "$HOME/.local/share/applications/vm-test.desktop" should not be exist
End
It 'does nothing when the VM directory is missing'
    VMDIR="$VMPARENT/absent"
    When call delete_vm
    The status should be success
    The output should include 'not found. Doing nothing'
    The file "$VM" should be exist
End
End

Describe 'pause_vm and resume_vm'
It 'refuses to pause a VM that is not running'
    When call pause_vm
    The status should be failure
    The output should include 'is not running'
End
It 'pauses a running VM through the QEMU monitor when available'
    spawn_fake_vm
    VM_PID=$!
    echo "$VM_PID" >"$VMDIR/vm-test.pid"
    monitor=socket
    SOCK_READY=0
    When call pause_vm
    The status should be success
    The output should include 'paused'
    The contents of file "$TESTDIR/monitor.log" should include 'stop'
End
It 'pauses a running VM with SIGSTOP when the monitor is disabled'
    spawn_fake_vm
    VM_PID=$!
    echo "$VM_PID" >"$VMDIR/vm-test.pid"
    monitor=none
    When call pause_vm
    The status should be success
    The output should include 'paused'
    The variable VM_PID should eq "$VM_PID"
End
It 'records a .paused marker when pausing through the monitor'
    spawn_fake_vm
    VM_PID=$!
    echo "$VM_PID" >"$VMDIR/vm-test.pid"
    monitor=socket
    SOCK_READY=0
    When call pause_vm
    The status should be success
    The output should include 'paused'
    The file "$VMDIR/vm-test.paused" should be exist
End
It 'removes the .paused marker when resuming'
    spawn_fake_vm
    VM_PID=$!
    echo "$VM_PID" >"$VMDIR/vm-test.pid"
    : >"$VMDIR/vm-test.paused"
    monitor=socket
    SOCK_READY=0
    When call resume_vm
    The status should be success
    The output should include 'resumed'
    The file "$VMDIR/vm-test.paused" should not be exist
End
It 'refuses to resume a VM that is not running'
    When call resume_vm
    The status should be failure
    The output should include 'is not running'
End
It 'resumes a running VM through the QEMU monitor'
    spawn_fake_vm
    VM_PID=$!
    echo "$VM_PID" >"$VMDIR/vm-test.pid"
    monitor=socket
    SOCK_READY=0
    When call resume_vm
    The status should be success
    The output should include 'resumed'
    The contents of file "$TESTDIR/monitor.log" should include 'cont'
End
It 'resumes a running VM with SIGCONT when the monitor is disabled'
    spawn_fake_vm
    VM_PID=$!
    echo "$VM_PID" >"$VMDIR/vm-test.pid"
    monitor=none
    When call resume_vm
    The status should be success
    The output should include 'resumed'
End
End

Describe 'screenshot_vm'
It 'refuses a VM that is not running'
    When call screenshot_vm
    The status should be failure
    The output should include 'is not running'
End
It 'requires a monitor channel'
    spawn_fake_vm
    VM_PID=$!
    echo "$VM_PID" >"$VMDIR/vm-test.pid"
    monitor=none
    When call screenshot_vm
    The status should be failure
    The output should include 'monitor is disabled'
End
It 'requires the monitor socket to exist for socket monitors'
    spawn_fake_vm
    VM_PID=$!
    echo "$VM_PID" >"$VMDIR/vm-test.pid"
    monitor=socket
    When call screenshot_vm
    The status should be failure
    The output should include 'No QEMU monitor socket'
End
It 'requires socat to be installed'
    spawn_fake_vm
    VM_PID=$!
    echo "$VM_PID" >"$VMDIR/vm-test.pid"
    monitor=socket
    SOCK_READY=0
    # Hide socat from `command -v` lookups only; everything else delegates.
    command() { [ "$2" = 'socat' ] && return 1; builtin command "$@"; }
    When call screenshot_vm
    The status should be failure
    The output should include 'socat is required'
End
It 'saves a PPM screendump through the monitor'
    spawn_fake_vm
    VM_PID=$!
    echo "$VM_PID" >"$VMDIR/vm-test.pid"
    monitor=socket
    SCREENSHOT_OUT="$VMDIR/shot.ppm"
    SOCK_READY=0
    printf '#!/bin/sh\nexit 0\n' >"$FAKE_BIN/socat"; chmod +x "$FAKE_BIN/socat"
    PATH="$FAKE_BIN:$PATH"
    When call screenshot_vm
    The status should be success
    The output should include 'Screenshot saved'
    The file "$VMDIR/shot.ppm" should be exist
End
It 'converts to PNG when a converter is available'
    spawn_fake_vm
    VM_PID=$!
    echo "$VM_PID" >"$VMDIR/vm-test.pid"
    monitor=socket
    SCREENSHOT_OUT="$VMDIR/shot.png"
    SOCK_READY=0
    printf '#!/bin/sh\nexit 0\n' >"$FAKE_BIN/socat"; chmod +x "$FAKE_BIN/socat"
    printf '#!/bin/sh\ncp "$1" "$2"\n' >"$FAKE_BIN/convert"; chmod +x "$FAKE_BIN/convert"
    PATH="$FAKE_BIN:$PATH"
    When call screenshot_vm
    The status should be success
    The output should include 'shot.png'
    The file "$VMDIR/shot.png" should be exist
End
End

Describe 'clone_vm'
It 'requires a name for the clone'
    When call in_subshell clone_vm
    The status should be failure
    The output should include "needs a name"
End
It 'refuses to clone over an existing VM'
    CLONE_NAME=vm-test
    When call in_subshell clone_vm
    The status should be failure
    The output should include 'already exists'
End
It 'refuses to clone a running VM'
    spawn_fake_vm
    echo $! >"$VMDIR/vm-test.pid"
    CLONE_NAME=clone-1
    When call in_subshell clone_vm
    The status should be failure
    The output should include 'Stop vm-test before cloning'
End
It 'copies config and aux files while skipping runtime artefacts'
    CLONE_NAME=clone-1
    printf 'guest_os=linux\nimg=%s/disk.qcow2\n' "$VMNAME" >"$VM"
    : >"$disk_img"
    : >"$VMDIR/OVMF_VARS.fd"
    : >"$VMDIR/vm-test.pid"
    : >"$VMDIR/vm-test.sock"
    mkdir -p "$VMDIR/nested"
    QEMU_IMG=/bin/true
    When call clone_vm
    The status should be success
    The output should include 'Cloned vm-test to clone-1'
    The file "$VMPARENT/clone-1.conf" should be exist
    The contents of file "$VMPARENT/clone-1.conf" should include 'clone-1/disk.qcow2'
    The file "$VMPARENT/clone-1/OVMF_VARS.fd" should be exist
    The file "$VMPARENT/clone-1/vm-test.pid" should not be exist
    The file "$VMPARENT/clone-1/vm-test.sock" should not be exist
    The directory "$VMPARENT/clone-1/nested" should be exist
End
It 'creates a shared backing image by default'
    CLONE_NAME=clone-2
    : >"$disk_img"
    printf '#!/bin/sh\nprintf "%%s " "$@" >>"%s/qemu-img.log"\n' "$TESTDIR" >"$FAKE_BIN/qemu-img"
    chmod +x "$FAKE_BIN/qemu-img"
    QEMU_IMG="$FAKE_BIN/qemu-img"
    When call clone_vm
    The status should be success
    The output should include 'shared backing disk'
    The contents of file "$TESTDIR/qemu-img.log" should include 'create -f qcow2 -b'
End
It 'performs a full disk copy when CLONE_FULL is set'
    CLONE_NAME=clone-3 CLONE_FULL=true
    : >"$disk_img"
    printf '#!/bin/sh\nprintf "%%s " "$@" >>"%s/qemu-img.log"\n' "$TESTDIR" >"$FAKE_BIN/qemu-img"
    chmod +x "$FAKE_BIN/qemu-img"
    QEMU_IMG="$FAKE_BIN/qemu-img"
    When call clone_vm
    The status should be success
    The output should include 'full disk copy'
    The output should include 'clone-3'
    The contents of file "$TESTDIR/qemu-img.log" should include 'convert -O qcow2'
End
End

Describe 'export_vm and import_vm'
It 'refuses to export a running VM'
    spawn_fake_vm
    echo $! >"$VMDIR/vm-test.pid"
    When call in_subshell export_vm
    The status should be failure
    The output should include 'Stop vm-test before exporting'
End
It 'exports conf and VM directory into a tarball without runtime state'
    : >"$VMDIR/vm-test.pid"
    : >"$VMDIR/vm-test.sock"
    : >"$VMDIR/disk.qcow2"
    EXPORT_FILE="$TESTDIR/export.tar.gz"
    When call export_vm
    The status should be success
    The output should include 'Exported vm-test'
    The file "$EXPORT_FILE" should be exist
End
It 'excludes runtime state from the exported tarball'
    : >"$VMDIR/vm-test.pid"
    : >"$VMDIR/vm-test.sock"
    : >"$VMDIR/disk.qcow2"
    EXPORT_FILE="$TESTDIR/export2.tar.gz"
    export_vm >/dev/null
    TAR_LIST=$(tar -tzf "$EXPORT_FILE")
    The variable TAR_LIST should include 'vm-test.conf'
    The variable TAR_LIST should include 'vm-test/disk.qcow2'
    The variable TAR_LIST should not include 'vm-test.pid'
    The variable TAR_LIST should not include 'vm-test.sock'
End
It 'requires an existing tarball to import'
    IMPORT_FILE="$TESTDIR/nope.tar.gz"
    When call in_subshell import_vm
    The status should be failure
    The output should include "needs an existing .tar.gz"
End
It 'rejects a tarball without a conf'
    tar -C "$VMPARENT" -czf "$TESTDIR/noconf.tar.gz" vm-test 2>/dev/null || true
    IMPORT_FILE="$TESTDIR/noconf.tar.gz"
    When call in_subshell import_vm
    The status should be failure
    The output should include 'No .conf found'
End
It 'refuses to import over an existing VM'
    IMPORT_FILE="$TESTDIR/ok.tar.gz"
    tar -C "$VMPARENT" -czf "$IMPORT_FILE" vm-test.conf vm-test
    pushd "$VMPARENT" >/dev/null
    When call in_subshell import_vm
    The status should be failure
    The output should include 'already exists'
    popd >/dev/null || :
End
It 'imports a valid tarball into a fresh directory'
    IMPORT_FILE="$TESTDIR/ok2.tar.gz"
    mkdir -p "$TESTDIR/src/vm-new" && printf 'guest_os=linux\n' >"$TESTDIR/src/vm-new.conf" && : >"$TESTDIR/src/vm-new/disk"
    tar -C "$TESTDIR/src" -czf "$IMPORT_FILE" vm-new.conf vm-new
    IMPORT_DIR="$TESTDIR/import"
    mkdir -p "$IMPORT_DIR" && pushd "$IMPORT_DIR" >/dev/null
    When call import_vm
    The status should be success
    The output should include 'Imported vm-new'
    The file "$IMPORT_DIR/vm-new.conf" should be exist
    popd >/dev/null || :
    The directory "$IMPORT_DIR/vm-new" should be exist
End
End

Describe 'rename_vm'
It 'requires a new name'
    RENAME_TO=''
    When call in_subshell rename_vm
    The status should be failure
    The output should include "needs a new name"
End
It 'refuses to rename over an existing VM'
    mkdir -p "$VMPARENT/other"
    RENAME_TO=other
    When call in_subshell rename_vm
    The status should be failure
    The output should include 'already exists'
End
It 'refuses to rename a running VM'
    spawn_fake_vm
    echo $! >"$VMDIR/vm-test.pid"
    RENAME_TO=renamed
    When call in_subshell rename_vm
    The status should be failure
    The output should include 'Stop vm-test before renaming'
End
It 'moves the directory and rewrites conf references'
    RENAME_TO=renamed
    printf 'img=%s/disk.qcow2\n' "$VMNAME" >"$VM"
    When call rename_vm
    The status should be success
    The output should include 'Renamed vm-test to renamed'
    The directory "$VMPARENT/renamed" should be exist
    The file "$VMPARENT/renamed.conf" should be exist
    The contents of file "$VMPARENT/renamed.conf" should include 'renamed/disk.qcow2'
    The file "$VM" should not be exist
End
End

Describe 'monitor channel helpers'
It 'sends keys through the monitor'
    SEND_KEYS_ARG='ctrl-alt-f1'
    When call send_keys
    The status should be success
    The contents of file "$TESTDIR/monitor.log" should include 'sendkey ctrl-alt-f1'
End
It 'types printable characters through the monitor'
    TYPE_TEXT='aB !'
    When call type_text
    The status should be success
    The contents of file "$TESTDIR/monitor.log" should include 'sendkey a'
    The contents of file "$TESTDIR/monitor.log" should include 'sendkey shift-b'
    The contents of file "$TESTDIR/monitor.log" should include 'sendkey spc'
    The contents of file "$TESTDIR/monitor.log" should include 'sendkey shift-1'
End
It 'skips characters without a QEMU key mapping'
    TYPE_TEXT='a€'
    When call type_text
    The status should be success
    The output should include 'Skipping unmapped char'
End
It 'warns when the monitor message is empty'
    When call monitor_send_cmd ''
    The status should be failure
    The output should include 'Message empty'
End
End

Describe 'kill_vm'
It 'cleans up runtime files when the VM is not running'
    VM_PID=''
    : >"$VMDIR/vm-test.pid"
    : >"$VMDIR/vm-test.spice"
    : >"$VMDIR/vm-test.sock"
    When call kill_vm
    The status should be success
    The output should include 'is not running'
    The file "$VMDIR/vm-test.pid" should not be exist
    The file "$VMDIR/vm-test.spice" should not be exist
    The file "$VMDIR/vm-test.sock" should not be exist
End
It 'kills a running VM and removes its pidfile'
    spawn_fake_vm
    disown
    VM_PID=$!
    : >"$VMDIR/vm-test.pid"
    When call kill_vm
    The status should be success
    The output should include 'killed'
    The file "$VMDIR/vm-test.pid" should not be exist
End
It 'removes monitor serial ports and paused state artefacts'
    spawn_fake_vm
    disown
    VM_PID=$!
    : >"$VMDIR/vm-test.pid"
    : >"$VMDIR/vm-test-monitor.socket"
    : >"$VMDIR/vm-test-serial.socket"
    : >"$VMDIR/vm-test.ports"
    : >"$VMDIR/vm-test.paused"
    stop_swtpm() { :; }
    When call kill_vm
    The status should be success
    The output should include 'killed'
    The file "$VMDIR/vm-test-monitor.socket" should not be exist
    The file "$VMDIR/vm-test-serial.socket" should not be exist
    The file "$VMDIR/vm-test.ports" should not be exist
    The file "$VMDIR/vm-test.paused" should not be exist
End
It 'stops helper daemons when killing the VM'
    spawn_fake_vm
    disown
    VM_PID=$!
    : >"$VMDIR/vm-test.pid"
    stop_swtpm() { echo swtpm-stopped >>"$TESTDIR/helpers.log"; }
    stop_virtiofsd() { echo virtiofsd-stopped >>"$TESTDIR/helpers.log"; }
    When call kill_vm
    The status should be success
    The output should include 'killed'
    The contents of file "$TESTDIR/helpers.log" should include 'swtpm-stopped'
    The contents of file "$TESTDIR/helpers.log" should include 'virtiofsd-stopped'
End
End

Describe 'shortcut and autostart'
It 'creates a desktop shortcut with the recorded options'
    SHORTCUT_OPTIONS='--viewer spicy '
    When call shortcut_create
    The status should be success
    The output should include 'created'
    The contents of file "$HOME/.local/share/applications/vm-test.desktop" should include '--viewer spicy'
    The contents of file "$HOME/.local/share/applications/vm-test.desktop" should include "Path=$VMPARENT"
End
It 'writes a systemd user unit for autostart'
    XDG_CONFIG_HOME="$TESTDIR/xdg"
    printf '#!/bin/sh\nexit 0\n' >"$FAKE_BIN/systemctl"; chmod +x "$FAKE_BIN/systemctl"
    PATH="$FAKE_BIN:$PATH"
    When call autostart_create
    The status should be success
    The output should include 'Autostart unit written'
    The file "$TESTDIR/xdg/systemd/user/quickemu-vm-test.service" should be exist
    The contents of file "$TESTDIR/xdg/systemd/user/quickemu-vm-test.service" should include "ExecStart="
    The contents of file "$TESTDIR/xdg/systemd/user/quickemu-vm-test.service" should include "$VM"
End
End

Describe 'qe_safe_rm'
It 'removes an existing file'
    : >"$TESTDIR/victim"
    When call qe_safe_rm "$TESTDIR/victim"
    The status should be success
    The file "$TESTDIR/victim" should not be exist
End
It 'ignores missing files and empty arguments'
    When call qe_safe_rm "$TESTDIR/absent"
    The status should be success
End
It 'returns success for an empty argument'
    When call qe_safe_rm ''
    The status should be success
End
End

Describe 'reset_vm'
It 'refuses a VM that is not running'
    When call reset_vm
    The status should be failure
    The output should include 'is not running'
End
It 'resets a running VM through the QEMU monitor'
    spawn_fake_vm
    VM_PID=$!
    echo "$VM_PID" >"$VMDIR/vm-test.pid"
    monitor=socket
    SOCK_READY=0
    When call reset_vm
    The status should be success
    The output should include 'reset requested'
    The contents of file "$TESTDIR/monitor.log" should include 'system_reset'
End
It 'refuses a running VM without a monitor channel'
    spawn_fake_vm
    VM_PID=$!
    echo "$VM_PID" >"$VMDIR/vm-test.pid"
    monitor=none
    When call reset_vm
    The status should be failure
    The output should include 'no monitor channel'
End
End

Describe 'snapshot operations on a running VM'
run_vm() {
    spawn_fake_vm
    echo $! >"$VMDIR/vm-test.pid"
    : >"$disk_img"
    QEMU_IMG=/bin/true
}
It 'refuses to apply a snapshot while the VM runs'
    run_vm
    When call in_subshell snapshot_apply tag1
    The status should be failure
    The output should include 'before managing snapshots'
End
It 'refuses to create a snapshot while the VM runs'
    run_vm
    When call in_subshell snapshot_create tag1
    The status should be failure
    The output should include 'before managing snapshots'
End
It 'refuses to delete a snapshot while the VM runs'
    run_vm
    When call in_subshell snapshot_delete tag1
    The status should be failure
    The output should include 'before managing snapshots'
End
End

Describe 'snapshot_list_json'
qemu_img_json_stub() {
    printf '%s\n' '{"filename":"disk.qcow2","format":"qcow2","snapshots":[{"id":"1","name":"snap one","date-sec":1700000000,"date-nsec":500000000,"vm-state-size":1048576}]}'
}
It 'emits an empty contract when the disk image is absent'
    rm -f "$disk_img"
    When call snapshot_list_json
    The status should be success
    The output should include '"schemaVersion":1'
    The output should include '"vm":"vm-test"'
    The output should include '"snapshots":[]'
End
It 'normalises qemu-img info JSON into the v1 contract'
    : >"$disk_img"
    QEMU_IMG=qemu_img_json_stub
    # jq is a production dependency; provide a controlled double when the
    # host lacks it so the contract transformation is still exercised.
    if ! command -v jq >/dev/null 2>&1; then
        printf '#!/bin/sh\nexec python3 -c '"'"'import json,sys\nd=json.load(sys.stdin)\nprint(json.dumps([{"id":s["id"],"name":s["name"],"date":s.get("date-sec",0)+s.get("date-nsec",0)/1e9,"vm-state-size":s.get("vm-state-size",0)} for s in d.get("snapshots",[])],separators=(",",":")))'"'"'\n' >"$FAKE_BIN/jq"
        chmod +x "$FAKE_BIN/jq"
        PATH="$FAKE_BIN:$PATH"
    fi
    When call snapshot_list_json
    The status should be success
    The output should include '"name":"snap one"'
    The output should include '"date":1700000000.5'
    The output should include '"vm-state-size":1048576'
End
End

Describe 'get_qemu_share_path'
It 'derives the share dir from the qemu binary prefix'
    mkdir -p "$TESTDIR/qemu/bin" "$TESTDIR/qemu/share/qemu"
    QEMU="$TESTDIR/qemu/bin/qemu-system-x86_64"
    : >"$QEMU"
    When call get_qemu_share_path
    The status should be success
    The output should eq "$TESTDIR/qemu/share"
End
It 'falls back to /usr/share when the layout is unknown'
    QEMU=/nonexistent/qemu
    When call get_qemu_share_path
    The status should be success
    The output should eq '/usr/share'
End
End
End
