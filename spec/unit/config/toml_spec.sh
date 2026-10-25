# shellcheck shell=bash
Describe 'toml config'
Include lib/quickemu/config/parser.sh
Include lib/quickemu/config/loader.sh
Include lib/quickemu/config/toml.sh

in_subshell() { ( "$@" ); }
echo_loaded() { echo "${guest_os:-unset} ${ram:-unset}"; }

setup() { TESTDIR=$(mktemp -d); }
cleanup() { rm -rf -- "$TESTDIR"; }
BeforeEach 'setup'
AfterEach 'cleanup'

    It 'parses sections, strings, ints, bools and arrays'
        cat >"$TESTDIR/vm.toml" <<'T'
formatVersion = 1
guest_os = "windows"
ram = "8G"
[net]
bridge = "br0"
T
        When call toml_parse "$TESTDIR/vm.toml"
        The output should include 'guest_os'
        The output should include 'windows'
        The output should include 'net.bridge'
        The status should be success
    End
    It 'rejects a malformed file'
        printf 'this is not toml\n' >"$TESTDIR/bad.toml"
        When call toml_parse "$TESTDIR/bad.toml"
        The status should be failure
    End
    It 'migrates a legacy conf without executing it'
        printf 'guest_os="linux"\nram="4G"\ndisk_img="/d/x.qcow2"\n' >"$TESTDIR/vm1.conf"
        When call in_subshell conf_migrate "$TESTDIR/vm1.conf" "$TESTDIR/vm1.toml"
        The status should be success
        The output should include 'Migrated' 
        The contents of file "$TESTDIR/vm1.toml" should include 'guest_os = "linux"'
        The contents of file "$TESTDIR/vm1.toml" should include 'ram = "4G"'
    End
End

Describe 'vm_config_load'
Include lib/quickemu/config/parser.sh
Include lib/quickemu/config/loader.sh
Include lib/quickemu/config/toml.sh
    echo_loaded() { echo "${guest_os:-unset} ${ram:-unset}"; }
    It 'loads a toml config without executing it'
        D=$(mktemp -d)
        printf 'guest_os = "windows"\nram = "8G"\n' >"$D/vm1.toml"
        vm_config_load "$D/vm1.toml"
        When call echo_loaded
        The output should include 'windows 8G'
        rm -rf "$D"
    End
    It 'rejects a malformed toml'
        D=$(mktemp -d)
        printf 'not valid\n' >"$D/bad.toml"
        When call vm_config_load "$D/bad.toml"
        The status should be failure
        The error should include 'Invalid TOML'
        rm -rf "$D"
    End
End
