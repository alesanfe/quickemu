#!/usr/bin/env bash
# Minimal REST shim (2.0 API readiness): unix-socket HTTP responder
# mapping GET /v1/vms to the inventory and GET /v1/vm/<n>/status to
# vm.status envelopes. socat forks per request; the handler emits the
# SAME v1 envelope as the CLI so QuickGUI could switch transports
# with zero contract changes.
SOCK="${1:-/tmp/quickemu-api.sock}"
rm -f "${SOCK}"
handler() {
    read -r REQ REST; read -r _; read -r _
    case "${REQ} ${REST}" in
    "GET /v1/vms")
        BODY=$([ -f "${VMS_DB:-/nonexistent}" ] && cat "${VMS_DB}" || echo '{}') ;;
    "GET /v1/vm/"*)
        VMNAME="${REST#/v1/vm/}"; VMNAME="${VMNAME%%/*}"
        BODY=$(vm_state 2>/dev/null | jq -Rs '{state:.[:-1]}' 2>/dev/null || echo '{"state":"unknown"}') ;;
    *)
        BODY='{"errors":[{"code":"NOT_FOUND"}]}' ;;
    esac
    printf 'HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n%s\n' "${BODY}"
}
export -f handler 2>/dev/null || true
exec socat "UNIX-LISTEN:${SOCK},fork" EXEC:"bash -c handler"
