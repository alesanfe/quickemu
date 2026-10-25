# shellcheck disable=SC2034
Describe 'catalog info, check and validate layers'
Include lib/quickget/catalog/info.sh
Include lib/quickget/catalog/check.sh
Include lib/quickget/catalog/validate.sh

setup() {
    TESTDIR=$(mktemp -d)
    OPERATION=test OS=fedora RELEASE=41 EDITION='' ARCH=amd64 I18NS=()
    PRETTY_OS_NAME='' VM_PATH="$TESTDIR" NRELEASE=''
    is_arch_supported() { return 0; }
    error_not_supported_release() { :; }
    error_specify_os() { echo "specify os"; exit 1; }
    web_check() { return 0; }
}
cleanup() { rm -rf -- "$TESTDIR"; }
BeforeEach 'setup'
AfterEach 'cleanup'

Describe 'os_info entries'
    Parameters
        alma alpine android antix archcraft archlinux artixlinux azurelinux
        batocera bazzite biglinux blackarch blendos blissos bodhi bunsenlabs
        cachyos centos-stream chimeralinux crunchbang++ debian deepin devuan
        dragonos dragonflybsd easyos edubuntu elementary endeavouros endless
        fedora freebsd freedos garuda gentoo ghostbsd gnomeos gobolinux guix
        haiku kali kaos kdeneon kdelinux kolibrios kubuntu linuxlite linuxmint
        lmde lubuntu maboxlinux midnightbsd mageia manjaro mxlinux netboot
        netbsd nitrux nobara nixos nwg-shell omarchy macos openbsd openfyde
        openindiana openmediavault openmandriva opnsense opensuse oraclelinux
        parrotsec pclinuxos peppermint popos porteus primtux q4os proxmox-ve
        pureos qubes reactos redoxos rebornos rockylinux siduction slackware
        slax slint slitaz solus sparkylinux spirallinux tails systemrescue
        templeos tinycore trisquel truenas-scale tuxedo-os ubuntu ubuntu-budgie
        ubuntucinnamon ubuntukylin ubuntu-mate ubuntu-server ubuntustudio
        ubuntu-unity vanillaos vyos void windows windows-server xubuntu zorin
    End
    It 'returns pipe-separated metadata for $1'
        When call os_info "$1"
        The output should include '|'
        The output should include 'http'
    End
End

Describe 'catalog metadata helpers'
    It 'show_os_info prints the four fields'
        When call show_os_info ubuntu
        The output should include 'Ubuntu'
        The output should include 'Website'
    End
    It 'pretty_name returns the display name'
        When call pretty_name alma
        The output should eq 'AlmaLinux'
    End
    It 'os_homepage returns the URL'
        When call os_homepage debian
        The output should eq 'https://www.debian.org/'
    End
    It 'os_support lists every supported OS'
        When call os_support
        The output should include 'ubuntu'
        The output should include 'zorin'
    End
    It 'os_info returns empty for unknown OS'
        When call os_info not-an-os
        The output should be blank
    End
End

Describe 'test_result formatting'
    It 'prints OS-release without result'
        When call test_result alpine 3.20 '' 'https://x/iso'
        The output should include 'alpine-3.20'
        The output should include 'https://x/iso'
    End
    It 'prints edition-qualified name with PASS result'
        When call test_result fedora 41 Workstation 'https://x' PASS
        The output should include 'PASS:'
        The output should include 'fedora-41-Workstation'
    End
    It 'prints SKIP reason instead of URL'
        When call test_result arch 2024 '' '' 'SKIP' '(not available)'
        The output should include 'SKIP:'
        The output should include '(not available)'
    End
End

Describe 'test_all'
    It 'skips OSes not available for the arch'
        is_arch_supported() { return 1; }
        releases_fedora() { echo "40 41"; }
        When call test_all fedora
        The output should include 'SKIP'
    End
    It 'checks URLs for each release of a plain OS'
        releases_alpine() { echo "3.20 3.21"; }
        get_alpine() { echo "https://dl/alpine.iso"; }
        When call test_all alpine
        The output should include 'PASS'
        The output should include 'alpine-3.21'
    End
    It 'iterates editions for editions OSes'
        releases_fedora() { echo 41; }
        editions_fedora() { echo "Workstation KDE"; }
        get_fedora() { echo "https://dl/f.iso"; }
        When call test_all fedora
        The output should include 'fedora-41-Workstation'
        The output should include 'fedora-41-KDE'
    End
    It 'reports windows languages as SKIP in test mode'
        releases_windows() { echo 11; }
        languages_windows() { I18NS=("English International" "Spanish"); }
        When call test_all windows
        The output should include 'SKIP'
        The output should include 'windows-11-Spanish'
    End
    It 'delegates macos to get_macos'
        releases_macos() { echo sonoma; }
        get_macos() { echo "MACOS-GET"; }
        When call test_all macos
        The output should include 'MACOS-GET'
    End
    It 'shows the URL instead of checking in show mode'
        OPERATION=show
        releases_alpine() { echo 3.20; }
        get_alpine() { echo "https://dl/a.iso"; }
        When call test_all alpine
        The output should include 'https://dl/a.iso'
        The output should not include 'PASS'
    End
End

Describe 'catalog_update_check'
    It 'reports latest upstream and local dirs'
        releases_alpine() { echo "3.21 3.20"; }
        mkdir -p "$TESTDIR/alpine-3.21" "$TESTDIR/alpine-3.19"
        in_dir() { ( cd "$1" && shift && "$@" ); }
        When run in_dir "$TESTDIR" catalog_update_check alpine
        The status should be success
        The output should include 'Latest alpine release upstream: 3.21'
        The output should include 'up to date'
        The output should include 'older release'
    End
    It 'reports when no local dirs exist'
        releases_gentoo() { echo "2024"; }
        in_dir() { ( cd "$1" && shift && "$@" ); }
        When run in_dir "$TESTDIR" catalog_update_check gentoo
        The status should be success
        The output should include 'No local gentoo VM directories found.'
    End
End

Describe 'catalog_check_all_arch'
    It 'skips unsupported arches and tests the rest'
        is_arch_supported() { [ "$2" = amd64 ]; }
        releases_alpine() { echo 3.20; }
        get_alpine() { echo "https://dl/a.iso"; }
        When run catalog_check_all_arch alpine 3.20
        The status should be success
        The output should include 'SKIP'
        The output should include 'PASS'
    End
    It 'delegates ubuntu releases to get_ubuntu'
        releases_ubuntu() { echo 24.04; }
        get_ubuntu() { echo "UBUNTU-GET"; }
        When run catalog_check_all_arch ubuntu 24.04
        The status should be success
        The output should include 'UBUNTU-GET'
    End
    It 'reports windows as not supported in check mode'
        releases_windows() { echo 11; }
        When run catalog_check_all_arch windows 11
        The status should be success
        The output should include 'Windows not supported'
    End
End

Describe 'validation helpers'
    It 'os_supported accepts a supported OS and sets PRETTY_OS_NAME'
        OS=alpine
        When call os_supported
        The variable PRETTY_OS_NAME should eq 'Alpine Linux'
    End
    It 'is_valid_language accepts listed languages'
        I18NS=("English International" "Spanish")
        When call is_valid_language 'Spanish'
        The status should be success
    End
    It 'is_valid_language rejects others'
        I18NS=("English International")
        When call is_valid_language 'Klingon'
        The status should be failure
    End
    It 'handle_missing rejects Fedora <40 Onyx'
        OS=fedora RELEASE=39 EDITION=Onyx
        When run handle_missing
        The status should be failure
        The output should include 'Unsupported combination'
    End
    It 'handle_missing accepts Fedora 40+ Onyx'
        OS=fedora RELEASE=41 EDITION=Onyx
        When call handle_missing
        The status should be success
    End
    It 'handle_missing strips the _Beta suffix before comparing'
        OS=fedora RELEASE='39_Beta' EDITION=Sericea
        When run handle_missing
        The status should be failure
        The output should include 'Unsupported combination'
    End
    It 'validate_release resolves the ubuntu generator for ubuntu flavours'
        OS=kubuntu
        releases_ubuntu() { echo "24.04"; }
        error_not_supported_release() { echo "ERR:$RELEASES via $RELEASE_GENERATOR"; }
        When call validate_release releases_kubuntu
        The output should eq 'ERR:24.04 via releases_ubuntu'
    End
    It 'catalog_check_os_arch exits early for unsupported arch in test mode'
        OPERATION=test OS=alpine
        is_arch_supported() { return 1; }
        When run catalog_check_os_arch alpine 3.20
        The status should be success
        The output should include 'SKIP'
    End
    It 'catalog_check_os_arch is a no-op for supported arches'
        OS=alpine
        When call catalog_check_os_arch alpine 3.20
        The status should be success
    End
    It 'catalog_resolve_edition sets EDITION and VM_PATH'
        OS=fedora RELEASE=41
        releases_fedora() { echo 41; }
        editions_fedora() { echo "Workstation KDE"; }
        arch_suffix() { echo ''; }
        When call catalog_resolve_edition fedora KDE
        The variable EDITION should eq 'KDE'
        The variable VM_PATH should eq 'fedora-41-KDE'
    End
    It 'catalog_resolve_edition rejects unknown editions'
        OS=fedora RELEASE=41
        releases_fedora() { echo 41; }
        editions_fedora() { echo "Workstation"; }
        When run catalog_resolve_edition fedora Bogus
        The status should be failure
        The output should include 'not a supported'
    End
End
End
