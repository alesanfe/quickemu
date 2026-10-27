# shellcheck disable=SC2034
# Functional tests: run the real bin/quickgetc entry point in-process
# (sourced in a subshell) so argument dispatch, exit codes and module
# wiring are exercised end to end without network access.
Describe 'quickgetc entry point'
Include spec/spec_helper.sh

setup() {
    TESTDIR=$(mktemp -d)
    FAKEBIN="$TESTDIR/bin"; mkdir -p "$FAKEBIN"
    export XDG_CACHE_HOME="$TESTDIR/cache"
    mkdir -p "$XDG_CACHE_HOME"

    # Same-day catalog cache; the fake zstd decompresses by cat(1).
    cat > "$XDG_CACHE_HOME/quickget_data.json.zst" <<'JSON'
[
  {"name":"testos","pretty_name":"TestOS","guest_os":"linux","homepage":"https://testos.example","description":"A test OS",
   "releases":[
     {"release":"1.0","edition":null,"arch":"x86_64","ram":4294967296,
      "iso":[{"web":{"url":"https://cdn.testos/1.iso","file_name":"t1.iso"}}]},
     {"release":"2.0","edition":"kde","arch":"x86_64",
      "iso":[{"web":{"url":"https://cdn.testos/2k.iso","file_name":"t2k.iso"}}]},
     {"release":"2.0","edition":"gnome","arch":"x86_64",
      "iso":[{"web":{"url":"https://cdn.testos/2g.iso","file_name":"t2g.iso"}}]}
   ]},
  {"name":"otheros","pretty_name":"OtherOS","guest_os":"linux","homepage":"https://other.example","description":"Other",
   "releases":[{"release":"9","edition":null,"arch":"x86_64",
      "iso":[{"web":{"url":"https://cdn.other/9.iso","file_name":"o9.iso"}}]}]}
]
JSON

    printf '#!/bin/sh\nwhile [ $# -gt 1 ]; do shift; done\nexec cat "$1"\n' > "$FAKEBIN/zstd"
    printf '#!/bin/sh\necho quickemu 4.9.9-stub\n' > "$FAKEBIN/quickemu"
    printf '#!/bin/sh\necho 80\n' > "$FAKEBIN/tput"
    printf '#!/bin/sh\nexit 0\n' > "$FAKEBIN/xdg-open"
    cp "${QE_ROOT}/spec/fake-bin/curl" "${QE_ROOT}/spec/fake-bin/qemu-img" "$FAKEBIN/"
    chmod +x "$FAKEBIN"/*
    export PATH="$FAKEBIN:$PATH" FIXTURES="${QE_ROOT}/spec/fixtures"

    # Source the real entry inside a subshell: args are the function's
    # positional params, exit() is contained, coverage is still traced.
    qgc() { ( cd "$TESTDIR" && . "${QE_ROOT}/bin/quickgetc.bash" ); }
}
cleanup() { rm -rf -- "$TESTDIR"; }
BeforeEach 'setup'
AfterEach 'cleanup'

Describe 'informational operations'
    It '--version prints the version'
        When run qgc --version
        The status should be success
        The output should include '0.1.0'
    End
    It '--help prints usage'
        When run qgc --help
        The status should be success
        The output should include 'quickgetc <os> <release>'
        The output should include '--download'
    End
    It '--list prints supported systems'
        When run qgc --list
        The status should be success
        The output should include 'testos'
        The output should include 'otheros'
    End
    It '--list-csv prints the CSV catalogue'
        When run qgc --list-csv
        The status should be success
        The output should include 'Display Name,OS,Release'
        The output should include 'TestOS,testos,1.0'
    End
    It '--list-json prints JSON'
        When run qgc --list-json
        The status should be success
        The output should include '"OS": "testos"'
    End
    It '--show prints OS details'
        When run qgc --show testos
        The status should be success
        The output should include 'TestOS'
        The output should include 'https://testos.example'
    End
    It '--show without args prints every OS'
        When run qgc --show
        The status should be success
        The output should include 'TestOS'
        The output should include 'OtherOS'
    End
    It '--open-homepage resolves the homepage'
        When run qgc --open-homepage testos
        The status should be success
    End
    It '--open-homepage reports OSes without a homepage'
        When run qgc --open-homepage ghost-os
        The status should be failure
        The output should include 'not a supported OS'
    End
End

Describe 'url and check operations'
    It '--url lists image URLs for an OS'
        When run qgc --url testos
        The status should be success
        The output should include 'https://cdn.testos/1.iso'
        The output should include 'https://cdn.testos/2k.iso'
    End
    It '--url with no OS iterates the whole catalog'
        When run qgc --url
        The status should be success
        The output should include 'testos-1.0'
        The output should include 'otheros-9'
    End
    It '--check probes URLs through curl'
        When run qgc --check testos
        The status should be success
        The output should include 'testos-1.0'
    End
End

Describe 'vm creation flow'
    It 'creates a VM dir and conf for a release'
        When run qgc testos 1.0
        The status should be success
        The output should include 'Making testos-1.0.conf'
        The file "$TESTDIR/testos-1.0.conf" should be exist
        The contents of file "$TESTDIR/testos-1.0.conf" should include 'guest_os="linux"'
        The contents of file "$TESTDIR/testos-1.0.conf" should include 'ram=4'
    End
    It 'requires an edition when every entry has one'
        When run qgc testos 2.0
        The status should be failure
        The output should include 'must specify an edition'
    End
    It 'resolves the requested edition'
        When run qgc testos 2.0 kde
        The output should include 'Making testos-2.0-kde.conf'
        The file "$TESTDIR/testos-2.0-kde.conf" should be exist
    End
    It 'rejects an edition on a release without editions'
        When run qgc testos 1.0 kde
        The status should be failure
        The output should include 'does not require an edition'
    End
    It 'rejects an unknown edition'
        When run qgc testos 2.0 bogus
        The status should be failure
        The output should include 'not a supported TestOS 2.0 edition'
    End
    It 'reports an unknown OS'
        When run qgc nosuchos 1.0
        The status should be failure
        The output should include 'not a supported OS'
    End
    It 'reports an unknown release'
        When run qgc testos 99
        The status should be failure
        The output should include 'not a supported release'
    End
    It '--download skips conf generation'
        When run qgc --download testos 1.0
        The status should be success
        The file "$TESTDIR/testos-1.0.conf" should not be exist
    End
    It '--arch sets the image architecture suffix'
        When run qgc --arch x86_64 testos 1.0
        The output should include 'testos-1.0-x86_64'
        The file "$TESTDIR/testos-1.0-x86_64.conf" should be exist
    End
    It '--arch rejects unknown architectures'
        When run qgc --arch sparc testos 1.0
        The status should be failure
        The output should include 'not a supported architecture'
    End
    It '--arch without value fails'
        When run qgc --arch
        The status should be failure
        The output should include 'must specify an architecture'
    End
End

Describe 'create-config operation'
    It 'builds a conf around a local ISO'
        printf 'iso-bytes' > "$TESTDIR/my-linux.iso"
        When run qgc --create-config custom "$TESTDIR/my-linux.iso"
        The output should include 'Creating custom.conf'
        The file "$TESTDIR/custom.conf" should be exist
        The contents of file "$TESTDIR/custom.conf" should include 'guest_os="linux"'
        The contents of file "$TESTDIR/custom.conf" should include 'iso='
        The file "$TESTDIR/custom/my-linux.iso" should be exist
    End
    It 'detects windows guests from the file name'
        printf 'iso-bytes' > "$TESTDIR/Win11_23H2.iso"
        When run qgc --create-config winvm "$TESTDIR/Win11_23H2.iso"
        The status should be success
        The output should include 'guest_os=windows'
        The contents of file "$TESTDIR/winvm.conf" should include 'guest_os="windows"'
    End
    It 'maps qcow2 images to disk_img'
        printf 'qcow-bytes' > "$TESTDIR/disk.qcow2"
        When run qgc --create-config qcowvm "$TESTDIR/disk.qcow2"
        The status should be success
        The output should include 'Creating qcowvm.conf'
        The contents of file "$TESTDIR/qcowvm.conf" should include 'disk_img='
    End
    It 'rejects missing inputs'
        When run qgc --create-config newvm
        The status should be failure
        The output should include 'requires a VM name'
    End
    It 'rejects an existing directory'
        mkdir "$TESTDIR/existing"
        printf 'x' > "$TESTDIR/a.iso"
        When run qgc --create-config existing "$TESTDIR/a.iso"
        The status should be failure
        The output should include 'Could not create directory'
    End
    It 'rejects non-image files'
        printf 'x' > "$TESTDIR/readme.txt"
        When run qgc --create-config txtvm "$TESTDIR/readme.txt"
        The status should be failure
        The output should include 'ISO, IMG and QCOW2'
    End
    It 'rejects a missing local file'
        When run qgc --create-config gonevm "$TESTDIR/gone.iso"
        The status should be failure
        The output should include 'valid URL or path'
    End
End

Describe 'argument validation'
    It 'rejects unsupported flags'
        When run qgc --bogus
        The status should be failure
        The output should include 'Not supported argument'
    End
    It 'requires an OS'
        When run qgc
        The status should be failure
        The output should include 'must specify an operating system'
    End
    It 'requires a release'
        When run qgc testos
        The status should be failure
        The output should include 'TestOS'
        The output should include 'Releases'
    End
End

Describe 'cache handling'
    It 're-downloads the catalog when the cache is stale'
        touch -d '3 days ago' "$XDG_CACHE_HOME/quickget_data.json.zst"
        cat > "$FAKEBIN/curl" <<'EOF'
#!/bin/sh
out=''
while [ $# -gt 0 ]; do
    case "$1" in -o|--output) out="$2"; shift 2;; *) shift;; esac
done
if [ -n "$out" ] && [ -d "$(dirname "$out")" ]; then
    printf '[{"name":"fresh","pretty_name":"Fresh","releases":[{"release":"1","edition":null,"arch":"x86_64"}]}]' > "$out"
fi
exit 0
EOF
        chmod +x "$FAKEBIN/curl"
        When run qgc --list-csv
        The status should be success
        The output should include 'Downloading quickget data'
        The output should include 'Fresh,fresh'
    End
    It 'fails cleanly when the cache cannot be downloaded nor decompressed'
        touch -d '3 days ago' "$XDG_CACHE_HOME/quickget_data.json.zst"
        printf '#!/bin/sh\nexit 22\n' > "$FAKEBIN/curl"
        printf '#!/bin/sh\nexit 1\n' > "$FAKEBIN/zstd"
        chmod +x "$FAKEBIN/curl" "$FAKEBIN/zstd"
        When run qgc --version
        The status should be failure
        The output should include 'Failed to decompress'
    End
End
End
