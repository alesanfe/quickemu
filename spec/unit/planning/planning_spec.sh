# shellcheck shell=bash
Describe 'planning gates'
Include lib/quickemu/support/validation.sh
Include lib/quickemu/support/logging.sh
Include lib/quickemu/host/cpu.sh
Include lib/quickemu/host/network.sh
Include lib/quickemu/vm/network.sh
Include lib/quickemu/planning/execution_plan.sh

Describe 'plan_check_ram'
# Stub the host probe: pretend 8192 MiB are free.
host_free_mem_mb() { echo 8192; }

It 'passes when enough memory is free'
When call plan_check_ram 4G
The status should be failure # function returns 1 = OK/no warning
End

It 'warns when requested RAM exceeds free memory'
When call plan_check_ram 64G
The status should be success # returns 0 = problem detected
End

It 'passes non-numeric input through'
When call plan_check_ram 'abc'
The status should be failure
End
End

Describe 'plan_check_tpm'
It 'flags tpm=on when swtpm is missing'
host_has_tool() { return 1; }
When call plan_check_tpm on
The status should be success
End

It 'passes tpm=on when swtpm exists'
host_has_tool() { return 0; }
When call plan_check_tpm on
The status should be failure
End

It 'ignores tpm=off'
host_has_tool() { return 1; }
When call plan_check_tpm off
The status should be failure
End
End
End
