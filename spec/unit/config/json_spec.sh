# shellcheck shell=bash
Describe 'json_escape'
Include lib/quickemu/cli/json.sh

It 'leaves plain text unchanged'
When call json_escape 'hello world'
The output should eq 'hello world'
End

It 'escapes double quotes'
When call json_escape 'say "hi"'
The output should eq 'say \"hi\"'
End

It 'escapes backslashes'
When call json_escape 'a\b'
The output should eq 'a\\b'
End

It 'escapes newlines'
When call json_escape $'a\nb'
The output should eq 'a\nb'
End

It 'handles unicode names'
When call json_escape 'VM-acá-'
The output should eq 'VM-acá-'
End

It 'handles paths with spaces and quotes'
When call json_escape '/tmp/my "vm"/disk.qcow2'
The output should eq '/tmp/my \"vm\"/disk.qcow2'
End
End
