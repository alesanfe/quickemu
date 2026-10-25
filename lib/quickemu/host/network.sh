# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# Host network probes.

host_iface_exists() { # iface -> 0 when the host interface exists
    [ -d "/sys/class/net/${1}" ]
}

host_default_iface() { # -> interface owning the default route (may be empty)
    ip route show default 2>/dev/null | awk '/default/{print $5; exit}'
}
