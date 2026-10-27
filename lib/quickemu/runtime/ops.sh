# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Async operation registry: long-running operations (export, import,
# clone, compact, backup) get an operationId and a versioned JSON state
# file under <vmdir>/ops/ so QuickGUI can poll progress, survive client
# restarts and cancel safely.

QE_OPS_DIR_SUFFIX="/ops"

# ADR-0001 phase 3: prefer the Go ops registry when the binary exists.
_ops_bin() {
    command -v quickemu-ops >/dev/null 2>&1 && { printf quickemu-ops; return; }
    [ -x "${QE_LIBDIR}/../tools/quickemu-ops/quickemu-ops" ]         && { printf '%s' "${QE_LIBDIR}/../tools/quickemu-ops/quickemu-ops"; return; }
    printf ''
}

op_dir() { printf '%s%s' "${VMDIR}" "${QE_OPS_DIR_SUFFIX}"; }

op_new() { # type -> prints operationId, creates state file
    # Lazily reap interrupted operations before creating a new one —
    # VMDIR is resolved by now so op_dir points at the real location.
    [ -z "${_OP_RECOVERED:-}" ] && { _OP_RECOVERED=1; op_recover; }
    local id dir
    id=$(vm_instance_uuid)
    dir=$(op_dir); mkdir -p "${dir}"
    printf '{"schemaVersion":%s,"operationId":"%s","type":"%s","vm":"%s","state":"queued","progress":0,"startedAt":"%s","result":null,"error":null}\n' \
        "${QE_SCHEMA_VERSION:-1}" "${id}" "${1}" "$(json_escape "${VMNAME}")" \
        "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >"${dir}/${id}.json"
    printf '%s' "${id}"
}

op_set() { # id state progress -> update state file
    local f; f="$(op_dir)/${1}.json"
    [ -f "${f}" ] || return 1
    sed -i "s/\"state\":\"[^\"]*\"/\"state\":\"${2}\"/; s/\"progress\":[0-9]*/\"progress\":${3:-0}/" "${f}"
}

op_done() { # id -> mark completed
    op_set "${1}" completed 100
    sed -i "s/\"result\":null/\"result\":{\"finishedAt\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"}/" "$(op_dir)/${1}.json"
}

op_fail() { # id code message -> mark failed
    local f; f="$(op_dir)/${1}.json"
    [ -f "${f}" ] || return 1
    sed -i "s/\"state\":\"[^\"]*\"/\"state\":\"failed\"/; s/\"error\":null/\"error\":{\"code\":\"$(json_escape "${2}")\",\"message\":\"$(json_escape "${3}")\"}/" "${f}"
}

op_status() { # id -> prints state file
    local B; B=$(_ops_bin)
    [ -n "${B}" ] && { "${B}" status --vmdir "${VMDIR}" --id "${1}"; return; }
    local f; f="$(op_dir)/${1}.json"
    [ -f "${f}" ] || {
        [ "${OUTPUT_FORMAT:-}" == "json" ] && qe_fail "vm.op-status" OPERATION_NOT_FOUND "unknown operation '${1}'" ||             echo "ERROR! Unknown operation '${1}'." >&2
        return 1
    }
    cat "${f}"
}

op_list() { # -> JSON array of all operation states for this VM
    local B; B=$(_ops_bin)
    [ -n "${B}" ] && { "${B}" list --vmdir "${VMDIR}"; return; }
    local DIR F JSON="" FIRST=1
    DIR=$(op_dir)
    for F in "${DIR}"/*.json; do
        [ -f "${F}" ] || continue
        [ ${FIRST} -eq 0 ] && JSON+=","
        FIRST=0
        JSON+="$(cat "${F}")"
    done
    printf '{"operations":[%s]}' "${JSON}"
}

# Cancel: marks the op cancelled. If the op recorded a workerPid that is
# still alive it is signalled first — a stale record never kills anyone.
op_cancel() { # id -> 0/1
    local B; B=$(_ops_bin)
    [ -n "${B}" ] && { "${B}" cancel --vmdir "${VMDIR}" --id "${1}"; return; }
    local F PID
    F="$(op_dir)/${1}.json"
    [ -f "${f:-$F}" ] || {
        [ "${OUTPUT_FORMAT:-}" == "json" ] && qe_fail "vm.op-cancel" OPERATION_NOT_FOUND "unknown operation '${1}'" ||             echo "ERROR! Unknown operation '${1}'." >&2
        return 1
    }
    case "$(sed -n 's/.*"state":"\([^"]*\)".*/\1/p' "${F}")" in
    completed | failed | cancelled)
        echo "ERROR! Operation ${1} already finished." >&2; return 1 ;;
    esac
    PID=$(sed -n 's/.*"workerPid":\([0-9]*\).*/\1/p' "${F}")
    if [ -n "${PID}" ] && kill -0 "${PID}" 2>/dev/null; then
        kill "${PID}" 2>/dev/null || true
    fi
    op_set "${1}" cancelled 0 2>/dev/null || sed -i 's/"state":"[^"]*"/"state":"cancelled"/' "${F}"
    echo " - Operation ${1} cancelled."
}

# Retention: keep only the newest N op files per VM.
op_prune() { # keep=N
    local KEEP="${1:-10}" DIR
    DIR=$(op_dir)
    [ -d "${DIR}" ] || return 0
    # shellcheck disable=SC2012
    ls -1t "${DIR}"/*.json 2>/dev/null | tail -n +"$((KEEP + 1))" | while IFS= read -r F; do rm -f "${F}"; done
}

# op_recover: scan .ops/ for ops stuck in queued/running whose owner is dead.
# Marks them "failed" with OPERATION_INTERRUPTED. Called once at startup.
op_recover() {
    local d f id state owner
    d=$(op_dir)
    [ -d "${d}" ] || return 0
    for f in "${d}"/*.json; do
        [ -f "${f}" ] || continue
        state=$(grep -o '"state":"[^"]*"' "${f}" | cut -d'"' -f4)
        case "${state}" in queued|running) ;; *) continue;; esac
        id=$(grep -o '"operationId":"[^"]*"' "${f}" | cut -d'"' -f4)
        owner=""
        [ -f "${d}/${id}.owner" ] && owner=$(cat "${d}/${id}.owner")
        # If the recorded owner PID is dead (or missing), the op is orphaned.
        if [ -z "${owner}" ] || ! kill -0 "${owner}" 2>/dev/null; then
            op_fail "${id}" "OPERATION_INTERRUPTED" "operation orphaned (owner ${owner:-unknown} dead)"
            qe_verbose "op_recover: marked ${id} interrupted (owner=${owner:-none})"
        fi
    done
}

# op_rollback: per-type compensation for a failed/cancelled operation.
# Reads the op's state file and undoes its effects where safe to do so.
# Returns 0 when compensation completed (or was a no-op), 1 on irrecoverable.
op_rollback() {
    local f type vm d
    f="$(op_dir)/${1}.json"
    [ -f "${f}" ] || return 1
    type=$(grep -o '"type":"[^"]*"' "${f}" | cut -d'"' -f4)
    vm=$(grep -o '"vm":"[^"]*"' "${f}" | cut -d'"' -f4)
    d="${VMDIR}"
    case "${type}" in
        download)
            # Remove partial .part + any temp download artifacts
            rm -f "${d}/"*.part "${d}/"*.zsync* "${d}/"*.tmp 2>/dev/null || true
            ;;
        import|export)
            # Remove the temp staging dir created by import/export
            rm -rf "${d}/.import-"* "${d}/.export-"* 2>/dev/null || true
            ;;
        clone|rename)
            # Partial clone/rename — remove the destination if it's incomplete
            rm -rf "${d}"/.partial-* 2>/dev/null || true
            ;;
        delete)
            # Move .trash back into place if present and destination missing
            [ -d "${d}/.trash" ] && [ ! -e "${d}/vm" ] && mv "${d}/.trash" "${d}/vm" 2>/dev/null || true
            ;;
        compact|snapshot)
            # Discard temp disk artifacts
            rm -f "${d}/"*.tmp.qcow2 "${d}/"*.compact-* 2>/dev/null || true
            ;;
        *) :;;  # unknown op — nothing to undo
    esac
    op_fail "${1}" "OPERATION_ROLLBACK" "rolled back ${type} effects"
}
