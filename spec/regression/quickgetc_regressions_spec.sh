# shellcheck disable=SC2034
# Regression guards for quickgetc — now exercised through the canonical
# Go entrypoint (bin/quickgetc -> tools/quickgetc):
#  - --list-csv emitted columns in (name, pretty_name) order while the
#    header declared (Display Name, OS).
#  - Unknown OS printed usage but exited 0.
#  - A local JSON catalog override (QUICKGETC_CATALOG) keeps the suite
#    hermetic — no zstd cache download needed.
Describe 'quickgetc regression guards'
Include spec/spec_helper.sh

setup() {
    TESTDIR=$(mktemp -d)
    cat >"$TESTDIR/cat.json" <<'JSON'
[{"name":"zzz","pretty_name":"ZZZ OS","releases":[{"release":"1","edition":null,"arch":"x86_64","url":"http://127.0.0.1/x.iso"}]}]
JSON
    export QUICKGETC_CATALOG="$TESTDIR/cat.json"
}
cleanup() { rm -rf -- "$TESTDIR"; }
BeforeEach 'setup'
AfterEach 'cleanup'

Describe 'list_csv column order matches its header'
    It 'emits pretty_name in the Display Name column'
        When run "${QE_ROOT}/bin/quickgetc" --list-csv
        The output should include 'ZZZ OS,zzz,1'
    End
End

Describe 'unknown OS exits non-zero'
    It 'rejects an OS absent from the catalog'
        When run "${QE_ROOT}/bin/quickgetc" not-a-real-os 1
        The status should be failure
        The output should include 'not a supported OS'
    End
End
End
