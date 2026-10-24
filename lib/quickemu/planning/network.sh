# Network plan gates.
plan_check_net() { # network name -> 0 ok, 1 missing bridge
    local net="${1}"
    case "${net}" in
    "" | none | restrict | passt | slirp) return 0 ;;
    *) [ -d "/sys/class/net/${net}" ] ;;
    esac
}
