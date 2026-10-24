# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# PID file helpers for VM liveness.
vm_pid_alive() { # pidfile -> 0 if process alive
    local pid
    [ -f "${1}" ] || return 1
    pid=$(tr -d '[:space:]' <"${1}")
    [ -n "${pid}" ] && kill -0 "${pid}" 2>/dev/null
}

# True when the VM's pidfile exists and its process is alive.
vm_running() { vm_pid_alive "${1}/${2:-$(basename "${1}" .conf)}.pid"; }
