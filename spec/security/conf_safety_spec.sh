# shellcheck disable=SC2034
# Security tests: conf parsing must never execute embedded commands,
# and helpers must reject pathological inputs.
Describe 'conf reading is injection-safe'
Include lib/quickemu/config/parser.sh
Include lib/quickemu/config/loader.sh

setup() {
    TESTDIR=$(mktemp -d)
    CANARY="$TESTDIR/pwned"
}
cleanup() { rm -rf -- "$TESTDIR"; }
BeforeEach 'setup'
AfterEach 'cleanup'

Describe 'conf_parse_value'
    It 'does not eval command substitution in values'
        When call conf_parse_value 'guest_os="linux";touch '"$CANARY"
        The output should include 'linux'
        The file "$CANARY" should not be exist
        The status should be success
    End
    It 'does not eval backticks or dollar expansion'
        When call conf_parse_value 'x="$(touch '"$CANARY"')"'
        The output should include 'touch'
        The file "$CANARY" should not be exist
        The status should be success
    End
    It 'strips surrounding quotes only'
        When call conf_parse_value 'disk_img="/tmp/vm/disk.qcow2"'
        The output should eq '/tmp/vm/disk.qcow2'
    End
    It 'returns the raw value when unquoted'
        When call conf_parse_value 'ram=4G'
        The output should eq '4G'
    End
End

Describe 'conf_get'
    It 'reads a key without executing the file'
        printf 'guest_os="linux"\nmalicious="`touch %s`"\n' "$CANARY" > "$TESTDIR/vm.conf"
        When call conf_get "$TESTDIR/vm.conf" guest_os
        The output should eq 'linux'
        The file "$CANARY" should not be exist
    End
    It 'fails cleanly on a missing key'
        printf 'guest_os="linux"\n' > "$TESTDIR/vm.conf"
        When call conf_get "$TESTDIR/vm.conf" nope
        The status should be failure
    End
    It 'fails cleanly on a missing file'
        When call conf_get "$TESTDIR/gone.conf" guest_os
        The status should be failure
    End
    It 'only matches keys at line start'
        printf 'xguest_os="fake"\nguest_os="real"\n' > "$TESTDIR/vm.conf"
        When call conf_get "$TESTDIR/vm.conf" guest_os
        The output should eq 'real'
    End
End
End
