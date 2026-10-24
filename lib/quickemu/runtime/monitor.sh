# shellcheck shell=bash
# shellcheck disable=SC2154 # monitor/monitor_telnet_* are quickemu runtime globals
# QEMU monitor socket command sender.

function monitor_send_cmd {
    local MSG="${1}"

    if [ -z "${MSG}" ]; then
        echo "WARNING! Send to QEMU-Monitor: Message empty!"
        return 1
    fi

    case "${monitor}" in
    socket)
        echo -e " - Sending:  via socket ${MSG}"
        echo -e "${MSG}" | socat -,shut-down unix-connect:"${SOCKET_MONITOR}" >/dev/null 2>&1
        ;;
    telnet)
        echo -e " - Sending:  via telnet ${MSG}"
        echo -e "${MSG}" | socat - tcp:"${monitor_telnet_host}":"${monitor_telnet_port}" >/dev/null 2>&1
        ;;
    *)
        echo "WARNING! No qemu-monitor channel available - Couldn't send message to monitor!"
        return 1
        ;;
    esac

    return 0
}
