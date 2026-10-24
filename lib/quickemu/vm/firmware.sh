# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# VM firmware artefacts (OVMF/AAVMF vars).

function efi_vars() {
    local VARS_IN=""
    local VARS_OUT=""
    VARS_IN="${1}"
    VARS_OUT="${2}"

    if [ ! -e "${VARS_OUT}" ]; then
        if [ -e "${VARS_IN}" ]; then
            cp "${VARS_IN}" "${VARS_OUT}"
        else
            echo "ERROR! ${VARS_IN} was not found. Please install edk2."
            exit 1
        fi
    fi
}

function is_firmware_qcow2() {
    # Check for the magic bytes that indicate the firmware is in qcow2 format,
    # otherwise default to assuming firmware files are in raw format.
    # Use od to read bytes as hex to avoid null byte warnings in command substitution.
    local magic
    magic=$(od -An -tx1 -N4 "$1" 2>/dev/null | tr -d ' ')
    [ "$magic" = "514649fb" ] && echo "true" || echo "false"
}
