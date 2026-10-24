# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# QEMU process spawn: writes the .sh launcher, starts the guest and
# verifies the process stayed up, cleaning runtime state on failure.

function qemu_spawn() {
    echo "${QEMU}" "${SHELL_ARGS}" "2>/dev/null" >>"${VMDIR}/${VMNAME}.sh"
    sed -i -e 's/ -/ \\\n    -/g' "${VMDIR}/${VMNAME}.sh"
    "${QEMU}" "$@" &>"${VMDIR}/${VMNAME}.log" &
    VM_PID=$!
    sleep 0.25
    if kill -0 "${VM_PID}" 2>/dev/null; then
        echo " - Process:  Started ${VM} as ${VMNAME} (${VM_PID})"
        configure_cpu_pinning
    else
        echo " - Process:  ERROR! Failed to start ${VM} as ${VMNAME}"
        rm -f "${VMDIR}/${VMNAME}.pid"
        rm -f "${VMDIR}/${VMNAME}.spice"
        rm -f "${VMDIR}/${VMNAME}.sock"
        stop_virtiofsd
        echo && cat "${VMDIR}/${VMNAME}.log"
        exit 1
    fi
}

# Initialise the generated launch script for this boot.
# Skipped in plan mode (--dry-run): the .sh is an execution artifact.
function runtime_launcher_init() {
    [ -z "${DRY_RUN}" ] && echo "#!/usr/bin/env bash" >"${VMDIR}/${VMNAME}.sh"
}

# True when the swtpm control socket will be usable for this boot —
# either already created by the running daemon or planned (--dry-run).
function swtpm_socket_ready() {
    [ -n "${DRY_RUN}" ] || [ -S "${VMDIR}/${VMNAME}.swtpm-sock" ]
}
