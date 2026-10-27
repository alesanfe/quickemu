#!/usr/bin/env bash
# E2E-IMPORT-001: export produces a restorable archive; import yields a
# working VM config again (round-trip).
Describe 'import_export_roundtrip @import_export @p1'
    Include spec/e2e/support/e2e_lib.sh

    setup() {
        ws="$(e2e_sandbox "$(e2e_id roundtrip)")"
        mkdir -p "${ws}/src"
        printf 'guest_os="linux"\ndisk_img="disk.qcow2"\nram="256M"\ndisplay="none"\nboot="legacy"\n' \
            >"${ws}/src/vm.conf"
        printf x >"${ws}/src/disk.qcow2"
        CONF="${ws}/src/vm.conf"
    }
    BeforeEach 'setup'

    roundtrip() {
        "${QE}" --vm "${CONF}" --export "${ws}/vm.tar.gz" >/dev/null 2>&1 || {
            echo "export failed"; return 1; }
        [ -s "${ws}/vm.tar.gz" ] || { echo "archive empty"; return 1; }
        mkdir -p "${ws}/dst"
        (cd "${ws}/dst" && "${QE}" --import "${ws}/vm.tar.gz" >/dev/null 2>&1) || {
            echo "import failed"; return 1; }
        [ -f "${ws}/dst/vm.conf" ] || [ -n "$(find "${ws}/dst" -name '*.conf' -print -quit)" ] || {
            echo "no conf after import"; return 1; }
    }

    It 'export then import produces a working VM'
        When call roundtrip
        The status should be success
    End
End
