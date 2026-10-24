# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the global namespace of bin/quickreport.
# Report composition: formats host probe results into the quickreport output.

function quick_report() {
    local GPUS
    local PRETTY_NAME
    local QUICKEMU
    local QEMU_ARCH
    local VERSION

    PRETTY_NAME=$(probe_os_name)
    QUICKEMU=$(probe_quickemu)

    if [ -n "${QUICKEMU}" ]; then
        VERSION=$(${QUICKEMU} --version)
        echo \
            "----------------------------------
        Quickemu ${VERSION}
----------------------------------"
        echo -e "Distro:\t${PRETTY_NAME}"
        echo -e "Kernel:\t$(uname -s -r -m)"
        echo -e "Memory:\t$(probe_memory_gb)G"

        # Break IFS on new line
        IFS=$'\n'
        GPUS=$(probe_gpus)
        if [ "$(echo "${GPUS}" | wc -l)" -eq 1 ]; then
            echo "GPU:"
        else
            echo "GPUs:"
        fi
        for GPU in ${GPUS}; do
            echo " -${GPU}"
        done
    else
        echo \
            "----------------------------------
        Quickemu missing!
----------------------------------"
        exit 1
    fi

    if command -v curl &>/dev/null; then
        VERSION=$(curl --version)
        echo \
            "----------------------------------
            curl $(echo "${VERSION}" | head -n 1 | cut -d' ' -f2)
----------------------------------"
        echo -e "Libraries:$(echo "${VERSION}" | head -n 1 | cut -d')' -f2-)"
        echo -e "Protocols:$(echo "${VERSION}" | tail -n +3 | head -n 1 | cut -d':' -f2-)"
        echo -e "Features: $(echo "${VERSION}" | tail -n +4 | head -n 1 | cut -d':' -f2-)"
    else
        echo \
            "----------------------------------
            curl missing
----------------------------------"
    fi

    QEMU_ARCH=$(probe_qemu_arch)
    if command -v "qemu-system-${QEMU_ARCH}" &>/dev/null; then
        VERSION=$("qemu-system-${QEMU_ARCH}" --version | head -n 1 | cut -d' ' -f4)
        echo \
            "----------------------------------
            QEMU ${VERSION}
----------------------------------"
        "qemu-system-${QEMU_ARCH}" -cpu help
    else
        echo \
            "----------------------------------
            QEMU missing
----------------------------------"
    fi

    echo \
        "----------------------------------
               CPU
----------------------------------"
    probe_cpu_info
}
