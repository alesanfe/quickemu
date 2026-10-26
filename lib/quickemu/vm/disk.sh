# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# VM disk image operations.

function delete_disk() {
    echo "Deleting ${VMNAME} virtual hard disk"
    if vm_running "${VMDIR}" "${VMNAME}"; then
        echo "ERROR! Stop ${VMNAME} before deleting its disk."
        exit 1
    fi
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

# --disk-compact: rewrite the qcow2 dropping zeroed/deallocated blocks
# (qemu-img convert to self via a temp file + atomic swap).
function disk_compact() {
    # Order matches disk_resize: refuse running VM first, then missing image.
    if vm_running "${VMDIR}" "${VMNAME}"; then
        echo "ERROR! Stop ${VMNAME} before compacting its disk."
        return 1
    fi
    if [ -z "${disk_img}" ] || [ ! -f "${disk_img}" ]; then
        echo "ERROR! ${disk_img:-disk_img} not found."
        return 1
    fi
    require_qemu_img
    local OUT="${disk_img}.compact"
    ${QEMU_IMG} convert -O "${disk_format:-qcow2}" "${disk_img}" "${OUT}" || { rm -f "${OUT}"; echo "ERROR! Failed to compact ${disk_img}."; return 1; }
    mv "${OUT}" "${disk_img}" || { rm -f "${OUT}"; echo "ERROR! Failed to replace ${disk_img}."; return 1; }
    command -v qe_event >/dev/null 2>&1 && qe_event "vm.disk-compacted"
    echo " - Compacted ${disk_img}"
}
