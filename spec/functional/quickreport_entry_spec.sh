# shellcheck disable=SC2034
# Coverage: quickreport report composition (pre-existing, merged in ad88b04).
# Covers lib/quickreport/cli/report.sh composition and the entry point.
Describe 'quickreport'
Include spec/spec_helper.sh
Include lib/quickreport/cli/report.sh
Include lib/quickreport/host/probes.sh

setup() {
    TESTDIR=$(mktemp -d)
    OS_KERNEL=Linux
    probe_os_name() { echo 'TestDistro 1.0'; }
    probe_quickemu() { echo 'quickemu'; }
    probe_memory_gb() { echo 32; }
    probe_gpus() { echo ' FakeGPU 3000'; }
    probe_qemu_arch() { echo x86_64; }
    probe_cpu_info() { echo 'fake-cpu-info'; }
}
cleanup() { rm -rf -- "$TESTDIR"; }
BeforeEach 'setup'
AfterEach 'cleanup'

Describe 'quick_report composition'
    It 'renders the full report when all tools are present'
        quickemu() { echo '4.9.9'; }
        qemu-system-x86_64() { echo 'QEMU emulator version 8.2.2'; }
        When call quick_report
        The output should include 'Quickemu 4.9.9'
        The output should include 'Distro:	TestDistro 1.0'
        The output should include 'Memory:	32G'
        The output should include 'FakeGPU 3000'
        The output should include 'curl'
        The output should include 'QEMU'
        The output should include 'CPU'
        The output should include 'fake-cpu-info'
    End
    It 'fails cleanly when quickemu is missing'
        probe_quickemu() { :; }
        When run quick_report
        The status should be failure
        The output should include 'Quickemu missing'
    End
    It 'reports QEMU missing when the binary is absent'
        quickemu() { echo '4.9.9'; }
        command() { case "$2" in qemu-system-*) return 1;; *) builtin command "$@";; esac; }
        When call quick_report
        The output should include 'QEMU missing'
    End
    It 'reports curl missing when the binary is absent'
        quickemu() { echo '4.9.9'; }
        qemu-system-x86_64() { echo 'QEMU emulator version 8.2.2'; }
        command() { case "$2" in curl) return 1;; *) builtin command "$@";; esac; }
        When call quick_report
        The output should include 'curl missing'
    End
    It 'lists multiple GPUs'
        quickemu() { echo '4.9.9'; }
        qemu-system-x86_64() { echo 'QEMU emulator version 8.2.2'; }
        probe_gpus() { printf ' GPU-A\n GPU-B\n'; }
        When call quick_report
        The output should include 'GPUs:'
        The output should include 'GPU-A'
        The output should include 'GPU-B'
    End
End

Describe 'probes'
    It 'probe_qemu_arch maps arm64 to aarch64'
        . lib/quickreport/host/probes.sh   # restore real probes over setup stubs
        uname() { if [ "$1" = '-m' ]; then echo arm64; else command uname "$@"; fi; }
        When call probe_qemu_arch
        The output should eq 'aarch64'
    End
    It 'probe_os_name reads /etc/os-release on Linux'
        . lib/quickreport/host/probes.sh
        OS_KERNEL=Linux
        When call probe_os_name
        The output should not be blank
    End
End
End
