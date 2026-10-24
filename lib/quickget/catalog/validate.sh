# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154,SC2317
# Sourced partial: shares the global namespace of bin/quickget.
# Catalog: validation of OS/release/edition/language selections.


function os_supported() {
    if [[ ! " $(os_support) " =~ \ "${OS}"\  ]]; then
        echo -e "ERROR! ${OS} is not a supported OS.\n"
        os_support | fmt -w 80
        exit 1
    fi
}


function is_valid_language() {
    local I18N=""
    local PASSED_I18N="${1}"
    for I18N in "${I18NS[@]}"; do
        if [[ "${I18N}" == "${PASSED_I18N}" ]]; then
            return 0
        fi
    done
    return 1
}


function handle_missing() {
    # Handle odd missing Fedora combinations
    case "${OS}" in
        fedora)
            # First we need to handle the Beta naming kludge
            if [[ "${RELEASE}" == *"_Beta" ]]; then
                NRELEASE="${RELEASE/_Beta/}"
                else
                NRELEASE="${RELEASE}"
            fi
            if [[ "${NRELEASE}" -lt 40 && "${EDITION}" == "Onyx" ]] || [[ "${NRELEASE}" -lt 40 && "${EDITION}" == "Sericea" ]]; then
                echo "ERROR! Unsupported combination"
                echo "       Fedora ${RELEASE} ${EDITION} is not available, please choose another Release or Edition"
                exit 1
            fi;;
    esac
}


function validate_release() {
    local DISPLAY_NAME=""
    local RELEASE_GENERATOR=""
    local RELEASES=""

    DISPLAY_NAME="$(pretty_name "${OS}")"
    case ${OS} in
        *ubuntu-server*) RELEASE_GENERATOR="releases_ubuntu-server";;
        *ubuntu*) RELEASE_GENERATOR="releases_ubuntu";;
        *) RELEASE_GENERATOR="${1}";;
    esac
    RELEASES=$(${RELEASE_GENERATOR})
    error_not_supported_release
}
