# QMP socket helper — send a command to a unix socket.
qmp_cmd() { # socket json -> response on stdout
    printf '%s\n' "${2}" | socat - "UNIX-CONNECT:${1}" 2>/dev/null
}
