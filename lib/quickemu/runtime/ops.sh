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
    [ -f "${f}" ] || { echo "ERROR! Unknown operation '${1}'." >&2; return 1; }
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
    [ -f "${f:-$F}" ] || { echo "ERROR! Unknown operation '${1}'." >&2; return 1; }
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
