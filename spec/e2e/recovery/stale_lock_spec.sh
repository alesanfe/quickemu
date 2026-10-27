#!/usr/bin/env bash
. "$(dirname "$0")/../support/e2e_lib.sh"
Describe 'stale_lock_is_recovered @recovery @p1'
ws="$(e2e_sandbox "$(e2e_id recovery)")"
vm_fixture "$ws" >/dev/null
touch "$ws/vm.conf.lock"
It 'a stale lock does not block a fresh status'
The value "$(qe_state "$ws/vm.conf")" should not equal ''
End
End
