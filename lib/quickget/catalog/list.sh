# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154,SC2317
# Sourced partial: shares the global namespace of bin/quickget.
# Catalog: --list output emitters.


function list_json() {
    # Reference: https://stackoverflow.com/a/67359273
    list_csv | jq -R 'split(",") as $h|reduce inputs as $in ([]; . += [$in|split(",")|. as $a|reduce range(0,length) as $i ({};.[$h[$i]]=$a[$i])])'
    exit 0
}


function list_csv() {
    CSV_DATA="$(csv_data)"

    echo "Display Name,OS,Release,Option,Downloader,PNG,SVG"
    sort -s -t',' -k2,2 <<<"${CSV_DATA}"

    exit 0
}


function csv_data() {
    local DISPLAY_NAME
    local DL=""
    local DOWNLOADER
    local FUNC
    local OPTION
    local OS
    local PNG
    local RELEASE
    local SVG
    local HAS_ZSYNC=0

    # Check if zsync is available
    if command -v zsync &>/dev/null; then
        HAS_ZSYNC=1
    fi

    for OS in $(os_support); do
        local EDITIONS=""
        DISPLAY_NAME="$(pretty_name "${OS}")"

        case ${OS} in
            *ubuntu-server*) FUNC="ubuntu-server";;
            *ubuntu*) FUNC="ubuntu";;
            *) FUNC="${OS}";;
        esac

        PNG="https://quickemu-project.github.io/quickemu-icons/png/${FUNC}/${FUNC}-quickemu-white-pinkbg.png"
        SVG="https://quickemu-project.github.io/quickemu-icons/svg/${FUNC}/${FUNC}-quickemu-white-pinkbg.svg"

        if [[ $(type -t "editions_${OS}") == function ]]; then
            EDITIONS=$(editions_"${OS}")
        fi

        for RELEASE in $("releases_${FUNC}"); do
            if [[ "${OS}" == *"ubuntu"* ]] && [[ ${RELEASE} == *"daily"*  ]] && [ ${HAS_ZSYNC} -eq 1 ]; then
                DOWNLOADER="zsync"
            else
                DOWNLOADER="${DL}"
            fi

            # If the OS has an editions_() function, use it.
            if [[ ${EDITIONS} ]]; then
                for OPTION in ${EDITIONS}; do
                    echo "${DISPLAY_NAME},${OS},${RELEASE},${OPTION},${DOWNLOADER},${PNG},${SVG}"
                done
            elif [[ "${OS}" == "windows"* ]]; then
                "languages_${OS}"
                for I18N in "${I18NS[@]}"; do
                    echo "${DISPLAY_NAME},${OS},${RELEASE},${I18N},${DOWNLOADER},${PNG},${SVG}"
                done
            else
                echo "${DISPLAY_NAME},${OS},${RELEASE},,${DOWNLOADER},${PNG},${SVG}"
            fi
        done &
    done
    wait
}


function list_supported() {
    list_csv | cut -d ',' -f2,3,4 | tr ',' ' '
    exit 0
}
