#!/usr/bin/env bash
# E2E-COMPAT-001: bash and Go backends emit the same structured envelope
# shape for operation status. Uses the prebuilt tools/quickemu-ops binary.
Describe 'bash_go_parity @compatibility @p1'
    Include spec/e2e/support/e2e_lib.sh

    setup() {
        ws="$(e2e_sandbox "$(e2e_id parity)")"
        vm_fixture "$ws" >/dev/null
        CONF="$ws/vm.conf"
        GO_BIN="$(pwd)/tools/quickemu-ops/quickemu-ops"
    }
    BeforeEach 'setup'

    go_missing() { [ ! -x "${GO_BIN}" ]; }

    parity_check() {
        bash_out=$("${QE}" --vm "${CONF}" --op-status no-such-op --format json 2>/dev/null)
        go_out=$("${GO_BIN}" status --vmdir "${ws}" no-such-op 2>/dev/null)
        [ -n "${bash_out}" ] || { echo "bash emitted nothing"; return 1; }
        [ -n "${go_out}" ] || { echo "go emitted nothing"; return 1; }
        # Parity contract: both emit schemaVersion + success + structured
        # errors[]. The Go error envelope is intentionally slimmer (no
        # command/data/warnings/meta); the contract keys must match.
        for src in bash go; do
            eval "out=\${${src}_out}"
            printf '%s' "${out}" | jq -e '.schemaVersion == 1 and (.success | type == "boolean") and (.errors | type == "array")' \
                >/dev/null 2>&1 || { echo "${src} envelope violates contract: ${out}"; return 1; }
        done
    }

    It 'bash and go emit the same envelope keys on error paths'
        Skip if 'quickemu-ops binary unavailable' go_missing
        When call parity_check
        The status should be success
    End
End
