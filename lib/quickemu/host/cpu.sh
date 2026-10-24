# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# Host CPU core count.
host_cpu_cores() {
    nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 1
}

function get_nproc() {
    if command -v nproc &>/dev/null; then
        nproc
    elif command -v sysctl &>/dev/null; then
        sysctl -n hw.ncpu
    else
        echo "ERROR! Unable to determine the number of processing units."
        exit 1
    fi
}

function get_cpu_info() {
    local INFO_NAME="${1}"

    if [ "${OS_KERNEL}" == "Darwin" ]; then
        if [ "^Model name:" == "${INFO_NAME}" ]; then
            sysctl -n machdep.cpu.brand_string
        elif [ "Socket" == "${INFO_NAME}" ]; then
            sysctl -n hw.packages
        elif [ "^Vendor ID" == "${INFO_NAME}" ]; then
            if [ "${ARCH_HOST}" == "arm64" ]; then
                sysctl -n machdep.cpu.brand_string | cut -d' ' -f1
            else
                sysctl -n machdep.cpu.vendor | sed 's/ //g'
            fi
        else
            echo "ERROR! Could not find macOS translation for ${INFO_NAME}"
            exit 1
        fi
    else
        if [ "^Model name:" == "${INFO_NAME}" ]; then
            for MODEL_NAME in $(IFS=$'\n' lscpu | grep "${INFO_NAME}" | cut -d':' -f2 | sed -e 's/^[[:space:]]*//'); do
                echo -n "${MODEL_NAME} "
            done
        else
            lscpu | grep -E "${INFO_NAME}" | cut -d':' -f2 | sed 's/ //g' | sort -u
        fi
    fi
}

function check_cpu_flag() {
    local HOST_CPU_FLAG=""
    if [ "${OS_KERNEL}" == "Darwin" ]; then
        # Make the macOS compatible: uppercase, replace _ with . and replace X2APIC with x2APIC
        HOST_CPU_FLAG="${1^^}"
        HOST_CPU_FLAG="${HOST_CPU_FLAG//_/.}"
        HOST_CPU_FLAG="${HOST_CPU_FLAG//X2APIC/x2APIC}"
        if [ "${HOST_CPU_FLAG}" == "AVX" ]; then
            HOST_CPU_FLAG="AVX1.0"
        fi
        if sysctl -n machdep.cpu.features | grep -o "${HOST_CPU_FLAG}" >/dev/null; then
            return 0
        else
            return 1
        fi
    else
        HOST_CPU_FLAG="${1}"
        if lscpu | grep -o "^Flags\b.*: .*\b${HOST_CPU_FLAG}\b" >/dev/null; then
            return 0
        else
            # AMD CPUs report invtsc as constant_tsc; check for equivalence
            if [ "${HOST_CPU_FLAG}" == "invtsc" ] && [ "${HOST_CPU_VENDOR}" == "AuthenticAMD" ]; then
                if lscpu | grep -o "^Flags\b.*: .*\bconstant_tsc\b" >/dev/null; then
                    return 0
                fi
            fi
            return 1
        fi
    fi
}
