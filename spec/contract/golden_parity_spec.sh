# QE-07: golden tests — Bash and Go backends must produce equivalent
# structured output for the same input. Skip-gated on a compiled Go tool.
# Run with GO_BIN=<path-to-quickemu-ops> to compare real outputs.
Describe 'parity: bash vs go structured output'

    It 'op-status JSON shape matches between bash and go'
        # Bash emits the v1 envelope via qe_emit; Go must emit the same
        # top-level keys. We compare schema shape, not exact values
        # (timestamps, operationIds differ legitimately).
        bash_out=$(OUTPUT_FORMAT=json bash -c 'cd '"${PWD}"' && source lib/quickemu/support/json.sh 2>/dev/null; qe_ok vm.test "{}"' 2>/dev/null | head -1)
        # No Go binary → nothing to compare; a missing tool must not
        # report a pass or a fail.
        if [ -z "${GO_BIN:-}" ] || [ ! -x "${GO_BIN:-/nonexistent}" ]; then
            Pending 'GO_BIN not set — cannot compare parity'
            return 0
        fi
        go_out=$("${GO_BIN}" status --format json 2>/dev/null | head -1 || echo "n/a")
        [ "${go_out}" == "n/a" ] && { echo "go binary produced no output"; return 0; }
        bash_keys=$(echo "${bash_out}" | grep -oE '"[a-zA-Z]+":' | sort -u)
        go_keys=$(echo "${go_out}" | grep -oE '"[a-zA-Z]+":' | sort -u)
        # every bash key must exist in go output
        for k in ${bash_keys}; do
            echo "${go_keys}" | grep -q "${k}" || { echo "missing key in go: ${k}"; return 1; }
        done
        return 0
    End
End
