# shellcheck disable=SC2034
Describe 'cli output boundary'
Include lib/quickemu/cli/output.sh
Include lib/quickemu/support/json.sh

VERSION='4.9.9' QEMU_VER_LONG='8.2.0' LAUNCHER='quickemu' QE_SCHEMA_VERSION=1
json_escape() { printf '%s' "$1"; }

Describe 'usage'
    It 'prints the banner and option list'
        When call usage
        The output should include 'quickemu'
        The output should include '--vm ubuntu.conf'
        The output should include '--snapshot create'
    End
End

Describe 'emit_execution_plan'
    It 'prints the command verbatim in human format'
        OUTPUT_FORMAT=human
        When call emit_execution_plan myvm 'qemu-system-x86_64 -m 4G'
        The output should eq 'qemu-system-x86_64 -m 4G'
    End
    It 'wraps the command in the schema contract in json format'
        OUTPUT_FORMAT=json
        When call emit_execution_plan myvm 'qemu -name "x"'
        The output should include '"schemaVersion":1'
        The output should include '"vm":"myvm"'
        The output should include '"command":'
    End
End
End
