# qemu-img adapter — image ops behind a named interface.
qimg_info() { "${QEMU_IMG:-qemu-img}" info "${1}" 2>/dev/null; }
