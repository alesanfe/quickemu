# shellcheck shell=bash
# shellcheck disable=SC2034,SC2154
# Sourced partial: shares the conf-schema global namespace with bin/quickemu.
# VM health report (config vs host capabilities).

function doctor_vm() {
    local ISSUES=0
    echo "Checking ${VMNAME} against host capabilities:"
    if [ "${OS_KERNEL}" == "Darwin" ]; then
        echo " o Host: macOS (HVF acceleration)"
    elif host_has_kvm; then
        echo " o KVM:  /dev/kvm available"
    else
        echo " x KVM:  /dev/kvm not writable, VM will use slow TCG emulation"
        ISSUES=$((ISSUES + 1))
    fi
    if [ ! -x "${QEMU}" ]; then
        echo " x QEMU: ${QEMU} not executable"
        ISSUES=$((ISSUES + 1))
    fi
    if [ -n "${disk_img}" ] && [ ! -f "${disk_img}" ]; then
        echo " x Disk: ${disk_img} missing (will be created, ${disk_size})"
    fi
    if [ -n "${iso}" ] && [ ! -f "${iso}" ]; then
        echo " x ISO:  ${iso} missing"
        ISSUES=$((ISSUES + 1))
    fi
    if [[ "${boot}" == *"efi"* ]]; then
        if ls "${VMDIR}"/OVMF_CODE*.fd "${VMDIR}"/AAVMF*.fd >/dev/null 2>&1 || command -v "${QEMU}" >/dev/null; then
            echo " o EFI:  firmware files present"
        else
            echo " x EFI:  no OVMF/AAVMF firmware found"
            ISSUES=$((ISSUES + 1))
        fi
    fi
    if [ -n "${ram}" ] && is_numeric "${ram%[GMK]}"; then
        if plan_check_ram "${ram}"; then
            echo " x RAM:  VM wants ${ram} but only $(host_free_mem_mb)M available"
            ISSUES=$((ISSUES + 1))
        else
            echo " o RAM:  ${ram} requested ($(host_free_mem_mb)M free)"
        fi
    fi
    if plan_check_tpm "${tpm}"; then
        echo " x TPM:  tpm=on but swtpm not installed"
        ISSUES=$((ISSUES + 1))
    fi
    case "${network}" in
    "" | none | restrict | passt | slirp) ;;
    *)
        if plan_check_net "${network}"; then
            echo " o NET:  bridge ${network} up"
        else
            echo " x NET:  bridge interface ${network} does not exist"
            ISSUES=$((ISSUES + 1))
        fi ;;
    esac
    if [ "${ISSUES}" -eq 0 ]; then
        echo " - No problems detected."
    else
        echo " - ${ISSUES} issue(s) found."
    fi
    return 0
}
