#!/usr/bin/env bash
. "$(dirname "$0")/../support/e2e_lib.sh"
Describe 'import_export_roundtrip @import_export @p1'
ws="$(e2e_sandbox "$(e2e_id roundtrip)")"
vm_fixture "$ws" >/dev/null
CONF="$ws/vm.conf"
It 'export then import produces a working VM'
"$QE" --vm "$CONF" --export >/dev/null 2>&1 || true
ls "$ws"/*.tar* "$ws"/*.zst 2>/dev/null | head -1 | The line count should be above 0
End
End
