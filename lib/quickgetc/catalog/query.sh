# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154,SC2317
# Sourced partial: shares the global namespace of bin/quickgetc.
# Catalog: jq queries against the cached OS dataset.


function find_edition_entry() {
    EDITION="${1}"
    local EDITION_ENTRIES

    if [ -n "${EDITION}" ]; then
        EDITION_ENTRIES=$(jq -c 'select(.edition=="'"${EDITION}"'")' <<< "${RELEASE_ENTRIES}")
    else
        EDITION_ENTRIES=$(jq -c '. | select(.edition==null)' <<< "${RELEASE_ENTRIES}")
    fi
    if [ -z "${EDITION_ENTRIES}" ]; then
        error_not_supported_edition
    fi

    local PREFERRED_ARCH="${ARCH:-$(uname -m)}"
    ENTRY=$(jq -c '. | select(.arch=="'"${PREFERRED_ARCH}"'")' <<< "${EDITION_ENTRIES}")

    if [ -z "${ENTRY}" ]; then
        if [ -n "${ARCH}" ]; then
            unsupported_arch
        else
            ENTRY=$(jq -c -s '.[0]' <<< "${EDITION_ENTRIES}")
        fi
    fi
}


function find_release_entries() {
    RELEASE="${1}"

    RELEASE_ENTRIES="$(jq -c '.releases[] | select(.release=="'"${RELEASE}"'")' <<< "${OS_ENTRY}")"

    if [ -z "${RELEASE_ENTRIES}" ]; then
        echo -e "ERROR! ${PRETTY_NAME} ${RELEASE} is not a supported release.\n"
        list_releases | fold -s -w "$(tput cols)"
        exit 1
    fi
}


function find_os_entry() {
    OS="${1}"

    OS_ENTRY="$(jq -c '.[] | select(.name=="'"${OS}"'") | .releases[] |= (. + {arch: (.arch // "x86_64"), release: (.release // "latest")})' <<< "${JSON}")"

    if [ -z "${OS_ENTRY}" ]; then
        echo -e "ERROR! ${OS} is not a supported OS.\n"
        os_support | fold -s -w "$(tput cols)"
        exit 1
    fi
    PRETTY_NAME=$(jq -r '.pretty_name' <<< "${OS_ENTRY}")
}
