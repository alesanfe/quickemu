# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# Filesystem helpers shared across modules.

qe_safe_rm() { # path -> remove file if present; missing paths are not an error
    [ -n "${1}" ] || return 0
    rm -f -- "${1}"
}
