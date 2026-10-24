# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154,SC2317
# Sourced partial: shares the global namespace of bin/quickgetc.
# CLI boundary: operation dispatch.
# # Runs at top level at the end of bin/quickgetc (sourced last).


function open_homepage() {
    find_os_entry "${1}"
    local URL
    URL=$(get_homepage)
    if [ -n "${URL}" ]; then
        xdg-open "${URL}" || sensible-browser "${URL}" || x-www-browser "${URL}" || gnome-open "${URL}"
    else
        echo "ERROR! No homepage found for ${PRETTY_NAME}"
    fi
}

trap cleanup EXIT

if ((BASH_VERSINFO[0] < 4)); then
    echo "Sorry, you need bash 4.0 or newer to run this script."
    exit 1
fi

OPERATION=""
CURL=$(command -v curl)
if [ ! -x "${CURL}" ]; then
    echo "ERROR! curl not found. Please install curl"
    exit 1
fi
CURL_VERSION=$("${CURL}" --version | head -n +1 | cut -d' ' -f2)

QEMU_IMG=$(command -v qemu-img)
if [ ! -x "${QEMU_IMG}" ]; then
    echo "ERROR! qemu-img not found. Please make sure qemu-img is installed."
    exit 1
fi

populate_cache

case "${1}" in
    --arch|-arch|-a)
        shift
        if [ -z "${1}" ]; then
            echo "ERROR! You must specify an architecture."
            echo " - Supported architectures: $(supported_archs)"
            exit 1
        fi
        set_arch "${1}"
        shift
        ;;
    --download|-download)
        OPERATION="download"
        shift
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
                find_os_entry "${OS}"
                show_os_info
            done
        else
            find_os_entry "${1,,}"
            show_os_info
        fi
        exit 0;;
    --version|-version|-V)
        echo "${QUICKGETC_VERSION}"
        exit 0;;
    --help|-help|--h|-h)
        help_message
        exit 0;;
    --url|-url)
        OPERATION="show"
        shift
        if [ -z "${1}" ]; then
            for OS in $(os_support); do
                (test_all "${OS}")
            done
            exit 0
        elif [ -z "${2}" ]; then
            test_all "${1}"
            exit 0
        fi;;
    --list-csv|-list-csv|list|list_csv) list_csv;;
    --list-json|-list-json|list_json) list_json;;
    --list|-list) list_supported;;
    -*) error_not_supported_argument;;
esac

if [ -n "${1}" ]; then
    find_os_entry "${1,,}"
else
    error_specify_os
fi

if [ -n "${2}" ]; then
    find_release_entries "${2}"
    VM_PATH="${OS}-${RELEASE}"

    if release_requires_edition; then
        if [ -n "${3}" ]; then
            find_edition_entry "${3}"
            VM_PATH+="-${EDITION}"
        else
            error_specify_edition
        fi
    elif [ -n "${3}" ]; then
        error_no_edition_required
    else
        find_edition_entry
    fi

    if [ -n "${ARCH}" ]; then
        VM_PATH+="-${ARCH}"
    fi

    create_vm
else
    error_specify_release
fi

# vim:tabstop=4:shiftwidth=4:expandtab
