# shellcheck shell=bash
Describe 'support helpers'
Include lib/quickemu/support/filesystem.sh
Include lib/quickemu/support/validation.sh
Include lib/quickemu/support/logging.sh

setup() {
  d=$(mktemp -d)
  VMDIR="$d"
  VMNAME=vm
  unset DRY_RUN QEMU_IMG
}
cleanup() {
  rm -f -- "$d/vm.log" "$d/qemu-img" "$d/file" "$d/link" "$d/-flag" "$d/a b ü"
  rmdir -- "$d"
}
BeforeEach 'setup'
AfterEach 'cleanup'

Describe 'qe_safe_rm'
It 'ignores an empty path without removing other files'
  : > "$d/file"
  When call qe_safe_rm ''
  The status should be success
  The path "$d/file" should be exist
End
It 'removes a symlink without deleting its target'
  : > "$d/file"
  ln -s "$d/file" "$d/link"
  When call qe_safe_rm "$d/link"
  The status should be success
  The path "$d/link" should not be exist
  The path "$d/file" should be exist
End
It 'treats spaces and Unicode as a single path'
  : > "$d/a b ü"
  When call qe_safe_rm "$d/a b ü"
  The status should be success
  The path "$d/a b ü" should not be exist
End
It 'treats option-like names as paths'
  : > "$d/-flag"
  When call qe_safe_rm "$d/-flag"
  The status should be success
  The path "$d/-flag" should not be exist
End
It 'removes an existing file'
  f="$d/file"
  : > "$f"
  When call qe_safe_rm "$f"
  The path "$f" should not be exist
End
It 'is idempotent on a missing path'
  When call qe_safe_rm "$d/missing"
  The status should be success
End
End

Describe 'host_has_tool'
It 'finds sh on PATH'
  When call host_has_tool sh
  The status should be success
End
It 'rejects a missing tool'
  When call host_has_tool __definitely_missing_tool__
  The status should be failure
End
End

Describe 'require_qemu_img'
It 'resolves qemu-img from PATH when unset'
  printf '#!/bin/sh\nexit 0\n' > "$d/qemu-img"
  chmod +x "$d/qemu-img"
  PATH="$d:$PATH"
  When call require_qemu_img
  The status should be success
End
It 'reports an error when QEMU_IMG points nowhere'
  QEMU_IMG=/nonexistent/qemu-img
  When run require_qemu_img
  The status should eq 1
  The output should eq 'ERROR! qemu-img not found. Please install qemu-img.'
End
End

Describe 'vm_log'
It 'appends to the VM log when not in plan mode'
  When call vm_log "hello"
  The contents of file "$d/vm.log" should eq 'hello'
End
It 'writes nothing in plan mode'
  DRY_RUN=1
  When call vm_log "x"
  The status should eq 1
  The path "$d/vm.log" should not be exist
End
End
End
