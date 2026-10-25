# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# Execution-plan validation helpers. Gates that run before QEMU.

plan_check_ram() { # ram (e.g. 4G) -> 0 warning (insufficient), 1 ok
    local want="${1%[GMK]}" free
    is_numeric "${want}" || return 1 # non-numeric input: no warning
    free=$(host_free_mem_mb)
    [ -n "${free}" ] && [ "${free}" -gt 0 ] && [ $((want * 1024)) -gt "${free}" ]
}

plan_check_tpm() { # tpm=on requires swtpm on host
    [ "${1}" == "on" ] && ! host_has_tool swtpm
}

plan_check_net() { # network name -> 0 ok / not applicable, 1 missing bridge
    [ "$(vm_net_mode "${1}")" == "bridge" ] || return 0
    host_iface_exists "${1}"
}

# Pre-execution gate: warn (and unless bypassed, prompt/abort) when the
# host is at risk of unstable-TSC freezes for macOS Ventura+ on AMD.
# The probe stays in host/; this gate owns warning UX, logging and abort.
plan_check_tsc() {
    host_macos_tsc_unstable || return 0

    if [ "${IGNORE_TSC_WARNING}" == "1" ]; then
        echo " - TSC:      WARNING! Unstable TSC detected (clocksource: ${TSC_CLOCKSOURCE})"
        echo "             Proceeding anyway due to --ignore-tsc-warning flag."
        return 0
    fi

    # Display warning and prompt user
    echo " - TSC:      WARNING! Unstable TSC detected (clocksource: ${TSC_CLOCKSOURCE})"
    echo "             macOS ${macos_release^} may freeze on AMD Ryzen mobile CPUs."
    echo
    echo "             Fix: Add 'tsc=reliable' to kernel boot parameters and reboot."
    echo "             Or:  Use macOS Big Sur (11) or Monterey (12) instead."
    echo "             See: https://github.com/quickemu-project/quickemu/wiki/03-Create-macOS-virtual-machines#tsc-instability-on-amd-ryzen-mobile-cpus"
    echo

    # Planning only (--dry-run): report the warning, never prompt or write logs.
    [ -n "${DRY_RUN}" ] && return 0

    # Log the warning
    vm_log "TSC_WARNING: clocksource=${TSC_CLOCKSOURCE} macos_release=${macos_release} cpu_vendor=${HOST_CPU_VENDOR}"

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
