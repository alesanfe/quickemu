# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154,SC2317
# Sourced partial: shares the global namespace of bin/quickgetc.
# Catalog: OS metadata queries and display.


function get_homepage() {
    jq -r '.homepage' <<< "${OS_ENTRY}"
}


function get_description() {
    jq -r '.description' <<< "${OS_ENTRY}"
}


function show_os_info() {
    local HOMEPAGE
    local DESCRIPTION
    
    HOMEPAGE=$(get_homepage)
    DESCRIPTION=$(get_description)

    echo -e "\n${PRETTY_NAME}"
    if [ -n "${HOMEPAGE}" ]; then
        echo " - Website: ${HOMEPAGE}"
    fi
    if [ -n "${DESCRIPTION}" ]; then
        echo -e " - Description: ${DESCRIPTION}"
    fi
}


function os_support() {
    jq -r '[.[] | .name] | join(" ")' <<< "${JSON}"
}
