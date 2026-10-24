# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# Host display-server capability probes.

function check_cocoa_gl_es_support() {
    [ "${OS_KERNEL}" != "Darwin" ] && return 1

    # Test QEMU directly for gl=es support - most reliable method
    # This catches both missing OpenGL build support and missing ANGLE libraries
    if "${QEMU}" -display cocoa,gl=es -M none 2>&1 | grep -Eqi "OpenGL support was not enabled|does not accept"; then
        return 1
    fi

    # Fallback: check for ANGLE libraries if QEMU test is inconclusive
    # Resolve QEMU's real path (follows Nix symlinks)
    local qemu_real qemu_dir qemu_prefix
    qemu_real=$(realpath "${QEMU}" 2>/dev/null || readlink -f "${QEMU}" 2>/dev/null || echo "${QEMU}")
    qemu_dir=$(dirname "${qemu_real}")
    qemu_prefix="${qemu_dir%/bin}"

    local angle_libs=(
        "${qemu_prefix}/lib/libEGL.dylib"
        "/opt/homebrew/lib/libEGL.dylib"
        "/usr/local/lib/libEGL.dylib"
    )

    # Also check DYLD paths if set (covers additional Nix scenarios)
    if [ -n "${DYLD_LIBRARY_PATH:-}" ]; then
        local IFS=':'
        for path in ${DYLD_LIBRARY_PATH}; do
            angle_libs+=("${path}/libEGL.dylib")
        done
    fi

    for lib in "${angle_libs[@]}"; do
        [ -f "$lib" ] && return 0
    done

    return 1
}
