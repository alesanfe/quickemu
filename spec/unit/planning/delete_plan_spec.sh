# shellcheck shell=bash
Describe 'delete_plan'
Include lib/quickemu/support/json.sh
Include lib/quickemu/planning/delete-plan.sh

setup() {
    TESTDIR=$(mktemp -d)
    cd "${TESTDIR}" || return
    mkdir -p vm1/subdir
    printf 'guest_os="linux"\n' >vm1.conf
    touch vm1/disk.qcow2 vm1/subdir/extra
}
cleanup() { cd "${OLDPWD:-$HOME}" && rm -rf "${TESTDIR}"; }
BeforeEach 'setup'
AfterEach 'cleanup'

It 'reports nothing to delete for missing vm (human)'
VMNAME="ghost" VM="ghost.conf" VMDIR="ghost" OUTPUT_FORMAT="human"
When call delete_plan
The output should include 'nothing to delete'
End

It 'lists conf and dir contents (human)'
VMNAME="vm1" VM="vm1.conf" VMDIR="vm1" OUTPUT_FORMAT="human"
When call delete_plan
The output should include 'vm1.conf'
The output should include 'vm1/disk.qcow2'
The output should include 'vm1/subdir'
End

It 'emits a v1 delete plan (json)'
VMNAME="vm1" VM="vm1.conf" VMDIR="vm1" OUTPUT_FORMAT="json"
When call delete_plan
The output should include '"schemaVersion":1'
The output should include '"vm":"vm1"'
The output should include '"vm1/disk.qcow2"'
End

It 'never deletes anything'
plan_and_verify() {
    delete_plan >/dev/null
    [ -f vm1/disk.qcow2 ] && [ -f vm1.conf ]
}
VMNAME="vm1" VM="vm1.conf" VMDIR="vm1" OUTPUT_FORMAT="json"
When call plan_and_verify
The status should be success
End
End
