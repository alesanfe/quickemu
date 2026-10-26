Describe 'CPU policy with controlled host probes'
Include lib/quickemu/support/filesystem.sh
Include lib/quickemu/runtime/launch.sh

setup() {
    declare -gA CPU_FLAG_MAP=()
    OS_KERNEL=Linux
    ARCH_VM=x86_64
    ARCH_HOST=x86_64
    VENDOR=GenuineIntel
    HOST_CORES=8
    MISSING=unavailable
    HOST_CPU_SMT=off
    MANUFACTURER=Fixture
    guest_os=linux
    cpu_cores=''
    cpu_model=''
    secureboot=off
    SMM=off
    QEMU_VER_SHORT=101
    MACHINE_TYPE=q35
    VMNAME=test
    macos_release=sonoma
    GUEST_TWEAKS=''
}
BeforeEach 'setup'
get_nproc() { printf '%s' "$HOST_CORES"; }
get_cpu_info() {
    case "$1" in
        *Model*) printf 'Fixture CPU';;
        *Socket*) printf 1;;
        *) printf '%s' "$VENDOR";;
    esac
}
check_cpu_flag() { [ "$1" != "$MISSING" ]; }
head() { printf Fixture; }
cat() { printf off; }
ioreg() { printf '"Manufacturer" = "Fixture"\n'; }
sysctl() {
    case "$2" in hw.physicalcpu) printf 4;; hw.logicalcpu) printf 8;; esac
}
ignore_msrs_alert() { :; }

Describe 'automatic core count'
Parameters
    2 1
    4 2
    8 4
    16 8
    32 16
End
It 'allocates at most half the host cores with a bounded maximum'
    HOST_CORES="$1"
    When call configure_cpu
    The status should be success
    The output should include 'Fixture CPU'
    The variable GUEST_CPU_CORES should eq "$2"
End
End
It 'preserves an explicit core count'
    cpu_cores=6
    When call configure_cpu
    The status should be success
    The output should include 'Fixture CPU'
    The variable GUEST_CPU_CORES should eq 6
End
It 'enforces the Windows 11 minimum core count'
    guest_os=windows VMNAME=windows-11 cpu_cores=1
    When call configure_cpu
    The status should be success
    The output should include 'Windows 11 minimum'
    The variable GUEST_CPU_CORES should eq 2
End

Describe 'architecture selection'
Parameters
    x86_64 x86_64 host kvm
    x86_64 aarch64 max tcg
    aarch64 aarch64 max kvm
    aarch64 x86_64 qemu64 tcg
End
It 'uses software emulation only for cross-architecture guests'
    ARCH_HOST="$1" ARCH_VM="$2"
    When call configure_cpu
    The status should be success
    The output should include 'Fixture CPU'
    The variable CPU_MODEL should eq "$3"
    The variable QEMU_ACCEL should eq "$4"
End
End

Describe 'missing hardware virtualization'
Parameters
    GenuineIntel vmx 'Intel VT-x'
    AuthenticAMD svm 'AMD SVM'
    HygonGenuine svm 'AMD SVM'
End
It 'fails before building an unsupported native CPU configuration'
    VENDOR="$1" MISSING="$2"
    When run configure_cpu
    The status should eq 1
    The output should include "$3 support is required"
End
End

Describe 'macOS core and feature policy'
Parameters
    sonoma sse4_2 'SSE 4.2 and AVX2'
    ventura avx2 'SSE 4.2 and AVX2'
    monterey sse4_2 'SSE 4.2'
    sierra sse4_1 'SSE 4.1'
End
It 'rejects a missing required native CPU feature'
    guest_os=macos macos_release="$1" MISSING="$2"
    When run configure_cpu
    The status should eq 1
    The output should include "$3 support"
End
End
Describe 'supported macOS releases'
Parameters
    sonoma GenuineIntel 6 4
    monterey GenuineIntel 3 2
    sierra AuthenticAMD 8 8
    sonoma AuthenticAMD 6 4
End
It 'rounds cores down to a bootable power of two'
    guest_os=macos macos_release="$1" VENDOR="$2" cpu_cores="$3"
    When call configure_cpu
    The status should be success
    The output should include 'Fixture CPU'
    The variable GUEST_CPU_CORES should eq "$4"
    The variable GUEST_TWEAKS should include 'isa-applesmc'
End
End

It 'adds older Hyper-V enlightenments for QEMU 6 Windows guests'
    guest_os=windows QEMU_VER_SHORT=60
    When call configure_cpu
    The status should be success
    The output should include 'Fixture CPU'
    The variable CPU should include 'hv_spinlocks=8191'
    The variable GUEST_TWEAKS should include '-no-hpet'
End
It 'uses Hyper-V passthrough on current QEMU Windows guests'
    guest_os=windows QEMU_VER_SHORT=101
    When call configure_cpu
    The status should be success
    The output should include 'Fixture CPU'
    The variable CPU should include 'hv_passthrough'
    The variable MACHINE_TYPE should include 'hpet=off'
End
It 'does not use x86 Hyper-V flags on ARM Windows guests'
    guest_os=windows ARCH_VM=aarch64 ARCH_HOST=aarch64 VENDOR=ARM
    When call configure_cpu
    The status should be success
    The output should include 'Fixture CPU'
    The variable CPU should not include 'hv_passthrough'
    The variable MACHINE_TYPE should eq 'virt,highmem=on'
End
It 'uses HVF and SMT topology on a controlled macOS host'
    OS_KERNEL=Darwin
    When call configure_cpu
    The status should be success
    The output should include 'Fixture CPU'
    The variable QEMU_ACCEL should eq hvf
    The variable GUEST_CPU_THREADS should eq 2
    The variable SMP should eq '-smp cores=2,threads=2,sockets=1'
End
End
