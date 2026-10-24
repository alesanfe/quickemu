# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154,SC2317
# Sourced partial: shares the global namespace of bin/quickget.
# CLI boundary: argument parsing, flag handling and operation dispatch.
# Runs at top level at the end of bin/quickget (sourced last).


function open_homepage() {
    local URL=""
    local XDG_OPEN=""
    if [ -z "$(os_info "${1}")" ]; then
        error_specify_os
    else
        URL="$(os_info "${1}" | cut -d'|' -f 3)"
        # shellcheck disable=SC2034
        XDG_OPEN=$(xdg-open "${URL}" || sensible-browser "${URL}" || x-www-browser "${URL}" || gnome-open "${URL}")
        exit 0
    fi
}

# Function to process --arch flag
function parse_arch_flag() {
    if [ "${1}" == "--arch" ] || [ "${1}" == "-arch" ]; then
        case "${2}" in
            arm64|aarch64) ARCH="arm64";;
            amd64|x86_64) ARCH="amd64";;
            *) echo "ERROR! Unsupported architecture: ${2}"; exit 1;;
        esac
        return 0  # Flag was found
    fi
    return 1  # Flag was not found
}

trap cleanup EXIT

if ((BASH_VERSINFO[0] < 4)); then
    echo "Sorry, you need bash 4.0 or newer to run this script."
    exit 1
fi

QUICKEMU=$(resolve_quickemu)
I18NS=()
OPERATION=""
CHECK_ALL_ARCH="false"
CURL_INSECURE="false"
[[ "${QUICKGET_INSECURE:-}" == "1" || "${QUICKGET_INSECURE:-}" == "true" ]] && CURL_INSECURE="true"
# Normalised host architecture for foreign arch detection
NORMALISED_HOST_ARCH="${ARCH}"
CURL=$(command -v curl)
if [ ! -x "${CURL}" ]; then
    echo "ERROR! curl not found. Please install curl"
    exit 1
fi
CURL_VERSION=$("${CURL}" --version | head -n 1 | cut -d' ' -f2)

#TODO: Deprecate `list`, `list_csv`, and `list_json` in favor of `--list`, `--list-csv`, and `--list-json`

# Process --arch flag first if present (can be combined with other flags)
if parse_arch_flag "${1}" "${2}"; then
    shift 2
fi

case "${1}" in
    --download|-download)
        OPERATION="download"
        shift
        # Handle --arch after --download
        if parse_arch_flag "${1}" "${2}"; then
            shift 2
        fi
        ;;
    --create-config|-create-config)
        OPERATION="config"
        shift
        create_config "${@}"
        ;;
    --open-homepage|-open-homepage)
        shift
        open_homepage "${1}"
        ;;
    --show|-show)
        shift
        if [ -z "${1}" ]; then
            for OS in $(os_support); do
                show_os_info "${OS}"
            done
        else
            show_os_info "${1}"
        fi
        exit 0;;
    --version|-version)
        WHERE="${QG_BINDIR}"
        "${WHERE}/quickemu" --version
        exit 0;;
    --help|-help|--h|-h)
        help_message
        exit 0;;
    --url|-url)
        OPERATION="show"
        shift
        # Handle --arch after --url
        if parse_arch_flag "${1}" "${2}"; then
            shift 2
        fi
        if [ -z "${1}" ]; then
            for OS in $(os_support); do
                (test_all "${OS}")
            done
            exit 0
        elif [ -z "${2}" ]; then
            test_all "${1}"
            exit 0
        fi;;
    --check|-check)
        OPERATION="test"
        shift
        # Handle --arch after --check
        if parse_arch_flag "${1}" "${2}"; then
            shift 2
        fi
        if [ -z "${1}" ]; then
            for OS in $(os_support); do
                (test_all "${OS}")
            done
            exit 0
        elif [ -z "${2}" ]; then
            test_all "${1}"
            exit 0
        fi;;
    --check-all-arch|-check-all-arch)
        OPERATION="test"
        CHECK_ALL_ARCH="true"
        shift
        if [ -z "${1}" ]; then
            for CHECK_ARCH in amd64 arm64; do
                ARCH="${CHECK_ARCH}"
                for OS in $(os_support); do
                    (test_all "${OS}")
                done
            done
            exit 0
        elif [ -z "${2}" ]; then
            for CHECK_ARCH in amd64 arm64; do
                ARCH="${CHECK_ARCH}"
                test_all "${1}"
            done
            exit 0
        fi;;
    --list-csv|-list-csv|list|list_csv) list_csv;;
    --list-json|-list-json|list_json) list_json;;
    --list|-list) list_supported;;
    --insecure|-insecure)
        CURL_INSECURE="true"
        shift
        ;;
    --cloud-init|-cloud-init)
        CLOUD_INIT="true"
        shift
        ;;
    --unattended|-unattended)
        UNATTENDED="true"
        shift
        ;;
    --update|-update)
        OPERATION="update"
        shift
        ;;
    -*) error_not_supported_argument;;
esac

if [ -n "${1}" ]; then
    OS="${1,,}"
else
    error_specify_os
fi

os_supported

if [ "${OPERATION}" == "update" ]; then
    catalog_update_check "${OS}"
fi

# Handle --check-all-arch for specific release/edition
if [ "${CHECK_ALL_ARCH}" == "true" ] && [ -n "${2}" ]; then
    catalog_check_all_arch "${OS}" "${2}" "${3:-}"
fi

# Validate architecture support before attempting download
catalog_check_os_arch "${OS}" "${2:-}" "${3:-}"

if [ -n "${2}" ]; then
    RELEASE="${2}"
    VM_PATH="${OS}-${RELEASE}$(arch_suffix)"
    # If the OS has an editions_() function, use it.
    if [[ $(type -t "editions_${OS}") == function ]]; then
        catalog_resolve_edition "${OS}" "${3:-}"
        create_vm "$("get_${OS}" "${EDITION}")"
    elif [ "${OS}" == "macos" ]; then
        # macOS doesn't use create_vm()
        validate_release releases_macos
        get_macos
    elif [[ "${OS}" == *"ubuntu-server"* ]]; then
        # (Comes before regular Ubuntu, or the code tries to download the desktop) #
        # Ubuntu doesn't use create_vm()
        validate_release releases_ubuntu-server
        get_ubuntu-server
    elif [[ "${OS}" == *"ubuntu"* ]]; then
        # Ubuntu doesn't use create_vm()
        validate_release releases_ubuntu
        get_ubuntu
    elif [[ "${OS}" == "windows"* ]]; then
        I18N="English International"
        "languages_${OS}"
        if [ -n "${3}" ]; then
            I18N="${3}"
            if ! is_valid_language "${I18N}"; then
                error_not_supported_lang
            fi
            VM_PATH="$(echo "${OS}-${RELEASE}-${I18N// /-}" | tr -d '()')$(arch_suffix)"
        fi
        validate_release "releases_${OS}"
        get_windows
    else
        validate_release "releases_${OS}"
        create_vm "$("get_${OS}")"
    fi
else
    error_specify_release
fi

# vim:tabstop=4:shiftwidth=4:expandtab
