# shellcheck shell=bash
# Process/tool helpers shared across actions.

require_qemu_img() {
    if [ -z "${QEMU_IMG}" ]; then
        QEMU_IMG=$(command -v qemu-img)
    fi
    if [ -z "${QEMU_IMG}" ] || [ ! -x "${QEMU_IMG}" ]; then
        echo "ERROR! qemu-img not found. Please install qemu-img."
        exit 1
    fi
}
