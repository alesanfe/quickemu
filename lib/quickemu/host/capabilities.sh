# shellcheck shell=bash
# Host capability probes (hardware, tools, resources).

host_has_kvm() {                               # -> 0 when KVM acceleration is usable
    [ "${OS_KERNEL}" == "Darwin" ] && return 0 # macOS uses HVF
    [ -w /dev/kvm ]
}

host_has_tool() { # tool -> 0 when available on PATH
    command -v "${1}" >/dev/null 2>&1
}

host_free_mem_mb() { # -> available memory in MiB (0 when unknown)
    awk '/MemAvailable/{print int($2/1024); exit}' /proc/meminfo 2>/dev/null || echo 0
}
