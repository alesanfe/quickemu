Describe 'catalog release and edition functions'
Include lib/quickget/catalog/data.sh

web_pipe() {
    case "$1" in
        *fedoraproject*|*getfedora*) printf '%s' '[{"version":"41"}]';;
        *api.github.com*|*ubuntu*|*openfyde*) printf '%s' '[{"tag_name":"24.04","name":"24.04","version":"24.04","releases":[{"version":"24.04"}]}]';;
        *) cat <<FIXTURE
$1
v3.20/
v3.19/
"2024-01-01"
artix-base-dinit-20240101-x86_64.iso
biglinux_2024-01-01_k69.iso
<a href="40/">40/</a>
<a href="stable/">stable/</a>
Fedora-Workstation-Live-x86_64-41-1.4.iso
batocera-x86_64-39-20240101.img.gz
endeavouros-2024.iso
easy-5.8-amd64.img
MX-23.4_x64.iso
kde-neon-user-20240101.iso
siduction-24.01-name-desktop-amd64-20240101.iso.md5
manjaro-gnome-24.0.iso
Trident-24.01-x64.iso
FIXTURE
    ;;
    esac
}
web_check() { return 0; }
web_pipe_json() { printf '%s' '["budgie","gnome","kde","xfce"]'; }
curl() { printf 'v3.20/\n"24.04/"\nname.iso\n'; }

# Every catalog function must complete against controlled upstream content
# and emit at most a whitespace-separated list of candidates.
Describe 'releases_* functions'
Parameters
    alma
    alpine
    android
    antix
    archcraft
    archlinux
    artixlinux
    azurelinux
    batocera
    bazzite
    biglinux
    blendos
    bodhi
    bunsenlabs
    cachyos
    centos-stream
    chimeralinux
    crunchbang++
    debian
    deepin
    devuan
    dragonflybsd
    dragonos
    easyos
    elementary
    endeavouros
    endless
    fedora
    freebsd
    freedos
    garuda
    gentoo
    ghostbsd
    gobolinux
    gnomeos
    guix
    haiku
    kali
    kdeneon
    kdelinux
    kolibrios
    templeos
    linuxlite
    linuxmint
    lmde
    maboxlinux
    macos
    mageia
    manjaro
    mxlinux
    netboot
    netbsd
    nitrux
    nixos
    nwg-shell
    omarchy
    openfyde
    openbsd
    openindiana
    opensuse
    oraclelinux
    parrotsec
    pclinuxos
    peppermint
    popos
    porteus
    primtux
    proxmox-ve
    pureos
    qubes
    redoxos
    reactos
    rebornos
    rockylinux
    siduction
    slackware
    slax
    slint
    slitaz
    solus
    sparkylinux
    spirallinux
    tails
    tinycore
    trisquel
    tuxedo-os
    ubuntu
    ubuntu-server
    vanillaos
    void
    windows
    windows-server
    zorin
    nobara
    opnsense
    vyos
    blackarch
    blissos
    kaos
    midnightbsd
    openmediavault
    q4os
    systemrescue
    truenas-scale
    openmandriva
End
It 'returns releases without touching the network'
    When call "releases_$1"
    The status should be success
    The output should not include 'syntax error'
    The stderr should not include 'command not found'
End
End

Describe 'editions_* functions'
Parameters
    alma
    android
    antix
    artixlinux
    bazzite
    biglinux
    bodhi
    cachyos
    centos-stream
    chimeralinux
    debian
    endless
    fedora
    freebsd
    garuda
    gentoo
    ghostbsd
    haiku
    kdeneon
    kolibrios
    linuxmint
    lmde
    mageia
    manjaro
    mxlinux
    nixos
    openindiana
    parrotsec
    pclinuxos
    peppermint
    popos
    porteus
    primtux
    pureos
    redoxos
    rockylinux
    siduction
    slax
    solus
    sparkylinux
    spirallinux
    tinycore
    trisquel
    void
    zorin
    blackarch
    openmandriva
    q4os
    nobara
End
It 'returns editions without touching the network'
    When call "editions_$1"
    The status should be success
    The output should not include 'syntax error'
    The stderr should not include 'command not found'
End
End

Describe 'language catalogs'
Parameters
    windows
    windows-server
End
It 'returns the supported Windows languages'
    lang_list() { languages_"$1"; printf '%s\n' "${I18NS[@]}"; }
    When call lang_list "$1"
    The status should be success
    The output should include 'English'
End
End

Describe 'stable data invariants'
Parameters
    alma:9:boot
    archlinux:latest:latest
    bazzite:latest:gnome
    freedos:1.3:1.3
    kolibrios:latest:eng
    windows:11:11
End
It 'keeps constant releases non-empty and ordered'
    When call "releases_${1%%:*}"
    The status should be success
    The output should not include 'syntax error'
    The stderr should not include 'command not found'
End
End
End
