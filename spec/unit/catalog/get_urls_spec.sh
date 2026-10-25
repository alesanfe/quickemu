Describe 'quickget download URL builders'
Include lib/quickget/catalog/data.sh
Include lib/quickget/catalog/arch.sh
Include lib/quickget/catalog/info.sh
Include lib/quickget/catalog/check.sh
Include lib/quickget/support/errors.sh
Include lib/quickget/download/web.sh
Include lib/quickget/catalog/get.sh

setup() {
    OPERATION=create
    OS=linux
    RELEASE=latest
    EDITION=default
    I18N=English
    I18NS=(English Spanish)
    ARCH=amd64
    NORMALISED_HOST_ARCH=amd64
    VM_PATH=/tmp/qg-spec
    QEMU_IMG=qemu-img
    FILE_NAME=''
    CURL_INSECURE=false
    PRETTY_OS_NAME=Test
}
BeforeEach 'setup'

web_pipe() {
    case "$1" in
        *getfedora.org*) printf '%s' '[{"variant":"Workstation","subvariant":"Workstation","arch":"x86_64","version":"41","link":"https://dl.fedoraproject.org/fedora.iso","sha256":"abc"},{"variant":"Spins","subvariant":"KDE","arch":"x86_64","version":"41 Beta","link":"https://dl.fedoraproject.org/kde.iso","sha256":"abc"}]';;
        *tails.boum.org*) printf '%s' '{"installations":[{"installation-paths":[{"type":"iso","target-files":[{"url":"https://tails.example/tails.iso","sha256":"abc"}]}]}]}';;
        *releng/releases/json*) printf '%s' '{"releases":[{"iso_url":"archlinux-x86_64.iso","sha256_sum":"abc"}]}';;
        *fosshub*) printf '%s' 'var settings ={"pool":{"f":[{"n":"android-x86_64-9.0.iso","hash":{"sha256":"abc"}}]}}';;
        *api.pop-os.org*) printf '%s' '{"url":"https://popos.example/pop.iso","sha_sum":"abc"}';;
        *soulharsh007*) printf '%s' '{"url":"https://rebornos.example/reborn.iso","md5":"abc"}';;
        *file-info.json*) printf '%s' '{"official":{"latest":{"image":"https://manjaro.example/m.iso"},"gnome":{"image":"https://manjaro.example/m.iso"}}}';;
        *) cat <<FIXTURE
$1
$1.iso
https://mirror.example/file.iso deadbeef
SHA256 (file.iso) = deadbeef
deadbeef file.iso.md5
<a href="file.iso">file.iso</a>
deadbeef *ubuntu-24.04-desktop-amd64.iso
deadbeef *ubuntu-24.04-live-server-amd64.iso
ISO/desktop;https://mirror.example/cachyos.iso
folder"https://mirror.example/dir/"
"browser_download_url": "https://github.example/v12-amd64.iso"
deadbeef *ubuntu-24.04-desktop-arm64.iso
deadbeef *ubuntu-24.04-live-server-arm64.iso
"iso_url": "x.iso"
Nobara-41-Official-2024-01-01.iso
Zorin-OS-17.3-Core-64-bit.iso
pclinuxos64-kde-2024.1.iso
deadbeef slax-64bit-15.0.iso
2024/
2023/
click here href="https://mirror.example/mageia.iso"
OPNsense-24.7-dvd-amd64.iso.bz2
deadbeef redox_default_1_livedisk.iso.zst
kde-linux_202501010101.iso
FIXTURE
    ;;
    esac
}
web_check() { return 0; }
web_redirect() { printf '%s\n' "$1"; }
web_get() { printf 'web_get %s\n' "$1"; }
zsync_get() { printf 'zsync %s\n' "$1"; }
check_hash() { printf 'hash %s %s\n' "$1" "$2"; }
curl() { printf '%s\n' 'session=1' 'https://oscdn.apple.com/x.dmg' 'expires=1 dmg' 'https://oscdn.apple.com/x.chunklist' 'expires=2 chunklist' 'kde-linux_202501010101.iso'; }
unattended_windows() { :; }
download_windows_workstation() { printf 'workstation %s\n' "$1"; }
download_windows_server() { printf 'server %s\n' "$1"; }
make_vm_config() { printf 'config %s\n' "$1"; }

Describe 'URL emission for every supported OS'
Parameters
    alma 9 dvd
    alpine v3.20 none
    android 9.0 x86_64
    antix 23.1 full-sysv
    antix 21 base-runit
    archcraft latest none
    archlinux latest none
    artixlinux latest base-dinit
    azurelinux 3.0 none
    batocera latest none
    bazzite latest gnome
    biglinux latest kde
    blendos latest none
    bodhi 7.0.0 none
    bunsenlabs latest none
    cachyos latest desktop
    centos-stream 9 none
    chimeralinux latest gnome
    crunchbang++ 12 none
    debian 12 netinst
    deepin 23 none
    devuan 5.0 none
    dragonflybsd 6.4.0 none
    dragonos latest none
    easyos latest none
    elementary 8.1 none
    endeavouros latest none
    endless latest none
    fedora 41 Workstation
    fedora 41_Beta KDE
    freebsd 14.1 none
    freedos 1.3 none
    garuda latest kde
    gentoo latest minimal
    gentoo latest livegui
    ghostbsd latest none
    gobolinux latest none
    gnomeos latest none
    guix latest none
    haiku r1beta5 none
    kali latest none
    kdeneon latest none
    kdelinux latest none
    kolibrios latest eng
    templeos 5.03 none
    linuxlite latest none
    linuxmint 22 cinnamon
    lmde 6 none
    maboxlinux latest none
    mageia 9 kde
    manjaro latest gnome
    mxlinux latest xfce
    netboot latest none
    netbsd 10.0 none
    nitrux latest none
    nixos latest gnome
    nwg-shell latest none
    omarchy latest none
    openfyde latest none
    openbsd 7.6 none
    openindiana latest none
    opensuse 15.6 kde
    oraclelinux 9 none
    parrotsec latest home
    pclinuxos latest kde
    peppermint latest none
    popos 22.04 none
    porteus latest xfce
    primtux latest none
    proxmox-ve 8 none
    pureos latest none
    qubes 4.2 none
    redoxos latest none
    reactos 0.4.15 bootcd
    rebornos latest none
    rockylinux 9 dvd
    siduction latest kde
    slackware 15.0 none
    slax latest debian
    slint 15.0 none
    slitaz latest none
    solus 4.5 budgie
    sparkylinux latest lxqt
    spirallinux latest xfce
    tails latest none
    tinycore 15 TinyCorePure64
    trisquel 11 mate
    tuxedo-os latest none
    ubuntu 24.04 desktop
    ubuntu-server 24.04 live-server
    vanillaos latest none
    void latest xfce
    zorin 17 core64
    nobara 41 Official
    opnsense 24.7 none
    vyos latest none
    blackarch latest none
    blissos latest foss
    kaos latest none
    midnightbsd latest none
    openmandriva latest plasma
    openmediavault 7 none
    q4os latest kde
    systemrescue latest none
    truenas-scale latest none
End
It 'emits a downloadable URL without network access'
    OS="$1" RELEASE="$2" EDITION="$3"
    [ "$EDITION" = none ] && EDITION=''
    When call "get_$1"
    The status should be success
    The output should include 'http'
End
End

Describe 'architecture-sensitive builders'
Parameters
    alma arm64 aarch64
    alma amd64 x86_64
    alpine arm64 aarch64
    azurelinux arm64 aarch64
    ubuntu arm64 arm64
End
It 'uses the requested guest architecture in the artifact path'
    OS="$1" ARCH="$2" RELEASE=latest EDITION=dvd
    [ "$OS" = alma ] && RELEASE=9
    [ "$OS" = alpine ] && RELEASE=v3.20
    [ "$OS" = azurelinux ] && RELEASE=3.0
    [ "$OS" = ubuntu ] && { RELEASE=24.04; EDITION=desktop; }
    When call "get_$1"
    The status should be success
    The output should include "$3"
End
End

Describe 'orchestrated downloads'
It 'routes Windows workstation downloads through the Windows downloader'
    OS=windows RELEASE=11 OPERATION=download
    When run get_windows
    The status should be success
    The output should include 'workstation 11'
End
It 'routes Windows Server downloads through the server downloader'
    OS=windows-server RELEASE=2022 OPERATION=download
    When run get_windows
    The status should be success
    The output should include 'server windows-server-2022'
End
It 'builds Windows unattended media when not in download-only mode'
    OS=windows RELEASE=11 OPERATION=create
    When run get_windows
    The status should be success
    The output should include 'workstation 11'
    The output should include 'virtio-win.iso'
End
Describe 'macOS recovery releases'
Parameters
    lion 10.7
    sonoma 14
    tahoe 26
End
It 'maps the release to a recovery board and fetches the image list'
    OS=macos RELEASE="$1" OPERATION=download
    When run get_macos
    The status should be success
    The output should include 'Downloading macOS'
End
End
It 'rejects an unknown macOS release'
    OS=macos RELEASE=invalid OPERATION=create
    When run get_macos
    The status should eq 1
    The output should include 'Unknown release'
End
End
End
