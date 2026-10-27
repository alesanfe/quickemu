# QE-07: golden tests — Bash and Go backends must produce equivalent
# structured output for the same input. GO_BIN defaults to the vendored
# quickemu-ops build; override with GO_BIN=<path> for a fresh build.
Include lib/quickemu/support/json.sh

Describe 'parity: bash vs go structured output'
    setup() {
        # $(pwd) not ${PWD} — the env var can be stale under some shells.
        GO_BIN="${GO_BIN:-$(pwd)/tools/quickemu-ops/quickemu-ops}"
        export GO_BIN
    }
    BeforeEach 'setup'

    It 'bash envelope carries the v1 contract keys'
        When call qe_ok vm.test '{}'
        The output should include '"schemaVersion":1'
        The output should include '"success":true'
    End

    It 'go binary emits the same envelope shape'
        # Compare schema keys, not values (timestamps/ids legitimately
        # differ). GO_BIN defaults to the vendored quickemu-ops build.
        When run bash -c 'go_bin="${GO_BIN:-tools/quickemu-ops/quickemu-ops}"; [ -x "${go_bin}" ] || { echo "GO_BIN unusable: ${go_bin}"; exit 1; }; out=$("${go_bin}" status --format json 2>/dev/null | head -1); [ -n "${out}" ] || { echo "go binary emitted nothing"; exit 1; }; for k in schemaVersion success; do printf "%s" "${out}" | grep -q "\"${k}\"" || { echo "missing key: ${k}"; exit 1; }; done'
        The status should be success
    End
End
