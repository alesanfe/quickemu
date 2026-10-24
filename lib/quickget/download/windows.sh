# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154,SC2317
# Sourced partial: shares the global namespace of bin/quickget.
# Download: Windows ISO download helpers.


function download_windows_server() {
    local iso_download_page_html=""
    # Copyright (C) 2024 Elliot Killick <contact@elliotkillick.com>
    # This function is adapted from the Mido project:
    # https://github.com/ElliotKillick/Mido

    # Download enterprise evaluation Windows versions
    local windows_version="$1"
    local enterprise_type="$2"
    local PRETTY_RELEASE=""

    case "${RELEASE}" in
        *) PRETTY_RELEASE="${RELEASE}";;
    esac

    echo "Downloading ${PRETTY_OS_NAME:-${OS}} ${PRETTY_RELEASE} (${I18N})"

    local url="https://www.microsoft.com/en-us/evalcenter/download-$windows_version"

    echo " - Parsing download page: ${url}"
    iso_download_page_html="$(curl --disable --silent --location --max-filesize 1M --fail --proto =https --tlsv1.2 --http1.1 -- "$url")" || {
        handle_curl_error $?
        return $?
    }

    if ! [ "$iso_download_page_html" ]; then
        # This should only happen if there's been some change to where this download page is located
        echo " - Windows server download page gave us an empty response"
        return 1
    fi

    local CULTURE=""
    local COUNTRY=""
    case "${I18N}" in
        "English (Great Britain)")
            CULTURE="en-gb"
            COUNTRY="GB";;
        "Chinese (Simplified)")
            CULTURE="zh-cn"
            COUNTRY="CN";;
        "Chinese (Traditional)")
            CULTURE="zh-tw"
            COUNTRY="TW";;
        "French")
            CULTURE="fr-fr"
            COUNTRY="FR";;
        "German")
            CULTURE="de-de"
            COUNTRY="DE";;
        "Italian")
            CULTURE="it-it"
            COUNTRY="IT";;
        "Japanese")
            CULTURE="ja-jp"
            COUNTRY="JP";;
        "Korean")
            CULTURE="ko-kr"
            COUNTRY="KR";;
        "Portuguese (Brazil)")
            CULTURE="pt-br"
            COUNTRY="BR";;
        "Spanish")
            CULTURE="es-es"
            COUNTRY="ES";;
        "Russian")
            CULTURE="ru-ru"
            COUNTRY="RU";;
        *)
            CULTURE="en-us"
            COUNTRY="US";;
    esac

    echo " - Getting download link.."
    iso_download_links="$(echo "$iso_download_page_html" | grep -o "https://go.microsoft.com/fwlink/p/?LinkID=[0-9]\+&clcid=0x[0-9a-z]\+&culture=${CULTURE}&country=${COUNTRY}")" || {
        # This should only happen if there's been some change to the download endpoint web address
        echo " - Windows server download page gave us no download link"
        return 1
    }

    # Limit untrusted size for input validation
    iso_download_links="$(echo "$iso_download_links" | head -c 1024)"

    case "$enterprise_type" in
        # Select x64 download link
        "enterprise") iso_download_link=$(echo "$iso_download_links" | head -n 2 | tail -n 1) ;;
        # Select x64 LTSC download link
        "ltsc") iso_download_link=$(echo "$iso_download_links" | head -n 4 | tail -n 1) ;;
        *) iso_download_link="$iso_download_links" ;;
    esac

    # Follow redirect so proceeding log message is useful
    # This is a request we make this Fido doesn't
    # We don't need to set "--max-filesize" here because this is a HEAD request and the output is to /dev/null anyway
    iso_download_link="$(curl --disable --silent --location --output /dev/null --silent --write-out "%{url_effective}" --head --fail --proto =https --tlsv1.2 --http1.1 -- "$iso_download_link")" || {
        # This should only happen if the Microsoft servers are down
        handle_curl_error $?
        return $?
    }

    # Limit untrusted size for input validation
    iso_download_link="$(echo "$iso_download_link" | head -c 1024)"

    echo " - URL: $iso_download_link"

    # Download ISO
    FILE_NAME="${iso_download_link##*/}"
    web_get "${iso_download_link}" "${VM_PATH}" "${FILE_NAME}"
    OS="windows-server"
}


function download_windows_workstation() {
    local HASH=""
    local session_id=""
    local iso_download_page_html=""
    local product_edition_id=""
    local language_skuid_table_json=""
    local sku_id=""
    local iso_download_link_json=""
    local iso_download_link=""

    echo "Downloading Windows ${RELEASE} (${I18N})"
    # This function is adapted from the Mido project:
    # https://github.com/ElliotKillick/Mido
    # Download newer consumer Windows versions from behind gated Microsoft API

    # Either 10, or 11
    local windows_version="$1"

    local url="https://www.microsoft.com/en-us/software-download/windows$windows_version"
    case "$windows_version" in
        10) url="${url}ISO";;
    esac

    local user_agent="Mozilla/5.0 (X11; Linux x86_64; rv:100.0) Gecko/20100101 Firefox/100.0"
    # Fall back to the kernel UUID source when uuidgen is not installed,
    # an empty session id makes Microsoft reject the download request.
    if command -v uuidgen &>/dev/null; then
        session_id="$(uuidgen)"
    else
        session_id="$(cat /proc/sys/kernel/random/uuid)"
    fi

    # Get product edition ID for latest release of given Windows version
    # Product edition ID: This specifies both the Windows release (e.g. 22H2) and edition ("multi-edition" is default, either Home/Pro/Edu/etc., we select "Pro" in the answer files) in one number
    # This is the *only* request we make that Fido doesn't. Fido manually maintains a list of all the Windows release/edition product edition IDs in its script (see: $WindowsVersions array). This is helpful for downloading older releases (e.g. Windows 10 1909, 21H1, etc.) but we always want to get the newest release which is why we get this value dynamically
    # Also, keeping a "$WindowsVersions" array like Fido does would be way too much of a maintenance burden
    # Remove "Accept" header that curl sends by default
    echo " - Parsing download page: ${url}"
    iso_download_page_html="$(curl --disable --silent --user-agent "$user_agent" --header "Accept:" --max-filesize 1M --fail --proto =https --tlsv1.2 --http1.1 -- "$url")" || {
        handle_curl_error $?
        return $?
    }

    echo -n " - Getting Product edition ID: "
    # tr: Filter for only numerics to prevent HTTP parameter injection
    # head -c was recently added to POSIX: https://austingroupbugs.net/view.php?id=407
    product_edition_id="$(echo "$iso_download_page_html" | grep -Eo '<option value="[0-9]+">Windows' | cut -d '"' -f 2 | head -n 1 | tr -cd '0-9' | head -c 16)"
    echo "$product_edition_id"

    echo " - Permit Session ID: $session_id"
    # Permit Session ID
    # "org_id" is always the same value
    curl --disable --silent --output /dev/null --user-agent "$user_agent" --header "Accept:" --max-filesize 100K --fail --proto =https --tlsv1.2 --http1.1 -- "https://vlscppe.microsoft.com/tags?org_id=y6jn8c31&session_id=$session_id" || {
        # This should only happen if there's been some change to how this API works
        handle_curl_error $?
        return $?
    }

    echo " - Request Microsoft ov-df mdt.js"

    # "instanceId" is always the same value
    local instanceId="560dc9f3-1aa5-4a2f-b63c-9e18f8d0e175"
    local mdt_js_response=""

    mdt_js_response="$(curl --disable --silent --user-agent "$user_agent" --header "Accept:" --max-filesize 100K --fail --proto =https --tlsv1.2 --http1.1 -- "https://ov-df.microsoft.com/mdt.js?instanceId={$instanceId}&PageId=si&session_id=$session_id")" || {
        handle_curl_error $?
        return $?
    }

    local w=""
    w="$(echo "$mdt_js_response" | grep -Eo '[?&]w=[A-F0-9]+' | head -1)"
    w="${w#*=}"

    local rticks=""
    rticks="$(echo "$mdt_js_response" | grep -Eo 'rticks="?\+?[0-9]+' | head -1)"
    rticks="${rticks##*[=\"+]}"

    local current_time=""
    current_time="$(date +%s%3N)"

    curl --disable --silent --user-agent "$user_agent" --header "Accept:" --max-filesize 100K --fail --proto =https --tlsv1.2 --http1.1 -- "https://ov-df.microsoft.com/?session_id={$session_id}&CustomerId={$instanceId}&PageId=si&w={$w}&mdt={$current_time}&rticks={$rticks}" || {
        handle_curl_error $?
        return $?
    }

    local profile="606624d44113"

    echo -n " - Getting language SKU ID: "
    # Get language -> skuID association table
    language_skuid_table_json="$(curl --disable -s --fail --max-filesize 100K --proto =https --tlsv1.2 --http1.1 "https://www.microsoft.com/software-download-connector/api/getskuinformationbyproductedition?profile=${profile}&ProductEditionId=${product_edition_id}&SKU=undefined&friendlyFileName=undefined&Locale=en-US&sessionID=${session_id}")" || {
        handle_curl_error $?
        return $?
    }

    sku_id="$(echo "${language_skuid_table_json}" | jq -r '.Skus[] | select(.LocalizedLanguage=="'"${I18N}"'" or .Language=="'"${I18N}"'").Id')"
    echo "$sku_id"

    echo " - Getting ISO download link..."
    # Get ISO download link
    # If any request is going to be blocked by Microsoft it's always this last one (the previous requests always seem to succeed)
    # --referer: Required by Microsoft servers to allow request
    iso_download_link_json="$(curl --disable -s --fail --referer "$url" "https://www.microsoft.com/software-download-connector/api/GetProductDownloadLinksBySku?profile=${profile}&productEditionId=undefined&SKU=${sku_id}&friendlyFileName=undefined&Locale=en-US&sessionID=${session_id}")"

    local failed=0

    if ! [ "$iso_download_link_json" ]; then
        # This should only happen if there's been some change to how this API works
        echo " - Microsoft servers gave us an empty response to our request for an automated download."
        failed=1
    fi

    if echo "$iso_download_link_json" | grep -q "Sentinel marked this request as rejected."; then
        echo " - WARNING! Microsoft blocked the automated download request based on your IP address."
        failed=1
    fi

    if [ ${failed} -eq 1 ]; then
        echo "   Manually download the Windows ${windows_version} ISO using a web browser from: ${url}"
        echo "   Save the downloaded ISO to: $(realpath "${VM_PATH}")"
        echo "   Update the config file to reference the downloaded ISO: ./${VM_PATH}.conf"
        echo "   Continuing with the VM creation process..."
        return 1
    fi

    # Filter for 64-bit ISO download URL
    iso_download_link="$(echo "${iso_download_link_json}" | jq -r '.ProductDownloadOptions[].Uri' | grep x64)"

    if ! [ "$iso_download_link" ]; then
        # This should only happen if there's been some change to the download endpoint web address
        echo " - Microsoft servers gave us no download link to our request for an automated download. Please manually download this ISO in a web browser: $url"
        return 1
    fi

    echo " - URL: ${iso_download_link%%\?*}"

    # Download ISO
    FILE_NAME="$(echo "$iso_download_link" | cut -d'?' -f1 | cut -d'/' -f5)"
    web_get "${iso_download_link}" "${VM_PATH}" "${FILE_NAME}"
}
