# shellcheck shell=bash
# Filesystem helpers shared across modules.

qe_safe_rm() { # path -> remove file if present; missing paths are not an error
    [ -n "${1}" ] || return 0
    rm -f -- "${1}"
}
