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
    It "$1 passes bash -n"
        When run bash -n "$QE_ROOT/$1"
        The status should be success
    End
End

Describe 'library files parse'
    It 'all lib/*.sh pass bash -n'
        lib_check() {
            local f s=0
            while IFS= read -r f; do
                bash -n "$f" || s=1
            done < <(find "$QE_ROOT/lib" -name '*.sh')
            return "$s"
        }
        When call lib_check
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
        v() { ( cd "$TMPDIRP" 2>/dev/null || cd /tmp; export XDG_CACHE_HOME="$TESTCACHE"; export PATH="$TESTBIN:$PATH"; . "$QE_ROOT/bin/quickgetc" --version ); }
        TESTCACHE=$(mktemp -d); TESTBIN=$(mktemp -d)
        printf '[]' > "$TESTCACHE/quickget_data.json.zst"
        printf '#!/bin/sh\nwhile [ $# -gt 1 ]; do shift; done\nexec cat "$1"\n' > "$TESTBIN/zstd"
        printf '#!/bin/sh\nexit 0\n' > "$TESTBIN/curl"; printf '#!/bin/sh\nexit 0\n' > "$TESTBIN/qemu-img"
        chmod +x "$TESTBIN"/*
        When run v
        The status should be success
        The output should include '0.1.0'
        rm -rf "$TESTCACHE" "$TESTBIN"
    End
End
End
