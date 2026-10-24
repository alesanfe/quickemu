# VM lifecycle helpers shared by start/stop/kill actions.
vm_running() { vm_pid_alive "${1}/${2:-$(basename "${1}" .conf)}.pid"; }
