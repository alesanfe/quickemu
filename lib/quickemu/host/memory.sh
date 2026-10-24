# Host memory helpers (see capabilities.sh for free-mem probe).
host_total_mem_mb() {
    awk '/MemTotal/{print int($2/1024)}' /proc/meminfo 2>/dev/null ||
        sysctl -n hw.memsize 2>/dev/null | awk '{print int($1/1048576)}'
}
