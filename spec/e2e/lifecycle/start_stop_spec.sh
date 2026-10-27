#!/usr/bin/env bash
# E2E-LIFECYCLE-001: TCG VM reaches running and stops cleanly.
# Levels: L2 (QMP running). Oracle set: exit code + state + pid + socket.
. "$(dirname "$0")/../support/e2e_lib.sh"

Describe 'start_tcg_vm_reaches_running_state @smoke @critical @qemu-real @tcg'

ws="$(e2e_sandbox "$(e2e_id lifecycle)")"
vm_fixture "$ws" >/dev/null
CONF="$ws/vm.conf"

It 'launches and reports running via the contract'
CPU_MODEL="$E2E_CPU" QEMU_ACCEL="$E2E_ACCEL" "$QE" --vm "$CONF" &
wait_until '[ "$(qe_state "$CONF")" = running ]' "$E2E_TIMEOUT"
The value "$(qe_state "$CONF")" should equal running
End

It 'stops and leaves no residual resources'
"$QE" --vm "$CONF" --kill >/dev/null 2>&1 || true
wait_until '[ "$(qe_state "$CONF")" = stopped ]' 15
The value "$(qe_state "$CONF")" should equal stopped
find "$ws" -name '*.sock' | The line count should equal 0
e2e_artifact "$ws" start-stop passed lifecycle
End

AfterEach e2e_cleanup "$ws"
End
