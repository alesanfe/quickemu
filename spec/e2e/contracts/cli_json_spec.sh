#!/usr/bin/env bash
# E2E-CONTRACT-001: envelope outputs satisfy the v1 schema.
Describe 'json_output_matches_schema @smoke @critical @contract'
    Include spec/e2e/support/e2e_lib.sh

    setup() {
        ws="$(e2e_sandbox "$(e2e_id contract)")"
        vm_fixture "$ws" >/dev/null
        CONF="$ws/vm.conf"
    }
    BeforeEach 'setup'

    # Commands that take --vm <conf>.
    It 'envelope is parseable for --status'
        When run "${QE}" --vm "${CONF}" --status --format json
        The output should include '"schemaVersion":1'
        The output should include '"success":'
    End

    It 'envelope is parseable for --backup-list'
        When run "${QE}" --vm "${CONF}" --backup-list --format json
        The output should include '"schemaVersion":1'
        The output should include '"success":'
    End

    It 'envelope is parseable for --host-info'
        When run "${QE}" --vm "${CONF}" --host-info --format json
        The output should include '"schemaVersion":1'
        The output should include '"success":'
    End

    # Standalone commands (no --vm).
    It 'envelope is parseable for --capabilities'
        When run "${QE}" --capabilities --format json
        The output should include '"schemaVersion":1'
        The output should include '"success":'
    End

    It 'vm.status carries the allowedOps the UI gates on'
        When run "${QE}" --vm "${CONF}" --status --format json
        The output should include 'allowedOps'
    End
End
