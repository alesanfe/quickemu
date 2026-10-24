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

# Early architecture validation for OSes without edition-dependent arch
# support (editions_* OSes are validated after EDITION is resolved).
function catalog_check_os_arch() {
    local OS="${1}"
    if [[ $(type -t "editions_${OS}") != function ]] && ! is_arch_supported "${OS}" "${ARCH}"; then
        if [ "${OPERATION}" == "test" ] || [ "${OPERATION}" == "show" ]; then
            test_result "${OS}" "${2:-}" "${3:-}" "" "SKIP" "(not available for ${ARCH})"
            exit 0
        else
            echo "ERROR! $(pretty_name "${OS}") is not available for ${ARCH} architecture."
            exit 1
        fi
    fi
}

# Resolve and validate the requested edition for an editions_* OS.
# Sets the EDITION and VM_PATH globals used by the caller.
function catalog_resolve_edition() {
    local OS="${1}"
    validate_release "releases_${OS}"
    EDITIONS=("$(editions_"${OS}")")
    if [ -n "${2}" ]; then
        EDITION="${2}"
        if [[ ! " ${EDITIONS[*]} " =~ \ "${EDITION}"\  ]]; then
            echo -e "ERROR! ${EDITION} is not a supported $(pretty_name "${OS}") edition\n"
            echo -n ' - Supported editions: '
            for EDITION in "${EDITIONS[@]}"; do
                echo -n "${EDITION} "
            done
            echo ""
            exit 1
        fi
    else
        show_os_info "${OS}"
        echo -e " - Editions:\t$("editions_${OS}" | fmt -w 80)"
        echo -e "\nERROR! You must specify an edition."
        exit 1
    fi
    # Now that EDITION is known, validate architecture support
    if ! is_arch_supported "${OS}" "${ARCH}"; then
        if [ "${OPERATION}" == "test" ] || [ "${OPERATION}" == "show" ]; then
            test_result "${OS}" "${RELEASE}" "${EDITION}" "" "SKIP" "(not available for ${ARCH})"
            exit 0
        else
            echo "ERROR! $(pretty_name "${OS}") ${EDITION} is not available for ${ARCH} architecture."
            exit 1
        fi
    fi
    handle_missing
    VM_PATH="${OS}-${RELEASE}-${EDITION}$(arch_suffix)"
}
