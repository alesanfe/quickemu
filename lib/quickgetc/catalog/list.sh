# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154,SC2317
# Sourced partial: shares the global namespace of bin/quickgetc.
# Catalog: --list output emitters.


function list_json() {
    # Reference: https://stackoverflow.com/a/67359273
    list_csv | jq -R 'split(",") as $h|reduce inputs as $in ([]; . += [$in|split(",")|. as $a|reduce range(0,length) as $i ({};.[$h[$i]]=$a[$i])])'

    exit 0
}


function list_csv() {
    echo "Display Name,OS,Release,Option,Downloader,PNG,SVG"

    jq -r '.[] | .name as $name | .pretty_name as $pretty_name | .releases[] | [$pretty_name, $name, .release, .edition, null, "https://quickemu-project.github.io/quickemu-icons/png/\($name)/\($name)-quickemu-white-pinkbg.png", "https://quickemu-project.github.io/quickemu-icons/svg/\($name)/\($name)-quickemu-white-pinkbg.svg"] | @tsv' <<< "${JSON}" | tr "\t" ","

    exit 0
}


function list_editions() {
    local RELEASE=${1}
    jq -r '[.releases[] | select(.release=="'"${RELEASE}"'") | .edition] | unique | join(" ")' <<< "${OS_ENTRY}"
}


function list_releases() {
    local CURRENT_EDITIONS
    RELEASES=$(list_releases_raw)
    declare -A EDITIONS
    for RELEASE in ${RELEASES}; do
        CURRENT_EDITIONS=$(list_editions "${RELEASE}")
        EDITIONS["${RELEASE}"]=${CURRENT_EDITIONS}

        if [ "${CURRENT_EDITIONS}" != "${EDITIONS["${RELEASES%% *}"]}" ]; then
            UNIQUE_EDITIONS=1
        elif [ "${CURRENT_EDITIONS}" ]; then
            HAS_EDITIONS=1
        fi
    done


    if [ "${UNIQUE_EDITIONS}" ]; then
        printf " - %-15s | %s\n" "Releases" "Editions"
        for RELEASE in ${RELEASES}; do
            printf "  %-16s | %s\n" "${RELEASE}" "$(echo "${EDITIONS["${RELEASE}"]}" | tr "!" " ")"
        done
    else
        echo " - Releases: $(list_releases_raw)"
        if [ "${HAS_EDITIONS}" ]; then
            echo " - Editions: ${CURRENT_EDITIONS}"
        fi
    fi
}


function list_releases_raw() {
    # Reference: https://unix.stackexchange.com/questions/738691/get-unique-without-sorting-in-jq
    jq -r 'reduce (.releases[].release) as $a ([]; if IN(.[]; $a) then . else . += [$a] end) | join(" ")' <<< "${OS_ENTRY}"
}

function list_supported() {
    list_csv | cut -d ',' -f2,3,4 | tr ',' ' '
    exit 0
}
