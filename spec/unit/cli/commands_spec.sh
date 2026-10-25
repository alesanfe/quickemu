Describe 'CLI commands and parameter checks'
Include lib/quickemu/cli/commands.sh
Include lib/quickemu/support/validation.sh
Include lib/quickemu/support/json.sh
Include lib/quickemu/runtime/lifecycle.sh

setup() {
    TESTDIR=$(mktemp -d)
    cd "$TESTDIR"
    FAKE_BIN="$TESTDIR/bin"
    mkdir -p "$FAKE_BIN"
    BRAILLE='' display='sdl' gl='' usb_controller='' OS_KERNEL=Linux
    QEMU=/bin/true DISPLAY='' WAYLAND_DISPLAY=''
    ssh_port='' spice_port='' monitor_telnet_port='' serial_telnet_port=''
    sound_card='' sound_duplex='hda-duplex' tpm='' viewer='none'
    PUBLIC='' guest_os=linux USER=tester STAT=stat
    VM_PID='' VMNAME=vm-test OUTPUT_FORMAT='' QE_SCHEMA_VERSION=1
    VERSION='4.9.9' VMDIR="$TESTDIR" DRY_RUN='' VIRTIOFSD=''
    VIRTIOFSD_PID='' VIRTIOFSD_SOCKET='' HOME="$TESTDIR"
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

in_subshell() { ( "$@" ); }

Describe 'display_param_check'
It 'forces SDL and xhci for braille output'
    BRAILLE=on display=gtk gl=off
    When call display_param_check
    The status should be success
    The variable display should eq 'sdl'
    The variable usb_controller should eq 'xhci'
End
It 'falls back to SDL when GTK is not in the QEMU build'
    display=gtk DISPLAY=':0' gl=off
    printf '#!/bin/sh\necho sdl\n' >"$FAKE_BIN/qemu"
    chmod +x "$FAKE_BIN/qemu"
    QEMU="$FAKE_BIN/qemu"
    When call display_param_check
    The status should be success
    The output should include 'falling back to SDL'
    The variable display should eq 'sdl'
End
It 'falls back to SDL without a display server even when GTK is supported'
    display=gtk DISPLAY='' WAYLAND_DISPLAY='' gl=off
    printf '#!/bin/sh\necho gtk\n' >"$FAKE_BIN/qemu"
    chmod +x "$FAKE_BIN/qemu"
    QEMU="$FAKE_BIN/qemu"
    When call display_param_check
    The status should be success
    The output should include 'falling back to SDL'
    The variable display should eq 'sdl'
End
It 'keeps GTK when QEMU supports it and a display server exists'
    display=gtk DISPLAY=':0'
    printf '#!/bin/sh\necho gtk\n' >"$FAKE_BIN/qemu"
    chmod +x "$FAKE_BIN/qemu"
    QEMU="$FAKE_BIN/qemu"
    gl=off
    When call display_param_check
    The status should be success
    The variable display should eq 'gtk'
End
Parameters
    sdl
    spice
    spice-app
    none
    vnc
End
It 'accepts the display option on Linux'
    display="$1" gl=off
    When call display_param_check
    The status should be success
End
It 'rejects unknown displays on Linux'
    display=wayland
    When call in_subshell display_param_check
    The status should be failure
    The output should include 'is not recognised'
End
It 'rejects non-cocoa displays on macOS'
    OS_KERNEL=Darwin display=sdl
    When call in_subshell display_param_check
    The status should be failure
    The output should include "only 'cocoa' and 'none'"
End
It 'accepts cocoa on macOS'
    OS_KERNEL=Darwin display=cocoa gl=off
    When call display_param_check
    The status should be success
End
It 'defaults gl off for old GLSL versions'
    display=sdl gl=''
    printf '#!/bin/sh\necho "OpenGL ES GLSL ES 1.10"\n' >"$FAKE_BIN/glxinfo"
    chmod +x "$FAKE_BIN/glxinfo"
    PATH="$FAKE_BIN:$PATH"
    When call display_param_check
    The status should be success
    The variable gl should eq 'off'
End
It 'defaults gl on for modern GLSL versions'
    display=sdl gl=''
    printf '#!/bin/sh\necho "OpenGL ES GLSL ES 3.20"\n' >"$FAKE_BIN/glxinfo"
    chmod +x "$FAKE_BIN/glxinfo"
    PATH="$FAKE_BIN:$PATH"
    When call display_param_check
    The status should be success
    The variable gl should eq 'on'
End
It 'defaults gl on when glxinfo is absent'
    display=sdl gl=''
    command() { [ "$2" = 'glxinfo' ] && return 1; builtin command "$@"; }
    When call display_param_check
    The status should be success
    The variable gl should eq 'on'
End
It 'exports SDL_MOUSE_FOCUS_CLICKTHROUGH for SDL displays'
    display=sdl gl=off
    When call display_param_check
    The status should be success
    The variable SDL_MOUSE_FOCUS_CLICKTHROUGH should eq 1
End
End

Describe 'ports_param_check'
Parameters
    ssh_port
    spice_port
    monitor_telnet_port
    serial_telnet_port
End
It 'rejects a non-numeric port'
    eval "$1=abc"
    When call in_subshell ports_param_check
    The status should be failure
    The output should include 'must be a number'
End
It 'accepts numeric ports'
    eval "$1=5901"
    When call ports_param_check
    The status should be success
End
End

Describe 'resolution_param_check'
It 'accepts an unset resolution'
    width='' height=''
    When call resolution_param_check
    The status should be success
End
It 'accepts a paired numeric resolution'
    width=1920 height=1080
    When call resolution_param_check
    The status should be success
End
It 'rejects width without height'
    width=1920 height=''
    When call in_subshell resolution_param_check
    The status should be failure
    The output should include "--height"
End
It 'rejects height without width'
    width='' height=1080
    When call in_subshell resolution_param_check
    The status should be failure
    The output should include "--width"
End
It 'rejects a non-numeric resolution'
    width=abc height=1080
    When call in_subshell resolution_param_check
    The status should be failure
    The output should include 'must be a number'
End
End

Describe 'access_param_check'
Parameters
    local
    remote
    192.168.1.10
    spice.example.lan
End
It 'accepts the supported access scope or a host'
    ACCESS="$1"
    When call access_param_check
    The status should be success
End
It 'rejects an empty-token access value'
    ACCESS='not a host'
    When call in_subshell access_param_check
    The status should be failure
    The output should include 'is not recognised'
End
It 'rejects a bad ssh-access value'
    SSH_ACCESS='bad value'
    When call in_subshell access_param_check
    The status should be failure
    The output should include 'is not recognised'
End
It 'accepts an empty access value'
    ACCESS='' SSH_ACCESS=''
    When call access_param_check
    The status should be success
End
End

Describe 'sound_card_param_check'
Parameters
    ac97
    es1370
    ich9-intel-hda
    intel-hda
    sb16
    usb-audio
    virtio-sound-pci
    none
End
It 'accepts the supported sound card'
    sound_card="$1"
    When call sound_card_param_check
    The status should be success
End
It 'rejects an unknown sound card'
    sound_card=crystal
    When call in_subshell sound_card_param_check
    The status should be failure
    The output should include 'is not recognised'
End
It 'forces xhci for usb audio'
    sound_card=usb-audio
    When call sound_card_param_check
    The status should be success
    The variable usb_controller should eq 'xhci'
End
End

Describe 'sound_duplex_param_check'
Parameters
    hda-duplex
    hda-micro
    hda-output
End
It 'accepts the supported duplex mode'
    sound_duplex="$1"
    When call sound_card_param_check
    The status should be success
End
It 'rejects an unknown duplex mode'
    sound_duplex=stereo
    When call in_subshell sound_card_param_check
    The status should be failure
    The output should include 'duplex'
End
End

Describe 'tpm_param_check'
It 'does nothing when TPM is off'
    tpm=off
    When call tpm_param_check
    The status should be success
End
It 'fails when swtpm is not installed'
    tpm=on
    command() { [ "$2" = 'swtpm' ] && return 1; builtin command "$@"; }
    When call in_subshell tpm_param_check
    The status should be failure
    The output should include 'swtpm was not found'
End
It 'passes when swtpm is available'
    tpm=on
    printf '#!/bin/sh\nexit 0\n' >"$FAKE_BIN/swtpm"; chmod +x "$FAKE_BIN/swtpm"
    PATH="$FAKE_BIN:$PATH"
    When call tpm_param_check
    The status should be success
End
End

Describe 'viewer_param_check'
It 'accepts none'
    viewer=none
    When call viewer_param_check
    The status should be success
End
It 'rejects an unknown viewer'
    viewer=virt-manager
    When call in_subshell viewer_param_check
    The status should be failure
    The output should include 'is not recognised'
End
It 'fails when spicy is requested but missing'
    viewer=spicy
    command() { [ "$2" = 'spicy' ] && return 1; builtin command "$@"; }
    When call in_subshell viewer_param_check
    The status should be failure
    The output should include "'spicy' is not installed"
End
It 'fails when remote-viewer is requested but missing'
    viewer=remote-viewer
    command() { [ "$2" = 'remote-viewer' ] && return 1; builtin command "$@"; }
    When call in_subshell viewer_param_check
    The status should be failure
    The output should include "'remote-viewer' is not installed"
End
It 'passes when the requested viewer exists'
    viewer=spicy
    printf '#!/bin/sh\nexit 0\n' >"$FAKE_BIN/spicy"; chmod +x "$FAKE_BIN/spicy"
    PATH="$FAKE_BIN:$PATH"
    When call viewer_param_check
    The status should be success
End
It 'skips validation entirely on macOS'
    OS_KERNEL=Darwin viewer=whatever
    When call viewer_param_check
    The status should be success
End
End

Describe 'fileshare_param_check'
It 'disables sharing for PUBLIC=none'
    PUBLIC=none
    When call fileshare_param_check
    The status should be success
    The variable PUBLIC should eq ''
End
It 'uses xdg-user-dir when no public dir was configured'
    PUBLIC=''
    printf '#!/bin/sh\necho "%s/Public"\n' "$TESTDIR" >"$FAKE_BIN/xdg-user-dir"
    chmod +x "$FAKE_BIN/xdg-user-dir"
    mkdir -p "$TESTDIR/Public"
    PATH="$FAKE_BIN:$PATH"
    When call fileshare_param_check
    The status should be success
    The variable PUBLIC should eq "$TESTDIR/Public"
End
It 'falls back to ~/Public without xdg-user-dir'
    PUBLIC=''
    mkdir -p "$HOME/Public"
    command() { [ "$2" = 'xdg-user-dir' ] && return 1; builtin command "$@"; }
    When call fileshare_param_check
    The status should be success
    The variable PUBLIC should eq "$HOME/Public"
End
It 'warns and disables sharing when the directory is missing'
    PUBLIC="$TESTDIR/missing"
    When call fileshare_param_check
    The status should be success
    The output should include "doesn't exist"
    The variable PUBLIC should eq ''
End
It 'derives a mount tag and permissions for a valid share'
    PUBLIC="$TESTDIR/Public"; mkdir -p "$PUBLIC"
    guest_os=windows
    When call fileshare_param_check
    The status should be success
    The variable PUBLIC_TAG should include 'Public-'
    The variable PUBLIC_PERMS should be present
End
It 'truncates the mount tag to 31 bytes'
    PUBLIC="$TESTDIR/Public"; mkdir -p "$PUBLIC"
    USER=averyveryverylongusernameforlinux
    guest_os=windows
    When call fileshare_param_check
    The status should be success
    The variable PUBLIC_TAG should eq 'Public-averyveryverylongusernam'
End
It 'enables virtiofs for Linux guests when virtiofsd exists'
    PUBLIC="$TESTDIR/Public"; mkdir -p "$PUBLIC"
    guest_os=linux
    printf '#!/bin/sh\nexit 0\n' >"$FAKE_BIN/virtiofsd"; chmod +x "$FAKE_BIN/virtiofsd"
    PATH="$FAKE_BIN:$PATH"
    When call fileshare_param_check
    The status should be success
    The variable VIRTIOFSD should include 'virtiofsd'
End
It 'leaves virtiofs disabled for Windows guests'
    PUBLIC="$TESTDIR/Public"; mkdir -p "$PUBLIC"
    guest_os=windows
    printf '#!/bin/sh\nexit 0\n' >"$FAKE_BIN/virtiofsd"; chmod +x "$FAKE_BIN/virtiofsd"
    PATH="$FAKE_BIN:$PATH"
    When call fileshare_param_check
    The status should be success
    The variable VIRTIOFSD should eq ''
End
End

Describe 'status_vm'
It 'reports a running VM in human format'
    VM_PID=4242
    When call in_subshell status_vm
    The status should be success
    The output should eq 'running 4242'
End
It 'reports a stopped VM in human format'
    VM_PID=''
    When call in_subshell status_vm
    The status should be success
    The output should eq 'stopped'
End
It 'reports a running VM as schema v1 JSON'
    VM_PID=4242 OUTPUT_FORMAT=json
    When call in_subshell status_vm
    The status should be success
    The output should include '"command":"vm.status"'
        The output should include '"success":true'
        The output should include '"data":{"name":"vm-test","state":"running","pid":4242}'
End
It 'reports a stopped VM as schema v1 JSON with null pid'
    VM_PID='' OUTPUT_FORMAT=json
    When call in_subshell status_vm
    The status should be success
    The output should include '"command":"vm.status"'
        The output should include '"data":{"name":"vm-test","state":"stopped","pid":null}'
End
It 'reports a monitor-paused VM via the .paused marker'
    spawn_fake_vm
    VM_PID=$!
    : >"$VMDIR/vm-test.paused"
    When call in_subshell status_vm
    The status should be success
    The output should include 'paused'
End
It 'reports a paused VM as schema v1 JSON'
    spawn_fake_vm
    VM_PID=$!
    : >"$VMDIR/vm-test.paused"
    OUTPUT_FORMAT=json
    When call in_subshell status_vm
    The status should be success
    The output should include "\"command\":\"vm.status\""
        The output should include "\"data\":{\"name\":\"vm-test\",\"state\":\"paused\",\"pid\":${VM_PID}}"
End
It 'ignores a stale .paused marker when the VM is stopped'
    VM_PID=''
    : >"$VMDIR/vm-test.paused"
    When call in_subshell status_vm
    The status should be success
    The output should eq 'stopped'
End
End

Describe 'list_vms'
It 'reports an empty directory in human format'
    When call list_vms
    The status should be success
    The output should include 'No VM configurations found'
End
It 'reports an empty directory as JSON'
    OUTPUT_FORMAT=json
    When call list_vms
    The status should be success
    The output should include '"command":"vm.list"'
        The output should include '"data":{"vms":[]}'
End
It 'lists stopped and running VMs in human format'
    printf 'guest_os="windows"\n' >a.conf
    printf 'guest_os=linux\n' >b.conf
    mkdir -p a b
    spawn_fake_vm a
    echo $! >a/a.pid
    echo 999999 >b/b.pid
    When call list_vms
    The status should be success
    The output should include 'a'
    The output should include 'windows'
    The output should include 'running'
    The output should include 'stale'
End
It 'lists VMs as schema v1 JSON'
    printf 'guest_os="windows"\n' >a.conf
    mkdir -p a
    spawn_fake_vm a
    echo $! >a/a.pid
    OUTPUT_FORMAT=json
    When call list_vms
    The status should be success
    The output should include '"schemaVersion":1'
    The output should include '"name":"a"'
    The output should include '"guest_os":"windows"'
    The output should include '"state":"running"'
End
End

Describe 'capabilities'
It 'emits the schema v1 capability contract'
    When call capabilities
    The status should be success
    The output should include '"command":"capabilities"'
    The output should include '"applicationVersion":"4.9.9"'
    The output should include '"structuredStatus":true'
    The output should include '"safeDeletePlan":true'
End
End
End
