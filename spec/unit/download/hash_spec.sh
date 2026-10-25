Describe 'download checksum verification'
Include lib/quickget/download/web.sh

setup() {
    TESTDIR=$(mktemp -d)
    cd "$TESTDIR"
    printf 'quickemu' > payload.iso
    OPERATION=create
    VM_PATH="$TESTDIR"
    OS=test
    RELEASE=1
    EDITION=''
    HOST_OS=Linux
    MD5=$(md5sum payload.iso | cut -d' ' -f1)
    SHA1=$(sha1sum payload.iso | cut -d' ' -f1)
    SHA256=$(sha256sum payload.iso | cut -d' ' -f1)
    SHA512=$(sha512sum payload.iso | cut -d' ' -f1)
    B2=$(b2sum payload.iso | cut -d' ' -f1)
}
cleanup() { cd /; rm -f -- "$TESTDIR"/*; rmdir -- "$TESTDIR"; }
BeforeEach 'setup'
AfterEach 'cleanup'

Describe 'algorithm detection by length'
Parameters
    md5 sha1 sha256 sha512
End
It 'accepts a correct bare hash'
    HASH_VAR=$(printf '%s' "$1" | tr '[:lower:]' '[:upper:]')
    eval "EXPECTED=\${$HASH_VAR}"
    When call check_hash payload.iso "$EXPECTED"
    The status should be success
    The output should include 'Good!'
End
It 'rejects a corrupted file for a bare hash'
    HASH_VAR=$(printf '%s' "$1" | tr '[:lower:]' '[:upper:]')
    eval "EXPECTED=\${$HASH_VAR}"
    printf 'tampered' > payload.iso
    When run check_hash payload.iso "$EXPECTED"
    The status should eq 1
    The output should include "doesn't match"
End
End

Describe 'explicit algorithm prefixes'
Parameters
    md5
    sha1
    sha256
    sha512
    b2sum
    blake2b
End
It 'honours an explicit case-insensitive prefix'
    case "$1" in
        md5) H=$MD5;;
        sha1) H=$SHA1;;
        sha256) H=$SHA256;;
        sha512) H=$SHA512;;
        *) H=$B2;;
    esac
    PREFIX=$(printf '%s' "$1" | tr '[:lower:]' '[:upper:]')
    When call check_hash payload.iso "${PREFIX}:$H"
    The status should be success
    The output should include 'Good!'
End
End

It 'warns and skips an unknown prefixed algorithm'
    When call check_hash payload.iso 'rot13:abc'
    The status should be success
    The output should include 'Unknown hash algorithm'
End
It 'warns and skips a hash of unusual length'
    When call check_hash payload.iso 'abc123'
    The status should be success
    The output should include "Can't guess hash algorithm"
End
It 'verifies a file inside the VM directory'
    When call check_hash payload.iso "$SHA256"
    The status should be success
    The output should include 'Good!'
End
End
