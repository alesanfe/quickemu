# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# Host TCP port allocation for guests and .ports runtime state.

function get_port() {
    local PORT_START=$1
    local PORT_RANGE=$((PORT_START + $2))
    local PORT
    for ((PORT = PORT_START; PORT <= PORT_RANGE; PORT++)); do
        # Make sure port scans do not block too long.
        timeout 0.1s bash -c "echo >/dev/tcp/127.0.0.1/${PORT}" >/dev/null 2>&1
        if [ ${?} -eq 1 ]; then
            echo "${PORT}"
            break
        fi
    done
}

function parse_ports_from_file {
    local FILE="${VMDIR}/${VMNAME}.ports"
    local host_name=""
    local port_name=""
    local port_number=""

    # Loop over each line in the file
    while IFS= read -r CONF || [ -n "${CONF}" ]; do
        # parse ports
        port_name=$(echo "${CONF}" | cut -d',' -f 1)
        port_number=$(echo "${CONF}" | cut -d',' -f 2)
        host_name=$(echo "${CONF}" | awk 'FS="," {print $3,"."}')

        if [ "${port_name}" == "ssh" ]; then
            ssh_port="${port_number}"
        elif [ "${port_name}" == "spice" ]; then
            spice_port="${port_number}"
        elif [ "${port_name}" == "monitor-telnet" ]; then
            monitor_telnet_port="${port_number}"
            monitor_telnet_host="${host_name}"
        elif [ "${port_name}" == "serial-telnet" ]; then
            serial_telnet_port="${port_number}"
            serial_telnet_host="${host_name}"
        fi
    done <"${FILE}"
}

# Record an exposed host endpoint for this boot in the runtime .ports file.
# Skipped in plan mode (--dry-run): the file is execution state.
function ports_record() {
    [ -z "${DRY_RUN}" ] && echo "${1}" >>"${VMDIR}/${VMNAME}.ports"
}

# Record where the SPICE viewer should connect (socket path or TCP port).
# Skipped in plan mode (--dry-run).
function spice_record() {
    [ -z "${DRY_RUN}" ] && echo "${1}" >"${VMDIR}/${VMNAME}.spice"
}
