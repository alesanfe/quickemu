# shellcheck disable=SC2034
Describe 'quickemu doctor_vm validation plan'
Include lib/quickemu/planning/validation_plan.sh

setup() {
    VMNAME=testvm OS_KERNEL=Linux QEMU=/bin/true
    VMDIR=/nonexistent disk_img='' disk_size='' iso='' boot='' ram='' tpm='' network=''
    host_has_kvm() { return 0; }
    host_free_mem_mb() { echo 8192; }
    is_numeric() { return 0; }
    plan_check_ram() { return 1; }
    plan_check_tpm() { return 1; }
    plan_check_net() { return 1; }
}
BeforeEach 'setup'

Describe 'doctor_vm'
    It 'reports a clean bill of health'
        When call doctor_vm
        The output should include 'KVM:  /dev/kvm available'
        The output should include 'No problems detected'
    End
    It 'reports macOS HVF acceleration instead of KVM'
        OS_KERNEL=Darwin
        When call doctor_vm
        The output should include 'macOS (HVF acceleration)'
        The output should include 'No problems detected'
    End
    It 'warns when KVM is unavailable'
        host_has_kvm() { return 1; }
        When call doctor_vm
        The output should include 'not writable'
        The output should include '1 issue(s) found'
    End
    It 'flags a non-executable QEMU binary'
        QEMU=/nonexistent/qemu
        When call doctor_vm
        The output should include 'not executable'
        The output should include 'issue(s) found'
    End
    It 'notes a missing disk image without failing'
        disk_img=/nonexistent/disk.qcow2 disk_size=64G
        When call doctor_vm
        The output should include 'missing (will be created, 64G)'
        The output should include 'No problems detected'
    End
    It 'flags a missing ISO'
        iso=/nonexistent/x.iso
        When call doctor_vm
        The output should include 'ISO'
        The output should include 'issue(s) found'
    End
    It 'checks EFI firmware presence'
        boot=efi
        When call doctor_vm
        The output should include 'EFI'
    End
    It 'reports insufficient RAM'
        ram=64G
        host_free_mem_mb() { echo 1024; }
        plan_check_ram() { return 0; }
        When call doctor_vm
        The output should include 'RAM'
        The output should include 'issue(s) found'
    End
    It 'reports sufficient RAM'
        ram=4G
        When call doctor_vm
        The output should include 'requested (8192M free)'
    End
    It 'flags tpm=on without swtpm'
        tpm=on
        plan_check_tpm() { return 0; }
        When call doctor_vm
        The output should include 'swtpm'
        The output should include 'issue(s) found'
    End
    It 'checks the bridge interface for bridged networking'
        network=br0
        plan_check_net() { return 0; }
        When call doctor_vm
        The output should include 'bridge br0 up'
    End
    It 'flags a missing bridge interface'
        network=br0
        When call doctor_vm
        The output should include 'does not exist'
        The output should include 'issue(s) found'
    End
    It 'ignores slirp-style networking'
        network=slirp
        When call doctor_vm
        The output should not include 'bridge'
    End
End
End
