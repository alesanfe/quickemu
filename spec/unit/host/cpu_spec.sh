# shellcheck disable=SC2034
Describe 'host CPU probes'
Include lib/quickemu/host/cpu.sh

setup() {
    TESTDIR=$(mktemp -d)
    FAKE_BIN="$TESTDIR/bin"
    mkdir -p "$FAKE_BIN"
    OS_KERNEL=Linux ARCH_HOST=x86_64 HOST_CPU_VENDOR=GenuineIntel
    guest_os=linux macos_release='' TSC_CLOCKSOURCE=''
}
cleanup() { rm -rf -- "$TESTDIR"; }
BeforeEach 'setup'
AfterEach 'cleanup'

in_subshell() { ( "$@" ); }

# A scripted lscpu fixture; the spec controls the whole Flags/vendor output.
fake_lscpu() {
    cat >"$FAKE_BIN/lscpu" <<EOF
#!/bin/sh
cat <<'LSCPU'
$1
LSCPU
EOF
    chmod +x "$FAKE_BIN/lscpu"
    PATH="$FAKE_BIN:$PATH"
}

fake_sysctl() {
    cat >"$FAKE_BIN/sysctl" <<EOF
#!/bin/sh
case "\$2" in
$1
esac
EOF
    chmod +x "$FAKE_BIN/sysctl"
    PATH="$FAKE_BIN:$PATH"
}

Describe 'core counting'
It 'uses nproc when available'
    printf '#!/bin/sh\necho 16\n' >"$FAKE_BIN/nproc"; chmod +x "$FAKE_BIN/nproc"
    PATH="$FAKE_BIN:$PATH"
    When call host_cpu_cores
    The status should be success
    The output should eq '16'
End
It 'falls back to sysctl hw.ncpu without nproc'
    command() { [ "$2" = 'nproc' ] && return 1; builtin command "$@"; }
    printf '#!/bin/sh\necho 8\n' >"$FAKE_BIN/sysctl"; chmod +x "$FAKE_BIN/sysctl"
    PATH="$FAKE_BIN:$PATH"
    When call get_nproc
    The status should be success
    The output should eq '8'
End
It 'fails when neither nproc nor sysctl exist'
    command() { [ "$2" = 'nproc' ] || [ "$2" = 'sysctl' ] && return 1; [ "$2" = 'sysctl' ] && return 1; builtin command "$@"; }
    When call in_subshell get_nproc
    The status should be failure
    The output should include 'Unable to determine'
End
End

Describe 'get_cpu_info'
It 'parses the CPU vendor from lscpu'
    fake_lscpu 'Vendor ID:             AuthenticAMD
Model name:            AMD Ryzen 9 5900X'
    When call get_cpu_info '^Vendor ID'
    The status should be success
    The output should eq 'AuthenticAMD'
End
It 'parses the model name from lscpu'
    fake_lscpu 'Model name:            AMD Ryzen 9 5900X
Model name:            AMD Ryzen 9 5900X'
    When call get_cpu_info '^Model name:'
    The status should be success
    The output should include 'AMD Ryzen 9 5900X'
End
It 'maps the model name to machdep.cpu.brand_string on macOS'
    OS_KERNEL=Darwin
    fake_sysctl 'machdep.cpu.brand_string) echo "Apple M2" ;;'
    When call get_cpu_info '^Model name:'
    The status should be success
    The output should eq 'Apple M2'
End
It 'maps the vendor to machdep.cpu.vendor on macOS Intel'
    OS_KERNEL=Darwin ARCH_HOST=x86_64
    fake_sysctl 'machdep.cpu.vendor) echo "Genuine Intel" ;;'
    When call get_cpu_info '^Vendor ID'
    The status should be success
    The output should eq 'GenuineIntel'
End
It 'maps the vendor to the brand string on Apple Silicon'
    OS_KERNEL=Darwin ARCH_HOST=arm64
    fake_sysctl 'machdep.cpu.brand_string) echo "Apple M2" ;;'
    When call get_cpu_info '^Vendor ID'
    The status should be success
    The output should eq 'Apple'
End
It 'fails for an unmapped macOS info name'
    OS_KERNEL=Darwin
    When call in_subshell get_cpu_info 'Bogus'
    The status should be failure
    The output should include 'Could not find macOS translation'
End
End

Describe 'check_cpu_flag'
It 'matches a flag present in lscpu output'
    fake_lscpu 'Flags:                 fpu vme de pse tsc msr pae mce cx8 apic sep mtrr sse4_2 avx2'
    When call check_cpu_flag sse4_2
    The status should be success
End
It 'rejects a flag absent from lscpu output'
    fake_lscpu 'Flags:                 fpu vme de pse tsc msr pae mce cx8'
    When call check_cpu_flag avx2
    The status should be failure
End
It 'treats AMD constant_tsc as equivalent to invtsc'
    HOST_CPU_VENDOR=AuthenticAMD
    fake_lscpu 'Flags:                 fpu vme constant_tsc'
    When call check_cpu_flag invtsc
    The status should be success
End
It 'does not treat constant_tsc as invtsc for Intel'
    HOST_CPU_VENDOR=GenuineIntel
    fake_lscpu 'Flags:                 fpu vme constant_tsc'
    When call check_cpu_flag invtsc
    The status should be failure
End
It 'translates flag names for macOS sysctl queries'
    OS_KERNEL=Darwin
    fake_sysctl 'machdep.cpu.features) echo "FPU VME SSE4.2 AVX1.0" ;;'
    When call check_cpu_flag sse4_2
    The status should be success
End
It 'maps avx to AVX1.0 on macOS'
    OS_KERNEL=Darwin
    fake_sysctl 'machdep.cpu.features) echo "FPU AVX1.0" ;;'
    When call check_cpu_flag avx
    The status should be success
End
End

Describe 'host_arch'
It 'normalises arm64 to aarch64'
    printf '#!/bin/sh\necho arm64\n' >"$FAKE_BIN/uname"; chmod +x "$FAKE_BIN/uname"
    PATH="$FAKE_BIN:$PATH"
    When call host_arch
    The status should be success
    The output should eq 'aarch64'
End
It 'passes other architectures through unchanged'
    printf '#!/bin/sh\necho riscv64\n' >"$FAKE_BIN/uname"; chmod +x "$FAKE_BIN/uname"
    PATH="$FAKE_BIN:$PATH"
    When call host_arch
    The status should be success
    The output should eq 'riscv64'
End
End

Describe 'ignore_msrs_alert'
It 'does nothing on macOS'
    OS_KERNEL=Darwin
    When call ignore_msrs_alert
    The status should be success
    The output should be blank
End
It 'warns when the kernel ignores MSRs is disabled'
    OS_KERNEL=Linux
    if [ -e /sys/module/kvm/parameters/ignore_msrs ] && [ "$(cat /sys/module/kvm/parameters/ignore_msrs)" = 'N' ]; then
        When call ignore_msrs_alert
        The status should be success
        The output should include 'Ignoring unhandled Model-Specific Registers'
    else
        When call ignore_msrs_alert
        The status should be success
        The output should be blank
    fi
End
End

Describe 'host_macos_tsc_unstable'
It 'only applies to Linux hosts'
    OS_KERNEL=Darwin HOST_CPU_VENDOR=AuthenticAMD guest_os=macos macos_release=ventura
    When call host_macos_tsc_unstable
    The status should be failure
End
It 'only applies to AMD CPUs'
    OS_KERNEL=Linux HOST_CPU_VENDOR=GenuineIntel guest_os=macos macos_release=ventura
    When call host_macos_tsc_unstable
    The status should be failure
End
It 'only applies to macOS guests'
    OS_KERNEL=Linux HOST_CPU_VENDOR=AuthenticAMD guest_os=linux macos_release=ventura
    When call host_macos_tsc_unstable
    The status should be failure
End
It 'only applies to Ventura and newer'
    OS_KERNEL=Linux HOST_CPU_VENDOR=AuthenticAMD guest_os=macos macos_release=monterey
    When call host_macos_tsc_unstable
    The status should be failure
End
End

Describe 'host_macos_tsc_unstable release gate'
Parameters
    ventura
    sonoma
    sequoia
    tahoe
End
It 'passes the release gate for $1'
    OS_KERNEL=Linux HOST_CPU_VENDOR=AuthenticAMD guest_os=macos macos_release="$1"
    if [ -r /proc/cmdline ] && [[ "$(cat /proc/cmdline)" == *tsc=reliable* ]]; then skip 'host kernel has tsc=reliable'; fi
    if [ ! -r /sys/devices/system/clocksource/clocksource0/current_clocksource ]; then skip 'no clocksource on host'; fi
    When call host_macos_tsc_unstable
    The status should be present
End
End

Describe 'host_has_kvm'
It 'always reports KVM usable on macOS (HVF)'
    OS_KERNEL=Darwin
    When call host_has_kvm
    The status should be success
End
It 'reflects /dev/kvm writability on Linux'
    OS_KERNEL=Linux
    if [ -w /dev/kvm ]; then
        When call host_has_kvm
        The status should be success
    else
        When call host_has_kvm
        The status should be failure
    fi
End
End
End
