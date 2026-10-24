# Storage plan gates.
plan_check_disk_free() { # dir bytes-needed -> 0 ok, 1 insufficient
    local avail
    avail=$(df -k "${1}" 2>/dev/null | awk 'NR==2{print $4}')
    [ -n "${avail}" ] && [ $((avail * 1024)) -ge "${2}" ]
}
