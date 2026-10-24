# Host CPU core count.
host_cpu_cores() {
    nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 1
}
