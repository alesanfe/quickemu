# shellcheck disable=SC2034
# Security tests: user-controlled values are validated before use.
Describe 'input validation boundaries'
Include lib/quickgetc/catalog/arch.sh
Include lib/quickemu/support/filesystem.sh

Describe 'set_arch whitelist'
    Parameters
        'x86_64;rm -rf /'
        '$(whoami)'
        'amd64|x86_64'
        'x86_64 && id'
        '../../../etc/passwd'
    End
    It "rejects injection-style arch $1"
        run_arch() { set_arch "$1"; }
        When run run_arch "$1"
        The status should be failure
        The output should include 'not a supported architecture'
    End
    It 'accepts real architectures'
        When call set_arch x86_64
        The variable ARCH should eq 'x86_64'
    End
    It 'accepts arm64'
        When call set_arch arm64
        The variable ARCH should eq 'aarch64'
    End
End

Describe 'qe_safe_rm refuses empty paths'
    It 'ignores empty input'
        When call qe_safe_rm ''
        The status should be success
    End
End
End
