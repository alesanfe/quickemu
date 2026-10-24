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
