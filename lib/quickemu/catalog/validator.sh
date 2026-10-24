# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# Catalog validation: required fields per entry.
catalog_entry_valid() { # display_name os release
    [ -n "${1}" ] && [ -n "${2}" ] && [ -n "${3}" ]
}
