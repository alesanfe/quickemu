# shellcheck shell=bash
Describe 'vm status'
Include lib/quickemu/cli/status.sh
Include lib/quickemu/runtime/pid.sh

Describe 'status_vm'
It 'prints human output when stopped'
VMNAME="testvm" VM_PID="" OUTPUT_FORMAT="human"
When call qe_in_subshell status_vm
The output should eq 'stopped'
The status should be success
End

It 'prints running pid in human output'
VMNAME="testvm" VM_PID="4242" OUTPUT_FORMAT="human"
When call qe_in_subshell status_vm
The output should eq 'running 4242'
End

It 'emits v1 JSON when stopped'
VMNAME="testvm" VM_PID="" OUTPUT_FORMAT="json"
When call qe_in_subshell status_vm
The output should eq '{"schemaVersion":1,"name":"testvm","state":"stopped","pid":null}'
End

It 'emits v1 JSON when running'
VMNAME="vm x" VM_PID="99" OUTPUT_FORMAT="json"
When call qe_in_subshell status_vm
The output should eq '{"schemaVersion":1,"name":"vm x","state":"running","pid":99}'
End
End

Describe 'list_vms'
setup() {
    TESTDIR=$(mktemp -d)
    cd "${TESTDIR}" || return
    mkdir -p alpha
    printf 'guest_os="windows"\n' >alpha.conf
    printf 'guest_os="linux"\n' >'my vm.conf'
}
cleanup() { cd "${OLDPWD:-$HOME}" && rm -rf "${TESTDIR}"; }
BeforeEach 'setup'
AfterEach 'cleanup'

It 'prints a message when no confs exist'
cd "$(mktemp -d)" || return
OUTPUT_FORMAT="human"
When call list_vms
The output should include 'No VM configurations found'
End

It 'emits empty JSON array when no confs exist'
cd "$(mktemp -d)" || return
OUTPUT_FORMAT="json"
When call list_vms
The output should eq '{"schemaVersion":1,"vms":[]}'
End

It 'lists confs as JSON with states'
OUTPUT_FORMAT="json"
When call list_vms
The output should include '"name":"alpha"'
The output should include '"guest_os":"windows"'
The output should include '"state":"stopped"'
End

It 'marks stale pid files'
echo 999999 >"alpha/alpha.pid"
OUTPUT_FORMAT="json"
When call list_vms
The output should include '"state":"stale"'
End

It 'handles names with spaces'
OUTPUT_FORMAT="json"
When call list_vms
The output should include '"name":"my vm"'
End
End
End
