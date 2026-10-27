# shellcheck disable=SC2034
Describe 'quickget CLI dispatch (real entry point)'
QG="${SHELLSPEC_PROJECT_ROOT}/bin/quickget"
setup() { TMP=$(mktemp); }
cleanup() { rm -f "$TMP"; }
BeforeEach 'setup'
AfterEach 'cleanup'
# Run the real script with stdin closed; cap output for the reporter but
# preserve the real exit status.
qg() {
    "$QG" "$@" </dev/null >"$TMP" 2>&1
    local s=$?
    head -60 "$TMP"
    return "$s"
}

It 'lists supported OSes'
    When run qg --list
    The status should be success
    The output should include 'alpine'
End
It 'lists as CSV'
    When run qg --list-csv
    The status should be success
    The output should include 'Display Name,OS,Release'
End
It 'lists arm64-supported OSes'
    When run qg --arch arm64 --list
    The status should be success
    The output should include 'OS'
End
It 'prints the version'
    When run qg --version
    The status should be success
    The output should match pattern '[0-9]*.[0-9]*.[0-9]*'
End
It 'prints help'
    When run qg --help
    The status should be success
    The output should include 'quickget'
End
It 'rejects unsupported arguments'
    When run qg --bogus-flag
    The status should be failure
    The output should include 'Not supported argument'
End
It 'rejects an unsupported OS'
    When run qg not-an-os 1
    The status should be failure
    The output should include 'not a supported OS'
End
It 'requires a release'
    When run qg alpine
    The status should be failure
    The output should include 'release'
End
It 'rejects an unsupported architecture flag'
    When run qg --arch riscv alpine
    The status should be failure
    The output should include 'Unsupported architecture'
End
It 'shows OS info for a supported OS'
    When run qg --show alpine
    The status should be success
    The output should include 'Alpine'
End
It 'rejects an invalid release'
    When run qg alpine 99
    The status should be failure
    The output should include 'not a supported release'
End
It 'rejects a missing edition for editions OSes'
    When run qg fedora 42
    The status should be failure
    The output should include 'edition'
End
End
