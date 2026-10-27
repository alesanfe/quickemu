#!/usr/bin/env bash
. "$(dirname "$0")/../support/e2e_lib.sh"
Describe 'qmp_socket_ready @qmp @p1'
ws="$(e2e_sandbox "$(e2e_id qmp)")"
vm_fixture "$ws" >/dev/null
CONF="$ws/vm.conf"
It 'qmp socket appears once the VM boots'
CPU_MODEL="$E2E_CPU" QEMU_ACCEL="$E2E_ACCEL" "$QE" --vm "$CONF" &
wait_until 'ls "$ws"/alpine*/*qmp*.sock >/dev/null 2>&1 || ls "$ws"/*qmp*.sock >/dev/null 2>&1' 15
ls "$ws" | The output should include qmp
"$QE" --vm "$CONF" --kill >/dev/null 2>&1 || true
End
End
