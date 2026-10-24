# Firmware plan gates (efi boot requires firmware blobs).
plan_check_firmware() { # boot mode
    [[ "${1}" == *efi* ]] && ! ls "${VMDIR}"/OVMF_CODE*.fd "${VMDIR}"/AAVMF*.fd >/dev/null 2>&1 && ! host_has_uefi
}
