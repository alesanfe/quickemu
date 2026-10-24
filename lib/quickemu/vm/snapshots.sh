# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# VM snapshot operations.

function snapshot_apply() {
    echo "Snapshot apply to ${disk_img}"
    local TAG="${1}"
    if [ -z "${TAG}" ]; then
        echo " - ERROR! No snapshot tag provided."
        exit
    fi

    if [ -e "${disk_img}" ]; then
        if ${QEMU_IMG} snapshot -q -a "${TAG}" "${disk_img}"; then
            echo " - Applied snapshot '${TAG}' to ${disk_img}"
        else
            echo " - ERROR! Failed to apply snapshot '${TAG}' to ${disk_img}"
        fi
    else
        echo " - NOTE! ${disk_img} not found. Doing nothing."
    fi
}

function snapshot_create() {
    echo "Snapshotting ${disk_img}"
    local TAG="${1}"
    if [ -z "${TAG}" ]; then
        echo "- ERROR! No snapshot tag provided."
        exit
    fi

    if [ -e "${disk_img}" ]; then
        if ${QEMU_IMG} snapshot -q -c "${TAG}" "${disk_img}"; then
            echo " - Created snapshot '${TAG}' for ${disk_img}"
        else
            echo " - ERROR! Failed to create snapshot '${TAG}' for ${disk_img}"
        fi
    else
        echo " - NOTE! ${disk_img} not found. Doing nothing."
    fi
}

function snapshot_delete() {
    echo "Snapshot removal ${disk_img}"
    local TAG="${1}"
    if [ -z "${TAG}" ]; then
        echo " - ERROR! No snapshot tag provided."
        exit
    fi

    if [ -e "${disk_img}" ]; then
        if ${QEMU_IMG} snapshot -q -d "${TAG}" "${disk_img}"; then
            echo " - Deleted snapshot '${TAG}' from ${disk_img}"
        else
            echo " - ERROR! Failed to delete snapshot '${TAG}' from ${disk_img}"
        fi
    else
        echo " - NOTE! ${disk_img} not found. Doing nothing."
    fi
}

function snapshot_info() {
    echo
    if [ -e "${disk_img}" ]; then
        ${QEMU_IMG} info "${disk_img}"
    fi
}
