Describe 'lib/config/loader.sh'
Include lib/quickemu/config/parser.sh
Include lib/quickemu/config/loader.sh

setup() {
    TESTD=$(mktemp -d)
    printf 'guest_os="linux"\nram="4G"\n' >"$TESTD/vm.conf"
}
Before 'setup'

It 'reads a key=value entry without executing the file'
When call conf_get "$TESTD/vm.conf" guest_os
The output should equal 'linux'
End

It 'fails silently for a missing key'
When call conf_get "$TESTD/vm.conf" nope
The status should be failure
End
End
