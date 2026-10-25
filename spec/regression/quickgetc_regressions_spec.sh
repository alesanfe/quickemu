# shellcheck disable=SC2034
# Regression tests for bugs found while building this suite:
#  - list_csv emitted columns in (name, pretty_name) order while the
#    header declared (Display Name, OS).
#  - find_edition_entry indexed a JSON *stream* with '.[0]' which fails
#    at runtime (needed -s slurp).
#  - error_not_supported_edition / error_not_supported_argument /
#    test_all / create_config / --check were referenced by cli/main.sh
#    but undefined (command-not-found at runtime).
#  - make_vm_config wrote the literal text '${IMAGES["iso"]}' into conf
#    files instead of the image path, and wrote guest_os="null".
#  - find_os_entry / error_specify_release did not exit on error, so
#    unknown OSes printed usage but exited 0.
Describe 'quickgetc regression guards'
Include spec/spec_helper.sh
Include lib/quickgetc/catalog/list.sh
Include lib/quickgetc/catalog/query.sh

setup() {
    TESTDIR=$(mktemp -d)
}
cleanup() { rm -rf -- "$TESTDIR"; }
BeforeEach 'setup'
AfterEach 'cleanup'

Describe 'every function cli/main.sh calls is defined'
    It 'all entry-called functions resolve to real definitions'
        check_defined() {
            local f missing=0
            for f in open_homepage populate_cache supported_archs set_arch \
                     create_config show_os_info help_message test_all \
                     list_csv list_json list_supported error_not_supported_argument \
                     find_os_entry find_release_entries release_requires_edition \
                     find_edition_entry error_specify_edition error_no_edition_required \
                     error_specify_os error_specify_release create_vm error_not_supported_edition; do
                local src
                src=$(grep -rl "function ${f}\|^${f}()" "$QE_ROOT/lib/quickgetc")
                if [ -z "$src" ]; then echo "MISSING: $f"; missing=1; fi
            done
            return "$missing"
        }
        When call check_defined
        The status should be success
        The output should be blank
    End
End

Describe 'list_csv column order matches its header'
    It 'emits pretty_name in the Display Name column'
        JSON='[{"name":"zzz","pretty_name":"ZZZ OS","releases":[{"release":"1","edition":null}]}]'
        When run list_csv
        The output should include 'ZZZ OS,zzz,1'
    End
End

Describe 'stream fallback in find_edition_entry'
    It 'does not crash indexing a jq stream'
        JSON='[{"name":"t","pretty_name":"T","releases":[{"release":"1","edition":"e","arch":"ppc64"}]}]'
        OS_ENTRY='{"name":"t","releases":[{"release":"1","edition":"e","arch":"ppc64"}]}'
        find_os_entry t
        find_release_entries 1
        EDITION_ENTRIES=$(jq -c 'select(.edition=="e")' <<< "$RELEASE_ENTRIES")
        ARCH=''
        uname() { echo mips; }
        When call find_edition_entry e
        The variable ENTRY should include '"edition":"e"'
    End
End
End
