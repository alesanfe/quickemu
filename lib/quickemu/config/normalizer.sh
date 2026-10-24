# shellcheck shell=bash
# Normalisation of values read from key="value" .conf files.

conf_normalize_bool() { # value -> "on" for truthy spellings, "off" otherwise
    case "${1,,}" in
        on|yes|true|1) echo "on" ;;
        *)             echo "off" ;;
    esac
}
