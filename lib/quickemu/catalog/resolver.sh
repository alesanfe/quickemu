# shellcheck shell=bash
# Catalog resolver: pick the release/option rows for an OS code from CSV.
catalog_lookup() { # csvfile oscode -> matching rows on stdout
    awk -F',' -v os="${2}" 'NR>1 && $2==os' "${1}"
}
