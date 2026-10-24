# PID file helpers for VM liveness.
vm_pid_alive() { # pidfile -> 0 if process alive
    local pid
    [ -f "${1}" ] || return 1
    pid=$(tr -d '[:space:]' <"${1}")
    [ -n "${pid}" ] && kill -0 "${pid}" 2>/dev/null
}
