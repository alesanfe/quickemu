# shellcheck disable=SC2034
Describe 'quickgetc download cache and vm config'
Include lib/quickgetc/download/cache.sh
Include lib/quickgetc/vm/config.sh
Include lib/quickgetc/support/host.sh
Include lib/quickgetc/cli/usage.sh

setup() {
    TESTDIR=$(mktemp -d)
    CACHE_FILE='quickget_data.json.zst'
    CACHE_URL='https://example/data.json.zst'
    XDG_CACHE_HOME="$TESTDIR/cache"
    mkdir -p "$XDG_CACHE_HOME" "$TESTDIR/bin"
    OPERATION='' VM_PATH="$TESTDIR/testos-1.0" OS=testos RELEASE=1.0 EDITION=''
    declare -gA IMAGES=()
    ENTRY='{"guest_os":"linux","arch":"x86_64","ram":4294967296}'
}
cleanup() { qe_spec_cleanup; }
BeforeEach 'setup'
AfterEach 'cleanup'

Describe 'cache_is_valid'
    It 'rejects a missing cache file'
        When call cache_is_valid "$TESTDIR/nope.zst"
        The status should be failure
    End
    It 'accepts a file written today'
        printf 'data' > "$TESTDIR/today.zst"
        When call cache_is_valid "$TESTDIR/today.zst"
        The status should be success
    End
    It 'rejects a stale cache file'
        printf 'data' > "$TESTDIR/old.zst"
        touch -d '3 days ago' "$TESTDIR/old.zst"
        When call cache_is_valid "$TESTDIR/old.zst"
        The status should be failure
    End
End

Describe 'populate_cache'
    It 'decompresses a valid same-day cache'
        printf '{"os":1}' > "$XDG_CACHE_HOME/$CACHE_FILE"
        zstd() { shift; cat "$1"; }
        When call populate_cache
        The variable JSON should eq '{"os":1}'
    End
    It 'downloads the cache when missing or stale'
        web_get() { printf '{"dl":true}' > "$2/$3"; }
        zstd() { shift; cat "$1"; }
        When call populate_cache
        The output should include 'Downloading quickget data'
        The variable JSON should eq '{"dl":true}'
    End
    It 'removes an undecodable cache and exits'
        printf 'garbage' > "$XDG_CACHE_HOME/$CACHE_FILE"
        zstd() { return 1; }
        When run populate_cache
        The status should be failure
        The output should include 'Failed to decompress'
        The file "$XDG_CACHE_HOME/$CACHE_FILE" should not be exist
    End
End

Describe 'make_vm_config'
    It 'writes a conf file with guest_os, disk and arch'
        IMAGES[iso]="$VM_PATH/t.iso"
        mkdir -p "$VM_PATH"
        When call make_vm_config
        The output should include 'Making'
        The file "$VM_PATH.conf" should be exist
        The contents of file "$VM_PATH.conf" should include 'guest_os="linux"'
        The contents of file "$VM_PATH.conf" should include 'disk_img='
        The contents of file "$VM_PATH.conf" should include 'arch_vm="x86_64"'
        The contents of file "$VM_PATH.conf" should include 'iso='
        The contents of file "$VM_PATH.conf" should include 'ram=4'
    End
    It 'does not overwrite an existing conf'
        printf 'guest_os="custom"\n' > "$TESTDIR/testos-1.0.conf"
        When call make_vm_config
        The contents of file "$VM_PATH.conf" should include 'guest_os="custom"'
        The contents of file "$VM_PATH.conf" should not include 'arch_vm'
    End
    It 'is a no-op in download mode'
        OPERATION=download
        When run make_vm_config
        The status should be success
        The output should be blank
    End
End

Describe 'create_vm'
    It 'handles sources then writes the config'
        web_get() { :; }
        handle_sources() { IMAGES[iso]="$VM_PATH/t.iso"; HANDLED=1; }
        mkdir -p "$VM_PATH"
        When call create_vm
        The output should include 'Making'
        The variable HANDLED should eq 1
        The file "$VM_PATH.conf" should be exist
    End
End

Describe 'cleanup / help_message'
    It 'cleanup terminates background jobs'
        sleep 60 &
        When call cleanup
        The status should be success
    End
    It 'help_message prints usage and the OS list'
        QUICKGETC_VERSION='0.1.0' CURL_VERSION='8.0'
        os_support() { echo testos otheros; }
        tput() { echo 80; }
        When call help_message
        The output should include 'quickgetc <os> <release>'
        The output should include '--download'
        The output should include 'testos'
    End
End
End
