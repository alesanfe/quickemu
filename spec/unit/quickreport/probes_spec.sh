# shellcheck disable=SC2034
Describe 'quickreport host probes'
Include lib/quickreport/host/probes.sh

setup() {
    TESTDIR=$(mktemp -d)
    FAKE_BIN="$TESTDIR/bin"; mkdir -p "$FAKE_BIN"
    OS_KERNEL=Linux
}
cleanup() { rm -rf -- "$TESTDIR"; }
BeforeEach 'setup'
AfterEach 'cleanup'

Describe 'probe_os_name'
    It 'reads PRETTY_NAME from os-release on Linux'
        When call probe_os_name
        The output should not be blank
    End
    It 'uses sw_vers on Darwin'
        OS_KERNEL=Darwin
        printf '#!/bin/sh\necho macOS\necho 14.0\n' >"$FAKE_BIN/sw_vers"; chmod +x "$FAKE_BIN/sw_vers"
        PATH="$FAKE_BIN:$PATH"
        When call probe_os_name
        The output should include 'macOS'
    End
    It 'falls back to macOS without sw_vers'
        OS_KERNEL=Darwin
        When call probe_os_name
        The output should eq 'macOS'
    End
End

Describe 'probe_quickemu'
    It 'finds quickemu next to the script'
        mkdir -p "$TESTDIR/qe"; touch "$TESTDIR/qe/quickemu"; chmod +x "$TESTDIR/qe/quickemu"
        When call probe_quickemu "$TESTDIR/qe/report"
        The output should include 'quickemu'
    End
End

Describe 'probe_memory_gb'
    It 'parses free output on Linux'
        printf '#!/bin/sh\necho "               total        used"\necho "Mem:          31Gi       8Gi"\n' >"$FAKE_BIN/free"; chmod +x "$FAKE_BIN/free"
        PATH="$FAKE_BIN:$PATH"
        When call probe_memory_gb
        The output should match pattern '*31*'
    End
    It 'uses sysctl on Darwin'
        OS_KERNEL=Darwin
        printf '#!/bin/sh\necho 34359738368\n' >"$FAKE_BIN/sysctl"; chmod +x "$FAKE_BIN/sysctl"
        PATH="$FAKE_BIN:$PATH"
        When call probe_memory_gb
        The output should eq '32'
    End
End

Describe 'probe_gpus'
    It 'extracts VGA adapters on Linux'
        printf '#!/bin/sh\necho "00:02.0 VGA compatible controller: Intel UHD"\n' >"$FAKE_BIN/lspci"; chmod +x "$FAKE_BIN/lspci"
        PATH="$FAKE_BIN:$PATH"
        When call probe_gpus
        The output should include 'Intel'
    End
    It 'reads chipset model on Darwin'
        OS_KERNEL=Darwin
        printf '#!/bin/sh\necho "      Chipset Model: Apple M2"\n' >"$FAKE_BIN/system_profiler"; chmod +x "$FAKE_BIN/system_profiler"
        PATH="$FAKE_BIN:$PATH"
        When call probe_gpus
        The output should include 'Apple M2'
    End
End

Describe 'probe_qemu_arch'
    It 'maps arm64 to aarch64'
        printf '#!/bin/sh\necho arm64\n' >"$FAKE_BIN/uname"; chmod +x "$FAKE_BIN/uname"
        PATH="$FAKE_BIN:$PATH"
        When call probe_qemu_arch
        The output should eq 'aarch64'
    End
    It 'passes x86_64 through'
        When call probe_qemu_arch
        The output should eq 'x86_64'
    End
End

Describe 'probe_cpu_info'
    It 'uses lscpu on Linux'
        printf '#!/bin/sh\necho "Model name: TestCPU"\n' >"$FAKE_BIN/lscpu"; chmod +x "$FAKE_BIN/lscpu"
        PATH="$FAKE_BIN:$PATH"
        When call probe_cpu_info
        The output should include 'TestCPU'
    End
    It 'uses machdep sysctl on Darwin'
        OS_KERNEL=Darwin
        printf '#!/bin/sh\necho "Apple M2"\n' >"$FAKE_BIN/sysctl"; chmod +x "$FAKE_BIN/sysctl"
        PATH="$FAKE_BIN:$PATH"
        When call probe_cpu_info
        The output should eq 'Apple M2'
    End
End
End
