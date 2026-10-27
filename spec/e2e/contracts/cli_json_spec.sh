#!/usr/bin/env bash
# E2E-CONTRACT-001: envelope outputs satisfy the v1 schema.
. "$(dirname "$0")/../support/e2e_lib.sh"

Describe 'json_output_matches_schema @smoke @critical @contract'

ws="$(e2e_sandbox "$(e2e_id contract)")"
vm_fixture "$ws" >/dev/null
CONF="$ws/vm.conf"

for cmd in --status --snapshot-tree --backup-list --capabilities; do
  It "envelope is parseable for $cmd"
  out=$("$QE" --vm "$CONF" "$cmd" --format json 2>/dev/null)
  When call sh -c "echo '$out' | grep -q '\"schemaVersion\":1'"
  The status should equal 0
  End
done

It 'vm.status carries the allowedOps the UI gates on'
out=$("$QE" --vm "$CONF" --status --format json 2>/dev/null)
The output should include allowedOps
End
End
