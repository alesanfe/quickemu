# Plugin system (item 15): any executable named quickemu-plugin-<name>
# on PATH becomes a `quickemu --plugin <name> [args]` command. Plugins
# get VMDIR/VMNAME/OUTPUT_FORMAT/QE_SCHEMA_VERSION in env — the same
# contract the helpers use, so plugins can emit envelopes too.
plugin_dispatch() { # name [args...]
    local NAME="${1}"; shift
    local BIN="quickemu-plugin-${NAME}"
    command -v "${BIN}" >/dev/null 2>&1 || {
        [ "${OUTPUT_FORMAT:-}" == "json" ] && qe_fail "vm.plugin" NOT_FOUND "plugin '${NAME}' not installed" ||             echo "ERROR! Plugin 'quickemu-plugin-${NAME}' not found on PATH." >&2
        return 1
    }
    exec "${BIN}" "$@"
}
plugin_list() { # installed plugins
    compgen -c quickemu-plugin- 2>/dev/null | sort -u | sed 's/^quickemu-plugin-//'
}
