# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# Normalisation of values read from key="value" .conf files.

conf_normalize_bool() { # value -> "on" for truthy spellings, "off" otherwise
    case "${1,,}" in
        on|yes|true|1) echo "on" ;;
        *)             echo "off" ;;
    esac
}
