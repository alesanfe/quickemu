# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# Logging helpers: runtime log of a running VM.

# Append a note to the VM runtime log. Skipped in plan mode: the .log is
# an execution artifact.
function vm_log() {
    [ -z "${DRY_RUN}" ] && echo "${1}" >>"${VMDIR}/${VMNAME}.log"
}
