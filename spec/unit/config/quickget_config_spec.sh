Describe 'quickget configuration generation'
Include lib/quickget/vm/config.sh

setup() {
    TESTDIR=$(mktemp -d)
    VM_PATH="$TESTDIR/vm with spaces"
    mkdir "$VM_PATH"
    OPERATION=create
    OS=linux
    RELEASE=24.04
    EDITION=desktop
    ARCH=amd64
    NORMALISED_HOST_ARCH=amd64
    QUICKEMU=/usr/bin/quickemu
    OS_DISPLAY_NAME='Test OS'
    CLOUD_INIT=false
    UNATTENDED=false
}
cleanup() {
    rm -f -- "$VM_PATH.conf" "$VM_PATH"/*
    rmdir -- "$VM_PATH" "$TESTDIR"
}
BeforeEach 'setup'
AfterEach 'cleanup'

Describe 'guest defaults'
Parameters
    batocera 1 batocera 'disk_size="8G"'
    dragonflybsd 1 dragonflybsd 'boot="legacy"'
    easyos 1 linux 'guest_os="linux"'
    freebsd 1 freebsd 'guest_os="freebsd"'
    ghostbsd 1 freebsd 'guest_os="freebsd"'
    haiku 1 haiku 'boot="legacy"'
    freedos 1 freedos 'ram="256M"'
    kolibrios 1 kolibrios 'ram="128M"'
    macos monterey macos 'cpu_cores=2'
    netbsd 1 netbsd 'boot="legacy"'
    openbsd 1 openbsd 'boot="legacy"'
    openfyde 1 linux 'guest_os="linux"'
    openindiana 1 solaris 'boot="legacy"'
    reactos 1 reactos 'ram="2048M"'
    templeos 1 templeos 'ram="512M"'
    ubuntu 14.04 linux_old 'guest_os="linux_old"'
    ubuntu 24.04 linux 'guest_os="linux"'
    ubuntu daily-live linux 'disk_size="32G"'
    ubuntu-server 22.04 linux 'tpm="on"'
    windows 11 windows 'tpm="on"'
    windows-server 2022 windows-server 'secureboot="off"'
    archlinux latest linux 'secureboot="on"'
    alma 9 linux 'disk_size="32G"'
    bazzite latest linux 'disk_size="64G"'
    qubes 4 linux 'disk_size="128G"'
    debian 12 linux 'tpm="on"'
    debian 13 linux 'tpm="on"'
    deepin 1 linux 'ram="4G"'
    slint 1 linux 'disk_size="50G"'
    slitaz 1 linux 'ram="512M"'
    vanillaos 1 linux 'disk_size="64G"'
    elementary 8.1 linux 'display="spice"'
    proxmox-ve 1 linux 'disk_size="20G"'
End
It 'generates a usable configuration with the required guest policy'
    OS="$1" RELEASE="$2"
    When run make_vm_config installer.iso
    The status should be success
    The output should include 'To start your Test OS'
    The contents of file "$VM_PATH.conf" should include "guest_os=\"$3\""
    The contents of file "$VM_PATH.conf" should include "$4"
    The path "$VM_PATH.conf" should be executable
End
End

Describe 'downloaded disk image formats'
Parameters
    image.qcow2 qcow2
    image.vmdk vmdk
    image.raw raw
    image.img raw
    IMAGE.QCOW2 qcow2
End
It 'uses the image as the VM disk instead of installer media'
    When run make_vm_config "$1"
    The status should be success
    The output should include 'Making'
    The contents of file "$VM_PATH.conf" should include "disk_img=\"$VM_PATH/$1\""
    The contents of file "$VM_PATH.conf" should include "disk_format=\"$2\""
    The contents of file "$VM_PATH.conf" should not include 'iso='
End
End
It 'does not set a disk format for an ordinary installer'
    When run make_vm_config installer.iso drivers.iso
    The status should be success
    The output should include 'Making'
    The contents of file "$VM_PATH.conf" should include "iso=\"$VM_PATH/installer.iso\""
    The contents of file "$VM_PATH.conf" should include "fixed_iso=\"$VM_PATH/drivers.iso\""
    The contents of file "$VM_PATH.conf" should not include 'disk_format='
End
It 'does not overwrite an existing configuration'
    printf 'ram="12G"\n' > "$VM_PATH.conf"
    When run make_vm_config installer.iso
    The status should be success
    The output should not include 'Making'
    The contents of file "$VM_PATH.conf" should eq 'ram="12G"'
End
It 'does not create a configuration in download-only mode'
    OPERATION=download
    When run make_vm_config installer.iso
    The status should be success
    The path "$VM_PATH.conf" should not be exist
End
Describe 'cross architecture configuration'
Parameters
    arm64 amd64 aarch64
    amd64 arm64 x86_64
End
It 'writes the guest architecture only when it differs from the host'
    ARCH="$1" NORMALISED_HOST_ARCH="$2"
    When run make_vm_config installer.iso
    The status should be success
    The output should include 'Making'
    The contents of file "$VM_PATH.conf" should include "arch=\"$3\""
End
End
End
