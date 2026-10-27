# shellcheck disable=SC2034
# Functional tests: run the real bin/quickget entry point in-process
# (sourced in a subshell) covering CLI dispatch, exit codes and output
# contract without network or QEMU.
Describe 'quickget entry point'
Include spec/spec_helper.sh

setup() {
    TESTDIR=$(mktemp -d)
    FAKEBIN="$TESTDIR/bin"; mkdir -p "$FAKEBIN"
    cp "${QE_ROOT}/spec/fake-bin/curl" "${QE_ROOT}/spec/fake-bin/qemu-img" "$FAKEBIN/"
    printf '#!/bin/sh\necho quickemu-stub 4.9.9\n' > "$FAKEBIN/quickemu"
    printf '#!/bin/sh\necho 80\n' > "$FAKEBIN/tput"
    printf '#!/bin/sh\nexit 0\n' > "$FAKEBIN/xdg-open"
    chmod +x "$FAKEBIN"/*
    export PATH="$FAKEBIN:$PATH" FIXTURES="${QE_ROOT}/spec/fixtures"
    qg() { ( cd "$TESTDIR" && . "${QE_ROOT}/bin/quickget" ); }
}
cleanup() { rm -rf -- "$TESTDIR"; }
BeforeEach 'setup'
AfterEach 'cleanup'

Describe 'informational operations'
    It '--version delegates to quickemu'
        When run qg --version
        The status should be success
        The output should match pattern '*.*'
    End
    It '--help prints usage'
        When run qg --help
        The status should be success
        The output should include 'quickget'
    End
    It '--show prints details for one OS'
        When run qg --show alpine
        The status should be success
        The output should include 'Alpine Linux'
        The output should include 'alpinelinux.org'
    End
    It '--show with no args iterates the catalogue'
        When run qg --show
        The status should be success
        The output should include 'Alpine Linux'
        The output should include 'Zorin'
    End
    It '--open-homepage opens the OS homepage'
        When run qg --open-homepage alpine
        The status should be success
    End
    It '--open-homepage rejects unknown OSes'
        When run qg --open-homepage nosuchos
        The status should be failure
        The output should include 'must specify an operating system'
    End
End

Describe 'catalogue listings'
    It '--list prints supported systems'
        When run qg --list
        The status should be success
        The output should include 'alma'
    End
    It '--list-csv prints the CSV catalogue'
        When run qg --list-csv
        The status should be success
        The output should include 'Display Name,OS,Release'
    End
    It '--list-json prints JSON'
        When run qg --list-json
        The status should be success
        The output should include '"OS":'
    End
    It 'plain list alias still works'
        When run qg list
        The status should be success
        The output should include 'alma'
    End
End

Describe 'url and check operations'
    It '--url prints image URLs for an OS'
        When run qg --url alma
        The status should be success
        The output should include 'alma-'
    End
    It '--check probes URLs through curl'
        When run qg --check alma
        The status should be success
        The output should include 'alma-'
    End
    It '--check honours --arch'
        When run qg --arch arm64 --check alma
        The status should be success
        The output should include 'alma-9'
    End
    It '--check-all-arch tests both architectures'
        When run qg --check-all-arch alma
        The status should be success
        The output should include 'alma-'
    End
End

Describe 'selection validation'
    It 'rejects unsupported flags'
        When run qg --bogus
        The status should be failure
        The output should include 'Not supported argument'
    End
    It 'rejects an unsupported architecture'
        When run qg --arch riscv alpine
        The status should be failure
        The output should include 'Unsupported architecture'
    End
    It 'rejects an unsupported OS'
        When run qg nosuchos 1
        The status should be failure
        The output should include 'not a supported OS'
    End
    It 'requires a release'
        When run qg alma
        The status should be failure
        The output should include 'must specify a release'
        The output should include 'AlmaLinux'
    End
    It 'rejects an unsupported release'
        When run qg alma 7
        The status should be failure
        The output should include 'not a supported release'
        The stderr should include 'integrity'
    End
    It 'requires an edition for editioned OSes'
        When run qg alma 9
        The status should be failure
        The output should include 'must specify an edition'
        The stderr should include 'integrity'
    End
    It 'rejects an unsupported edition'
        When run qg alma 9 bogus
        The status should be failure
        The output should include 'not a supported'
        The stderr should include 'integrity'
    End
End

Describe 'create-config operation'
    It 'builds a conf around a local ISO'
        printf 'iso-bytes' > "$TESTDIR/my-linux.iso"
        When run qg --create-config custom "$TESTDIR/my-linux.iso"
        The output should include 'custom.conf'
        The file "$TESTDIR/custom.conf" should be exist
        The contents of file "$TESTDIR/custom.conf" should include 'guest_os="linux"'
    End
    It 'detects windows guests and can skip unattended setup'
        printf 'iso-bytes' > "$TESTDIR/random.iso"
        When run qg --create-config winvm "$TESTDIR/random.iso" --disable-unattended
        The output should include 'Selecting OS'
    End
    It 'rejects a missing image'
        When run qg --create-config newvm "$TESTDIR/gone.iso"
        The status should be failure
        The output should include 'valid URL or path'
    End
    It 'rejects an existing VM directory'
        mkdir "$TESTDIR/existing"
        printf 'x' > "$TESTDIR/a.iso"
        When run qg --create-config existing "$TESTDIR/a.iso"
        The status should be failure
        The output should include 'Could not create directory'
    End
    It 'rejects non-image extensions'
        printf 'x' > "$TESTDIR/readme.txt"
        When run qg --create-config txtvm "$TESTDIR/readme.txt"
        The status should be failure
        The output should include 'ISO,IMG and QCOW2'
    End
End

Describe 'download flow'
    It 'generates the conf for a full os/release/edition selection'
        When run qg alma 9 dvd
        The status should be success
        The output should include 'Downloading AlmaLinux 9 dvd'
        The stderr should include 'integrity'
        The file "$TESTDIR/alma-9-dvd.conf" should be exist
        The contents of file "$TESTDIR/alma-9-dvd.conf" should include 'guest_os="linux"'
    End
End
End
