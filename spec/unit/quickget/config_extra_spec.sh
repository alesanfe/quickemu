# shellcheck disable=SC2034
Describe 'quickget vm config generation'
Include lib/quickget/vm/config.sh
Include lib/quickget/image/generate.sh

setup() {
    TESTDIR=$(mktemp -d)
    VM_PATH="$TESTDIR/os-1"
    mkdir -p "$VM_PATH"
    cd "$TESTDIR" || return
    OS=alpine RELEASE=1 EDITION='' ARCH=amd64 NORMALISED_HOST_ARCH=amd64
    OPERATION='' QUICKEMU=/usr/bin/quickemu CONF_FILE=''
    CUSTOM_OS='' CUSTOM_IMAGE_TYPE='' CLOUD_INIT=false UNATTENDED=false
    # make_vm_config always exits; silence its chatter in run-subshells.
    mkconf() { make_vm_config "$@" >/dev/null 2>&1; }
}
cleanup() { cd "$SHELLSPEC_PROJECT_ROOT" || cd /; rm -rf -- "$TESTDIR"; }
BeforeEach 'setup'
AfterEach 'cleanup'

Describe 'make_vm_config guest/image mapping'
    Parameters
        batocera    batocera    img
        dragonflybsd dragonflybsd iso
        easyos      linux       img
        freebsd     freebsd     iso
        ghostbsd    freebsd     iso
        haiku       haiku       iso
        freedos     freedos     iso
        kolibrios   kolibrios   iso
        macos       macos       img
        netbsd      netbsd      iso
        openbsd     openbsd     iso
        openfyde    linux       img
        openindiana solaris     iso
        reactos     reactos     iso
        templeos    templeos    iso
        windows     windows     iso
        windows-server windows-server iso
        alpine      linux       iso
    End
    It "maps $1 to guest_os=$2 image type $3"
        OS="$1"
        When run mkconf inst.iso
        The contents of file "$VM_PATH.conf" should include "guest_os=\"$2\""
        The contents of file "$VM_PATH.conf" should include "$3=\"$VM_PATH/inst.iso\""
    End
    It 'maps old ubuntu releases to linux_old'
        OS=ubuntu RELEASE=14.04
        When run mkconf u.iso
        The contents of file "$VM_PATH.conf" should include 'guest_os="linux_old"'
    End
    It 'keeps linux for modern ubuntu releases'
        OS=ubuntu RELEASE=24.04
        When run mkconf u.iso
        The contents of file "$VM_PATH.conf" should include 'guest_os="linux"'
    End
    It 'keeps linux for ubuntu dailies'
        OS=ubuntu RELEASE=daily-live
        When run mkconf u.iso
        The contents of file "$VM_PATH.conf" should include 'guest_os="linux"'
        The contents of file "$VM_PATH.conf" should include 'disk_size="32G"'
    End
    It 'maps a custom OS to the CUSTOM_* globals'
        OS=custom CUSTOM_OS=windows CUSTOM_IMAGE_TYPE=iso
        When run mkconf c.iso
        The contents of file "$VM_PATH.conf" should include 'guest_os="windows"'
    End
End

Describe 'make_vm_config disk image overrides'
    Parameters
        qcow2   qcow2
        vmdk    vmdk
        raw     raw
        img     raw
    End
    It "treats .$1 files as the VM disk (format $2)"
        When run mkconf "disk.$1"
        The contents of file "$VM_PATH.conf" should include "disk_img=\"$VM_PATH/disk.$1\""
        The contents of file "$VM_PATH.conf" should include "disk_format=\"$2\""
        The contents of file "$VM_PATH.conf" should not include 'iso='
    End
End

Describe 'make_vm_config details'
    It 'emits an arch line for foreign architectures'
        ARCH=arm64 NORMALISED_HOST_ARCH=amd64
        When run mkconf i.iso
        The contents of file "$VM_PATH.conf" should include 'arch="aarch64"'
    End
    It 'omits the arch line on the native architecture'
        When run mkconf i.iso
        The contents of file "$VM_PATH.conf" should not include 'arch='
    End
    It 'writes fixed_iso when a second image is given'
        When run mkconf i.iso virtio.iso
        The contents of file "$VM_PATH.conf" should include "fixed_iso=\"$VM_PATH/virtio.iso\""
    End
    It 'does not overwrite an existing conf'
        printf 'guest_os="mine"\n' > "$VM_PATH.conf"
        When run mkconf i.iso
        The contents of file "$VM_PATH.conf" should eq 'guest_os="mine"'
    End
    It 'exits early in download mode'
        OPERATION=download
        When run make_vm_config i.iso
        The status should be success
        The output should be blank
    End
    It 'prints the quickemu hint and exits'
        When run make_vm_config i.iso
        The status should be success
        The output should include 'quickemu --vm'
    End
    It 'applies per-OS disk tweaks'
        OS=archlinux
        When run mkconf i.iso
        The contents of file "$VM_PATH.conf" should include 'secureboot="on"'
        The contents of file "$VM_PATH.conf" should include 'tpm="on"'
    End
    It 'enables TPM for Windows 11'
        OS=windows RELEASE=11
        When run mkconf w.iso
        The contents of file "$VM_PATH.conf" should include 'tpm="on"'
        The contents of file "$VM_PATH.conf" should include 'secureboot="off"'
        The contents of file "$VM_PATH.conf" should include 'disk_size="64G"'
    End
    It 'records the macos release and monterey core tweak'
        OS=macos RELEASE=monterey
        When run mkconf m.img
        The contents of file "$VM_PATH.conf" should include 'macos_release="monterey"'
        The contents of file "$VM_PATH.conf" should include 'cpu_cores=2'
    End
    It 'warns that unattended only supports ubuntu-server'
        OS=debian UNATTENDED=true
        When run make_vm_config i.iso
        The output should include 'only supported for ubuntu-server'
    End
    It 'generates autoinstall files for ubuntu-server'
        OS=ubuntu-server UNATTENDED=true RELEASE=24.04
        gen_ubuntu_autoinstall() { echo "AUTOINSTALL $1 $2"; }
        When run make_vm_config i.iso
        The output should include 'AUTOINSTALL'
    End
    It 'runs gen_seed_iso when CLOUD_INIT is set'
        OS=alpine CLOUD_INIT=true
        cloud-localds() { touch "$VM_PATH/seed.iso"; }
        mkdir -p "$VM_PATH"
        When run make_vm_config i.iso
        The file "$VM_PATH/seed.iso" should be exist
        The contents of file "$VM_PATH.conf" should include 'seed='
        The output should include 'NoCloud seed.iso created'
    End
End

Describe 'create_vm'
    It 'downloads, normalises and writes the config'
        web_get() { :; }
        check_hash() { echo "HASH $1"; }
        image_normalize() { :; }
        When run create_vm 'https://x/os.iso abc123'
        The output should include 'HASH os.iso'
        The file "$VM_PATH.conf" should be exist
    End
    It 'skips the checksum when none is given'
        web_get() { :; }
        image_normalize() { :; }
        When run create_vm 'https://x/os.iso'
        The output should include 'Making'
        The file "$VM_PATH.conf" should be exist
    End
End

Describe 'gen_seed_iso'
    It 'writes user-data and meta-data and builds seed.iso'
        cloud-localds() { touch "$VM_PATH/seed.iso"; }
        HOME="$TESTDIR" USER=tester CONF_FILE="$VM_PATH.conf"
        printf 'x' > "$VM_PATH.conf"
        mkdir -p "$HOME/.ssh" && printf 'ssh-ed25519 AAAA k\n' > "$HOME/.ssh/id_ed25519.pub"
        When call gen_seed_iso "$VM_PATH"
        The output should include 'NoCloud seed.iso created'
        The file "$VM_PATH/user-data" should be exist
        The file "$VM_PATH/meta-data" should be exist
        The file "$VM_PATH/seed.iso" should be exist
        The contents of file "$VM_PATH/user-data" should include 'name: tester'
        The contents of file "$VM_PATH/user-data" should include 'ssh_authorized_keys'
        The contents of file "$VM_PATH.conf" should include 'seed='
    End
    It 'errors when no ISO tool is available'
        command() { case "$2" in cloud-localds|genisoimage|xorrisofs|mkisofs) return 1;; *) builtin command "$@";; esac; }
        When run gen_seed_iso "$VM_PATH"
        The status should be failure
        The output should include 'No tool to build seed.iso'
    End
End
End
