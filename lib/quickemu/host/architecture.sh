# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# Host CPU architecture normalised to qemu terms.
host_arch() { # -> x86_64 | aarch64 | riscv64 ...
    local a
    a=$(uname -m)
    case "${a}" in
    arm64) echo aarch64 ;;
    *) echo "${a}" ;;
    esac
}
