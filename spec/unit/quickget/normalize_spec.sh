# shellcheck disable=SC2034
Describe 'quickget image normalization'
Include lib/quickget/image/normalize.sh

setup() {
    TESTDIR=$(mktemp -d)
    FAKEBIN="$TESTDIR/bin"; mkdir -p "$FAKEBIN"
    VM_PATH="$TESTDIR/vm"; mkdir -p "$VM_PATH"
    OS='' ISO='' QEMU_IMG=''
    PATH="$FAKEBIN:$PATH"
    # Recording stubs: decompressors create the uncompressed artifact.
    for tool in gzip bzip2 xz zstd 7z unzip; do
        printf '#!/bin/sh\nfor a in "$@"; do case "$a" in -*) ;; *) touch "${a%%.*}" 2>/dev/null;; esac; done\nexit 0\n' > "$FAKEBIN/$tool"
        chmod +x "$FAKEBIN/$tool"
    done
    require_qemu_img() { QEMU_IMG="$FAKEBIN/qemu-img"; }
    printf '#!/bin/sh\necho qemu-img-stub "$@" >> "%s/qemu-img.log"\nexit 0\n' "$TESTDIR" > "$FAKEBIN/qemu-img"
    chmod +x "$FAKEBIN/qemu-img"
}
cleanup() { rm -rf -- "$TESTDIR"; }
BeforeEach 'setup'
AfterEach 'cleanup'

Describe 'image_normalize'
    It 'decompresses batocera .gz and resizes the image'
        OS=batocera ISO=bato.img.gz
        touch "$VM_PATH/bato.img.gz"
        When call image_normalize
        The variable ISO should eq 'bato.img'
        The contents of file "$TESTDIR/qemu-img.log" should include 'resize -f raw'
        The contents of file "$TESTDIR/qemu-img.log" should include '128G'
    End
    It 'leaves uncompressed batocera images alone except resizing'
        OS=batocera ISO=bato.img
        touch "$VM_PATH/bato.img"
        When call image_normalize
        The variable ISO should eq 'bato.img'
    End
    It 'decompresses dragonflybsd .bz2'
        OS=dragonflybsd ISO=dfly.iso.bz2
        touch "$VM_PATH/dfly.iso.bz2"
        When call image_normalize
        The variable ISO should eq 'dfly.iso'
    End
    It 'converts easyos .img to qcow2'
        OS=easyos ISO=easy.img
        touch "$VM_PATH/easy.img"
        When call image_normalize
        The contents of file "$TESTDIR/qemu-img.log" should include 'convert -f raw -O qcow2'
        The contents of file "$TESTDIR/qemu-img.log" should include 'disk.qcow2'
    End
    It 'decompresses and converts openfyde .xz'
        OS=openfyde ISO=fyde.img.xz
        touch "$VM_PATH/fyde.img.xz"
        When call image_normalize
        The variable ISO should eq 'disk.qcow2'
        The contents of file "$TESTDIR/qemu-img.log" should include 'convert'
    End
    It 'decompresses redoxos .zst when zstd exists'
        OS=redoxos ISO=redox.img.zst
        touch "$VM_PATH/redox.img.zst"
        When call image_normalize
        The variable ISO should eq 'redox.img'
        The file "$VM_PATH/redox.img.zst" should not be exist
    End
    It 'warns for redoxos .zst when zstd is missing'
        OS=redoxos ISO=redox.img.zst
        rm -f "$FAKEBIN/zstd"
        command() { if [ "$2" = zstd ]; then return 1; fi; builtin command "$@"; }
        touch "$VM_PATH/redox.img.zst"
        When call image_normalize
        The output should include 'needs installing'
        The variable ISO should eq 'redox.img.zst'
    End
    It 'unzips freedos .zip and picks the extracted iso'
        OS=freedos ISO=fd.zip
        touch "$VM_PATH/fd.zip"
        unzip() { touch "$VM_PATH/FreeDOS.iso"; }
        When call image_normalize
        The variable ISO should eq 'FreeDOS.iso'
        The file "$VM_PATH/fd.zip" should not be exist
    End
    It 'extracts kolibrios .7z when 7z exists'
        OS=kolibrios ISO=kolibri.7z
        touch "$VM_PATH/kolibri.7z"
        rm -f "$FAKEBIN/7z"
        printf '#!/bin/sh\ntouch "%s/kolibri.iso"\nexit 0\n' "$VM_PATH" > "$FAKEBIN/7z"
        chmod +x "$FAKEBIN/7z"
        When call image_normalize
        The variable ISO should eq 'kolibri.iso'
    End
    It 'warns for kolibrios .7z when 7z is missing'
        OS=kolibrios ISO=kolibri.7z
        rm -f "$FAKEBIN/7z"
        When call image_normalize
        The output should include 'needs installing'
    End
    It 'unzips reactos .zip'
        OS=reactos ISO=react.zip
        touch "$VM_PATH/react.zip"
        unzip() { touch "$VM_PATH/ReactOS.iso"; }
        When call image_normalize
        The variable ISO should eq 'ReactOS.iso'
    End
    It 'does nothing for other OSes'
        OS=alpine ISO=alpine.iso
        When call image_normalize
        The output should be blank
        The variable ISO should eq 'alpine.iso'
    End
End
End
