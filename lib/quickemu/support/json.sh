# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# JSON output helpers (public contract). Owns the schema version emitted by
# every JSON surface so schemas/v1/*.schema.json and the emitters cannot drift.

: "${QE_SCHEMA_VERSION:=1}"

# Stable process exit-code contract (documented in docs/cli/json-output.md).
# Idempotent assignments: re-sourcing this file (tests, nested includes)
# must not fail when the values are already present.
: "${QE_EXIT_OK:=0}"
: "${QE_EXIT_INVALID_ARG:=2}"
: "${QE_EXIT_INVALID_CONFIG:=3}"
: "${QE_EXIT_NOT_FOUND:=4}"
: "${QE_EXIT_INCOMPATIBLE_STATE:=5}"
: "${QE_EXIT_LOCKED:=6}"
: "${QE_EXIT_DEP_MISSING:=7}"
: "${QE_EXIT_PERMISSION:=8}"
: "${QE_EXIT_STORAGE:=9}"
: "${QE_EXIT_NETWORK:=10}"
: "${QE_EXIT_QEMU:=11}"
: "${QE_EXIT_UNSAFE:=12}"
: "${QE_EXIT_PARTIAL:=13}"
: "${QE_EXIT_INTERNAL:=70}"

json_escape() {
    local s="${1//\\/\\\\}"
    s="${s//\"/\\\"}"
    s="${s//$'\n'/\\n}"
    s="${s//$'\t'/\\t}"
    s="${s//$'\r'/\\r}"
    printf '%s' "${s}"
}

# Unified machine-readable envelope:
#   {schemaVersion, command, success, data, warnings[], errors[], meta}
# data/warnings/errors are pre-built JSON fragments (escaped by callers).
qe_emit() { # command success data warnings errors
    printf '{"schemaVersion":%s,"command":"%s","success":%s,"data":%s,"warnings":%s,"errors":%s,"meta":{"quickemuVersion":"%s","timestamp":"%s"}}\n' \
        "${QE_SCHEMA_VERSION}" "$(json_escape "${1}")" "${2}" "${3:-null}" "${4:-[]}" "${5:-[]}" \
        "${VERSION:-unknown}" "$(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || echo unknown)"
}

qe_ok() { qe_emit "${1}" true "${2:-null}" "${3:-[]}" '[]'; }

qe_fail() { # command error_code message [details_json]
    local err
    err=$(printf '[{"code":"%s","message":"%s","severity":"error","retryable":false%s}]' \
        "${2}" "$(json_escape "${3}")" "${4:+,\"details\":${4}}")
    qe_emit "${1}" false 'null' '[]' "${err}"
}

# Central error boundary: JSON-mode callers get a valid error envelope on
# stdout; human mode gets the message on stderr. Always exits.
qe_die() { # command error_code message exit_code
    if [ "${OUTPUT_FORMAT:-}" == "json" ]; then
        qe_fail "${1}" "${2}" "${3}"
    else
        echo "ERROR! ${3}" >&2
    fi
    exit "${4:-${QE_EXIT_INTERNAL}}"
}

# Convenience mappers for the most common failures.
qe_die_locked() { qe_die "${1}" VM_LOCKED "${2:-VM is locked by another operation.}" "${QE_EXIT_LOCKED}"; }
qe_die_running() { qe_die "${1}" VM_ALREADY_RUNNING "${2:-The virtual machine is running.}" "${QE_EXIT_INCOMPATIBLE_STATE}"; }
qe_die_stopped() { qe_die "${1}" VM_MUST_BE_STOPPED "${2:-The virtual machine must be stopped.}" "${QE_EXIT_INCOMPATIBLE_STATE}"; }
qe_die_not_found() { qe_die "${1}" VM_NOT_FOUND "${2:-Virtual machine not found.}" "${QE_EXIT_NOT_FOUND}"; }
qe_die_unsafe() { qe_die "${1}" UNSAFE_ARCHIVE "${2:-Unsafe archive.}" "${QE_EXIT_UNSAFE}"; }
qe_die_invalid() { qe_die "${1}" INVALID_ARGUMENT "${2:-Invalid argument.}" "${QE_EXIT_INVALID_ARG}"; }
