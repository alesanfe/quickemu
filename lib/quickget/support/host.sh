# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154,SC2317
# Sourced partial: shares the global namespace of bin/quickget.
# Support: host/tool resolution helpers.


function arch_suffix() {
    # Return architecture suffix for foreign architectures, empty for native
    if [ "${ARCH}" != "${NORMALISED_HOST_ARCH}" ]; then
        echo "-${ARCH}"
    fi
}


function cleanup() {
    if [ -n "$(jobs -p)" ]; then
        kill "$(jobs -p)" 2>/dev/null
    fi
}


function require_qemu_img() {
    QEMU_IMG=$(command -v qemu-img)
    if [ ! -x "${QEMU_IMG}" ]; then
        echo "ERROR! qemu-img not found. Please make sure qemu-img is installed."
        exit 1
    fi
}


# Use command -v command to check if quickemu is in the system's PATH and
# fallback to checking if quickemu is in the current directory.
function resolve_quickemu() {
    command -v quickemu || \
    if [ -x "./quickemu" ]; then
        echo "$(pwd)/quickemu"
    else
        echo "quickemu not found" >&2
        exit 1
    fi
}
