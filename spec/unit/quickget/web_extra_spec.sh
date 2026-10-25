# shellcheck disable=SC2034
Describe 'quickget download transport extras'
Include lib/quickget/download/web.sh

setup() {
    TESTDIR=$(mktemp -d)
    FAKEBIN="$TESTDIR/bin"; mkdir -p "$FAKEBIN"
    cp "${SHELLSPEC_PROJECT_ROOT}/spec/fake-bin/curl" "$FAKEBIN/curl"
    chmod +x "$FAKEBIN/curl"
    PATH="$FAKEBIN:$PATH"
    FIXTURES="${SHELLSPEC_PROJECT_ROOT}/spec/fixtures"
    OPERATION='' OS=testos RELEASE=1 EDITION='' ARCH=amd64 CURL_INSECURE=false
    PRETTY_OS_NAME='TestOS'
    test_result() { echo "TR $1 $2 ${3:-} $4 ${5:-} ${6:-}"; }
    is_arch_supported() { return 0; }
}
cleanup() { rm -rf -- "$TESTDIR"; }
BeforeEach 'setup'
AfterEach 'cleanup'

Describe 'web_pipe / web_pipe_json'
    It 'pipes a fixture URL to stdout'
        printf '#!/bin/sh\necho PIPED\n' > "$FAKEBIN/curl"
        When call web_pipe https://x/y
        The output should eq 'PIPED'
    End
    It 'pipes JSON with an Accept header'
        printf '#!/bin/sh\necho JSONED\n' > "$FAKEBIN/curl"
        When call web_pipe_json https://x/y
        The output should eq 'JSONED'
    End
End

Describe 'zsync_get'
    It 'reports the URL in show mode and exits'
        OPERATION=show
        When run zsync_get https://x/f.iso "$TESTDIR"
        The status should be success
        The output should include 'TR testos 1'
        The output should include 'https://x/f.iso'
    End
    It 'skips when the arch is unsupported in test mode'
        OPERATION=test
        is_arch_supported() { return 1; }
        When run zsync_get https://x/f.iso "$TESTDIR"
        The status should be success
        The output should include 'SKIP'
    End
    It 'reports PASS/FAIL in test mode'
        OPERATION=test
        web_check() { return 0; }
        When run zsync_get https://x/f.iso "$TESTDIR"
        The output should include 'PASS'
    End
    It 'uses zsync when available'
        cat > "$FAKEBIN/zsync" <<'EOF'
#!/bin/sh
while [ $# -gt 1 ]; do shift; done
touch "$1"
exit 0
EOF
        chmod +x "$FAKEBIN/zsync"
        When call zsync_get https://x/f.iso "$TESTDIR"
        The output should include 'Downloading'
        The file "$TESTDIR/f.iso" should be exist
    End
    It 'reports a zsync failure'
        printf '#!/bin/sh\nexit 3\n' > "$FAKEBIN/zsync"
        chmod +x "$FAKEBIN/zsync"
        When run zsync_get https://x/f.iso "$TESTDIR"
        The status should be failure
        The output should include 'Failed to download'
    End
    It 'falls back to web_get without zsync'
        mkdir "$TESTDIR/empty-bin"
        no_zsync() { ( PATH="$TESTDIR/empty-bin"; web_get() { echo "FELLBACK $1 $2 ${3:-}"; }; zsync_get "$@" ); }
        When call no_zsync https://x/f.iso "$TESTDIR" out.iso
        The output should include 'INFO: zsync not found'
        The output should include 'FELLBACK https://x/f.iso'
    End
End

Describe 'handle_curl_error'
    Parameters
        6 'Failed to resolve'      2
        7 'Failed to contact'      1
        8 'malformed HTTP'         1
        22 'failing HTTP status'   1
    End
    It "maps curl error $1 to a message and status $3"
        When run handle_curl_error "$1"
        The status should eq "$3"
        The output should include "$2"
    End
    It 'reports interruption for signal-derived codes'
        When run handle_curl_error 130
        The status should be failure
        The output should include 'interrupted'
    End
End

Describe 'web_get extra branches'
    It 'honours the file argument and header passthrough'
        When run web_get --header 'X-Test: 1' https://cdn/x.iso "$TESTDIR" out.iso
        The status should be success
        The output should include 'Downloading TestOS'
    End
    It 'reports a curl failure'
        printf '#!/bin/sh\nexit 1\n' > "$FAKEBIN/curl"
        When run web_get https://cdn/x.iso "$TESTDIR"
        The status should be failure
        The output should include 'Failed to download'
    End
    It 'uses --insecure when CURL_INSECURE is set'
        CURL_INSECURE=true
        printf '#!/bin/sh\nfor a in "$@"; do [ "$a" = "--insecure" ] && { echo INSECURE-USED; exit 1; }; done\nexit 1\n' > "$FAKEBIN/curl"
        When run web_get https://cdn/x.iso "$TESTDIR"
        The status should be failure
        The output should include 'INSECURE-USED'
    End
    It 'skips the unsupported arch in test mode'
        OPERATION=test
        is_arch_supported() { return 1; }
        When run web_get https://cdn/x.iso "$TESTDIR"
        The status should be success
        The output should include 'SKIP'
    End
End
End
