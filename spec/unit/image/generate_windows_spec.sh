# shellcheck disable=SC2034
Describe 'image generation helpers'
Include lib/quickget/image/generate.sh
Include lib/quickget/image/windows.sh

setup() {
    TESTDIR=$(mktemp -d)
    FAKE_BIN="$TESTDIR/bin"; mkdir -p "$FAKE_BIN"
    VM_PATH="$TESTDIR/vm"; mkdir -p "$VM_PATH"
    CONF_FILE="$TESTDIR/vm.conf"; : >"$CONF_FILE"
    HOME="$TESTDIR/home"; mkdir -p "$HOME/.ssh"
    USER=tester I18N='English International'
    web_get() { :; }
}
cleanup() { rm -rf -- "$TESTDIR"; }
BeforeEach 'setup'
AfterEach 'cleanup'

Describe 'gen_seed_iso'
    It 'writes user-data/meta-data and builds the iso via cloud-localds'
        printf '#!/bin/sh\ntouch "$1"\n' >"$FAKE_BIN/cloud-localds"; chmod +x "$FAKE_BIN/cloud-localds"
        PATH="$FAKE_BIN:$PATH"
        echo 'ssh-ed25519 AAAA key' >"$HOME/.ssh/id_ed25519.pub"
        When call gen_seed_iso "$VM_PATH"
        The status should be success
        The file "$VM_PATH/user-data" should be exist
        The file "$VM_PATH/meta-data" should be exist
        The contents of file "$VM_PATH/user-data" should include 'ssh_authorized_keys'
        The contents of file "$CONF_FILE" should include 'seed='
        The output should include 'NoCloud seed.iso created'
    End
    It 'honours a provided user-data file'
        printf '#!/bin/sh\ntouch "$1"\n' >"$FAKE_BIN/cloud-localds"; chmod +x "$FAKE_BIN/cloud-localds"
        PATH="$FAKE_BIN:$PATH"
        echo 'custom' >"$TESTDIR/custom-user-data"
        When call gen_seed_iso "$VM_PATH" "$TESTDIR/custom-user-data"
        The status should be success
        The file "$VM_PATH/user-data" should not be exist
        The output should include 'NoCloud seed.iso created'
    End
    It 'fails when no iso tool exists'
        command() { [ "$1" = -v ] && return 1; builtin command "$@"; }
        When call gen_seed_iso "$VM_PATH"
        The status should be failure
        The output should include 'No tool to build seed.iso'
    End
End

Describe 'iso_extract_bootfiles'
    It 'extracts via bsdtar and normalises names'
        cat >"$FAKE_BIN/bsdtar" <<'EOF'
#!/bin/sh
out=""
while [ $# -gt 0 ]; do [ "$1" = -C ] && out="$2"; shift; done
mkdir -p "$out/casper"; echo k >"$out/casper/vmlinuz"; echo i >"$out/casper/initrd"
EOF
        chmod +x "$FAKE_BIN/bsdtar"
        PATH="$FAKE_BIN:$PATH"
        When call iso_extract_bootfiles iso.img "$VM_PATH" casper
        The status should be success
        The file "$VM_PATH/vmlinuz" should be exist
        The file "$VM_PATH/initrd" should be exist
    End
    It 'fails without extraction tools'
        command() { return 1; }
        When call iso_extract_bootfiles iso.img "$VM_PATH" casper
        The status should be failure
        The output should include 'Need bsdtar'
    End
End

Describe 'gen_ubuntu_autoinstall'
    It 'writes autoinstall user-data, seed and kernel conf'
        cat >"$FAKE_BIN/bsdtar" <<'EOF'
#!/bin/sh
out=""
while [ $# -gt 0 ]; do [ "$1" = -C ] && out="$2"; shift; done
mkdir -p "$out/casper"; echo k >"$out/casper/vmlinuz"; echo i >"$out/casper/initrd"
EOF
        printf '#!/bin/sh\ntouch "$1"\n' >"$FAKE_BIN/cloud-localds"
        printf '#!/bin/sh\necho "$6$hAsh"\n' >"$FAKE_BIN/openssl"
        chmod +x "$FAKE_BIN"/{bsdtar,cloud-localds,openssl}
        PATH="$FAKE_BIN:$PATH"
        When call gen_ubuntu_autoinstall "$VM_PATH" iso.img
        The status should be success
        The contents of file "$VM_PATH/user-data" should include 'autoinstall'
        The contents of file "$CONF_FILE" should include 'kernel='
        The contents of file "$CONF_FILE" should include 'ds=nocloud'
        The output should include 'Unattended install configured'
    End
    It 'fails when extraction fails'
        command() { return 1; }
        When call gen_ubuntu_autoinstall "$VM_PATH" iso.img
        The status should be failure
        The output should include 'Could not extract'
    End
End

Describe 'is_unattended_windows_iso'
    Parameters
        'Win10_22H2_English_x64.iso' success
        'Win11_23H2_EnglishInternational_x64v2.iso' success
        'Win11_24H2_Spanish_x32.iso' success
        'ubuntu-24.04.iso' failure
        'Win10_x64.img' failure
    End
    It 'classifies $1'
        When call is_unattended_windows_iso "$1"
        The status should be "$2"
    End
End

Describe 'unattended_windows'
    It 'generates autounattend.xml with the right culture'
        mkisofs() { :; }
        I18N='Spanish'
        When call unattended_windows "$VM_PATH" w10
        The status should be success
        The contents of file "$VM_PATH/unattended/autounattend.xml" should include 'es-ES'
        The contents of file "$VM_PATH/unattended/autounattend.xml" should include 'qxldod'
        The output should include 'unattended.iso'
    End
    It 'omits qxldod for Windows 11 driver dir'
        mkisofs() { :; }
        I18N='Klingon'
        When call unattended_windows "$VM_PATH" w11
        The contents of file "$VM_PATH/unattended/autounattend.xml" should not include 'qxldod'
        The contents of file "$VM_PATH/unattended/autounattend.xml" should include 'en-US'
        The output should include 'unattended.iso'
    End
End
End
