# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154,SC2317
# Sourced partial: shares the global namespace of bin/quickgetc.
# Catalog: selection validation and errors.


function unsupported_arch() {
    echo "ERROR! ${ARCH} is not a supported architecture for ${PRETTY_NAME} ${RELEASE}${EDITION:+ $EDITION}."
    echo " - Supported architectures: $(jq -s -r '[.[] | .arch] | join(" ")' <<< "${EDITION_ENTRIES}")"
    exit 1
}


function error_specify_edition() {
    show_os_info
    echo -e " - Editions:\t$(list_editions "${RELEASE}" | fold -s -w "$(tput cols)")"
    echo -e "\nERROR! You must specify an edition."
    exit 1
}


function error_no_edition_required() {
    echo "ERROR! ${PRETTY_NAME} ${RELEASE} does not require an edition."
    exit 1
}


function error_specify_release() {
    show_os_info
    list_releases
}


function release_requires_edition() {
    [ "$(jq -s 'any(.[]; .edition == null)' <<< "${RELEASE_ENTRIES}")" == "false" ]
}


function error_specify_os() {
    echo "ERROR! You must specify an operating system."
    echo "- Supported Operating Systems:"
    os_support | fold -s -w "$(tput cols)"
    echo -e "\nTo see all possible arguments, use:\n   quickgetc -h  or  quickgetc --help"
    exit 1
}
