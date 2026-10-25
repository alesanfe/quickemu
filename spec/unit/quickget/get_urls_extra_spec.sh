# shellcheck disable=SC2034
# Exercise every get_<os> URL builder with network stubs. Each emits
# "URL [hash]" (or nothing for unsupported selections) — the value of
# these tests is covering the per-OS URL construction branches.
Describe 'quickget get_<os> URL builders'
Include lib/quickget/catalog/get.sh
Include lib/quickget/catalog/data.sh
Include lib/quickget/catalog/info.sh
Include lib/quickget/catalog/arch.sh

setup() {
    TESTDIR=$(mktemp -d)
    FAKEBIN="$TESTDIR/bin"; mkdir -p "$FAKEBIN"
    cp "${SHELLSPEC_PROJECT_ROOT}/spec/fake-bin/curl" "$FAKEBIN/curl"
    chmod +x "$FAKEBIN/curl"
    PATH="$FAKEBIN:$PATH"
    FIXTURES="${SHELLSPEC_PROJECT_ROOT}/spec/fixtures"
    OS='' RELEASE='' EDITION='' ARCH=amd64 I18N='' PRETTY_OS_NAME=''
    OPERATION='' CHECK_ALL_ARCH=false CURL_INSECURE=false
}
cleanup() { rm -rf -- "$TESTDIR"; }
BeforeEach 'setup'
AfterEach 'cleanup'
web_pipe() { :; }
web_pipe_json() { printf '[]'; }
web_check() { return 0; }
web_redirect() { echo "$1"; }
zsync_get() { :; }
web_get() { echo "WG $1"; }
handle_curl_error() { return 0; }

# For each OS pick the newest static release; functions that need one
# bail gracefully on empty output.
Describe 'URL emission per OS'
    Parameters
        alma        9       dvd
        alpine      latest  ''
        android     9.0     x86_64
        antix       23.1    full-sysv
        archcraft   latest  ''
        archlinux   latest  ''
        artixlinux  latest  ''
        azurelinux  3.0     ''
        batocera    latest  ''
        bazzite     latest  gnome
        biglinux    latest  ''
        blendos     latest  ''
        bodhi       7.0     standard
        bunsenlabs  latest  ''
        cachyos     latest  kde
        centos-stream 10    ''
        chimeralinux latest ''
        debian      13      netinst
        deepin      latest  ''
        devuan      latest  ''
        dragonflybsd latest ''
        dragonos    latest  ''
        easyos      latest  ''
        elementary  latest  ''
        endeavouros latest  ''
        endless     latest  ''
        fedora      latest  Workstation
        freebsd     latest  ''
        freedos     latest  ''
        garuda      latest  ''
        gentoo      latest  ''
        ghostbsd    latest  ''
        gobolinux   latest  ''
        gnomeos     latest  ''
        guix        latest  ''
        haiku       latest  ''
        kali        latest  ''
        kdeneon     latest  ''
        kdelinux    latest  ''
    End
    It "get_$1 $2 $3 emits output"
        OS="$1" RELEASE="$2" EDITION="$3"
        When call "get_${1}"
        The status should be success
        The output should not be blank
    End
End

Describe 'get functions needing helpers'
    It 'get_alma emits a mirror URL'
        OS=alma RELEASE=9 EDITION=dvd
        When call get_alma
        The output should include 'almalinux'
    End
    It 'get_freedos emits a download URL'
        OS=freedos RELEASE=1.3
        When call get_freedos
        The output should include 'freedos'
    End
    It 'get_reactos emits a URL'
        OS=reactos RELEASE=latest
        When call get_reactos
        The output should include 'reactos'
    End
    It 'get_templeos emits a URL'
        OS=templeos RELEASE=latest
        When call get_templeos
        The output should include 'TempleOS'
    End
    It 'get_kolibrios emits a URL'
        OS=kolibrios RELEASE=latest EDITION=''
        When call get_kolibrios
        The output should include '.7z'
    End
End
End
