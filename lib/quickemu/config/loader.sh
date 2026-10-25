# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# Safe reading of key="value" .conf entries without sourcing the file.
# The value parsing itself lives in config/parser.sh.
conf_get() {
    local file="${1}" key="${2}" line
    [ -f "${file}" ] || return 1
    line=$(grep -m1 "^${key}=" "${file}") || return 1
    conf_parse_value "${line}"
}
