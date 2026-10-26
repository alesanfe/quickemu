# shellcheck shell=bash
Describe 'async operation registry'
Include lib/quickemu/support/json.sh
Include lib/quickemu/support/filesystem.sh
Include lib/quickemu/runtime/lifecycle.sh
Include lib/quickemu/runtime/ops.sh

setup() { TESTDIR=$(mktemp -d); VMDIR="$TESTDIR/vm1"; VMNAME=vm1; mkdir -p "$VMDIR"; }
cleanup() { rm -rf -- "$TESTDIR"; }
BeforeEach 'setup'
AfterEach 'cleanup'

    It 'creates, updates and completes an operation'
        OPID=$(op_new export)
        op_set "$OPID" running 40
        op_done "$OPID"
        When call op_status "$OPID"
        The output should include '"state":"completed"'
        The output should include '"type":"export"'
        The status should be success
    End
    It 'records a typed failure'
        OPID=$(op_new import)
        op_fail "$OPID" CHECKSUM_MISMATCH "bad archive"
        When call op_status "$OPID"
        The output should include '"state":"failed"'
        The output should include 'CHECKSUM_MISMATCH'
    End
    It 'rejects an unknown operationId'
        When call op_status "no-such-id"
        The status should be failure
        The error should include 'Unknown operation'
    End
End
