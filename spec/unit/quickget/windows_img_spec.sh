# shellcheck disable=SC2034
Describe 'quickget windows image helpers'
Include lib/quickget/image/windows.sh

setup() {
    TESTDIR=$(mktemp -d)
    VM_PATH="$TESTDIR/vm"
    mkdir -p "$VM_PATH"
    I18N='English International'
    web_get() { :; }
    mkisofs() { touch "${@: -2:1}/../unattended.iso" 2>/dev/null || :; }
}
cleanup() { rm -rf -- "$TESTDIR"; }
BeforeEach 'setup'
AfterEach 'cleanup'

Describe 'is_unattended_windows_iso'
    Parameters
        'Win10_22H2_English_x64.iso'    0
        'Win11_23H2_English_x64.iso'    0
        'Win11_24H2_English_x64v1.iso'  0
        'windows-11.iso'                1
        'Win11_English_x64.iso'         1
        'my-random-file.iso'            1
    End
    It "returns $2 for $1"
        When call is_unattended_windows_iso "$1"
        The status should eq "$2"
    End
End

Describe 'unattended_windows'
    It 'writes an autounattend.xml with the qxldod path for w10'
        When call unattended_windows "$VM_PATH" w10
        The output should include 'Making unattended.iso'
        The file "$VM_PATH/unattended/autounattend.xml" should be exist
        The contents of file "$VM_PATH/unattended/autounattend.xml" should include 'qxldod'
        The contents of file "$VM_PATH/unattended/autounattend.xml" should include 'BypassTPMCheck'
        The contents of file "$VM_PATH/unattended/autounattend.xml" should include 'en-GB'
    End
    It 'omits qxldod for w11'
        When call unattended_windows "$VM_PATH" w11
        The output should include 'Making unattended.iso'
        The contents of file "$VM_PATH/unattended/autounattend.xml" should not include 'qxldod'
    End
    It 'honours the requested language'
        I18N='Spanish'
        When call unattended_windows "$VM_PATH" w10
        The output should include 'Making unattended.iso'
        The contents of file "$VM_PATH/unattended/autounattend.xml" should include 'es-ES'
        The contents of file "$VM_PATH/unattended/autounattend.xml" should include '0c0a'
    End
    It 'builds unattended.iso via mkisofs'
        mkisofs() { echo "MKISOFS $*"; touch "$VM_PATH/unattended.iso"; }
        When call unattended_windows "$VM_PATH" w10
        The output should include 'Making unattended.iso'
        The file "$VM_PATH/unattended.iso" should be exist
    End
End
End
