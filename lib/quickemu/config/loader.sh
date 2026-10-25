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

# Load a VM config: legacy .conf is executable Bash (sourced, trusting
# the file); .toml is declarative — parsed and assigned, never executed.
vm_config_load() { # conf-or-toml -> 0 ok / 1 invalid
    local file="${1}" k v
    case "${file}" in
    *.toml)
        toml_validate "${file}" || {
            echo "ERROR! Invalid TOML config ${file}." >&2
            return 1
        }
        while IFS=$'\t' read -r k v; do
            [ -z "${k}" ] && continue
            case "${k}" in
            formatVersion) : ;;                        # schema marker, not a conf var
            *.*) echo "WARNING! Ignoring nested TOML key '${k}' (flat config model)." >&2 ;;
            *) printf -v "${k}" '%s' "${v}" ;;
            esac
        done < <(toml_parse "${file}")
        ;;
    *)
        # shellcheck source=/dev/null
        source "${file}"
        ;;
    esac
}
