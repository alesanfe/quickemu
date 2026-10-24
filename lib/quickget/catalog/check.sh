# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154,SC2317
# Sourced partial: shares the global namespace of bin/quickget.
# Catalog: --url/--check URL testing operations.


function test_result() {
    local OS="${1}"
    local RELEASE="${2}"
    local EDITION="${3:-}"
    local URL="${4:-}"
    local RESULT="${5:-}"
    local REASON="${6:-}"
    if [ -n "${EDITION}" ]; then
        OS="${OS}-${RELEASE}-${EDITION}"
    else
        OS="${OS}-${RELEASE}"
    fi

    if [ -n "${RESULT}" ]; then
        # Pad the OS string for consistent output
        OS=$(printf "%-35s" "${OS}")
        if [ -n "${REASON}" ]; then
            echo -e "${RESULT}: ${OS} ${REASON}"
        else
            echo -e "${RESULT}: ${OS} ${URL}"
        fi
    else
        OS=$(printf "%-36s" "${OS}:")
        echo -e "${OS} ${URL}"
    fi
}


function test_all() {
    OS="${1}"
    os_supported

    local CHECK=""
    local FUNC="${OS}"
    if [[ "${OS}" == *ubuntu* && "${OS}" != "ubuntu-server" ]]; then
        FUNC="ubuntu"
    fi
    local URL=""

    for RELEASE in $("releases_${FUNC}"); do
        if [[ $(type -t "editions_${OS}") == function ]]; then
            for EDITION in $(editions_"${OS}"); do
                # Check architecture support before generating URL
                if ! is_arch_supported "${OS}" "${ARCH}"; then
                    test_result "${OS}" "${RELEASE}" "${EDITION}" "" "SKIP" "(not available for ${ARCH})"
                    continue
                fi
                validate_release releases_"${OS}"
                URL=$(get_"${OS}" | cut -d' ' -f1 | head -n 1)
                if [ "${OPERATION}" == "show" ]; then
                    test_result "${OS}" "${RELEASE}" "${EDITION}" "${URL}"
                elif [ "${OPERATION}" == "test" ]; then
                    CHECK=$(web_check "${URL}" && echo "PASS" || echo "FAIL")
                    test_result "${OS}" "${RELEASE}" "${EDITION}" "${URL}" "${CHECK}"
                fi
            done
        elif [[ "${OS}" == "windows"* ]]; then
            "languages_${OS}"
            for I18N in "${I18NS[@]}"; do
                validate_release releases_"${OS}"
                if [ "${OPERATION}" == "show" ]; then
                    test_result "${OS}" "${RELEASE}" "${I18N}" ""
                elif [ "${OPERATION}" == "test" ]; then
                    test_result "${OS}" "${RELEASE}" "${I18N}" "${URL}" "SKIP"
                fi
            done
        elif [[ "${OS}" == "macos" ]]; then
            validate_release releases_macos
            (get_macos)
        elif [ "${OS}" == "ubuntu-server" ]; then
            # Check architecture support before generating URL
            if ! is_arch_supported "${OS}" "${ARCH}"; then
                test_result "${OS}" "${RELEASE}" "" "" "SKIP" "(not available for ${ARCH})"
                continue
            fi
            validate_release releases_ubuntu-server
            (get_ubuntu-server)
        elif [[ "${OS}" == *ubuntu* ]]; then
            # Ubuntu desktop is amd64 only (no arch function = amd64 default)
            if ! is_arch_supported "${OS}" "${ARCH}"; then
                test_result "${OS}" "${RELEASE}" "" "" "SKIP" "(not available for ${ARCH})"
                continue
            fi
            validate_release releases_ubuntu
            (get_ubuntu)
        else
            # Check architecture support before generating URL
            if ! is_arch_supported "${OS}" "${ARCH}"; then
                test_result "${OS}" "${RELEASE}" "" "" "SKIP" "(not available for ${ARCH})"
                continue
            fi
            validate_release releases_"${OS}"
            URL=$(get_"${OS}" | cut -d' ' -f1 | head -n 1)
            if [ "${OPERATION}" == "show" ]; then
                test_result "${OS}" "${RELEASE}" "${EDITION}" "${URL}"
            elif [ "${OPERATION}" == "test" ]; then
                CHECK=$(web_check "${URL}" && echo "PASS" || echo "FAIL")
                test_result "${OS}" "${RELEASE}" "${EDITION}" "${URL}" "${CHECK}"
            fi
        fi
    done
}

# Report the newest upstream release vs local VM directories for --update.
function catalog_update_check() {
    local OS="${1}" LATEST="" LOCAL_DIRS="" d=""
    case ${OS} in
        *ubuntu-server*) LATEST=$(releases_ubuntu-server | tr ' ' '\n' | head -1);;
        *)               LATEST=$(releases_"${OS}" | tr ' ' '\n' | head -1);;
    esac
    echo "Latest ${OS} release upstream: ${LATEST}"
    LOCAL_DIRS=$(find . -maxdepth 1 -type d -name "${OS}-*" | sed 's|^\./||' | sort)
    if [ -n "${LOCAL_DIRS}" ]; then
        echo "Local VM dirs:"
        for d in ${LOCAL_DIRS}; do
            echo "  ${d}$([ "${d}" == "${OS}-${LATEST}" ] && echo '  (up to date)' || echo '  (older release available)')"
        done
    else
        echo "No local ${OS} VM directories found."
    fi
    exit 0
}

# Check download URLs for all architectures for a specific release/edition.
function catalog_check_all_arch() {
    local OS="${1}" CHECK_ARCH="" URL="" CHECK=""
    RELEASE="${2}"
    EDITION="${3:-}"
    for CHECK_ARCH in amd64 arm64; do
        ARCH="${CHECK_ARCH}"
        # Check architecture support before testing URL
        if ! is_arch_supported "${OS}" "${ARCH}"; then
            if [ -n "${EDITION}" ]; then
                test_result "${OS}" "${RELEASE}" "${EDITION}" "" "SKIP" "(not available for ${ARCH})"
            else
                test_result "${OS}" "${RELEASE}" "" "" "SKIP" "(not available for ${ARCH})"
            fi
            continue
        fi
        # Validate release
        case ${OS} in
            *ubuntu-server*) validate_release releases_ubuntu-server;;
            *ubuntu*) validate_release releases_ubuntu;;
            *) validate_release "releases_${OS}";;
        esac
        # Generate and check URL
        if [[ $(type -t "editions_${OS}") == function ]] && [ -n "${EDITION}" ]; then
            URL=$(get_"${OS}" | cut -d' ' -f1 | head -n 1)
        elif [ "${OS}" == "macos" ]; then
            (get_macos)
            continue
        elif [[ "${OS}" == *"ubuntu-server"* ]]; then
            (get_ubuntu-server)
            continue
        elif [[ "${OS}" == *"ubuntu"* ]]; then
            (get_ubuntu)
            continue
        elif [[ "${OS}" == "windows"* ]]; then
            test_result "${OS}" "${RELEASE}" "${EDITION}" "" "SKIP" "(Windows not supported in check mode)"
            continue
        else
            URL=$(get_"${OS}" | cut -d' ' -f1 | head -n 1)
        fi
        CHECK=$(web_check "${URL}" && echo "PASS" || echo "FAIL")
        test_result "${OS}" "${RELEASE}" "${EDITION}" "${URL}" "${CHECK}"
    done
    exit 0
}
