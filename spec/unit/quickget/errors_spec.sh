# shellcheck disable=SC2034
Describe 'quickget error reporting helpers'
Include lib/quickget/support/errors.sh
Include lib/quickget/catalog/info.sh
Include lib/quickget/catalog/arch.sh
Include lib/quickget/catalog/data.sh

setup() {
    OS=alpine RELEASE='' EDITION='' I18N='' I18NS=() RELEASES=''
}
BeforeEach 'setup'

# releases_* functions reach the network through these helpers; stub
# them so error paths are exercised hermetically.
web_pipe() { :; }
web_pipe_json() { :; }
web_check() { return 0; }

Describe 'error_specify_os'
    It 'lists every supported OS and exits 1'
        When run error_specify_os
        The status should be failure
        The output should include 'must specify an operating system'
        The output should include 'alpine'
        The output should include 'quickget'
    End
End

Describe 'error_specify_release'
    It 'shows releases and archs for a plain OS'
        OS=alpine
        When run error_specify_release
        The status should be failure
        The output should include 'must specify a release'
        The output should include 'Archs'
        The output should include 'amd64'
    End
    It 'shows editions when the OS has them'
        OS=debian
        When run error_specify_release
        The status should be failure
        The output should include 'Editions'
    End
    It 'shows releases and archs for ubuntu'
        OS=ubuntu
        When run error_specify_release
        The status should be failure
        The output should include 'Releases'
        The output should include 'Archs'
    End
    It 'shows releases and archs for ubuntu-server'
        OS=ubuntu-server
        When run error_specify_release
        The status should be failure
        The output should include 'Releases'
        The output should include 'Archs'
    End
    It 'shows releases and languages for windows'
        OS=windows
        releases_windows() { echo '11'; }
        languages_windows() { I18NS=('English International' 'Spanish'); }
        When run error_specify_release
        The status should be failure
        The output should include 'Languages'
        The output should include 'Spanish'
    End
End

Describe 'error_not_supported_release'
    It 'passes silently when the release is supported'
        OS=alpine RELEASE=3.20 RELEASES='3.19 3.20'
        releases_alpine() { echo '3.19 3.20'; }
        When call error_not_supported_release
        The status should be success
        The output should be blank
    End
    It 'fails listing releases when unsupported'
        OS=alpine RELEASE=9.9 RELEASES='3.19 3.20'
        releases_alpine() { echo '3.19 3.20'; }
        When run error_not_supported_release
        The status should be failure
        The output should include 'not a supported release'
        The output should include '3.20'
    End
End

Describe 'error_not_supported_lang'
    It 'lists the valid languages and exits 1'
        OS=windows I18N=Klingon I18NS=('English International' 'Spanish')
        When run error_not_supported_lang
        The status should be failure
        The output should include 'Klingon'
        The output should include 'not a supported'
        The output should include 'Spanish'
    End
End

Describe 'error_not_supported_argument'
    It 'points at the help flags and exits 1'
        When run error_not_supported_argument
        The status should be failure
        The output should include 'Not supported argument'
        The output should include 'quickget --help'
    End
End
End
