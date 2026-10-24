Describe 'CLI integration'
Include spec/spec_helper.sh

It 'bin/quickemu delegates to the root script'
When call "$QE_ROOT/bin/quickemu" --capabilities
The output should include '"schemaVersion": 1'
The status should be success
End

It '--help prints usage'
When call "$QE_ROOT/bin/quickemu" --help
The output should include '--vm'
The status should be success
End
End
