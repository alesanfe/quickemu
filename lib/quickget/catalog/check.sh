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
