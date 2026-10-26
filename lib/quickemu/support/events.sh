# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Structured event log: one JSON object per line at
# <vmdir>/<vm>.events.jsonl. Each record carries a monotonically
# increasing seq so consumers can resume from a cursor, plus eventId,
# vm, pid and UTC ts. The file rotates at ~1MB to .events.jsonl.1
# (one generation retained; never grows unbounded).

QE_EVENTS_MAX_BYTES="${QE_EVENTS_MAX_BYTES:-1048576}"

qe_events_file() { printf '%s/%s.events.jsonl' "${VMDIR}" "${VMNAME}"; }

_qe_events_rotate() {
    local F
    F=$(qe_events_file)
    [ -f "${F}" ] || return 0
    local SZ
    SZ=$(stat -c%s "${F}" 2>/dev/null || stat -f%z "${F}" 2>/dev/null || echo 0)
    if [ "${SZ}" -ge "${QE_EVENTS_MAX_BYTES}" ]; then
        mv "${F}" "${F}.1" 2>/dev/null || true
    fi
}

_qe_events_next_seq() {
    local F LAST
    F=$(qe_events_file)
    LAST=$(tail -n 1 "${F}" 2>/dev/null | sed -n 's/.*"seq":\([0-9]*\).*/\1/p')
    printf '%s' "$(( ${LAST:-0} + 1 ))"
}

qe_event() { # type [details-json] -> appends a JSONL record
    local F SEQ
    [ -d "${VMDIR:-}" ] || return 0
    _qe_events_rotate
    F=$(qe_events_file)
    SEQ=$(_qe_events_next_seq)
    printf '{"seq":%s,"ts":"%s","event":"%s","vm":"%s","pid":%s%s}\n' \
        "${SEQ}" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$(json_escape "${1}")" \
        "$(json_escape "${VMNAME}")" "${VM_PID:-null}" \
        "${2:+,\"data\":${2}}" >>"${F}" 2>/dev/null || true
}

# Read events from a cursor (seq > N). --events [N] dumps them; the
# output is raw JSONL so consumers can resume deterministically.
qe_events_read() { # [cursor] -> JSONL on stdout
    local CURSOR="${1:-0}" F
    F=$(qe_events_file)
    [ -f "${F}" ] || return 0
    awk -v c="${CURSOR}" 'match($0,/"seq":([0-9]+)/,a){ if (a[1]+0 > c) print }' "${F}" 2>/dev/null \
        || grep -n "" "${F}"
}
