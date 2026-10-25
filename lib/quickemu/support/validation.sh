# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# Input validation helpers shared across modules.

is_numeric() { # value -> 0 when it is a non-negative integer
    [[ "${1}" =~ ^[0-9]+$ ]]
}

# host_has_tool (was host/capabilities.sh)
host_has_tool() { # tool -> 0 when available on PATH
    command -v "${1}" >/dev/null 2>&1
}

# require_qemu_img (was support/process.sh)

require_qemu_img() {
    if [ -z "${QEMU_IMG}" ]; then
        QEMU_IMG=$(command -v qemu-img)
    fi
    if [ -z "${QEMU_IMG}" ] || [ ! -x "${QEMU_IMG}" ]; then
        echo "ERROR! qemu-img not found. Please install qemu-img."
        exit 1
    fi
}
