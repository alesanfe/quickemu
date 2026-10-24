# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# Contract emitters: the machine-readable output boundary (schemas/v1).
# The sibling emitters for --status/--list live inline in cli/status.sh.

# Emit the dry-run execution plan in the requested output format.
emit_execution_plan() {
    local vm_name="$1" command="$2"
    if [ "${OUTPUT_FORMAT}" == "json" ]; then
        printf '{"schemaVersion":%s,"vm":"%s","command":"%s"}\n' \
            "${QE_SCHEMA_VERSION}" "${vm_name}" "$(json_escape "${command}")"
    else
        echo "${command}"
    fi
}
