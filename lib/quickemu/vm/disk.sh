# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# VM disk image operations.

function delete_disk() {
    echo "Deleting ${VMNAME} virtual hard disk"
    if [ -e "${disk_img}" ]; then
        rm "${disk_img}" >/dev/null 2>&1
        # Remove any EFI vars, but not for macOS
        rm "${VMDIR}"/OVMF_VARS*.fd >/dev/null 2>&1
        rm "${VMDIR}/${VMNAME}-vars.fd" >/dev/null 2>&1
        echo " - Deleted ${disk_img}"
        delete_shortcut
    else
        echo " - ${disk_img} not found. Doing nothing."
    fi
}

function disk_resize() {
    if [ -z "${DISK_SIZE_ARG}" ]; then
        echo "ERROR! '--disk-resize' needs a size (e.g. 32G)."
        exit 1
    fi
    if vm_running "${VMDIR}" "${VMNAME}"; then
        echo "ERROR! Stop ${VMNAME} before resizing."
        exit 1
    fi
    if [ ! -f "${disk_img}" ]; then
        echo "ERROR! ${disk_img} not found."
        exit 1
    fi
    require_qemu_img
    if ${QEMU_IMG} resize "${disk_img}" "${DISK_SIZE_ARG}"; then
        echo " - Resized ${disk_img} to ${DISK_SIZE_ARG}."
        echo "             Remember to grow the partition and filesystem inside the guest."
    else
        echo "ERROR! Failed to resize ${disk_img}."
        exit 1
    fi
}

function disk_compact() {
    local TMP="${disk_img}.compact"
    if vm_running "${VMDIR}" "${VMNAME}"; then
        echo "ERROR! Stop ${VMNAME} before compacting."
        exit 1
    fi
    if [ ! -f "${disk_img}" ]; then
        echo "ERROR! ${disk_img} not found."
        exit 1
    fi
    require_qemu_img
    if ${QEMU_IMG} convert -O "${disk_format}" "${disk_img}" "${TMP}"; then
        mv "${TMP}" "${disk_img}"
        echo " - Compacted ${disk_img}."
    else
        rm -f "${TMP}"
        echo "ERROR! Failed to compact ${disk_img}."
        exit 1
    fi
}
