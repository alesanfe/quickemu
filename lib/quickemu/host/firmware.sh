# Firmware detection for the guest arch.
host_has_uefi() {
    [ -d /sys/firmware/efi ] || [ "${OS_KERNEL}" == "Darwin" ]
}
