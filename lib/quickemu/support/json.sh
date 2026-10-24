# shellcheck shell=bash
# JSON output helpers (public contract, schema v1).

json_escape() {
    local s="${1//\\/\\\\}"
    s="${s//\"/\\\"}"
    s="${s//$'\n'/\\n}"
    printf '%s' "${s}"
}
