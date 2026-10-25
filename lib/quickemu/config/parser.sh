# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# Parsing of raw key="value" lines from .conf files.

conf_parse_value() { # raw key="value" line -> unquoted value on stdout
    local line="${1#*=}"
    line="${line%\"}"
    line="${line#\"}"
    printf '%s' "${line}"
}
