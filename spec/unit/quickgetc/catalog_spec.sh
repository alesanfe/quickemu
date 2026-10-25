# shellcheck disable=SC2034
Describe 'quickgetc catalog modules'
Include lib/quickgetc/catalog/arch.sh
Include lib/quickgetc/catalog/check.sh
Include lib/quickgetc/catalog/info.sh
Include lib/quickgetc/catalog/list.sh
Include lib/quickgetc/catalog/query.sh
Include lib/quickgetc/catalog/validate.sh

setup() {
    # Minimal two-OS JSON catalog used across the quickgetc specs.
    JSON='[
      {"name":"testos","pretty_name":"TestOS","guest_os":"linux","homepage":"https://testos.example","description":"A test OS",
       "releases":[
         {"release":"1.0","edition":null,"arch":"x86_64","iso":[{"web":{"url":"https://cdn.testos/1.iso","file_name":"t1.iso"}}]},
         {"release":"2.0","edition":"kde","arch":"x86_64","iso":[{"web":{"url":"https://cdn.testos/2k.iso","file_name":"t2k.iso"}}]},
         {"release":"2.0","edition":"gnome","arch":"x86_64","iso":[{"web":{"url":"https://cdn.testos/2g.iso","file_name":"t2g.iso"}}]},
         {"release":"2.0","edition":"gnome","arch":"aarch64","iso":[{"web":{"url":"https://cdn.testos/2ga.iso","file_name":"t2ga.iso"}}]}
       ]},
      {"name":"otheros","pretty_name":"OtherOS","guest_os":"linux","homepage":"https://other.example","description":"Other",
       "releases":[{"release":"9","edition":null,"arch":"x86_64"}]}
    ]'
    OS='' RELEASE='' EDITION='' ARCH='' PRETTY_NAME=''
    OS_ENTRY='' RELEASE_ENTRIES='' ENTRY='' EDITION_ENTRIES=''
    ARCH=x86_64
}
BeforeEach 'setup'

Describe 'supported_archs / set_arch'
    It 'lists the supported architectures'
        When call supported_archs
        The output should include 'x86_64'
        The output should include 'aarch64'
        The output should include 'riscv64'
    End
    Parameters
        x86_64    x86_64
        amd64     x86_64
        aarch64   aarch64
        arm64     aarch64
        armv8     aarch64
        riscv64   riscv64
        riscv     riscv64
    End
    It "normalises $1 to $2"
        When call set_arch "$1"
        The variable ARCH should eq "$2"
    End
    It 'rejects an unknown architecture'
        When run set_arch sparc
        The status should be failure
        The output should include 'not a supported architecture'
    End
End

Describe 'test_result'
    It 'prints an unresolved URL line without a result'
        When call test_result testos 1.0 '' https://u/x.iso ''
        The output should include 'testos-1.0:'
        The output should include 'https://u/x.iso'
    End
    It 'prints a result line with edition'
        When call test_result testos 2.0 kde https://u/x.iso PASS
        The output should include 'PASS: testos-2.0-kde'
        The output should include 'https://u/x.iso'
    End
End

Describe 'os_support / info queries'
    It 'lists every OS name in the catalog'
        When call os_support
        The output should include 'testos'
        The output should include 'otheros'
    End
    It 'returns the homepage for the selected entry'
        find_os_entry testos
        When call get_homepage
        The output should eq 'https://testos.example'
    End
    It 'returns the description for the selected entry'
        find_os_entry testos
        When call get_description
        The output should eq 'A test OS'
    End
    It 'show_os_info prints name, website and description'
        find_os_entry testos
        When call show_os_info
        The output should include 'TestOS'
        The output should include 'https://testos.example'
        The output should include 'A test OS'
    End
End

Describe 'find_os_entry'
    It 'selects the OS and defaults missing fields'
        When call find_os_entry testos
        The status should be success
        The variable PRETTY_NAME should eq 'TestOS'
        The variable OS_ENTRY should include '"arch":"x86_64"'
    End
    It 'reports an unknown OS and lists the catalog'
        tput() { echo 80; }
        When run find_os_entry nonexistent
        The status should be failure
        The output should include 'not a supported OS'
        The output should include 'testos'
    End
End

Describe 'find_release_entries / find_edition_entry'
    It 'selects all entries for a release'
        find_os_entry testos
        When call find_release_entries 2.0
        The variable RELEASE should eq '2.0'
        The variable RELEASE_ENTRIES should include '"edition":"kde"'
    End
    It 'exits when the release is not in the catalog'
        find_os_entry testos
        tput() { echo 80; }
        When run find_release_entries 99
        The status should be failure
        The output should include 'not a supported release'
    End
    It 'selects the preferred-arch entry for an edition'
        find_os_entry testos
        find_release_entries 2.0
        ARCH=x86_64
        When call find_edition_entry gnome
        The variable ENTRY should include '"file_name":"t2g.iso"'
    End
    It 'falls back to the first entry when no arch matches'
        find_os_entry testos
        find_release_entries 2.0
        ARCH=''
        uname() { echo sparc; }
        When call find_edition_entry kde
        The variable ENTRY should include '"edition":"kde"'
    End
    It 'errors when the requested arch is not offered'
        find_os_entry testos
        find_release_entries 2.0
        ARCH=riscv64
        When run find_edition_entry kde
        The status should be failure
        The output should include 'not a supported architecture'
    End
    It 'errors on an unknown edition'
        find_os_entry testos
        find_release_entries 2.0
        error_not_supported_edition() { echo 'NO-EDITION'; exit 1; }
        When run find_edition_entry bogus
        The status should be failure
        The output should include 'NO-EDITION'
    End
End

Describe 'list emitters'
    It 'list_csv emits a header plus one row per release/edition'
        When run list_csv
        The status should be success
        The output should include 'Display Name,OS,Release,Option,Downloader,PNG,SVG'
        The output should include 'TestOS,testos,1.0'
        The output should include 'TestOS,testos,2.0,gnome'
    End
    It 'list_json converts the CSV to JSON objects'
        When run list_json
        The status should be success
        The output should include '"OS": "testos"'
        The output should include '"Release": "2.0"'
    End
    It 'list_supported prints os/release/edition columns'
        When run list_supported
        The status should be success
        The output should include 'testos 1.0'
        The output should include 'testos 2.0 gnome'
    End
    It 'list_editions returns the editions of a release'
        find_os_entry testos
        When call list_editions 2.0
        The output should include 'kde'
        The output should include 'gnome'
    End
    It 'list_releases_raw deduplicates releases keeping order'
        find_os_entry testos
        When call list_releases_raw
        The output should eq '1.0 2.0'
    End
    It 'list_releases shows per-release editions when they differ'
        find_os_entry testos
        When call list_releases
        The output should include 'Releases'
        The output should include 'gnome kde'
    End
    It 'list_releases shows a flat list when editions are uniform'
        find_os_entry otheros
        When call list_releases
        The output should include '9'
    End
End

Describe 'validation helpers'
    It 'release_requires_edition is true when every entry has an edition'
        find_os_entry testos
        find_release_entries 2.0
        # Drop the no-edition 1.0 by narrowing RELEASE_ENTRIES manually
        RELEASE_ENTRIES=$(jq -c 'select(.edition!=null)' <<< "${RELEASE_ENTRIES}")
        When call release_requires_edition
        The status should be success
    End
    It 'release_requires_edition is false when an entry lacks an edition'
        find_os_entry testos
        find_release_entries 1.0
        When call release_requires_edition
        The status should be failure
    End
    It 'error_no_edition_required fails with the OS name'
        find_os_entry testos; RELEASE=1.0
        When run error_no_edition_required
        The status should be failure
        The output should include 'does not require an edition'
    End
    It 'error_specify_os lists the catalog and fails'
        tput() { echo 80; }
        When run error_specify_os
        The status should be failure
        The output should include 'must specify an operating system'
        The output should include 'testos'
    End
    It 'error_specify_release shows info and releases'
        find_os_entry testos
        tput() { echo 80; }
        When run error_specify_release
        The status should be failure
        The output should include 'TestOS'
        The output should include 'Releases'
    End
    It 'error_specify_edition lists the editions and fails'
        find_os_entry testos; RELEASE=2.0
        tput() { echo 80; }
        When run error_specify_edition
        The status should be failure
        The output should include 'must specify an edition'
    End
    It 'unsupported_arch reports the offered archs'
        find_os_entry testos; RELEASE=2.0; ARCH=riscv64
        find_release_entries 2.0
        EDITION_ENTRIES=$(jq -c 'select(.edition=="gnome")' <<< "${RELEASE_ENTRIES}")
        When run unsupported_arch
        The status should be failure
        The output should include 'riscv64 is not a supported architecture'
        The output should include 'x86_64 aarch64'
    End
End
End
