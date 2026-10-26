# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# quickemu doctor: diagnoses stale/corrupt state; --fix repairs what's
# safe (never deletes VM disks or conf). Idempotent; typed findings.

qe_doctor() { # [--fix] -> findings to stdout
    local FIX="${1:-}" FINDINGS=() N=0
    # stale runtime.json (schema version mismatch / unparseable)
    local RT="${VMDIR}/${VMNAME}.runtime.json"
    if [ -f "${RT}" ] && ! grep -q '"schemaVersion"' "${RT}" 2>/dev/null; then
        FINDINGS+=("runtime-corrupt:${RT}")
        [ "${FIX}" = "--fix" ] && { rm -f "${RT}"; FINDINGS+=("fixed:removed-corrupt-runtime"); }
    fi
    # stale pid file pointing at a dead process
    local PF="${VMDIR}/${VMNAME}.pid"
    if [ -f "${PF}" ]; then
        local P
        P=$(tr -d '[:space:]' <"${PF}" 2>/dev/null)
        if [ -n "${P}" ] && ! kill -0 "${P}" 2>/dev/null; then
            FINDINGS+=("stale-pidfile:${P}")
            [ "${FIX}" = "--fix" ] && { rm -f "${PF}" "${VMDIR}/${VMNAME}.paused"; FINDINGS+=("fixed:removed-stale-pid"); }
        fi
    fi
    # stale locks with dead owners
    local L
    for L in "${VMDIR}/${VMNAME}.lock" "${VMDIR}/${VMNAME}.lock.shared"/*; do
        [ -e "${L}" ] || continue
        local LP
        LP=$(cat "${L}/pid" 2>/dev/null || basename "${L}")
        if [[ "${LP}" =~ ^[0-9]+$ ]] && ! kill -0 "${LP}" 2>/dev/null; then
            FINDINGS+=("stale-lock:${L}")
            [ "${FIX}" = "--fix" ] && { rm -rf "${L}"; FINDINGS+=("fixed:reclaimed-lock"); }
        fi
    done
    # stale sockets with no live pid
    for L in "${VMDIR}/${VMNAME}"*.socket "${VMDIR}/${VMNAME}.monitor" "${VMDIR}/${VMNAME}.qmp"; do
        [ -S "${L}" ] || continue
        if [ ! -f "${PF}" ] || ! kill -0 "$(tr -d '[:space:]' <"${PF}" 2>/dev/null)" 2>/dev/null; then
            FINDINGS+=("stale-socket:${L}")
            [ "${FIX}" = "--fix" ] && { rm -f "${L}"; FINDINGS+=("fixed:removed-socket"); }
        fi
    done
    # corrupt last event line (crash mid-append)
    local EV="${VMDIR}/${VMNAME}.events.jsonl"
    if [ -f "${EV}" ] && ! tail -n1 "${EV}" | grep -q '}'; then
        FINDINGS+=("event-truncated:${EV}")
        [ "${FIX}" = "--fix" ] && { sed -i '$d' "${EV}"; FINDINGS+=("fixed:dropped-truncated-event"); }
    fi
    # orphaned operation files (queued/running with dead worker)
    local OP
    for OP in "${VMDIR}/ops"/*.json; do
        [ -f "${OP}" ] || continue
        grep -q '"state":"\(queued\|running\)"' "${OP}" || continue
        local WP
        WP=$(sed -n 's/.*"workerPid":\([0-9]*\).*/\1/p' "${OP}")
        if [ -z "${WP}" ] || ! kill -0 "${WP}" 2>/dev/null; then
            FINDINGS+=("orphan-operation:${OP}")
            [ "${FIX}" = "--fix" ] && { sed -i 's/"state":"[^"]*"/"state":"interrupted"/' "${OP}"; FINDINGS+=("fixed:marked-interrupted"); }
        fi
    done
    N=${#FINDINGS[@]}
    if [ "${OUTPUT_FORMAT:-}" == "json" ]; then
        local J="" I
        for I in "${FINDINGS[@]:-}"; do [ -n "${I}" ] && J+="${J:+,}\"$(json_escape "${I}")\""; done
        qe_ok "vm.doctor" "{\"vm\":\"$(json_escape "${VMNAME:-}")\",\"fix\":$([ "${FIX}" = "--fix" ] && echo true || echo false),\"findings\":[${J}]}"
    else
        [ "${N}" -eq 0 ] && echo " - ${VMNAME}: healthy."
        for I in "${FINDINGS[@]:-}"; do [ -n "${I}" ] && echo " - ${I}"; done
    fi
}
