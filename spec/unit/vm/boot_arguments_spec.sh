Describe 'VM boot argument assembly'
Include lib/quickemu/vm/vm.sh

setup() {
    TESTDIR=$(mktemp -d)
    VMDIR="$TESTDIR"
    VMNAME='test-vm'
    VM=test-vm.conf
    OS_KERNEL=Linux
    VERSION=test
    QEMU=qemu_probe
    QEMU_VER_LONG=10.1
    QEMU_VER_SHORT=101
    ARCH_VM=x86_64
    ACCEL=kvm
    boot=legacy
    guest_os=linux
    display=sdl
    network=''
    bridge=''
    macaddr=''
    usb_controller=xhci
    keyboard=usb
    mouse=tablet
    keyboard_layout=''
    CPU_PINNING=''
    BRAILLE=''
    disk_img="$TESTDIR/disk with spaces.qcow2"
    disk_format=qcow2
    iso=''
    fixed_iso=''
    floppy=''
    img=''
    PUBLIC=''
    VIRTIOFSD_SOCKET=''
    USB_PASSTHROUGH=''
    gpu_passthrough=''
    tpm=off
    monitor=none
    serial=none
    extra_args=''
    kernel=''
    initrd=''
    boot_args=''
    seed=''
    STATUS_QUO=''
    MACHINE_TYPE=q35
    RAM_MB=8192
    SOCKET_MONITOR="$TESTDIR/monitor.sock"
    SOCKET_SERIAL="$TESTDIR/serial.sock"
}
cleanup() { rm -f -- "$TESTDIR"/*; rmdir -- "$TESTDIR"; }
BeforeEach 'setup'
AfterEach 'cleanup'

runtime_launcher_init() { :; }
configure_cpu() { CPU='-cpu host'; SMP='-smp 2'; GUEST_CPU_LOGICAL_CORES=2; QEMU_ACCEL="$ACCEL"; }
configure_ram() { RAM_VM=4G; }
plan_check_tsc() { :; }
configure_bios() { EFI_CODE="$TESTDIR/code.fd"; EFI_VARS="$TESTDIR/vars.fd"; }
configure_os_quirks() { :; }
configure_storage() { :; }
start_virtiofsd() { :; }
configure_display() { DISPLAY_RENDER="$display"; }
configure_audio() { AUDIO_DEV='none,id=audio0'; }
configure_ports() { :; }
configure_file_sharing() { :; }
configure_usb() { :; }
configure_tpm() { :; }
stop_virtiofsd() { :; }
swtpm_socket_ready() { return 0; }
host_total_mem_mb() { printf '%s' "$RAM_MB"; }
qemu_probe() { printf 'no optional devices\n'; }
build_arguments() { vm_boot >/dev/null; printf '%s\n' "${args[@]}"; }

Describe 'network isolation'
Parameters
    none '-nic' none
    restrict '-netdev' 'user,hostname=test-vm,restrict=y,id=nic'
    passt '-netdev' 'passt,id=nic'
    '' '-netdev' 'user,hostname=test-vm,id=nic'
End
It 'builds the requested network backend without starting a guest'
    network="$1"
    When call build_arguments
    The status should be success
    The output should include "$2"
    The output should include "$3"
    The path "$TESTDIR/test-vm.pid" should not exist
End
End
It 'rejects passt on an unsupported QEMU version'
    network=passt QEMU_VER_SHORT=100
    When run vm_boot
    The status should eq 1
    The output should include 'requires QEMU 10.1'
End

Describe 'architecture and accelerator compatibility'
Parameters
    x86_64 kvm 'q35,smm=off,vmport=off,accel=kvm'
    aarch64 kvm 'virt,accel=kvm'
    x86_64 tcg 'q35,smm=off,vmport=off'
    aarch64 tcg virt
End
It 'selects compatible machine properties'
    ARCH_VM="$1" ACCEL="$2"
    [ "$ARCH_VM" = aarch64 ] && MACHINE_TYPE=virt
    When call build_arguments
    The status should be success
    The output should include "$3"
End
End
It 'uses a larger TCG cache on a large-memory host'
    ACCEL=tcg RAM_MB=32768
    When call build_arguments
    The status should be success
    The output should include 'tcg,tb-size=512,thread=multi'
End
It 'uses the bounded TCG cache on a small-memory host'
    ACCEL=tcg RAM_MB=8192
    When call build_arguments
    The status should be success
    The output should include 'tcg,tb-size=256,thread=multi'
End
It 'uses named ARM pflash nodes rather than x86 secure flash'
    ARCH_VM=aarch64 MACHINE_TYPE=virt boot=efi
    When call build_arguments
    The status should be success
    The output should include 'pflash0=rom,pflash1=efivars'
    The output should include 'node-name=rom,driver=file'
    The output should not include 'cfi.pflash01'
End
It 'uses secure pflash drives for x86 EFI'
    boot=efi
    When call build_arguments
    The status should be success
    The output should include 'driver=cfi.pflash01,property=secure,value=on'
    The output should include 'if=pflash,format=raw,unit=0'
End

Describe 'guest storage compatibility'
Parameters
    linux 'virtio-blk-pci,drive=SystemDisk'
    macos 'ide-hd,bus=ahci.0,drive=BootLoader,bootindex=0'
    kolibrios 'ide-hd,bus=ahci.0,drive=SystemDisk'
    batocera 'virtio-blk-pci,drive=BootDisk'
    reactos 'if=ide,index=0,media=disk'
    templeos 'if=ide,index=0,media=disk'
    windows-server 'ide-hd,drive=SystemDisk'
End
It 'selects a controller supported by the guest'
    guest_os="$1"
    When call build_arguments
    The status should be success
    The output should include "$2"
    The output should include "$disk_img"
End
End

It 'preserves spaces in the direct kernel command line'
    kernel="$TESTDIR/kernel with spaces" initrd="$TESTDIR/initrd with spaces"
    boot_args='console=ttyS0 root=/dev/vda rw'
    When call build_arguments
    The status should be success
    The output should include "$kernel"
    The output should include "$initrd"
    The output should include 'console=ttyS0 root=/dev/vda rw'
End
It 'attaches an existing cloud-init seed without creating other artifacts'
    seed="$TESTDIR/seed.iso"
    : > "$seed"
    When call build_arguments
    The status should be success
    The output should include "file=$seed,media=cdrom,if=virtio,format=raw"
    The path "$TESTDIR/test-vm.sh" should not exist
End
It 'rejects malformed CPU pinning before launching'
    CPU_PINNING='0; touch bad'
    When run vm_boot
    The status should eq 1
    The output should include "Couldn't parse CPU pinning"
End
It 'rejects a CPU pinning count different from the VM core count'
    CPU_PINNING=0
    When run vm_boot
    The status should eq 1
    The output should include 'Number of host cores for pinning'
End
End
