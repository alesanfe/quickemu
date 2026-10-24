# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154,SC2317
# Sourced partial: shares the global namespace of bin/quickgetc.
# Download: remote catalog dataset fetch and cache validity.


function cache_is_valid() {
    if [ ! -f "${1}" ]; then
        return 1
    fi

    CURRENT_DAY=$(date -u +%j%Y)
    CACHE_DAY=$(date -u -d "$(stat -c %y "${1}")" +%j%Y)

    if [ "${CURRENT_DAY}" -ne "${CACHE_DAY}" ]; then
        return 1
    else 
        return 0
    fi
}


function populate_cache() {
    local CACHE_DIR="${XDG_CACHE_HOME:-${HOME}/.cache}"
    if ! cache_is_valid "${CACHE_DIR}/${CACHE_FILE}"; then
        echo "Downloading quickget data from ${CACHE_URL}"
        web_get "${CACHE_URL}" "${CACHE_DIR}" "${CACHE_FILE}"
    fi 
    if ! JSON=$(zstd -dc "${CACHE_DIR}/${CACHE_FILE}"); then
        echo "ERROR! Failed to decompress ${CACHE_DIR}/${CACHE_FILE}. Try running quickgetc again."
        rm "${CACHE_DIR}/${CACHE_FILE}"
        exit 1
    fi
}
