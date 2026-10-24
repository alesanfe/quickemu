# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# JSON output helpers (public contract). Owns the schema version emitted by
# every JSON surface so schemas/v1/*.schema.json and the emitters cannot drift.

QE_SCHEMA_VERSION=1

json_escape() {
    local s="${1//\\/\\\\}"
    s="${s//\"/\\\"}"
    s="${s//$'\n'/\\n}"
    printf '%s' "${s}"
}

