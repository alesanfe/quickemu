# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# VM identity: what a VM is — name, config dir, disk dir, artefact paths.
vm_identify() { # conf_path -> sets VMNAME VMDIR VMPATH SOCKET_MONITOR SOCKET_SERIAL
    VMDIR=$(dirname "${disk_img}")          # directory the VM disk and state files are stored
    VMNAME=$(basename "${1}" .conf)         # name of the VM
    VMPATH=$(realpath "$(dirname "${1}")")  # path to the top-level VM directory
    SOCKET_MONITOR="${VMDIR}/${VMNAME}-monitor.socket"
    SOCKET_SERIAL="${VMDIR}/${VMNAME}-serial.socket"
}
