# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# Safe delete plan: inspect what --delete-vm would remove BEFORE
# executing it. Never deletes anything.

function delete_plan() {
    local ITEMS=() F=""
    [ -f "${VM}" ] && ITEMS+=("${VM}")
    if [ -d "${VMDIR}" ]; then
        while IFS= read -r F; do ITEMS+=("${F}"); done < <(find "${VMDIR}" -mindepth 1)
    fi
    if [ "${OUTPUT_FORMAT}" == "json" ]; then
        local PLAN="[" FIRST=1 I=""
        for I in "${ITEMS[@]}"; do
            [ ${FIRST} -eq 0 ] && PLAN+=","
            FIRST=0
            PLAN+="\"$(json_escape "${I}")\""
        done
        qe_ok "vm.delete-plan" "$(printf '{"vm":"%s","delete":%s}' \
            "$(json_escape "${VMNAME}")" "${PLAN}]")"
    else
        echo "Delete plan for ${VMNAME}:"
        if [ ${#ITEMS[@]} -eq 0 ]; then
            echo " - nothing to delete"
        else
            for I in "${ITEMS[@]}"; do echo " - ${I}"; done
        fi
    fi
}
