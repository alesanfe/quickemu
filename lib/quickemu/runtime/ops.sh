# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Async operation registry: long-running operations (export, import,
# clone, compact, backup) get an operationId and a versioned JSON state
# file under <vmdir>/ops/ so QuickGUI can poll progress, survive client
# restarts and cancel safely.

QE_OPS_DIR_SUFFIX="/ops"

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
    local f; f="$(op_dir)/${1}.json"
    [ -f "${f}" ] || { echo "ERROR! Unknown operation '${1}'." >&2; return 1; }
    cat "${f}"
}
