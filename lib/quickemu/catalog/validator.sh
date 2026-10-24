# shellcheck shell=bash
# Catalog validation: required fields per entry.
catalog_entry_valid() { # display_name os release
    [ -n "${1}" ] && [ -n "${2}" ] && [ -n "${3}" ]
}
