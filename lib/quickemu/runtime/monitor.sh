# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# shellcheck disable=SC2154 # monitor/monitor_telnet_* are quickemu runtime globals
# QEMU monitor socket command sender.

# ---- qmp.sh ----
# Minimal QMP (QEMU Machine Protocol) channel: machine-readable command
# and state access next to the human HMP monitor. QEMU serves it on
# <vm>-qmp.socket when launched with `-qmp unix:...,server,nowait`.

vm_qmp_socket_ready() { [ -S "${SOCKET_QMP}" ]; }

# Send one QMP command and print every reply line (greeting, caps
# handshake reply, command reply, async events if any). Needs socat.
qmp_command() { # json-command -> reply lines
    [ -n "${1}" ] || return 1
    vm_qmp_socket_ready || return 1
    command -v socat >/dev/null 2>&1 || return 1
    printf '{"execute":"qmp_capabilities"}\n%s\n' "${1}" |
        socat -t "${QMP_TIMEOUT:-1}" -,shut-down unix-connect:"${SOCKET_QMP}" 2>/dev/null
}

# Last {"return":...} object of a query (the command reply), empty on
# error/absent channel.
qmp_query() { # json-command -> {"return":...} or empty
    qmp_command "${1}" | grep -o '{"return".*}' | tail -1
}

# VM run state via QMP query-status: running|paused|shutdown|...
# Empty when the QMP channel is unavailable.
qmp_vm_status() {
    qmp_query '{"execute":"query-status"}' | sed -n 's/.*"status":"\([^"]*\)".*/\1/p'
}

# True when the QEMU monitor unix socket is present for this VM.
# Extracted so callers (pause/resume/shutdown/screenshot) share one check.
function vm_monitor_socket_ready() {
    [ -S "${SOCKET_MONITOR}" ]
}

# Unified guest control: prefer the structured QMP channel, fall back to
# the human monitor (HMP). Args: <hmp command> <qmp execute json>.
# Returns 0 only when a channel accepted the command.
vm_control() { # hmp-cmd qmp-json -> 0 ok / 1 no channel or rejected
    local hmp="${1}" qmp="${2}"
    if vm_qmp_socket_ready && command -v socat >/dev/null 2>&1; then
        qmp_query "${qmp}" | grep -q '"return"' && return 0
        # QMP up but the command errored: fall through to HMP rather
        # than silently failing (e.g. command unsupported on this QEMU).
    fi
    monitor_send_cmd "${hmp}"
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
