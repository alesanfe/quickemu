# shellcheck disable=SC2034
Describe 'quickemu host probes'
Include lib/quickemu/host/memory.sh
Include lib/quickemu/host/network.sh
Include lib/quickemu/host/display.sh

Describe 'host memory probes'
    It 'reports total memory in MiB'
        When call host_total_mem_mb
        The output should match pattern '[0-9]*'
    End
    It 'reports free memory in MiB'
        When call host_free_mem_mb
        The output should match pattern '[0-9]*'
    End
End

Describe 'host network probes'
    It 'detects the loopback interface'
        When call host_iface_exists lo
        The status should be success
    End
    It 'rejects a nonexistent interface'
        When call host_iface_exists nosuch0
        The status should be failure
    End
    It 'may report a default route interface'
        # outcome is host dependent; just exercise the code path
        probe() { host_default_iface >/dev/null; }
        When call probe
        The status should be success
    End
End

Describe 'check_cocoa_gl_es_support'
    It 'is false on non-Darwin hosts'
        OS_KERNEL=Linux QEMU=/bin/true
        When call check_cocoa_gl_es_support
        The status should be failure
    End
    It 'is false on Darwin when QEMU lacks gl=es'
        OS_KERNEL=Darwin
        QEMU() { echo 'OpenGL support was not enabled'; }
        When call check_cocoa_gl_es_support
        The status should be failure
    End
    It 'is true on Darwin when ANGLE libs exist'
        OS_KERNEL=Darwin
        TESTDIR=$(mktemp -d)
        mkdir -p "$TESTDIR/prefix/bin" "$TESTDIR/prefix/lib"
        printf '#!/bin/sh\nexit 0\n' > "$TESTDIR/prefix/bin/qemu"
        chmod +x "$TESTDIR/prefix/bin/qemu"
        touch "$TESTDIR/prefix/lib/libEGL.dylib"
        QEMU="$TESTDIR/prefix/bin/qemu"
        When call check_cocoa_gl_es_support
        The status should be success
        rm -rf "$TESTDIR"
    End
    It 'is false on Darwin without ANGLE libs'
        OS_KERNEL=Darwin
        TESTDIR=$(mktemp -d)
        mkdir -p "$TESTDIR/prefix/bin" "$TESTDIR/prefix/lib"
        printf '#!/bin/sh\nexit 0\n' > "$TESTDIR/prefix/bin/qemu"
        chmod +x "$TESTDIR/prefix/bin/qemu"
        QEMU="$TESTDIR/prefix/bin/qemu"
        DYLD_LIBRARY_PATH=''
        When call check_cocoa_gl_es_support
        The status should be failure
        rm -rf "$TESTDIR"
    End
End
End
