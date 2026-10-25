# shellcheck disable=SC2034
Describe 'download transport and checksum helpers'
Include lib/quickget/download/web.sh

setup() {
    TESTDIR=$(mktemp -d)
    FAKE_BIN="$TESTDIR/bin"
    mkdir -p "$FAKE_BIN"
    OPERATION='' VM_PATH="$TESTDIR" HOST_OS=Linux CURL_INSECURE=''
    OS=linux RELEASE=1 EDITION='' ARCH=x86_64 PRETTY_OS_NAME=''
    NO_ARIA2='' CURL_LOG="$TESTDIR/curl.log"
}
cleanup() { rm -rf -- "$TESTDIR"; }
BeforeEach 'setup'
AfterEach 'cleanup'

in_subshell() { ( "$@" ); }

# Fake curl records argv and optionally fails.
fake_curl() {
    cat >"$FAKE_BIN/curl" <<EOF
#!/bin/sh
printf '%s\n' "\$@" >>"$CURL_LOG"
${1:-exit 0}
EOF
    chmod +x "$FAKE_BIN/curl"
    PATH="$FAKE_BIN:$PATH"
}

Describe 'check_hash algorithm selection'
It 'honours an explicit algorithm prefix'
    OPERATION=download
    printf 'data' >"$TESTDIR/f.iso"
    H=$(sha256sum "$TESTDIR/f.iso" | cut -d' ' -f1)
    When call check_hash "$TESTDIR/f.iso" "SHA256:$H"
    The status should be success
    The output should include 'Good'
End
Parameters
    md5sum md5 32
    sha1sum sha1 40
    sha512sum sha512 128
End
It 'accepts the $2 prefix'
    OPERATION=download
    printf 'data' >"$TESTDIR/f.iso"
    H=$($1 "$TESTDIR/f.iso" | cut -d' ' -f1)
    When call check_hash "$TESTDIR/f.iso" "$2:$H"
    The status should be success
    The output should include 'Good'
End
End

Describe 'check_hash length guessing'
It 'guesses md5 for 32-char hashes'
    OPERATION=download
    printf 'data' >"$TESTDIR/f.iso"
    H=$(md5sum "$TESTDIR/f.iso" | cut -d' ' -f1)
    When call check_hash "$TESTDIR/f.iso" "$H"
    The status should be success
    The output should include 'md5sum'
    The output should include 'Good'
End
It 'guesses sha256 for 64-char hashes'
    OPERATION=download
    printf 'data' >"$TESTDIR/f.iso"
    H=$(sha256sum "$TESTDIR/f.iso" | cut -d' ' -f1)
    When call check_hash "$TESTDIR/f.iso" "$H"
    The status should be success
    The output should include 'sha256sum'
End
It 'warns and skips unknown prefixes'
    When call check_hash x.iso 'crc32:abc'
    The status should be success
    The output should include 'Unknown hash algorithm'
End
It 'warns and skips unguessable lengths'
    When call check_hash x.iso 'abcde'
    The status should be success
    The output should include "Can't guess hash algorithm"
End
It 'exits on a checksum mismatch'
    printf 'data' >"$TESTDIR/f.iso"
    OPERATION=download
    printf 'data' >"$TESTDIR/f.iso"
    When call in_subshell check_hash "$TESTDIR/f.iso" "$(printf '0%.0s' {1..64})"
    The status should be failure
    The output should include "doesn't match"
End
It 'joins the iso to VM_PATH outside download operation'
    OPERATION=verify
    mkdir -p "$VM_PATH" && printf 'data' >"$VM_PATH/f.iso"
    H=$(sha256sum "$VM_PATH/f.iso" | cut -d' ' -f1)
    When call check_hash f.iso "$H"
    The status should be success
    The output should include 'Good'
End
End

Describe 'check_hash on macOS'
It 'uses gsha256sum when GNU coreutils is installed'
    HOST_OS=Darwin
    OPERATION=download
    printf 'data' >"$TESTDIR/f.iso"
    H=$(sha256sum "$TESTDIR/f.iso" | cut -d' ' -f1)
    printf '#!/bin/sh\nexec sha256sum "$@"\n' >"$FAKE_BIN/gsha256sum"
    chmod +x "$FAKE_BIN/gsha256sum"
    PATH="$FAKE_BIN:$PATH"
    When call check_hash "$TESTDIR/f.iso" "sha256:$H"
    The status should be success
    The output should include 'gsha256sum'
End
It 'falls back to shasum without GNU coreutils'
    HOST_OS=Darwin
    OPERATION=download
    printf 'data' >"$TESTDIR/f.iso"
    H=$(sha256sum "$TESTDIR/f.iso" | cut -d' ' -f1)
    command() { case "$2" in g*) return 1;; esac; builtin command "$@"; }
    printf '#!/bin/sh\nshift 2\nexec sha256sum "$@"\n' >"$FAKE_BIN/shasum"
    chmod +x "$FAKE_BIN/shasum"
    PATH="$FAKE_BIN:$PATH"
    When call check_hash "$TESTDIR/f.iso" "sha256:$H"
    The status should be success
    The output should include 'shasum -a 256'
End
It 'warns that b2sum needs GNU coreutils on macOS'
    HOST_OS=Darwin
    command() { [ "$2" = 'gb2sum' ] && return 1; builtin command "$@"; }
    When call check_hash x.iso 'b2sum:abc'
    The status should be success
    The output should include 'b2sum not available on macOS'
End
End

Describe 'web_pipe and web_pipe_json'
It 'fetches a URL to stdout with silent curl'
    fake_curl 'echo PAYLOAD'
    When call web_pipe 'https://x/f'
    The status should be success
    The output should eq 'PAYLOAD'
    The contents of file "$CURL_LOG" should include '--location'
End
It 'requests JSON with an Accept header'
    fake_curl 'echo {}'
    When call web_pipe_json 'https://x/api'
    The status should be success
    The output should eq '{}'
    The contents of file "$CURL_LOG" should include 'Accept: application/json'
End
End

Describe 'web_check and web_redirect'
It 'checks URL reachability with a HEAD request'
    fake_curl
    When call web_check 'https://x/f' --header 'A: b'
    The status should be success
    The contents of file "$CURL_LOG" should include '--head'
    The contents of file "$CURL_LOG" should include 'A: b'
End
It 'propagates check failures'
    fake_curl 'exit 22'
    When call web_check 'https://x/dead'
    The status should be failure
End
It 'returns the effective URL after redirection'
    fake_curl 'echo https://x/final'
    When call web_redirect 'https://x/first'
    The status should be success
    The output should eq 'https://x/final'
End
It 'returns the original URL when there is no redirect'
    fake_curl 'echo https://x/same'
    When call web_redirect 'https://x/same'
    The status should be success
    The output should eq 'https://x/same'
End
End

Describe 'web_get'
It 'reports the resolved URL in show mode and exits'
    OPERATION=show
    test_result() { echo "TEST:$4"; }
    fake_curl 'echo https://x/final'
    When call in_subshell web_get 'https://x/first' "$TESTDIR"
    The status should be success
    The output should eq 'TEST:https://x/final'
End
It 'checks reachability in test mode'
    OPERATION="test"
    test_result() { echo "TEST:$4 $5"; }
    is_arch_supported() { return 0; }
    fake_curl 'case "$*" in *url_effective*) echo "$4";; *) exit 0;; esac'
    When call in_subshell web_get 'https://x/f' "$TESTDIR"
    The status should be success
    The output should include 'PASS'
End
It 'skips the check for unsupported architectures'
    OPERATION="test" ARCH=arm64
    test_result() { echo "TEST:$5 $6"; }
    is_arch_supported() { return 1; }
    fake_curl 'echo https://x/f'
    When call in_subshell web_get 'https://x/f' "$TESTDIR"
    The status should be success
    The output should include 'SKIP'
End
It 'downloads with curl when aria2c is absent'
    OPERATION=''
    fake_curl 'exit 0'
    When call web_get 'https://x/file.iso' "$TESTDIR" out.iso
    The status should be success
    The output should include 'Downloading linux'
    The contents of file "$CURL_LOG" should include 'file.iso'
End
It 'removes the partial file and exits when curl fails'
    OPERATION=''
    fake_curl 'case "$*" in *url_effective*) echo "$4";; *--output*) touch "$4"; exit 22;; esac'
    When call in_subshell web_get 'https://x/file.iso' "$TESTDIR" out.iso
    The status should be failure
    The output should include 'Failed to download'
End
End

Describe 'zsync_get'
It 'falls back to web_get when zsync is absent'
    command() { [ "$2" = 'zsync' ] && return 1; builtin command "$@"; }
    fake_curl 'echo https://x/f'
    When call zsync_get 'https://x/f' "$TESTDIR" out.iso
    The status should be success
    The output should include 'falling back to curl'
End
It 'runs zsync against the .zsync URL when present'
    ZSYNC_LOG="$TESTDIR/zsync.log"
    printf '#!/bin/sh\nprintf "%%s\n" "$@" >>"%s"\n' "$ZSYNC_LOG" >"$FAKE_BIN/zsync"
    chmod +x "$FAKE_BIN/zsync"
    PATH="$FAKE_BIN:$PATH"
    When call zsync_get 'https://x/f.iso' "$TESTDIR" out.iso
    The status should be success
    The output should include 'Downloading'
    The contents of file "$ZSYNC_LOG" should include 'http://x/f.iso.zsync'
End
It 'exits when the zsync transfer fails'
    printf '#!/bin/sh\nexit 3\n' >"$FAKE_BIN/zsync"
    chmod +x "$FAKE_BIN/zsync"
    PATH="$FAKE_BIN:$PATH"
    When call in_subshell zsync_get 'https://x/f.iso' "$TESTDIR" out.iso
    The status should be failure
    The output should include 'Failed to download'
End
It 'reports the URL in show mode without downloading'
    OPERATION=show
    test_result() { echo "TEST:$4"; }
    When call in_subshell zsync_get 'https://x/f.iso' "$TESTDIR"
    The status should be success
    The output should eq 'TEST:https://x/f.iso'
End
End

Describe 'handle_curl_error'
Parameters
    6 'Failed to resolve' 2
    7 'Failed to contact' 1
    22 'failing HTTP status' 1
    23 'writing Windows media' 2
    26 'out of memory' 2
    126 'Curl command not found' 2
End
It 'maps curl error $1 to a message and status $3'
    When call in_subshell handle_curl_error "$1"
    The status should eq "$3"
    The output should include "$2"
End
It 'reports SIGINT kills as interruptions'
    When call in_subshell handle_curl_error 130
    The status should be failure
    The output should include 'interrupted'
End
End
End
