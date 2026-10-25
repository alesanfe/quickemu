Describe 'display and audio configuration'
Include lib/quickemu/vm/display.sh

setup() {
    QEMU_VER_SHORT=101
    QEMU=qemu_probe
    guest_os=linux
    display=sdl
    gl=off
    width=''
    height=''
    ARCH_VM=x86_64
    ACCESS=''
    FULLSCREEN=''
    max_outputs=''
    viewer=none
    PUBLIC=''
    AUDIO_SOCKS=''
    XDG_RUNTIME_DIR=/nonexistent
    TESTDIR=$(mktemp -d)
    PIPEWIRE_REMOTE="$TESTDIR/pipewire-0"
    PULSE_SERVER="$TESTDIR/native"
}
cleanup() { qe_spec_cleanup; }
BeforeEach 'setup'
AfterEach 'cleanup'

# Seam stub: simulate live pipewire/pulse sockets (real AF_UNIX sockets
# cannot be created on every host the suite runs on).
make_socket() { AUDIO_SOCK_READY=0; }
host_socket_present() { return "${AUDIO_SOCK_READY:-1}"; }
check_cocoa_gl_es_support() { return "${COCOA_GL:-1}"; }
qemu_probe() {
    case "$*" in
        *-audio-help*) printf 'driver=%s\n' "${AUDIO_DRIVERS:-alsa pipewire pa}";;
        *-device*) printf 'fake-device\n';;
    esac
}

Describe 'guest display device selection'
Parameters
    linux sdl x86_64 virtio-vga
    linux spice x86_64 virtio-gpu
    linux sdl aarch64 virtio-gpu-pci
    freebsd sdl x86_64 VGA
    solaris sdl x86_64 vmware-svga
    linux_old sdl x86_64 vmware-svga
    macos sdl x86_64 vmware-svga
    windows sdl x86_64 virtio-vga
    windows spice x86_64 qxl-vga
    windows vnc x86_64 virtio-vga
    windows sdl aarch64 virtio-gpu-pci
    haiku sdl x86_64 qxl-vga
End
It 'maps the guest OS and display to a supported device'
    guest_os="$1" display="$2" ARCH_VM="$3"
    When call configure_display
    The status should be success
    The variable VIDEO should include "$4"
    The output should include "$4"
End
End

Describe 'display backend rendering'
Parameters
    none off none
    vnc off none
    gtk off gtk,grab-on-hover=on,zoom-to-fit=off,gl=off
    sdl on sdl,gl=on
    spice-app on spice-app,gl=on
End
It 'maps the display value to a QEMU -display backend'
    display="$1" gl="$2"
    When call configure_display
    The status should be success
    The variable DISPLAY_RENDER should eq "$3"
    The output should include 'Display:'
End
End
It 'uses egl-headless for local SPICE with GL enabled'
    display=spice gl=on
    When call configure_display
    The status should be success
    The variable DISPLAY_RENDER should eq 'egl-headless,rendernode=/dev/dri/renderD128'
    The output should include 'VirGL (on)'
End
It 'disables GL for remote SPICE'
    display=spice gl=on ACCESS=remote
    When call configure_display
    The status should be success
    The variable DISPLAY_RENDER should eq 'none'
    The variable gl should eq 'off'
    The output should include 'Display:'
End
It 'forces GL off with no display'
    display=none gl=on
    When call configure_display
    The status should be success
    The variable DISPLAY_RENDER should eq 'none'
    The variable gl should eq 'off'
    The output should include 'Display:'
End
It 'adds fullscreen grab and disables zoom-to-fit when requested'
    display=gtk FULLSCREEN='-full-screen'
    When call configure_display
    The status should be success
    The variable DISPLAY_RENDER should include 'full-screen=on'
    The output should include 'Display:'
End

Describe 'audio driver selection'
Parameters
    pipewire pipewire,id=audio0
    pa pa,id=audio0
    alsa alsa,id=audio0
End
It 'selects the first audio backend the QEMU build supports'
    make_socket "$PIPEWIRE_REMOTE"
    make_socket "$PULSE_SERVER"
    AUDIO_DRIVERS="$1" QEMU_VER_SHORT=90
    When call configure_display
    The status should be success
    The variable AUDIO_DEV should eq "$2"
    The output should include 'Display:'
End
It 'uses the SPICE audio device for headless displays'
    display=none
    When call configure_display
    The status should be success
    The variable AUDIO_DEV should eq 'spice,id=audio0'
    The output should include 'Display:'
End
It 'uses CoreAudio for the cocoa display'
    display=cocoa gl=on COCOA_GL=0
    When call configure_display
    The status should be success
    The variable AUDIO_DEV should eq 'coreaudio,id=audio0'
    The output should include 'Display:'
    The variable DISPLAY_RENDER should include 'gl=es'
End
End

Describe 'geometry and multiscreen'
It 'applies a configured resolution to virtio devices'
    width=1920 height=1080
    When call configure_display
    The status should be success
    The variable VIDEO should include 'xres=1920,yres=1080'
    The output should include 'Display:'
End
It 'applies max_outputs when configured'
    max_outputs=2
    When call configure_display
    The status should be success
    The variable VIDEO should include 'max_outputs=2'
    The output should include 'Display:'
End
It 'allocates VRAM for a qxl device'
    guest_os=windows display=spice
    When call configure_display
    The status should be success
    The variable VIDEO should include 'vgamem_mb=64'
    The output should include 'Display:'
End
It 'adds ramfb for ARM64 firmware consoles without virtio-gpu'
    guest_os=freebsd ARCH_VM=aarch64
    When call configure_display
    The status should be success
    The variable VIDEO should include 'ramfb'
    The output should include 'Display:'
End
End
End
