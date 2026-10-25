# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# Catalog: resolve release/option rows for an OS code and validate entries.

# ---- resolver.sh ----
# Catalog resolver: pick the release/option rows for an OS code from CSV.
catalog_lookup() { # csvfile oscode -> matching rows on stdout
    awk -F',' -v os="${2}" 'NR>1 && $2==os' "${1}"
}

# ---- validator.sh ----
# Catalog validation: required fields per entry.
catalog_entry_valid() { # display_name os release
    [ -n "${1}" ] && [ -n "${2}" ] && [ -n "${3}" ]
}
