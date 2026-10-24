# Display server detection (Wayland vs X11 vs Cocoa).
host_display_server() {
    if [ -n "${WAYLAND_DISPLAY:-}" ]; then
        echo wayland
    elif [ -n "${DISPLAY:-}" ]; then
        echo x11
    elif [ "${OS_KERNEL}" == "Darwin" ]; then
        echo cocoa
    else echo headless; fi
}
