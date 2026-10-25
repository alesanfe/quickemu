Describe 'disk operations with an isolated image tool'
Include lib/quickemu/vm/disk.sh

setup() {
    TESTDIR=$(mktemp -d)
    VMDIR="$TESTDIR"
    VMNAME='test vm'
    disk_img="$TESTDIR/disk with spaces.qcow2"
    disk_format=qcow2
    DISK_SIZE_ARG=32G
    RUNNING=1
    IMAGE_STATUS=0
    QEMU_IMG=image_tool
    printf original > "$disk_img"
}
cleanup() {
    rm -f -- "$TESTDIR"/*
    rmdir -- "$TESTDIR"
}
vm_running() { return "$RUNNING"; }
require_qemu_img() { :; }
delete_shortcut() { printf called > "$TESTDIR/shortcut-call"; }
image_tool() {
    printf '%s\n' "$@" > "$TESTDIR/image-args"
    if [ "$1" = convert ]; then printf converted > "$5"; fi
    return "$IMAGE_STATUS"
}
BeforeEach 'setup'
AfterEach 'cleanup'

Describe 'resize'
It 'refuses a missing size before invoking the image tool'
    DISK_SIZE_ARG=''
    When run disk_resize
    The status should eq 1
    The output should include 'needs a size'
    The path "$TESTDIR/image-args" should not be exist
    The contents of file "$disk_img" should eq original
End
It 'refuses to resize a running VM'
    RUNNING=0
    When run disk_resize
    The status should eq 1
    The output should include 'before resizing'
    The path "$TESTDIR/image-args" should not be exist
End
It 'refuses a missing image'
    disk_img="$TESTDIR/missing.qcow2"
    When run disk_resize
    The status should eq 1
    The output should include 'not found'
    The path "$TESTDIR/image-args" should not be exist
End
It 'passes the literal disk path and requested size'
    When call disk_resize
    The status should be success
    The output should include 'Resized'
    The line 1 of contents of file "$TESTDIR/image-args" should eq resize
    The line 2 of contents of file "$TESTDIR/image-args" should eq "$disk_img"
    The line 3 of contents of file "$TESTDIR/image-args" should eq 32G
End
It 'reports image tool failure'
    IMAGE_STATUS=1
    When run disk_resize
    The status should eq 1
    The output should include 'Failed to resize'
    The contents of file "$disk_img" should eq original
End
End

Describe 'compact'
It 'refuses a running VM without invoking the tool'
    RUNNING=0
    When run disk_compact
    The status should eq 1
    The output should include 'before compacting'
    The path "$TESTDIR/image-args" should not be exist
End
It 'refuses a missing image'
    disk_img="$TESTDIR/missing.qcow2"
    When run disk_compact
    The status should eq 1
    The output should include 'not found'
    The path "$TESTDIR/image-args" should not be exist
End
It 'replaces the image only after successful conversion'
    When call disk_compact
    The status should be success
    The output should include 'Compacted'
    The contents of file "$disk_img" should eq converted
    The path "$disk_img.compact" should not be exist
    The line 2 of contents of file "$TESTDIR/image-args" should eq '-O'
    The line 3 of contents of file "$TESTDIR/image-args" should eq qcow2
End
It 'keeps the original image and cleans partial output on conversion failure'
    IMAGE_STATUS=1
    When run disk_compact
    The status should eq 1
    The output should include 'Failed to compact'
    The contents of file "$disk_img" should eq original
    The path "$disk_img.compact" should not be exist
End
End

Describe 'delete disk'
It 'refuses to delete the disk of a running VM'
    RUNNING=0
    When run delete_disk
    The status should eq 1
    The output should include 'before deleting'
    The contents of file "$disk_img" should eq original
End
It 'removes only the image and EFI variable files'
    : > "$TESTDIR/OVMF_VARS.fd"
    : > "$TESTDIR/test vm-vars.fd"
    : > "$TESTDIR/keep.iso"
    When call delete_disk
    The status should be success
    The output should include 'Deleted'
    The path "$disk_img" should not be exist
    The path "$TESTDIR/OVMF_VARS.fd" should not be exist
    The path "$TESTDIR/test vm-vars.fd" should not be exist
    The path "$TESTDIR/keep.iso" should be exist
    The path "$TESTDIR/shortcut-call" should be exist
End
It 'does nothing when the image is absent'
    disk_img="$TESTDIR/missing.qcow2"
    When call delete_disk
    The status should be success
    The output should include 'Doing nothing'
    The path "$TESTDIR/shortcut-call" should not be exist
End
End
End
