# shellcheck shell=bash
Describe 'catalog'
Include lib/quickemu/catalog/resolver.sh
Include lib/quickemu/catalog/validator.sh

Describe 'catalog_lookup'
setup() {
    CSV=$(mktemp)
    cat >"${CSV}" <<'CSV'
Display Name,OS,Release,Option
Ubuntu,ubuntu,24.04,daily
Ubuntu,ubuntu,22.04,lts
Debian,debian,12,
CSV
}
cleanup() { rm -f "${CSV}"; }
BeforeEach 'setup'
AfterEach 'cleanup'

It 'returns only rows for the requested os'
When call catalog_lookup "${CSV}" ubuntu
The line 1 should include '24.04'
The line 2 should include '22.04'
The lines of output should equal 2
End

It 'returns nothing for unknown os'
When call catalog_lookup "${CSV}" gentoo
The output should eq ''
End
End

Describe 'catalog_entry_valid'
It 'accepts complete entries'
When call catalog_entry_valid 'Ubuntu' 'ubuntu' '24.04'
The status should be success
End

It 'rejects entries missing release'
When call catalog_entry_valid 'Ubuntu' 'ubuntu' ''
The status should be failure
End
End
End
