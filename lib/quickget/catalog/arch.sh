# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154,SC2317
# Sourced partial: shares the global namespace of bin/quickget.
# Catalog: per-OS architecture support queries.


# Check if a given architecture is supported for an OS
function is_arch_supported() {
    local OS="${1}"
    local CHECK_ARCH="${2}"
    local SUPPORTED=""

    # Check if arch function exists for this OS
    if [[ $(type -t "arch_${OS}") == function ]]; then
        SUPPORTED=$(arch_"${OS}")
    else
        # Default: amd64 only
        SUPPORTED="amd64"
    fi

    # Check if requested arch is in supported list
    [[ " ${SUPPORTED} " == *" ${CHECK_ARCH} "* ]]
}


# Get supported architectures for an OS
function get_supported_archs() {
    local OS="${1}"
    # Check if arch function exists for this OS
    if [[ $(type -t "arch_${OS}") == function ]]; then
        arch_"${OS}"
    else
        # Default: amd64 only
        echo "amd64"
    fi
}
