# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# Safe delete plan: inspect what --delete-vm would remove BEFORE
# executing it. Never deletes anything.

# Collect the resources --delete-vm would remove (sorted, canonical).
delete_plan_items() { # -> ITEMS[]
    ITEMS=()
    local F=""
    [ -f "${VM}" ] && ITEMS+=("${VM}")
    if [ -d "${VMDIR}" ]; then
        # Runtime artifacts (locks, instance identity, pid/socket/port
        # state) are cleaned by vm_runtime_cleanup, not user data, and
        # must not appear in the plan or influence planId.
        while IFS= read -r F; do ITEMS+=("${F}"); done < <(
            find "${VMDIR}" -mindepth 1 \
                ! -name '*.lock' ! -name '*.lock.shared' \
                ! -name '*.runtime.json' ! -name '*.pid' \
                ! -name '*.sock' ! -name '*.socket' ! -name '*.ports' \
                ! -name '*.paused' ! -name '*.spice' ! -name '*.monitor' \
                | LC_ALL=C sort
        )
    fi
}

# planId = sha256 over a canonical serialization of the plan. Any change to
# the resource set or contents (size/mtime) after --delete-plan invalidates
# the id, so --delete-vm --plan-id can refuse a stale plan.
delete_plan_id() { # ITEMS[] -> prints sha256:<hash>
    local I="" line=""
    {
        printf 'vm=%s\n' "${VMNAME}"
        for I in "${ITEMS[@]}"; do
            line="${I}"
            [ -e "${I}" ] || [ -L "${I}" ] && line+="|$(stat -c '%s|%y|%F' "${I}" 2>/dev/null || stat -f '%z|%m|%HT' "${I}" 2>/dev/null)"
            printf '%s\n' "${line}"
        done
    } | sha256sum 2>/dev/null | awk '{print "sha256:"$1}'
}

function delete_plan() {
    # Shared (reader) lock: the plan must not enumerate resources while a
    # writer is mid-mutation, and its own planId check then matches what
    # --delete-vm --plan-id will see.
    vm_lock_acquire shared || return 1
    local ITEMS=()
    delete_plan_items
    local PLAN_ID
    PLAN_ID=$(delete_plan_id)
    if [ "${OUTPUT_FORMAT}" == "json" ]; then
        local PLAN="[" FIRST=1 I=""
        for I in "${ITEMS[@]}"; do
            [ ${FIRST} -eq 0 ] && PLAN+=","
            FIRST=0
            PLAN+="\"$(json_escape "${I}")\""
        done
        qe_ok "vm.delete-plan" "$(printf '{"vm":"%s","planId":"%s","delete":%s}' \
            "$(json_escape "${VMNAME}")" "${PLAN_ID}" "${PLAN}]")"
    else
        echo "Delete plan for ${VMNAME}:"
        if [ ${#ITEMS[@]} -eq 0 ]; then
            echo " - nothing to delete"
        else
            for I in "${ITEMS[@]}"; do echo " - ${I}"; done
        fi
        echo "Plan id: ${PLAN_ID} (required by --delete-vm --plan-id)"
    fi
    vm_lock_release shared
}
