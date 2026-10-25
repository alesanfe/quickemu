# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154,SC2317
# Sourced partial: shares the global namespace of bin/quickget.
# Support: error reporting helpers.


function error_specify_os() {
    echo "ERROR! You must specify an operating system."
    echo "- Supported Operating Systems:"
    os_support | fmt -w 80
    echo -e "\nTo see all possible arguments, use:\n   quickget -h  or  quickget --help"
    exit 1
}


function error_specify_release() {
    show_os_info "${OS}"
    case ${OS} in
        *ubuntu-server*)
            echo -en " - Releases:\t"
            releases_ubuntu-server
            echo -en " - Archs:\t"
            get_supported_archs "${OS}"
            ;;
        *ubuntu*)
            echo -en " - Releases:\t"
            releases_ubuntu
            echo -en " - Archs:\t"
            get_supported_archs "${OS}"
            ;;
        *windows*)
            echo -en " - Releases:\t"
            "releases_${OS}"
            echo -en " - Languages:\t"
            "languages_${OS}"
            echo "${I18NS[@]}"
            # Windows uses multi-arch ISOs, skip architecture display
            ;;
        *)
            echo -en " - Releases:\t"
            "releases_${OS}" | fmt -w 80
            if [[ $(type -t "editions_${OS}") == function ]]; then
                echo -en " - Editions:\t"
                "editions_${OS}" | fmt -w 80
            fi
            echo -en " - Archs:\t"
            get_supported_archs "${OS}"
            ;;
    esac
    echo -e "\nERROR! You must specify a release."
    exit 1
}


function error_not_supported_release() {
    local GENERATOR="${1:-releases_${OS}}"
    if [[ ! " ${RELEASES[*]} " =~ \ "${RELEASE}"\  ]]; then
        echo -e "ERROR! ${DISPLAY_NAME} ${RELEASE} is not a supported release.\n"
        echo -n ' - Supported releases: '
        "${GENERATOR}"
        exit 1
    fi
}


function error_not_supported_lang() {
    echo -e "ERROR! ${I18N} is not a supported $(pretty_name "${OS}") language\n"
    echo -n ' - Editions: '
    for I18N in "${I18NS[@]}"; do
        echo -n "${I18N} "
    done
    exit 1
}


function error_not_supported_argument() {
    echo "ERROR! Not supported argument"
    echo "To see all possible arguments, use:"
    echo "   quickget -h  or  quickget --help"
    exit 1
}
