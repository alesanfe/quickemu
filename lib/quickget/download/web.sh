# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154,SC2317
# Sourced partial: shares the global namespace of bin/quickget.
# Download: HTTP/zsync transport and checksum verification.


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

    # Check for algorithm prefix (e.g., "sha256:abc123..." or "b2sum:abc123...")
    if [[ "${hash}" == *":"* ]]; then
        local hash_prefix="${hash%%:*}"
        hash="${hash#*:}"
        # Normalise algorithm prefix to lowercase
        hash_prefix="$(echo "${hash_prefix}" | tr '[:upper:]' '[:lower:]')"
        case "${hash_prefix}" in
            md5) hash_algo=md5sum;;
            sha1) hash_algo=sha1sum;;
            sha256) hash_algo=sha256sum;;
            sha512) hash_algo=sha512sum;;
            b2sum|blake2|blake2b) hash_algo=b2sum;;
            *) echo "WARNING! Unknown hash algorithm '${hash_prefix}', not checking ${iso} hash."
               return;;
        esac
    else
        # Guess the hash algorithm by the hash length
        case ${#hash} in
            32) hash_algo=md5sum;;
            40) hash_algo=sha1sum;;
            64) hash_algo=sha256sum;;
            128) hash_algo=sha512sum;;
            *) echo "WARNING! Can't guess hash algorithm, not checking ${iso} hash."
                return;;
        esac
    fi

    # On macOS/Darwin, prefer GNU coreutils (prefixed with 'g') if available,
    # otherwise fall back to native 'shasum' command
    local hash_cmd="${hash_algo}"
    if [ "${HOST_OS}" = "Darwin" ]; then
        case ${hash_algo} in
            md5sum)
                if command -v gmd5sum &>/dev/null; then
                    hash_cmd=gmd5sum
                else
                    # MD5 not directly supported by shasum; use native md5 command
                    hash_cmd="md5 -r"
                fi;;
            sha1sum)
                if command -v gsha1sum &>/dev/null; then
                    hash_cmd=gsha1sum
                else
                    hash_cmd="shasum -a 1"
                fi;;
            sha256sum)
                if command -v gsha256sum &>/dev/null; then
                    hash_cmd=gsha256sum
                else
                    hash_cmd="shasum -a 256"
                fi;;
            sha512sum)
                if command -v gsha512sum &>/dev/null; then
                    hash_cmd=gsha512sum
                else
                    hash_cmd="shasum -a 512"
                fi;;
            b2sum)
                if command -v gb2sum &>/dev/null; then
                    hash_cmd=gb2sum
                else
                    echo "WARNING! b2sum not available on macOS without GNU coreutils, not checking ${iso} hash."
                    return
                fi;;
        esac
    fi

    echo -n "Checking ${iso} with ${hash_cmd}... "
    # Handle MD5 on macOS specially (md5 -r outputs "hash filename" format)
    if [ "${hash_cmd}" = "md5 -r" ]; then
        local computed_hash
        computed_hash=$(md5 -r "${iso}" | cut -d' ' -f1)
        if [ "${computed_hash}" != "${hash}" ]; then
            echo "ERROR!"
            echo "${iso} doesn't match ${hash}. Try running 'quickget' again."
            exit 1
        else
            echo "Good!"
        fi
    elif ! printf '%s  %s\n' "${hash}" "${iso}" | ${hash_cmd} --check --status; then
        echo "ERROR!"
        echo "${iso} doesn't match ${hash}. Try running 'quickget' again."
        exit 1
    else
        echo "Good!"
    fi
}


# Download a file from the web and pipe it to stdout
function web_pipe() {
    # https only + bounded: these feed parsers, never file:// or LAN hosts.
    curl --disable --silent --location --fail --proto '=https' --connect-timeout 15 --max-time 60 -- "${1}"
}


# Download a JSON file from the web and pipe it to stdout
function web_pipe_json() {
     curl --disable --silent --location --fail --proto '=https' --connect-timeout 15 --max-time 60 --header "Accept: application/json" -- "${1}"
}


# Download a file from the web
function web_get() {
    local CHECK=""
    local HEADERS=()
    local INSECURE=()
    [ "${CURL_INSECURE}" == "true" ] && INSECURE=("--insecure")
    local URL="${1}"
    local DIR="${2}"
    local FILE=""
    local USER_AGENT="Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36"

    if [ -n "${3}" ]; then
        FILE="${3}"
    else
        FILE="${URL##*/}"
    fi

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
        # Check architecture support before testing URL
        if ! is_arch_supported "${OS}" "${ARCH}"; then
            test_result "${OS}" "${RELEASE}" "${EDITION}" "" "SKIP" "(not available for ${ARCH})"
            exit 0
        fi
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

    if [[ ${OS} != windows && ${OS} != macos && ${OS} != windows-server ]]; then
        echo "Downloading ${PRETTY_OS_NAME:-${OS}} ${RELEASE} ${EDITION}"
        echo "- URL: ${URL}"
    fi

    local PIPE_OPTS=()
    if [ ! -t 1 ]; then
        local CONTENT_LENGTH
        CONTENT_LENGTH=$(curl --disable --silent --head --location \
            --write-out '%{content_length_download}' --output /dev/null \
            --user-agent "${USER_AGENT}" "${HEADERS[@]}" "${INSECURE[@]}" -- "${URL}" 2>/dev/null)
        [ -n "${CONTENT_LENGTH}" ] && echo "content-length: ${CONTENT_LENGTH}"
        PIPE_OPTS=("--stderr" "-")
    fi

    if command -v aria2c >/dev/null 2>&1 && [ -z "${NO_ARIA2}" ]; then
        if ! aria2c -x8 -s8 -k1M --file-allocation=none --continue=true --summary-interval=10 --dir="${DIR}" --out="${FILE}" "${URL}"; then
            echo "WARNING! aria2c failed, falling back to curl."
        else
            return 0
        fi
    fi
    # Bounded download: http(s) only (no file:// or exotic schemes),
    # retry transient failures, abort on a 60s stall below 1KB/s, and
    # keep resume support across attempts.
    if ! curl --disable --progress-bar "${PIPE_OPTS[@]}" --location         --proto '=http,https' --proto-redir '=http,https'         --connect-timeout 30 --retry 3 --retry-delay 2 --retry-connrefused         --speed-limit 1024 --speed-time 60         --output "${DIR}/${FILE}" --continue-at -         --user-agent "${USER_AGENT}" "${HEADERS[@]}" "${INSECURE[@]}" -- "${URL}"; then
        echo "ERROR! Failed to download ${URL} with curl."
        rm -f "${DIR}/${FILE}"
        exit 1
    fi
}


# checks if a URL needs to be redirected and returns the final URL
function web_redirect() {
    local REDIRECT_URL=""
    local URL="${1}"
    # Check for URL redirections
    # Output to nonexistent directory so the download fails fast
    REDIRECT_URL=$(curl --disable --silent --location --fail --proto '=http,https' --connect-timeout 15 --max-time 60 --write-out '%{url_effective}' --output /var/cache/${RANDOM}/${RANDOM} "${URL}" )
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
    if curl --disable --silent --location --head --output /dev/null --fail         --connect-timeout 30 --max-time 30 --retry 3         "${HEADERS[@]}" "${URL}"; then
        return 0
    fi
    # Presigned URLs (S3/R2/Azure SAS) bind the signature to GET and reject
    # HEAD; fall back to a single-byte ranged GET to validate reachability.
    curl --disable --silent --location --range 0-0 --output /dev/null --fail         --connect-timeout 30 --max-time 30 --retry 3         "${HEADERS[@]}" "${URL}"
}


function zsync_get() {
    local CHECK=""
    local DIR="${2}"
    local FILE="${1##*/}"
    local OUT=""
    local URL="${1}"
    # Test mode for ISO
    if [ "${OPERATION}" == "show" ]; then
        test_result "${OS}" "${RELEASE}" "${EDITION}" "${URL}"
        exit 0
    elif [ "${OPERATION}" == "test" ]; then
        # Check architecture support before testing URL
        if ! is_arch_supported "${OS}" "${ARCH}"; then
            test_result "${OS}" "${RELEASE}" "${EDITION}" "" "SKIP" "(not available for ${ARCH})"
            exit 0
        fi
        CHECK=$(web_check "${URL}" && echo "PASS" || echo "FAIL")
        test_result "${OS}" "${RELEASE}" "${EDITION}" "${URL}" "${CHECK}"
        exit 0
    elif command -v zsync &>/dev/null; then
        if [ -n "${3}" ]; then
            OUT="${3}"
        else
            OUT="${FILE}"
        fi

        if ! mkdir -p "${DIR}" 2>/dev/null; then
            echo "ERROR! Unable to create directory ${DIR}"
            exit 1
        fi
        echo "Downloading ${PRETTY_OS_NAME:-${OS}} ${RELEASE} ${EDITION} from ${URL}"
        # Only force http for zsync - not earlier because we might fall through here
        if ! zsync "${URL/https/http}.zsync" -i "${DIR}/${OUT}" -o "${DIR}/${OUT}" 2>/dev/null; then
            echo "ERROR! Failed to download ${URL/https/http}.zsync"
            exit 1
        fi

        if [ -e "${DIR}/${OUT}.zs-old" ]; then
            rm "${DIR}/${OUT}.zs-old"
        fi
    else
        echo "INFO: zsync not found, falling back to curl"
        if [ -n "${3}" ]; then
            web_get "${1}" "${2}" "${3}"
        else
            web_get "${1}" "${2}"
        fi
    fi
}


function handle_curl_error() {
    local error_code="$1"
    local fatal_error_action=2
    case "$error_code" in
        6)
            echo "Failed to resolve Microsoft servers! Is there an Internet connection? Exiting..."
            return "$fatal_error_action"
            ;;
        7)
            echo "Failed to contact Microsoft servers! Is there an Internet connection or is the server down?"
            ;;
        8)
            echo "Microsoft servers returned a malformed HTTP response!"
            ;;
        22)
            echo "Microsoft servers returned a failing HTTP status code!"
            ;;
        23)
            echo "Failed at writing Windows media to disk! Out of disk space or permission error? Exiting..."
            return "$fatal_error_action"
            ;;
        26)
            echo "Ran out of memory during download! Exiting..."
            return "$fatal_error_action"
            ;;
        36)
            echo "Failed to continue earlier download!"
            ;;
        63)
            echo "Microsoft servers returned an unexpectedly large response!"
            ;;
            # POSIX defines exit statuses 1-125 as usable by us
            # https://pubs.opengroup.org/onlinepubs/9699919799/utilities/V3_chap02.html#tag_18_08_02
            $((error_code <= 125)))
            # Must be some other server or network error (possibly with this specific request/file)
            # This is when accounting for all possible errors in the curl manual assuming a correctly formed curl command and an HTTP(S) request, using only the curl features we're using, and a sane build
            echo "Miscellaneous server or network error!"
            ;;
        126 | 127 )
            echo "Curl command not found! Please install curl and try again. Exiting..."
            return "$fatal_error_action"
            ;;
        # Exit statuses are undefined by POSIX beyond this point
        *)
            case "$(kill -l "$error_code")" in
            # Signals defined to exist by POSIX:
            # https://pubs.opengroup.org/onlinepubs/009695399/basedefs/signal.h.html
            INT)
                echo "Curl was interrupted!"
                ;;
            # There could be other signals but these are most common
            SEGV | ABRT )
                echo "Curl crashed! Failed exploitation attempt? Please report any core dumps to curl developers. Exiting..."
                return "$fatal_error_action"
                ;;
            *)
                echo "Curl terminated due to a fatal signal!"
                ;;
            esac
    esac
    return 1
}
