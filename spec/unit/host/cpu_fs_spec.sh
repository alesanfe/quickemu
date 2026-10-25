# shellcheck disable=SC2034
Describe 'quickemu host cpu extras and filesystem helpers'
Include lib/quickemu/host/cpu.sh
Include lib/quickemu/support/filesystem.sh

Describe 'host_macos_tsc_unstable'
    It 'is false on non-Linux hosts'
        OS_KERNEL=Darwin HOST_CPU_VENDOR=AuthenticAMD guest_os=macos macos_release=sonoma
        When call host_macos_tsc_unstable
        The status should be failure
    End
    It 'is false on Intel hosts'
        OS_KERNEL=Linux HOST_CPU_VENDOR=GenuineIntel guest_os=macos macos_release=sonoma
        When call host_macos_tsc_unstable
        The status should be failure
    End
    It 'is false for non-macos guests'
        OS_KERNEL=Linux HOST_CPU_VENDOR=AuthenticAMD guest_os=linux macos_release=sonoma
        When call host_macos_tsc_unstable
        The status should be failure
    End
    It 'is false for old macos releases'
        OS_KERNEL=Linux HOST_CPU_VENDOR=AuthenticAMD guest_os=macos macos_release=big-sur
        When call host_macos_tsc_unstable
        The status should be failure
    End
End

Describe 'ignore_msrs_alert'
    It 'is silent on Darwin'
        OS_KERNEL=Darwin
        When call ignore_msrs_alert
        The output should be blank
    End
End

Describe 'qe_safe_rm'
    It 'removes an existing file'
        TESTDIR=$(mktemp -d); touch "$TESTDIR/f"
        When call qe_safe_rm "$TESTDIR/f"
        The file "$TESTDIR/f" should not be exist
        rm -rf "$TESTDIR"
    End
    It 'tolerates a missing file'
        When call qe_safe_rm /nonexistent/f
        The status should be success
    End
    It 'tolerates an empty argument'
        When call qe_safe_rm ''
        The status should be success
    End
End

Describe 'get_qemu_share_path'
    It 'returns the prefix share dir when it contains qemu data'
        TESTDIR=$(mktemp -d)
        mkdir -p "$TESTDIR/prefix/bin" "$TESTDIR/prefix/share/qemu"
        printf '#!/bin/sh\nexit 0\n' > "$TESTDIR/prefix/bin/qemu"; chmod +x "$TESTDIR/prefix/bin/qemu"
        QEMU="$TESTDIR/prefix/bin/qemu"
        When call get_qemu_share_path
        The output should eq "$TESTDIR/prefix/share"
        rm -rf "$TESTDIR"
    End
    It 'falls back to /usr/share otherwise'
        TESTDIR=$(mktemp -d)
        mkdir -p "$TESTDIR/prefix/bin"
        printf '#!/bin/sh\nexit 0\n' > "$TESTDIR/prefix/bin/qemu"; chmod +x "$TESTDIR/prefix/bin/qemu"
        QEMU="$TESTDIR/prefix/bin/qemu"
        When call get_qemu_share_path
        The output should eq '/usr/share'
        rm -rf "$TESTDIR"
    End
End
End
