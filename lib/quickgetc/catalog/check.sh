# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154,SC2317
# Sourced partial: shares the global namespace of bin/quickgetc.
# Catalog: URL testing output.


function test_result() {
    local OS="${1}"
    local RELEASE="${2}"
    local EDITION="${3:-}"
    local URL="${4:-}"
    local RESULT="${5:-}"
    if [ -n "${EDITION}" ]; then
        OS="${OS}-${RELEASE}-${EDITION}"
    else
        OS="${OS}-${RELEASE}"
    fi

    if [ -n "${RESULT}" ]; then
        # Pad the OS string for consistent output
        OS=$(printf "%-35s" "${OS}")
        echo -e "${RESULT}: ${OS} ${URL}"
    else
        OS=$(printf "%-36s" "${OS}:")
        echo -e "${OS} ${URL}"
    fi
}


# Iterate every release entry of an OS and report each source URL,
# either printing it (show) or probing it (test).
function test_all() {
    local OS_NAME="${1}"
    local ENTRY REL EDN URLS URL CHECK
    find_os_entry "${OS_NAME}"

    while IFS= read -r ENTRY; do
        REL=$(jq -r '.release' <<< "${ENTRY}")
        EDN=$(jq -r '.edition // ""' <<< "${ENTRY}")
        URLS=$(jq -r '[.. | objects | .url? // empty] | unique | .[]' <<< "${ENTRY}")
        while IFS= read -r URL; do
            [ -z "${URL}" ] && continue
            if [ "${OPERATION}" == "show" ]; then
                test_result "${OS_NAME}" "${REL}" "${EDN}" "${URL}"
            elif [ "${OPERATION}" == "test" ]; then
                CHECK=$(web_check "${URL}" && echo "PASS" || echo "FAIL")
                test_result "${OS_NAME}" "${REL}" "${EDN}" "${URL}" "${CHECK}"
            fi
        done <<< "${URLS}"
    done < <(jq -c '.releases[]' <<< "${OS_ENTRY}")
}
