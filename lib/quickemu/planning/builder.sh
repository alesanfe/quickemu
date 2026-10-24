# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# Execution-plan validation helpers. Gates that run before QEMU.

plan_check_ram() { # ram (e.g. 4G) -> 0 warning (insufficient), 1 ok
    local want="${1%[GMK]}" free
    is_numeric "${want}" || return 1 # non-numeric input: no warning
    free=$(host_free_mem_mb)
    [ -n "${free}" ] && [ "${free}" -gt 0 ] && [ $((want * 1024)) -gt "${free}" ]
}

plan_check_tpm() { # tpm=on requires swtpm on host
    [ "${1}" == "on" ] && ! host_has_tool swtpm
}

plan_check_net() { # network name -> 0 ok / not applicable, 1 missing bridge
    case "${1}" in
    "" | none | restrict | passt | slirp) return 0 ;;
    *) [ -d "/sys/class/net/${1}" ] ;;
    esac
}
