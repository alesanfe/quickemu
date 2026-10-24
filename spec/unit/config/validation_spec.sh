# shellcheck shell=bash
Describe 'is_numeric'
Include lib/quickemu/support/validation.sh

It 'accepts integers'
When call is_numeric 4096
The status should be success
End

It 'rejects empty strings'
When call is_numeric ''
The status should be failure
End

It 'rejects decimals'
When call is_numeric '4.5'
The status should be failure
End

It 'rejects negatives'
When call is_numeric '-1'
The status should be failure
End

It 'rejects text'
When call is_numeric '4G'
The status should be failure
End
End

Describe 'qe_safe_rm'
Include lib/quickemu/support/filesystem.sh

It 'removes an existing file'
local tmp
tmp=$(mktemp)
When call qe_safe_rm "${tmp}"
The status should be success
The path "${tmp}" should not be file
End

It 'ignores missing files'
When call qe_safe_rm '/nonexistent/path/file'
The status should be success
End
End

Describe 'conf_normalize_bool'
Include lib/quickemu/config/normalizer.sh

Parameters
on on
ON on
yes on
true on
1 on
off off
no off
'' off
garbage off
End

It 'normalizes "$1" to "$2"'
When call conf_normalize_bool "${1}"
The output should eq "${2}"
End
End
