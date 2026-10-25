# shellcheck disable=SC2034
Describe 'quickget host helpers'
Include lib/quickget/support/host.sh

Describe 'arch_suffix'
    It 'is empty for the native architecture'
        ARCH=amd64 NORMALISED_HOST_ARCH=amd64
        When call arch_suffix
        The output should be blank
    End
    It 'suffixes foreign architectures'
        ARCH=arm64 NORMALISED_HOST_ARCH=amd64
        When call arch_suffix
        The output should eq '-arm64'
    End
End

Describe 'cleanup'
    It 'terminates background jobs'
        sleep 60 &
        When call cleanup
        The status should be success
    End
    It 'is a no-op without jobs'
        When call cleanup
        The status should be success
    End
End

Describe 'require_qemu_img'
    setup() {
        TESTDIR=$(mktemp -d)
        mkdir "$TESTDIR/empty-bin"
        printf '#!/bin/sh\necho stub\n' > "$TESTDIR/qemu-img"
        chmod +x "$TESTDIR/qemu-img"
    }
    cleanup() { qe_spec_cleanup; }
    BeforeEach 'setup'
    AfterEach 'cleanup'

    It 'resolves qemu-img from PATH'
        PATH="$TESTDIR:$PATH"
        When call require_qemu_img
        The variable QEMU_IMG should eq "$TESTDIR/qemu-img"
    End
    It 'exits when qemu-img is missing'
        no_qemuimg() { ( PATH="$TESTDIR/empty-bin"; require_qemu_img ); }
        When run no_qemuimg
        The status should be failure
        The output should include 'qemu-img not found'
    End
End

Describe 'resolve_quickemu'
    setup() {
        TESTDIR=$(mktemp -d)
        mkdir "$TESTDIR/empty-bin"
        printf '#!/bin/sh\necho qe-stub\n' > "$TESTDIR/quickemu"
        chmod +x "$TESTDIR/quickemu"
    }
    cleanup() { qe_spec_cleanup; }
    BeforeEach 'setup'
    AfterEach 'cleanup'

    It 'finds quickemu on PATH'
        PATH="$TESTDIR:$PATH"
        When call resolve_quickemu
        The output should eq "$TESTDIR/quickemu"
    End
    It 'falls back to ./quickemu in the current directory'
        in_subshell() { ( PATH="$TESTDIR/empty-bin"; cd "$TESTDIR" && resolve_quickemu ); }
        When call in_subshell
        The output should eq "$TESTDIR/quickemu"
    End
    It 'fails when quickemu cannot be found anywhere'
        in_subshell() { ( PATH="$TESTDIR/empty-bin"; cd "$TESTDIR/empty-bin" && resolve_quickemu ); }
        When run in_subshell
        The status should be failure
        The error should include 'quickemu not found'
    End
End
End
