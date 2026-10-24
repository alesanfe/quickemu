# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# .ports file parsing (key=value runtime state).

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
