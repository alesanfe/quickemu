Describe 'QEMU command builder policies'
Include lib/quickemu/runtime/launch.sh

setup() {
    declare -gA CPU_FLAG_MAP=()
    CPU='-cpu host'
    OS_KERNEL=Linux
    HOST_CPU_VENDOR=GenuineIntel
    FLAG_SUPPORTED=0
    ram=''
    min_ram=''
    guest_os=linux
    VM=test.conf
    RAM_MB=8192
    sound_card=''
    GUEST_TWEAKS=''
    NET_DEVICE=''
    macos_release=''
}
check_cpu_flag() { return "$FLAG_SUPPORTED"; }
host_total_mem_mb() { printf '%s\n' "$RAM_MB"; }
BeforeEach 'setup'

Describe 'CPU flags'
Parameters
    '+aes' ',+aes'
    ',+avx' ',+avx'
    'hv_spinlocks=8191' ',hv_spinlocks=8191'
    '' ''
End
It 'normalises and appends a valid flag'
    When call add_cpu_flag "$1"
    The status should be success
    The variable CPU should eq "-cpu host$2"
End
End

Describe 'invalid CPU flags'
Parameters
    'has space'
    '$(touch injected)'
    ';exit'
    '123'
End
It 'rejects the input without changing the CPU argument'
    When call add_cpu_flag "$1"
    The status should eq 1
    The output should include 'Invalid CPU flag format'
    The variable CPU should eq '-cpu host'
End
End

It 'deduplicates repeated CPU flags'
    add_cpu_flag +aes
    When call add_cpu_flag +aes
    The status should be success
    The variable CPU should eq '-cpu host,+aes'
End
It 'rejects opposite CPU flags without overriding the first'
    add_cpu_flag +aes
    When call add_cpu_flag -aes
    The status should eq 1
    The output should include 'Conflicting CPU flag'
    The variable CPU should eq '-cpu host,+aes'
End
It 'rejects conflicting values without overriding the first'
    add_cpu_flag hv_spinlocks=8191
    When call add_cpu_flag hv_spinlocks=2
    The status should eq 1
    The output should include 'Conflicting CPU flag'
    The variable CPU should eq '-cpu host,hv_spinlocks=8191'
End
It 'resets the deduplication map between builds'
    add_cpu_flag +aes
    reset_and_count() { reset_cpu_flags; printf '%s' "${#CPU_FLAG_MAP[@]}"; }
    When call reset_and_count
    The status should be success
    The output should eq 0
End

Describe 'host CPU flag translation'
Parameters
    Linux GenuineIntel sse4_2 0 ',+sse4.2'
    Linux GenuineIntel sse4_1 0 ',+sse4.1'
    Linux GenuineIntel tsc_adjust 0 ',+tsc-adjust'
    Darwin GenuineIntel avx 0 ',+AVX1.0'
    Linux AuthenticAMD pcid 1 ',-pcid'
    Linux GenuineIntel avx 1 ''
End
It 'maps host features to QEMU flags'
    OS_KERNEL="$1" HOST_CPU_VENDOR="$2" FLAG_SUPPORTED="$4"
    When call configure_cpu_flag "$3"
    The status should be success
    The output should eq "$5"
End
End

Describe 'automatic guest RAM allocation'
Parameters
    4096 2G
    8192 4G
    16384 8G
    65536 16G
    131072 32G
End
It 'allocates a bounded share of controlled host RAM'
    RAM_MB="$1"
    When call configure_ram
    The status should be success
    The output should include "$2 RAM"
    The variable RAM_VM should eq "$2"
End
End
It 'honours explicit RAM even when below the Windows recommendation'
    ram=2G guest_os=windows
    When call configure_ram
    The status should be success
    The output should include 'WARNING'
    The variable RAM_VM should eq 2G
End
It 'fails when automatic allocation is insufficient for macOS'
    RAM_MB=8192 guest_os=macos
    When run configure_ram
    The status should eq 1
    The output should include 'insufficient RAM'
End

Describe 'guest network compatibility'
Parameters
    linux virtio-net-pci
    windows virtio-net-pci
    windows-server e1000
    freedos pcnet
    reactos e1000
    templeos e1000
    freebsd virtio-net-pci
    unknown rtl8139
End
It 'selects the compatible network adapter'
    guest_os="$1"
    When call configure_os_quirks
    The status should be success
    The variable NET_DEVICE should eq "$2"
End
End

Describe 'macOS release compatibility'
Parameters
    monterey virtio-net-pci virtio-blk-pci virtio-sound-pci
    ventura virtio-net-pci virtio-blk-pci virtio-sound-pci
    sequoia virtio-net-pci virtio-blk-pci virtio-sound-pci
    big-sur virtio-net-pci virtio-blk-pci ich9-intel-hda
    catalina vmxnet3 virtio-blk-pci ''
    mojave vmxnet3 ide-hd,bus=ahci.2 ''
End
It 'selects devices supported by the guest release'
    guest_os=macos macos_release="$1"
    When call configure_os_quirks
    The status should be success
    The variable NET_DEVICE should eq "$2"
    The variable MAC_DISK_DEV should eq "$3"
    The variable sound_card should eq "$4"
End
End
End
