# QE-14: formal acceptance scenarios — independent of implementation.
# These exercise the user-visible workflows end-to-end.
Describe 'uat: acceptance scenarios'
    setup() { UAT_DIR=$(mktemp -d); }
    cleanup() { rm -rf "${UAT_DIR}"; }
    BeforeEach 'setup'
    AfterEach 'cleanup'

    It 'uat-01: quickget --list returns a non-empty catalog'
        When run bash bin/quickget --list
        The status should be success
        The stdout should include 'ubuntu'
    End

    It 'uat-02: quickget rejects an unsupported OS'
        # quickget prints user-facing errors to stdout, not stderr.
        When run bash bin/quickget --download notarealos 1.0
        The status should be failure
        The output should include 'not a supported OS'
    End

    It 'uat-03: quickget rejects an unsupported release'
        # stderr carries a trust warning; stdout carries the rejection.
        When run bash bin/quickget --download debian 9999
        The status should be failure
        The output should include 'not a supported'
        The stderr should include 'integrity'
    End

    It 'uat-04: quickemu --version emits the version contract'
        When run bash bin/quickemu --version
        The status should be success
        The stdout should match pattern '*[0-9]*.[0-9]*'
    End

    It 'uat-05: --format json emits a valid envelope'
        When run bash bin/quickemu --capabilities --format json
        The status should be success
        The output should include '"schemaVersion":1'
    End

    It 'uat-06: invalid --minimum-trust value is rejected'
        # Unlike other rejections this one reports on stderr.
        When run bash bin/quickget --download --minimum-trust Z debian 13
        The status should be failure
        The stderr should include 'invalid trust level'
    End

    It 'uat-07: quickget --check on a non-OS does not crash'
        When run bash bin/quickget --check totallyfake 1.0
        The status should be failure
        The output should include 'not a supported OS'
    End
End
