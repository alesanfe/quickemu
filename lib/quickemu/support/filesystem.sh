# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# Filesystem helpers shared across modules.

qe_safe_rm() { # path -> remove file if present; missing paths are not an error
    [ -n "${1}" ] || return 0
    rm -f -- "${1}"
}

# get_qemu_share_path (was host/capabilities.sh)
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

# freedesktop/XDG paths for per-user state. XDG_RUNTIME_DIR is the right
# home for sockets, pids and the port registry on Linux; macOS/unset fall
# back to TMPDIR. Data/state dirs are used by trash/backups.
qe_runtime_dir() { printf '%s/quickemu-%s' "${XDG_RUNTIME_DIR:-${TMPDIR:-/tmp}}" "${UID}"; }
qe_state_dir() { printf '%s/quickemu' "${XDG_STATE_HOME:-${HOME}/.local/state}"; }
qe_data_dir() { printf '%s/quickemu' "${XDG_DATA_HOME:-${HOME}/.local/share}"; }
