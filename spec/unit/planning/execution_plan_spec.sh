# shellcheck disable=SC2034
Describe 'execution plan gates'
Include lib/quickemu/support/filesystem.sh
Include lib/quickemu/planning/execution_plan.sh
Include lib/quickemu/support/filesystem.sh
Include lib/quickemu/support/validation.sh

setup() {
    TESTDIR=$(mktemp -d)
    IGNORE_TSC_WARNING='' DRY_RUN='' TSC_CLOCKSOURCE=''
    macos_release='' HOST_CPU_VENDOR='' MEM_FREE_MB=''
}
cleanup() { rm -rf -- "$TESTDIR"; }
BeforeEach 'setup'
AfterEach 'cleanup'

in_subshell() { ( "$@" ); }

# Host probes replaced with controllable doubles.
host_free_mem_mb() { echo "${MEM_FREE_MB}"; }
host_has_tool() { [ "$1" != swtpm ]; }
vm_net_mode() { echo "${NET_MODE}"; }
host_iface_exists() { [ "$1" = 'br0-present' ]; }
host_macos_tsc_unstable() { return "${TSC_UNSTABLE:-1}"; }
vm_log() { echo "LOG:$1" >>"$TESTDIR/vm.log"; }

Describe 'plan_check_ram'
Parameters
    4096M 512     success
    512M  1048576 failure
End
It 'warns only when the requested RAM exceeds free memory'
    MEM_FREE_MB="$2"
    When call plan_check_ram "$1"
    The status should be "$3"
End
It 'ignores non-numeric values'
    MEM_FREE_MB=1
    When call plan_check_ram 'plenty'
    The status should be failure
End
End

Describe 'plan_check_tpm'
# plan_check_* return 0 when the gate triggers (problem found), 1 when clear.
It 'triggers when tpm=on but swtpm is missing'
    When call plan_check_tpm on
    The status should be success
End
It 'does not trigger when tpm=on and swtpm exists'
    host_has_tool() { return 0; }
    When call plan_check_tpm on
    The status should be failure
End
It 'does not trigger when TPM is off'
    When call plan_check_tpm off
    The status should be failure
End
End

Describe 'plan_check_net'
It 'only checks bridge networks'
    NET_MODE=nat
    When call plan_check_net anything
    The status should be success
End
It 'requires the bridge interface to exist'
    NET_MODE=bridge
    When call plan_check_net br9-missing
    The status should be failure
End
It 'accepts an existing bridge interface'
    NET_MODE=bridge
    When call plan_check_net br0-present
    The status should be success
End
End

Describe 'plan_check_tsc'
It 'passes silently on a stable host'
    TSC_UNSTABLE=1
    When call plan_check_tsc
    The status should be success
    The output should be blank
End
It 'warns but proceeds with --ignore-tsc-warning'
    TSC_UNSTABLE=0 IGNORE_TSC_WARNING=1 TSC_CLOCKSOURCE=hpet macos_release=ventura
    When call plan_check_tsc
    The status should be success
    The output should include 'Unstable TSC detected'
    The output should include 'ignore-tsc-warning'
End
It 'logs and reports without prompting in dry-run'
    TSC_UNSTABLE=0 DRY_RUN=1 TSC_CLOCKSOURCE=hpet macos_release=ventura HOST_CPU_VENDOR=AuthenticAMD
    When call plan_check_tsc
    The status should be success
    The output should include 'Unstable TSC detected'
    The file "$TESTDIR/vm.log" should not be exist
End
It 'aborts non-interactive runs without the bypass flag'
    TSC_UNSTABLE=0 DRY_RUN='' TSC_CLOCKSOURCE=hpet macos_release=ventura HOST_CPU_VENDOR=AuthenticAMD
    When call in_subshell plan_check_tsc </dev/null
    The status should be failure
    The output should include 'Non-interactive mode'
    The contents of file "$TESTDIR/vm.log" should include 'TSC_WARNING'
End
End
End
