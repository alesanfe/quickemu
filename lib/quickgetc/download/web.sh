# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154,SC2317
# Sourced partial: shares the global namespace of bin/quickgetc.
# Download: HTTP transport, source handling and checksum verification.


function check_hash() {
    local iso=""
    local hash=""
    local hash_algo=""
    if [ "${OPERATION}" == "download" ]; then
        iso="${1}"
    else
        iso="${VM_PATH}/${1}"
    fi
    hash="${2}"
    # Guess the hash algorithm by the hash length
    case ${#hash} in
        32) hash_algo=md5sum;;
        40) hash_algo=sha1sum;;
        64) hash_algo=sha256sum;;
        128) hash_algo=sha512sum;;
        *) echo "WARNING! Can't guess hash algorithm, not checking ${iso} hash."
            return;;
    esac
    echo -n "Checking ${iso} with ${hash_algo}... "
    if ! echo "${hash} ${iso}" | ${hash_algo} --check --status; then
        echo "ERROR!"
        echo "${iso} doesn't match ${hash}. Try running 'quickgetc' again."
        exit 1
    else
        echo "Good!"
    fi
}


function web_get() {
    local CHECK=""
    local HEADERS=()
    local URL="${1}"
    local DIR="${2}"
    local FILE="${3}"
    local USER_AGENT="Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36"

    # Process any URL redirections after the file name has been extracted
    URL=$(web_redirect "${URL}")

    # Process any headers
    while (( "$#" )); do
        if [ "${1}" == "--header" ]; then
            HEADERS+=("${1}" "${2}")
            shift 2
        else
            shift
        fi
    done

    # Test mode for ISO
    if [ "${OPERATION}" == "show" ]; then
        test_result "${OS}" "${RELEASE}" "${EDITION}" "${URL}"
        exit 0
    elif [ "${OPERATION}" == "test" ]; then
        CHECK=$(web_check "${URL}" && echo "PASS" || echo "FAIL")
        test_result "${OS}" "${RELEASE}" "${EDITION}" "${URL}" "${CHECK}"
        exit 0
    elif [ "${OPERATION}" == "download" ]; then
        DIR="$(pwd)"
    fi

    if [ "${DIR}" != "$(pwd)" ] && ! mkdir -p "${DIR}" 2>/dev/null; then
        echo "ERROR! Unable to create directory ${DIR}"
        exit 1
    fi

    if ! curl --progress-bar --location --output "${DIR}/${FILE}" --continue-at - --user-agent "${USER_AGENT}" "${HEADERS[@]}" -- "${URL}"; then
        echo "ERROR! Failed to download ${URL} with curl."
        rm -f "${DIR}/${FILE}"
    fi
}


# checks if a URL needs to be redirected and returns the final URL
function web_redirect() {
    local REDIRECT_URL=""
    local URL="${1}"
    # Check for URL redirections
    # Output to nonexistent directory so the download fails fast
    REDIRECT_URL=$(curl --silent --location --fail --write-out '%{url_effective}' --output /var/cache/${RANDOM}/${RANDOM} "${URL}")
    if [ "${REDIRECT_URL}" != "${URL}" ]; then
        echo "${REDIRECT_URL}"
    else
        echo "${URL}"
    fi
}


# checks if a URL is reachable
function web_check() {
    local HEADERS=()
    local URL="${1}"
    # Process any headers
    while (( "$#" )); do
        if [ "${1}" == "--header" ]; then
            HEADERS+=("${1}" "${2}")
            shift 2
        else
            shift
        fi
    done
    curl --silent --location --head --output /dev/null --fail --connect-timeout 30 --max-time 30 --retry 3 "${HEADERS[@]}" "${URL}"
}



function handle_sources() {
    local IMAGE_TYPE
    local SOURCE

    for IMAGE_TYPE in "${IMAGE_TYPES[@]}"; do
        SOURCE=$(jq -r '."'${IMAGE_TYPE}'"[0]' <<< "${ENTRY}")
        if [ "${SOURCE}" != "null" ]; then
            handle_source_type "${IMAGE_TYPE}" "${SOURCE}"
        fi
    done
}


function handle_source_type() {
    local IMAGE_TYPE="${1}"
    local SOURCE="${2}"
    local WEB_SOURCE
    local FILE_NAME
    local CUSTOM

    WEB_SOURCE=$(jq -r '.web // ""' <<< "${SOURCE}")
    FILE_NAME=$(jq -r '.file_name// ""' <<< "${SOURCE}")
    CUSTOM=$(jq -r '.custom // ""' <<< "${SOURCE}")

    if [ "${WEB_SOURCE}" ]; then
        handle_websource "${IMAGE_TYPE}" "${WEB_SOURCE}"
    elif [ "${FILE_NAME}" ]; then
        IMAGES["${IMAGE_TYPE}"]="${VM_PATH}/${FILE_NAME}"
    elif [ "${CUSTOM}" ]; then
        echo "ERROR! Custom source not implemented"
        exit 1
    else
        echo "ERROR! No source found for ${IMAGE_TYPE}"
        exit 1
    fi
}


function handle_websource() {
    local IMAGE_TYPE="${1}"
    local SOURCE="${2}"
    local URL
    local CHECKSUM
    local ARCHIVE_FORMAT
    local FILE_NAME

    URL=$(jq -r '.url' <<< "${SOURCE}")
    CHECKSUM=$(jq -r '.checksum // ""' <<< "${SOURCE}")
    ARCHIVE_FORMAT=$(jq -r '.archive_format // ""' <<< "${SOURCE}")
    FILE_NAME=$(jq -r '.file_name' <<< "${SOURCE}")

    if [ "${FILE_NAME}" == "null" ]; then
        FILE_NAME="${URL##*/}"
    fi

    web_get "${URL}" "${VM_PATH}" "${FILE_NAME}"
    if [ -n "${CHECKSUM}" ]; then
        check_hash "${FILE_NAME}" "${CHECKSUM}"
    fi

    if [ "${OPERATION}" = "download" ]; then
        FINAL_FILE="${FILE_NAME}"
        VM_PATH="$(pwd)"
    else
        FINAL_FILE="${VM_PATH}/${FILE_NAME}"
    fi

    case "${ARCHIVE_FORMAT}" in
        tar*)
            tar xf "${FINAL_FILE}" -C "${VM_PATH}"
            IMAGES["${IMAGE_TYPE}"]="$(ls -1 "${VM_PATH}/"*".${IMAGE_TYPE}")"
            ;;
        xz)
            xz -d "${FINAL_FILE}"
            IMAGES["${IMAGE_TYPE}"]="${FINAL_FILE/.xz/}"
            ;;
        gz)
            gzip -d "${FINAL_FILE}"
            IMAGES["${IMAGE_TYPE}"]="${FINAL_FILE/.gz/}"
            ;;
        bz2)
            bzip2 -d "${FINAL_FILE}"
            IMAGES["${IMAGE_TYPE}"]="${FINAL_FILE/.bz2/}"
            ;;
        zip)
            unzip -d "${VM_PATH}" "${FINAL_FILE}"
            IMAGES["${IMAGE_TYPE}"]="$(ls -1 "${VM_PATH}/"*".${IMAGE_TYPE}")"
            ;;
        *)
            IMAGES["${IMAGE_TYPE}"]="${FINAL_FILE}"
            ;;
    esac
}
