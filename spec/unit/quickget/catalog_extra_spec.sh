# shellcheck disable=SC2034
Describe 'quickget catalog validation extras'
Include lib/quickget/catalog/validate.sh
Include lib/quickget/catalog/info.sh
Include lib/quickget/catalog/arch.sh
Include lib/quickget/catalog/data.sh
Include lib/quickget/catalog/check.sh
Include lib/quickget/support/host.sh
Include lib/quickget/support/errors.sh

setup() {
    OS=alpine RELEASE=3.20 EDITION='' I18N='' I18NS=() ARCH=amd64
    NORMALISED_HOST_ARCH=amd64 OPERATION='' VM_PATH='' NRELEASE=''
    CHECK_ALL_ARCH=false
}
BeforeEach 'setup'
web_pipe() { :; }
web_pipe_json() { :; }
web_check() { return 0; }

Describe 'os_supported'
    It 'accepts a known OS and resolves the pretty name'
        OS=alpine
        When call os_supported
        The status should be success
        The variable PRETTY_OS_NAME should eq 'Alpine Linux'
    End
    It 'rejects an unknown OS listing the catalogue'
        OS=nosuchos
        When run os_supported
        The status should be failure
        The output should include 'not a supported OS'
        The output should include 'alpine'
    End
End

Describe 'is_valid_language'
    It 'accepts a listed language'
        I18NS=('English International' 'Spanish')
        When call is_valid_language 'Spanish'
        The status should be success
    End
    It 'rejects an unlisted language'
        I18NS=('English International')
        When call is_valid_language 'Klingon'
        The status should be failure
    End
End

Describe 'handle_missing'
    It 'passes for non-fedora OSes'
        OS=alpine RELEASE=3.20 EDITION=''
        When call handle_missing
        The status should be success
        The output should be blank
    End
    It 'rejects fedora Onyx before release 40'
        OS=fedora RELEASE=39 EDITION=Onyx
        When run handle_missing
        The status should be failure
        The output should include 'Unsupported combination'
    End
    It 'rejects fedora Sericea before release 40'
        OS=fedora RELEASE=39_Beta EDITION=Sericea
        When run handle_missing
        The status should be failure
        The output should include 'not available'
    End
    It 'allows fedora Onyx from release 40'
        OS=fedora RELEASE=40 EDITION=Onyx
        When call handle_missing
        The output should be blank
    End
End

Describe 'validate_release'
    It 'passes for a supported release'
        OS=alma RELEASE=9
        When call validate_release releases_alma
        The status should be success
    End
    It 'fails for an unsupported release'
        OS=alma RELEASE=7
        When run validate_release releases_alma
        The status should be failure
        The output should include 'not a supported release'
    End
    It 'uses the ubuntu generator for ubuntu flavours'
        OS=kubuntu RELEASE=24.10
        web_pipe() { printf '{"entries":[{"version":"24.10","status":"Supported"},{"version":"24.04","status":"Supported"}]}'; }
        When call validate_release releases_kubuntu
        The status should be success
    End
    It 'rejects a bad ubuntu flavour release'
        OS=kubuntu RELEASE=9.9
        web_pipe() { printf '{"entries":[{"version":"24.10","status":"Supported"},{"version":"24.04","status":"Supported"}]}'; }
        When run validate_release releases_kubuntu
        The status should be failure
        The output should include 'not a supported release'
        The output should include '24.10'
    End
End

Describe 'catalog_check_os_arch'
    It 'passes when the OS supports the arch'
        OS=debian ARCH=arm64
        When call catalog_check_os_arch debian 12 ''
        The status should be success
    End
    It 'exits 1 for unsupported arch outside test/show'
        OS=alpine ARCH=arm64 OPERATION=''
        arch_alpine() { :; }
        When run catalog_check_os_arch alpine 3.20 ''
        The status should be failure
        The output should include 'not available for arm64'
    End
    It 'skips politely in test mode'
        OS=windows ARCH=arm64 OPERATION=test
        test_result() { echo "TR:$1:$2:$5:$6"; }
        When run catalog_check_os_arch windows 11 ''
        The status should be success
        The output should include 'SKIP'
    End
    It 'defers to edition resolution when editions exist'
        OS=debian ARCH=mips OPERATION=''
        When call catalog_check_os_arch debian 12 ''
        The status should be success
    End
End

Describe 'catalog_resolve_edition'
    It 'sets EDITION and VM_PATH for a valid edition'
        OS=alma RELEASE=9 ARCH=amd64
        When call catalog_resolve_edition alma dvd
        The variable EDITION should eq 'dvd'
        The variable VM_PATH should eq 'alma-9-dvd'
    End
    It 'appends the arch suffix for foreign archs'
        OS=alma RELEASE=9 ARCH=arm64
        When call catalog_resolve_edition alma dvd
        The variable VM_PATH should eq 'alma-9-dvd-arm64'
    End
    It 'lists editions when none is given'
        OS=alma RELEASE=9
        When run catalog_resolve_edition alma ''
        The status should be failure
        The output should include 'must specify an edition'
        The output should include 'boot'
    End
    It 'rejects an unknown edition'
        OS=alma RELEASE=9
        When run catalog_resolve_edition alma bogus
        The status should be failure
        The output should include 'not a supported'
        The output should include 'dvd'
    End
    It 'skips politely in test mode on unsupported arch'
        OS=alma RELEASE=9 ARCH=arm64 OPERATION=test
        arch_alma() { echo amd64; }
        test_result() { echo "TR:$5"; }
        When run catalog_resolve_edition alma dvd
        The status should be success
        The output should include 'SKIP'
    End
End

Describe 'is_arch_supported / get_supported_archs'
    It 'defaults to amd64 without an arch function'
        When call is_arch_supported nixos amd64
        The status should be success
    End
    It 'rejects other archs without an arch function'
        When call is_arch_supported nixos arm64
        The status should be failure
    End
    It 'honours the OS arch function'
        arch_debian() { echo 'amd64 arm64'; }
        When call is_arch_supported debian arm64
        The status should be success
    End
    It 'get_supported_archs defaults to amd64'
        When call get_supported_archs nixos
        The output should eq 'amd64'
    End
    It 'get_supported_archs echoes the arch function'
        arch_debian() { echo 'amd64 arm64'; }
        When call get_supported_archs debian
        The output should eq 'amd64 arm64'
    End
End

Describe 'catalog_update_check'
    It 'reports the newest release and local dirs'
        releases_alma() { echo '9 8'; }
        TESTDIR=$(mktemp -d)
        mkdir -p "$TESTDIR/alma-8" "$TESTDIR/alma-9" "$TESTDIR/other-1"
        in_dir() { ( cd "$1" && shift && "$@" ); }
        When run in_dir "$TESTDIR" catalog_update_check alma
        The status should be success
        The output should include 'Latest alma release upstream: 9'
        The output should include 'alma-9'
        The output should include 'up to date'
        The output should include 'older release available'
        rm -rf "$TESTDIR"
    End
    It 'reports when no local dirs exist'
        releases_alma() { echo '9 8'; }
        TESTDIR=$(mktemp -d)
        in_dir() { ( cd "$1" && shift && "$@" ); }
        When run in_dir "$TESTDIR" catalog_update_check alma
        The status should be success
        The output should include 'No local alma VM directories'
        rm -rf "$TESTDIR"
    End
End

Describe 'catalog_check_all_arch'
    It 'tests each supported arch'
        releases_alma() { echo '9'; }
        get_alma() { echo 'https://x/alma.iso deadbeef'; }
        OPERATION=test
        When run catalog_check_all_arch alma 9 dvd
        The output should include 'alma-9'
    End
End

Describe 'ubuntu_daily_codename'
    It 'returns the first series hosting daily images'
        web_pipe() { printf '{"entries":[{"name":"questing"},{"name":"plucky"}]}'; }
        web_check() { return 0; }
        When call ubuntu_daily_codename ubuntu
        The output should eq 'questing'
    End
    It 'skips series without daily images'
        web_pipe() { printf '{"entries":[{"name":"questing"},{"name":"plucky"}]}'; }
        web_check() { case "$1" in *plucky*) return 0;; *) return 1;; esac; }
        When call ubuntu_daily_codename ubuntu
        The output should eq 'plucky'
    End
End
End
