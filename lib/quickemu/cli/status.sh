# shellcheck shell=bash
# VM state queries (public contract, schema v1).

function status_vm() {
    # Report the real VM state instead of only the presence of a pid file.
    local STATE="stopped" PID_JSON="null"
    if [ -n "${VM_PID}" ]; then
        STATE="running"
        PID_JSON="${VM_PID}"
    fi
    if [ "${OUTPUT_FORMAT}" == "json" ]; then
        printf '{"schemaVersion":1,"name":"%s","state":"%s","pid":%s}\n' \
            "${VMNAME}" "${STATE}" "${PID_JSON}"
        exit 0
    fi
    if [ -n "${VM_PID}" ]; then
        echo "running ${VM_PID}"
        exit 0
    fi
    echo "stopped"
    exit 0
}

function list_vms() {
    local CONFS=() CONF="" DIR="" OS="" STATE="" PID="" FIRST=1
    shopt -s nullglob
    CONFS=(*.conf)
    shopt -u nullglob
    if [ ${#CONFS[@]} -eq 0 ]; then
        if [ "${OUTPUT_FORMAT}" == "json" ]; then
            echo '{"schemaVersion":1,"vms":[]}'
        else
            echo "No VM configurations found in $(pwd)."
        fi
        return 0
    fi
    if [ "${OUTPUT_FORMAT}" == "json" ]; then
        printf '{"schemaVersion":1,"vms":['
        for CONF in "${CONFS[@]}"; do
            DIR="$(basename "${CONF}" .conf)"
            OS=$(sed -n 's/^guest_os="*\([^"]*\)"*/\1/p' "${CONF}" | head -1)
            STATE="stopped"
            PID="null"
            if [ -f "${DIR}/${DIR}.pid" ]; then
                PID=$(cat "${DIR}/${DIR}.pid")
                if vm_pid_alive "${DIR}/${DIR}.pid"; then
                    STATE="running"
                else
                    STATE="stale"
                    PID="null"
                fi
            fi
            [ ${FIRST} -eq 0 ] && printf ','
            FIRST=0
            printf '{"name":"%s","guest_os":"%s","state":"%s","pid":%s}' \
                "${DIR}" "${OS:-linux}" "${STATE}" "${PID}"
        done
        printf ']}\n'
        return 0
    fi
    printf '%-28s %-16s %-9s %s\n' "VM" "GUEST OS" "STATE" "PID"
    for CONF in "${CONFS[@]}"; do
        DIR="$(basename "${CONF}" .conf)"
        OS=$(sed -n 's/^guest_os="*\([^"]*\)"*/\1/p' "${CONF}" | head -1)
        STATE="stopped"
        PID=""
        if [ -f "${DIR}/${DIR}.pid" ]; then
            PID=$(cat "${DIR}/${DIR}.pid")
            if vm_pid_alive "${DIR}/${DIR}.pid"; then
                STATE="running"
            else
                STATE="stale"
                PID=""
            fi
        fi
        printf '%-28s %-16s %-9s %s\n' "${DIR}" "${OS:-linux}" "${STATE}" "${PID}"
    done
}

function capabilities() {
    # Public contract for external consumers (quickgui, scripts, CI).
    # Keep this in sync with schemas/capabilities-v1.schema.json.
    cat <<JSON
{
  "schemaVersion": 1,
  "applicationVersion": "${VERSION}",
  "capabilities": {
    "structuredCatalog": true,
    "structuredStatus": true,
    "listVms": true,
    "snapshots": true,
    "monitorCommands": true,
    "sendKeys": true,
    "screenshot": true,
    "clone": true,
    "rename": true,
    "exportImport": true,
    "diskOps": true,
    "downloadProgress": false,
    "safeDeletePlan": false
  }
}
JSON
}
