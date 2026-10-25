# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# shellcheck disable=SC2154 # monitor/monitor_telnet_* are quickemu runtime globals
# QEMU monitor socket command sender.

# True when the QEMU monitor unix socket is present for this VM.
# Extracted so callers (pause/resume/shutdown/screenshot) share one check.
function vm_monitor_socket_ready() {
    [ -S "${SOCKET_MONITOR}" ]
}

function monitor_send_cmd {
    local MSG="${1}"

    if [ -z "${MSG}" ]; then
        echo "WARNING! Send to QEMU-Monitor: Message empty!"
        return 1
    fi

    if ! command -v socat >/dev/null 2>&1; then
        echo "ERROR! socat is required to send monitor commands."
        return 1
    fi

    case "${monitor}" in
    socket)
        if ! vm_monitor_socket_ready; then
            echo "WARNING! No qemu-monitor socket at ${SOCKET_MONITOR} - Couldn't send message to monitor!"
            return 1
        fi
        echo -e " - Sending:  via socket ${MSG}"
        # -t N closes the connection N seconds after the last traffic, so the
        # monitor reply is captured before QEMU's persistent session ends.
        # The '(qemu) ' prompt prefix is stripped from the reply lines.
        echo -e "${MSG}" | socat -t 1 -,shut-down unix-connect:"${SOCKET_MONITOR}" 2>/dev/null | tr -d '\r' | sed 's/^(qemu) \?//'
        ;;
    telnet)
        echo -e " - Sending:  via telnet ${MSG}"
        echo -e "${MSG}" | socat -t 1 - tcp:"${monitor_telnet_host}":"${monitor_telnet_port}" 2>/dev/null | tr -d '\r' | sed 's/^(qemu) \?//'
        ;;
    *)
        echo "WARNING! No qemu-monitor channel available - Couldn't send message to monitor!"
        return 1
        ;;
    esac

    return 0
}
