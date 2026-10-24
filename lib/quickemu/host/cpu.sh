# Host CPU core count.
host_cpu_cores() {
    nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 1
}

# Host acceleration name: kvm | hvf | tcg.
host_accel_name() {
    if [ "${OS_KERNEL:-$(uname -s)}" = "Darwin" ]; then
        echo hvf
    elif [ -w /dev/kvm ]; then
        echo kvm
    else
        echo tcg
    fi
}
