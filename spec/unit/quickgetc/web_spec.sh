# shellcheck disable=SC2034
Describe 'quickgetc download transport'
Include lib/quickgetc/download/web.sh

setup() {
    TESTDIR=$(mktemp -d)
    FAKE_BIN="$TESTDIR/bin"; mkdir -p "$FAKE_BIN"
    VM_PATH="$TESTDIR/vm"; mkdir -p "$VM_PATH"
    OPERATION=download OS=os RELEASE=1 EDITION=''
    declare -gA IMAGES=()
    test_result() { :; }
}
cleanup() { rm -rf -- "$TESTDIR"; }
BeforeEach 'setup'
AfterEach 'cleanup'

Describe 'check_hash'
    It 'verifies a sha256 checksum'
        printf 'data'  > "$TESTDIR/f.iso"
        H=$(sha256sum "$TESTDIR/f.iso" | cut -d' ' -f1)
        When call check_hash "$TESTDIR/f.iso" "$H"
        The status should be success
        The output should include 'Good!'
    End
    It 'exits on a wrong checksum'
        printf 'data'  > "$TESTDIR/f.iso"
        H=$(printf '0%.0s' $(seq 64))
        When run check_hash "$TESTDIR/f.iso" "$H"
        The status should be failure
        The output should include "doesn't match"
    End
    It 'warns and skips unguessable hash lengths'
        When call check_hash "$TESTDIR/f.iso" abc123
        The status should be success
        The output should include "Can't guess hash algorithm"
    End
    It 'uses md5 for 32-char hashes'
        printf 'data'  > "$TESTDIR/m.iso"
        H=$(md5sum "$TESTDIR/m.iso" | cut -d' ' -f1)
        When call check_hash "$TESTDIR/m.iso" "$H"
        The output should include 'md5sum'
        The output should include 'Good!'
    End
End

Describe 'web_redirect / web_check'
    It 'web_redirect returns the effective URL when it differs'
        printf '#!/bin/sh\necho https://cdn.example/final.iso\n' >"$FAKE_BIN/curl"; chmod +x "$FAKE_BIN/curl"
        PATH="$FAKE_BIN:$PATH"
        When call web_redirect https://a/x.iso
        The output should eq 'https://cdn.example/final.iso'
    End
    It 'web_redirect returns the original URL when unchanged'
        printf '#!/bin/sh\necho "https://a/x.iso"\n' >"$FAKE_BIN/curl"; chmod +x "$FAKE_BIN/curl"
        PATH="$FAKE_BIN:$PATH"
        When call web_redirect https://a/x.iso
        The output should eq 'https://a/x.iso'
    End
    It 'web_check succeeds when curl succeeds'
        printf '#!/bin/sh\nexit 0\n' >"$FAKE_BIN/curl"; chmod +x "$FAKE_BIN/curl"
        PATH="$FAKE_BIN:$PATH"
        When call web_check https://a/x.iso
        The status should be success
    End
    It 'web_check fails when curl fails'
        printf '#!/bin/sh\nexit 22\n' >"$FAKE_BIN/curl"; chmod +x "$FAKE_BIN/curl"
        PATH="$FAKE_BIN:$PATH"
        When call web_check https://a/x.iso
        The status should be failure
    End
End

Describe 'web_get'
    It 'downloads through curl in download mode'
        cat >"$FAKE_BIN/curl" <<EOF
#!/bin/sh
echo ok >"$TESTDIR/vm/f.iso"
EOF
        chmod +x "$FAKE_BIN/curl"; PATH="$FAKE_BIN:$PATH"
        When call web_get https://a/f.iso "$VM_PATH" f.iso
        The status should be success
        The file "$VM_PATH/f.iso" should be exist
    End
    It 'reports test mode through test_result and exits'
        OPERATION=test
        test_result() { echo "TR $1 $2 $4 $5"; }
        printf '#!/bin/sh\necho ok; exit 0\n' >"$FAKE_BIN/curl"; chmod +x "$FAKE_BIN/curl"
        PATH="$FAKE_BIN:$PATH"
        When run web_get https://a/f.iso "$VM_PATH" f.iso
        The status should be success
        The output should include 'TR os 1'
        The output should include 'PASS'
    End
End

Describe 'handle_sources / handle_source_type / handle_websource'
    It 'routes web sources through web_get and records IMAGES'
        web_get() { touch "$VM_PATH/$3"; }
        check_hash() { echo "HASHCHECK $1"; }
        ENTRY='{}' IMAGE_TYPES=(iso)
        SOURCE='{"web":{"url":"https://a/x.iso","file_name":"x.iso","checksum":"abc123"}}'
        When call handle_source_type iso "$SOURCE"
        The output should include 'HASHCHECK x.iso'
        The variable "IMAGES[iso]" should include 'x.iso'
    End
    It 'routes archive formats through the right decompressor'
        web_get() { :; }
        xz() { touch "${2/.xz/}"; }
        SOURCE='{"web":{"url":"https://a/x.iso.xz","file_name":"x.iso.xz","archive_format":"xz"}}'
        When call handle_source_type iso "$SOURCE"
        The variable "IMAGES[iso]" should include 'x.iso'
        The variable "IMAGES[iso]" should not include '.xz'
    End
    It 'records file_name sources directly'
        SOURCE='{"file_name":"local.iso"}'
        When call handle_source_type iso "$SOURCE"
        The variable "IMAGES[iso]" should eq "$VM_PATH/local.iso"
    End
    It 'rejects custom sources'
        When run handle_source_type iso '{"custom":{}}'
        The status should be failure
        The output should include 'Custom source not implemented'
    End
    It 'iterates IMAGE_TYPES in handle_sources'
        web_get() { :; }
        ENTRY='{"iso":[{"web":{"url":"https://a/x.iso","file_name":"x.iso"}}],"img":[null]}'
        IMAGE_TYPES=(iso img)
        When call handle_sources
        The variable "IMAGES[iso]" should include 'x.iso'
    End
End
End
