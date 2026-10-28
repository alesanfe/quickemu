# shellcheck disable=SC2034
# Functional tests: run the real bin/quickget entry point (Go backend
# via the public wrapper) covering CLI dispatch, exit codes and output
# contract without upstream network or QEMU.
Describe 'quickget entry point'
Include spec/spec_helper.sh

setup() {
    TESTDIR=$(mktemp -d)
    FAKEBIN="$TESTDIR/bin"; mkdir -p "$FAKEBIN"
    cp "${QE_ROOT}/spec/fake-bin/qemu-img" "$FAKEBIN/"
    printf '#!/bin/sh\nexit 0\n' > "$FAKEBIN/xdg-open"
    chmod +x "$FAKEBIN"/*
    export PATH="$FAKEBIN:$PATH" FIXTURES="${QE_ROOT}/spec/fixtures"
    qg() { ( cd "$TESTDIR" && "${QE_ROOT}/bin/quickget" "$@" ); }
}
cleanup() {
    [ -f "$TESTDIR/srv.pid" ] && kill "$(cat "$TESTDIR/srv.pid")" 2>/dev/null
    pkill -f "http.server 18743" 2>/dev/null || true
    rm -rf -- "$TESTDIR"
}
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
    It '--check probes URLs (network-dependent)'
        When run qg --check alma 9 minimal
        The status should be success
        The output should include 'alma-'
    End
    It '--check honours --arch (network-dependent)'
        When run qg --arch arm64 --check alma 9 minimal
        The status should be success
        The output should include 'alma-9'
    End
    It '--check-all-arch tests both architectures (network-dependent)'
        When run qg --check-all-arch alma 9 minimal
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
        # Trust probe is live-network dependent: stderr may carry a
        # 'trust D' integrity warning (or nothing when online).
        The stderr should match pattern '*'
    End
    It 'requires an edition for editioned OSes'
        When run qg alma 9
        The status should be failure
        The output should include 'must specify an edition'
    End
    It 'rejects an unsupported edition'
        When run qg alma 9 bogus
        The status should be failure
        The output should include 'not a supported'
        The stderr should match pattern '*'
    End
    It 'warns about unverifiable sources on stderr'
        When run qg debian 9999
        The status should be failure
        The stderr should include 'integrity not verifiable'
        The output should include 'not a supported release'
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
    It 'fetches a remote image over HTTP and generates the conf'
        Skip if "python3 not on PATH" sh -c '! command -v python3 >/dev/null'
        printf 'iso-bytes' > "$TESTDIR/tiny.iso"
        # Kill any orphan on the port; serve detached with every fd
        # >=3 closed so the server never holds a harness pipe open
        # (ShellSpec executor wedges on EOF otherwise).
        pkill -f "http.server 18743" 2>/dev/null || true
        ( cd "$TESTDIR" && exec 3>&- 4>&- 5>&- 6>&- 7>&- 8>&- 9>&- \
            && nohup python3 -m http.server 18743 </dev/null >/dev/null 2>&1 \
            & echo $! >"$TESTDIR/srv.pid" )
        sleep 1
        When run qg --create-config dlvm "http://127.0.0.1:18743/tiny.iso"
        The status should be success
        The output should match pattern '*'
        The file "$TESTDIR/dlvm.conf" should be exist
        The file "$TESTDIR/dlvm/tiny.iso" should be exist
        The contents of file "$TESTDIR/dlvm.conf" should include 'guest_os="linux"'
    End
End
End
