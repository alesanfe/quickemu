# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# Host capability probes (hardware, tools, resources).

host_has_kvm() {                               # -> 0 when KVM acceleration is usable
    [ "${OS_KERNEL}" == "Darwin" ] && return 0 # macOS uses HVF
    [ -w /dev/kvm ]
}

host_has_tool() { # tool -> 0 when available on PATH
    command -v "${1}" >/dev/null 2>&1
}

host_free_mem_mb() { # -> available memory in MiB (0 when unknown)
    awk '/MemAvailable/{print int($2/1024); exit}' /proc/meminfo 2>/dev/null || echo 0
}

function get_qemu_share_path() {
    local qemu_bin="${QEMU}"
    local qemu_real qemu_prefix share_path

    # Resolve the actual binary path (handles symlinks, Nix store paths)
    if command -v realpath &>/dev/null; then
        qemu_real=$(realpath "${qemu_bin}" 2>/dev/null) || qemu_real="${qemu_bin}"
    else
        # macOS fallback: follow symlink chain manually
        qemu_real="${qemu_bin}"
        while [ -L "${qemu_real}" ]; do
            local link_target
            link_target=$(readlink "${qemu_real}")
            # Handle relative symlinks
            if [[ "${link_target}" != /* ]]; then
                qemu_real="$(dirname "${qemu_real}")/${link_target}"
            else
                qemu_real="${link_target}"
            fi
        done
    fi

    # /path/to/bin/qemu-system-x86_64 -> /path/to/share
    qemu_prefix="$(dirname "$(dirname "${qemu_real}")")"
    share_path="${qemu_prefix}/share"

    # Validate: must contain qemu firmware directory
    if [ -d "${share_path}/qemu" ]; then
        echo "${share_path}"
        return 0
    fi

    # Fallback for system installations
    echo "/usr/share"
}
