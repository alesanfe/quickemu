# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the global namespace of bin/quickreport.
# Host probes: answer facts about the machine this report runs on.

# Print the OS display name (macOS via sw_vers, else /etc/os-release).
function probe_os_name() {
    if [ "${OS_KERNEL}" == "Darwin" ]; then
        if [ -x "$(command -v sw_vers)" ]; then
            echo "$(sw_vers -productName) $(sw_vers -productVersion)"
        else
            echo "macOS"
        fi
    elif [ -e /etc/os-release ]; then
        grep PRETTY_NAME /etc/os-release | cut -d'"' -f2
    else
        echo "Unknown OS"
    fi
}

# Print the resolved quickemu executable path, or nothing if missing.
function probe_quickemu() {
    local CWD
    CWD="$(dirname "${1:-${0}}")"
    if [ -x "${CWD}/quickemu" ]; then
        echo "${CWD}/quickemu"
    elif [ -x "$(command -v quickemu)" ]; then
        command -v quickemu
    fi
}

# Print installed RAM in GB.
function probe_memory_gb() {
    if [ "${OS_KERNEL}" == "Darwin" ]; then
        echo "$(($(sysctl -n hw.memsize) / (1048576 * 1024)))"
    else
        # Determine the number of gigabytes of RAM in the host by extracting the first numerical value from the output.
        free --giga -h | tr ' ' '\n' | grep -m 1 "[0-9]" | cut -d'G' -f 1
    fi
}

# Print GPU model names, one per line.
function probe_gpus() {
    if [ "${OS_KERNEL}" == "Darwin" ]; then
        system_profiler SPDisplaysDataType | grep "Chipset Model" | awk -F: '{print $2}' | sed 's/^ *//'
    else
        lspci | grep -i vga | cut -d':' -f3
    fi
}

# Print the QEMU architecture matching the host (arm64 -> aarch64).
function probe_qemu_arch() {
    local HOST_ARCH
    HOST_ARCH=$(uname -m)
    if [ "${HOST_ARCH}" == "arm64" ]; then
        echo "aarch64"
    else
        echo "${HOST_ARCH}"
    fi
}

# Print CPU information (lscpu / machdep brand string).
function probe_cpu_info() {
    if [ "${OS_KERNEL}" == "Darwin" ]; then
        sysctl -n machdep.cpu.brand_string
    else
        lscpu
    fi
}
