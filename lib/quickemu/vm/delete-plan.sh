# shellcheck shell=bash
# Safe delete plan: inspect what --delete-vm would remove BEFORE
# executing it. Never deletes anything.

function delete_plan() {
    local ITEMS=() F=""
    [ -f "${VM}" ] && ITEMS+=("${VM}")
    if [ -d "${VMDIR}" ]; then
        while IFS= read -r F; do ITEMS+=("${F}"); done < <(find "${VMDIR}" -mindepth 1)
    fi
    if [ "${OUTPUT_FORMAT}" == "json" ]; then
        printf '{"schemaVersion":1,"vm":"%s","delete":[' "${VMNAME}"
        local FIRST=1 I=""
        for I in "${ITEMS[@]}"; do
            [ ${FIRST} -eq 0 ] && printf ','
            FIRST=0
            printf '"%s"' "$(json_escape "${I}")"
        done
        printf ']}\n'
    else
        echo "Delete plan for ${VMNAME}:"
        if [ ${#ITEMS[@]} -eq 0 ]; then
            echo " - nothing to delete"
        else
            for I in "${ITEMS[@]}"; do echo " - ${I}"; done
        fi
    fi
}
