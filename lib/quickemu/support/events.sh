# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Structured event log: one JSON object per line at
# <vmdir>/<vm>.events.jsonl. Append-only, never rotated by quickemu —
# consumers tail it for lifecycle/progress/error notifications.

qe_event() { # type [details-json] -> appends a JSONL record
    local f
    f="${VMDIR}/${VMNAME}.events.jsonl"
    [ -d "${VMDIR}" ] || return 0
    printf '{"ts":"%s","event":"%s","vm":"%s","pid":%s%s}\n' \
        "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$(json_escape "${1}")" \
        "$(json_escape "${VMNAME}")" "${VM_PID:-null}" \
        "${2:+,\"data\":${2}}" >>"${f}" 2>/dev/null || true
}
