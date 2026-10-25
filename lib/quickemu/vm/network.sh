# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# VM network domain helpers: classify and resolve the network= conf value.

vm_net_mode() { # network value -> user|none|restrict|passt|bridge
    case "${1:-}" in
    "" | slirp)            echo user ;;
    none | restrict | passt) echo "${1}" ;;
    *)                     echo bridge ;;
    esac
}

vm_net_bridge() { # network value -> bridge iface name when mode is bridge
    [ "$(vm_net_mode "${1}")" == "bridge" ] && printf '%s' "${1}"
}
