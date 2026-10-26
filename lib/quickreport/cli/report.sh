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

# qr_redact: stream filter removing host-identifying secrets from the
# report (HOME, username, hostname, IPs, MACs, credentialed URLs).
qr_redact() {
    sed -E \
        -e "s|${HOME//\//\/}|<home>|g" \
        -e "s|${USER:-nobody}|<user>|g" \
        -e "s|$(hostname -s 2>/dev/null || hostname)|<host>|g" \
        -e 's|([0-9]{1,3}\.){3}[0-9]{1,3}|<ip>|g' \
        -e 's|([0-9a-fA-F]{2}:){5}[0-9a-fA-F]{2}|<mac>|g' \
        -e 's|(https?://)[^:/@]+:[^@/]+@|\1<creds>@|g'
}

# Structured report: the same facts as a versioned JSON envelope.
quick_report_json() {
    local QEMU_VER=""
    QEMU_ARCH=$(probe_qemu_arch)
    command -v "qemu-system-${QEMU_ARCH}" >/dev/null 2>&1 \
        && QEMU_VER=$("qemu-system-${QEMU_ARCH}" --version 2>/dev/null | head -n1)
    printf '{"schemaVersion":1,"command":"host.report","success":true,"data":{"os":"%s","kernel":"%s","arch":"%s","quickemu":"%s","qemu":"%s","curl":%s},"warnings":[],"errors":[],"meta":{"redacted":true}}\n' \
        "$(probe_os_name | qr_redact)" "$(uname -sr)" "$(uname -m)" \
        "$(probe_quickemu "${0}")" "${QEMU_VER}" \
        "$(command -v curl >/dev/null && echo true || echo false)" | qr_redact
}
