# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
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

function check_macos_tsc_stability() {
    # Gate 1: Only on Linux hosts
    if [ "${OS_KERNEL}" != "Linux" ]; then
        return 0
    fi

    # Gate 2: Only for AMD CPUs
    if [ "${HOST_CPU_VENDOR}" != "AuthenticAMD" ]; then
        return 0
    fi

    # Gate 3: Only for macOS guests
    if [ "${guest_os}" != "macos" ]; then
        return 0
    fi

    # Gate 4: Only for macOS Ventura (13) and newer
    case ${macos_release} in
    ventura | sonoma | sequoia | tahoe) ;;
    *) return 0 ;;
    esac

    # Gate 5: Skip if user has already set tsc=reliable in kernel cmdline
    local cmdline=""
    if [ -r /proc/cmdline ]; then
        cmdline=$(cat /proc/cmdline)
        if [[ "${cmdline}" == *"tsc=reliable"* ]]; then
            return 0
        fi
    fi

    # Gate 6: Check if TSC is the current clocksource (indicates stable TSC)
    local clocksource_path="/sys/devices/system/clocksource/clocksource0/current_clocksource"
    local current_clocksource=""
    if [ -r "${clocksource_path}" ]; then
        current_clocksource=$(cat "${clocksource_path}")
        if [ "${current_clocksource}" == "tsc" ]; then
            return 0
        fi
    else
        # Cannot determine clocksource - assume OK and let user discover issues
        return 0
    fi

    # All gates failed - this system is at risk
    # Check if warning should be skipped
    if [ "${IGNORE_TSC_WARNING}" == "1" ]; then
        echo " - TSC:      WARNING! Unstable TSC detected (clocksource: ${current_clocksource})"
        echo "             Proceeding anyway due to --ignore-tsc-warning flag."
        return 0
    fi

    # Display warning and prompt user
    echo " - TSC:      WARNING! Unstable TSC detected (clocksource: ${current_clocksource})"
    echo "             macOS ${macos_release^} may freeze on AMD Ryzen mobile CPUs."
    echo
    echo "             Fix: Add 'tsc=reliable' to kernel boot parameters and reboot."
    echo "             Or:  Use macOS Big Sur (11) or Monterey (12) instead."
    echo "             See: https://github.com/quickemu-project/quickemu/wiki/03-Create-macOS-virtual-machines#tsc-instability-on-amd-ryzen-mobile-cpus"
    echo

    # Planning only (--dry-run): report the warning, never prompt or write logs.
    [ -n "${DRY_RUN}" ] && return 0

    # Log the warning
    echo "TSC_WARNING: clocksource=${current_clocksource} macos_release=${macos_release} cpu_vendor=${HOST_CPU_VENDOR}" >>"${VMDIR}/${VMNAME}.log"

    # Interactive prompt - check if stdin is a terminal
    if [ -t 0 ]; then
        echo -n "Do you want to continue anyway? [y/N] "
        read -r response
        case "${response}" in
        [yY] | [yY][eE][sS])
            echo " - TSC:      Proceeding despite unstable TSC warning."
            return 0
            ;;
        *)
            echo " - TSC:      Aborting. Please apply one of the solutions above."
            exit 1
            ;;
        esac
    else
        # Non-interactive mode - abort by default for safety
        echo "ERROR! Non-interactive mode detected. Use --ignore-tsc-warning to bypass this check."
        exit 1
    fi
}
