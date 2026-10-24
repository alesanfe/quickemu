# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154,SC2317
# Sourced partial: shares the global namespace of bin/quickgetc.
# Catalog: architecture support queries.


function supported_archs() {
    echo x86_64 aarch64 riscv64
}


function set_arch() {
    case "${1}" in
        x86_64|amd64) ARCH="x86_64";;
        aarch64|arm64|armv8) ARCH="aarch64";;
        riscv64|riscv) ARCH="riscv64";;
        *)
            echo "ERROR! ${1} is not a supported architecture."
            echo " - Supported architectures: $(supported_archs)"
            exit 1;;
    esac
}
