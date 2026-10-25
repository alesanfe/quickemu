# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# Host CPU probes: architecture, cores, flags, KVM usability and TSC stability.

# ---- cpu.sh ----
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

# ---- architecture.sh ----
# Host CPU architecture normalised to qemu terms.
host_arch() { # -> x86_64 | aarch64 | riscv64 ...
    local a
    a=$(uname -m)
    case "${a}" in
    arm64) echo aarch64 ;;
    *) echo "${a}" ;;
    esac
}

# ---- kvm.sh ----
# Host KVM module and CPU-stability probes.

function ignore_msrs_always() {
    # Make sure the host has /etc/modprobe.d
    if [ -d /etc/modprobe.d ]; then
        # Skip if ignore_msrs is already enabled, assumes initramfs has been rebuilt
        if ! grep -lq 'ignore_msrs=Y' /etc/modprobe.d/kvm-quickemu.conf >/dev/null 2>&1; then
            echo "options kvm ignore_msrs=Y" | sudo tee /etc/modprobe.d/kvm-quickemu.conf

            # Rebuild initramfs so the new kvm option takes effect
            flag=0
            if command -v update-initramfs &>/dev/null; then
                flag=1
            elif command -v mkinitcpio &>/dev/null; then
                flag=2
            else
                flag=3
            fi

            # echo $flag;

            case "$flag" in
            1)
                sudo update-initramfs -k all -u
                ;;
            2)
                sudo mkinitcpio -P
                ;;
            3)
                echo "ERROR! User does not have update-initramfs or mkinitcpio installed, please find out respective utility to update/regenrate intiramfs."
                echo "Run respective command for rebuilding initramfs: 'sudo update-initramfs -k all -u'(debian) or 'sudo mkinitcpio -P'(arch)"
                exit 1
                ;;
            esac

        fi

    else
        echo "ERROR! /etc/modprobe.d was not found, I don't know how to configure this system."
        exit 1
    fi
}

function ignore_msrs_alert() {
    local ignore_msrs=""
    if [ "${OS_KERNEL}" == "Darwin" ]; then
        return
    elif [ -e /sys/module/kvm/parameters/ignore_msrs ]; then
        ignore_msrs=$(cat /sys/module/kvm/parameters/ignore_msrs)
        if [ "${ignore_msrs}" == "N" ]; then
            echo " - MSR:      WARNING! Ignoring unhandled Model-Specific Registers is disabled."
            echo
            echo "             echo 1 | sudo tee /sys/module/kvm/parameters/ignore_msrs"
            echo
            echo "             If you are unable to run macOS or Windows VMs then run the above 👆"
            echo "             This will enable ignoring of unhandled MSRs until you reboot the host."
            echo "             You can make this change permanent by running: 'quickemu --ignore-msrs-always'"
        fi
    fi
}

# Pure host probe: returns 0 when this machine is at risk of unstable-TSC
# freezes for the configured macOS guest; exports TSC_CLOCKSOURCE for the
# caller's report. Never prints, prompts or writes — that is the gate's job.
function host_macos_tsc_unstable() {
    # Gate 1: Only on Linux hosts
    if [ "${OS_KERNEL}" != "Linux" ]; then
        return 1
    fi

    # Gate 2: Only for AMD CPUs
    if [ "${HOST_CPU_VENDOR}" != "AuthenticAMD" ]; then
        return 1
    fi

    # Gate 3: Only for macOS guests
    if [ "${guest_os}" != "macos" ]; then
        return 1
    fi

    # Gate 4: Only for macOS Ventura (13) and newer
    case ${macos_release} in
    ventura | sonoma | sequoia | tahoe) ;;
    *) return 1 ;;
    esac

    # Gate 5: Skip if user has already set tsc=reliable in kernel cmdline
    if [ -r /proc/cmdline ] && [[ "$(cat /proc/cmdline)" == *"tsc=reliable"* ]]; then
        return 1
    fi

    # Gate 6: Check if TSC is the current clocksource (indicates stable TSC)
    local clocksource_path="/sys/devices/system/clocksource/clocksource0/current_clocksource"
    if [ ! -r "${clocksource_path}" ]; then
        # Cannot determine clocksource - assume OK and let user discover issues
        return 1
    fi
    TSC_CLOCKSOURCE=$(cat "${clocksource_path}")
    [ "${TSC_CLOCKSOURCE}" == "tsc" ] && return 1
    return 0
}

# host_has_kvm (was host/capabilities.sh)
host_has_kvm() {                               # -> 0 when KVM acceleration is usable
    [ "${OS_KERNEL}" == "Darwin" ] && return 0 # macOS uses HVF
    [ -w /dev/kvm ]
}
