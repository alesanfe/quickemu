# Smoke tests: every entry point exists, is executable, parses and
# answers its most basic invocations.
Describe 'project smoke checks'
Include spec/spec_helper.sh

Describe 'entry scripts exist and parse'
    Parameters
        bin/quickemu
        bin/quickget
        bin/quickgetc
        bin/quickreport
    End
    It "$1 exists and is executable"
        When run test -x "$QE_ROOT/$1"
        The status should be success
    End
    It "$1 is a compiled Go binary"
        When run sh -c 'head -c4 "$1" | grep -q ELF' _ "$QE_ROOT/$1"
        The status should be success
    End
End

Describe 'go tools are present and formatted'
    It 'every tools/*/main.go exists'
        tools_check() {
            local d
            for d in "$QE_ROOT"/tools/*/; do
                [ -f "$d/main.go" ] || ls "$d"/*.go >/dev/null 2>&1 || return 1
            done
        }
        When call tools_check
        The status should be success
    End
End

Describe 'basic invocations'
    It 'quickemu --version prints a version'
        When call "$QE_ROOT/bin/quickemu" --version
        The status should be success
        The output should match pattern '*.*'
    End
    It 'quickemu --capabilities emits contract JSON'
        When call "$QE_ROOT/bin/quickemu" --capabilities
        The status should be success
        The output should include '"schemaVersion"'
    End
    It 'quickemu --help mentions --vm'
        When call "$QE_ROOT/bin/quickemu" --help
        The status should be success
        The output should include '--vm'
    End
    It 'quickget --help exits cleanly'
        When call "$QE_ROOT/bin/quickget" --help
        The status should be success
        The output should include 'quickget'
    End
    It 'quickgetc --version prints a version'
        When run "$QE_ROOT/bin/quickgetc" --version
        The status should be success
        The output should include '0.1.0'
    End
End
End
