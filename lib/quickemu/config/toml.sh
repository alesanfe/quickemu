# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Minimal TOML reader/writer for the declarative VM config format.
# Supports the subset quickemu needs: [section] headers, key = "string" |
# 123 | true/false | ["a","b"] at top level or inside one-level sections.
# Comments (#) and blank lines are ignored; malformed lines fail loudly.

QE_TOML_FORMAT_VERSION=1

# toml_parse <file>: emits `key<TAB>value` lines. Sections flatten as
# section.key. Values are unquoted strings / numbers / true|false /
# JSON-style string arrays emitted as comma lists.
toml_parse() { # file -> key\tvalue lines on stdout
    local file="${1}" line="" section="" key="" val=""
    [ -f "${file}" ] || return 1
    while IFS= read -r line || [ -n "${line}" ]; do
        line="${line%%#*}"
        line="${line#"${line%%[![:space:]]*}"}"
        line="${line%"${line##*[![:space:]]}"}"
        [ -z "${line}" ] && continue
        case "${line}" in
        \[*\])
            section="${line#\[}"; section="${section%\]}"
            [[ "${section}" =~ ^[A-Za-z0-9._-]+$ ]] || return 1
            ;;
        *=*)
            key="${line%%=*}"; val="${line#*=}"
            key="${key%"${key##*[![:space:]]}"}"; key="${key#"${key%%[![:space:]]*}"}"
            val="${val#"${val%%[![:space:]]*}"}"; val="${val%"${val##*[![:space:]]}"}"
            [[ "${key}" =~ ^[A-Za-z0-9_-]+$ ]] || return 1
            # Strip a single layer of double/single quotes.
            case "${val}" in
            \"*\") val="${val#\"}"; val="${val%\"}" ;;
            \'*\') val="${val#\'}"; val="${val%\'}" ;;
            \[*\]) val="${val#\[}"; val="${val%\]}" ;;
            esac
            [ -n "${section}" ] && key="${section}.${key}"
            printf '%s\t%s\n' "${key}" "${val}"
            ;;
        *) return 1 ;;
        esac
    done <"${file}"
}

# toml_validate <file>: 0 when the file parses cleanly and only uses keys
# from the known config schema.
toml_validate() { # file -> 0 ok / 1 malformed or unknown key
    local parsed="" k=""
    local KNOWN="formatVersion extra_disks extra_nics extra_macs guest_os arch disk_img disk_format disk_size iso img fixed_iso floppy ram cpu_cores network bridge macaddr ssh_port spice_port display monitor serial sound_card sound_duplex public_dir seed kernel initrd boot_args boot tpm secureboot gl width height vnc_listen monitor_cmd port_forwards usb_devices gpu_passthrough viewer max_outputs cpu_pinning"
    parsed=$(toml_parse "${1}") || return 1
    while IFS=$'\t' read -r k _; do
        [ -z "${k}" ] && continue
        case " ${KNOWN} " in *" ${k%%.*} "*) ;; *) return 1 ;; esac
    done <<<"${parsed}"
    return 0
}

# toml_from_conf <conf-file>: convert the legacy bash .conf into TOML on
# stdout. Uses the non-executing conf inspection (conf_get) so a hostile
# conf is never sourced.
toml_from_conf() { # conf-file -> toml on stdout
    local conf="${1}" k v
    printf '# Migrated by quickemu --migrate-conf\n'
    printf 'formatVersion = %s\n\n' "${QE_TOML_FORMAT_VERSION}"
    for k in guest_os arch disk_img disk_format disk_size iso img fixed_iso floppy ram cpu_cores network bridge macaddr ssh_port spice_port display monitor serial sound_card sound_duplex public_dir seed kernel initrd boot_args boot tpm secureboot gl width height vnc_listen monitor_cmd port_forwards gpu_passthrough viewer; do
        v=$(conf_get "${conf}" "${k}" 2>/dev/null)
        [ -n "${v}" ] && printf '%s = "%s"\n' "${k}" "${v//\"/\\\"}"
    done
    return 0
}

# conf_migrate <conf> <toml-out>: write the TOML migration next to the
# original (original is never modified).
conf_migrate() { # conf toml-out -> 0 ok
    local conf="${1}" out="${2}"
    [ -f "${conf}" ] || return 1
    [ -e "${out}" ] && { echo "ERROR! ${out} already exists." >&2; return 1; }
    toml_from_conf "${conf}" >"${out}" || return 1
    toml_validate "${out}" || { rm -f "${out}"; return 1; }
    echo " - Migrated ${conf} -> ${out} (original kept)"
}
